// SPDX-License-Identifier: ice License 1.0

package verificationscenarios

import (
	"context"
	_ "embed"
	"errors"
	"io"
	"mime/multipart"
	"sync/atomic"
	stdlibtime "time"

	"github.com/ice-blockchain/eskimo/kyc/scraper"
	"github.com/ice-blockchain/eskimo/kyc/social"
	"github.com/ice-blockchain/eskimo/users"
	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
)

// Public API.

const (
	// Scenarios.
	CoinDistributionScenarioCmc           Scenario = "join_cmc"
	CoinDistributionScenarioTwitter       Scenario = "join_twitter"
	CoinDistributionScenarioTelegram      Scenario = "join_telegram"
	CoinDistributionScenarioSignUpTenants Scenario = "signup_tenants"

	// Tenant scenarios.
	CoinDistributionScenarioSignUpSunwaves   TenantScenario = "signup_sunwaves"
	CoinDistributionScenarioSignUpSealsend   TenantScenario = "signup_sealsend"
	CoinDistributionScenarioSignUpCallfluent TenantScenario = "signup_callfluent"
	CoinDistributionScenarioSignUpSauces     TenantScenario = "signup_sauces"
	CoinDistributionScenarioSignUpDoctorx    TenantScenario = "signup_doctorx"
)

// .
var (
	ErrVerificationNotPassed = errors.New("not passed")
	ErrNoPendingScenarios    = errors.New("not pending scenarios")
	ErrWrongTenantTokens     = errors.New("wrong tenant tokens")
)

type (
	Tenant         string
	Token          string
	Scenario       string
	TenantScenario string
	Repository     interface {
		io.Closer
		VerifyScenarios(ctx context.Context, metadata *VerificationMetadata) (*social.Verification, error)
		GetPendingVerificationScenarios(ctx context.Context, userID string) ([]*Scenario, error)
	}
	UserRepository interface {
		io.Closer
		GetUserByID(ctx context.Context, userID string) (*users.UserProfile, error)
		ModifyUser(ctx context.Context, usr *users.User, profilePicture *multipart.FileHeader) (*users.UserProfile, error)
	}
	VerificationMetadata struct {
		Authorization    string                   `header:"Authorization" swaggerignore:"true" required:"true" example:"some token"`
		UserID           string                   `uri:"userId" required:"true" allowForbiddenWriteOperation:"true" swaggerignore:"true" example:"did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"` //nolint:lll // .
		ScenarioEnum     Scenario                 `uri:"scenarioEnum" example:"join_cmc" swaggerignore:"true" required:"true" enums:"join_cmc,join_twitter,join_telegram,signup_tenants"`               //nolint:lll // .
		Language         string                   `json:"language" required:"false" swaggerignore:"true" example:"en"`
		TenantTokens     map[TenantScenario]Token `json:"tenantTokens" required:"false" example:"sunwaves:sometoken,sealsend:sometoken"`
		CMCProfileLink   string                   `json:"cmcProfileLink" required:"false" example:"some profile"`
		TweetURL         string                   `json:"tweetUrl" required:"false" example:"some tweet"`
		TelegramUsername string                   `json:"telegramUsername" required:"false" example:"some telegram username"`
	}
)

// Private API.

const (
	applicationYamlKey       = "kyc/coinDistributionEligibility"
	globalApplicationYamlKey = "globalDb"
	authorizationCtxValueKey = "authorizationCtxValueKey"

	verificationTwitterScenarioKYCStep int8 = 120
	requestDeadline                         = 25 * stdlibtime.Second
)

// .
var (
	//go:embed DDL.sql
	ddl string
)

type (
	repository struct {
		cfg             *config
		globalDB        *storage.DB
		db              *storage.DB
		userRepo        UserRepository
		twitterVerifier scraper.Verifier
		host            string
	}
	config struct {
		TenantURLs         map[string]string `yaml:"tenantURLs" mapstructure:"tenantURLs"` //nolint:tagliatelle // .
		kycConfigJSON1     *atomic.Pointer[social.KycConfigJSON]
		Tenant             string              `yaml:"tenant" mapstructure:"tenant"`
		ConfigJSONURL1     string              `yaml:"configJsonUrl1" mapstructure:"configJsonUrl1"` //nolint:tagliatelle // .
		SessionWindow      stdlibtime.Duration `yaml:"sessionWindow" mapstructure:"sessionWindow"`   //nolint:tagliatelle // .
		MaxAttemptsAllowed uint8               `yaml:"maxAttemptsAllowed" mapstructure:"maxAttemptsAllowed"`
	}
)
