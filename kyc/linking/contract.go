// SPDX-License-Identifier: ice License 1.0

package linking

import (
	"context"
	_ "embed"
	stdlibtime "time"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/time"
)

type (
	Token          = string
	UserID         = string
	Tenant         = string
	LinkedProfiles = map[Tenant]UserID
	Linker         interface {
		Verify(ctx context.Context, now *time.Time, userID UserID, tokens map[Tenant]Token) (LinkedProfiles, error)
		Get(ctx context.Context, userID UserID) (LinkedProfiles, error)
	}
)

type (
	linker struct {
		globalDB *storage.DB
		cfg      *config
	}
	config struct {
		TenantURLs map[Tenant]string `yaml:"tenantUrls" mapstructure:"tenantUrls"`
	}
)

const (
	requestDeadline = 25 * stdlibtime.Second
	tenantCtxKey    = "tenantCtxKey"
)

var (
	//go:embed global_ddl.sql
	ddl                   string
	errRemoteUserNotFound = errors.New("remote user not found")
	ErrNotOwnRemoteUser   = errors.New("not own remote user")
)
