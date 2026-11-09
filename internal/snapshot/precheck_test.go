package snapshot

import (
	"context"
	"fmt"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/shopware/shopware-operator/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubSizer struct {
	usage map[string]struct {
		bytes   int64
		objects int64
	}
	err error
}

func (s *stubSizer) BucketUsage(_ context.Context, bucket string) (int64, int64, error) {
	if s.err != nil {
		return 0, 0, s.err
	}
	u := s.usage[bucket]
	return u.bytes, u.objects, nil
}

func TestRequiredTempBytes(t *testing.T) {
	assert.Equal(t, int64(200), requiredTempBytes(100))
}

func TestFormatBytes(t *testing.T) {
	assert.Equal(t, "0B", formatBytes(0))
	assert.Equal(t, "100B", formatBytes(100))
	assert.Equal(t, "1Ki", formatBytes(1024))
	assert.Equal(t, "1.5Ki", formatBytes(1536))
	assert.Equal(t, "100Gi", formatBytes(100*1024*1024*1024))
	assert.Equal(t, "200Gi", formatBytes(200*1024*1024*1024))
	assert.Equal(t, "95.2Gi", formatBytes(102211734557))
	assert.Equal(t, "190.4Gi", formatBytes(204423469114))
}

func TestSumObjectSizes(t *testing.T) {
	ch := make(chan minio.ObjectInfo, 3)
	ch <- minio.ObjectInfo{Key: "a", Size: 10}
	ch <- minio.ObjectInfo{Key: "dir/", Size: 0}
	ch <- minio.ObjectInfo{Key: "b", Size: 25}
	close(ch)

	size, count, err := sumObjectSizes(ch)
	require.NoError(t, err)
	assert.Equal(t, int64(35), size)
	assert.Equal(t, int64(3), count)
}

func TestSumObjectSizesError(t *testing.T) {
	ch := make(chan minio.ObjectInfo, 2)
	ch <- minio.ObjectInfo{Key: "a", Size: 10}
	ch <- minio.ObjectInfo{Err: fmt.Errorf("list failed")}
	close(ch)

	_, _, err := sumObjectSizes(ch)
	require.EqualError(t, err, "list failed")
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
		&stubSizer{
			usage: map[string]struct {
				bytes   int64
				objects int64
			}{
				"priv": {bytes: 30, objects: 3},
				"pub":  {bytes: 20, objects: 2},
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

func TestPrecheckTempStorageInsufficient(t *testing.T) {
	svc := NewSnapshotService(
		&config.SnapshotConfig{
			S3: config.S3Config{PrivateBucket: "priv", PublicBucket: "pub"},
		},
		&stubSizer{
			usage: map[string]struct {
				bytes   int64
				objects int64
			}{
				"priv": {bytes: 50, objects: 1},
				"pub":  {bytes: 50, objects: 1},
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

func TestPrecheckTempStorageEmptyBuckets(t *testing.T) {
	svc := NewSnapshotService(
		&config.SnapshotConfig{
			S3: config.S3Config{PrivateBucket: "priv"},
		},
		&stubSizer{usage: map[string]struct {
			bytes   int64
			objects int64
		}{}},
	)
	err := svc.precheckTempStorage(context.Background(), svc.config, &SnapshotContext{
		IncludeS3:     true,
		TempSizeBytes: 100,
	})
	assert.NoError(t, err)
}

func TestPrecheckTempStorageListError(t *testing.T) {
	svc := NewSnapshotService(
		&config.SnapshotConfig{
			S3: config.S3Config{PrivateBucket: "priv"},
		},
		&stubSizer{err: fmt.Errorf("access denied")},
	)
	err := svc.precheckTempStorage(context.Background(), svc.config, &SnapshotContext{
		IncludeS3:     true,
		TempSizeBytes: 100,
	})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrInsufficientTempSpace)
	assert.Contains(t, err.Error(), "access denied")
}

type cancelOnOtherBucketSizer struct {
	failBucket string
	failErr    error
}

func (s *cancelOnOtherBucketSizer) BucketUsage(ctx context.Context, bucket string) (int64, int64, error) {
	if bucket == s.failBucket {
		return 0, 0, s.failErr
	}
	<-ctx.Done()
	return 0, 0, ctx.Err()
}

func TestPrecheckTempStorageKeepsListingErrorOverCancel(t *testing.T) {
	denied := fmt.Errorf("AccessDenied")
	svc := NewSnapshotService(
		&config.SnapshotConfig{
			S3: config.S3Config{PrivateBucket: "priv", PublicBucket: "pub"},
		},
		&cancelOnOtherBucketSizer{failBucket: "priv", failErr: denied},
	)
	err := svc.precheckTempStorage(context.Background(), svc.config, &SnapshotContext{
		IncludeS3:     true,
		TempSizeBytes: 100,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, denied)
	assert.NotErrorIs(t, err, context.Canceled)
	assert.Contains(t, err.Error(), "priv")
}

func TestPrecheckTempStorageRunsForMinIOEndpoint(t *testing.T) {
	svc := NewSnapshotService(
		&config.SnapshotConfig{
			S3: config.S3Config{PrivateBucket: "priv", Endpoint: "minio.minio.svc:9000"},
		},
		&stubSizer{
			usage: map[string]struct {
				bytes   int64
				objects int64
			}{
				"priv": {bytes: 10, objects: 1},
			},
		},
	)
	err := svc.precheckTempStorage(context.Background(), svc.config, &SnapshotContext{
		IncludeS3:     true,
		TempSizeBytes: 100,
	})
	require.NoError(t, err)
}

func TestIsNoSpaceErrorDetectsInsufficientTempSpace(t *testing.T) {
	err := fmt.Errorf("precheck: %w", ErrInsufficientTempSpace)
	assert.True(t, IsNoSpaceError(err))
}
