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
		err := uploadBucket(ctx, s3Client, s.PublicBucketName, filepath.Join(directory, "public"), emptyBeforeRestore)
		if err != nil {
			errChan <- fmt.Errorf("error uploading to public bucket: %w", err)
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		err := uploadBucket(ctx, s3Client, s.PrivateBucketName, filepath.Join(directory, "private"), emptyBeforeRestore)
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

func uploadBucket(ctx context.Context, s3Client *minio.Client, bucketName, sourcePath string, emptyBeforeRestore bool) error {
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
	var files []string
	err := filepath.WalkDir(sourcePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			logger.Errorw("Error walking directory", zap.String("path", path), zap.Error(err))
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		logger.Errorw("Failed to walk source path", zap.Error(err))
		return fmt.Errorf("walking source path: %w", err)
	}

	logger.Infow("Found files to upload", zap.Int("file_count", len(files)))
	if len(files) == 0 {
		logger.Info("No files to upload, operation complete")
		return nil
	}

	const workers = 30
	wg := sync.WaitGroup{}
	errCh := make(chan error, workers)
	sem := make(chan struct{}, workers)
	uploadedCount := int64(0)

	logger.Infow("Starting upload with workers", zap.Int("worker_count", workers))

	for i, file := range files {
		file := file
		fileIndex := i + 1
		wg.Add(1)
		go func() {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				logger.Debug("Upload cancelled by context")
				return
			}
			defer func() { <-sem }()

			relPath, err := filepath.Rel(sourcePath, file)
			if err != nil {
				logger.Errorw("Failed to get relative path", zap.String("file", file), zap.Error(err))
				errCh <- fmt.Errorf("rel path error for %s: %w", file, err)
				return
			}

			logger.Debugw("Uploading file",
				zap.String("local_file", file),
				zap.String("s3_key", relPath),
				zap.Int("file_index", fileIndex),
				zap.Int("total_files", len(files)))

			info, err := s3Client.FPutObject(ctx, bucketName, relPath, file, minio.PutObjectOptions{})
			if err != nil {
				logger.Errorw("Failed to upload file", zap.String("file", file), zap.String("s3_key", relPath), zap.Error(err))
				errCh <- fmt.Errorf("error uploading %s: %w", file, err)
				return
			}

			uploaded := atomic.AddInt64(&uploadedCount, 1)
			if uploaded%100 == 0 || uploaded == int64(len(files)) {
				logger.Infow("Upload progress",
					zap.Int64("uploaded_count", uploaded),
					zap.Int("total_files", len(files)),
					zap.String("latest_file", relPath),
					zap.Int64("size_bytes", info.Size))
			}
		}()
	}

	logger.Info("Waiting for all uploads to complete")
	wg.Wait()
	close(errCh)

	for err := range errCh {
		logger.Errorw("Upload failed", zap.Error(err))
		return err
	}

	logger.Infow("Successfully completed bucket upload",
		zap.Int("total_files_uploaded", len(files)),
		zap.String("bucket", bucketName))
	return nil
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

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wg := sync.WaitGroup{}
	errCh := make(chan error, 1)
	sem := make(chan struct{}, batchCount)

	for object := range objects {
		if object.Err != nil {
			return fmt.Errorf("error listing objects: %w", object.Err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		wg.Add(1)
		go func(obj minio.ObjectInfo) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			r, err := s.client.GetObject(ctx, s.bucket, obj.Key, minio.GetObjectOptions{})
			if err != nil {
				select {
				case errCh <- fmt.Errorf("error downloading %s: %w", obj.Key, err):
					cancel() // Stop other downloads
				default:
				}
				return
			}

			err = f(obj, r)
			if err != nil {
				select {
				case errCh <- fmt.Errorf("error processing in func %s: %w", obj.Key, err):
					cancel() // Stop other downloads
				default:
				}
				return
			}
		}(object)
	}

	// Wait for all goroutines and close error channel
	go func() {
		wg.Wait()
		close(errCh)
	}()

	// Return first error encountered
	for err := range errCh {
		return err
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
