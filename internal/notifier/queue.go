package notifier

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/tripplemay/proxywatch/internal/store"
)

type Queue struct {
	Store      *store.Store
	Telegram   *Telegram
	MaxRetries int // default 10; after this, the entry stops being retried
}

func (q *Queue) maxRetries() int {
	if q.MaxRetries <= 0 {
		return 10
	}
	return q.MaxRetries
}

// DrainOnce drains the pending queue once. Each entry is attempted; failures are recorded.
func (q *Queue) DrainOnce(ctx context.Context) error {
	pending, err := q.Store.PendingNotifications(50)
	if err != nil {
		return err
	}
	for _, n := range pending {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if n.RetryCount >= q.maxRetries() {
			continue
		}
		var sendErr error
		if n.Buttons != "" {
			var buttons []InlineButton
			if jerr := json.Unmarshal([]byte(n.Buttons), &buttons); jerr != nil {
				// malformed JSON — fall back to plain Send so we don't block the queue
				sendErr = q.Telegram.Send(n.Text)
			} else if len(buttons) > 0 {
				sendErr = q.Telegram.SendWithButtons(n.Text, buttons)
			} else {
				sendErr = q.Telegram.Send(n.Text)
			}
		} else {
			sendErr = q.Telegram.Send(n.Text)
		}
		if sendErr != nil {
			_ = q.Store.RecordNotificationFailure(n.ID, sendErr.Error())
			continue
		}
		_ = q.Store.MarkNotificationSent(n.ID, time.Now())
	}
	return nil
}

// Loop drains the queue at interval until ctx is cancelled.
func (q *Queue) Loop(ctx context.Context, interval time.Duration, log *slog.Logger) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		if err := q.DrainOnce(ctx); err != nil && ctx.Err() == nil {
			log.Error("notifier drain", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
