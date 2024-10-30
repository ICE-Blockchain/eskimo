// SPDX-License-Identifier: ice License 1.0

package verificationscenarios

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	stdlibtime "time"

	"github.com/goccy/go-json"
	"github.com/hashicorp/go-multierror"
	"github.com/imroc/req/v3"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/kyc/linking"
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
		userRepo:     usrRepo,
		cfg:          &cfg,
		host:         host,
		socialClient: social.New(ctx, usrRepo),
		globalDB:     storage.MustConnect(ctx, "", applicationYamlKey),
	}

	return repo
}

func (r *repository) Close() error {
	return errors.Wrap(multierror.Append(nil,
		errors.Wrap(r.socialClient.Close(), "closing distribution repository failed"),
		errors.Wrap(r.userRepo.Close(), "closing users repository failed"),
		errors.Wrap(r.globalDB.Close(), "closing db connection failed"),
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
		verification, sErr := r.socialClient.VerifyPostForDistibutionVerification(ctx, &social.VerificationMetadata{
			UserID:   metadata.UserID,
			Language: metadata.Language,
			Social:   social.TwitterType,
			Twitter: social.Twitter{
				TweetURL: metadata.TweetURL,
			},
			KYCStep: users.Social1KYCStep,
		})
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
	if err != nil {
		return errors.Wrapf(err, "failed to modify user for userID: %v, error: %v", usr.ID, err)
	}

	return errors.Wrapf(err, "failed to set completed distribution scenarios:%v", scenarios)
}

//nolint:funlen // .
func (r *repository) getCompletedSantaTasks(ctx context.Context, userID string) (res []*tasks.Task, err error) {
	resp, err := req.
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
		Get(fmt.Sprintf("%v%v?language=en&status=completed", r.cfg.SantaTasksURL, userID))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get fetch `%v`", r.cfg.SantaTasksURL)
	}
	data, err2 := resp.ToBytes()
	if err2 != nil {
		return nil, errors.Wrapf(err2, "failed to read body of `%v`", r.cfg.SantaTasksURL)
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
