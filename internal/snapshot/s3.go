package snapshot

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/minio/minio-go/v7"
	"github.com/shopware/shopware-operator/internal/config"
	"github.com/shopware/shopware-operator/internal/logging"
	"github.com/shopware/shopware-operator/internal/util"
	"go.uber.org/zap"
)

func (s *SnapshotService) RestoreBackup(
	ctx context.Context, cfg *config.SnapshotConfig, snapshotCtx *SnapshotContext,
) error {
	logger := logging.FromContext(ctx)
	logger.Infow("Restoring snapshot", zap.Any("snapshot", snapshotCtx))

	// First, extract the archive (from S3 or local file)
	err := s.ExtractArchive(ctx, cfg, snapshotCtx)
	if err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	// Restore database if included
	if snapshotCtx.IncludeDB {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := s.restoreDatabaseBackup(cancelCtx, cfg, snapshotCtx)
			if err != nil {
				cancel()
				logger.Errorw("Failed to restore database backup", zap.Error(err))
				errChan <- fmt.Errorf("failed to restore database backup: %w", err)
			}
		}()
	}

	// Restore S3 assets if included
	if snapshotCtx.IncludeS3 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := s.restoreAssetBackup(cancelCtx, cfg, snapshotCtx)
			if err != nil {
				cancel()
				logger.Errorw("Failed to restore asset backup", zap.Error(err))
				errChan <- fmt.Errorf("failed to restore asset backup: %w", err)
			}
		}()
	}

	wg.Wait()
	cancel()
	close(errChan)
	if err := <-errChan; err != nil {
		return fmt.Errorf("snapshot restore failed: %w", err)
	}

	logger.Infow("Successfully restored snapshot")
	return nil
}

func (s *SnapshotService) CreateBackup(
	ctx context.Context, cfg *config.SnapshotConfig, snapshotCtx *SnapshotContext,
) error {
	logger := logging.FromContext(ctx)
	logger.Infow("Creating snapshot", zap.Any("snapshot", snapshotCtx))
	cancelCtx, cancel := context.WithCancel(ctx)

	var wg sync.WaitGroup
	errChan := make(chan error)

	if snapshotCtx.IncludeDB {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := s.createDatabaseBackup(cancelCtx, cfg, snapshotCtx)
			if err != nil {
				cancel()
				logger.Errorw("Failed to create database backup", zap.Error(err))
				errChan <- fmt.Errorf("failed to create database backup: %w", err)
			}
		}()
	}

	if snapshotCtx.IncludeS3 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := s.createAssetBackup(cancelCtx, cfg, snapshotCtx)
			if err != nil {
				cancel()
				logger.Errorw("Failed to create asset backup", zap.Error(err))
				errChan <- fmt.Errorf("failed to create asset backup: %w", err)
			}
		}()
	}

	wg.Wait()
	cancel()
	close(errChan)
	if err := <-errChan; err != nil {
		return fmt.Errorf("snapshot creation failed: %w", err)
	}

	err := s.CreateArchive(ctx, cfg, snapshotCtx)
	if err != nil {
		return fmt.Errorf("failed to create archive: %w", err)
	}

	return nil
}

func (s *SnapshotService) createAssetBackup(
	ctx context.Context, cfg *config.SnapshotConfig, snapshotCtx *SnapshotContext,
) error {
	logger := logging.FromContext(ctx)

	var wg sync.WaitGroup
	errChan := make(chan error, 2)
	parallelDownloads := 30

	cred, err := getAWSKeysWithAsumeRole(ctx, cfg.S3)
	if err != nil {
		return fmt.Errorf("failed to get AWS credentials: %w", err)
	}

	logger.Infow("Init minio client", zap.String("endpoint", cfg.S3.Endpoint))
	minioClient, err := minio.New(cfg.S3.Endpoint, &minio.Options{
		Creds:  cred,
		Secure: true,
	})
	if err != nil {
		return fmt.Errorf("failed to create minio client: %w", err)
	}

	// Start loading private bucket
	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error
		logger.Info("Starting S3 private bucket read")
		downloader := util.NewS3Downloader(minioClient, cfg.S3.PrivateBucket)
		err = downloader.DownloadBucket(ctx, parallelDownloads, func(i minio.ObjectInfo, o *minio.Object) error {
			var err error

			file := filepath.Join(snapshotCtx.TempArchiveDir, "private", i.Key)
			if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

			f, err := os.Create(file)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
			defer func() {
				if err := f.Close(); err != nil {
					logger.Warnw("failed to close private bucket file", zap.String("file", file), zap.Error(err))
				}
			}()

			_, err = io.CopyBuffer(f, o, make([]byte, 1024*8))
			if err != nil {
				return fmt.Errorf("failed to copy object to file: %w", err)
			}

			return o.Close()
		})
		if err != nil {
			logger.Errorw("Private bucket backup failed", zap.Error(err))
			errChan <- fmt.Errorf("private bucket backup: %w", err)
		}

		logger.Info("Done S3 private bucket read")
	}()

	// Start loading public bucket
	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error
		logger.Info("Starting S3 public bucket read")
		downloader := util.NewS3Downloader(minioClient, cfg.S3.PublicBucket)
		err = downloader.DownloadBucket(ctx, parallelDownloads, func(i minio.ObjectInfo, o *minio.Object) error {
			var err error

			file := filepath.Join(snapshotCtx.TempArchiveDir, "public", i.Key)
			if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

			f, err := os.Create(file)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
			defer func() {
				if err := f.Close(); err != nil {
					logger.Warnw("failed to close public bucket file", zap.String("file", file), zap.Error(err))
				}
			}()

			_, err = io.CopyBuffer(f, o, make([]byte, 1024*8))
			if err != nil {
				return fmt.Errorf("failed to copy object to file: %w", err)
			}
			return o.Close()
		})
		if err != nil {
			logger.Errorw("Public bucket backup failed", zap.Error(err))
			errChan <- fmt.Errorf("public bucket backup: %w", err)
		}

		logger.Info("Done S3 public bucket read")
	}()

	wg.Wait()
	close(errChan)

	if err, ok := <-errChan; ok {
		return fmt.Errorf("snapshot creation failed errChan: %w", err)
	}

	return nil
}

func (s *SnapshotService) createDatabaseBackup(
	ctx context.Context, cfg *config.SnapshotConfig, snapshotCtx *SnapshotContext,
) error {
	logger := logging.FromContext(ctx)

	logger.Info("Starting database backup")
	targetDir := filepath.Join(snapshotCtx.TempArchiveDir, "database")

	dump := util.NewMySQLShell(cfg.Database.MysqlShellBinaryPath)
	out, err := dump.Dump(ctx, util.DumpInput{
		DumpFilePath: targetDir,
		DatabaseSpec: util.DatabaseSpec{
			Host:     cfg.Database.Host,
			Password: []byte(cfg.Database.Password),
			User:     cfg.Database.User,
			Port:     cfg.Database.Port,
			Name:     cfg.Database.Database,
			Version:  cfg.Database.Version,
			SSLMode:  cfg.Database.SSLMode,
			Options:  cfg.Database.Options,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to dump database: %w", err)
	}

	logger.Infow("Finish database backup", zap.Any("output", out))

	return nil
}

func (s *SnapshotService) restoreDatabaseBackup(
	ctx context.Context, cfg *config.SnapshotConfig, snapshotCtx *SnapshotContext,
) error {
	logger := logging.FromContext(ctx)
	logger.Info("Starting database restore")

	// Database dump should be extracted to database subdirectory
	dumpFilePath := filepath.Join(snapshotCtx.TempArchiveDir, "database")

	shell := util.NewMySQLShell(cfg.Database.MysqlShellBinaryPath)
	err := shell.RestoreDump(ctx, util.RestoreInput{
		DatabaseSpec: util.DatabaseSpec{
			Host:     cfg.Database.Host,
			Password: []byte(cfg.Database.Password),
			User:     cfg.Database.User,
			Port:     cfg.Database.Port,
			Name:     cfg.Database.Database,
			Version:  cfg.Database.Version,
			SSLMode:  cfg.Database.SSLMode,
			Options:  cfg.Database.Options,
		},
		DumpFilePath: dumpFilePath,
	})
	if err != nil {
		return fmt.Errorf("failed to restore database: %w", err)
	}

	logger.Info("Finished database restore")
	return nil
}

func (s *SnapshotService) restoreAssetBackup(
	ctx context.Context, cfg *config.SnapshotConfig, snapshotCtx *SnapshotContext,
) error {
	logger := logging.FromContext(ctx)
	logger.Info("Starting asset restore to S3")

	// Use existing restoreS3 function to upload extracted assets back to S3 buckets
	err := s.restoreS3(ctx, cfg.S3, snapshotCtx.TempArchiveDir)
	if err != nil {
		return fmt.Errorf("failed to restore S3 assets: %w", err)
	}

	logger.Info("Finished asset restore")
	return nil
}

func (s *SnapshotService) uploadToS3(ctx context.Context, cfg *config.SnapshotConfig, s3Url string, contentType string, reader io.ReadCloser) error {
	defer func() {
		if err := reader.Close(); err != nil {
			logging.FromContext(ctx).Warnw("failed to close S3 upload reader", zap.Error(err))
		}
	}()
	bucket, objectFile, err := parseS3URL(s3Url)
	if err != nil {
		return fmt.Errorf("failed to parse S3 URL: %w", err)
	}

	cred, err := getAWSKeysWithAsumeRole(ctx, cfg.S3)
	if err != nil {
		return fmt.Errorf("failed to get AWS credentials: %w", err)
	}

	logging.FromContext(ctx).Debugw("Init minio client", zap.String("endpoint", cfg.S3.Endpoint), zap.String("bucket", bucket), zap.String("objectFile", objectFile))

	minioClient, err := minio.New(cfg.S3.Endpoint, &minio.Options{
		Creds:  cred,
		Secure: true,
	})
	if err != nil {
		return fmt.Errorf("failed to create minio client: %w", err)
	}

	_, err = minioClient.PutObject(ctx, bucket, objectFile, reader, -1,
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("failed to upload to s3: %w", err)
	}
	return nil
}

func (s *SnapshotService) downloadFromS3(ctx context.Context, cfg *config.SnapshotConfig, s3Url string) (io.ReadCloser, error) {
	bucket, objectFile, err := parseS3URL(s3Url)
	if err != nil {
		return nil, fmt.Errorf("failed to parse S3 URL: %w", err)
	}

	cred, err := getAWSKeysWithAsumeRole(ctx, cfg.S3)
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS credentials: %w", err)
	}

	logging.FromContext(ctx).Debugw("Init minio client for download", zap.String("endpoint", cfg.S3.Endpoint), zap.String("bucket", bucket), zap.String("objectFile", objectFile))

	minioClient, err := minio.New(cfg.S3.Endpoint, &minio.Options{
		Creds:  cred,
		Secure: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create minio client: %w", err)
	}

	reader, err := minioClient.GetObject(ctx, bucket, objectFile, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to download from s3: %w", err)
	}
	return reader, nil
}
