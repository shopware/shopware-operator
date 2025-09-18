package util

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sync"

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

	var errors error
	for err := range errChan {
		logger.Errorw("Error downloading bucket", zap.Error(err))
		errors = fmt.Errorf("%w; %s", errors, err.Error())
	}

	return errors
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

	var errors error
	for err := range errChan {
		logger.Errorw("Error uploading bucket", zap.Error(err))
		errors = fmt.Errorf("%w; %s", errors, err.Error())
	}

	return errors
}

func uploadBucket(ctx context.Context, s3Client *minio.Client, bucketName, sourcePath string, emptyBeforeRestore bool) error {
	if emptyBeforeRestore {
		objects := s3Client.ListObjects(ctx, bucketName, minio.ListObjectsOptions{
			Recursive: true,
		})
		toDeleteCh := make(chan minio.ObjectInfo)

		go func() {
			for obj := range objects {
				if obj.Err == nil {
					toDeleteCh <- obj
				}
			}
			close(toDeleteCh)
		}()

		for rErr := range s3Client.RemoveObjects(ctx, bucketName, toDeleteCh, minio.RemoveObjectsOptions{}) {
			if rErr.Err != nil {
				return fmt.Errorf("failed to delete object %s: %w", rErr.ObjectName, rErr.Err)
			}
		}
	}

	var files []string
	err := filepath.WalkDir(sourcePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walking source path: %w", err)
	}

	const workers = 30
	wg := sync.WaitGroup{}
	errCh := make(chan error, workers)
	sem := make(chan struct{}, workers)

	for _, file := range files {
		file := file
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			relPath, err := filepath.Rel(sourcePath, file)
			if err != nil {
				errCh <- fmt.Errorf("rel path error for %s: %w", file, err)
				return
			}

			_, err = s3Client.FPutObject(ctx, bucketName, relPath, file, minio.PutObjectOptions{})
			if err != nil {
				errCh <- fmt.Errorf("error uploading %s: %w", file, err)
				return
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		return err
	}

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
			break
		default:
		}

		wg.Add(1)
		go func(obj minio.ObjectInfo) {
			defer wg.Done()

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
					cancel()
				default:
				}
				return
			}

			err = f(obj, r)
			if err != nil {
				select {
				case errCh <- fmt.Errorf("error processing in func %s: %w", obj.Key, err):
					cancel()
				default:
				}
				return
			}
		}(object)
	}

	go func() {
		wg.Wait()
		close(errCh)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			cancel()
			return err
		}
	case <-ctx.Done():
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

	for object := range objects {
		if object.Err != nil {
			return fmt.Errorf("error listing objects: %w", object.Err)
		}

		wg.Add(1)
		go func(obj minio.ObjectInfo) {
			destPath := filepath.Join(destinationPath, obj.Key)
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				errCh <- fmt.Errorf("error creating dirs for %s: %w", obj.Key, err)
				return
			}

			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

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
