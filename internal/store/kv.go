package store

import (
	"database/sql"
	"fmt"
	"time"
)

func (s *Store) SetKV(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO config_kv (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now().UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("set kv: %w", err)
	}
	return nil
}

func (s *Store) GetKV(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM config_kv WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get kv: %w", err)
	}
	return v, true, nil
}
