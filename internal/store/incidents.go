package store

import (
	"database/sql"
	"fmt"
	"time"
)

type Incident struct {
	ID            int64
	StartedAt     time.Time
	EndedAt       time.Time // zero if open
	TriggerReason string    // "passive_4xx" | "active_failure" | "manual"
	InitialState  string
	TerminalState string
	RotationCount int
}

func (s *Store) OpenIncident(in Incident) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO incidents (started_at, trigger_reason, initial_state)
		 VALUES (?, ?, ?)`,
		in.StartedAt.UnixMilli(), in.TriggerReason, in.InitialState,
	)
	if err != nil {
		return 0, fmt.Errorf("open incident: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) CloseIncident(id int64, endedAt time.Time, terminalState string) error {
	_, err := s.db.Exec(
		`UPDATE incidents SET ended_at = ?, terminal_state = ? WHERE id = ?`,
		endedAt.UnixMilli(), terminalState, id,
	)
	return err
}

func (s *Store) IncrementRotationCount(id int64) error {
	_, err := s.db.Exec(`UPDATE incidents SET rotation_count = rotation_count + 1 WHERE id = ?`, id)
	return err
}

func (s *Store) OpenIncidents() ([]Incident, error) {
	return s.queryIncidents(`SELECT id, started_at, ended_at, trigger_reason, initial_state, terminal_state, rotation_count
	                          FROM incidents WHERE ended_at IS NULL ORDER BY id DESC`)
}

func (s *Store) RecentIncidents(limit int) ([]Incident, error) {
	return s.queryIncidents(
		`SELECT id, started_at, ended_at, trigger_reason, initial_state, terminal_state, rotation_count
		 FROM incidents ORDER BY id DESC LIMIT ?`,
		limit,
	)
}

func (s *Store) queryIncidents(q string, args ...any) ([]Incident, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query incidents: %w", err)
	}
	defer rows.Close()

	var out []Incident
	for rows.Next() {
		var (
			in        Incident
			startedMS int64
			endedMS   sql.NullInt64
			term      sql.NullString
			initial   sql.NullString
		)
		if err := rows.Scan(&in.ID, &startedMS, &endedMS, &in.TriggerReason, &initial, &term, &in.RotationCount); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		in.StartedAt = time.UnixMilli(startedMS)
		if endedMS.Valid {
			in.EndedAt = time.UnixMilli(endedMS.Int64)
		}
		in.InitialState = initial.String
		in.TerminalState = term.String
		out = append(out, in)
	}
	return out, rows.Err()
}
