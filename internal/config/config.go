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

type DatabaseConfig struct {
	MysqlShellBinaryPath string `env:"MYSQL_SHELL_BINARY_PATH, default=/opt/mysqlsh/bin/mysqlsh"`
	MysqlDumpBinaryPath  string `env:"MYSQL_DUMP_BINARY_PATH, default=mysqldump"`
	Host                 string `env:"HOST"`
	Port                 int32  `env:"PORT, default=3306"`
	User                 string `env:"USER"`
	Password             string `env:"PASSWORD"`
	Database             string `env:"DATABASE"`
	Version              string `env:"VERSION"`
	Options              string `env:"OPTIONS"`
	SSLMode              string `env:"SSL_MODE, default=disable"`
}

type S3Config struct {
	// Specified when running in an EKS cluster with IAM roles for service accounts
	RoleARN              string `env:"ROLE_ARN"`
	WebIdentityTokenFile string `env:"WEB_IDENTITY_TOKEN_FILE"`
	STSReginalEndpoints  string `env:"STS_REGIONAL_ENDPOINTS, default=regional"`
	DefaultRegion        string `env:"DEFAULT_REGION, default=eu-central-1"`
	Region               string `env:"REGION, default=eu-central-1"`

	Endpoint        string `env:"ENDPOINT, default=s3.eu-central-1.amazonaws.com"`
	AccessKeyID     string `env:"ACCESS_KEY_ID"`
	SecretAccessKey string `env:"SECRET_ACCESS_KEY"`

	PrivateBucket string `env:"PRIVATE_BUCKET"`
	PublicBucket  string `env:"PUBLIC_BUCKET"`
}

type SnapshotConfig struct {
	Config
	Database      DatabaseConfig `env:",prefix=DB_"`
	S3            S3Config       `env:",prefix=AWS_"`
	MetaStoreJson string         `env:"META_STORE_STATE"`
}

type StoreConfig struct {
	Config
	NatsHandler NatsHandler `env:",prefix=NATS_"`

	// Metrics and health probe configuration
	MetricsAddr string `env:"METRICS_BIND_ADDRESS, default=0"`
	ProbeAddr   string `env:"HEALTH_PROBE_BIND_ADDRESS, default=:8081"`

	EnableLeaderElection bool   `env:"LEADER_ELECT, default=true"`
	DisableChecks        bool   `env:"DISABLE_CHECKS, default=false"`
	Namespace            string `env:"NAMESPACE, default=default"`
}

type Config struct {
	LogLevel  string `env:"LOG_LEVEL, default=info"`
	LogFormat string `env:"LOG_FORMAT, default=json"`
}

func LoadStoreConfig(ctx context.Context) (*StoreConfig, error) {
	cfg := &StoreConfig{}

	// Load configuration from environment variables
	if err := envconfig.Process(ctx, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func LoadSnapshotConfig(ctx context.Context) (*SnapshotConfig, error) {
	cfg := &SnapshotConfig{}

	// Load configuration from environment variables
	if err := envconfig.Process(ctx, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c StoreConfig) String() string {
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
