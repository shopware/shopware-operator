package snapshot

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/shopware/shopware-operator/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubMetrics struct {
	// bucket -> storageType -> datapoint averages
	series map[string]map[string][]float64
	err    error
}

func (s *stubMetrics) GetMetricStatistics(_ context.Context, params *cloudwatch.GetMetricStatisticsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricStatisticsOutput, error) {
	if s.err != nil {
		return nil, s.err
	}
	bucket := ""
	storageType := ""
	for _, d := range params.Dimensions {
		switch aws.ToString(d.Name) {
		case "BucketName":
			bucket = aws.ToString(d.Value)
		case "StorageType":
			storageType = aws.ToString(d.Value)
		}
	}
	values := s.series[bucket][storageType]
	out := &cloudwatch.GetMetricStatisticsOutput{}
	for _, v := range values {
		maxVal := v
		out.Datapoints = append(out.Datapoints, cwtypes.Datapoint{Maximum: &maxVal})
	}
	return out, nil
}

func TestRequiredTempBytes(t *testing.T) {
	assert.Equal(t, int64(200), requiredTempBytes(100))
}

func TestPrecheckTempStorageRequiresMetricsClient(t *testing.T) {
	svc := NewSnapshotService(&config.SnapshotConfig{
		S3: config.S3Config{PrivateBucket: "priv"},
	}, nil)
	err := svc.precheckTempStorage(context.Background(), svc.config, &SnapshotContext{
		IncludeS3:     true,
		TempSizeBytes: 100,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metrics client is required")
}

func TestPrecheckTempStorageSkipsWhenS3Disabled(t *testing.T) {
	svc := NewSnapshotService(&config.SnapshotConfig{}, nil)
	err := svc.precheckTempStorage(context.Background(), svc.config, &SnapshotContext{
		IncludeS3:     false,
		TempSizeBytes: 1,
	})
	assert.NoError(t, err)
}

func TestPrecheckTempStorageFits(t *testing.T) {
	svc := NewSnapshotService(
		&config.SnapshotConfig{
			S3: config.S3Config{PrivateBucket: "priv", PublicBucket: "pub"},
		},
		&stubMetrics{
			series: map[string]map[string][]float64{
				"priv": {"StandardStorage": {10, 30}},
				"pub":  {"StandardStorage": {20, 20}},
			},
		},
	)
	err := svc.precheckTempStorage(context.Background(), svc.config, &SnapshotContext{
		IncludeS3:     true,
		IncludeDB:     true,
		TempSizeBytes: 100,
	})
	require.NoError(t, err)
}

func TestPrecheckTempStorageUsesMaxNotAverage(t *testing.T) {
	svc := NewSnapshotService(
		&config.SnapshotConfig{
			S3: config.S3Config{PrivateBucket: "priv", PublicBucket: "pub"},
		},
		&stubMetrics{
			series: map[string]map[string][]float64{
				"priv": {"StandardStorage": {10, 60}},
				"pub":  {"StandardStorage": {10}},
			},
		},
	)
	err := svc.precheckTempStorage(context.Background(), svc.config, &SnapshotContext{
		IncludeS3:     true,
		TempSizeBytes: 100,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInsufficientTempSpace)
}

func TestPrecheckTempStorageInsufficient(t *testing.T) {
	svc := NewSnapshotService(
		&config.SnapshotConfig{
			S3: config.S3Config{PrivateBucket: "priv", PublicBucket: "pub"},
		},
		&stubMetrics{
			series: map[string]map[string][]float64{
				"priv": {"StandardStorage": {50}},
				"pub":  {"StandardStorage": {50}},
			},
		},
	)
	err := svc.precheckTempStorage(context.Background(), svc.config, &SnapshotContext{
		IncludeS3:     true,
		TempSizeBytes: 100,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInsufficientTempSpace)
	assert.True(t, IsNoSpaceError(err))
}

func TestPrecheckTempStorageNoDatapoints(t *testing.T) {
	svc := NewSnapshotService(
		&config.SnapshotConfig{
			S3: config.S3Config{PrivateBucket: "priv"},
		},
		&stubMetrics{series: map[string]map[string][]float64{}},
	)
	err := svc.precheckTempStorage(context.Background(), svc.config, &SnapshotContext{
		IncludeS3:     true,
		TempSizeBytes: 100,
	})
	assert.NoError(t, err)
}

func TestPrecheckTempStorageCloudWatchError(t *testing.T) {
	svc := NewSnapshotService(
		&config.SnapshotConfig{
			S3: config.S3Config{PrivateBucket: "priv"},
		},
		&stubMetrics{err: fmt.Errorf("access denied")},
	)
	err := svc.precheckTempStorage(context.Background(), svc.config, &SnapshotContext{
		IncludeS3:     true,
		TempSizeBytes: 100,
	})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrInsufficientTempSpace)
}

func TestMaxBucketSizeUsesStandardStoragePeak(t *testing.T) {
	client := &stubMetrics{
		series: map[string]map[string][]float64{
			"b": {
				"StandardStorage":   {10, 30},
				"StandardIAStorage": {5},
			},
		},
	}
	size, points, err := maxBucketSize(context.Background(), client, "b", time.Now().Add(-time.Hour), time.Now())
	require.NoError(t, err)
	assert.Equal(t, int64(30), size)
	assert.Equal(t, 2, points)
}

func TestIsNoSpaceErrorDetectsInsufficientTempSpace(t *testing.T) {
	err := fmt.Errorf("precheck: %w", ErrInsufficientTempSpace)
	assert.True(t, IsNoSpaceError(err))
}
