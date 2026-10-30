package snapshot

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	aconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/shopware/shopware-operator/internal/config"
	"github.com/shopware/shopware-operator/internal/logging"
	"go.uber.org/zap"
)

const (
	s3SizeLookback      = 7 * 24 * time.Hour
	s3SizePeriodSeconds = 86400
)

// StandardStorage only: one GetMetricStatistics call per bucket.
// Requires cloudwatch:GetMetricStatistics.
var s3BucketSizeStorageTypes = []string{"StandardStorage"}

// MetricStatisticsAPI is the CloudWatch surface used by the S3 size precheck.
type MetricStatisticsAPI interface {
	GetMetricStatistics(ctx context.Context, params *cloudwatch.GetMetricStatisticsInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricStatisticsOutput, error)
}

func NewCloudWatchMetrics(ctx context.Context, cfg *config.SnapshotConfig) (MetricStatisticsAPI, error) {
	awsCfg, err := aconfig.LoadDefaultConfig(ctx, aconfig.WithRegion(cfg.S3.Region))
	if err != nil {
		return nil, err
	}
	return cloudwatch.NewFromConfig(awsCfg), nil
}

func (s *SnapshotService) precheckTempStorage(ctx context.Context, cfg *config.SnapshotConfig, snapshotCtx *SnapshotContext) error {
	if !snapshotCtx.IncludeS3 {
		return nil
	}
	if snapshotCtx.TempSizeBytes <= 0 {
		return fmt.Errorf("s3 size precheck: temp size must be positive")
	}

	if s.metrics == nil {
		return fmt.Errorf("s3 size precheck: metrics client is required")
	}

	logger := logging.FromContext(ctx)
	client := s.metrics

	end := time.Now().UTC()
	start := end.Add(-s3SizeLookback)
	var total int64
	var anyDatapoints bool

	for _, bucket := range []string{cfg.S3.PrivateBucket, cfg.S3.PublicBucket} {
		if bucket == "" {
			continue
		}
		size, datapoints, err := maxBucketSize(ctx, client, bucket, start, end)
		if err != nil {
			return fmt.Errorf("s3 size precheck: bucket %s: %w", bucket, err)
		}
		if datapoints == 0 {
			logger.Warnw("S3 size precheck: no CloudWatch BucketSizeBytes datapoints, skipping bucket",
				zap.String("bucket", bucket),
			)
			continue
		}
		anyDatapoints = true
		total += size
		logger.Infow("S3 bucket size from CloudWatch",
			zap.String("bucket", bucket),
			zap.Int64("maxBytes", size),
			zap.Int("datapoints", datapoints),
		)
	}

	if !anyDatapoints {
		logger.Warnw("S3 size precheck: no CloudWatch datapoints, continuing without size estimate")
		return nil
	}

	required := requiredTempBytes(total)
	logger.Infow("Snapshot temp storage precheck",
		zap.Int64("s3MaxBytes", total),
		zap.Int64("requiredBytes", required),
		zap.Int64("tempSizeBytes", snapshotCtx.TempSizeBytes),
	)
	if required > snapshotCtx.TempSizeBytes {
		return fmt.Errorf("%w: estimated %d bytes (S3 7-day max + archive) exceeds temp size %d bytes",
			ErrInsufficientTempSpace, required, snapshotCtx.TempSizeBytes)
	}
	return nil
}

func maxBucketSize(ctx context.Context, client MetricStatisticsAPI, bucket string, start, end time.Time) (int64, int, error) {
	var sum float64
	var typesWithData int
	totalPoints := 0
	for _, storageType := range s3BucketSizeStorageTypes {
		maxSize, n, err := maxStorageTypeSize(ctx, client, bucket, storageType, start, end)
		if err != nil {
			return 0, 0, err
		}
		if n == 0 {
			continue
		}
		sum += maxSize
		typesWithData++
		totalPoints += n
	}
	if typesWithData == 0 {
		return 0, 0, nil
	}
	return int64(sum), totalPoints, nil
}

func maxStorageTypeSize(ctx context.Context, client MetricStatisticsAPI, bucket, storageType string, start, end time.Time) (float64, int, error) {
	out, err := client.GetMetricStatistics(ctx, &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/S3"),
		MetricName: aws.String("BucketSizeBytes"),
		Dimensions: []cwtypes.Dimension{
			{Name: aws.String("BucketName"), Value: aws.String(bucket)},
			{Name: aws.String("StorageType"), Value: aws.String(storageType)},
		},
		StartTime:  aws.Time(start),
		EndTime:    aws.Time(end),
		Period:     aws.Int32(s3SizePeriodSeconds),
		Statistics: []cwtypes.Statistic{cwtypes.StatisticMaximum},
	})
	if err != nil {
		return 0, 0, err
	}
	if len(out.Datapoints) == 0 {
		return 0, 0, nil
	}
	var maxSize float64
	var found bool
	for _, dp := range out.Datapoints {
		if dp.Maximum == nil {
			continue
		}
		if !found || *dp.Maximum > maxSize {
			maxSize = *dp.Maximum
			found = true
		}
	}
	if !found {
		return 0, 0, nil
	}
	return maxSize, len(out.Datapoints), nil
}

func requiredTempBytes(s3MaxBytes int64) int64 {
	// Peak on --tempdir is extracted objects plus the zip written next to them.
	return s3MaxBytes * 2
}
