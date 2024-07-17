// SPDX-License-Identifier: ice License 1.0

package auth

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/wintr/connectors/storage/v2"
)

func FindOrGenerateUserID(ctx context.Context, db *storage.DB, inProgressTable, searchField, searchValue string) (userID string, err error) {
	if ctx.Err() != nil {
		return "", errors.Wrap(ctx.Err(), "find or generate user by id or telegram id context failed")
	}
	randomID := IceIDPrefix + uuid.NewString()

	return GetUserIDFromSearch(ctx, db, inProgressTable, searchField, searchValue, randomID)
}

//nolint:revive // We parameterize search.
func GetUserIDFromSearch(ctx context.Context, db *storage.DB, inProgressTable, searchField, searchValue, idIfNotFound string) (userID string, err error) {
	type dbUserID struct {
		ID string
	}
	sql := fmt.Sprintf(`SELECT id FROM (
				SELECT users.id, 1 as idx
					FROM users 
						WHERE %[1]v = $1
				UNION ALL
				(SELECT COALESCE(user_id, $2) AS id, 2 as idx
					FROM %[2]v
						WHERE %[1]v = $1)
			) t ORDER BY idx LIMIT 1`, searchField, inProgressTable)
	ids, err := storage.Select[dbUserID](ctx, db, sql, searchValue, idIfNotFound)
	if err != nil || len(ids) == 0 {
		if storage.IsErr(err, storage.ErrNotFound) || (err == nil && len(ids) == 0) {
			return idIfNotFound, nil
		}

		return "", errors.Wrapf(err, "failed to find user by:%v", searchValue)
	}

	return ids[0].ID, nil
}
