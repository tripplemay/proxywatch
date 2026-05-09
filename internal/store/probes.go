package store

import (
	"database/sql"
	"fmt"
	"time"
)

type Probe struct {
	ID        int64
	TS        time.Time
	Kind      string // "active" | "passive"
	Target    string
	HTTPCode  int
	LatencyMS int
	ExitIP    string
	OK        bool
	RawError  string
}

func (s *Store) InsertProbe(p Probe) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO probes (ts, kind, target, http_code, latency_ms, exit_ip, ok, raw_error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.TS.UnixMilli(), p.Kind, p.Target, p.HTTPCode, p.LatencyMS, p.ExitIP, boolToInt(p.OK), p.RawError,
	)
	if err != nil {
		return 0, fmt.Errorf("insert probe: %w", err)
	}
	return res.LastInsertId()
}

// RecentProbes returns up to limit rows, newest first.
// kind filter is optional ("" = no filter).
func (s *Store) RecentProbes(limit int, kind string) ([]Probe, error) {
	q := `SELECT id, ts, kind, target, http_code, latency_ms, exit_ip, ok, raw_error
	      FROM probes`
	args := []any{}
	if kind != "" {
		q += ` WHERE kind = ?`
		args = append(args, kind)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query probes: %w", err)
	}
	defer rows.Close()

	var out []Probe
	for rows.Next() {
		var (
			p      Probe
			tsMS   int64
			okInt  int
			tgt    sql.NullString
			ip     sql.NullString
			rawErr sql.NullString
		)
		if err := rows.Scan(&p.ID, &tsMS, &p.Kind, &tgt, &p.HTTPCode, &p.LatencyMS, &ip, &okInt, &rawErr); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		p.TS = time.UnixMilli(tsMS)
		p.Target = tgt.String
		p.ExitIP = ip.String
		p.OK = okInt == 1
		p.RawError = rawErr.String
		out = append(out, p)
	}
	return out, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
