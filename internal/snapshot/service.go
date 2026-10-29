package snapshot

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
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
	// TempSizeBytes is the allocated temp volume cap from --temp-size (default 100Gi).
	TempSizeBytes int64
	Labels        map[string]string
}

type TarEntry struct {
	Name       string
	Size       int64
	ReadCloser io.ReadCloser
}

// SnapshotService handles backup operations
type SnapshotService struct {
	config  *config.SnapshotConfig
	metrics MetricStatisticsAPI
}

func NewSnapshotService(cfg *config.SnapshotConfig, metrics MetricStatisticsAPI) *SnapshotService {
	return &SnapshotService{
		config:  cfg,
		metrics: metrics,
	}
}

func (s *SnapshotService) restoreS3(ctx context.Context, cfg config.S3Config, directory string) error {
	cred, err := getAWSKeysWithAsumeRole(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to get AWS credentials: %w", err)
	}

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
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" && cfg.SessionToken == "" {
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

	logging.FromContext(ctx).Infow("Using refreshable AWS default credential chain",
		zap.Bool("canExpire", creds.CanExpire),
		zap.Time("expiresAt", creds.Expires),
		zap.Bool("hasSessionToken", creds.SessionToken != ""),
	)

	return credentials.New(&awsSDKRefreshProvider{provider: awsCfg.Credentials}), nil
}

// awsSDKRefreshProvider lets minio ask the AWS SDK for creds on each request.
// The SDK caches until expiry and then refreshes IRSA / web-identity / IMDS tokens.
type awsSDKRefreshProvider struct {
	provider aws.CredentialsProvider
}

func (p *awsSDKRefreshProvider) Retrieve() (credentials.Value, error) {
	return p.RetrieveWithCredContext(nil)
}

func (p *awsSDKRefreshProvider) RetrieveWithCredContext(_ *credentials.CredContext) (credentials.Value, error) {
	creds, err := p.provider.Retrieve(context.Background())
	if err != nil {
		return credentials.Value{}, err
	}
	return credentials.Value{
		AccessKeyID:     creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:    creds.SessionToken,
		SignerType:      credentials.SignatureV4,
		Expiration:      creds.Expires,
	}, nil
}

func (p *awsSDKRefreshProvider) IsExpired() bool {
	return true
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
