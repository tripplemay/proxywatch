package store

import (
	"database/sql"
	"fmt"
	"time"
)

type Notification struct {
	ID         int64
	TS         time.Time
	IncidentID int64 // 0 if not associated
	Level      string
	Text       string
	Buttons    string    // raw JSON of []InlineButton; empty = no buttons
	SentAt     time.Time // zero if pending
	Error      string
	RetryCount int
}

func (s *Store) EnqueueNotification(n Notification) (int64, error) {
	var incID sql.NullInt64
	if n.IncidentID > 0 {
		incID.Valid = true
		incID.Int64 = n.IncidentID
	}
	var btn sql.NullString
	if n.Buttons != "" {
		btn.Valid = true
		btn.String = n.Buttons
	}
	res, err := s.db.Exec(
		`INSERT INTO notifications (ts, incident_id, level, text, buttons) VALUES (?, ?, ?, ?, ?)`,
		n.TS.UnixMilli(), incID, n.Level, n.Text, btn,
	)
	if err != nil {
		return 0, fmt.Errorf("enqueue: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) PendingNotifications(limit int) ([]Notification, error) {
	rows, err := s.db.Query(
		`SELECT id, ts, incident_id, level, text, buttons, error, retry_count
		 FROM notifications WHERE sent_at IS NULL ORDER BY id ASC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("pending: %w", err)
	}
	defer rows.Close()

	var out []Notification
	for rows.Next() {
		var (
			n      Notification
			tsMS   int64
			incID  sql.NullInt64
			btn    sql.NullString
			errStr sql.NullString
		)
		if err := rows.Scan(&n.ID, &tsMS, &incID, &n.Level, &n.Text, &btn, &errStr, &n.RetryCount); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		n.TS = time.UnixMilli(tsMS)
		if incID.Valid {
			n.IncidentID = incID.Int64
		}
		n.Buttons = btn.String
		n.Error = errStr.String
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) MarkNotificationSent(id int64, at time.Time) error {
	_, err := s.db.Exec(`UPDATE notifications SET sent_at = ?, error = NULL WHERE id = ?`, at.UnixMilli(), id)
	return err
}

func (s *Store) RecordNotificationFailure(id int64, errMsg string) error {
	_, err := s.db.Exec(
		`UPDATE notifications SET retry_count = retry_count + 1, error = ? WHERE id = ?`,
		errMsg, id,
	)
	return err
}
