// SPDX-License-Identifier: ice License 1.0

package telegram

import (
	"context"
	"strconv"

	"github.com/pkg/errors"
	initdata "github.com/telegram-mini-apps/init-data-golang"

	"github.com/ice-blockchain/eskimo/auth"
	"github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/time"
)

func (c *client) SignIn(ctx context.Context, tmaToken string, telegramBotID *telegramBotID) (tokens *auth.Tokens, err error) {
	botID, vErr := c.verifyTelegramTMAToken(tmaToken, telegramBotID)
	if vErr != nil {
		return nil, errors.Wrapf(vErr, "failed to verify TMA token %v", tmaToken)
	}
	tgData, err := initdata.Parse(tmaToken)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse telegramToken %v", tmaToken)
	}
	now := time.Now()
	telegramUserID := strconv.FormatInt(tgData.User.ID, 10)
	userID, err := auth.FindOrGenerateUserID(ctx, c.db, "telegram_sign_ins", "telegram_user_id", telegramUserID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch or generate userID for telegram: %v", telegramUserID)
	}
	signIn, err := c.upsertTelegramSignIn(ctx, now, userID, telegramUserID, botID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to save user information from telegram %#v", tgData.User)
	}

	return c.generateTokens(now, signIn, signIn.IssuedTokenSeq)
}

//nolint:funlen // .
func (c *client) verifyTelegramTMAToken(tmaToken string, telegramBotID *telegramBotID) (string, error) {
	var vErr error
	botID := ""
	if telegramBotID == nil || *telegramBotID == "" {
		for bID, botToken := range c.cfg.TelegramBots {
			if vErr = initdata.Validate(tmaToken, botToken.BotToken, c.cfg.TelegramTokenExpiration); vErr == nil {
				botID = bID

				break
			}
		}
	} else {
		bot, found := c.cfg.TelegramBots[*telegramBotID]
		if !found {
			return "", ErrInvalidBotID
		}
		if vErr = initdata.Validate(tmaToken, bot.BotToken, c.cfg.TelegramTokenExpiration); vErr == nil {
			botID = *telegramBotID
		}
	}
	if botID == "" && vErr != nil {
		switch {
		case errors.Is(vErr, initdata.ErrSignInvalid):
			return "", ErrInvalidToken
		case errors.Is(vErr, initdata.ErrSignMissing):
			return "", ErrInvalidToken
		case errors.Is(vErr, initdata.ErrUnexpectedFormat):
			return "", ErrInvalidToken
		case errors.Is(vErr, initdata.ErrAuthDateMissing):
			return "", ErrInvalidToken
		case errors.Is(vErr, initdata.ErrExpired):
			return "", ErrExpiredToken
		default:
			return "", errors.Wrapf(vErr, "failed to validate telegram token %v for unknown reason", tmaToken)
		}
	}

	return botID, nil
}

func (c *client) upsertTelegramSignIn(ctx context.Context, now *time.Time, userID, telegramUserID, botID string) (*telegramSignIn, error) {
	params := []any{now.Time, telegramUserID, botID, userID}
	sql := `INSERT INTO telegram_sign_ins (
							created_at,token_issued_at, telegram_user_id, telegram_bot_id,user_id,issued_token_seq,previously_issued_token_seq)
						VALUES ($1, $1, $2, $3,$4, 1, 0)
						ON CONFLICT (telegram_user_id) DO UPDATE 
							SET created_at    				     	   = EXCLUDED.created_at,
							    token_issued_at                        = EXCLUDED.token_issued_at,
							    telegram_bot_id                        = EXCLUDED.telegram_bot_id,
								issued_token_seq = COALESCE(telegram_sign_ins.issued_token_seq, 0) + 1,
								previously_issued_token_seq = COALESCE(telegram_sign_ins.issued_token_seq, 0) + 1
						RETURNING *`
	res, err := storage.ExecOne[telegramSignIn](ctx, c.db, sql, params...)

	return res, errors.Wrapf(err, "failed to insert/update telegram sign ins record for telegramUserID:%v", telegramUserID)
}
