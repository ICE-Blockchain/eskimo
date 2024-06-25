// SPDX-License-Identifier: ice License 1.0

package users

import (
	"context"
	"github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/time"
	"math/rand"
	stdlibtime "time"

	"github.com/goccy/go-json"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
)

func (s *userSnapshotSource) Process(ctx context.Context, msg *messagebroker.Message) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline while processing message")
	}
	if len(msg.Value) == 0 {
		return nil
	}
	usr := new(UserSnapshot)
	if err := json.UnmarshalContext(ctx, msg.Value, usr); err != nil {
		return errors.Wrapf(err, "process: cannot unmarshall %v into %#v", string(msg.Value), usr)
	}

	return multierror.Append( //nolint:wrapcheck // Not needed.
		errors.Wrap(s.updateTotalUsersCount(ctx, usr, msg.Timestamp), "failed to updateTotalUsersCount"),
		errors.Wrap(s.updateTotalUsersPerCountryCount(ctx, usr, msg.Timestamp), "failed to updateTotalUsersPerCountryCount"),
		errors.Wrap(s.updateReferralCount(ctx, msg.Timestamp, usr), "failed to updateReferralCount"),
	).ErrorOrNil()
}

func (r *repository) sendTombstonedUserMessage(ctx context.Context, userID string) error {
	msg := &messagebroker.Message{
		Headers: map[string]string{"producer": "eskimo"},
		Key:     userID,
		Topic:   r.cfg.MessageBroker.Topics[1].Name,
	}
	responder := make(chan error, 1)
	defer close(responder)
	r.mb.SendMessage(ctx, msg, responder)

	return errors.Wrapf(<-responder, "failed to send tombstoned user message to broker")
}

func (r *repository) sendUserSnapshotMessage(ctx context.Context, user *UserSnapshot) error {
	valueBytes, err := json.MarshalContext(ctx, user)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal %#v", user)
	}

	var key string
	if user.User == nil {
		key = user.Before.ID
	} else {
		key = user.ID
	}

	msg := &messagebroker.Message{
		Headers: map[string]string{"producer": "eskimo"},
		Key:     key,
		Topic:   r.cfg.MessageBroker.Topics[1].Name,
		Value:   valueBytes,
	}

	responder := make(chan error, 1)
	defer close(responder)
	r.mb.SendMessage(ctx, msg, responder)

	return errors.Wrapf(<-responder, "failed to send user snapshot message to broker")
}

func (p *processor) startOldProcessedUsersCleaner(ctx context.Context) {
	ticker := stdlibtime.NewTicker(stdlibtime.Duration(1+rand.Intn(24)) * stdlibtime.Minute) //nolint:gosec,gomnd // Not an  issue.
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			const deadline = 30 * stdlibtime.Second
			reqCtx, cancel := context.WithTimeout(ctx, deadline)
			log.Error(errors.Wrap(p.deleteOldProcessedUsers(reqCtx), "failed to deleteOldProcessedUsers"))
			cancel()
		case <-ctx.Done():
			return
		}
	}
}

func (p *processor) deleteOldProcessedUsers(ctx context.Context) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	sql := `DELETE FROM processed_users WHERE processed_at < $1`
	if _, err := storage.Exec(ctx, p.db, sql, time.Now().Add(-24*stdlibtime.Hour)); err != nil {
		return errors.Wrap(err, "failed to delete old data from processed_users")
	}

	return nil
}
