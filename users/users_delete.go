// SPDX-License-Identifier: ice License 1.0

package users

import (
	"context"
	"fmt"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/wintr/connectors/storage/v2"
)

func (r *repository) DeleteUser(ctx context.Context, userID UserID) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "context failed")
	}
	gUser, err := r.getUserByID(ctx, userID)
	if err != nil {
		return errors.Wrapf(err, "failed to get user for userID:%v", userID)
	}
	if err = r.deleteUser(ctx, gUser.User); err != nil {
		return errors.Wrapf(err, "failed to deleteUser for:%#v", gUser)
	}
	u := &UserSnapshot{Before: r.sanitizeUser(gUser.User)}
	if err = r.sendUserSnapshotMessage(ctx, u); err != nil {
		return errors.Wrapf(err, "failed to send deleted user message for %#v", u)
	}
	if err = r.sendTombstonedUserMessage(ctx, userID); err != nil {
		return errors.Wrapf(err, "failed to sendTombstonedUserMessage for userID:%v", userID)
	}

	return nil
}

func (r *repository) deleteUser(ctx context.Context, usr *User) error { //nolint:revive // .
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "delete user failed because context failed")
	}
	if err := r.deleteUserReferences(ctx, usr.ID); err != nil {
		return errors.Wrapf(err, "failed to deleteUserReferences for userID:%v", usr.ID)
	}
	if err := r.updateReferredByForAllT1Referrals(ctx, usr.ID); err != nil {
		for err != nil && (storage.IsErr(err, storage.ErrRelationNotFound) || storage.IsErr(err, storage.ErrNotFound)) {
			err = r.updateReferredByForAllT1Referrals(ctx, usr.ID)
		}
		if err != nil {
			return errors.Wrapf(err, "failed to update referredBy for all t1 referrals of userID:%v", usr.ID)
		}
	}
	gUser, err := r.getUserByID(ctx, usr.ID)
	if err != nil {
		return errors.Wrapf(err, "failed to get user for userID:%v", usr.ID)
	}
	*usr = *gUser.User
	sql := `DELETE FROM users WHERE id = $1`
	if _, tErr := storage.Exec(ctx, r.db, sql, usr.ID); tErr != nil {
		if storage.IsErr(tErr, storage.ErrRelationNotFound) {
			return r.deleteUser(ctx, usr)
		}

		return errors.Wrapf(tErr, "failed to delete user with id %v", usr.ID)
	}

	return nil
}

func (r *repository) deleteUserReferences(ctx context.Context, userID UserID) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "delete user failed because context failed")
	}
	wg := new(sync.WaitGroup)
	wg.Add(1)
	errChan := make(chan error, 2) //nolint:gomnd // .
	go func() {
		defer wg.Done()
		errChan <- errors.Wrapf(r.DeleteAllDeviceMetadata(ctx, userID), "failed to DeleteAllDeviceMetadata for userID:%v", userID)
		errChan <- errors.Wrapf(r.deleteReferralAcquisitionHistory(ctx, userID), "failed to deleteReferralAcquisitionHistory for userID:%v", userID)
	}()
	wg.Wait()
	close(errChan)
	errs := make([]error, 0, len(errChan))
	for err := range errChan {
		errs = append(errs, err)
	}

	return multierror.Append(nil, errs...).ErrorOrNil() //nolint:wrapcheck // Not needed.
}

//nolint:funlen // It's better to isolate everything together to decrease complexity; and it has some SQL, so...
func (r *repository) updateReferredByForAllT1Referrals(ctx context.Context, userID UserID) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "context failed")
	}
	sql := fmt.Sprintf(`
	WITH randomized AS (
		SELECT id, referred_by, created_at, hash_code FROM users
		WHERE id != '%[1]v'
			  AND id != 'bogus'
			  AND id != 'icenetwork'
			  AND username != id 
			  AND referred_by != id 
			  AND id != $1
			  AND referred_by != $1
			ORDER BY random()
	)
	SELECT (SELECT X.ID 
					FROM (	SELECT X.ID
								FROM (
									SELECT r.id
									FROM randomized r											
									WHERE r.id != u.id
										AND r.referred_by != u.id
										AND r.created_at < u.created_at
										AND MOD(r.hash_code,(floor(random()*COALESCE(
															(SELECT 
																CASE WHEN (SELECT value FROM global WHERE key='TOTAL_USERS')>1000 THEN 1000
																ELSE 5 
															END),5))+1)::bigint)=0
									LIMIT 1
							) X
		
							UNION ALL 

							SELECT u.id AS ID
							) X
					LIMIT 1
				) new_referred_by,
				u.ID as id
		FROM users u
		WHERE u.referred_by = $1
			AND u.id != $1`, r.cfg.DefaultReferralName)
	type resp struct {
		NewReferredBy UserID
		ID            UserID `db:"id"`
	}
	res, err := storage.Select[resp](ctx, r.db, sql, userID)
	if err != nil {
		return errors.Wrapf(err, "failed to select for all t1 referrals of userID:%v + their new random referralID", userID)
	}

	wg := new(sync.WaitGroup)
	wg.Add(len(res))
	errChan := make(chan error, len(res))
	for ii := range res {
		go func(ix int) {
			defer wg.Done()
			valTrue := true
			updatedReferral := new(User)
			updatedReferral.ID = res[ix].ID
			updatedReferral.ReferredBy = res[ix].NewReferredBy
			updatedReferral.RandomReferredBy = &valTrue
			_, mErr := r.ModifyUser(ctx, updatedReferral, nil)
			errChan <- errors.Wrapf(mErr,
				"failed to update referred by for userID:%v", res[ix].ID)
		}(ii)
	}
	wg.Wait()
	close(errChan)
	errs := make([]error, 0, len(res))
	for err := range errChan {
		errs = append(errs, err)
	}

	return errors.Wrap(multierror.Append(nil, errs...).ErrorOrNil(), "failed to update referred by for some/all of user's t1 referrals")
}
