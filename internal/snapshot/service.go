package snapshot

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	aconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/minio/minio-go/v7/pkg/credentials"
	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/config"
	"github.com/shopware/shopware-operator/internal/logging"
	"github.com/shopware/shopware-operator/internal/util"
	"go.uber.org/zap"
)

type SnapshotContext struct {
	TempArchiveDir string
	BackupFile     string
	IncludeDB      bool
	IncludeS3      bool
}

type TarEntry struct {
	Name       string
	Size       int64
	ReadCloser io.ReadCloser
}

// SnapshotService handles backup operations
type SnapshotService struct {
	config *config.SnapshotConfig
}

// NewSnapshotService creates a new snapshot service
func NewSnapshotService(cfg *config.SnapshotConfig) *SnapshotService {
	return &SnapshotService{
		config: cfg,
	}
}

func (s *SnapshotService) backupDatabaseMysqlShell(ctx context.Context, cfg config.DatabaseConfig, dumpFile string) error {
	logger := logging.FromContext(ctx)
	logger.Debugw(
		"Starting database backup with following configuration",
		zap.Any("config", cfg),
		zap.String("dumpFile", dumpFile),
	)

	shell := util.NewMySQLShell(cfg.MysqlShellBinaryPath)
	out, err := shell.Dump(ctx, util.DumpInput{
		DumpFilePath: dumpFile,
		DatabaseSpec: util.DatabaseSpec{
			Host:     cfg.Host,
			Password: []byte(cfg.Password),
			User:     cfg.User,
			Port:     cfg.Port,
			Name:     cfg.Database,
			Version:  cfg.Version,
			SSLMode:  cfg.SSLMode,
			Options:  cfg.Options,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to dump database: %w", err)
	}

	logger.Infow("Database dump output", zap.Any("output", out))
	return nil
}

func (s *SnapshotService) restoreDatabase(ctx context.Context, cfg config.DatabaseConfig, dumpFile string) error {
	logger := logging.FromContext(ctx)
	logger.Debugw(
		"Starting database restore with following configuration",
		zap.Any("config", cfg),
		zap.String("dumpFile", dumpFile),
	)

	shell := util.NewMySQLShell(cfg.MysqlShellBinaryPath)
	err := shell.RestoreDump(ctx, util.RestoreInput{
		DatabaseSpec: util.DatabaseSpec{
			Host:     cfg.Host,
			Password: []byte(cfg.Password),
			User:     cfg.User,
			Port:     cfg.Port,
			Name:     cfg.Database,
			Version:  cfg.Version,
			SSLMode:  cfg.SSLMode,
			Options:  cfg.Options,
		},
		DumpFilePath: dumpFile,
	})
	if err != nil {
		return fmt.Errorf("failed to dump database: %w", err)
	}
	return nil
}

func (s *SnapshotService) backupS3(ctx context.Context, cfg config.S3Config, dumpDirectory string) error {
	logger := logging.FromContext(ctx)
	logger.Debugw("Starting with following s3 configuration", zap.Any("config", cfg))

	cred, err := getAWSKeysWithAsumeRole(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to get AWS credentials: %w", err)
	}
	logger.Debugw("S3 Credentials", zap.Any("credentials", cred))

	err = util.CopyToFilesystem(ctx, cred, dumpDirectory, v1.S3Storage{
		EndpointURL:        cfg.Endpoint,
		PrivateBucketName:  cfg.PrivateBucket,
		PublicBucketName:   cfg.PublicBucket,
		Region:             cfg.Region,
		AccessKeyRef:       v1.SecretRef{},
		SecretAccessKeyRef: v1.SecretRef{},
	})
	if err != nil {
		return fmt.Errorf("failed to copy S3 data to filesystem: %w", err)
	}

	return nil
}

func (s *SnapshotService) restoreS3(ctx context.Context, cfg config.S3Config, directory string) error {
	logger := logging.FromContext(ctx)
	logger.Debugw("Starting with following s3 configuration", zap.Any("config", cfg))

	cred, err := getAWSKeysWithAsumeRole(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to get AWS credentials: %w", err)
	}
	logger.Debugw("S3 Credentials", zap.Any("credentials", cred))

	err = util.RestoreFromFilesystem(ctx, cred, directory, v1.S3Storage{
		EndpointURL:        cfg.Endpoint,
		PrivateBucketName:  cfg.PrivateBucket,
		PublicBucketName:   cfg.PublicBucket,
		Region:             cfg.Region,
		AccessKeyRef:       v1.SecretRef{},
		SecretAccessKeyRef: v1.SecretRef{},
	}, true)
	if err != nil {
		return fmt.Errorf("failed to copy filesystem data to s3: %w", err)
	}

	return nil
}

// This is only used if the secret and access key are not provided by the config.
// We assume that a service role is defined in the pod and use this to assume the role for the backup.
// AWS_ROLE_ARN=arn:aws:iam::471112763676:role/new-update-020561d1-64c1-4964-bc51-c6420e76f5cc
// AWS_WEB_IDENTITY_TOKEN_FILE=/var/run/secrets/eks.amazonaws.com/serviceaccount/token
// AWS_STS_REGIONAL_ENDPOINTS=regional
// AWS_DEFAULT_REGION=eu-central-1
// AWS_REGION=eu-central-1
func getAWSKeysWithAsumeRole(ctx context.Context, cfg config.S3Config) (*credentials.Credentials, error) {
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		logging.FromContext(ctx).
			Infow("AccessKey and SecretAccessKey set, using provided AWS credentials, instead of assuming role")

		return credentials.NewStaticV4(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"",
		), nil
	}

	awsCfg, err := aconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// stsClient := sts.NewFromConfig(awsCfg)
	// if cfg.RoleARN != "" {
	// 	logging.FromContext(ctx).
	// 		Infow("RoleARN is given try to assume role", zap.String("roleARN", cfg.RoleARN))
	// 	assumeRoleOutput, err := stsClient.AssumeRole(ctx, &sts.AssumeRoleInput{
	// 		RoleArn:         aws.String(cfg.RoleARN),
	// 		RoleSessionName: aws.String("shopware-snapshot"),
	// 		DurationSeconds: aws.Int32(1800),
	// 	})
	// 	if err != nil {
	// 		return nil, fmt.Errorf("failed to assume role: %w", err)
	// 	}
	// 	return credentials.NewStaticV4(
	// 		*assumeRoleOutput.Credentials.AccessKeyId,
	// 		*assumeRoleOutput.Credentials.SecretAccessKey,
	// 		*assumeRoleOutput.Credentials.SessionToken,
	// 	), nil
	// }

	creds, err := awsCfg.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve AWS credentials: %w", err)
	}

	return credentials.NewStaticV4(
		creds.AccessKeyID,
		creds.SecretAccessKey,
		creds.SessionToken,
	), nil
}

func parseS3URL(raw string) (bucket, object string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", err
	}
	if u.Scheme != "s3" {
		return "", "", fmt.Errorf("invalid scheme: %s", u.Scheme)
	}

	bucket = u.Host
	object = strings.TrimPrefix(u.Path, "/")

	return bucket, object, nil
}
