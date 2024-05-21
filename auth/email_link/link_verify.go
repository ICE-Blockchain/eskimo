// SPDX-License-Identifier: ice License 1.0

package emaillinkiceauth

import (
	"context"
	"fmt"
	"strings"

	"dario.cat/mergo"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	"github.com/ice-blockchain/wintr/auth"
	"github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/time"
)

func (c *client) ResetEmailChange(ctx context.Context, emailLinkPayload, confirmationCode string) error {
	now := time.Now()
	var token magicLinkToken
	if err := parseJwtToken(emailLinkPayload, c.cfg.EmailValidation.JwtSecret, &token); err != nil {
		return errors.Wrapf(err, "invalid email link payload:%v", emailLinkPayload)
	}
	email := token.Subject
	id := loginID{Email: email, DeviceUniqueID: token.DeviceUniqueID}
	els, err := c.getEmailLinkSignInByPk(ctx, &id, token.OldEmail)
	if err != nil {
		if storage.IsErr(err, storage.ErrNotFound) {
			return errors.Wrapf(ErrNoConfirmationRequired, "[getEmailLinkSignInByPk] no pending confirmation for email:%v", id.Email)
		}

		return errors.Wrapf(err, "failed to get user info by email:%v(old email:%v)", id.Email, token.OldEmail)
	}
	if els.OTP == *els.UserID || els.OTP != token.OTP {
		return errors.Wrapf(ErrNoConfirmationRequired, "no pending confirmation for email:%v", id.Email)
	}
	_, err = c.signIn(ctx, now, els, &id, token.OldEmail, token.NotifyEmail, token.OTP, confirmationCode)
	if err != nil {
		return errors.Wrapf(err, "can't sign in for email:%v, deviceUniqueID:%v", id.Email, id.DeviceUniqueID)
	}
	if rErr := c.resetLoginSession(ctx, &id, els, confirmationCode, "", 0); rErr != nil {
		return errors.Wrapf(rErr, "can't reset login session for id:%#v", id)
	}

	return nil
}

func (c *client) SignIn(ctx context.Context, loginSession, confirmationCode string) (tokens *Tokens, emailConfirmed bool, err error) {
	now := time.Now()
	var token loginFlowToken
	if err = parseJwtToken(loginSession, c.cfg.EmailValidation.JwtSecret, &token); err != nil {
		return nil, false, errors.Wrapf(err, "invalid login flow token:%v", loginSession)
	}
	email := token.Subject
	id := loginID{Email: email, DeviceUniqueID: token.DeviceUniqueID}
	els, err := c.getEmailLinkSignInByPk(ctx, &id, token.OldEmail)
	if err != nil {
		if storage.IsErr(err, storage.ErrNotFound) {
			return nil, false, errors.Wrapf(ErrNoConfirmationRequired, "[getEmailLinkSignInByPk] no pending confirmation for email:%v", id.Email)
		}

		return nil, false, errors.Wrapf(err, "failed to get user info by email:%v(old email:%v)", id.Email, token.OldEmail)
	}
	var otp string
	emailConfirmed, err = c.signIn(ctx, now, els, &id, token.OldEmail, token.NotifyEmail, otp, confirmationCode)
	if err != nil {
		return nil, false, errors.Wrapf(err, "can't sign in for email:%v, deviceUniqueID:%v", id.Email, id.DeviceUniqueID)
	}
	els.TokenIssuedAt = now
	tokens, err = c.generateTokens(els.TokenIssuedAt, els, els.IssuedTokenSeq)
	if err != nil {
		return nil, false, errors.Wrapf(err, "can't generate tokens for id:%#v", id)
	}
	if rErr := c.resetLoginSession(ctx, &id, els, confirmationCode, token.ClientIP, token.LoginSessionNumber); rErr != nil {
		return nil, false, errors.Wrapf(rErr, "can't reset login session for id:%#v", id)
	}

	return tokens, emailConfirmed, nil
}

//nolint:funlen,revive // .
func (c *client) signIn(
	ctx context.Context, now *time.Time, els *emailLinkSignIn, id *loginID, oldEmail, notifyEmail, otp, confirmationCode string,
) (emailConfirmed bool, err error) {
	if els.UserID != nil && els.ConfirmationCode == *els.UserID {
		return false, errors.Wrapf(ErrNoPendingLoginSession, "tokens already provided for id:%#v", id)
	}
	if vErr := c.verifySignIn(ctx, els, id, confirmationCode); vErr != nil {
		return false, errors.Wrapf(vErr, "can't verify sign in for id:%#v", id)
	}
	if oldEmail != "" || (els.PhoneNumberToEmailMigrationUserID != nil && *els.PhoneNumberToEmailMigrationUserID != "") {
		if err = c.handleEmailModification(ctx, els, id.Email, oldEmail, notifyEmail); err != nil {
			return false, errors.Wrapf(err, "failed to handle email modification:%v", id.Email)
		}
		emailConfirmed = oldEmail != ""
		els.Email = id.Email
	}
	if fErr := c.finishAuthProcess(ctx, now, id, *els.UserID, otp, els.IssuedTokenSeq, emailConfirmed, els.Metadata); fErr != nil {
		var mErr *multierror.Error
		if oldEmail != "" {
			mErr = multierror.Append(mErr,
				errors.Wrapf(c.resetEmailModification(ctx, *els.UserID, oldEmail),
					"[reset] resetEmailModification failed for email:%v", oldEmail),
				errors.Wrapf(c.resetFirebaseEmailModification(ctx, els.Metadata, oldEmail),
					"[reset] resetEmailModification failed for email:%v", oldEmail),
			)
		}
		mErr = multierror.Append(mErr, errors.Wrapf(fErr, "can't finish auth process for userID:%v,email:%v", els.UserID, id.Email))

		return false, mErr.ErrorOrNil() //nolint:wrapcheck // .
	}

	return emailConfirmed, nil
}

func (c *client) verifySignIn(ctx context.Context, els *emailLinkSignIn, id *loginID, confirmationCode string) error {
	var shouldBeBlocked bool
	var mErr *multierror.Error
	if els.ConfirmationCodeWrongAttemptsCount >= c.cfg.ConfirmationCode.MaxWrongAttemptsCount {
		blockEndTime := time.Now().Add(c.cfg.EmailValidation.BlockDuration)
		blockTimeFitsNow := (els.BlockedUntil.Before(blockEndTime) && els.BlockedUntil.After(*els.CreatedAt.Time))
		if els.BlockedUntil == nil || !blockTimeFitsNow {
			shouldBeBlocked = true
		}
		if !shouldBeBlocked {
			return errors.Wrapf(ErrConfirmationCodeAttemptsExceeded, "confirmation code wrong attempts count exceeded for id:%#v", id)
		}
		mErr = multierror.Append(mErr, errors.Wrapf(ErrConfirmationCodeAttemptsExceeded, "confirmation code wrong attempts count exceeded for id:%#v", id))
	}
	if els.ConfirmationCode != confirmationCode || shouldBeBlocked {
		if els.ConfirmationCodeWrongAttemptsCount+1 >= c.cfg.ConfirmationCode.MaxWrongAttemptsCount {
			shouldBeBlocked = true
		}
		if iErr := c.increaseWrongConfirmationCodeAttemptsCount(ctx, id, shouldBeBlocked); iErr != nil {
			mErr = multierror.Append(mErr, errors.Wrapf(iErr,
				"can't increment wrong confirmation code attempts count for email:%v,deviceUniqueID:%v", id.Email, id.DeviceUniqueID))
		}
		mErr = multierror.Append(mErr, errors.Wrapf(ErrConfirmationCodeWrong, "wrong confirmation code:%v", confirmationCode))

		return mErr.ErrorOrNil() //nolint:wrapcheck // Not needed.
	}

	return nil
}

//nolint:revive // Not to create duplicated function with/without bool flag.
func (c *client) increaseWrongConfirmationCodeAttemptsCount(ctx context.Context, id *loginID, shouldBeBlocked bool) error {
	params := []any{id.Email, id.DeviceUniqueID}
	var blockSQL string
	if shouldBeBlocked {
		blockSQL = ",blocked_until = $3"
		params = append(params, time.Now().Add(c.cfg.EmailValidation.BlockDuration))
	}
	sql := fmt.Sprintf(`UPDATE email_link_sign_ins
				SET confirmation_code_wrong_attempts_count = confirmation_code_wrong_attempts_count + 1
				%v
			WHERE email = $1
				  AND device_unique_id = $2`, blockSQL)
	_, err := storage.Exec(ctx, c.db, sql, params...)

	return errors.Wrapf(err, "can't update email link sign ins for the user with pk:%#v", id)
}

//nolint:revive,funlen // .
func (c *client) finishAuthProcess(
	ctx context.Context, now *time.Time,
	id *loginID, userID, otp string, issuedTokenSeq int64,
	emailConfirmed bool, md *users.JSON,
) error {
	emailConfirmedAt := "null"
	if emailConfirmed {
		emailConfirmedAt = "$2"
	}
	mdToUpdate := users.JSON(map[string]any{auth.IceIDClaim: userID})
	if md == nil {
		empty := users.JSON(map[string]any{})
		md = &empty
	}
	if _, hasRegisteredWith := (*md)[auth.RegisteredWithProviderClaim]; !hasRegisteredWith {
		if firebaseID, hasFirebaseID := (*md)[auth.FirebaseIDClaim]; hasFirebaseID {
			if !strings.HasPrefix(firebaseID.(string), iceIDPrefix) && !strings.HasPrefix(userID, iceIDPrefix) { //nolint:forcetypeassert // .
				mdToUpdate[auth.RegisteredWithProviderClaim] = auth.ProviderFirebase
			}
		}
	}
	if err := mergo.Merge(&mdToUpdate, md, mergo.WithOverride, mergo.WithTypeCheck); err != nil {
		return errors.Wrapf(err, "failed to merge %#v and %v:%v", md, auth.IceIDClaim, userID)
	}
	params := []any{id.Email, now.Time, userID, id.DeviceUniqueID, issuedTokenSeq, mdToUpdate}
	var otpSet, otpWhere string
	if otp != "" {
		otpSet = "otp = $3,"
		otpWhere = "AND otp = $7" //nolint:gosec // .
		params = append(params, otp)
	}
	sql := fmt.Sprintf(`
			with metadata_update as (
				INSERT INTO account_metadata(user_id, metadata)
				VALUES ($3, $6::jsonb) ON CONFLICT(user_id) DO UPDATE
					SET metadata = EXCLUDED.metadata
				WHERE account_metadata.metadata != EXCLUDED.metadata
			) 
			UPDATE email_link_sign_ins
				SET token_issued_at = $2,
					user_id = $3,
					%[1]v
					email_confirmed_at = %[2]v,
					phone_number_to_email_migration_user_id = null,
					issued_token_seq = COALESCE(issued_token_seq, 0) + 1,
					previously_issued_token_seq = COALESCE(issued_token_seq, 0) + 1
			WHERE email_link_sign_ins.email = $1
				  AND device_unique_id = $4
				  AND issued_token_seq = $5
				  %[3]v
			`, otpSet, emailConfirmedAt, otpWhere)
	rowsUpdated, err := storage.Exec(ctx, c.db, sql, params...)
	if err != nil {
		return errors.Wrapf(err, "failed to insert generated token data for:%#v", params...)
	}
	if rowsUpdated == 0 {
		return errors.Wrapf(ErrNoConfirmationRequired, "[finishAuthProcess] No records were updated to finish: race condition")
	}

	return nil
}

//nolint:revive // .
func (c *client) resetLoginSession(
	ctx context.Context, id *loginID, els *emailLinkSignIn,
	prevConfirmationCode, clientIP string, loginSessionNumber int64,
) error {
	decrementIPAttempts := ""
	params := []any{els.UserID, id.Email, id.DeviceUniqueID, prevConfirmationCode, els.IssuedTokenSeq + 1}
	if clientIP != "" && loginSessionNumber > 0 {
		decrementIPAttempts = `with decrement_ip_login_attempts as (
				UPDATE sign_ins_per_ip SET
					login_attempts = GREATEST(sign_ins_per_ip.login_attempts - 1, 0)
				WHERE ip = $6 AND login_session_number = $7
			)`
		params = append(params, clientIP, loginSessionNumber)
	}
	sql := fmt.Sprintf(`%v UPDATE email_link_sign_ins
								SET confirmation_code = $1
							WHERE email = $2
								AND device_unique_id = $3
								AND confirmation_code = $4
								AND issued_token_seq = $5`, decrementIPAttempts)

	_, err := storage.Exec(ctx, c.db, sql, params...)

	return errors.Wrapf(err, "failed to reset login session by id:%#v and confirmationCode:%v", id, prevConfirmationCode)
}
