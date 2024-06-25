// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"context"
	"github.com/pkg/errors"
	"mime/multipart"

	"github.com/ice-blockchain/eskimo/users"
)

type (
	Client interface {
		Available(ctx context.Context) error
		CheckAndUpdateStatus(ctx context.Context, user *users.User) (hasFaceKYCResult bool, err error)
		Reset(ctx context.Context, user *users.User, fetchState bool) error
	}
	UserRepository interface {
		ModifyUser(ctx context.Context, usr *users.User, profilePicture *multipart.FileHeader) (*users.UserProfile, error)
	}
)

var (
	ErrNotAvailable = errors.Errorf("not available")
)
