// SPDX-License-Identifier: ice License 1.0

package verificationscenarios

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"text/template"
	stdlibtime "time"

	"github.com/goccy/go-json"
	"github.com/imroc/req/v3"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/kyc/linking"
	"github.com/ice-blockchain/eskimo/kyc/scraper"
	"github.com/ice-blockchain/eskimo/kyc/social"
	"github.com/ice-blockchain/eskimo/users"
	"github.com/ice-blockchain/santa/tasks"
	appcfg "github.com/ice-blockchain/wintr/config"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/time"
)

func New(ctx context.Context, usrRepo UserRepository, linker linking.Linker, socialRepo social.Repository, host string) Repository {
	var cfg config
	appcfg.MustLoadFromKey(applicationYamlKey, &cfg)
	repo := &repository{
		userRepo:        usrRepo,
		cfg:             &cfg,
		host:            host,
		twitterVerifier: scraper.New(scraper.StrategyTwitter),
		cmcVerifier:     scraper.New(scraper.StrategyCMC),
		linkerRepo:      linker,
		socialRepo:      socialRepo,
	}
	go repo.startKYCConfigJSONSyncer(ctx)

	return repo
}

//nolint:funlen,gocognit,gocyclo,revive,cyclop // .
func (r *repository) VerifyScenarios(ctx context.Context, metadata *VerificationMetadata) (res *Verification, err error) {
	now := time.Now()
	userIDScenarioMap := make(map[TenantScenario]users.UserID, 0)
	usr, err := r.userRepo.GetUserByID(ctx, metadata.UserID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get user by id: %v", metadata.UserID)
	}
	pendingScenarios, err := r.getPendingScenarios(ctx, usr.User)
	if err != nil {
		return nil, errors.Wrapf(err, "can't call getPendingScenarios for %v", usr.ID)
	}
	if len(pendingScenarios) == 0 || !isScenarioPending(pendingScenarios, string(metadata.ScenarioEnum)) {
		return nil, errors.Wrapf(ErrNoPendingScenarios, "no pending scenarios for user: %v", metadata.UserID)
	}
	switch metadata.ScenarioEnum {
	case CoinDistributionScenarioCmc:
		if vErr := r.VerifyCMC(ctx, metadata); vErr != nil {
			return nil, errors.Wrapf(vErr, "haven't passed the CMC verification for userID:%v", metadata.UserID)
		}
	case CoinDistributionScenarioTwitter:
		if metadata.TweetURL == "" {
			return &Verification{
				ExpectedPostText: r.socialRepo.ExpectedPostTemplateText(usr.User, &social.VerificationMetadata{
					Language: metadata.Language,
					KYCStep:  users.Social1KYCStep,
					Social:   social.TwitterType,
				}),
			}, nil
		}
	case CoinDistributionScenarioTelegram:
	case CoinDistributionScenarioSignUpTenants:
		skippedTokenCount := 0
		tenantTokens := make(map[linking.Tenant]linking.Token, len(metadata.TenantTokens))
		for tenantScenario, token := range metadata.TenantTokens {
			if !isScenarioPending(pendingScenarios, tenantScenario) {
				skippedTokenCount++

				continue
			}
			splitted := strings.Split(tenantScenario, "_") // Here we have: signup_<tenant>.
			tenantTokens[splitted[1]] = token
		}
		if skippedTokenCount == len(metadata.TenantTokens) {
			return nil, errors.Wrapf(ErrWrongTenantTokens, "no pending tenant tokens for userID:%v", metadata.UserID)
		}
		var links linking.LinkedProfiles
		links, _, err = r.linkerRepo.Verify(ctx, now, usr.ID, tenantTokens)
		if err != nil {
			if errors.Is(err, linking.ErrRemoteUserNotFound) {
				return nil, errors.Wrapf(linking.ErrNotOwnRemoteUser, "foreign token of userID:%v", metadata.UserID)
			}

			return nil, errors.Wrapf(err, "failed to verify linking user for userID:%v", metadata.UserID)
		}
		for tenant, linkedUser := range links {
			for inputTenant := range tenantTokens {
				if inputTenant == tenant {
					userIDScenarioMap[singUpPrefix+"_"+tenant] = linkedUser
				}
			}
		}
	}

	return res, errors.Wrapf(r.setCompletedDistributionScenario(ctx, usr.User, metadata.ScenarioEnum, userIDScenarioMap, pendingScenarios),
		"failed to setCompletedDistributionScenario for userID:%v", metadata.UserID)
}

func (r *repository) GetPendingVerificationScenarios(ctx context.Context, userID string) ([]Scenario, error) {
	usr, err := r.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get user by id: %v", userID)
	}
	pendingScenarios, err := r.getPendingScenarios(ctx, usr.User)
	if err != nil {
		return nil, errors.Wrapf(err, "can't call getPendingScenarios for %v", usr.ID)
	}
	sort.SliceStable(pendingScenarios, func(i, j int) bool {
		return scenarioOrder[pendingScenarios[i]] < scenarioOrder[pendingScenarios[j]]
	})

	return pendingScenarios, nil
}

func (r *repository) getPendingScenarios(ctx context.Context, usr *users.User) (scenarios []Scenario, err error) {
	if r.isMandatoryScenariosEnabled() {
		if isDistributionScenariosVerified(usr) {
			return []Scenario{}, nil
		}

		return r.getMandatoryScenarios(usr), nil
	}
	completedSantaTasks, err := r.getCompletedSantaTasks(ctx, usr.ID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to getCompletedSantaTasks for userID: %v", usr.ID)
	}

	return r.getUsualPendingScenarios(usr, completedSantaTasks), nil
}

func isDistributionScenariosVerified(usr *users.User) bool {
	return usr.DistributionScenariosVerified != nil && *usr.DistributionScenariosVerified
}

func (r *repository) getMandatoryScenarios(usr *users.User) []Scenario {
	if usr.DistributionScenariosCompleted == nil {
		return r.cfg.MandatoryScenarios
	}
	scenarios := make([]Scenario, 0)
	for _, mandatoryScenario := range r.cfg.MandatoryScenarios {
		found := false
		for _, completedScenario := range *usr.DistributionScenariosCompleted {
			if completedScenario == string(mandatoryScenario) {
				found = true

				break
			}
		}
		if !found {
			scenarios = append(scenarios, mandatoryScenario)
		}
	}

	return scenarios
}

//nolint:funlen,gocognit,gocyclo,revive,cyclop // .
func (r *repository) getUsualPendingScenarios(usr *users.User, completedSantaTasks []*tasks.Task) []Scenario {
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
					tenantsScenariosCompleted[completedScenario] = true
				}
			}
		}
	}
	scenarios := make([]Scenario, 0)
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
					scenarios = append(scenarios, scenario)
				}
			}
		}
	}
	if joinBulllishCMCTaskCompleted && joinIONCMCTaskCompleted && joinWatchlistCMCTaskCompleted && !cmcScenarioCompleted {
		val := CoinDistributionScenarioCmc
		scenarios = append(scenarios, val)
	}
	if joinTwitterTaskCompleted && !twitterScenarioCompleted {
		val := CoinDistributionScenarioTwitter
		scenarios = append(scenarios, val)
	}
	if joinTelegramTaskCompleted && !telegramScenarioCompleted {
		val := CoinDistributionScenarioTelegram
		scenarios = append(scenarios, val)
	}

	return scenarios
}

func isScenarioPending(pendingScenarios []Scenario, scenario string) bool {
	if scenario == string(CoinDistributionScenarioSignUpTenants) {
		return true
	}
	for _, pending := range pendingScenarios {
		if string(pending) == scenario {
			return true
		}
	}

	return false
}

//nolint:funlen // .
func (r *repository) setCompletedDistributionScenario(
	ctx context.Context, usr *users.User, scenario Scenario, userIDMap map[TenantScenario]users.UserID, pendingTasks []Scenario,
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
			scenarios = append(scenarios, tenant)
		}
	}
	updUsr := new(users.User)
	updUsr.ID = usr.ID
	updUsr.DistributionScenariosCompleted = &scenarios
	completedCount := 0
	for _, completed := range scenarios {
		for _, pending := range pendingTasks {
			if completed == string(pending) {
				completedCount++
			}
		}
	}
	if completedCount == len(pendingTasks) {
		trueVal := true
		updUsr.DistributionScenariosVerified = &trueVal
	}
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

func (r *repository) VerifyTwitterPost(ctx context.Context, metadata *VerificationMetadata) error {
	user, err := r.userRepo.GetUserByID(ctx, metadata.UserID)
	if err != nil {
		return errors.Wrapf(err, "failed to GetUserByID: %v", metadata.UserID)
	}
	pvm := &social.Metadata{
		PostURL:          metadata.TweetURL,
		ExpectedPostText: r.expectedPostSubtext(user.User, metadata),
		ExpectedPostURL:  r.expectedPostURL(),
	}
	userHandle, err := r.twitterVerifier.VerifyPost(ctx, pvm)
	if err != nil {
		return errors.Wrapf(ErrVerificationNotPassed,
			"can't verify post for twitter verifier userID:%v,Language:%v,reason:%v", metadata.UserID, metadata.Language, social.DetectReason(err))
	}
	if userHandle == "" {
		return errors.Wrapf(ErrVerificationNotPassed,
			"user handle is empty after the verifyPost call for twitter verifier,Language:%v,userID:%v", metadata.Language, metadata.UserID)
	}

	return nil
}

func (r *repository) VerifyCMC(ctx context.Context, metadata *VerificationMetadata) error {
	pvm := &social.Metadata{
		PostURL: metadata.CMCProfileLink,
	}
	_, err := r.cmcVerifier.VerifyPost(ctx, pvm)
	if err != nil {
		return errors.Wrapf(ErrVerificationNotPassed,
			"can't verify post for cmc verifier userID:%v,reason:%v", metadata.UserID, err.Error())
	}
	if sErr := r.socialRepo.SaveSocial(ctx, social.CMCType, metadata.UserID, metadata.CMCProfileLink); sErr != nil {
		return errors.Wrapf(ErrVerificationNotPassed,
			"can't save social for userID:%v for post link: %v, reason: %v", metadata.UserID, metadata.CMCProfileLink, social.DetectReason(sErr))
	}

	return nil
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

func (r *repository) isMandatoryScenariosEnabled() bool {
	return len(r.cfg.MandatoryScenarios) > 0
}
