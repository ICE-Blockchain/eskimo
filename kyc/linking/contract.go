// SPDX-License-Identifier: ice License 1.0

package linking

import (
	"context"
	_ "embed"
	"io"
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
		io.Closer
		Verify(ctx context.Context, now *time.Time, userID UserID, tokens map[Tenant]Token) (allLinkedProfiles LinkedProfiles, verified Tenant, err error)
		Get(ctx context.Context, userID UserID) (allLinkedProfiles LinkedProfiles, verified Tenant, err error)
		SetTenantVerified(ctx context.Context, userID UserID, tenant Tenant) error
		StoreLinkedAccounts(ctx context.Context, now *time.Time, userID, verifiedTenant string, res map[Tenant]UserID) error
	}
)

type (
	linker struct {
		globalDB *storage.DB
		cfg      *config
		host     string
	}
	config struct {
		TenantURLs map[Tenant]string `yaml:"tenantURLs" mapstructure:"tenantURLs"` //nolint:tagliatelle // .
		Tenant     string            `yaml:"tenant" mapstructure:"tenant"`
	}
)

const (
	requestDeadline    = 25 * stdlibtime.Second
	applicationYamlKey = "kyc/linking"
)

var (
	//go:embed global_ddl.sql
	ddl                   string
	ErrRemoteUserNotFound = errors.New("remote user not found")
	ErrNotOwnRemoteUser   = errors.New("not own remote user")
	ErrDuplicate          = storage.ErrDuplicate
)
