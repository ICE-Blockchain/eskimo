// SPDX-License-Identifier: ice License 1.0

package emaillinkiceauth

import (
	"context"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/wintr/connectors/storage/v2"
)

func (c *client) getUserByEmail(ctx context.Context, email, oldEmail string) (*minimalUser, error) {
	userID, err := c.findOrGenerateUserIDByEmail(ctx, email, oldEmail)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch or generate userID for email:%v", email)
	}
	usr, err := c.getUserByIDOrEmail(ctx, userID, email)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get user info by userID:%v", userID)
	}

	return usr, nil
}

func (c *client) findOrGenerateUserIDByEmail(ctx context.Context, email, oldEmail string) (userID string, err error) {
	if ctx.Err() != nil {
		return "", errors.Wrap(ctx.Err(), "find or generate user by id or email context failed")
	}
	randomID := uuid.NewString()
	searchEmail := email
	if oldEmail != "" {
		searchEmail = oldEmail
	}
	type dbUserID struct {
		ID string
	}
	ids, err := storage.Select[dbUserID](ctx, c.db, `SELECT id FROM users WHERE email = $1 OR id = $2`, searchEmail, randomID)
	if err != nil || len(ids) == 0 {
		if storage.IsErr(err, storage.ErrNotFound) || len(ids) == 0 {
			return randomID, nil
		}

		return "", errors.Wrapf(err, "failed to find user by userID:%v or email:%v", randomID, email)
	}
	if ids[0].ID == randomID || (len(ids) > 1) {
		return c.findOrGenerateUserIDByEmail(ctx, email, oldEmail)
	}

	return ids[0].ID, nil
}

func (c *client) getUserByIDOrEmail(ctx context.Context, id, email string) (*minimalUser, error) {
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "get user by id or email failed because context failed")
	}
	usr, err := storage.Get[minimalUser](ctx, c.db, `
		WITH emails AS (
			SELECT 
				$1 AS id,
				email,
				COALESCE((custom_claims -> 'hash_code')::BIGINT,0) AS hash_code,
				custom_claims
			FROM email_link_sign_ins
			WHERE email = $2
		)
		SELECT 
			u.id,
			u.email,
			u.hash_code,
			emails.custom_claims AS custom_claims
		FROM users u, emails
		WHERE u.id = $1
	`, id, email)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get user by id:%v or email:%v", id, email)
	}

	return usr, nil
}
