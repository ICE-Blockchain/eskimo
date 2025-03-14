// SPDX-License-Identifier: ice License 1.0

package main

import (
	_ "embed"
	"mime/multipart"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/auth"
	emaillink "github.com/ice-blockchain/eskimo/auth/email_link"
	telegramauth "github.com/ice-blockchain/eskimo/auth/telegram"
	facekyc "github.com/ice-blockchain/eskimo/kyc/face"
	linkerkyc "github.com/ice-blockchain/eskimo/kyc/linking"
	kycquiz "github.com/ice-blockchain/eskimo/kyc/quiz"
	kycsocial "github.com/ice-blockchain/eskimo/kyc/social"
	verificationscenarios "github.com/ice-blockchain/eskimo/kyc/verification_scenarios"
	"github.com/ice-blockchain/eskimo/users"
)

// Public API.

type (
	GetMetadataArg                  struct{}
	ProcessFaceRecognitionResultArg struct {
		Disabled             *bool    `json:"disabled" required:"true"`
		PotentiallyDuplicate *bool    `json:"potentiallyDuplicate" required:"false"`
		APIKey               string   `header:"X-API-Key" swaggerignore:"true" required:"true" example:"some secret"`  //nolint:tagliatelle // Nope.
		UserID               string   `header:"X-User-ID" swaggerignore:"true" required:"false" example:"some secret"` //nolint:tagliatelle // Nope.
		LastUpdatedAt        []string `json:"lastUpdatedAt" required:"true" example:"2006-01-02T15:04:05Z"`
	}
	GetValidUserForPhoneNumberMigrationArg struct {
		PhoneNumber string `form:"phoneNumber" swaggerignore:"true" allowUnauthorized:"true" required:"true" example:"+12099216581"`
		Email       string `form:"email" swaggerignore:"true" required:"false" example:"jdoe@gmail.com"`
	}
	Metadata struct {
		UserID   string `json:"userId" example:"did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"`
		Metadata string `json:"metadata"`
	}
	CreateUserRequestBody struct {
		// Optional. Example: `{"key1":{"something":"somethingElse"},"key2":"value"}`.
		ClientData *users.JSON `json:"clientData"`
		// Optional.
		PhoneNumber string `json:"phoneNumber" example:"+12099216581"`
		// Optional. Required only if `phoneNumber` is set.
		PhoneNumberHash string `json:"phoneNumberHash" example:"Ef86A6021afCDe5673511376B2"`
		// Optional.
		Email string `json:"email" example:"jdoe@gmail.com"`
		// Optional.
		FirstName string `json:"firstName" example:"John"`
		// Optional.
		LastName string `json:"lastName" example:"Doe"`
		// Optional. Defaults to `en`.
		Language string `json:"language" example:"en"`
		// Optional.
		ReferredBy string `json:"referredBy" example:"did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"`
		// Optional.
		Username string `json:"username" example:"john.doe"`
		// Optional.
		TelegramUserID string `json:"telegramUserId" example:"1234566787"`
		// Optional.
		TelegramBotID string `json:"telegramBotId" example:"1234566787"`
	}
	ModifyUserRequestBody struct {
		UserID string `uri:"userId" swaggerignore:"true" required:"true" example:"did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"`
		// Optional. Example:`did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2`.
		ReferredBy string `form:"referredBy" formMultipart:"referredBy"`
		// Optional. Example: Array of [`globalRank`,`referralCount`,`level`,`role`,`badges`].
		HiddenProfileElements               *users.Enum[users.HiddenProfileElement] `form:"hiddenProfileElements" formMultipart:"hiddenProfileElements" swaggertype:"array,string" enums:"globalRank,referralCount,level,role,badges"` //nolint:lll // .
		ClearHiddenProfileElements          *bool                                   `form:"clearHiddenProfileElements" formMultipart:"clearHiddenProfileElements"`
		ClearMiningBlockchainAccountAddress *bool                                   `form:"clearMiningBlockchainAccountAddress" formMultipart:"clearMiningBlockchainAccountAddress"` //nolint:lll //.
		ClearTelegramInfo                   *bool                                   `form:"clearTelegramInfo" formMultipart:"clearTelegramInfo"`                                     //nolint:lll //.
		// Optional. Example: `{"key1":{"something":"somethingElse"},"key2":"value"}`.
		ClientData *string     `form:"clientData" formMultipart:"clientData"`
		clientData *users.JSON //nolint:revive // It's meant for internal use only.
		// Optional. Example:`true`.
		ResetProfilePicture *bool `form:"resetProfilePicture" formMultipart:"resetProfilePicture"`
		// Optional. Example:`true`.
		T1ReferralsSharingEnabled *bool `form:"t1ReferralsSharingEnabled" formMultipart:"t1ReferralsSharingEnabled"`
		// Optional.
		ProfilePicture *multipart.FileHeader `form:"profilePicture" formMultipart:"profilePicture" swaggerignore:"true"`
		// Optional. Example:`US`.
		Country string `form:"country" formMultipart:"country"`
		// Optional. Example:`New York`.
		City string `form:"city" formMultipart:"city"`
		// Optional. Example:`jdoe`.
		Username string `form:"username" formMultipart:"username"`
		// Optional. Example:`John`.
		FirstName string `form:"firstName" formMultipart:"firstName"`
		// Optional. Example:`Doe`.
		LastName string `form:"lastName" formMultipart:"lastName"`
		// Optional. Example:`+12099216581`.
		PhoneNumber string `form:"phoneNumber" formMultipart:"phoneNumber"`
		// Optional. Required only if `phoneNumber` is set. Example:`Ef86A6021afCDe5673511376B2`.
		PhoneNumberHash string `form:"phoneNumberHash" formMultipart:"phoneNumberHash"`
		// Optional. Example:`jdoe@gmail.com`.
		Email string `form:"email" formMultipart:"email"`
		// Optional. Example:`Ef86A6021afCDe5673511376B2,Ef86A6021afCDe5673511376B2,Ef86A6021afCDe5673511376B2,Ef86A6021afCDe5673511376B2`.
		AgendaPhoneNumberHashes string `form:"agendaPhoneNumberHashes" formMultipart:"agendaPhoneNumberHashes"`
		// Optional. Example:`some hash`.
		BlockchainAccountAddress       string `form:"blockchainAccountAddress" formMultipart:"blockchainAccountAddress"`
		MiningBlockchainAccountAddress string `form:"miningBlockchainAccountAddress" formMultipart:"miningBlockchainAccountAddress"`
		// Optional. Example:`en`.
		Language       string `form:"language" formMultipart:"language"`
		TelegramUserID string `form:"telegramUserId" formMultipart:"telegramUserId"`
		TelegramBotID  string `form:"telegramBotId" formMultipart:"telegramBotId"`
		// Optional. Example:`1232412415326543647657`.
		Checksum string `form:"checksum" formMultipart:"checksum"`
	}
	DeleteUserArg struct {
		UserID string `uri:"userId" required:"true" allowForbiddenWriteOperation:"true" example:"did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"`
	}
	GetDeviceLocationArg struct {
		// Optional. Set it to `-` if unknown.
		UserID string `uri:"userId" required:"true" allowUnauthorized:"true" example:"did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"`
		// Optional. Set it to `-` if unknown.
		DeviceUniqueID string `uri:"deviceUniqueId" required:"true" example:"FCDBD8EF-62FC-4ECB-B2F5-92C9E79AC7F9"`
	}
	ReplaceDeviceMetadataRequestBody struct {
		UserID         string `uri:"userId" allowUnauthorized:"true" required:"true" swaggerignore:"true" example:"did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"` //nolint:lll // .
		DeviceUniqueID string `uri:"deviceUniqueId" required:"true" swaggerignore:"true" example:"FCDBD8EF-62FC-4ECB-B2F5-92C9E79AC7F9"`
		Bogus          string `json:"bogus" swaggerignore:"true"` // It's just for the router to register the JSON body binder.
		users.DeviceMetadata
	}
	SendSignInLinkToEmailRequestArg struct {
		APIKey         string `header:"X-API-Key" swaggerignore:"true" required:"false" example:"some secret"` //nolint:tagliatelle // Nope.
		UserID         string `header:"X-User-ID" swaggerignore:"true" required:"false" example:"some secret"` //nolint:tagliatelle // Nope.
		Email          string `json:"email" allowUnauthorized:"true" required:"true" example:"jdoe@gmail.com"`
		DeviceUniqueID string `json:"deviceUniqueId" required:"true" example:"70063ABB-E69F-4FD2-8B83-90DD372802DA"`
		Language       string `json:"language" required:"true" example:"en"`
	}
	ClaimUserByThirdPartyRequestArg struct {
		APIKey     string `header:"X-API-Key" swaggerignore:"true" allowUnauthorized:"true" required:"true" example:"some secret"` //nolint:tagliatelle // Nope.
		Username   string `uri:"username" swaggerignore:"true" allowUnauthorized:"true" required:"true" example:"jdoe"`
		ThirdParty string `uri:"thirdParty" swaggerignore:"true" allowUnauthorized:"true" required:"true" example:"Facebook"`
	}
	StatusArg struct {
		LoginSession string `json:"loginSession" allowUnauthorized:"true" required:"true" example:"eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJPbmxpbmUgSldUIEJ1aWxkZXIiLCJpYXQiOjE2ODQzMjQ0NTYsImV4cCI6MTcxNTg2MDQ1NiwiYXVkIjoiIiwic3ViIjoianJvY2tldEBleGFtcGxlLmNvbSIsIm90cCI6IjUxMzRhMzdkLWIyMWEtNGVhNi1hNzk2LTAxOGIwMjMwMmFhMCJ9.q3xa8Gwg2FVCRHLZqkSedH3aK8XBqykaIy85rRU40nM"` //nolint:lll // .
	}
	Status struct {
		*RefreshedToken
		EmailConfirmed bool `json:"emailConfirmed,omitempty" example:"true"`
	}
	ModifyUserResponse struct {
		*UserProfile
		LoginSession string `json:"loginSession,omitempty" example:"eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJPbmxpbmUgSldUIEJ1aWxkZXIiLCJpYXQiOjE2ODQzMjQ0NTYsImV4cCI6MTcxNTg2MDQ1NiwiYXVkIjoiIiwic3ViIjoianJvY2tldEBleGFtcGxlLmNvbSIsIm90cCI6IjUxMzRhMzdkLWIyMWEtNGVhNi1hNzk2LTAxOGIwMjMwMmFhMCJ9.q3xa8Gwg2FVCRHLZqkSedH3aK8XBqykaIy85rRU40nM"` //nolint:lll // .
	}
	User struct {
		*users.UserProfile
		*kycquiz.QuizStatus
		Checksum         string `json:"checksum,omitempty" example:"1232412415326543647657"`
		KycFaceAvailable bool   `json:"kycFaceAvailable,omitempty" example:"true"`
	}
	Auth struct {
		RateLimit       string `json:"rateLimit,omitempty" example:"1000:24h"`
		LoginSession    string `json:"loginSession" example:"eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJPbmxpbmUgSldUIEJ1aWxkZXIiLCJpYXQiOjE2ODQzMjQ0NTYsImV4cCI6MTcxNTg2MDQ1NiwiYXVkIjoiIiwic3ViIjoianJvY2tldEBleGFtcGxlLmNvbSIsIm90cCI6IjUxMzRhMzdkLWIyMWEtNGVhNi1hNzk2LTAxOGIwMjMwMmFhMCJ9.q3xa8Gwg2FVCRHLZqkSedH3aK8XBqykaIy85rRU40nM"` //nolint:lll // .
		PositionInQueue int64  `json:"positionInQueue,omitempty" example:"675"`
	}
	RefreshedToken struct {
		*auth.Tokens
	}
	MagicLinkPayload struct {
		EmailToken       string `json:"emailToken" required:"true" allowUnauthorized:"true" example:"eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJPbmxpbmUgSldUIEJ1aWxkZXIiLCJpYXQiOjE2ODQzMjQ0NTYsImV4cCI6MTcxNTg2MDQ1NiwiYXVkIjoiIiwic3ViIjoianJvY2tldEBleGFtcGxlLmNvbSIsIm90cCI6IjUxMzRhMzdkLWIyMWEtNGVhNi1hNzk2LTAxOGIwMjMwMmFhMCJ9.q3xa8Gwg2FVCRHLZqkSedH3aK8XBqykaIy85rRU40nM"` //nolint:lll // .
		ConfirmationCode string `json:"confirmationCode" required:"true" example:"999"`
	}
	LoginFlowPayload struct {
		LoginSession     string `json:"loginSession" required:"true" allowUnauthorized:"true" example:"eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJPbmxpbmUgSldUIEJ1aWxkZXIiLCJpYXQiOjE2ODQzMjQ0NTYsImV4cCI6MTcxNTg2MDQ1NiwiYXVkIjoiIiwic3ViIjoianJvY2tldEBleGFtcGxlLmNvbSIsIm90cCI6IjUxMzRhMzdkLWIyMWEtNGVhNi1hNzk2LTAxOGIwMjMwMmFhMCJ9.q3xa8Gwg2FVCRHLZqkSedH3aK8XBqykaIy85rRU40nM"` //nolint:lll // .
		ConfirmationCode string `json:"confirmationCode" required:"true" example:"999"`
	}
	RefreshToken struct {
		Authorization string `header:"Authorization" swaggerignore:"true" allowForbiddenWriteOperation:"true" allowUnauthorized:"true"`
	}
	TelegramSignIn struct {
		TelegramBotID *string `json:"telegramBotId" required:"false"`
		Authorization string  `header:"Authorization" swaggerignore:"true" allowForbiddenWriteOperation:"true" allowUnauthorized:"true"`
	}
	StartOrContinueKYCStep4SessionRequestBody struct {
		QuestionNumber *uint8 `form:"questionNumber" required:"true" swaggerignore:"true" example:"11"`
		SelectedOption *uint8 `form:"selectedOption" required:"true" swaggerignore:"true" example:"0"`
		Language       string `form:"language" required:"true" swaggerignore:"true" example:"en"`
	}
	CheckKYCStep4StatusRequestBody struct {
		XClientType string `form:"x_client_type" swaggerignore:"true" required:"false" example:"web"`
	}
	TryResetKYCStepsRequestBody struct {
		NextKYCStep      *users.KYCStep  `form:"nextKYCStep" required:"false" swaggerignore:"true" example:"1"`
		Authorization    string          `header:"Authorization" swaggerignore:"true" required:"true" example:"some token"`
		XAccountMetadata string          `header:"X-Account-Metadata" swaggerignore:"true" required:"false" example:"some token"`
		UserID           string          `uri:"userId" required:"true" allowForbiddenWriteOperation:"true" swaggerignore:"true" example:"did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"` //nolint:lll // .
		SkipKYCSteps     []users.KYCStep `form:"skipKYCSteps" swaggerignore:"true" example:"3,4,5,6,7,8,9,10"`
	}
	ForwardToFaceKYCRequestBody struct {
		XClientType string            `form:"x_client_type" swaggerignore:"true" required:"false" example:"web"`
		Tokens      map[string]string `json:"tokens"`
		UserID      string            `uri:"userId" required:"true" allowForbiddenWriteOperation:"true" swaggerignore:"true" example:"did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"` //nolint:lll // .
	}
	ForwardToFaceKYCResponse struct {
		KycFaceAvailable bool `json:"kycFaceAvailable" example:"true"`
	}
	GetRequiredVerificationEligibilityScenariosArg struct {
		Authorization string `header:"Authorization" swaggerignore:"true" required:"true" example:"some token"`
		UserID        string `uri:"userId" required:"true" example:"did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"`
	}
)

// Private API.

const (
	applicationYamlKey = "cmd/eskimo-hut"
	swaggerRootSuffix  = "/users/w"
	tenantCtxKey       = "tenantCtxKey"
	doctorxTenant      = "doctorx"
)

// Values for server.ErrorResponse#Code.
const (
	deviceMetadataAppUpdateRequireErrorCode = "UPDATE_REQUIRED"
	invalidUsernameErrorCode                = "INVALID_USERNAME"
	userNotFoundErrorCode                   = "USER_NOT_FOUND"
	metadataNotFoundErrorCode               = "METADATA_NOT_FOUND"
	userBlockedErrorCode                    = "USER_BLOCKED"
	duplicateUserErrorCode                  = "CONFLICT_WITH_ANOTHER_USER"
	referralNotFoundErrorCode               = "REFERRAL_NOT_FOUND"
	raceConditionErrorCode                  = "RACE_CONDITION"
	invalidPropertiesErrorCode              = "INVALID_PROPERTIES"
	invalidEmail                            = "INVALID_EMAIL"
	emailUsedBySomebodyElseEmail            = "EMAIL_USED_BY_SOMEBODY_ELSE"
	emailAlreadySetErrorCode                = "EMAIL_ALREADY_SET"
	accountLostErrorCode                    = "ACCOUNT_LOST"

	expiredLoginSessionErrorCode = "EXPIRED_LOGIN_SESSION"
	invalidLoginSessionErrorCode = "INVALID_LOGIN_SESSION"
	dataMismatchErrorCode        = "DATA_MISMATCH"
	seqMismatchErrorCode         = "TOKEN_SEQUENCE_MISMATCH"

	confirmationCodeNotFoundErrorCode         = "CONFIRMATION_CODE_NOT_FOUND"
	confirmationCodeAttemptsExceededErrorCode = "CONFIRMATION_CODE_ATTEMPTS_EXCEEDED"
	confirmationCodeWrongErrorCode            = "CONFIRMATION_CODE_WRONG"
	tooManyRequests                           = "TOO_MANY_REQUESTS"

	quizUnknownQuestionNumErrorCode = "QUIZ_UNKNOWN_QUESTION_NUM"
	quizDisbledErrorCode            = "QUIZ_DISABLED"

	socialKYCStepAlreadyCompletedSuccessfullyErrorCode = "SOCIAL_KYC_STEP_ALREADY_COMPLETED_SUCCESSFULLY"
	socialKYCStepNotAvailableErrorCode                 = "SOCIAL_KYC_STEP_NOT_AVAILABLE"
	linkingNotOwnedProfile                             = "NOT_OWNER_OF_REMOTE_USER"
	linkingDuplicate                                   = "DUPLICATE"

	kycVerificationScenariosVadidationFailedErrorCode   = "VALIDATION_FAILED"
	kycVerificationScenariosNoPendingScenariosErrorCode = "NO_PENDING_SCENARIOS"
	kycVerificationScenariosNoTenantTokens              = "NO_TENANT_TOKENS" //nolint:gosec // .

	deviceIDTokenClaim = "deviceUniqueID" //nolint:gosec // .

	adminRole = "admin"
)

// .
var (
	//nolint:gochecknoglobals // Because its loaded once, at runtime.
	cfg             config
	errNoPermission = errors.New("insufficient role")
)

type (
	// | service implements server.State and is responsible for managing the state and lifecycle of the package.
	service struct {
		usersProcessor                  users.Processor
		quizRepository                  kycquiz.Repository
		authEmailLinkClient             emaillink.Client
		telegramAuthClient              telegramauth.Client
		tokenRefresher                  auth.TokenRefresher
		socialRepository                kycsocial.Repository
		faceKycClient                   facekyc.Client
		usersLinker                     linkerkyc.Linker
		verificationScenariosRepository verificationscenarios.Repository
	}
	config struct {
		APIKey           string `yaml:"api-key" mapstructure:"api-key"`                         //nolint:tagliatelle // Nope.
		ThirdPartyAPIKey string `yaml:"third-party-api-key" mapstructure:"third-party-api-key"` //nolint:tagliatelle // Nope.
		Host             string `yaml:"host"`
		Version          string `yaml:"version"`
		Tenant           string `yaml:"tenant"`
	}
)
