// SPDX-License-Identifier: ice License 1.0

package linking

import (
	"context"
	"crypto/tls"
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
	req.DefaultClient().SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
}

func NewAccountLinker(ctx context.Context) Linker {
	var cfg config
	appcfg.MustLoadFromKey("kyc/linking", &cfg)
	if len(cfg.TenantURLs) == 0 {
		log.Panic("kyc/linking: Must provide tenantURLs")
	}

	return &linker{
		globalDB: storage.MustConnect(ctx, ddl, ""),
		cfg:      &cfg,
	}
}

func (l *linker) Verify(ctx context.Context, now *time.Time, userID UserID, tokens map[Tenant]Token) (allProfiles LinkedProfiles, verified Tenant, err error) {
	allProfiles = map[Tenant]UserID{}
	verified = l.cfg.Tenant
	for tenant, token := range tokens {
		var remoteID string
		var hasKYCResult bool
		remoteID, hasKYCResult, err = l.verifyToken(ctx, userID, tenant, token)
		if err != nil {
			return allProfiles, verified, err
		}
		allProfiles[tenant] = remoteID
		if hasKYCResult && verified == l.cfg.Tenant && tenant != l.cfg.Tenant {
			verified = tenant
		}
	}
	allProfiles[l.cfg.Tenant] = userID
	if err = l.storeLinkedAccounts(ctx, now, userID, verified, allProfiles); err != nil {
		return nil, "", errors.Wrapf(err, "failed to save linked accounts for %v", userID)
	}

	return l.Get(ctx, userID)
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

func (l *linker) Get(ctx context.Context, userID UserID) (allLinkedProfiles LinkedProfiles, verified Tenant, err error) {
	verified = l.cfg.Tenant
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
	if err != nil && storage.IsErr(err, storage.ErrNotFound) {
		return nil, "", errors.Wrapf(err, "failed to fetch linked accounts for user %v", userID)
	}
	allLinkedProfiles = make(map[Tenant]UserID)
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
	usr, err := l.fetchTokenData(ctx, tenant, token)
	if err != nil {
		if errors.Is(err, errRemoteUserNotFound) {
			return "", false, ErrNotOwnRemoteUser
		}

		return "", false, errors.Wrapf(err, "failed to link accounts with %v", userID)
	}
	if usr.CreatedAt == nil || usr.ReferredBy == "" || usr.Username == "" {
		return "", false, ErrNotOwnRemoteUser
	}

	return usr.ID, usr.HasFaceKYCResult(), nil
}

//nolint:funlen // Single http call.
func (l *linker) fetchTokenData(ctx context.Context, tenant, token string) (*users.User, error) {
	tok, err := server.Auth(ctx).ParseToken(token, false)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid token passed")
	}
	var resp *req.Response
	var usr users.User
	getUserURL, err := l.buildGetUserURL(tenant, tok.Subject)
	if err != nil {
		log.Panic(errors.Wrapf(err, "failed to detect tenant url"))
	}
	if resp, err = req.
		SetContext(ctx).
		SetRetryCount(10).                                                       //nolint:gomnd // .
		SetRetryBackoffInterval(10*stdlibtime.Millisecond, 1*stdlibtime.Second). //nolint:gomnd // .
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
			return nil, errRemoteUserNotFound
		}

		return nil, errors.Errorf("[%v]failed to link accounts", statusCode)
	} else if data, err2 := resp.ToBytes(); err2 != nil {
		return nil, errors.Wrapf(err2, "failed to read body of linking users: %v", getUserURL)
	} else if jErr := json.UnmarshalContext(ctx, data, &usr); jErr != nil {
		return nil, errors.Wrapf(err2, "failed to decode body of linking users: %v", string(data))
	}

	return &usr, nil
}

func (l *linker) buildGetUserURL(tenant, userID string) (string, error) {
	u, hasURL := l.cfg.TenantURLs[tenant]
	if !hasURL {
		return "", errors.Errorf("unknown tenant %v", tenant)
	}

	userURL, err := url.JoinPath(u, "v1r/users/", userID)
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
