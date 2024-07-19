// SPDX-License-Identifier: ice License 1.0

package telegram

import (
	"context"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/wintr/connectors/storage/v2"
)

//nolint:funlen // SQL.
func (c *client) getUserByIDOrTelegram(ctx context.Context, userID, telegramID string) (*telegramSignIn, error) {
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "get user by id or telegram failed because context failed")
	}
	usr, err := storage.Get[telegramSignIn](ctx, c.db, `
		SELECT 		created_at,
					token_issued_at,
					COALESCE(previously_issued_token_seq, 0) 			AS previously_issued_token_seq, 
					COALESCE(issued_token_seq, 0) 			 			AS issued_token_seq,
					user_id,
					telegram_user_id,
					telegram_bot_id,
					email,
		    		hash_code,
		    		metadata
		FROM (
			WITH telegrams AS (
				SELECT
					created_at,
					token_issued_at,
		    		previously_issued_token_seq,
					issued_token_seq,
					$1 												   AS user_id,
					telegram_user_id,
					telegram_bot_id,
					''                                                 as email,
					COALESCE((account_metadata.metadata -> 'hash_code')::BIGINT,0) AS hash_code,
					account_metadata.metadata,
					2                                                  AS idx
				FROM telegram_sign_ins
				LEFT JOIN account_metadata ON account_metadata.user_id = $1
				WHERE telegram_user_id = $2
			)
			SELECT
					telegrams.created_at                                   AS created_at,
					telegrams.token_issued_at       			 	  	   AS token_issued_at,
		    		telegrams.previously_issued_token_seq                  AS previously_issued_token_seq,
					telegrams.issued_token_seq       			 	  	   AS issued_token_seq,
					u.id 									 	  	       AS user_id,
					NULLIF(u.telegram_user_id, u.id)                       AS telegram_user_id,
					u.telegram_bot_id,
					COALESCE(NULLIF(u.email, u.id)  ,'')                   AS email,
					u.hash_code,
					account_metadata.metadata    				 	   AS metadata,
					1 												   AS idx
				FROM users u 
				LEFT JOIN telegrams ON telegrams.telegram_user_id = $2 and u.id = telegrams.user_id
				LEFT JOIN account_metadata ON u.id = account_metadata.user_id
				WHERE u.id = $1
			UNION ALL (select * from telegrams)
		) t WHERE t.telegram_user_id IS NOT NULL ORDER BY idx LIMIT 1
	`, userID, telegramID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get user by telegram:(%v,%v)", userID, telegramID)
	}

	return usr, nil
}
