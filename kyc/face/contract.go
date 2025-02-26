// SPDX-License-Identifier: ice License 1.0

package face

import (
	"context"
	_ "embed"
	"io"
	"sync/atomic"
	stdlibtime "time"

	"github.com/ice-blockchain/eskimo/kyc/face/internal"
	"github.com/ice-blockchain/eskimo/kyc/face/internal/threedivi"
	"github.com/ice-blockchain/eskimo/users"
	"github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/time"
)

type (
	Tenant         = string
	Token          = string
	UserRepository = internal.UserRepository
	Config         struct {
		kycConfigJSON           *atomic.Pointer[kycConfigJSON]
		ConfigJSONURL           string           `yaml:"config-json-url" mapstructure:"config-json-url"` //nolint:tagliatelle // .
		ThreeDiVi               threedivi.Config `mapstructure:",squash"`                                //nolint:tagliatelle // .
		UnexpectedErrorsAllowed uint64           `yaml:"unexpectedErrorsAllowed" mapstructure:"unexpectedErrorsAllowed"`
	}
	Linker interface {
		Verify(ctx context.Context, now *time.Time, userID string, tokens map[Tenant]Token) (allLinkedProfiles map[Tenant]string, verified Tenant, err error)
		Get(ctx context.Context, userID string) (allLinkedProfiles map[Tenant]string, verified Tenant, err error)
		SetTenantVerified(ctx context.Context, userID string, tenant Tenant) error
	}
	Client interface {
		io.Closer
		Reset(ctx context.Context, user *users.User, fetchState bool) error
		CheckStatus(ctx context.Context, user *users.User, nextKYCStep users.KYCStep) (available bool, err error)
		ForwardToKYC(ctx context.Context, userID string) (available bool, err error)
	}
)

type (
	client struct {
		client           internalClient
		users            UserRepository
		accountsLinker   Linker
		db               *storage.DB
		cfg              Config
		unexpectedErrors atomic.Uint64
	}
	internalClient = internal.Client
	kycConfigJSON  struct {
		FaceKYC struct {
			Enabled bool `json:"enabled"`
		} `json:"face-auth"` //nolint:tagliatelle // .
		WebFaceKYC struct {
			Enabled bool `json:"enabled"`
		} `json:"web-face-auth"` //nolint:tagliatelle // .
	}
)

const (
	applicationYamlKey    = "kyc/face"
	refreshTime           = 1 * stdlibtime.Minute
	requestDeadline       = 25 * stdlibtime.Second
	clientTypeCtxValueKey = "clientTypeCtxValueKey"
)

//nolint:grouper // .
//go:embed DDL.sql
var ddl string
