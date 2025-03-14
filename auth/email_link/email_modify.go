// SPDX-License-Identifier: ice License 1.0

package emaillinkiceauth

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	"github.com/ice-blockchain/wintr/auth"
	"github.com/ice-blockchain/wintr/email"
	"github.com/ice-blockchain/wintr/server"
	"github.com/ice-blockchain/wintr/time"
)

//nolint:funlen,gocognit,nestif,revive // Big rollback logic.
func (c *client) handleEmailModification(ctx context.Context, els *emailLinkSignIn, newEmail, oldEmail, notifyEmail string) error {
	usr := new(users.User)
	usr.ID = *els.UserID
	usr.Email = newEmail
	_, err := c.userModifier.ModifyUser(users.ConfirmedEmailContext(ctx, newEmail), usr, nil)
	if err != nil {
		return errors.Wrapf(err, "failed to modify user %v with email modification", els.UserID)
	}
	if els.Metadata != nil {
		if firebaseID, hasFirebaseID := (*els.Metadata)[auth.FirebaseIDClaim]; hasFirebaseID {
			if fErr := server.Auth(ctx).UpdateEmail(ctx, firebaseID.(string), newEmail); fErr != nil { //nolint:forcetypeassert,errcheck // .
				oldEmailVal := oldEmail
				if els.PhoneNumberToEmailMigrationUserID != nil && *els.PhoneNumberToEmailMigrationUserID != "" {
					oldEmailVal = usr.ID
				}

				return multierror.Append( //nolint:wrapcheck // .
					errors.Wrapf(c.resetEmailModification(ctx, usr.ID, oldEmailVal), "[reset] resetEmailModification failed for email:%v", oldEmailVal),
					errors.Wrapf(fErr, "failed to change email in firebase to:%v fbUserID:%v", newEmail, firebaseID),
				).ErrorOrNil()
			}
		}
	}
	if notifyEmail != "" {
		now := time.Now()
		resetConfirmationCode := c.generateConfirmationCode("")
		uErr := c.upsertEmailLinkSignIn(ctx, oldEmail, els.DeviceUniqueID, resetConfirmationCode, els.Language, now)
		if uErr != nil {
			return multierror.Append( //nolint:wrapcheck // .
				errors.Wrapf(c.resetEmailModification(ctx, usr.ID, oldEmail), "[reset] resetEmailModification failed for email:%v", oldEmail),
				errors.Wrapf(c.resetFirebaseEmailModification(ctx, els.Metadata, oldEmail), "[reset] updateEmail in firebase failed for email:%v", oldEmail),
				errors.Wrapf(uErr, "failed to store/update email confirmation for email:%v", oldEmail),
			).ErrorOrNil()
		}
		resetEmailPayload, rErr := c.generateMagicLinkPayload(
			&loginID{Email: oldEmail, DeviceUniqueID: els.DeviceUniqueID},
			newEmail, now)
		if rErr != nil {
			return multierror.Append( //nolint:wrapcheck // .
				errors.Wrapf(c.resetEmailModification(ctx, usr.ID, oldEmail), "[reset] resetEmailModification failed for email:%v", oldEmail),
				errors.Wrapf(c.resetFirebaseEmailModification(ctx, els.Metadata, oldEmail), "[reset] updateEmail in firebase failed for email:%v", oldEmail),
				errors.Wrapf(rErr, "can't generate link payload for email: %v", oldEmail),
			).ErrorOrNil()
		}
		authLink := c.getResetAuthLink(resetEmailPayload, els.Language, resetConfirmationCode)
		if sErr := c.sendNotifyEmailChanged(ctx, notifyEmail, newEmail, authLink, els.Language); sErr != nil {
			return multierror.Append( //nolint:wrapcheck // .
				errors.Wrapf(c.resetEmailModification(ctx, usr.ID, oldEmail), "[reset] resetEmailModification failed for email:%v", oldEmail),
				errors.Wrapf(c.resetFirebaseEmailModification(ctx, els.Metadata, oldEmail), "[reset] updateEmail in firebase failed for email:%v", oldEmail),
				errors.Wrapf(sErr, "failed to send notification email about email change for userID %v email %v", els.UserID, oldEmail),
			).ErrorOrNil()
		}
	}

	return nil
}

func (c *client) getResetAuthLink(token, language, confirmationCode string) string {
	return fmt.Sprintf("%s?token=%s&lang=%s&confirmationCode=%s", c.cfg.EmailValidation.AuthLink, token, language, confirmationCode)
}

func (c *client) resetEmailModification(ctx context.Context, userID users.UserID, oldEmail string) error {
	if oldEmail == "" {
		return nil
	}
	usr := new(users.User)
	usr.ID = userID
	usr.Email = oldEmail
	_, mErr := c.userModifier.ModifyUser(users.ConfirmedEmailContext(ctx, oldEmail), usr, nil)

	return errors.Wrapf(mErr, "[rollback] failed to modify user:%v", userID)
}

func (*client) resetFirebaseEmailModification(ctx context.Context, md *users.JSON, oldEmail string) error {
	if md != nil {
		if firebaseID, hasFirebaseID := (*md)[auth.FirebaseIDClaim]; hasFirebaseID {
			return errors.Wrapf(
				server.Auth(ctx).UpdateEmail(ctx, firebaseID.(string), oldEmail), //nolint:forcetypeassert,errcheck // .
				"failed to change email in firebase to:%v fbUserID:%v", oldEmail, firebaseID,
			)
		}
	}

	return nil
}

//nolint:funlen // .
func (c *client) sendNotifyEmailChanged(ctx context.Context, notifyEmail, newEmail, link, language string) error {
	var tmpl *emailTemplate
	tmpl, ok := allEmailLinkTemplates[notifyEmailChangedType][language]
	if !ok {
		tmpl = allEmailLinkTemplates[notifyEmailChangedType][defaultLanguage]
	}
	dataBody := struct {
		NewEmail string
		Link     string
		PetName  string
		AppName  string
		TeamName string
	}{
		NewEmail: newEmail,
		Link:     link,
		PetName:  c.cfg.PetName,
		AppName:  c.cfg.AppName,
		TeamName: c.cfg.TeamName,
	}
	dataSubject := struct {
		AppName string
	}{
		AppName: c.cfg.AppName,
	}
	lbIdx := atomic.AddUint64(&c.emailClientLBIndex, 1) % uint64(c.cfg.ExtraLoadBalancersCount+1) //nolint:gosec // .

	return errors.Wrapf(c.emailClients[lbIdx].Send(ctx, &email.Parcel{
		Body: &email.Body{
			Type: email.TextHTML,
			Data: tmpl.getBody(dataBody),
		},
		Subject: tmpl.getSubject(dataSubject),
		From: email.Participant{
			Name:  c.fromRecipients[lbIdx].FromEmailName,
			Email: c.fromRecipients[lbIdx].FromEmailAddress,
		},
	}, email.Participant{
		Name:  "",
		Email: notifyEmail,
	}), "failed to send notify email changed for user with email:%v", notifyEmail)
}
