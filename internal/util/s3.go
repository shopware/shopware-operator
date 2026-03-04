package util

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/logging"
	"go.uber.org/zap"
)

const totalUploadWorkers = 30

type FileEntry struct {
	Name string
	Size int64
	R    io.ReadCloser
}

func TestS3Connection(ctx context.Context, s v1.S3Storage, c aws.Credentials) error {
	url, err := url.Parse(s.EndpointURL)
	if err != nil {
		return fmt.Errorf("parsing url failed: %w", err)
	}

	endpoint := fmt.Sprintf("%s:%s", url.Hostname(), url.Port())
	s3Client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(c.AccessKeyID, c.SecretAccessKey, ""),
		Secure: url.Scheme == "https",
	})
	if err != nil {
		return err
	}

	buckets, err := s3Client.ListBuckets(ctx)
	if err != nil {
		return fmt.Errorf("listing buckets failed: %w", err)
	}

	var public bool
	var private bool
	for _, b := range buckets {
		if b.Name == s.PublicBucketName {
			public = true
		}
		if b.Name != s.PrivateBucketName {
			private = true
		}
	}

	if public && private {
		return nil
	}
	return fmt.Errorf("public bucket is not available yet")
}

func CopyToFilesystem(ctx context.Context, cred *credentials.Credentials, directory string, s v1.S3Storage) error {
	logger := logging.FromContext(ctx).With(zap.Any("s3_storage", s))

	url, err := url.Parse(s.EndpointURL)
	if err != nil {
		return fmt.Errorf("parsing url failed: %w", err)
	}

	endpoint := fmt.Sprintf("%s:%s", url.Hostname(), url.Port())
	s3Client, err := minio.New(endpoint, &minio.Options{
		Creds:  cred,
		Secure: url.Scheme == "https",
	})
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	wg.Add(1)
	go func() {
		err := downloadBucket(ctx, s3Client, s.PublicBucketName, filepath.Join(directory, "public"))
		if err != nil {
			errChan <- fmt.Errorf("error downloading public bucket: %w", err)
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		err := downloadBucket(ctx, s3Client, s.PrivateBucketName, filepath.Join(directory, "private"))
		if err != nil {
			errChan <- fmt.Errorf("error downloading private bucket: %w", err)
		}
		wg.Done()
	}()

	wg.Wait()
	close(errChan)

	// nolint: prealloc
	var errs []error
	for err := range errChan {
		logger.Errorw("error downloading bucket", zap.Error(err))
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func RestoreFromFilesystem(ctx context.Context, cred *credentials.Credentials, directory string, s v1.S3Storage, emptyBeforeRestore bool) error {
	logger := logging.FromContext(ctx).With(zap.Any("s3_storage", s))

	logger.Debug("Init minio client")
	s3Client, err := minio.New(s.EndpointURL, &minio.Options{
		Creds:  cred,
		Secure: true,
	})
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	wg.Add(1)
	go func() {
		err := uploadBucket(ctx, s3Client, s.PublicBucketName, filepath.Join(directory, "public"), totalUploadWorkers/2, emptyBeforeRestore)
		if err != nil {
			errChan <- fmt.Errorf("error uploading to public bucket: %w", err)
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		err := uploadBucket(ctx, s3Client, s.PrivateBucketName, filepath.Join(directory, "private"), totalUploadWorkers/2, emptyBeforeRestore)
		if err != nil {
			errChan <- fmt.Errorf("error uploading to private bucket: %w", err)
		}
		wg.Done()
	}()

	wg.Wait()
	close(errChan)

	// nolint: prealloc
	var errs []error
	for err := range errChan {
		logger.Errorw("error uploading bucket", zap.Error(err))
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func emptyBucketAsync(ctx context.Context, s3Client *minio.Client, bucketName string) error {
	logger := logging.FromContext(ctx).With(zap.String("bucket", bucketName))
	logger.Info("Starting bucket emptying process")

	objects := s3Client.ListObjects(ctx, bucketName, minio.ListObjectsOptions{
		Recursive: true,
	})
	toDeleteCh := make(chan minio.ObjectInfo, 100)
	errCh := make(chan error, 1)
	doneCh := make(chan struct{})

	var objectCount int
	go func() {
		defer func() {
			close(toDeleteCh)
			logger.Debugw("Finished listing objects", zap.Int("total_objects", objectCount))
		}()

		for obj := range objects {
			if obj.Err != nil {
				logger.Errorw("Error listing object", zap.Error(obj.Err))
				select {
				case errCh <- fmt.Errorf("error listing object: %w", obj.Err):
				case <-ctx.Done():
				}
				return
			}

			objectCount++
			logger.Debugw("Found object to delete", zap.String("object_key", obj.Key), zap.Int64("size", obj.Size))

			select {
			case toDeleteCh <- obj:
			case <-ctx.Done():
				logger.Info("Context cancelled while listing objects")
				return
			}
		}
	}()

	var deletedCount int
	go func() {
		defer func() {
			close(doneCh)
			logger.Infow("Finished deleting objects", zap.Int("deleted_count", deletedCount))
		}()

		for rErr := range s3Client.RemoveObjects(ctx, bucketName, toDeleteCh, minio.RemoveObjectsOptions{}) {
			if rErr.Err != nil {
				logger.Errorw("Failed to delete object", zap.String("object_name", rErr.ObjectName), zap.Error(rErr.Err))
				select {
				case errCh <- fmt.Errorf("failed to delete object %s: %w", rErr.ObjectName, rErr.Err):
					return
				case <-ctx.Done():
					logger.Info("Context cancelled while deleting objects")
					return
				}
			} else {
				deletedCount++
				if deletedCount%100 == 0 {
					logger.Infow("Deletion progress", zap.Int("deleted_count", deletedCount))
				}
			}
		}
	}()

	select {
	case err := <-errCh:
		logger.Errorw("Bucket emptying failed", zap.Error(err))
		return err
	case <-doneCh:
		logger.Infow("Successfully emptied bucket", zap.Int("total_deleted", deletedCount))
		return nil
	case <-ctx.Done():
		logger.Info("Bucket emptying cancelled")
		return ctx.Err()
	}
}

func uploadBucket(ctx context.Context, s3Client *minio.Client, bucketName, sourcePath string, workers int, emptyBeforeRestore bool) error {
	logger := logging.FromContext(ctx).With(zap.String("bucket", bucketName), zap.String("source_path", sourcePath))
	logger.Info("Starting bucket upload process")

	if emptyBeforeRestore {
		logger.Info("Emptying bucket before restore")
		if err := emptyBucketAsync(ctx, s3Client, bucketName); err != nil {
			logger.Errorw("Failed to empty bucket", zap.Error(err))
			return err
		}
		logger.Info("Successfully emptied bucket, proceeding with upload")
	}

	logger.Info("Scanning source directory for files")
	if _, err := os.Stat(sourcePath); err != nil {
		if os.IsNotExist(err) {
			logger.Warnw("Source path does not exist", zap.String("source_path", sourcePath))
			logger.Info("No files to upload, operation complete")
			return nil
		}
		logger.Errorw("Failed to stat source path", zap.String("source_path", sourcePath), zap.Error(err))
		return fmt.Errorf("failed to stat source path: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	effectiveWorkers := workers
	if effectiveWorkers <= 0 {
		effectiveWorkers = 1
	}

	fileCh := make(chan string, effectiveWorkers)
	walkErrCh := make(chan error, 1)
	fileCount := int64(0)
	uploadedCount := int64(0)

	logger.Infow("Starting upload with workers", zap.Int("worker_count", effectiveWorkers))
	go func() {
		walkErr := filepath.WalkDir(sourcePath, func(path string, d fs.DirEntry, fileErr error) error {
			if fileErr != nil {
				logger.Errorw("Error walking directory", zap.String("path", path), zap.Error(fileErr))
				return fileErr
			}

			if d.IsDir() {
				return nil
			}

			atomic.AddInt64(&fileCount, 1)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case fileCh <- path:
				return nil
			}
		})

		close(fileCh)
		walkErrCh <- walkErr
	}()

	err := runUploadWorkers(ctx, effectiveWorkers, fileCh, func(ctx context.Context, file string) error {
		relPath, err := filepath.Rel(sourcePath, file)
		if err != nil {
			return fmt.Errorf("rel path error for %s: %w", file, err)
		}

		logger.Debugw("Uploading file",
			zap.String("local_file", file),
			zap.String("s3_key", relPath))

		info, err := s3Client.FPutObject(ctx, bucketName, relPath, file, minio.PutObjectOptions{})
		if err != nil {
			return fmt.Errorf("error uploading %s: %w", file, err)
		}

		uploaded := atomic.AddInt64(&uploadedCount, 1)
		if uploaded%100 == 0 {
			logger.Infow("Upload progress",
				zap.Int64("uploaded_count", uploaded),
				zap.Int64("scanned_files", atomic.LoadInt64(&fileCount)),
				zap.String("latest_file", relPath),
				zap.Int64("size_bytes", info.Size))
		}

		return nil
	})
	if err != nil {
		cancel()
	}

	walkErr := <-walkErrCh

	if err != nil {
		logger.Errorw("Upload failed", zap.Error(err))
		return err
	}

	if walkErr != nil {
		if !errors.Is(walkErr, context.Canceled) {
			logger.Errorw("Failed to walk source path", zap.Error(walkErr))
			return fmt.Errorf("walking source path: %w", walkErr)
		}
		if ctx.Err() != nil {
			logger.Errorw("Failed to walk source path", zap.Error(ctx.Err()))
			return fmt.Errorf("walking source path: %w", ctx.Err())
		}
	}

	if fileCount == 0 {
		logger.Info("No files to upload, operation complete")
		return nil
	}

	logger.Infow("Successfully completed bucket upload",
		zap.Int64("total_files_uploaded", uploadedCount),
		zap.Int64("total_files_scanned", fileCount),
		zap.String("bucket", bucketName))
	return nil
}

func runUploadWorkers(
	ctx context.Context,
	workers int,
	files <-chan string,
	worker func(context.Context, string) error,
) error {
	return runWorkers(ctx, workers, files, worker)
}

type S3Downloader struct {
	client *minio.Client
	bucket string
}

func NewS3Downloader(client *minio.Client, bucket string) *S3Downloader {
	return &S3Downloader{
		client: client,
		bucket: bucket,
	}
}

func (s *S3Downloader) DownloadBucket(ctx context.Context, batchCount int, f func(minio.ObjectInfo, *minio.Object) error) error {
	objects := s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Recursive: true,
	})

	return runDownloadWorkers(ctx, batchCount, objects, func(ctx context.Context, obj minio.ObjectInfo) error {
		r, err := s.client.GetObject(ctx, s.bucket, obj.Key, minio.GetObjectOptions{})
		if err != nil {
			return fmt.Errorf("error downloading %s: %w", obj.Key, err)
		}

		if err := f(obj, r); err != nil {
			return fmt.Errorf("error processing in func %s: %w", obj.Key, err)
		}

		return nil
	})
}

func runDownloadWorkers(
	ctx context.Context,
	batchCount int,
	objects <-chan minio.ObjectInfo,
	worker func(context.Context, minio.ObjectInfo) error,
) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if batchCount <= 0 {
		batchCount = 1
	}

	objCh := make(chan minio.ObjectInfo, batchCount)
	listErrCh := make(chan error, 1)

	go func() {
		defer close(objCh)

		for object := range objects {
			if object.Err != nil {
				listErrCh <- fmt.Errorf("error listing objects: %w", object.Err)
				cancel()
				return
			}

			select {
			case <-ctx.Done():
				listErrCh <- ctx.Err()
				return
			case objCh <- object:
			}
		}

		listErrCh <- nil
	}()

	workerErr := runWorkers(ctx, batchCount, objCh, worker)
	if workerErr != nil {
		cancel()
	}
	listErr := <-listErrCh

	if workerErr != nil {
		if errors.Is(workerErr, context.Canceled) && listErr != nil && !errors.Is(listErr, context.Canceled) {
			return listErr
		}
		return workerErr
	}

	if listErr != nil && !errors.Is(listErr, context.Canceled) {
		return listErr
	}

	return nil
}

func runWorkers[T any](
	ctx context.Context,
	workers int,
	items <-chan T,
	worker func(context.Context, T) error,
) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if workers <= 0 {
		workers = 1
	}

	wg := sync.WaitGroup{}
	errCh := make(chan error, 1)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				case item, ok := <-items:
					if !ok {
						return
					}

					if err := worker(ctx, item); err != nil {
						select {
						case errCh <- err:
							cancel()
						default:
						}
						return
					}
				}
			}
		}()
	}

	wg.Wait()

	select {
	case err := <-errCh:
		return err
	default:
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	return nil
}

func downloadBucket(ctx context.Context, s3Client *minio.Client, bucketName string, destinationPath string) error {
	objects := s3Client.ListObjects(ctx, bucketName, minio.ListObjectsOptions{
		Recursive: true,
	})

	const workers = 30
	wg := sync.WaitGroup{}
	errCh := make(chan error, workers)
	sem := make(chan struct{}, workers)

	if err := os.MkdirAll(destinationPath, 0755); err != nil {
		return fmt.Errorf("error creating destination directory: %w", err)
	}

	for object := range objects {
		if object.Err != nil {
			return fmt.Errorf("error listing objects: %w", object.Err)
		}

		wg.Add(1)
		go func(obj minio.ObjectInfo) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			destPath := filepath.Join(destinationPath, obj.Key)
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				errCh <- fmt.Errorf("error creating dirs for %s: %w", obj.Key, err)
				return
			}

			err := s3Client.FGetObject(ctx, bucketName, obj.Key, destPath, minio.GetObjectOptions{})
			if err != nil {
				errCh <- fmt.Errorf("error downloading %s: %w", obj.Key, err)
				return
			}
		}(object)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		return err
	}

	return nil
}
