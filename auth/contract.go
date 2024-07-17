// SPDX-License-Identifier: ice License 1.0

package auth

import (
	"context"
	"errors"

	wintrauth "github.com/ice-blockchain/wintr/auth"
)

type (
	Tokens struct {
		RefreshToken string `json:"refreshToken,omitempty" example:"eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJPbmxpbmUgSldUIEJ1aWxkZXIiLCJpYXQiOjE2ODQzMjQ0NTYsImV4cCI6MTcxNTg2MDQ1NiwiYXVkIjoiIiwic3ViIjoianJvY2tldEBleGFtcGxlLmNvbSIsIm90cCI6IjUxMzRhMzdkLWIyMWEtNGVhNi1hNzk2LTAxOGIwMjMwMmFhMCJ9.q3xa8Gwg2FVCRHLZqkSedH3aK8XBqykaIy85rRU40nM"` //nolint:lll // .
		AccessToken  string `json:"accessToken,omitempty"  example:"eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJPbmxpbmUgSldUIEJ1aWxkZXIiLCJpYXQiOjE2ODQzMjQ0NTYsImV4cCI6MTcxNTg2MDQ1NiwiYXVkIjoiIiwic3ViIjoianJvY2tldEBleGFtcGxlLmNvbSIsIm90cCI6IjUxMzRhMzdkLWIyMWEtNGVhNi1hNzk2LTAxOGIwMjMwMmFhMCJ9.q3xa8Gwg2FVCRHLZqkSedH3aK8XBqykaIy85rRU40nM"` //nolint:lll // .
	}
	TokenRefresher interface {
		RegenerateTokens(ctx context.Context, prevToken string) (tokens *Tokens, err error)
	}
	ProviderRefresher interface {
		RefreshToken(ctx context.Context, token *wintrauth.IceToken) (*Tokens, error)
	}
)

const (
	IceIDPrefix = "ice_"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("expired token")
)

// Private API.
type (
	tokenRefresher struct {
		authClient wintrauth.Client
		platforms  map[string]ProviderRefresher
	}
)

const (
	platformEmail    = "email"
	platformTelegram = "telegram"
)
