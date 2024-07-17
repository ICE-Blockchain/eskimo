// SPDX-License-Identifier: ice License 1.0

package telegram

import (
	"context"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/auth"
	wintrauth "github.com/ice-blockchain/wintr/auth"
	"github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/time"
)

func (c *client) RefreshToken(ctx context.Context, token *wintrauth.IceToken) (tokens *auth.Tokens, err error) {
	telegramUserID := ""
	if token.Claims != nil {
		if tUserIDInterface, found := token.Claims["telegramUserID"]; found {
			telegramUserID = tUserIDInterface.(string) //nolint:errcheck,forcetypeassert // .
		}
	}
	if telegramUserID == "" {
		return nil, errors.New("unexpected empty telegramID")
	}
	usr, err := c.getUserByIDOrTelegram(ctx, token.Subject, telegramUserID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, errors.Wrapf(ErrUserNotFound, "user with userID:%v or tegegramID:%v not found", token.Subject, telegramUserID)
		}

		return nil, errors.Wrapf(err, "failed to get user by userID:%v", token.Subject)
	}
	now := time.Now()
	refreshTokenSeq, err := auth.IncrementRefreshTokenSeq(ctx, c.db, "telegram_sign_ins",
		"telegram_sign_ins.telegram_user_id = $4",
		[]any{telegramUserID}, token.Subject, token.Seq, now)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to update telegram sign ins for:%v,%v", telegramUserID, token.Subject)
	}
	tokens, err = c.generateTokens(now, usr, refreshTokenSeq)

	return tokens, errors.Wrapf(err, "can't generate tokens for userID:%v, telegram:%v", token.Subject, telegramUserID)
}

func (c *client) generateTokens(now *time.Time, signIn *telegramSignIn, seq int64) (tokens *auth.Tokens, err error) {
	role := ""
	if signIn.Metadata != nil {
		if roleInterface, found := (*signIn.Metadata)["role"]; found {
			role = roleInterface.(string) //nolint:errcheck,forcetypeassert // .
		}
	}
	refreshToken, accessToken, err := c.authClient.GenerateTokens(now, *signIn.UserID, "", signIn.Email, signIn.HashCode, seq, role, map[string]any{
		"telegramUserID": signIn.TelegramUserID,
		"loginType":      "telegram",
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate tokens for user:%#v", signIn)
	}

	return &auth.Tokens{AccessToken: accessToken, RefreshToken: refreshToken}, nil
}
