package config

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sethvargo/go-envconfig"
)

type Config struct {
	Namespace            string `env:"NAMESPACE, default=default"`
	MetricsAddr          string `env:"METRICS_BIND_ADDRESS, default=:8080"`
	ProbeAddr            string `env:"HEALTH_PROBE_BIND_ADDRESS, default=:8081"`
	LogLevel             string `env:"LOG_LEVEL, default=Info"`
	LogFormat            string `env:"LOG_FORMAT, default=json"`
	DisableChecks        bool   `env:"DISABLE_CHECKS, default=false"`
	EnableLeaderElection bool   `env:"LEADER_ELECT, default=false"`
}

func Load(ctx context.Context) (*Config, error) {
	cfg := &Config{}

	// Load configuration from environment variables
	if err := envconfig.Process(ctx, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c Config) String() string {
	var out []byte
	var err error
	if c.IsDebug() {
		out, err = json.MarshalIndent(c, "", "  ")
	} else {
		out, err = json.Marshal(c)
	}

	if err != nil {
		return fmt.Sprintf("Error marshalling config to string: %s", err.Error())
	}
	return string(out)
}

func (c Config) IsDebug() bool {
	return strings.ToLower(c.LogFormat) == "debug"
}
