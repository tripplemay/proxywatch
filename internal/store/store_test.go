package store

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesAllTables(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	want := []string{"probes", "incidents", "rotations", "notifications", "config_kv"}
	for _, name := range want {
		var count int
		err := s.DB().QueryRow(
			`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`,
			name,
		).Scan(&count)
		if err != nil {
			t.Fatalf("query for %s: %v", name, err)
		}
		if count != 1 {
			t.Errorf("table %s: got %d rows, want 1", name, count)
		}
	}
}
