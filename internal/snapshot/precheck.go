package snapshot

import (
	"context"
	"fmt"
	"sync"

	"github.com/minio/minio-go/v7"
	"github.com/shopware/shopware-operator/internal/config"
	"github.com/shopware/shopware-operator/internal/logging"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/resource"
)

// BucketSizer reports the current-object size of an S3 bucket (live keys only).
type BucketSizer interface {
	BucketUsage(ctx context.Context, bucket string) (bytes int64, objects int64, err error)
}

type minioBucketSizer struct {
	client *minio.Client
}

func (m minioBucketSizer) BucketUsage(ctx context.Context, bucket string) (int64, int64, error) {
	return sumObjectSizes(m.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Recursive: true,
	}))
}

func newMinioClient(ctx context.Context, cfg config.S3Config) (*minio.Client, error) {
	cred, err := getAWSKeysWithAsumeRole(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return minio.New(cfg.Endpoint, &minio.Options{
		Creds:  cred,
		Secure: true,
	})
}

func (s *SnapshotService) resolveSizer(ctx context.Context) (BucketSizer, error) {
	if s.sizer != nil {
		return s.sizer, nil
	}
	client, err := newMinioClient(ctx, s.config.S3)
	if err != nil {
		return nil, err
	}
	return minioBucketSizer{client: client}, nil
}

func (s *SnapshotService) precheckTempStorage(ctx context.Context, cfg *config.SnapshotConfig, snapshotCtx *SnapshotContext) error {
	if !snapshotCtx.IncludeS3 {
		return nil
	}
	if snapshotCtx.TempSizeBytes <= 0 {
		return fmt.Errorf("s3 size precheck: temp size must be positive")
	}

	sizer, err := s.resolveSizer(ctx)
	if err != nil {
		return fmt.Errorf("s3 size precheck: %w", err)
	}

	logger := logging.FromContext(ctx)
	buckets := make([]string, 0, 2)
	for _, bucket := range []string{cfg.S3.PrivateBucket, cfg.S3.PublicBucket} {
		if bucket != "" {
			buckets = append(buckets, bucket)
		}
	}
	if len(buckets) == 0 {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type bucketUsage struct {
		bucket  string
		bytes   int64
		objects int64
		err     error
	}
	results := make(chan bucketUsage, len(buckets))
	var wg sync.WaitGroup
	for _, bucket := range buckets {
		wg.Add(1)
		go func(bucket string) {
			defer wg.Done()
			bytes, objects, err := sizer.BucketUsage(ctx, bucket)
			results <- bucketUsage{bucket: bucket, bytes: bytes, objects: objects, err: err}
			if err != nil {
				cancel()
			}
		}(bucket)
	}
	wg.Wait()
	close(results)

	var total int64
	var errs []error
	for usage := range results {
		if usage.err != nil {
			errs = append(errs, fmt.Errorf("s3 size precheck: bucket %s: %w", usage.bucket, usage.err))
			continue
		}
		total += usage.bytes
		logger.Infow("S3 bucket size from listing",
			zap.String("bucket", usage.bucket),
			zap.String("size", formatBytes(usage.bytes)),
			zap.Int64("objects", usage.objects),
		)
	}
	if err := joinSnapshotErrors(errs); err != nil {
		return err
	}

	required := requiredTempBytes(total)
	logger.Infow("Snapshot temp storage precheck",
		zap.String("s3Size", formatBytes(total)),
		zap.String("required", formatBytes(required)),
		zap.String("tempSize", formatBytes(snapshotCtx.TempSizeBytes)),
	)
	if required > snapshotCtx.TempSizeBytes {
		return fmt.Errorf("%w: listed %s (S3 current objects + archive) exceeds temp size %s",
			ErrInsufficientTempSpace, formatBytes(required), formatBytes(snapshotCtx.TempSizeBytes))
	}
	return nil
}

func sumObjectSizes(objects <-chan minio.ObjectInfo) (int64, int64, error) {
	var size, count int64
	for obj := range objects {
		if obj.Err != nil {
			return 0, 0, obj.Err
		}
		count++
		size += obj.Size
	}
	return size, count, nil
}

func requiredTempBytes(s3Bytes int64) int64 {
	// Peak on --tempdir is extracted objects plus the zip written next to them.
	return s3Bytes * 2
}

func formatBytes(n int64) string {
	return resource.NewQuantity(n, resource.BinarySI).String()
}
