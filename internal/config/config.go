package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen      string `yaml:"listen"`
	DataDir     string `yaml:"data_dir"`
	CPAProxyURL string `yaml:"cpa_proxy_url"`
	CPALogDir   string `yaml:"cpa_log_dir"`

	ActiveProbe ActiveProbeConfig `yaml:"active_probe"`

	// AuthKey comes from env, not yaml
	AuthKey string `yaml:"-"`
}

type ActiveProbeConfig struct {
	Target          string `yaml:"target"`
	IntervalSeconds int    `yaml:"interval_seconds"`
	TimeoutSeconds  int    `yaml:"timeout_seconds"`
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	c := &Config{
		Listen:  ":18318",
		DataDir: "/data",
		ActiveProbe: ActiveProbeConfig{
			Target:          "https://api.openai.com/v1/models",
			IntervalSeconds: 60,
			TimeoutSeconds:  15,
		},
	}
	if err := yaml.Unmarshal(raw, c); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	c.AuthKey = os.Getenv("PROXYWATCH_KEY")
	if c.AuthKey == "" {
		return nil, fmt.Errorf("PROXYWATCH_KEY environment variable is required")
	}
	return c, nil
}
