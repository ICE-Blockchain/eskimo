// SPDX-License-Identifier: ice License 1.0

package emaillinkiceauth

import (
	"context"
	"embed"
	"io"
	"mime/multipart"
	"sync"
	"text/template"
	stdlibtime "time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/auth"
	"github.com/ice-blockchain/eskimo/users"
	wintrauth "github.com/ice-blockchain/wintr/auth"
	"github.com/ice-blockchain/wintr/connectors/storage/v2"
	storagev3 "github.com/ice-blockchain/wintr/connectors/storage/v3"
	"github.com/ice-blockchain/wintr/email"
	"github.com/ice-blockchain/wintr/time"
)

// Public API.

type (
	UserModifier interface {
		ModifyUser(ctx context.Context, usr *users.User, profilePicture *multipart.FileHeader) (*users.UserProfile, error)
	}
	Client interface {
		IceUserIDClient
		SendSignInLinkToEmail(ctx context.Context, emailValue, deviceUniqueID, language, clientIP string) (queuePos int64, rateLimit, loginSession string, err error)
		SignIn(ctx context.Context, loginFlowToken, confirmationCode string) (tokens *auth.Tokens, emailConfirmed bool, err error)
		UpdateMetadata(ctx context.Context, userID string, metadata *users.JSON) (*users.JSON, error)
		CheckHealth(ctx context.Context) error
		RefreshToken(ctx context.Context, token *wintrauth.IceToken) (tokens *auth.Tokens, err error)
	}
	IceUserIDClient interface {
		io.Closer
		IceUserID(ctx context.Context, mail string) (iceID string, err error)
		Metadata(ctx context.Context, userID, emailAddress string) (metadata string, metadataFields *users.JSON, err error)
	}
	Metadata struct {
		UserID   string `json:"userId" example:"1c0b9801-cfb2-4c4e-b48a-db18ce0894f9"`
		Metadata string `json:"metadata"`
	}
)

var (
	ErrInvalidToken           = auth.ErrInvalidToken
	ErrExpiredToken           = auth.ErrExpiredToken
	ErrNoConfirmationRequired = errors.New("no pending confirmation")

	ErrUserDataMismatch = errors.New("parameters were not equal to user data in db")
	ErrUserNotFound     = storage.ErrNotFound
	ErrUserDuplicate    = errors.New("such user already exists")

	ErrConfirmationCodeWrong            = errors.New("wrong confirmation code provided")
	ErrConfirmationCodeAttemptsExceeded = errors.New("confirmation code attempts exceeded")
	ErrStatusNotVerified                = errors.New("not verified")
	ErrNoPendingLoginSession            = errors.New("no pending login session")
	ErrUserBlocked                      = errors.New("user is blocked")
	ErrTooManyAttempts                  = errors.New("too many attempts")
)

const (
	TelegramUserSettingUpEmailPrefix = "telegram@@"
)

// Private API.

const (
	applicationYamlKey = "auth/email-link"
	jwtIssuer          = "ice.io"
	defaultLanguage    = "en"

	phoneNumberToEmailMigrationCtxValueKey = "phoneNumberToEmailMigrationCtxValueKey"

	signInEmailType        string = "signin"
	notifyEmailChangedType string = "notify_changed"
	modifyEmailType        string = "modify_email"

	iceIDPrefix = "ice_"

	textExtension = "txt"
	htmlExtension = "html"

	sameIPCheckRate = 24 * stdlibtime.Hour

	duplicatedSignInRequestsInLessThan = 2 * stdlibtime.Second
	loginQueueKey                      = "login_queue"
	loginQueueTTLKey                   = "login_queue_ttl"
	loginRateLimitKey                  = "login_rate_limit"
	initEmailRateLimit                 = "1000:1m"
	loginCodeLength                    = 3
)

type (
	languageCode = string
	client       struct {
		queueDB            storagev3.DB
		db                 *storage.DB
		cfg                *config
		shutdown           func() error
		authClient         wintrauth.Client
		userModifier       UserModifier
		cancel             context.CancelFunc
		emailClients       []email.Client
		fromRecipients     []fromRecipient
		queueWg            sync.WaitGroup
		emailClientLBIndex uint64
	}
	config struct {
		FromEmailName    string `yaml:"fromEmailName"`
		FromEmailAddress string `yaml:"fromEmailAddress"`
		PetName          string `yaml:"petName"`
		AppName          string `yaml:"appName"`
		TeamName         string `yaml:"teamName"`
		LoginSession     struct {
			JwtSecret string `yaml:"jwtSecret"`
		} `yaml:"loginSession"`
		ConfirmationCode struct {
			ConstCodes            map[string]string `yaml:"constCodes" mapstructure:"constCodes"`
			MaxWrongAttemptsCount int64             `yaml:"maxWrongAttemptsCount"`
		} `yaml:"confirmationCode"`
		EmailValidation struct {
			AuthLink       string              `yaml:"authLink"`
			ExpirationTime stdlibtime.Duration `yaml:"expirationTime" mapstructure:"expirationTime"`
			BlockDuration  stdlibtime.Duration `yaml:"blockDuration"`
		} `yaml:"emailValidation"`
		QueueAliveTTL           stdlibtime.Duration `yaml:"queueAliveTTL" mapstructure:"queueAliveTTL"` //nolint:tagliatelle // .
		ExtraLoadBalancersCount int                 `yaml:"extraLoadBalancersCount"`
		DisableEmailSending     bool                `yaml:"disableEmailSending"`
		QueueProcessing         bool                `yaml:"queueProcessing"`
	}
	loginID struct {
		Email          string `json:"email,omitempty" example:"someone1@example.com"`
		DeviceUniqueID string `json:"deviceUniqueId,omitempty" example:"6FB988F3-36F4-433D-9C7C-555887E57EB2" db:"device_unique_id"`
	}
	loginFlowToken struct {
		*jwt.RegisteredClaims
		DeviceUniqueID     string `json:"deviceUniqueId,omitempty"`
		ClientIP           string `json:"clientIP,omitempty"` //nolint:tagliatelle //.
		OldEmail           string `json:"oldEmail,omitempty"`
		NotifyEmail        string `json:"notifyEmail,omitempty"`
		LoginSessionNumber int64  `json:"loginSessionNumber,omitempty"`
	}
	emailLinkSignIn struct {
		CreatedAt                          *time.Time
		TokenIssuedAt                      *time.Time
		BlockedUntil                       *time.Time
		EmailConfirmedAt                   *time.Time
		Metadata                           *users.JSON `json:"metadata,omitempty"`
		UserID                             *string     `json:"userId" example:"did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"`
		PhoneNumberToEmailMigrationUserID  *string     `json:"-" example:"did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"`
		Email                              string      `json:"email,omitempty" example:"someone1@example.com"`
		Language                           string      `json:"language,omitempty" example:"en"`
		DeviceUniqueID                     string      `json:"deviceUniqueId,omitempty" example:"6FB988F3-36F4-433D-9C7C-555887E57EB2" db:"device_unique_id"`
		ConfirmationCode                   string      `json:"confirmationCode,omitempty" example:"123"`
		IssuedTokenSeq                     int64       `json:"issuedTokenSeq,omitempty" example:"1"`
		PreviouslyIssuedTokenSeq           int64       `json:"previouslyIssuedTokenSeq,omitempty" example:"1"`
		ConfirmationCodeWrongAttemptsCount int64       `json:"confirmationCodeWrongAttemptsCount,omitempty" example:"3" db:"confirmation_code_wrong_attempts_count"`
		HashCode                           int64       `json:"hashCode,omitempty" example:"43453546464576547"`
	}
	emailTemplate struct {
		subject, body *template.Template
		Subject       string `json:"subject"` //nolint:revive // That's intended.
		Body          string `json:"body"`    //nolint:revive // That's intended.
	}
	metadata struct {
		Metadata *users.JSON
		Email    *string
		UserID   *string
	}
	fromRecipient struct {
		FromEmailName    string
		FromEmailAddress string
	}
)

// .
var (
	//go:embed DDL.sql
	ddl string
	//go:embed translations
	translations embed.FS
	//nolint:gochecknoglobals // Its loaded once at startup.
	allEmailLinkTemplates map[string]map[languageCode]*emailTemplate

	//nolint:gochecknoglobals // It's just for more descriptive validation messages.
	allEmailTypes = users.Enum[string]{
		signInEmailType,
		modifyEmailType,
		notifyEmailChangedType,
	}
	errAlreadyEnqueued = errors.New("already enqueued")
)
