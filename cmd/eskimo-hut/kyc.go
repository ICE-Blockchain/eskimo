// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/pkg/errors"

	facekyc "github.com/ice-blockchain/eskimo/kyc/face"
	"github.com/ice-blockchain/eskimo/kyc/linking"
	kycquiz "github.com/ice-blockchain/eskimo/kyc/quiz"
	kycsocial "github.com/ice-blockchain/eskimo/kyc/social"
	verificationscenarios "github.com/ice-blockchain/eskimo/kyc/verification_scenarios"
	"github.com/ice-blockchain/eskimo/users"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/server"
)

func (s *service) setupKYCWriteRoutes(router *server.Router) {
	router.
		Group("v1w").
		POST("kyc/startOrContinueKYCStep4Session/users/:userId", server.RootHandler(s.StartOrContinueKYCStep4Session)).
		POST("kyc/checkKYCStep4Status/users/:userId", server.RootHandler(s.CheckKYCStep4Status)).
		POST("kyc/verifySocialKYCStep/users/:userId", server.RootHandler(s.VerifySocialKYCStep)).
		POST("kyc/tryResetKYCSteps/users/:userId", server.RootHandler(s.TryResetKYCSteps)).
		POST("kyc/checkFaceKYCStatus/users/:userId", server.RootHandler(s.ForwardToFaceKYC)).
		POST("kyc/verifyCoinDistributionEligibility/users/:userId/scenarios/:scenarioEnum", server.RootHandler(s.VerifyKYCScenarios))
}

func (s *service) setupKYCReadRoutes(router *server.Router) {
	router.
		Group("v1r").
		GET("kyc/verifyCoinDistributionEligibility/users/:userId", server.RootHandler(s.GetPendingKYCVerificationScenarios))
}

func (s *service) startQuizSession(ctx context.Context, userID users.UserID, lang string) (*kycquiz.Quiz, error) {
	const defaultLanguage = "en"

	if strings.EqualFold(lang, defaultLanguage) {
		return s.quizRepository.StartQuizSession(ctx, userID, defaultLanguage) //nolint:wrapcheck // .
	}

	quiz, err := s.quizRepository.StartQuizSession(ctx, userID, lang)
	if err != nil {
		if errors.Is(err, kycquiz.ErrUnknownLanguage) {
			log.Warn(fmt.Sprintf("failed to StartQuizSession for userID:%v,language:%v, trying default language:%v", userID, lang, defaultLanguage))

			return s.quizRepository.StartQuizSession(ctx, userID, defaultLanguage) //nolint:wrapcheck // .
		}
	}

	return quiz, err //nolint:wrapcheck // .
}

// StartOrContinueKYCStep4Session godoc
//
//	@Schemes
//	@Description	Starts or continues the kyc 4 session (Quiz), if available and if not already finished successfully.
//	@Tags			KYC
//	@Accept			json
//	@Produce		json
//
//	@Param			Authorization		header		string	true	"Insert your access token"		default(Bearer <Add access token here>)
//	@Param			X-Account-Metadata	header		string	false	"Insert your metadata token"	default(<Add metadata token here>)
//	@Param			userId				path		string	true	"ID of the user"
//	@Param			language			query		string	true	"language of the user"
//	@Param			selectedOption		query		int		true	"index of the options array. Set it to 222 for the first call."
//	@Param			questionNumber		query		int		true	"previous question number. Set it to 222 for the first call."
//	@Success		200					{object}	kycquiz.Quiz
//	@Failure		400					{object}	server.ErrorResponse	"if validations fail"
//	@Failure		401					{object}	server.ErrorResponse	"if not authorized"
//	@Failure		403					{object}	server.ErrorResponse	"not allowed due to various reasons"
//	@Failure		404					{object}	server.ErrorResponse	"user is not found"
//	@Failure		409					{object}	server.ErrorResponse	"if any conflicts occur or any prerequisites are not met"
//	@Failure		422					{object}	server.ErrorResponse	"if syntax fails"
//	@Failure		500					{object}	server.ErrorResponse
//	@Failure		504					{object}	server.ErrorResponse	"if request times out"
//	@Router			/v1w/kyc/startOrContinueKYCStep4Session/users/{userId} [POST].
func (s *service) StartOrContinueKYCStep4Session( //nolint:gocritic,funlen // .
	ctx context.Context,
	req *server.Request[StartOrContinueKYCStep4SessionRequestBody, kycquiz.Quiz],
) (*server.Response[kycquiz.Quiz], *server.Response[server.ErrorResponse]) {
	const (
		magicNumberQuizStart = 222
	)

	// Handle the session start.
	if *req.Data.QuestionNumber == magicNumberQuizStart && *req.Data.SelectedOption == magicNumberQuizStart {
		quiz, err := s.startQuizSession(ctx, req.AuthenticatedUser.UserID, req.Data.Language)
		err = errors.Wrapf(err, "failed to StartQuizSession for userID:%v,language:%v", req.AuthenticatedUser.UserID, req.Data.Language)
		if err != nil {
			log.Error(err)
			switch {
			case errors.Is(err, kycquiz.ErrUnknownUser):
				return nil, server.NotFound(err, userNotFoundErrorCode)

			case errors.Is(err, kycquiz.ErrSessionFinished), errors.Is(err, kycquiz.ErrSessionFinishedWithError), errors.Is(err, kycquiz.ErrInvalidKYCState): //nolint:lll // .
				return nil, server.BadRequest(err, raceConditionErrorCode)

			case errors.Is(err, kycquiz.ErrNotAvailable):
				return nil, server.ForbiddenWithCode(err, quizDisbledErrorCode)

			default:
				return nil, server.Unexpected(err)
			}
		}

		return server.OK(quiz), nil
	}

	// Handle the session continuation.
	session, err := s.quizRepository.ContinueQuizSession(ctx, req.AuthenticatedUser.UserID, *req.Data.QuestionNumber, *req.Data.SelectedOption)
	if err != nil {
		err = errors.Wrapf(err, "failed to ContinueQuizSession for userID:%v,question:%v,option:%v", req.AuthenticatedUser.UserID, *req.Data.QuestionNumber, *req.Data.SelectedOption) //nolint:lll // .
		switch {
		case errors.Is(err, kycquiz.ErrUnknownUser) || errors.Is(err, kycquiz.ErrUnknownSession):
			return nil, server.NotFound(err, userNotFoundErrorCode)

		case errors.Is(err, kycquiz.ErrSessionFinished), errors.Is(err, kycquiz.ErrSessionFinishedWithError), errors.Is(err, kycquiz.ErrInvalidKYCState): //nolint:lll // .
			return nil, server.BadRequest(err, raceConditionErrorCode)

		case errors.Is(err, kycquiz.ErrUnknownQuestionNumber):
			return nil, server.BadRequest(err, quizUnknownQuestionNumErrorCode)

		default:
			return nil, server.Unexpected(err)
		}
	}

	return server.OK(session), nil
}

// CheckKYCStep4Status godoc
//
//	@Schemes
//	@Description	Checks the status of the quiz kyc step (4).
//	@Tags			KYC
//	@Accept			json
//	@Produce		json
//
//	@Param			Authorization		header		string	true	"Insert your access token"		default(Bearer <Add access token here>)
//	@Param			X-Account-Metadata	header		string	false	"Insert your metadata token"	default(<Add metadata token here>)
//	@Param			x_client_type		query		string	false	"the type of the client calling this API. I.E. `web`"
//	@Param			userId				path		string	true	"ID of the user"
//	@Success		200					{object}	kycquiz.QuizStatus
//	@Failure		400					{object}	server.ErrorResponse	"if validations fail"
//	@Failure		401					{object}	server.ErrorResponse	"if not authorized"
//	@Failure		403					{object}	server.ErrorResponse	"not allowed due to various reasons"
//	@Failure		422					{object}	server.ErrorResponse	"if syntax fails"
//	@Failure		500					{object}	server.ErrorResponse
//	@Failure		504					{object}	server.ErrorResponse	"if request times out"
//	@Router			/v1w/kyc/checkKYCStep4Status/users/{userId} [POST].
func (s *service) CheckKYCStep4Status( //nolint:gocritic // .
	ctx context.Context,
	req *server.Request[CheckKYCStep4StatusRequestBody, kycquiz.QuizStatus],
) (*server.Response[kycquiz.QuizStatus], *server.Response[server.ErrorResponse]) {
	ctx = kycquiz.ContextWithClientType(ctx, req.Data.XClientType) //nolint:revive // .
	resp, err := s.quizRepository.CheckQuizStatus(ctx, req.AuthenticatedUser.UserID)
	if err != nil {
		return nil, server.Unexpected(errors.Wrapf(err, "failed to CheckQuizStatus for userID:%v", req.AuthenticatedUser.UserID))
	}

	return server.OK(resp), nil
}

// VerifySocialKYCStep godoc
//
//	@Schemes
//	@Description	Verifies if the user has posted the expected verification post on their social media account.
//	@Tags			KYC
//	@Accept			json
//	@Produce		json
//
//	@Param			Authorization		header		string							true	"Insert your access token"		default(Bearer <Add access token here>)
//	@Param			X-Account-Metadata	header		string							false	"Insert your metadata token"	default(<Add metadata token here>)
//	@Param			userId				path		string							true	"ID of the user"
//	@Param			language			query		string							true	"language of the user"
//	@Param			kycStep				query		int								true	"the value of the social kyc step to verify"	Enums(3,5)
//	@Param			social				query		string							true	"the desired social you wish to verify it with"	Enums(facebook,twitter)
//	@Param			request				body		kycsocial.VerificationMetadata	false	"Request params"
//	@Success		200					{object}	kycsocial.Verification
//	@Success		201					{object}	kycsocial.Verification
//	@Failure		400					{object}	server.ErrorResponse	"if validations fail"
//	@Failure		401					{object}	server.ErrorResponse	"if not authorized"
//	@Failure		403					{object}	server.ErrorResponse	"not allowed due to various reasons"
//	@Failure		404					{object}	server.ErrorResponse	"user is not found"
//	@Failure		409					{object}	server.ErrorResponse	"if any conflicts occur or any prerequisites are not met"
//	@Failure		422					{object}	server.ErrorResponse	"if syntax fails"
//	@Failure		500					{object}	server.ErrorResponse
//	@Failure		504					{object}	server.ErrorResponse	"if request times out"
//	@Router			/v1w/kyc/verifySocialKYCStep/users/{userId} [POST].
func (s *service) VerifySocialKYCStep( //nolint:gocritic // .
	ctx context.Context,
	req *server.Request[kycsocial.VerificationMetadata, kycsocial.Verification],
) (*server.Response[kycsocial.Verification], *server.Response[server.ErrorResponse]) {
	if err := validateVerifySocialKYCStep(req); err != nil {
		return nil, server.UnprocessableEntity(errors.Wrapf(err, "validations failed for %#v", req.Data), invalidPropertiesErrorCode)
	}
	result, err := s.socialRepository.VerifyPost(ctx, req.Data)
	if err != nil {
		err = errors.Wrapf(err, "failed to verify post for %#v", req.Data)
		switch {
		case errors.Is(err, users.ErrRelationNotFound):
			return nil, server.NotFound(err, userNotFoundErrorCode)
		case errors.Is(err, users.ErrNotFound):
			return nil, server.NotFound(err, userNotFoundErrorCode)
		case errors.Is(err, kycsocial.ErrDuplicate):
			return nil, server.Conflict(err, socialKYCStepAlreadyCompletedSuccessfullyErrorCode)
		case errors.Is(err, kycsocial.ErrNotAvailable):
			return nil, server.ForbiddenWithCode(err, socialKYCStepNotAvailableErrorCode)
		default:
			return nil, server.Unexpected(err)
		}
	}
	if result.Result == kycsocial.SuccessVerificationResult {
		return server.Created(result), nil
	}

	return server.OK(result), nil
}

func validateVerifySocialKYCStep(req *server.Request[kycsocial.VerificationMetadata, kycsocial.Verification]) error {
	if !slices.Contains(kycsocial.AllSupportedKYCSteps, req.Data.KYCStep) {
		return errors.Errorf("unsupported kycStep `%v`", req.Data.KYCStep)
	}
	if !slices.Contains(kycsocial.AllTypes, req.Data.Social) {
		return errors.Errorf("unsupported social `%v`", req.Data.Social)
	}
	switch req.Data.Social {
	case kycsocial.FacebookType:
		if req.Data.Twitter.TweetURL != "" {
			return errors.Errorf("unsupported twitter.tweetUrl `%v`", req.Data.Twitter.TweetURL)
		}
	case kycsocial.TwitterType:
		if req.Data.Facebook.AccessToken != "" {
			return errors.Errorf("unsupported facebook.accessToken `%v`", req.Data.Facebook.AccessToken)
		}
	case kycsocial.CMCType:
		return errors.Errorf("unsupported social type: %v", kycsocial.CMCType)
	}

	return nil
}

// TryResetKYCSteps godoc
//
//	@Schemes
//	@Description	Checks if there are any kyc steps that should be reset, if so, it resets them and returns the updated latest user state.
//	@Tags			KYC
//	@Accept			json
//	@Produce		json
//
//	@Param			Authorization		header		string	true	"Insert your access token"		default(Bearer <Add access token here>)
//	@Param			X-Account-Metadata	header		string	false	"Insert your metadata token"	default(<Add metadata token here>)
//	@Param			userId				path		string	true	"ID of the user"
//	@Param			skipKYCSteps		query		[]int	false	"the kyc steps you wish to skip"	collectionFormat(multi)
//	@Param			nextKYCStep			query		int		false	"the kyc step which would be next"
//	@Success		200					{object}	User
//	@Failure		400					{object}	server.ErrorResponse	"if validations fail"
//	@Failure		401					{object}	server.ErrorResponse	"if not authorized"
//	@Failure		403					{object}	server.ErrorResponse	"not allowed due to various reasons"
//	@Failure		404					{object}	server.ErrorResponse	"user is not found"
//	@Failure		422					{object}	server.ErrorResponse	"if syntax fails"
//	@Failure		500					{object}	server.ErrorResponse
//	@Failure		504					{object}	server.ErrorResponse	"if request times out"
//	@Router			/v1w/kyc/tryResetKYCSteps/users/{userId} [POST].
func (s *service) TryResetKYCSteps( //nolint:gocritic,funlen,gocognit,revive,cyclop,gocyclo // .
	ctx context.Context,
	req *server.Request[TryResetKYCStepsRequestBody, User],
) (*server.Response[User], *server.Response[server.ErrorResponse]) {
	if req.AuthenticatedUser.Role != "admin" && req.Data.UserID != req.AuthenticatedUser.UserID {
		return nil, server.Forbidden(errors.New("operation not allowed"))
	}
	ctx = users.ContextWithXAccountMetadata(ctx, req.Data.XAccountMetadata) //nolint:revive // .
	ctx = users.ContextWithAuthorization(ctx, req.Data.Authorization)       //nolint:revive // .
	for _, kycStep := range req.Data.SkipKYCSteps {
		switch kycStep { //nolint:exhaustive // .
		case users.Social1KYCStep, users.Social2KYCStep, users.Social3KYCStep, users.Social4KYCStep, users.Social5KYCStep, users.Social6KYCStep, users.Social7KYCStep:
			if err := s.socialRepository.SkipVerification(ctx, kycStep, req.Data.UserID); err != nil {
				if errors.Is(err, kycsocial.ErrNotAvailable) || errors.Is(err, kycsocial.ErrDuplicate) {
					log.Error(errors.Wrapf(err, "skipVerification failed unexpectedly during tryResetKYCSteps for kycStep:%v,userID:%v",
						kycStep, req.Data.UserID))
					err = nil
				}
				if err != nil {
					return nil, server.Unexpected(errors.Wrapf(err, "failed to skip kycStep %v", kycStep))
				}
			}
		case users.QuizKYCStep:
			if err := s.quizRepository.SkipQuizSession(ctx, req.Data.UserID); err != nil {
				if errors.Is(err, kycquiz.ErrInvalidKYCState) || errors.Is(err, kycquiz.ErrNotAvailable) || errors.Is(err, kycquiz.ErrSessionFinished) || errors.Is(err, kycquiz.ErrSessionFinishedWithError) { //nolint:lll // .
					log.Error(errors.Wrapf(err, "skipQuizSession failed unexpectedly during tryResetKYCSteps for userID:%v", req.Data.UserID))
					err = nil
				}
				if err != nil {
					return nil, server.Unexpected(errors.Wrapf(err, "failed to SkipQuizSession for userID:%v", req.Data.UserID))
				}
			}
		}
	}
	quizStatus, err := s.quizRepository.CheckQuizStatus(ctx, req.Data.UserID)
	if err != nil {
		return nil, server.Unexpected(errors.Wrapf(err, "failed to CheckQuizStatus for userID:%v", req.Data.UserID))
	}
	resp, err := s.usersProcessor.TryResetKYCSteps(ctx, s.faceKycClient, req.Data.UserID)
	if err = errors.Wrapf(err, "failed to TryResetKYCSteps for userID:%v", req.Data.UserID); err != nil {
		switch {
		case errors.Is(err, users.ErrNotFound):
			return nil, server.NotFound(err, userNotFoundErrorCode)
		default:
			return nil, server.Unexpected(err)
		}
	}
	kycFaceAvailable := false
	if req.Data.NextKYCStep != nil &&
		(*req.Data.NextKYCStep == users.FacialRecognitionKYCStep || *req.Data.NextKYCStep == users.LivenessDetectionKYCStep) {
		kycFaceAvailable, err = s.faceKycClient.CheckStatus(ctx, resp.User, *req.Data.NextKYCStep)
		if err != nil {
			return nil, server.Unexpected(err)
		}
	}

	return server.OK(&User{UserProfile: resp, QuizStatus: quizStatus, KycFaceAvailable: kycFaceAvailable, Checksum: resp.Checksum()}), nil
}

// ForwardToFaceKYC godoc
//
//	@Schemes
//	@Description	Checks if user already passed face kyc on other tenants and if its available
//	@Tags			KYC
//	@Accept			json
//	@Produce		json
//
//	@Param			Authorization		header		string						true	"Insert your access token"		default(Bearer <Add access token here>)
//	@Param			X-Account-Metadata	header		string						false	"Insert your metadata token"	default(<Add metadata token here>)
//	@Param			userId				path		string						true	"ID of the user"
//	@Param			request				body		ForwardToFaceKYCRequestBody	false	"Request params"
//	@Success		200					{object}	ForwardToFaceKYCResponse
//	@Failure		400					{object}	server.ErrorResponse	"if user dont own remote profile"
//	@Failure		401					{object}	server.ErrorResponse	"if not authorized"
//	@Failure		403					{object}	server.ErrorResponse	"not allowed due to various reasons"
//	@Failure		404					{object}	server.ErrorResponse	"user is not found"
//	@Failure		422					{object}	server.ErrorResponse	"if syntax fails"
//	@Failure		500					{object}	server.ErrorResponse
//	@Failure		504					{object}	server.ErrorResponse	"if request times out"
//	@Router			/v1w/kyc/checkFaceKYCStatus/users/{userId} [POST].
//
//nolint:gocritic // .
func (s *service) ForwardToFaceKYC(
	ctx context.Context,
	req *server.Request[ForwardToFaceKYCRequestBody, ForwardToFaceKYCResponse],
) (*server.Response[ForwardToFaceKYCResponse], *server.Response[server.ErrorResponse]) {
	ctx = facekyc.ContextWithClientType(ctx, req.Data.XClientType) //nolint:revive // .
	kycFaceAvailable, err := s.faceKycClient.ForwardToKYC(ctx, req.Data.UserID)
	if err != nil {
		switch {
		case errors.Is(err, linking.ErrNotOwnRemoteUser):
			return nil, server.BadRequest(err, linkingNotOwnedProfile)
		case errors.Is(err, linking.ErrDuplicate):
			return nil, server.Conflict(err, linkingDuplicate)
		default:
			return nil, server.Unexpected(err)
		}
	}

	return server.OK(&ForwardToFaceKYCResponse{KycFaceAvailable: kycFaceAvailable}), nil
}

// VerifyCoinDistributionEligibility godoc
//
//	@Schemes
//	@Description	Verifies if a user is eligible for coin verificationscenarios.
//	@Tags			KYC
//	@Accept			json
//	@Produce		json
//
//	@Param			Authorization		header		string										true	"Insert your access token"		default(Bearer <Add access token here>)
//	@Param			X-Account-Metadata	header		string										false	"Insert your metadata token"	default(<Add metadata token here>)
//	@Param			userId				path		string										true	"ID of the user"
//	@Param			scenarioEnum		path		string										true	"the scenario"	enums(join_cmc,join_twitter,join_telegram,signup_tenants)
//	@Param			request				body		verificationscenarios.VerificationMetadata	false	"Request params"
//	@Success		200					{object}	any
//	@Failure		400					{object}	server.ErrorResponse	"if validations fail"
//	@Failure		401					{object}	server.ErrorResponse	"if not authorized"
//	@Failure		403					{object}	server.ErrorResponse	"not allowed due to various reasons"
//	@Failure		422					{object}	server.ErrorResponse	"if syntax fails"
//	@Failure		500					{object}	server.ErrorResponse
//	@Router			/v1w/kyc/verifyCoinDistributionEligibility/users/{userId}/scenarios/{scenarioEnum} [POST].
func (s *service) VerifyKYCScenarios( //nolint:gocritic // .
	ctx context.Context,
	req *server.Request[verificationscenarios.VerificationMetadata, verificationscenarios.Verification],
) (*server.Response[verificationscenarios.Verification], *server.Response[server.ErrorResponse]) {
	if err := validateScenariosData(req.Data); err != nil {
		return nil, server.UnprocessableEntity(errors.Wrapf(err, "validations failed for %#v", req.Data), invalidPropertiesErrorCode)
	}
	ctx = users.ContextWithAuthorization(ctx, req.Data.Authorization) //nolint:revive // .
	res, err := s.verificationScenariosRepository.VerifyScenarios(ctx, req.Data)
	if err != nil {
		switch {
		case errors.Is(err, verificationscenarios.ErrVerificationNotPassed):
			return nil, server.BadRequest(err, kycVerificationScenariosVadidationFailedErrorCode)
		case errors.Is(err, users.ErrRelationNotFound):
			return nil, server.NotFound(err, userNotFoundErrorCode)
		case errors.Is(err, users.ErrNotFound):
			return nil, server.NotFound(err, userNotFoundErrorCode)
		case errors.Is(err, linking.ErrNotOwnRemoteUser):
			return nil, server.BadRequest(err, linkingNotOwnedProfile)
		case errors.Is(err, verificationscenarios.ErrNoPendingScenarios):
			return nil, server.NotFound(err, kycVerificationScenariosNoPendingScenariosErrorCode)
		case errors.Is(err, verificationscenarios.ErrWrongTenantTokens):
			return nil, server.BadRequest(err, kycVerificationScenariosNoTenantTokens)
		default:
			return nil, server.Unexpected(err)
		}
	}

	return server.OK[verificationscenarios.Verification](res), nil
}

//nolint:funlen,gocognit,revive // .
func validateScenariosData(data *verificationscenarios.VerificationMetadata) error {
	switch data.ScenarioEnum {
	case verificationscenarios.CoinDistributionScenarioCmc:
		if data.CMCProfileLink == "" || !strings.HasPrefix(data.CMCProfileLink, "https://coinmarketcap.com") {
			return errors.Errorf("wrong cmc profile link `%v`", data.CMCProfileLink)
		}
	case verificationscenarios.CoinDistributionScenarioTwitter:
		if data.Language == "" {
			return errors.Errorf("empty language `%v`", data.Language)
		}
	case verificationscenarios.CoinDistributionScenarioTelegram:
		if data.TelegramUsername == "" {
			return errors.Errorf("empty telegram username `%v`", data.TelegramUsername)
		}
	case verificationscenarios.CoinDistributionScenarioSignUpTenants:
		if len(data.TenantTokens) == 0 {
			return errors.Errorf("empty tenant tokens `%v`", data.TenantTokens)
		}
		var (
			supportedTenants = []verificationscenarios.TenantScenario{
				verificationscenarios.CoinDistributionScenarioSignUpSunwaves,
				verificationscenarios.CoinDistributionScenarioSignUpCallfluent,
				verificationscenarios.CoinDistributionScenarioSignUpSealsend,
				verificationscenarios.CoinDistributionScenarioSignUpSauces,
				verificationscenarios.CoinDistributionScenarioSignUpDoctorx,
				verificationscenarios.CoinDistributionScenarioSignUpCryptomayors,
			}
			unsupportedTenants []verificationscenarios.TenantScenario
		)
		for tenant, token := range data.TenantTokens {
			if token == "" { //nolint:gosec // .
				return errors.Errorf("empty token for tenant `%v`", tenant)
			}
			if !slices.Contains(supportedTenants, tenant) {
				unsupportedTenants = append(unsupportedTenants, tenant)
			}
		}
		if len(unsupportedTenants) > 0 {
			return errors.Errorf("unsupported tenants `%v`", unsupportedTenants)
		}
	default:
		return errors.Errorf("unsupported scenario `%v`", data.ScenarioEnum)
	}

	return nil
}
