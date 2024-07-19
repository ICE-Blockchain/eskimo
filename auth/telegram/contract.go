// SPDX-License-Identifier: ice License 1.0

package telegram

import (
	"context"
	_ "embed"
	"time"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/auth"
	"github.com/ice-blockchain/eskimo/users"
	wintrauth "github.com/ice-blockchain/wintr/auth"
	"github.com/ice-blockchain/wintr/connectors/storage/v2"
)

type (
	Client interface {
		SignIn(ctx context.Context, tmaToken string, telegramBotID *telegramBotID) (tokens *auth.Tokens, err error)
		RefreshToken(ctx context.Context, token *wintrauth.IceToken) (tokens *auth.Tokens, err error)
	}
)

var (
	ErrInvalidToken = auth.ErrInvalidToken
	ErrExpiredToken = auth.ErrExpiredToken
	ErrUserNotFound = storage.ErrNotFound
	ErrInvalidBotID = errors.Errorf("invalid bot ID")
)

// Private API.
type (
	client struct {
		db         *storage.DB
		authClient wintrauth.Client
		shutdown   func() error
		cfg        *config
	}
	config struct {
		TelegramBots map[telegramBotID]struct {
			BotToken telegramBotToken `yaml:"botToken"`
		} `yaml:"telegramBots" mapstructure:"telegramBots"`
		TelegramTokenExpiration time.Duration `yaml:"telegramTokenExpiration"`
	}
	telegramBotToken = string
	telegramBotID    = string
	telegramSignIn   struct {
		CreatedAt                *time.Time
		TokenIssuedAt            *time.Time
		Metadata                 *users.JSON `json:"metadata,omitempty"`
		UserID                   *string     `json:"userId" example:"did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"`
		TelegramUserID           string      `json:"telegramUserId,omitempty" example:"12345678990" db:"telegram_user_id"`
		TelegramBotID            string      `json:"telegramBotId,omitempty" example:"1" db:"telegram_bot_id"`
		Email                    string      `json:"email,omitempty" example:"someone1@example.com"`
		IssuedTokenSeq           int64       `json:"issuedTokenSeq,omitempty" example:"1"`
		PreviouslyIssuedTokenSeq int64       `json:"previouslyIssuedTokenSeq,omitempty" example:"1"`
		HashCode                 int64       `json:"hashCode,omitempty" example:"43453546464576547"`
	}
)

const (
	applicationYamlKey = "auth/telegram"
)

var ( //nolint:gofumpt // .
	//go:embed DDL.sql
	ddl string
)
