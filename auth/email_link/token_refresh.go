// SPDX-License-Identifier: ice License 1.0

package emaillinkiceauth

import (
	"context"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/auth"
	wintrauth "github.com/ice-blockchain/wintr/auth"
	"github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/time"
)

func (c *client) RefreshToken(ctx context.Context, token *wintrauth.IceToken) (tokens *auth.Tokens, err error) {
	id := loginID{Email: token.Email, DeviceUniqueID: token.DeviceUniqueID}
	usr, err := c.getUserByIDOrPk(ctx, token.Subject, &id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, errors.Wrapf(ErrUserNotFound, "user with userID:%v or email:%v not found", token.Subject, token.Email)
		}

		return nil, errors.Wrapf(err, "failed to get user by userID:%v", token.Subject)
	}
	if usr.Email != token.Email || usr.DeviceUniqueID != token.DeviceUniqueID {
		return nil, errors.Wrapf(ErrUserDataMismatch,
			"user's email:%v does not match token's email:%v or deviceID:%v (userID %v)", usr.Email, token.Email, token.DeviceUniqueID, token.Subject)
	}
	now := time.Now()
	refreshTokenSeq, err := auth.IncrementRefreshTokenSeq(ctx, c.db, "email_link_sign_ins",
		"email_link_sign_ins.email = $4 AND email_link_sign_ins.device_unique_id = $5",
		[]any{id.Email, id.DeviceUniqueID}, token.Subject, token.Seq, now)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to update email link sign ins for email:%v", token.Email)
	}
	tokens, err = c.generateTokens(now, usr, refreshTokenSeq)

	return tokens, errors.Wrapf(err, "can't generate tokens for userID:%v, email:%v", token.Subject, token.Email)
}

func (c *client) generateTokens(now *time.Time, els *emailLinkSignIn, seq int64) (tokens *auth.Tokens, err error) {
	role := ""
	if els.Metadata != nil {
		if roleInterface, found := (*els.Metadata)["role"]; found {
			role = roleInterface.(string) //nolint:errcheck,forcetypeassert // .
		}
	}
	refreshToken, accessToken, err := c.authClient.GenerateTokens(now, *els.UserID, els.DeviceUniqueID, els.Email, els.HashCode, seq, role, map[string]any{
		"loginType": "email",
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate tokens for user:%#v", els)
	}

	return &auth.Tokens{AccessToken: accessToken, RefreshToken: refreshToken}, nil
}
