// SPDX-License-Identifier: ice License 1.0

package verificationscenarios

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"sync/atomic"
	stdlibtime "time"

	"github.com/ice-blockchain/eskimo/kyc/linking"
	"github.com/ice-blockchain/eskimo/kyc/scraper"
	"github.com/ice-blockchain/eskimo/kyc/social"
	"github.com/ice-blockchain/eskimo/users"
)

// Public API.

const (
	// Scenarios.
	CoinDistributionScenarioCmc           Scenario = "join_cmc"
	CoinDistributionScenarioTwitter       Scenario = "join_twitter"
	CoinDistributionScenarioTelegram      Scenario = "join_telegram"
	CoinDistributionScenarioSignUpTenants Scenario = "signup_tenants"

	// Tenant scenarios.
	CoinDistributionScenarioSignUpSunwaves     TenantScenario = "signup_sunwaves"
	CoinDistributionScenarioSignUpSealsend     TenantScenario = "signup_sealsend"
	CoinDistributionScenarioSignUpCallfluent   TenantScenario = "signup_callfluent"
	CoinDistributionScenarioSignUpSauces       TenantScenario = "signup_sauces"
	CoinDistributionScenarioSignUpDoctorx      TenantScenario = "signup_doctorx"
	CoinDistributionScenarioSignUpCryptomayors TenantScenario = "signup_cryptomayors"

	singUpPrefix = "signup"
)

// .
var (
	ErrVerificationNotPassed = errors.New("not passed")
	ErrNoPendingScenarios    = errors.New("not pending scenarios")
	ErrWrongTenantTokens     = errors.New("wrong tenant tokens")
)

type (
	Tenant         string
	Scenario       string
	Token          = string
	TenantScenario = string
	Repository     interface {
		VerifyScenarios(ctx context.Context, metadata *VerificationMetadata) (res *Verification, err error)
		GetPendingVerificationScenarios(ctx context.Context, userID string) ([]Scenario, error)
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
		Language         string                   `json:"language" required:"false" example:"en"`
		TenantTokens     map[TenantScenario]Token `json:"tenantTokens" required:"false" example:"signup_sunwaves:sometoken,signup_sealsend:sometoken,signup_callfluent:sometoken,signup_doctorx:sometoken,signup_sauces:sometoken,signup_cryptomayors:sometoken"` //nolint:lll // .
		CMCProfileLink   string                   `json:"cmcProfileLink" required:"false" example:"some profile"`
		TweetURL         string                   `json:"tweetUrl" required:"false" example:"some tweet"`
		TelegramUsername string                   `json:"telegramUsername" required:"false" example:"some telegram username"`
	}
	Verification struct {
		ExpectedPostText string `json:"expectedPostText,omitempty" example:"This is a verification post!"`
	}
)

// Private API.

const (
	applicationYamlKey       = "kyc/coinDistributionEligibility"
	authorizationCtxValueKey = "authorizationCtxValueKey"

	requestDeadline = 25 * stdlibtime.Second
)

// .
var (
	//nolint:gochecknoglobals,gomnd // We need it to sort scenarios.
	scenarioOrder = map[Scenario]int{
		Scenario(CoinDistributionScenarioSignUpSunwaves):     0,
		Scenario(CoinDistributionScenarioSignUpCallfluent):   1,
		Scenario(CoinDistributionScenarioSignUpDoctorx):      2,
		Scenario(CoinDistributionScenarioSignUpSauces):       3,
		Scenario(CoinDistributionScenarioSignUpSealsend):     4,
		Scenario(CoinDistributionScenarioSignUpCryptomayors): 5,
		CoinDistributionScenarioTwitter:                      6,
		CoinDistributionScenarioTelegram:                     7,
		CoinDistributionScenarioCmc:                          8,
	}
)

type (
	repository struct {
		cfg             *config
		userRepo        UserRepository
		twitterVerifier scraper.Verifier
		cmcVerifier     scraper.Verifier
		linkerRepo      linking.Linker
		socialRepo      social.Repository
		host            string
	}
	config struct {
		TenantURLs         map[string]string `yaml:"tenantURLs" mapstructure:"tenantURLs"` //nolint:tagliatelle // .
		kycConfigJSON1     *atomic.Pointer[social.KycConfigJSON]
		Tenant             string     `yaml:"tenant" mapstructure:"tenant"`
		ConfigJSONURL1     string     `yaml:"config-json-url1" mapstructure:"config-json-url1"`     //nolint:tagliatelle // .
		MandatoryScenarios []Scenario `yaml:"mandatoryScenarios" mapstructure:"mandatoryScenarios"` //nolint:tagliatelle // .
	}
)
