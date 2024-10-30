// SPDX-License-Identifier: ice License 1.0

package verificationscenarios

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"text/template"
	stdlibtime "time"

	"github.com/goccy/go-json"
	"github.com/hashicorp/go-multierror"
	"github.com/imroc/req/v3"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/kyc/linking"
	"github.com/ice-blockchain/eskimo/kyc/scraper"
	"github.com/ice-blockchain/eskimo/kyc/social"
	"github.com/ice-blockchain/eskimo/users"
	"github.com/ice-blockchain/santa/tasks"
	appcfg "github.com/ice-blockchain/wintr/config"
	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/time"
)

func New(ctx context.Context, usrRepo UserRepository, host string) Repository {
	var cfg config
	appcfg.MustLoadFromKey(applicationYamlKey, &cfg)
	repo := &repository{
		userRepo:        usrRepo,
		cfg:             &cfg,
		host:            host,
		globalDB:        storage.MustConnect(ctx, "", globalApplicationYamlKey),
		db:              storage.MustConnect(ctx, ddl, applicationYamlKey),
		twitterVerifier: scraper.New(scraper.StrategyTwitter),
	}
	go repo.startKYCConfigJSONSyncer(ctx)

	return repo
}

func (r *repository) Close() error {
	return errors.Wrap(multierror.Append(nil,
		errors.Wrap(r.globalDB.Close(), "closing db connection failed"),
		errors.Wrap(r.db.Close(), "closing db connection failed"),
	).ErrorOrNil(), "some of close functions failed")
}

//nolint:funlen,gocognit,gocyclo,revive,cyclop // .
func (r *repository) VerifyScenarios(ctx context.Context, metadata *VerificationMetadata) (*social.Verification, error) {
	userIDScenarioMap := make(map[TenantScenario]users.UserID, 0)
	usr, err := r.userRepo.GetUserByID(ctx, metadata.UserID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get user by id: %v", metadata.UserID)
	}
	completedSantaTasks, err := r.getCompletedSantaTasks(ctx, usr.ID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to getCompletedSantaTasks for userID: %v", usr.ID)
	}
	pendingScenarios := r.getPendingScenarios(usr.User, completedSantaTasks)
	if len(pendingScenarios) == 0 || !isScenarioPending(pendingScenarios, string(metadata.ScenarioEnum)) {
		return nil, errors.Wrapf(ErrNoPendingScenarios, "no pending scenarios for user: %v", metadata.UserID)
	}
	switch metadata.ScenarioEnum {
	case CoinDistributionScenarioCmc:
		if false {
			return nil, errors.Wrapf(ErrVerificationNotPassed, "haven't passed the CMC verification for userID:%v", metadata.UserID)
		}
	case CoinDistributionScenarioTwitter:
		verification, sErr := r.VerifyTwitterPost(ctx, metadata)
		if sErr != nil {
			return verification, errors.Wrapf(sErr, "failed to call VerifyPostForDistibutionVerification for userID:%v", metadata.UserID)
		}
		if verification.Result != social.SuccessVerificationResult {
			return verification, nil
		}
	case CoinDistributionScenarioTelegram:
	case CoinDistributionScenarioSignUpTenants:
		skippedTokenCount := 0
		for tenantScenario, token := range metadata.TenantTokens {
			if !isScenarioPending(pendingScenarios, string(tenantScenario)) {
				skippedTokenCount++

				continue
			}
			splitted := strings.Split(string(tenantScenario), "_")
			tenantUsr, fErr := linking.FetchTokenData(ctx, splitted[1], string(token), r.host, r.cfg.TenantURLs)
			if fErr != nil {
				if errors.Is(fErr, linking.ErrRemoteUserNotFound) {
					return nil, errors.Wrapf(linking.ErrNotOwnRemoteUser, "foreign token of userID:%v for the tenant: %v", metadata.UserID, tenantScenario)
				}

				return nil, errors.Wrapf(fErr, "failed to fetch remote user data for %v", metadata.UserID)
			}
			if tenantUsr.CreatedAt == nil || tenantUsr.ReferredBy == "" || tenantUsr.Username == "" {
				return nil, errors.Wrapf(linking.ErrNotOwnRemoteUser, "foreign token of userID:%v for the tenant: %v", metadata.UserID, tenantScenario)
			}
			userIDScenarioMap[tenantScenario] = tenantUsr.ID
		}
		if skippedTokenCount == len(metadata.TenantTokens) {
			return nil, errors.Wrapf(ErrWrongTenantTokens, "all passed tenant tokens don't wait for verification for userID:%v", metadata.UserID)
		}
		if sErr := r.storeLinkedAccounts(ctx, usr.ID, userIDScenarioMap); sErr != nil {
			return nil, errors.Wrap(sErr, "failed to store linked accounts")
		}
	}

	return nil, errors.Wrapf(r.setCompletedDistributionScenario(ctx, usr.User, metadata.ScenarioEnum, userIDScenarioMap),
		"failed to setCompletedDistributionScenario for userID:%v", metadata.UserID)
}

func (r *repository) GetPendingVerificationScenarios(ctx context.Context, userID string) ([]*Scenario, error) {
	usr, err := r.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get user by id: %v", userID)
	}
	completedSantaTasks, err := r.getCompletedSantaTasks(ctx, usr.ID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to getCompletedSantaTasks for userID: %v", usr.ID)
	}

	return r.getPendingScenarios(usr.User, completedSantaTasks), nil
}

//nolint:funlen,gocognit,gocyclo,revive,cyclop // .
func (r *repository) getPendingScenarios(usr *users.User, completedSantaTasks []*tasks.Task) []*Scenario {
	var (
		joinBulllishCMCTaskCompleted, joinIONCMCTaskCompleted, joinWatchlistCMCTaskCompleted, cmcScenarioCompleted = false, false, false, false
		joinTwitterTaskCompleted, twitterScenarioCompleted                                                         = false, false
		joinTelegramTaskCompleted, telegramScenarioCompleted                                                       = false, false
		tenantsScenariosCompleted                                                                                  = map[TenantScenario]bool{
			CoinDistributionScenarioSignUpSunwaves:   false,
			CoinDistributionScenarioSignUpSealsend:   false,
			CoinDistributionScenarioSignUpCallfluent: false,
			CoinDistributionScenarioSignUpSauces:     false,
			CoinDistributionScenarioSignUpDoctorx:    false,
		}
	)
	const singUpPrefix = "signup"
	if usr.DistributionScenariosCompleted != nil {
		for _, completedScenario := range *usr.DistributionScenariosCompleted {
			switch completedScenario {
			case string(CoinDistributionScenarioCmc):
				cmcScenarioCompleted = true
			case string(CoinDistributionScenarioTwitter):
				twitterScenarioCompleted = true
			case string(CoinDistributionScenarioTelegram):
				telegramScenarioCompleted = true
			default:
				splitted := strings.Split(completedScenario, "_")
				if splitted[0] == singUpPrefix {
					tenantsScenariosCompleted[TenantScenario(completedScenario)] = true
				}
			}
		}
	}
	scenarios := make([]*Scenario, 0)
	for _, task := range completedSantaTasks {
		switch task.Type { //nolint:exhaustive // We handle only tasks related to distribution verification.
		case tasks.JoinBullishCMCType:
			joinBulllishCMCTaskCompleted = true
		case tasks.JoinIONCMCType:
			joinIONCMCTaskCompleted = true
		case tasks.JoinWatchListCMCType:
			joinWatchlistCMCTaskCompleted = true
		case tasks.JoinTwitterType:
			joinTwitterTaskCompleted = true
		case tasks.JoinTelegramType:
			joinTelegramTaskCompleted = true
		default:
			if splitted := strings.Split(string(task.Type), "_"); len(splitted) > 1 && splitted[0] == singUpPrefix {
				if completed, ok := tenantsScenariosCompleted[TenantScenario(task.Type)]; !ok || !completed {
					scenario := Scenario(task.Type)
					scenarios = append(scenarios, &scenario)
				}
			}
		}
	}
	if joinBulllishCMCTaskCompleted && joinIONCMCTaskCompleted && joinWatchlistCMCTaskCompleted && !cmcScenarioCompleted {
		val := CoinDistributionScenarioCmc
		scenarios = append(scenarios, &val)
	}
	if joinTwitterTaskCompleted && !twitterScenarioCompleted {
		val := CoinDistributionScenarioTwitter
		scenarios = append(scenarios, &val)
	}
	if joinTelegramTaskCompleted && !telegramScenarioCompleted {
		val := CoinDistributionScenarioTelegram
		scenarios = append(scenarios, &val)
	}

	return scenarios
}

func isScenarioPending(pendingScenarios []*Scenario, scenario string) bool {
	if scenario == string(CoinDistributionScenarioSignUpTenants) {
		return true
	}
	for _, pending := range pendingScenarios {
		if string(*pending) == scenario {
			return true
		}
	}

	return false
}

func (r *repository) setCompletedDistributionScenario(
	ctx context.Context, usr *users.User, scenario Scenario, userIDMap map[TenantScenario]users.UserID,
) error {
	var lenScenarios int
	if usr.DistributionScenariosCompleted != nil {
		lenScenarios = len(*usr.DistributionScenariosCompleted)
	}
	scenarios := make(users.Enum[string], 0, lenScenarios+1)
	if usr.DistributionScenariosCompleted != nil {
		scenarios = append(scenarios, *usr.DistributionScenariosCompleted...)
	}
	if scenario != CoinDistributionScenarioSignUpTenants {
		scenarios = append(scenarios, string(scenario))
	} else {
		for tenant := range userIDMap {
			scenarios = append(scenarios, string(tenant))
		}
	}
	updUsr := new(users.User)
	updUsr.ID = usr.ID
	updUsr.DistributionScenariosCompleted = &scenarios
	_, err := r.userRepo.ModifyUser(ctx, updUsr, nil)

	return errors.Wrapf(err, "failed to set completed distribution scenarios:%v", scenarios)
}

//nolint:funlen // .
func (r *repository) getCompletedSantaTasks(ctx context.Context, userID string) (res []*tasks.Task, err error) {
	getCompletedTasksURL, err := buildGetCompletedTasksURL(r.cfg.Tenant, userID, r.host, r.cfg.TenantURLs)
	if err != nil {
		log.Panic(errors.Wrapf(err, "failed to detect completed santa task url"))
	}
	resp, err := req. //nolint:dupl // .
				SetContext(ctx).
				SetRetryCount(25). //nolint:gomnd,mnd // .
				SetRetryInterval(func(_ *req.Response, attempt int) stdlibtime.Duration {
			switch {
			case attempt <= 1:
				return 100 * stdlibtime.Millisecond //nolint:gomnd // .
			case attempt == 2: //nolint:gomnd // .
				return 1 * stdlibtime.Second
			default:
				return 10 * stdlibtime.Second //nolint:gomnd // .
			}
		}).
		SetRetryHook(func(resp *req.Response, err error) {
			if err != nil {
				log.Error(errors.Wrap(err, "failed to fetch completed santa tasks, retrying...")) //nolint:revive // .
			} else {
				log.Error(errors.Errorf("failed to fetch completed santa tasks with status code:%v, retrying...", resp.GetStatusCode())) //nolint:revive // .
			}
		}).
		SetRetryCondition(func(resp *req.Response, err error) bool {
			return err != nil || resp.GetStatusCode() != http.StatusOK
		}).
		SetHeader("Accept", "application/json").
		SetHeader("Accept", "application/json").
		SetHeader("Authorization", authorization(ctx)).
		SetHeader("Cache-Control", "no-cache, no-store, must-revalidate").
		SetHeader("Pragma", "no-cache").
		SetHeader("Expires", "0").
		Get(fmt.Sprintf("%v?language=en&status=completed", getCompletedTasksURL))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get fetch `%v`", getCompletedTasksURL)
	}
	data, err2 := resp.ToBytes()
	if err2 != nil {
		return nil, errors.Wrapf(err2, "failed to read body of `%v`", getCompletedTasksURL)
	}
	var tasksResp []*tasks.Task
	if err = json.UnmarshalContext(ctx, data, &tasksResp); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal into %#v, data: %v", tasksResp, string(data))
	}

	return tasksResp, nil
}

func authorization(ctx context.Context) (authorization string) {
	authorization, _ = ctx.Value(authorizationCtxValueKey).(string) //nolint:errcheck // Not needed.

	return
}

func (r *repository) storeLinkedAccounts(ctx context.Context, userID string, res map[TenantScenario]string) error {
	now := time.Now()
	params := []any{}
	values := []string{}
	idx := 1
	for linkTenant, linkUserID := range res {
		lingTenantVal := strings.Split(string(linkTenant), "_")[1]
		params = append(params, now.Time, r.cfg.Tenant, userID, lingTenantVal, linkUserID)
		//nolint:gomnd // .
		values = append(values, fmt.Sprintf("($%[1]v,$%[2]v,$%[3]v,$%[4]v,$%[5]v)", idx, idx+1, idx+2, idx+3, idx+4))
		idx += 5
	}
	sql := fmt.Sprintf(`INSERT INTO 
   									 linked_user_accounts(linked_at, tenant, user_id, linked_tenant, linked_user_id)
    							VALUES %v 
								ON CONFLICT(user_id, linked_user_id, tenant, linked_tenant) DO NOTHING`, strings.Join(values, ",\n"))
	_, err := storage.Exec(ctx, r.globalDB, sql, params...)
	if err != nil {
		return errors.Wrapf(err, "failed to save linked accounts for usr %v: %#v", userID, res)
	}

	return nil
}

func buildGetCompletedTasksURL(tenant, userID, host string, tenantURLs map[string]string) (string, error) {
	var hasURL bool
	var baseURL string
	if len(tenantURLs) > 0 {
		baseURL, hasURL = tenantURLs[tenant]
	}
	if !hasURL {
		var err error
		if baseURL, err = url.JoinPath("https://"+host, tenant); err != nil {
			return "", errors.Wrapf(err, "failed to build user url for get completed tasks %v", tenant)
		}
	}
	userURL, err := url.JoinPath(baseURL, "/v1r/tasks/x/users/", userID)
	if err != nil {
		return "", errors.Wrapf(err, "failed to build user url for tenant %v", tenant)
	}

	return userURL, nil
}

//nolint:funlen,gocognit,revive // .
func (r *repository) VerifyTwitterPost(ctx context.Context, metadata *VerificationMetadata) (*social.Verification, error) {
	now := time.Now()
	user, err := r.userRepo.GetUserByID(ctx, metadata.UserID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to GetUserByID: %v", metadata.UserID)
	}
	sql := `SELECT ARRAY_AGG(x.created_at) AS unsuccessful_attempts 
			FROM (SELECT created_at 
				  FROM verification_distribution_kyc_unsuccessful_attempts 
				  WHERE user_id = $1
				    AND reason != ANY($2)
				  ORDER BY created_at DESC) x`
	res, err := storage.Get[struct {
		UnsuccessfulAttempts *[]time.Time `db:"unsuccessful_attempts"`
	}](ctx, r.db, sql, metadata.UserID, []string{social.ExhaustedRetriesReason})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get unsuccessful_attempts for userID:%v", metadata.UserID)
	}
	remainingAttempts := r.cfg.MaxAttemptsAllowed
	if res.UnsuccessfulAttempts != nil {
		for _, unsuccessfulAttempt := range *res.UnsuccessfulAttempts {
			if unsuccessfulAttempt.After(now.Add(-r.cfg.SessionWindow)) {
				remainingAttempts--
				if remainingAttempts == 0 {
					break
				}
			}
		}
	}
	if remainingAttempts < 1 {
		return nil, social.ErrNotAvailable
	}
	pvm := &social.Metadata{
		PostURL:          metadata.TweetURL,
		ExpectedPostText: r.expectedPostSubtext(user.User, metadata),
		ExpectedPostURL:  r.expectedPostURL(),
	}
	userHandle, err := r.twitterVerifier.VerifyPost(ctx, pvm)
	if err != nil { //nolint:nestif // .
		log.Error(errors.Wrapf(err, "social verification failed for twitter verifier,Language:%v,userID:%v", metadata.Language, metadata.UserID))
		reason := social.DetectReason(err)
		if userHandle != "" {
			reason = strings.ToLower(userHandle) + ": " + reason
		}
		if err = r.saveUnsuccessfulAttempt(ctx, now, reason, metadata); err != nil {
			return nil, errors.Wrapf(err, "[1]failed to saveUnsuccessfulAttempt reason:%v,metadata:%#v", reason, metadata)
		}
		remainingAttempts--
		if remainingAttempts == 0 {
			if err = r.saveUnsuccessfulAttempt(ctx, time.New(now.Add(stdlibtime.Microsecond)), social.ExhaustedRetriesReason, metadata); err != nil {
				return nil, errors.Wrapf(err, "[1]failed to saveUnsuccessfulAttempt reason:%v,metadata:%#v", social.ExhaustedRetriesReason, metadata)
			}
		}

		return &social.Verification{RemainingAttempts: &remainingAttempts, Result: social.FailureVerificationResult}, nil
	}

	return &social.Verification{Result: social.SuccessVerificationResult}, nil
}

func (r *repository) saveUnsuccessfulAttempt(ctx context.Context, now *time.Time, reason string, metadata *VerificationMetadata) error {
	var socialName string
	switch metadata.ScenarioEnum { //nolint:exhaustive // We know what socials we can use here.
	case CoinDistributionScenarioTwitter:
		socialName = "twitter"
	default:
		return errors.Errorf("unknown scenario: %v", metadata.ScenarioEnum)
	}
	sql := `INSERT INTO verification_distribution_kyc_unsuccessful_attempts(created_at, reason, user_id, social) VALUES ($1,$2,$3,$4)`
	_, err := storage.Exec(ctx, r.db, sql, now.Time, reason, metadata.UserID, socialName)

	return errors.Wrapf(err, "failed to `%v`; userId:%v,social:%v,reason:%v", sql, metadata.UserID, socialName, reason)
}

func (r *repository) expectedPostSubtext(user *users.User, metadata *VerificationMetadata) string {
	if tmpl := r.cfg.kycConfigJSON1.Load().XPostPatternTemplate; tmpl != nil {
		bf := new(bytes.Buffer)
		cpy := *user
		cpy.Username = strings.ReplaceAll(cpy.Username, ".", "-")
		log.Panic(errors.Wrapf(tmpl.Execute(bf, cpy), "failed to execute expectedPostSubtext template for metadata:%+v user:%+v", metadata, user))

		return bf.String()
	}

	return ""
}

func (r *repository) expectedPostURL() (resp string) {
	resp = r.cfg.kycConfigJSON1.Load().XPostLink
	resp = strings.Replace(resp, `https://x.com`, "", 1)
	if paramsIndex := strings.IndexRune(resp, '?'); resp != "" && paramsIndex > 0 {
		resp = resp[:paramsIndex]
	}

	return resp
}

func (r *repository) startKYCConfigJSONSyncer(ctx context.Context) {
	ticker := stdlibtime.NewTicker(stdlibtime.Minute)
	defer ticker.Stop()
	r.cfg.kycConfigJSON1 = new(atomic.Pointer[social.KycConfigJSON])
	log.Panic(errors.Wrap(r.syncKYCConfigJSON1(ctx), "failed to syncKYCConfigJSON1")) //nolint:revive // .

	for {
		select {
		case <-ticker.C:
			reqCtx, cancel := context.WithTimeout(ctx, requestDeadline)
			log.Error(errors.Wrap(r.syncKYCConfigJSON1(reqCtx), "failed to syncKYCConfigJSON1"))
			cancel()
		case <-ctx.Done():
			return
		}
	}
}

//nolint:funlen,gomnd,nestif,dupl,revive // .
func (r *repository) syncKYCConfigJSON1(ctx context.Context) error {
	if resp, err := req.
		SetContext(ctx).
		SetRetryCount(25).
		SetRetryInterval(func(_ *req.Response, attempt int) stdlibtime.Duration {
			switch {
			case attempt <= 1:
				return 100 * stdlibtime.Millisecond //nolint:gomnd // .
			case attempt == 2: //nolint:gomnd // .
				return 1 * stdlibtime.Second
			default:
				return 10 * stdlibtime.Second //nolint:gomnd // .
			}
		}).
		SetRetryHook(func(resp *req.Response, err error) {
			if err != nil {
				log.Error(errors.Wrap(err, "failed to fetch KYCConfigJSON, retrying...")) //nolint:revive // .
			} else {
				log.Error(errors.Errorf("failed to fetch KYCConfigJSON with status code:%v, retrying...", resp.GetStatusCode())) //nolint:revive // .
			}
		}).
		SetRetryCondition(func(resp *req.Response, err error) bool {
			return err != nil || resp.GetStatusCode() != http.StatusOK
		}).
		SetHeader("Accept", "application/json").
		SetHeader("Cache-Control", "no-cache, no-store, must-revalidate").
		SetHeader("Pragma", "no-cache").
		SetHeader("Expires", "0").
		Get(r.cfg.ConfigJSONURL1); err != nil {
		return errors.Wrapf(err, "failed to get fetch `%v`", r.cfg.ConfigJSONURL1)
	} else if data, err2 := resp.ToBytes(); err2 != nil {
		return errors.Wrapf(err2, "failed to read body of `%v`", r.cfg.ConfigJSONURL1)
	} else { //nolint:revive // .
		var kycConfig social.KycConfigJSON
		if err = json.UnmarshalContext(ctx, data, &kycConfig); err != nil {
			return errors.Wrapf(err, "failed to unmarshal into %#v, data: %v", kycConfig, string(data))
		}
		if body := string(data); !strings.Contains(body, "xPostPattern") && !strings.Contains(body, "xPostLink") {
			return errors.Errorf("there's something wrong with the KYCConfigJSON body: %v", body)
		}
		if pattern := kycConfig.XPostPattern; pattern != "" {
			if kycConfig.XPostPatternTemplate, err = template.New("kycCfg.Social1KYC.XPostPattern").Parse(pattern); err != nil {
				return errors.Wrapf(err, "failed to parse kycCfg.Social1KYC.xPostPatternTemplate `%v`", pattern)
			}
		}
		r.cfg.kycConfigJSON1.Swap(&kycConfig)

		return nil
	}
}
