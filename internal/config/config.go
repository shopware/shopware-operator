package config

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sethvargo/go-envconfig"
)

type NatsHandler struct {
	Enable          bool   `env:"ENABLE, default=false"`
	NkeyFile        string `env:"NKEY_FILE"`
	CredentialsFile string `env:"CREDENTIALS_FILE"`
	Address         string `env:"ADDRESS, default=nats://localhost:4222"`
	NatsTopic       string `env:"TOPIC, default=shopware-events"`
}

type Config struct {
	NatsHandler NatsHandler `env:",prefix=NATS_"`

	// Metrics and health probe configuration
	MetricsAddr string `env:"METRICS_BIND_ADDRESS, default=0"`
	ProbeAddr   string `env:"HEALTH_PROBE_BIND_ADDRESS, default=:8081"`

	// Logging configuration
	LogLevel  string `env:"LOG_LEVEL, default=info"`
	LogFormat string `env:"LOG_FORMAT, default=json"`

	EnableLeaderElection bool   `env:"LEADER_ELECT, default=true"`
	DisableChecks        bool   `env:"DISABLE_CHECKS, default=false"`
	Namespace            string `env:"NAMESPACE, default=default"`
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
	return strings.ToLower(c.LogLevel) == "debug"
}
