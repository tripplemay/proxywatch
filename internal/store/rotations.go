package store

import (
	"database/sql"
	"fmt"
	"time"
)

type Rotation struct {
	ID              int64
	IncidentID      int64
	StartedAt       time.Time
	EndedAt         time.Time // zero if still in flight
	OldIP           string
	NewIP           string
	DetectionMethod string // "auto" | "manual_button"
	OK              bool
	Error           string
}

func (s *Store) InsertRotation(r Rotation) (int64, error) {
	var endedMS sql.NullInt64
	if !r.EndedAt.IsZero() {
		endedMS.Valid = true
		endedMS.Int64 = r.EndedAt.UnixMilli()
	}
	res, err := s.db.Exec(
		`INSERT INTO rotations (incident_id, started_at, ended_at, old_ip, new_ip, detection_method, ok, error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.IncidentID, r.StartedAt.UnixMilli(), endedMS,
		r.OldIP, r.NewIP, r.DetectionMethod, boolToInt(r.OK), r.Error,
	)
	if err != nil {
		return 0, fmt.Errorf("insert rotation: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) RecentRotations(limit int) ([]Rotation, error) {
	rows, err := s.db.Query(
		`SELECT id, incident_id, started_at, ended_at, old_ip, new_ip, detection_method, ok, error
		 FROM rotations ORDER BY id DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query rotations: %w", err)
	}
	defer rows.Close()

	var out []Rotation
	for rows.Next() {
		var (
			r       Rotation
			startMS int64
			endMS   sql.NullInt64
			oldIP   sql.NullString
			newIP   sql.NullString
			method  sql.NullString
			okInt   sql.NullInt64
			errStr  sql.NullString
		)
		if err := rows.Scan(&r.ID, &r.IncidentID, &startMS, &endMS, &oldIP, &newIP, &method, &okInt, &errStr); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		r.StartedAt = time.UnixMilli(startMS)
		if endMS.Valid {
			r.EndedAt = time.UnixMilli(endMS.Int64)
		}
		r.OldIP = oldIP.String
		r.NewIP = newIP.String
		r.DetectionMethod = method.String
		r.OK = okInt.Valid && okInt.Int64 == 1
		r.Error = errStr.String
		out = append(out, r)
	}
	return out, rows.Err()
}
