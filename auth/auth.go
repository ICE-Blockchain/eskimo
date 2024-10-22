// SPDX-License-Identifier: ice License 1.0

package auth

import (
	"context"

	"github.com/pkg/errors"

	wintrauth "github.com/ice-blockchain/wintr/auth"
)

func NewRefresher(authClient wintrauth.Client, email, telegram ProviderRefresher) TokenRefresher {
	return &tokenRefresher{
		authClient: authClient,
		platforms: map[string]ProviderRefresher{
			platformEmail:    email,
			platformTelegram: telegram,
		},
	}
}

func (c *tokenRefresher) RegenerateTokens(ctx context.Context, previousRefreshToken string) (tokens *Tokens, err error) {
	token, err := c.authClient.ParseToken(previousRefreshToken, true)
	if err != nil {
		if errors.Is(err, wintrauth.ErrExpiredToken) {
			return nil, errors.Wrapf(ErrExpiredToken, "failed to verify due to expired token:%v", previousRefreshToken)
		}
		if errors.Is(err, wintrauth.ErrInvalidToken) {
			return nil, errors.Wrapf(ErrInvalidToken, "failed to verify due to invalid token:%v", previousRefreshToken)
		}

		return nil, errors.Wrapf(ErrInvalidToken, "failed to verify token:%v (token:%v)", err.Error(), previousRefreshToken)
	}
	telegramUserID := ""
	if len(token.Claims) > 0 {
		if tUserIDInterface, found := token.Claims["telegramUserID"]; found {
			telegramUserID = tUserIDInterface.(string) //nolint:errcheck,forcetypeassert // .
		}
	}
	var provider ProviderRefresher
	switch {
	case telegramUserID != "":
		provider = c.platforms[platformTelegram]
	case token.Email != "":
		provider = c.platforms[platformEmail]
	default:
		return nil, errors.Wrapf(ErrInvalidToken, "invalid token %v cannot detect both email and telegram", previousRefreshToken)
	}
	tokens, err = provider.RefreshToken(ctx, token)

	return tokens, errors.Wrapf(err, "failed to refresh tokens for %#v", token)
}
