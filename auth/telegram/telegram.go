// SPDX-License-Identifier: ice License 1.0

package telegram

import (
	"context"

	"github.com/pkg/errors"

	wintrauth "github.com/ice-blockchain/wintr/auth"
	appcfg "github.com/ice-blockchain/wintr/config"
	"github.com/ice-blockchain/wintr/connectors/storage/v2"
)

func NewClient(ctx context.Context, authClient wintrauth.Client) Client {
	var cfg config
	appcfg.MustLoadFromKey(applicationYamlKey, &cfg)
	db := storage.MustConnect(ctx, ddl, applicationYamlKey)

	return &client{authClient: authClient, cfg: &cfg, db: db, shutdown: db.Close}
}

func (c *client) Close() error {
	return errors.Wrap(c.shutdown(), "closing auth/telegram repository failed")
}
