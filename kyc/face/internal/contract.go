// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"context"
	"mime/multipart"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
)

type (
	Client interface {
		Available(ctx context.Context, userWasPreviouslyForwardedToFaceKYC bool) error
		CheckAndUpdateStatus(ctx context.Context, searchUserID string, user *users.User) (hasFaceKYCResult bool, faceID string, err error)
		Reset(ctx context.Context, user *users.User, fetchState bool) error
	}
	UserRepository interface {
		ModifyUser(ctx context.Context, usr *users.User, profilePicture *multipart.FileHeader) (*users.UserProfile, error)
		GetUserByID(ctx context.Context, userID string) (*users.UserProfile, error)
	}
)

//nolint:grouper // .
var ErrNotAvailable = errors.Errorf("not available")
