// SPDX-License-Identifier: ice License 1.0

package auth

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/time"
)

//nolint:funlen,revive // Large SQL.
func IncrementRefreshTokenSeq(
	ctx context.Context,
	db *storage.DB,
	table, searchCondition string,
	searchParams []any,
	userID string,
	currentSeq int64,
	now *time.Time,
) (tokenSeq int64, err error) {
	params := []any{now.Time, userID, currentSeq}
	params = append(params, searchParams...)
	type resp struct {
		IssuedTokenSeq int64
	}
	sql := fmt.Sprintf(`UPDATE %[1]v
			SET token_issued_at = $1,
				user_id = $2,
				issued_token_seq = COALESCE(%[1]v.issued_token_seq, 0) + 1,
				previously_issued_token_seq = (CASE WHEN COALESCE(%[1]v.issued_token_seq, 0) = $3 THEN %[1]v.issued_token_seq ELSE $3 END)
			WHERE  %[2]v
				   AND %[1]v.user_id = $2 
			  	   AND (%[1]v.issued_token_seq = $3 
			         OR (%[1]v.previously_issued_token_seq <= $3 AND
			             %[1]v.previously_issued_token_seq<=COALESCE(%[1]v.issued_token_seq,0)+1)
			       )
			RETURNING issued_token_seq`, table, searchCondition)
	updatedValue, err := storage.ExecOne[resp](ctx, db, sql, params...)
	if err != nil {
		if storage.IsErr(err, storage.ErrNotFound) {
			return 0, errors.Wrapf(ErrInvalidToken, "refreshToken with wrong sequence:%v provided (userID:%v)", currentSeq, userID)
		}

		return 0, errors.Wrapf(err, "failed to assign refreshed token to %v for params:%#v", table, params)
	}

	return updatedValue.IssuedTokenSeq, nil
}
