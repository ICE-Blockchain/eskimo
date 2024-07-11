// SPDX-License-Identifier: ice License 1.0

package threedivi

import (
	"sync/atomic"
	stdlibtime "time"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/kyc/face/internal"
)

type (
	threeDivi struct {
		users                  internal.UserRepository
		cfg                    *Config
		loadBalancedUsersCount atomic.Uint64
		activeUsersCount       atomic.Uint64
	}
	Config struct {
		ThreeDiVi struct {
			BAFHost         string `yaml:"bafHost"`
			BAFToken        string `yaml:"bafToken"`
			SecretAPIToken  string `yaml:"secretApiToken"`
			AvailabilityURL string `yaml:"availabilityUrl"`
			ConcurrentUsers int    `yaml:"concurrentUsers"`
		} `yaml:"threeDiVi"`
	}
)

// Private API.
type (
	applicant struct {
		Code                   string              `json:"code"`
		LastValidationResponse *validationResponse `json:"lastValidationResponse"`
		Metadata               *metadata           `json:"metadata"`
		ApplicantID            string              `json:"applicantId"`
		Email                  string              `json:"email"`
		Status                 int                 `json:"status"`
		HasRiskEvents          bool                `json:"hasRiskEvents"`
	}
	validationResponse struct {
		SimilarApplicants *struct {
			IDs []string `json:"ids"` //nolint:tagliatelle // .
		} `json:"similarApplicants"`
		RiskEvents           *[]riskEvent `json:"riskEvents"`
		CreatedAt            stdlibtime.Time
		Created              string `json:"created"`
		ResponseStatusName   string `json:"responseStatusName"`
		ValidationResponseID uint64 `json:"validationResponseId"`
		ResponseStatus       int    `json:"responseStatus"`
	}
	riskEvent struct {
		RiskName string `json:"riskName"`
		IsActive bool   `json:"isActive"`
	}
	metadata struct {
		Tenant string `json:"tenant"`
	}
)

const (
	requestDeadline               = 30 * stdlibtime.Second
	metricOpenConnections         = "stunner_listener_connections"
	connsPerUser                  = 2
	metricOpenConnectionsLabelTCP = "default/tcp-gateway/tcp-listener"
	statusPassed                  = 1
	statusFailed                  = 2
	codeApplicantNotFound         = "120024"
	duplicatedFaceRisk            = "DuplicateFace"
	bafTimeFormat                 = "2006-01-02T15:04:05.999999"
)

var ( //nolint:gofumpt // .
	errFaceAuthNotStarted = errors.New("face auth not started")
)
