// SPDX-License-Identifier: ice License 1.0

package face

import (
	"context"
	stdlibtime "time"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/kyc/face/internal"
	"github.com/ice-blockchain/eskimo/kyc/face/internal/threedivi"
	"github.com/ice-blockchain/eskimo/users"
	appcfg "github.com/ice-blockchain/wintr/config"
	"github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/time"
)

func New(ctx context.Context, usersRep UserRepository, linker Linker) Client {
	var cfg Config
	appcfg.MustLoadFromKey(applicationYamlKey, &cfg)
	if cfg.UnexpectedErrorsAllowed == 0 {
		cfg.UnexpectedErrorsAllowed = 5
	}
	db := storage.MustConnect(ctx, ddl, applicationYamlKey)
	cl := &client{
		client:         threedivi.New3Divi(ctx, usersRep, &cfg.ThreeDiVi),
		cfg:            cfg,
		db:             db,
		users:          usersRep,
		accountsLinker: linker,
	}
	go cl.clearErrs(ctx)
	go cl.startKYCConfigJSONSyncer(ctx)

	return cl
}

//nolint:funlen,gocognit,revive // .
func (c *client) CheckStatus(ctx context.Context, user *users.User, nextKYCStep users.KYCStep) (bool, error) {
	kycFaceAvailable := false
	if errs := c.unexpectedErrors.Load(); errs >= c.cfg.UnexpectedErrorsAllowed {
		log.Error(errors.Errorf("some unexpected error occurred recently for user id %v", user.ID))

		return false, nil
	}
	userWasPreviouslyForwardedToFaceKYC, err := c.checkIfUserWasForwardedToFaceKYC(ctx, user.ID)
	if err != nil {
		return false, errors.Wrapf(err, "failed to check if user id %v was forwarded to face kyc before", user.ID)
	}
	hasResult := false
	now := time.Now()
	var faceID, verifiedTenantFrom3rdParty, verifiedTenant string
	if userWasPreviouslyForwardedToFaceKYC {
		if hasResult, faceID, verifiedTenantFrom3rdParty, err = c.client.CheckAndUpdateStatus(ctx, user.ID, user); err != nil {
			c.unexpectedErrors.Add(1)
			log.Error(errors.Wrapf(err, "[unexpected]failed to update face auth status for user ID %s", user.ID))

			return false, nil
		}
		if verifiedTenantFrom3rdParty != "" {
			verifiedTenant = verifiedTenantFrom3rdParty
		}
	}
	if hasResult {
		if dErr := c.accountsLinker.SetTenantVerified(ctx, user.ID, verifiedTenant); dErr != nil {
			return false, errors.Wrapf(dErr, "failed to mark tenant %v verified for user %v", verifiedTenant, user.ID)
		}
		if dErr := c.saveFaceID(ctx, user.ID, faceID); dErr != nil {
			return false, errors.Wrapf(dErr, "failed to save user face id kyc for user id %v", user.ID)
		}
	}
	if !hasResult || nextKYCStep == users.LivenessDetectionKYCStep {
		availabilityErr := c.client.Available(ctx, userWasPreviouslyForwardedToFaceKYC)
		if availabilityErr == nil {
			kycFaceAvailable = true
			if fErr := c.saveUserForwarded(ctx, user.ID, now); fErr != nil {
				return false, errors.Wrapf(fErr, "failed ")
			}
		} else if !errors.Is(availabilityErr, internal.ErrNotAvailable) {
			c.unexpectedErrors.Add(1)
			log.Error(errors.Wrapf(availabilityErr, "[unexpected]face auth is unavailable for userID %v KYCStep %v", user.ID, nextKYCStep))
		}
	}

	return kycFaceAvailable, nil
}

func (c *client) saveFaceID(ctx context.Context, userID, faceID string) error {
	_, err := storage.Exec(ctx, c.db, `INSERT INTO users_forwarded_to_face_kyc (forwarded_at, user_id, face_id) 
			VALUES(NOW(),$1,$2)
			ON CONFLICT (user_id) DO UPDATE 
		SET face_id = $2 WHERE users_forwarded_to_face_kyc.user_id = $1 and users_forwarded_to_face_kyc.face_id != $2;
`, userID, faceID)

	return errors.Wrapf(err, "failed to delete user forwarded to face kyc for userID %v", userID)
}

func (c *client) saveUserForwarded(ctx context.Context, userID string, now *time.Time) error {
	_, err := storage.Exec(ctx, c.db, `INSERT INTO users_forwarded_to_face_kyc(forwarded_at, user_id, face_id) 
														values ($1, $2, $2) ON CONFLICT DO NOTHING;`, now.Time, userID)

	return errors.Wrapf(err, "failed to save user forwarded to face kyc for userID %v", userID)
}

func (c *client) checkIfUserWasForwardedToFaceKYC(ctx context.Context, userID string) (bool, error) {
	_, err := storage.Get[struct {
		ForwardedAt *time.Time `db:"forwarded_at"`
		UserID      string     `db:"user_id"`
		FaceID      string     `db:"face_id"`
	}](ctx, c.db, "SELECT * FROM users_forwarded_to_face_kyc WHERE user_id = $1;", userID)
	if err != nil {
		if storage.IsErr(err, storage.ErrNotFound) {
			return false, nil
		}

		return false, errors.Wrapf(err, "failed to check if user was forwarded to face kyc")
	}

	return true, nil
}

func (c *client) clearErrs(ctx context.Context) {
	ticker := stdlibtime.NewTicker(refreshTime)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.unexpectedErrors.Store(0)
		case <-ctx.Done():
			return
		}
	}
}

func (c *client) Reset(ctx context.Context, user *users.User, fetchState bool) error {
	return errors.Wrapf(c.client.Reset(ctx, user, fetchState), "failed to reset face auth state for userID %s", user.ID)
}

func (c *client) Close() error {
	return c.db.Close() //nolint:wrapcheck // .
}

//nolint:funlen // .
func (c *client) ForwardToKYC(ctx context.Context, userID string) (kycFaceAvailable bool, err error) {
	if !c.isKYCEnabled(ctx) {
		return false, nil
	}
	now := time.Now()
	usr, err := c.users.GetUserByID(ctx, userID)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get user %v", userID)
	}
	if usr.HasFaceKYCResult() {
		return false, nil
	}
	var passed bool
	var faceID string
	passed, faceID, verifiedTenant, err := c.client.CheckAndUpdateStatus(ctx, usr.ID, usr.User)
	if err != nil {
		return false, errors.Wrapf(err, "failed to sync face kyc status back for user %v", usr.ID)
	}
	if passed {
		if err = c.accountsLinker.SetTenantVerified(ctx, usr.ID, verifiedTenant); err != nil {
			return false, errors.Wrapf(err, "failed to save user face id kyc for user id %v", usr.ID)
		}
		if err = c.saveFaceID(ctx, usr.ID, faceID); err != nil {
			return false, errors.Wrapf(err, "failed to save user face id kyc for user id %v", usr.ID)
		}

		return false, nil
	}
	userWasPreviouslyForwardedToFaceKYC, err := c.checkIfUserWasForwardedToFaceKYC(ctx, usr.ID)
	if err != nil {
		return false, errors.Wrapf(err, "failed to check if user %v was forwarded to face kyc before", usr.ID)
	}
	if kycFaceAvailable, err = c.isAvailable(ctx, now, userWasPreviouslyForwardedToFaceKYC, usr.ID); err != nil {
		return false, errors.Wrapf(err, "failed to flag user %v was forwarded to face kyc", usr.ID)
	}

	return kycFaceAvailable, nil
}

//nolint:revive // .
func (c *client) isAvailable(ctx context.Context, now *time.Time, userWasPreviouslyForwardedToFaceKYC bool, userID string) (kycFaceAvailable bool, err error) {
	availabilityErr := c.client.Available(ctx, userWasPreviouslyForwardedToFaceKYC)
	if userWasPreviouslyForwardedToFaceKYC || availabilityErr == nil {
		kycFaceAvailable = true
		if err = c.saveUserForwarded(ctx, userID, now); err != nil {
			return false, errors.Wrapf(err, "failed to flag user as forwarded to face kyc")
		}
	}

	return kycFaceAvailable, nil
}
