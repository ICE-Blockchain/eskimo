// SPDX-License-Identifier: ice License 1.0

package users

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
	stdlibtime "time"

	"github.com/goccy/go-json"
	"github.com/imroc/req/v3"
	"github.com/pkg/errors"

	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/time"
)

func (r *repository) GetUserGrowth(ctx context.Context, days uint64, tz *stdlibtime.Location) (*UserGrowthStatistics, error) {
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "context failed")
	}
	now := time.Now()
	keys := r.generateUserGrowthKeys(now, days)
	values, err := r.getGlobalValues(ctx, keys...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to getGlobalValues for keys:%#v", keys)
	}
	totalActiveUsers, err := r.getAdoptionTotalActiveUsersValue(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to getAdoptionTotalActiveUsersValue")
	}

	return r.aggregateGlobalValuesToGrowth(days, now, values, keys, tz, totalActiveUsers), nil
}

//nolint:funlen // .
func (r *repository) getAdoptionTotalActiveUsersValue(ctx context.Context) (totalActiveUsers uint64, err error) {
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
				log.Error(errors.Wrap(err, "failed to fetch adoption, retrying...")) //nolint:revive // .
			} else {
				log.Error(errors.Errorf("failed to fetch users adoption with status code:%v, retrying...", resp.GetStatusCode())) //nolint:revive // .
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
		Get(r.cfg.AdoptionURL)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to get fetch `%v`", r.cfg.AdoptionURL)
	}
	data, err2 := resp.ToBytes()
	if err2 != nil {
		return 0, errors.Wrapf(err2, "failed to read body of `%v`", r.cfg.AdoptionURL)
	}
	var adoption struct {
		TotalActiveUsers uint64 `json:"totalActiveUsers" example:"11"`
	}
	if err = json.UnmarshalContext(ctx, data, &adoption); err != nil {
		return 0, errors.Wrapf(err, "failed to unmarshal into %#v, data: %v", adoption, string(data))
	}

	return adoption.TotalActiveUsers, nil
}

func (r *repository) generateUserGrowthKeys(now *time.Time, days uint64) []string {
	const totalAndActiveFactor = 2
	keys := make([]string, 0, totalAndActiveFactor*days+1)
	keys = append(keys, totalUsersGlobalKey)
	for day := stdlibtime.Duration(0); day < stdlibtime.Duration(days); day++ { //nolint:gosec // .
		currentDay := now.Add(-1 * day * r.cfg.GlobalAggregationInterval.Parent)
		keys = append(append(keys, r.totalUsersGlobalParentKey(&currentDay)), r.totalActiveUsersGlobalChildrenKeys(&currentDay)...)
	}

	return keys
}

//nolint:gocognit,revive,funlen // .
func (r *repository) aggregateGlobalValuesToGrowth(
	days uint64, now *time.Time,
	values []*GlobalUnsigned,
	keys []string,
	tz *stdlibtime.Location,
	totalActiveUsers uint64,
) *UserGrowthStatistics {
	nsSinceParentIntervalZeroValue := r.cfg.nanosSinceGlobalAggregationIntervalParentZeroValue(now)
	stats := make([]*UserCountTimeSeriesDataPoint, days, days) //nolint:gosimple // .
	var activeMaxPerParent, dayIdx uint64
	nowInTZ := time.New(now.In(tz))
	for ix, key := range keys {
		if ix == 0 {
			continue
		}
		var val uint64
		for _, row := range values {
			if key == row.Key {
				val = row.Value

				break
			}
		}
		if strings.HasPrefix(key, totalUsersGlobalKey) { //nolint:nestif // .
			if dayIdx > 0 {
				stats[dayIdx-1].UserCount.Active = activeMaxPerParent
			}
			if stats[dayIdx] == nil {
				stats[dayIdx] = new(UserCountTimeSeriesDataPoint)
			}
			stats[dayIdx].UserCount.Total = val
			if stats[dayIdx].Date == nil {
				if dayIdx == 0 {
					stats[dayIdx].Date = now
				} else {
					nowInTzWithUTC := time.New(stdlibtime.Date(
						nowInTZ.Year(), nowInTZ.Month(), nowInTZ.Day(),
						nowInTZ.Hour(), nowInTZ.Minute(), nowInTZ.Second(), nowInTZ.Nanosecond(),
						stdlibtime.UTC,
					))
					if math.Abs(float64(nowInTzWithUTC.Sub(*now.Time))) > float64(r.cfg.GlobalAggregationInterval.Parent) {
						nowInTzWithUTC = now
					}
					fullNegativeDayDuration := (-1) * r.cfg.GlobalAggregationInterval.Parent * stdlibtime.Duration(dayIdx-1) //nolint:gosec // .
					stats[dayIdx].Date = time.New(nowInTzWithUTC.Add(fullNegativeDayDuration).Add(-nsSinceParentIntervalZeroValue - 1))
					if stats[dayIdx].Date.Truncate(r.cfg.GlobalAggregationInterval.Parent).Equal(stats[dayIdx-1].Date.Truncate(r.cfg.GlobalAggregationInterval.Parent)) {
						stats[dayIdx].Date = time.New(stats[dayIdx].Date.Add(-r.cfg.GlobalAggregationInterval.Parent))
					}
				}
			}
			activeMaxPerParent = 0
			dayIdx++
		} else if val > activeMaxPerParent {
			activeMaxPerParent = val
		}
	}
	stats[dayIdx-1].UserCount.Active = activeMaxPerParent
	stats[0].Total = values[0].Value

	return &UserGrowthStatistics{
		TimeSeries: stats,
		UserCount: UserCount{
			Active: totalActiveUsers,
			Total:  values[0].Value,
		},
	}
}

func (r *repository) getGlobalValues(ctx context.Context, keys ...string) ([]*GlobalUnsigned, error) {
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "context failed")
	}
	placeholders := make([]string, 0, len(keys))
	params := make([]any, len(keys)+1) //nolint:makezero // .
	params[0] = ""
	for i, key := range keys {
		params[i+1] = key
		placeholders = append(placeholders, fmt.Sprintf("$%v", i+2)) //nolint:gomnd // Not a magic number.
		params[0] = fmt.Sprintf("%v,%v", params[0], key)
	}
	sql := fmt.Sprintf(`SELECT *
						FROM global
						WHERE key in (%v)
						ORDER BY POSITION(key in $1)`, strings.Join(placeholders, ","))
	vals, err := storage.Select[GlobalUnsigned](ctx, r.db, sql, params...)

	return vals, errors.Wrapf(err, "failed to select global vals for keys:%#v", keys)
}

func (r *repository) updateTotalUsersCount(ctx context.Context, usr *UserSnapshot) error {
	if (usr.Before == nil || usr.Before.ID == "") && usr.User != nil && usr.User.ID != "" {
		if true || usr.User.isFirstMiningAfterHumanVerification(r) { //nolint:revive // .
			return r.incrementOrDecrementTotalUsers(ctx, time.Now(), true)
		}
	}

	if (usr.User == nil || usr.User.ID == "") && usr.Before != nil && usr.Before.ID != "" {
		if true || usr.Before.hadAtLeastAMiningAfterHumanVerification(r) { //nolint:revive // .
			return r.incrementOrDecrementTotalUsers(ctx, time.Now(), false)
		}
	}

	return nil
}

//nolint:revive,funlen // .
func (r *repository) incrementOrDecrementTotalUsers(ctx context.Context, date *time.Time, increment bool) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	operation := "+"
	if !increment {
		operation = "-"
	}
	params := []any{totalUsersGlobalKey, r.totalUsersGlobalParentKey(date.Time), r.totalUsersGlobalChildKey(date.Time)}
	sqlParams := make([]string, 0, len(params))
	for idx := range params {
		if idx > 0 {
			sqlParams = append(sqlParams, fmt.Sprintf(
				"($%[1]v,(select GREATEST(total.value %[2]v 1,0) FROM global total WHERE total.key = '%[3]v'))",
				idx+1, operation, params[0]))
		} else {
			sqlParams = append(sqlParams, fmt.Sprintf("($%v,1)", idx+1))
		}
	}
	sql := fmt.Sprintf(`INSERT INTO global (key, value) VALUES %[2]v
								ON CONFLICT (key) DO UPDATE    
						SET value = (
							select GREATEST(total.value %[1]v 1,0) FROM global total WHERE total.key = '%[3]v' FOR UPDATE
						)`, operation, strings.Join(sqlParams, ","), params[0])
	if _, err := storage.Exec(ctx, r.db, sql, params...); err != nil && !storage.IsErr(err, storage.ErrNotFound) {
		return errors.Wrapf(err, "failed to update global.value to global.value%v1 of key='%v', for params:%#v ", operation, totalUsersGlobalKey, params)
	} else if err != nil && errors.Is(err, storage.ErrSerializationFailure) {
		stdlibtime.Sleep(10 * stdlibtime.Millisecond) //nolint:mnd,gomnd // Not a magic number.

		return r.incrementOrDecrementTotalUsers(ctx, date, increment)
	}
	keys := make([]string, 0, len(params))
	for _, v := range params {
		keys = append(keys, v.(string)) //nolint:forcetypeassert,errcheck // We know for sure.
	}

	return errors.Wrapf(r.notifyGlobalValueUpdateMessage(ctx, keys...), "failed to notifyGlobalValueUpdateMessage, keys:%#v", keys)
}

func (r *repository) incrementTotalActiveUsersCount(ctx context.Context, ms *miningSession) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	keys := ms.detectIncrTotalActiveUsersKeys(r)
	if len(keys) == 0 {
		return nil
	}
	sqlParams := make([]string, 0, len(keys))
	for idx := range keys {
		sqlParams = append(sqlParams, fmt.Sprintf("($%v,1)", idx+1))
	}
	sql := fmt.Sprintf(`
				INSERT INTO global (key, value) VALUES 
					%v
				ON CONFLICT (key) DO UPDATE   
						SET value = global.value + 1`, strings.Join(sqlParams, ","))

	_, err := storage.Exec(ctx, r.db, sql, keys...)
	if err != nil && errors.Is(err, storage.ErrSerializationFailure) {
		stdlibtime.Sleep(10 * stdlibtime.Millisecond) //nolint:mnd,gomnd // Not a magic number.

		return r.incrementTotalActiveUsersCount(ctx, ms)
	}
	if err != nil && !storage.IsErr(err, storage.ErrNotFound) {
		return errors.Wrapf(err, "failed to update global.value to global.value+1 for keys:%#v", keys)
	}

	return nil
}

func (ms *miningSession) detectIncrTotalActiveUsersKeys(repo *repository) []any {
	keys := make([]any, 0)
	start, end := ms.EndedAt.Add(-ms.Extension), *ms.EndedAt.Time
	if !ms.LastNaturalMiningStartedAt.Equal(*ms.StartedAt.Time) ||
		(!ms.PreviouslyEndedAt.IsNil() &&
			repo.totalActiveUsersGlobalChildKey(ms.StartedAt.Time) == repo.totalActiveUsersGlobalChildKey(ms.PreviouslyEndedAt.Time)) {
		start = start.Add(repo.cfg.GlobalAggregationInterval.Child)
	}
	start = start.Truncate(repo.cfg.GlobalAggregationInterval.Child)
	end = end.Truncate(repo.cfg.GlobalAggregationInterval.Child)
	for start.Before(end) {
		keys = append(keys, repo.totalActiveUsersGlobalChildKey(&start))
		start = start.Add(repo.cfg.GlobalAggregationInterval.Child)
	}
	if ms.PreviouslyEndedAt.IsNil() || repo.totalActiveUsersGlobalChildKey(&end) != repo.totalActiveUsersGlobalChildKey(ms.PreviouslyEndedAt.Time) {
		keys = append(keys, repo.totalActiveUsersGlobalChildKey(&end))
	}

	return keys
}

func (r *repository) notifyGlobalValueUpdateMessage(ctx context.Context, keys ...string) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	values, err := r.getGlobalValues(ctx, keys...)
	if err != nil {
		return errors.Wrapf(err, "failed to get global values for keys:%#v", keys)
	}

	return errors.Wrapf(sendMessagesConcurrently(ctx, r.sendGlobalValueMessage, values),
		"failed to sendMessagesConcurrently[sendGlobalValueMessage] for %#v", values)
}

func (r *repository) sendGlobalValueMessage(ctx context.Context, globalVal *GlobalUnsigned) error {
	valueBytes, err := json.MarshalContext(ctx, globalVal)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal %#v", globalVal)
	}

	msg := &messagebroker.Message{
		Headers: map[string]string{"producer": "eskimo"},
		Key:     globalVal.Key,
		Topic:   r.cfg.MessageBroker.Topics[3].Name,
		Value:   valueBytes,
	}

	responder := make(chan error, 1)
	defer close(responder)
	r.mb.SendMessage(ctx, msg, responder)

	return errors.Wrapf(<-responder, "failed to send `%v` message to broker, msg:%#v", msg.Topic, globalVal)
}

func (r *repository) totalUsersGlobalParentKey(date *stdlibtime.Time) string {
	return fmt.Sprintf("%v_%v", totalUsersGlobalKey, date.Format(r.cfg.globalAggregationIntervalParentDateFormat()))
}

func (r *repository) totalUsersGlobalChildKey(date *stdlibtime.Time) string {
	return fmt.Sprintf("%v_%v", totalUsersGlobalKey, date.Format(r.cfg.globalAggregationIntervalChildDateFormat()))
}

func (r *repository) totalActiveUsersGlobalChildKey(date *stdlibtime.Time) string {
	return fmt.Sprintf("%v_%v", totalActiveUsersGlobalKey, date.Format(r.cfg.globalAggregationIntervalChildDateFormat()))
}

func (r *repository) totalActiveUsersGlobalChildrenKeys(date *stdlibtime.Time) []string {
	parent := date.Truncate(r.cfg.GlobalAggregationInterval.Parent)
	current := parent
	keys := make([]string, 0)
	for current.Before(parent.Add(r.cfg.GlobalAggregationInterval.Parent)) {
		keys = append(keys, fmt.Sprintf("%v_%v", totalActiveUsersGlobalKey, current.Format(r.cfg.globalAggregationIntervalChildDateFormat())))
		current = current.Add(r.cfg.GlobalAggregationInterval.Child)
	}

	return keys
}

func NanosSinceMidnight(now *time.Time) stdlibtime.Duration {
	return stdlibtime.Duration(now.Nanosecond()) +
		stdlibtime.Duration(now.Second())*stdlibtime.Second +
		stdlibtime.Duration(now.Minute())*stdlibtime.Minute +
		stdlibtime.Duration(now.Hour())*stdlibtime.Hour
}

func authorization(ctx context.Context) (authorization string) {
	authorization, _ = ctx.Value(authorizationCtxValueKey).(string) //nolint:errcheck // Not needed.

	return
}
