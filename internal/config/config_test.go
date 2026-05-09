package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromYAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "p.yaml")
	yaml := `
listen: ":18318"
data_dir: "/data"
cpa_proxy_url: "socks5h://u:p@host:1111"
cpa_log_dir: "/cpa-logs"
active_probe:
  target: "https://api.openai.com/v1/models"
  interval_seconds: 60
  timeout_seconds: 15
`
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROXYWATCH_KEY", "secret")

	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Listen != ":18318" {
		t.Errorf("Listen=%q, want :18318", c.Listen)
	}
	if c.AuthKey != "secret" {
		t.Errorf("AuthKey=%q, want secret", c.AuthKey)
	}
	if c.ActiveProbe.IntervalSeconds != 60 {
		t.Errorf("IntervalSeconds=%d, want 60", c.ActiveProbe.IntervalSeconds)
	}
}

func TestLoadRejectsMissingAuthKey(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "p.yaml")
	os.WriteFile(p, []byte("listen: \":18318\"\ndata_dir: \"/d\""), 0o644)
	os.Unsetenv("PROXYWATCH_KEY")

	_, err := Load(p)
	if err == nil {
		t.Error("expected error for missing PROXYWATCH_KEY, got nil")
	}
}
