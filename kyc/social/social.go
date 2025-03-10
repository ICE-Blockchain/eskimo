// SPDX-License-Identifier: ice License 1.0

package social

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"sync"
	"text/template"
	stdlibtime "time"

	"github.com/pkg/errors"

	scraper "github.com/ice-blockchain/eskimo/kyc/scraper"
	"github.com/ice-blockchain/eskimo/users"
	appcfg "github.com/ice-blockchain/wintr/config"
	"github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/terror"
	"github.com/ice-blockchain/wintr/time"
)

//nolint:gochecknoinits // We load embedded stuff at runtime.
func init() {
	loadTranslations()
}

func loadTranslations() { //nolint:gocognit,revive // .
	tenantDirs, err := translations.ReadDir("translations")
	log.Panic(err) //nolint:revive // Nope.

	for _, tenantFile := range tenantDirs {
		for _, kycStep := range AllSupportedKYCSteps {
			for _, socialType := range AllTypes {
				for _, templateType := range allLanguageTemplateType {
					files, rErr := translations.ReadDir(fmt.Sprintf("translations/%v/%v/%v/%v", tenantFile.Name(), kycStep, socialType, templateType))
					log.Panic(rErr) //nolint:revive // Nope.
					for _, file := range files {
						content, fErr := translations.ReadFile(fmt.Sprintf("translations/%v/%v/%v/%v/%v", tenantFile.Name(), kycStep, socialType, templateType, file.Name()))
						log.Panic(fErr) //nolint:revive // Nope.
						language := strings.Split(file.Name(), ".")[0]
						language = strings.Split(language, "-")[0]
						templName := fmt.Sprintf("translations_%v_%v_%v_%v_%v", tenantFile.Name(), kycStep, socialType, templateType, language)
						tmpl := languageTemplate{Content: fixTemplateParameters(string(content))}
						tmpl.content = template.Must(template.New(templName).Parse(tmpl.Content))

						if _, found := allTemplates[tenantName(tenantFile.Name())]; !found {
							allTemplates[tenantName(tenantFile.Name())] = make(map[users.KYCStep]map[scraper.StrategyType]map[languageTemplateType]map[string][]*languageTemplate)
						}
						if _, found := allTemplates[tenantName(tenantFile.Name())][kycStep]; !found {
							allTemplates[tenantName(tenantFile.Name())][kycStep] = make(map[scraper.StrategyType]map[languageTemplateType]map[string][]*languageTemplate, len(AllTypes)) //nolint:lll // .
						}
						if _, found := allTemplates[tenantName(tenantFile.Name())][kycStep][socialType]; !found {
							allTemplates[tenantName(tenantFile.Name())][kycStep][socialType] = make(map[languageTemplateType]map[string][]*languageTemplate, len(&allLanguageTemplateType)) //nolint:lll // .
						}
						if _, found := allTemplates[tenantName(tenantFile.Name())][kycStep][socialType][templateType]; !found {
							allTemplates[tenantName(tenantFile.Name())][kycStep][socialType][templateType] = make(map[string][]*languageTemplate, len(files))
						}
						allTemplates[tenantName(tenantFile.Name())][kycStep][socialType][templateType][language] = append(allTemplates[tenantName(tenantFile.Name())][kycStep][socialType][templateType][language], &tmpl) //nolint:lll // .
					}
				}
			}
		}
	}
}

func New(ctx context.Context, usrRepo UserRepository) Repository {
	var cfg config
	appcfg.MustLoadFromKey(applicationYamlKey, &cfg)

	socialVerifiers := make(map[Type]scraper.Verifier, len(AllTypes))
	for _, tp := range AllTypes {
		socialVerifiers[tp] = scraper.New(tp)
	}
	cfg.alertFrequency = new(sync.Map)

	repo := &repository{
		user:            usrRepo,
		socialVerifiers: socialVerifiers,
		cfg:             &cfg,
		db:              storage.MustConnect(ctx, ddl, applicationYamlKey),
	}
	for _, kycStep := range AllSupportedKYCSteps {
		cfg.alertFrequency.Store(kycStep, alertFrequency)
		go repo.startUnsuccessfulKYCStepsAlerter(ctx, kycStep)
	}
	go repo.startKYCConfigJSONSyncer(ctx)

	return repo
}

func (r *repository) Close() error {
	return errors.Wrap(r.db.Close(), "closing kyc/social repository failed")
}

func (r *repository) SkipVerification(ctx context.Context, kycStep users.KYCStep, userID string) error {
	now := time.Now()
	user, err := r.user.GetUserByID(ctx, userID)
	if err != nil {
		return errors.Wrapf(err, "failed to GetUserByID: %v", userID)
	}
	if err = r.validateKycStep(user.User, kycStep, now); err != nil {
		return errors.Wrap(err, "failed to validateKycStep")
	}
	metadata := &VerificationMetadata{UserID: userID, Social: TwitterType, KYCStep: kycStep}
	skippedCount, err := r.verifySkipped(ctx, metadata, now)
	if err != nil {
		return errors.Wrapf(err, "failed to verifySkipped for metadata:%#v", metadata)
	}
	if err = r.saveUnsuccessfulAttempt(ctx, now, skippedReason, metadata); err != nil {
		return errors.Wrapf(err, "failed to saveUnsuccessfulAttempt reason:%v,metadata:%#v", skippedReason, metadata)
	}

	return errors.Wrapf(r.modifyUser(ctx, skippedCount+1 == r.cfg.MaxSessionsAllowed, true, kycStep, now, user.User),
		"[skip][kycStep:%v][count:%v]failed to modifyUser", kycStep, skippedCount+1)
}

func (r *repository) verifySkipped(ctx context.Context, metadata *VerificationMetadata, now *time.Time) (int, error) {
	sql := `SELECT count(1) AS skipped,
                   max(created_at) AS latest_created_at
		    FROM social_kyc_unsuccessful_attempts 
		    WHERE user_id = $1
			  AND kyc_step = $2
			  AND reason = ANY($3)`
	res, err := storage.Get[struct {
		LatestCreatedAt *time.Time `db:"latest_created_at"`
		SkippedCount    int        `db:"skipped"`
	}](ctx, r.db, sql, metadata.UserID, metadata.KYCStep, []string{skippedReason, exhaustedRetriesReason})
	if err != nil {
		return 0, errors.Wrapf(err, "failed to get skipped attempt count for kycStep:%v,userID:%v", metadata.KYCStep, metadata.UserID)
	}
	if !res.LatestCreatedAt.IsNil() && now.Sub(*res.LatestCreatedAt.Time) < r.cfg.DelayBetweenSessions {
		return 0, ErrNotAvailable
	}
	if res.SkippedCount >= r.cfg.MaxSessionsAllowed {
		return 0, errors.Wrap(ErrDuplicate, "potential de-sync between social_kyc_unsuccessful_attempts and users")
	}

	return res.SkippedCount, nil
}

//nolint:funlen,gocognit,gocyclo,revive,cyclop // .
func (r *repository) VerifyPost(ctx context.Context, metadata *VerificationMetadata) (*Verification, error) {
	now := time.Now()
	user, err := r.user.GetUserByID(ctx, metadata.UserID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to GetUserByID: %v", metadata.UserID)
	}

	if err = r.validateKycStep(user.User, metadata.KYCStep, now); err != nil {
		return nil, errors.Wrap(err, "failed to validateKycStep")
	}

	skippedCount, err := r.verifySkipped(ctx, metadata, now)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to verifySkipped for metadata:%#v", metadata)
	}
	sql := `SELECT ARRAY_AGG(x.created_at) AS unsuccessful_attempts 
			FROM (SELECT created_at 
				  FROM social_kyc_unsuccessful_attempts 
				  WHERE user_id = $1
				    AND kyc_step = $2
				    AND reason != ANY($3)
				  ORDER BY created_at DESC) x`
	res, err := storage.Get[struct {
		UnsuccessfulAttempts *[]time.Time `db:"unsuccessful_attempts"`
	}](ctx, r.db, sql, metadata.UserID, metadata.KYCStep, []string{skippedReason, exhaustedRetriesReason})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get unsuccessful_attempts for kycStep:%v,userID:%v", metadata.KYCStep, metadata.UserID)
	}
	remainingAttempts := r.cfg.MaxAttemptsAllowed
	if res.UnsuccessfulAttempts != nil {
		for _, unsuccessfulAttempt := range *res.UnsuccessfulAttempts {
			if unsuccessfulAttempt.After(now.Add(-r.cfg.SessionWindow)) {
				remainingAttempts--
			}
		}
	}
	if remainingAttempts < 1 {
		return nil, ErrNotAvailable
	}
	if metadata.Twitter.TweetURL == "" && metadata.Facebook.AccessToken == "" {
		return &Verification{ExpectedPostText: r.expectedPostText(user.User, metadata)}, nil
	}
	pvm := &Metadata{
		AccessToken:      metadata.Facebook.AccessToken,
		PostURL:          metadata.Twitter.TweetURL,
		ExpectedPostText: r.expectedPostSubtext(user.User, metadata),
		ExpectedPostURL:  r.expectedPostURL(metadata),
	}
	userHandle, err := r.socialVerifiers[metadata.Social].VerifyPost(ctx, pvm)
	if err != nil { //nolint:nestif // .
		log.Error(errors.Wrapf(err, "social verification failed for KYCStep:%v,Social:%v,Language:%v,userID:%v",
			metadata.KYCStep, metadata.Social, metadata.Language, metadata.UserID))
		reason := DetectReason(err)
		if userHandle != "" {
			reason = strings.ToLower(userHandle) + ": " + reason
		}
		if err = r.saveUnsuccessfulAttempt(ctx, now, reason, metadata); err != nil {
			return nil, errors.Wrapf(err, "[1]failed to saveUnsuccessfulAttempt reason:%v,metadata:%#v", reason, metadata)
		}
		remainingAttempts--
		if remainingAttempts == 0 {
			if err = r.saveUnsuccessfulAttempt(ctx, time.New(now.Add(stdlibtime.Microsecond)), exhaustedRetriesReason, metadata); err != nil {
				return nil, errors.Wrapf(err, "[1]failed to saveUnsuccessfulAttempt reason:%v,metadata:%#v", exhaustedRetriesReason, metadata)
			}
			end := skippedCount+1 == r.cfg.MaxSessionsAllowed

			if err = r.modifyUser(ctx, end, end, metadata.KYCStep, now, user.User); err != nil {
				return nil, errors.Wrapf(err, "[1failure][%v]failed to modifyUser", metadata.KYCStep)
			}
		}

		return &Verification{RemainingAttempts: &remainingAttempts, Result: FailureVerificationResult}, nil
	}
	if userHandle != "" { //nolint:nestif // .
		userHandle = strings.ToLower(userHandle)
		if err = r.SaveSocial(ctx, metadata.Social, metadata.UserID, userHandle); err != nil {
			if storage.IsErr(err, storage.ErrDuplicate) {
				log.Error(errors.Wrapf(err, "[duplicate]social verification failed for KYCStep:%v,Social:%v,Language:%v,userID:%v,userHandle:%v",
					metadata.KYCStep, metadata.Social, metadata.Language, metadata.UserID, userHandle))
				reason := DetectReason(terror.New(err, map[string]any{"user_handle": userHandle}))
				if err = r.saveUnsuccessfulAttempt(ctx, now, reason, metadata); err != nil {
					return nil, errors.Wrapf(err, "[2]failed to saveUnsuccessfulAttempt reason:%v,metadata:%#v", reason, metadata)
				}
				remainingAttempts--
				if remainingAttempts == 0 {
					if err = r.saveUnsuccessfulAttempt(ctx, time.New(now.Add(stdlibtime.Microsecond)), exhaustedRetriesReason, metadata); err != nil {
						return nil, errors.Wrapf(err, "[2]failed to saveUnsuccessfulAttempt reason:%v,metadata:%#v", exhaustedRetriesReason, metadata)
					}
					end := skippedCount+1 == r.cfg.MaxSessionsAllowed
					if err = r.modifyUser(ctx, end, end, metadata.KYCStep, now, user.User); err != nil {
						return nil, errors.Wrapf(err, "[2failure][%v]failed to modifyUser", metadata.KYCStep)
					}
				}

				return &Verification{RemainingAttempts: &remainingAttempts, Result: FailureVerificationResult}, nil
			}

			return nil, errors.Wrapf(err, "failed to saveSocial social:%v, userID:%v, userHandle:%v", metadata.Social, metadata.UserID, userHandle)
		}
	} else {
		userHandle = metadata.UserID
	}
	if err = r.saveSocialKYCStep(ctx, now, userHandle, metadata); err != nil {
		return nil, errors.Wrapf(err, "failed to saveSocialKYCStep, userHandle:%v, metadata:%#v", userHandle, metadata)
	}
	if err = r.modifyUser(ctx, true, false, metadata.KYCStep, now, user.User); err != nil {
		return nil, errors.Wrapf(err, "[success][%v]failed to modifyUser", metadata.KYCStep)
	}

	return &Verification{Result: SuccessVerificationResult}, nil
}

func (r *repository) validateKycStep(user *users.User, kycStep users.KYCStep, now *time.Time) error {
	allowSocialBeforeFace := true
	if !allowSocialBeforeFace && (user.KYCStepPassed == nil ||
		*user.KYCStepPassed < kycStep-1 ||
		(user.KYCStepPassed != nil &&
			*user.KYCStepPassed == kycStep-1 &&
			user.KYCStepsLastUpdatedAt != nil &&
			len(*user.KYCStepsLastUpdatedAt) >= int(kycStep) &&
			!(*user.KYCStepsLastUpdatedAt)[kycStep-1].IsNil() &&
			now.Sub(*(*user.KYCStepsLastUpdatedAt)[kycStep-1].Time) < r.cfg.DelayBetweenSessions)) {
		return ErrNotAvailable
	} else if user.KYCStepPassed != nil && *user.KYCStepPassed >= kycStep {
		return ErrDuplicate
	}

	return nil
}

//nolint:revive,funlen,gocognit // Nope.
func (r *repository) modifyUser(ctx context.Context, success, skip bool, kycStep users.KYCStep, now *time.Time, user *users.User) error {
	usr := new(users.User)
	usr.ID = user.ID
	usr.KYCStepsLastUpdatedAt = user.KYCStepsLastUpdatedAt
	switch {
	case success:
		usr.KYCStepPassed = &kycStep
		if usr.KYCStepsLastUpdatedAt == nil || len(*usr.KYCStepsLastUpdatedAt) == 0 {
			emptyFaceRecognition := []*time.Time{nil, nil}
			usr.KYCStepsLastUpdatedAt = &emptyFaceRecognition
		}
		if len(*usr.KYCStepsLastUpdatedAt) < int(kycStep) {
			*usr.KYCStepsLastUpdatedAt = append(*usr.KYCStepsLastUpdatedAt, now)
		} else {
			(*usr.KYCStepsLastUpdatedAt)[int(kycStep)-1] = now
		}
		// This is just a hack so that we can differentiate between a failed/skipped Social 2 and a successful one:
		// Social2KYCStep is a failed/skipped Social 2 outcome
		// Social3KYCStep is a completed Social 2 outcome
		// And, in actuality, there is no Social 3.
		if (kycStep == users.Social1KYCStep || kycStep == users.Social2KYCStep) && !skip {
			nextStep := kycStep + 1
			usr.KYCStepPassed = &nextStep
			if len(*usr.KYCStepsLastUpdatedAt) < int(nextStep) {
				*usr.KYCStepsLastUpdatedAt = append(*usr.KYCStepsLastUpdatedAt, now)
			} else {
				(*usr.KYCStepsLastUpdatedAt)[int(nextStep)-1] = now
			}
		}
	case !success:
		if usr.KYCStepsLastUpdatedAt == nil || len(*usr.KYCStepsLastUpdatedAt) == 0 {
			emptyFaceRecognition := []*time.Time{nil, nil}
			usr.KYCStepsLastUpdatedAt = &emptyFaceRecognition
		}
		if len(*usr.KYCStepsLastUpdatedAt) < int(kycStep) {
			*usr.KYCStepsLastUpdatedAt = append(*usr.KYCStepsLastUpdatedAt, now)
		} else {
			(*usr.KYCStepsLastUpdatedAt)[int(kycStep)-1] = now
		}
	}
	_, mErr := r.user.ModifyUser(ctx, usr, nil)

	return errors.Wrapf(mErr, "[skip:%v]failed to modify user %#v", skip, usr)
}

func (r *repository) saveSocialKYCStep(ctx context.Context, now *time.Time, userHandle string, metadata *VerificationMetadata) error {
	sql := `insert into social_kyc_steps(created_at,kyc_step,user_id,social,user_handle) VALUES ($1,$2,$3,$4,$5)`
	_, err := storage.Exec(ctx, r.db, sql, now.Time, metadata.KYCStep, metadata.UserID, metadata.Social, userHandle)

	return errors.Wrapf(err, "failed to `%v`;KYCStep:%v, userID:%v, social:%v, userHandle:%v", sql, metadata.KYCStep, metadata.UserID, metadata.Social, userHandle)
}

func (r *repository) saveUnsuccessfulAttempt(ctx context.Context, now *time.Time, reason string, metadata *VerificationMetadata) error {
	sql := `INSERT INTO social_kyc_unsuccessful_attempts(created_at, kyc_step, reason, user_id, social) VALUES ($1,$2,$3,$4,$5)`
	_, err := storage.Exec(ctx, r.db, sql, now.Time, metadata.KYCStep, reason, metadata.UserID, metadata.Social)

	return errors.Wrapf(err, "failed to `%v`; kycStep:%v,userId:%v,social:%v,reason:%v", sql, metadata.KYCStep, metadata.UserID, metadata.Social, reason)
}

func (r *repository) SaveSocial(ctx context.Context, socialType Type, userID, userHandle string) error {
	sql := `INSERT INTO socials(user_id,social,user_handle) VALUES ($1,$2,$3)`
	_, err := storage.Exec(ctx, r.db, sql, userID, socialType, userHandle)
	if socialType != CMCType {
		if err != nil && storage.IsErr(err, storage.ErrDuplicate, "pk") {
			sql = `SELECT true AS bogus WHERE EXISTS (SELECT 1 FROM socials WHERE user_id = $1 AND social = $2 AND lower(user_handle) = $3)`
			if _, err2 := storage.ExecOne[struct{ Bogus bool }](ctx, r.db, sql, userID, socialType, userHandle); err2 == nil {
				return nil
			} else if !storage.IsErr(err2, storage.ErrNotFound) {
				err = errors.Wrapf(err2, "failed to check if user used the same userhandle previously; userID:%v, social:%v, userHandle:%v",
					userID, socialType, userHandle)
			}
		}
	}

	return errors.Wrapf(err, "failed to `%v`; userID:%v, social:%v, userHandle:%v", sql, userID, socialType, userHandle)
}

func DetectReason(err error) string {
	switch {
	case errors.Is(err, scraper.ErrInvalidPageContent):
		return "invalid page content"
	case errors.Is(err, scraper.ErrTextNotFound):
		return "expected text not found"
	case errors.Is(err, scraper.ErrUsernameNotFound):
		return "username not found"
	case errors.Is(err, scraper.ErrPostNotFound):
		return "post not found"
	case errors.Is(err, scraper.ErrInvalidURL):
		return "invalid URL"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancellation"
	case errors.Is(err, scraper.ErrFetchFailed):
		return "post fetch failed"
	case storage.IsErr(err, storage.ErrDuplicate):
		if tErr := terror.As(err); tErr != nil {
			if unwrapped := tErr.Unwrap(); storage.IsErr(unwrapped, storage.ErrDuplicate, "pk") {
				return fmt.Sprintf("duplicate userhandle '%v'", tErr.Data["user_handle"])
			}
		}

		fallthrough
	default:
		return "unexpected"
	}
}

func (r *repository) expectedPostText(user *users.User, vm *VerificationMetadata) string {
	if r.expectedPostTextIsExactMatch(vm) {
		return r.expectedPostSubtext(user, vm)
	}

	return r.ExpectedPostTemplateText(user, vm)
}

func (r *repository) ExpectedPostTemplateText(user *users.User, vm *VerificationMetadata) string {
	var (
		templ *languageTemplate
		tname = tenantName(r.cfg.TenantName)
	)
	if _, found := allTemplates[tname][vm.KYCStep][vm.Social][postContentLanguageTemplateType][vm.Language]; found {
		randVal := getRandomIndex(int64(len(allTemplates[tname][vm.KYCStep][vm.Social][postContentLanguageTemplateType][vm.Language])))
		templ = allTemplates[tname][vm.KYCStep][vm.Social][postContentLanguageTemplateType][vm.Language][randVal]
	} else {
		randVal := getRandomIndex(int64(len(allTemplates[tname][vm.KYCStep][vm.Social][postContentLanguageTemplateType]["en"])))
		templ = allTemplates[tname][vm.KYCStep][vm.Social][postContentLanguageTemplateType]["en"][randVal]
	}
	bf := new(bytes.Buffer)
	data := map[string]any{"Username": strings.ReplaceAll(user.Username, ".", "-"), "WelcomeBonus": r.cfg.WelcomeBonusV2Amount}
	log.Panic(errors.Wrapf(templ.content.Execute(bf, data), "failed to execute postContentLanguageTemplateType template for data:%#v", data))

	return bf.String()
}

func getRandomIndex(maxVal int64) uint64 {
	if maxVal == 0 {
		log.Panic(errors.New("no translations"))
	}
	if maxVal == 1 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(maxVal))
	log.Panic(errors.Wrap(err, "crypto random generator failed"))

	return n.Uint64()
}

func (r *repository) expectedPostTextIsExactMatch(metadata *VerificationMetadata) bool {
	if metadata.Social == TwitterType {
		switch metadata.KYCStep { //nolint:exhaustive // Not needed. Everything else is validated before this.
		case users.Social1KYCStep:
			return r.cfg.kycConfigJSON1.Load().XPostPatternTemplate != nil && r.cfg.kycConfigJSON1.Load().XPostPatternExactMatch
		case users.Social2KYCStep:
			return r.cfg.kycConfigJSON2.Load().XPostPatternTemplate != nil && r.cfg.kycConfigJSON2.Load().XPostPatternExactMatch
		default:
			panic(fmt.Sprintf("social step `%v` not implemented ", metadata.KYCStep))
		}
	}

	return false
}

func (r *repository) expectedPostSubtext(user *users.User, metadata *VerificationMetadata) string {
	if metadata.Social == TwitterType {
		var tmpl *template.Template
		switch metadata.KYCStep { //nolint:exhaustive // Not needed. Everything else is validated before this.
		case users.Social1KYCStep:
			tmpl = r.cfg.kycConfigJSON1.Load().XPostPatternTemplate
		case users.Social2KYCStep:
			tmpl = r.cfg.kycConfigJSON2.Load().XPostPatternTemplate
		default:
			panic(fmt.Sprintf("social step `%v` not implemented ", metadata.KYCStep))
		}
		if tmpl != nil {
			bf := new(bytes.Buffer)
			cpy := *user
			cpy.Username = strings.ReplaceAll(cpy.Username, ".", "-")
			log.Panic(errors.Wrapf(tmpl.Execute(bf, cpy), "failed to execute expectedPostSubtext template for metadata:%+v user:%+v", metadata, user))

			return bf.String()
		}
	}

	return ""
}

func (r *repository) expectedPostURL(metadata *VerificationMetadata) (url string) {
	if metadata.Social == TwitterType {
		switch metadata.KYCStep { //nolint:exhaustive // Not needed. Everything else is validated before this.
		case users.Social1KYCStep:
			url = r.cfg.kycConfigJSON1.Load().XPostLink
		case users.Social2KYCStep:
			url = r.cfg.kycConfigJSON2.Load().XPostLink
		default:
			panic(fmt.Sprintf("social step `%v` not implemented ", metadata.KYCStep))
		}

		url = strings.Replace(url, `https://x.com`, "", 1)
		if paramsIndex := strings.IndexRune(url, '?'); url != "" && paramsIndex > 0 {
			url = url[:paramsIndex]
		}
	}

	return url
}

func fixTemplateParameters(orig string) string {
	result := orig
	re := regexp.MustCompile(`{{[^{}]*}}`)
	newStrs := re.FindAllString(result, -1)
	for _, s := range newStrs {
		if strings.HasSuffix(s, ".}}") {
			textWithoutBrackets := s[0 : len(s)-3]
			textWithoutBrackets = textWithoutBrackets[2:]
			result = strings.ReplaceAll(result, s, fmt.Sprintf("{{.%v}}", textWithoutBrackets))
		}
	}

	return result
}
