// SPDX-License-Identifier: ice License 1.0

package threedivi

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	stdlibtime "time"

	"github.com/goccy/go-json"
	"github.com/imroc/req/v3"
	"github.com/pkg/errors"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/prometheus/promql/parser"

	"github.com/ice-blockchain/eskimo/kyc/face/internal"
	"github.com/ice-blockchain/eskimo/users"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/time"
)

func init() { //nolint:gochecknoinits // It's the only way to tweak the client.
	req.DefaultClient().SetJsonMarshal(json.Marshal)
	req.DefaultClient().SetJsonUnmarshal(json.Unmarshal)
	req.DefaultClient().GetClient().Timeout = requestDeadline
}

func New3Divi(ctx context.Context, usersRepository internal.UserRepository, cfg *Config) internal.Client {
	if cfg.ThreeDiVi.BAFHost == "" {
		log.Panic(errors.Errorf("no baf-host for 3divi integration"))
	}
	if cfg.ThreeDiVi.BAFToken == "" {
		log.Panic(errors.Errorf("no baf-token for 3divi integration"))
	}
	if cfg.ThreeDiVi.ConcurrentUsers == 0 {
		log.Panic(errors.Errorf("concurrent users is zero for 3divi integration"))
	}
	cfg.ThreeDiVi.BAFHost, _ = strings.CutSuffix(cfg.ThreeDiVi.BAFHost, "/")

	tdv := &threeDivi{
		users: usersRepository,
		cfg:   cfg,
	}
	go tdv.clearUsers(ctx)
	reqCtx, reqCancel := context.WithTimeout(ctx, requestDeadline)
	log.Error(errors.Wrapf(tdv.updateAvailability(reqCtx), "failed to update face availability on startup"))
	reqCancel()
	go tdv.startAvailabilitySyncer(ctx)

	return tdv
}

func (t *threeDivi) startAvailabilitySyncer(ctx context.Context) {
	ticker := stdlibtime.NewTicker(100 * stdlibtime.Millisecond) //nolint:gomnd // .
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			reqCtx, cancel := context.WithTimeout(ctx, requestDeadline)
			log.Error(errors.Wrap(t.updateAvailability(reqCtx), "failed to updateAvailability"))
			cancel()
		case <-ctx.Done():
			return
		}
	}
}

//nolint:funlen // .
func (t *threeDivi) updateAvailability(ctx context.Context) error {
	if t.cfg.ThreeDiVi.AvailabilityURL == "" {
		return nil
	}
	if resp, err := req.
		SetContext(ctx).
		SetRetryCount(10).                                                       //nolint:gomnd // .
		SetRetryBackoffInterval(10*stdlibtime.Millisecond, 1*stdlibtime.Second). //nolint:gomnd // .
		SetRetryHook(func(resp *req.Response, err error) {
			if err != nil {
				log.Error(errors.Wrap(err, "failed to check availability of face auth, retrying... "))
			} else {
				body, bErr := resp.ToString()
				log.Error(errors.Wrapf(bErr, "failed to parse negative response body for check availability of face auth"))
				log.Error(errors.Errorf("failed check availability of face auth with status code:%v, body:%v, retrying... ", resp.GetStatusCode(), body))
			}
		}).
		SetRetryCondition(func(resp *req.Response, err error) bool {
			return err != nil || (resp.GetStatusCode() != http.StatusOK)
		}).
		AddQueryParam("caller", "eskimo-hut").
		Get(t.cfg.ThreeDiVi.AvailabilityURL); err != nil {
		return errors.Wrap(err, "failed to check availability of face auth")
	} else if statusCode := resp.GetStatusCode(); statusCode != http.StatusOK {
		return errors.Errorf("[%v]failed to check availability of face auth", statusCode)
	} else if data, err2 := resp.ToBytes(); err2 != nil {
		return errors.Wrapf(err2, "failed to read body of availability of face auth")
	} else { //nolint:revive // .
		activeUsers, cErr := t.activeUsers(data)
		if cErr != nil {
			return errors.Wrapf(cErr, "failed to parse metrics of availability of face auth")
		}
		t.activeUsersCount.Store(uint64(activeUsers))
	}

	return nil
}

func (t *threeDivi) Available(_ context.Context, userWasPreviouslyForwardedToFaceKYC bool) error {
	return t.isAvailable(userWasPreviouslyForwardedToFaceKYC)
}

//nolint:revive // .
func (t *threeDivi) isAvailable(userWasPreviouslyForwardedToFaceKYC bool) error {
	if int64(t.cfg.ThreeDiVi.ConcurrentUsers)-(int64(t.activeUsersCount.Load())+int64(t.loadBalancedUsersCount.Load())) >= 1 {
		if !userWasPreviouslyForwardedToFaceKYC {
			t.loadBalancedUsersCount.Add(1)
		}

		return nil
	}

	return internal.ErrNotAvailable
}

func (t *threeDivi) clearUsers(ctx context.Context) {
	ticker := stdlibtime.NewTicker(1 * stdlibtime.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.loadBalancedUsersCount.Store(0)
		case <-ctx.Done():
			return
		}
	}
}

func (*threeDivi) activeUsers(data []byte) (int, error) {
	p := parser.NewParser(string(data))
	defer p.Close()
	var expparser expfmt.TextParser
	metricFamilies, err := expparser.TextToMetricFamilies(bytes.NewReader(data))
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse metrics for availability of face auth")
	}
	openConns := 0
	if connsMetric, hasConns := metricFamilies[metricOpenConnections]; hasConns {
		for _, metric := range connsMetric.GetMetric() {
			labelMatch := false
			for _, l := range metric.GetLabel() {
				if l.GetValue() == metricOpenConnectionsLabelTCP {
					labelMatch = true
				}
			}
			if labelMatch && metric.GetGauge() != nil {
				openConns = int(metric.GetGauge().GetValue())
			}
		}
	}

	return openConns / connsPerUser, nil
}

func (t *threeDivi) CheckAndUpdateStatus(ctx context.Context, user *users.User) (hasFaceKYCResult bool, originalAccount string, err error) {
	bafApplicant, err := t.searchIn3DiviForApplicant(ctx, user.ID)
	if err != nil && !errors.Is(err, errFaceAuthNotStarted) {
		return false, "", errors.Wrapf(err, "failed to sync face auth status from 3divi BAF")
	}
	var originalApplicant *applicant
	if bafApplicant != nil && bafApplicant.LastValidationResponse != nil && bafApplicant.HasRiskEvents {
		if originalApplicant, err = t.findOriginalApplicant(ctx, user, bafApplicant); err != nil {
			return false, "", errors.Wrapf(err, "failed to find original applicant for duplicate userID %v", user.ID)
		}
	}
	usr, originalEmail, originalTenant := t.parseApplicant(user, bafApplicant, originalApplicant)
	hasFaceKYCResult = (usr.KYCStepPassed != nil && *usr.KYCStepPassed >= users.LivenessDetectionKYCStep) ||
		(usr.KYCStepBlocked != nil && *usr.KYCStepBlocked > users.NoneKYCStep)
	_, mErr := t.users.ModifyUser(ctx, usr, nil)
	if originalEmail != "" && originalTenant != "" {
		originalAccount = fmt.Sprintf("%v:%v", originalTenant, originalEmail)
	}

	return hasFaceKYCResult, originalAccount, errors.Wrapf(mErr, "failed to update user with face kyc result")
}

func (t *threeDivi) findOriginalApplicant(ctx context.Context, user *users.User, bafApplicant *applicant) (originalApplicant *applicant, err error) {
	validationResp, isDuplFace, verifErr := t.fetchVerificationAttempt(ctx, user.ID, bafApplicant.ApplicantID, bafApplicant.LastValidationResponse.ValidationResponseID) //nolint:lll // .
	if verifErr != nil {
		return nil, errors.Wrapf(verifErr,
			"failed to sync face auth status from 3divi BAF on sync validationResponse %v for userID %v",
			bafApplicant.LastValidationResponse.ValidationResponseID, user.ID)
	}
	if isDuplFace && validationResp.SimilarApplicants != nil && len(validationResp.SimilarApplicants.IDs) > 0 {
		similarApplicant := validationResp.SimilarApplicants.IDs[0]
		if originalApplicant, err = t.getApplicantByID(ctx, similarApplicant); err != nil {
			return nil, errors.Wrapf(err,
				"failed to sync face auth status from 3divi BAF on getting most similar applicant %v for userID %v", similarApplicant, user.ID)
		}
	}

	return originalApplicant, nil
}

//nolint:funlen,revive // .
func (t *threeDivi) Reset(ctx context.Context, user *users.User, fetchState bool) error {
	bafApplicant, err := t.searchIn3DiviForApplicant(ctx, user.ID)
	if err != nil {
		if errors.Is(err, errFaceAuthNotStarted) {
			return nil
		}

		return errors.Wrapf(err, "failed to find matching applicant for userID %v", user.ID)
	}
	var resp *req.Response
	if resp, err = req.
		SetContext(ctx).
		SetRetryCount(10).                                                       //nolint:gomnd // .
		SetRetryBackoffInterval(10*stdlibtime.Millisecond, 1*stdlibtime.Second). //nolint:gomnd // .
		SetRetryHook(func(resp *req.Response, err error) {
			if err != nil {
				log.Error(errors.Wrap(err, "failed to delete face auth state for user, retrying... "))
			} else {
				body, bErr := resp.ToString()
				log.Error(errors.Wrapf(bErr, "failed to parse negative response body for delete face auth state for user"))
				log.Error(errors.Errorf("failed to delete face auth state for user with status code:%v, body:%v, retrying... ", resp.GetStatusCode(), body))
			}
		}).
		SetRetryCondition(func(resp *req.Response, err error) bool {
			return err != nil || (resp.GetStatusCode() != http.StatusOK && resp.GetStatusCode() != http.StatusNoContent)
		}).
		AddQueryParam("caller", "eskimo-hut").
		SetHeader("Authorization", fmt.Sprintf("Bearer %v", t.cfg.ThreeDiVi.BAFToken)).
		SetHeader("X-Secret-Api-Token", t.cfg.ThreeDiVi.SecretAPIToken).
		Delete(fmt.Sprintf("%v/publicapi/api/v2/private/Applicants/%v", t.cfg.ThreeDiVi.BAFHost, bafApplicant.ApplicantID)); err != nil {
		return errors.Wrapf(err, "failed to delete face auth state for userID:%v", user.ID)
	} else if statusCode := resp.GetStatusCode(); statusCode != http.StatusOK && statusCode != http.StatusNoContent {
		return errors.Errorf("[%v]failed to delete face auth state for userID:%v", statusCode, user.ID)
	} else if _, err2 := resp.ToBytes(); err2 != nil {
		return errors.Wrapf(err2, "failed to read body of delete face auth state request for userID:%v", user.ID)
	} else { //nolint:revive // .
		if fetchState {
			_, _, err = t.CheckAndUpdateStatus(ctx, user)

			return errors.Wrapf(err, "failed to check user's face auth state after reset for userID %v", user.ID)
		}

		return nil
	}
}

//nolint:funlen // .
func (t *threeDivi) UpdateEmail(ctx context.Context, userID, newEmail string) error {
	bafApplicant, err := t.searchIn3DiviForApplicant(ctx, userID)
	if err != nil {
		if errors.Is(err, errFaceAuthNotStarted) {
			return nil
		}

		return errors.Wrapf(err, "failed to find matching applicant for userID %v", userID)
	}
	var resp *req.Response
	if resp, err = req.
		SetContext(ctx).
		SetRetryCount(10).                                                       //nolint:gomnd // .
		SetRetryBackoffInterval(10*stdlibtime.Millisecond, 1*stdlibtime.Second). //nolint:gomnd // .
		SetRetryHook(func(resp *req.Response, err error) {
			if err != nil {
				log.Error(errors.Wrap(err, "failed to modify email in face kyc for user, retrying... "))
			} else {
				body, bErr := resp.ToString()
				log.Error(errors.Wrapf(bErr, "failed to parse negative response body for modify email in face kyc"))
				log.Error(errors.Errorf("failed to modify email in face kyc for user with status code:%v, body:%v, retrying... ", resp.GetStatusCode(), body))
			}
		}).
		SetRetryCondition(func(resp *req.Response, err error) bool {
			return err != nil || (resp.GetStatusCode() != http.StatusOK)
		}).
		AddQueryParam("caller", "eskimo-hut").
		SetHeader("Authorization", fmt.Sprintf("Bearer %v", t.cfg.ThreeDiVi.BAFToken)).
		SetHeader("X-Secret-Api-Token", t.cfg.ThreeDiVi.SecretAPIToken).
		SetBodyJsonMarshal(struct {
			Email string `json:"email"`
		}{Email: newEmail}).
		Put(fmt.Sprintf("%v/publicapi/api/v2/private/Applicants/%v", t.cfg.ThreeDiVi.BAFHost, bafApplicant.ApplicantID)); err != nil {
		return errors.Wrapf(err, "failed to modify email in face kyc for userID:%v", userID)
	} else if statusCode := resp.GetStatusCode(); statusCode != http.StatusOK {
		return errors.Errorf("[%v]failed to modify email in face kyc for userID:%v", statusCode, userID)
	} else if _, err2 := resp.ToBytes(); err2 != nil {
		return errors.Wrapf(err2, "failed to read body of modify email in face kyc request for userID:%v", userID)
	} else { //nolint:revive // .
		return nil
	}
}

//nolint:funlen,gocyclo,revive,cyclop //.
func (*threeDivi) parseApplicant(user *users.User, bafApplicant, originalApplicant *applicant) (*users.User, string, string) {
	updUser := new(users.User)
	updUser.ID = user.ID
	if bafApplicant != nil && bafApplicant.LastValidationResponse != nil && bafApplicant.Status == statusPassed {
		updUser = bafApplicant.passed(updUser, user)
	} else if user.KYCStepsCreatedAt != nil && len(*user.KYCStepsCreatedAt) >= int(users.LivenessDetectionKYCStep) {
		updUser.KYCStepsCreatedAt = user.KYCStepsCreatedAt
		updUser.KYCStepsLastUpdatedAt = user.KYCStepsLastUpdatedAt
		if user.KYCStepPassed != nil && (*user.KYCStepPassed == users.LivenessDetectionKYCStep || *user.KYCStepPassed == users.FacialRecognitionKYCStep) {
			stepPassed := users.NoneKYCStep
			updUser.KYCStepPassed = &stepPassed
		}
		(*updUser.KYCStepsCreatedAt)[stepIdx(users.FacialRecognitionKYCStep)] = nil
		(*updUser.KYCStepsCreatedAt)[stepIdx(users.LivenessDetectionKYCStep)] = nil
		(*updUser.KYCStepsLastUpdatedAt)[stepIdx(users.FacialRecognitionKYCStep)] = nil
		(*updUser.KYCStepsLastUpdatedAt)[stepIdx(users.LivenessDetectionKYCStep)] = nil
	}
	var emailFromAnotherTenant, originalTenant string
	switch {
	case bafApplicant != nil && bafApplicant.LastValidationResponse != nil && bafApplicant.Status == statusFailed && bafApplicant.HasRiskEvents:
		var block bool
		block, emailFromAnotherTenant, originalTenant = bafApplicant.isDuplicateFromDifferentTenant(originalApplicant, user.Email)
		switch {
		case block && emailFromAnotherTenant == "":
			kycStepBlocked := users.FacialRecognitionKYCStep
			updUser.KYCStepBlocked = &kycStepBlocked
		case !block:
			updUser = originalApplicant.passed(updUser, user)
		}

	default:
		kycStepBlocked := users.NoneKYCStep
		updUser.KYCStepBlocked = &kycStepBlocked
	}
	user.KYCStepsLastUpdatedAt = updUser.KYCStepsLastUpdatedAt
	user.KYCStepsCreatedAt = updUser.KYCStepsCreatedAt
	if updUser.KYCStepPassed != nil {
		user.KYCStepPassed = updUser.KYCStepPassed
	}
	if updUser.KYCStepBlocked != nil {
		user.KYCStepBlocked = updUser.KYCStepBlocked
	}

	return updUser, emailFromAnotherTenant, originalTenant
}

func (bafApplicant *applicant) isDuplicateFromDifferentTenant(original *applicant, userEmail string) (block bool, email, tenant string) {
	if original == nil {
		return true, "", ""
	}
	if original.Email != userEmail {
		if !(bafApplicant.Metadata != nil && original.Metadata != nil) {
			return true, "", ""
		}
		if original.Metadata.Tenant == bafApplicant.Metadata.Tenant {
			return true, "", ""
		}

		return true, original.Email, original.Metadata.Tenant
	}

	return false, "", ""
}

func (bafApplicant *applicant) passed(updUser, user *users.User) *users.User {
	passedTime := time.New(bafApplicant.LastValidationResponse.CreatedAt)
	if user.KYCStepsCreatedAt != nil && len(*user.KYCStepsCreatedAt) >= int(users.LivenessDetectionKYCStep) {
		updUser.KYCStepsCreatedAt = user.KYCStepsCreatedAt
		updUser.KYCStepsLastUpdatedAt = user.KYCStepsLastUpdatedAt
		if user.KYCStepPassed == nil || *user.KYCStepPassed == 0 {
			stepPassed := users.LivenessDetectionKYCStep
			updUser.KYCStepPassed = &stepPassed
		}
		(*updUser.KYCStepsCreatedAt)[stepIdx(users.FacialRecognitionKYCStep)] = passedTime
		(*updUser.KYCStepsCreatedAt)[stepIdx(users.LivenessDetectionKYCStep)] = passedTime
		(*updUser.KYCStepsLastUpdatedAt)[stepIdx(users.FacialRecognitionKYCStep)] = passedTime
		(*updUser.KYCStepsLastUpdatedAt)[stepIdx(users.LivenessDetectionKYCStep)] = passedTime
	} else {
		times := []*time.Time{passedTime, passedTime}
		updUser.KYCStepsLastUpdatedAt = &times
		stepPassed := users.LivenessDetectionKYCStep
		updUser.KYCStepPassed = &stepPassed
	}

	return updUser
}

func stepIdx(step users.KYCStep) int {
	return int(step) - 1
}

func (t *threeDivi) searchIn3DiviForApplicant(ctx context.Context, userID users.UserID) (*applicant, error) {
	bafApplicant, err := t.fetchApplicant(ctx, fmt.Sprintf("%v/publicapi/api/v2/private/Applicants/ByReferenceId/%v", t.cfg.ThreeDiVi.BAFHost, userID))

	return bafApplicant, errors.Wrapf(err, "failed to match applicant in BAF by userID %v", userID)
}

func (t *threeDivi) getApplicantByID(ctx context.Context, applicantID string) (*applicant, error) {
	bafApplicant, err := t.fetchApplicant(ctx, fmt.Sprintf("%v/publicapi/api/v2/private/Applicants/%v", t.cfg.ThreeDiVi.BAFHost, applicantID))

	return bafApplicant, errors.Wrapf(err, "failed to fetch applicant %v by id", applicantID)
}

func (t *threeDivi) fetchApplicant(ctx context.Context, url string) (*applicant, error) {
	if resp, err := req.
		SetContext(ctx).
		SetRetryCount(10).                                                       //nolint:gomnd // .
		SetRetryBackoffInterval(10*stdlibtime.Millisecond, 1*stdlibtime.Second). //nolint:gomnd // .
		SetRetryHook(func(resp *req.Response, err error) {
			if err != nil {
				log.Error(errors.Wrap(err, "failed to match applicantId for user, retrying... "))
			} else {
				body, bErr := resp.ToString()
				log.Error(errors.Wrapf(bErr, "failed to parse negative response body for match applicantId for user"))
				log.Error(errors.Errorf("failed to match applicantId for user with status code:%v, body:%v, retrying... ", resp.GetStatusCode(), body))
			}
		}).
		SetRetryCondition(func(resp *req.Response, err error) bool {
			return err != nil || (resp.GetStatusCode() != http.StatusOK && resp.GetStatusCode() != http.StatusNotFound)
		}).
		SetHeader("Authorization", fmt.Sprintf("Bearer %v", t.cfg.ThreeDiVi.BAFToken)).
		Get(url); err != nil {
		return nil, errors.Wrapf(err, "failed to fetch applicant for url:%v", url)
	} else if statusCode := resp.GetStatusCode(); statusCode != http.StatusOK && statusCode != http.StatusNotFound {
		return nil, errors.Errorf("[%v]failed tofetch applicant for url:%v", statusCode, url)
	} else if data, err2 := resp.ToBytes(); err2 != nil {
		return nil, errors.Wrapf(err2, "failed to fetch applicant for url:%v", url)
	} else { //nolint:revive // .
		return t.extractApplicant(data)
	}
}

func (*threeDivi) extractApplicant(data []byte) (*applicant, error) {
	var bafApplicant applicant
	if jErr := json.Unmarshal(data, &bafApplicant); jErr != nil {
		return nil, errors.Wrapf(jErr, "failed to decode %v into applicants page", string(data))
	}
	if bafApplicant.Code == codeApplicantNotFound {
		return nil, errFaceAuthNotStarted
	}
	if bafApplicant.LastValidationResponse != nil {
		var err error
		if bafApplicant.LastValidationResponse.CreatedAt, err = stdlibtime.Parse(bafTimeFormat, bafApplicant.LastValidationResponse.Created); err != nil {
			return nil, errors.Wrapf(err, "failed to parse time at %v", bafApplicant.LastValidationResponse.Created)
		}
	}

	return &bafApplicant, nil
}

func (t *threeDivi) fetchVerificationAttempt(ctx context.Context, userID, applicantID string, attemptID uint64) (*validationResponse, bool, error) {
	if resp, err := req.
		SetContext(ctx).
		SetRetryCount(10).                                                       //nolint:gomnd // .
		SetRetryBackoffInterval(10*stdlibtime.Millisecond, 1*stdlibtime.Second). //nolint:gomnd // .
		SetRetryHook(func(resp *req.Response, err error) {
			if err != nil {
				log.Error(errors.Wrap(err, "failed to fetch verificationAttempt, retrying... "))
			} else {
				body, bErr := resp.ToString()
				log.Error(errors.Wrapf(bErr, "failed to parse negative response body for fetching verificationAttempt"))
				log.Error(errors.Errorf("failed to fetch verificationAttempt:%v, body:%v, retrying... ", resp.GetStatusCode(), body))
			}
		}).
		SetRetryCondition(func(resp *req.Response, err error) bool {
			return err != nil || (resp.GetStatusCode() != http.StatusOK)
		}).
		SetHeader("Authorization", fmt.Sprintf("Bearer %v", t.cfg.ThreeDiVi.BAFToken)).
		Get(fmt.Sprintf("%v/publicapi/api/v2/private/Applicants/%v/Attempts/%v", t.cfg.ThreeDiVi.BAFHost, applicantID, attemptID)); err != nil {
		return nil, false, errors.Wrapf(err, "failed to fetch verificaionAttempt %v for applicant %v, userID %v", attemptID, applicantID, userID)
	} else if statusCode := resp.GetStatusCode(); statusCode != http.StatusOK {
		return nil, false, errors.Errorf("[%v]failed to fetch verification attempt %v for applicant %v, userID %v", statusCode, attemptID, applicantID, userID)
	} else if data, err2 := resp.ToBytes(); err2 != nil {
		return nil, false, errors.Wrapf(err2, "failed to read body of verification attempt %v for applicant %v, userID %v", attemptID, applicantID, userID)
	} else { //nolint:revive // .
		return t.parseValidationResponse(data)
	}
}

func (*threeDivi) parseValidationResponse(data []byte) (*validationResponse, bool, error) {
	var vr validationResponse
	if jErr := json.Unmarshal(data, &vr); jErr != nil {
		return nil, false, errors.Wrapf(jErr, "failed to decode %v into validationResponse", string(data))
	}
	var err error
	if vr.CreatedAt, err = stdlibtime.Parse(bafTimeFormat, vr.Created); err != nil {
		return nil, false, errors.Wrapf(err, "failed to parse time at %v", vr.Created)
	}
	isDuplicatedFace := false
	if vr.RiskEvents != nil {
		for _, r := range *vr.RiskEvents {
			if r.IsActive && r.RiskName == duplicatedFaceRisk {
				isDuplicatedFace = true

				break
			}
		}
	}

	return &vr, isDuplicatedFace, nil
}
