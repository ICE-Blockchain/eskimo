// SPDX-License-Identifier: ice License 1.0

package users

import (
	"context"
	"strings"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/wintr/connectors/storage/v2"
)

func (r *repository) ClaimUserBy3rdParty(ctx context.Context, username, thirdParty string) error {
	rowsAffected, err := storage.Exec(ctx, r.db, `
	UPDATE users
		set claimed_by_third_party_at = now(),
			claimed_by_third_party = $2
		WHERE username = $1
		  AND claimed_by_third_party is null
          AND claimed_by_third_party_at is null
		  AND created_at > now() - interval '24 hour'
		  AND last_mining_started_at is not null`, strings.ToLower(username), thirdParty)
	if err != nil {
		return errors.Wrapf(err, "failed to execute sql to ClaimUserBy3rdParty (%v,%v)", username, thirdParty)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}
