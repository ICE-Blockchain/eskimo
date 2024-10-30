// SPDX-License-Identifier: ice License 1.0

package linking

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	stdlibtime "time"

	"github.com/goccy/go-json"
	"github.com/imroc/req/v3"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	appcfg "github.com/ice-blockchain/wintr/config"
	"github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/server"
	"github.com/ice-blockchain/wintr/time"
)

func init() { //nolint:gochecknoinits // It's the only way to tweak the client.
	req.DefaultClient().SetJsonMarshal(json.Marshal)
	req.DefaultClient().SetJsonUnmarshal(json.Unmarshal)
	req.DefaultClient().GetClient().Timeout = requestDeadline
}

func NewAccountLinker(ctx context.Context, host string) Linker {
	var cfg config
	appcfg.MustLoadFromKey(applicationYamlKey, &cfg)
	if len(cfg.TenantURLs) == 0 && host == "" {
		log.Panic("kyc/linking: Must provide tenantURLs or host")
	}
	db := storage.MustConnect(ctx, ddl, applicationYamlKey)

	return &linker{
		globalDB: db,
		cfg:      &cfg,
		host:     host,
	}
}

func (l *linker) Verify(ctx context.Context, now *time.Time, userID UserID, tokens map[Tenant]Token) (allProfiles LinkedProfiles, verified Tenant, err error) {
	allProfiles, verified, err = l.Get(ctx, userID)
	if err != nil {
		return allProfiles, verified, errors.Wrapf(err, "failed to get existing links for user %v", userID)
	}
	for tenant, token := range tokens {
		var remoteID string
		var hasKYCResult bool
		remoteID, hasKYCResult, err = l.verifyToken(ctx, userID, tenant, token)
		if err != nil {
			return allProfiles, verified, errors.Wrap(err, "failed to verify token")
		}
		var remoteProfiles LinkedProfiles
		remoteProfiles, verified, err = l.Get(ctx, remoteID)
		if err != nil {
			return allProfiles, verified, errors.Wrapf(err, "failed to get remote profiles for remote %v %v", tenant, remoteID)
		}
		for remTenant, rem := range remoteProfiles {
			allProfiles[remTenant] = rem
		}
		allProfiles[tenant] = remoteID
		if hasKYCResult && verified == "" {
			verified = tenant
		}
	}
	allProfiles[l.cfg.Tenant] = userID
	if err = l.storeLinkedAccounts(ctx, now, userID, verified, allProfiles); err != nil {
		return nil, "", errors.Wrapf(err, "failed to save linked accounts for %v", userID)
	}

	return allProfiles, verified, nil
}

func (l *linker) storeLinkedAccounts(ctx context.Context, now *time.Time, userID, verifiedTenant string, res map[Tenant]UserID) error {
	params := []any{}
	values := []string{}
	idx := 1
	for linkTenant, linkUserID := range res {
		params = append(params, *now.Time, l.cfg.Tenant, userID, linkTenant, linkUserID, linkTenant == verifiedTenant)
		//nolint:gomnd // .
		values = append(values, fmt.Sprintf("($%[1]v,$%[2]v,$%[3]v,$%[4]v,$%[5]v, $%[6]v)", idx, idx+1, idx+2, idx+3, idx+4, idx+5))
		idx += 6
	}
	sql := fmt.Sprintf(`INSERT INTO 
   									 linked_user_accounts(linked_at, tenant, user_id, linked_tenant, linked_user_id, has_kyc)
    							VALUES %v`, strings.Join(values, ",\n"))
	rows, err := storage.Exec(ctx, l.globalDB, sql, params...)
	if err != nil {
		return errors.Wrapf(err, "failed to save linked accounts for usr %v: %#v", userID, res)
	}
	if rows != uint64(len(res)) {
		return errors.Errorf("failed unexpected rows on saving linked accounts for usr %v %v instead of %v", userID, rows, len(res))
	}

	return nil
}

//nolint:funlen // .
func (l *linker) Get(ctx context.Context, userID UserID) (allLinkedProfiles LinkedProfiles, verified Tenant, err error) {
	verified = ""
	linkedUsers, err := storage.Select[struct {
		LinkedAt     *time.Time `db:"linked_at"`
		UserID       string     `db:"user_id"`
		Tenant       string     `db:"tenant"`
		LinkedTenant string     `db:"linked_tenant"`
		LinkedUserID string     `db:"linked_user_id"`
		HasKYC       bool       `db:"has_kyc"`
	}](ctx, l.globalDB, `WITH RECURSIVE rec AS (
								SELECT * FROM linked_user_accounts WHERE user_id = $1 OR linked_user_id = $1
								UNION SELECT linked_user_accounts.* FROM linked_user_accounts
								JOIN rec on rec.user_id = linked_user_accounts.user_id OR rec.linked_user_id = linked_user_accounts.linked_user_id
							) SELECT * FROM rec;`, userID)
	if err != nil && !storage.IsErr(err, storage.ErrNotFound) {
		return nil, "", errors.Wrapf(err, "failed to fetch linked accounts for user %v", userID)
	}
	allLinkedProfiles = make(map[Tenant]UserID)
	if len(linkedUsers) == 0 {
		return allLinkedProfiles, verified, nil
	}
	for _, usr := range linkedUsers {
		if usr.LinkedTenant == l.cfg.Tenant && usr.LinkedUserID == userID {
			allLinkedProfiles[usr.Tenant] = usr.UserID
			allLinkedProfiles[usr.LinkedTenant] = usr.LinkedUserID
		} else {
			allLinkedProfiles[usr.LinkedTenant] = usr.LinkedUserID
		}
		if usr.HasKYC {
			verified = usr.Tenant
		}
	}

	return allLinkedProfiles, verified, nil
}

func (l *linker) verifyToken(ctx context.Context, userID, tenant, token string) (remoteID UserID, hasFaceResult bool, err error) {
	usr, err := FetchTokenData(ctx, tenant, token, l.host, l.cfg.TenantURLs)
	if err != nil {
		if errors.Is(err, ErrRemoteUserNotFound) {
			return "", false, errors.Wrapf(ErrNotOwnRemoteUser, "token is not belong to %v", userID)
		}

		return "", false, errors.Wrapf(err, "failed to fetch remote user data for %v", userID)
	}
	if usr.CreatedAt == nil || usr.ReferredBy == "" || usr.Username == "" {
		return "", false, errors.Wrapf(ErrNotOwnRemoteUser, "token is not belong to %v", userID)
	}

	return usr.ID, usr.HasFaceKYCResult(), nil
}

//nolint:funlen // Single http call.
func FetchTokenData(ctx context.Context, tenant, token, host string, tenantURLs map[Tenant]string) (*users.User, error) {
	tok, err := server.Auth(ctx).ParseToken(token, false)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid token passed")
	}
	var resp *req.Response
	var usr users.User
	getUserURL, err := buildGetUserURL(tenant, tok.Subject, host, tenantURLs)
	if err != nil {
		log.Panic(errors.Wrapf(err, "failed to detect tenant url"))
	}
	if resp, err = req.
		SetContext(ctx).
		SetRetryCount(10). //nolint:gomnd // .
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
				log.Error(errors.Wrap(err, "failed to check accounts linking, retrying... "))
			} else {
				body, bErr := resp.ToString()
				log.Error(errors.Wrapf(bErr, "failed to parse negative response body for account linking"))
				log.Error(errors.Errorf("failed check link accounts with status code:%v, body:%v, retrying... ", resp.GetStatusCode(), body))
			}
		}).
		SetRetryCondition(func(resp *req.Response, err error) bool {
			return err != nil ||
				((resp.GetStatusCode() != http.StatusOK) && (resp.GetStatusCode() != http.StatusNotFound) && (resp.GetStatusCode() != http.StatusUnauthorized))
		}).
		SetHeader("Authorization", token).
		AddQueryParam("caller", "eskimo-hut").
		Get(getUserURL); err != nil {
		return nil, errors.Wrap(err, "failed to link accounts")
	} else if statusCode := resp.GetStatusCode(); statusCode != http.StatusOK {
		if statusCode == http.StatusNotFound {
			return nil, errors.Wrapf(ErrRemoteUserNotFound, "wrong status code for fetch token data for user %v", tok.Subject)
		}

		return nil, errors.Errorf("[%v]failed to link accounts", statusCode)
	} else if data, err2 := resp.ToBytes(); err2 != nil {
		return nil, errors.Wrapf(err2, "failed to read body of linking users: %v", getUserURL)
	} else if jErr := json.UnmarshalContext(ctx, data, &usr); jErr != nil {
		return nil, errors.Wrapf(err2, "failed to decode body of linking users: %v", string(data))
	}

	return &usr, nil
}

func buildGetUserURL(tenant, userID, host string, tenantURLs map[Tenant]string) (string, error) {
	var hasURL bool
	var baseURL string
	if len(tenantURLs) > 0 {
		baseURL, hasURL = tenantURLs[tenant]
	}
	if !hasURL {
		var err error
		if baseURL, err = url.JoinPath("https://"+host, tenant); err != nil {
			return "", errors.Wrapf(err, "failed to build user url for tenant %v", tenant)
		}
	}

	userURL, err := url.JoinPath(baseURL, "v1r/users/", userID)
	if err != nil {
		return "", errors.Wrapf(err, "failed to build user url for tenant %v", tenant)
	}

	return userURL, nil
}

func (l *linker) SetTenantVerified(ctx context.Context, userID UserID, tenant Tenant) error {
	_, err := storage.Exec(ctx, l.globalDB, `UPDATE linked_user_accounts SET has_kyc = true 
                            WHERE linked_tenant = $1 AND linked_user_id = $2
                            AND user_id = linked_user_id AND tenant = linked_tenant`, tenant, userID)

	return errors.Wrapf(err, "failed to set verified tenant for %v %v", userID, tenant)
}

func (l *linker) Close() error {
	return errors.Wrap(l.globalDB.Close(), "failed to close global db")
}
