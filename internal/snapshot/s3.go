package snapshot

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/minio/minio-go/v7"
	"github.com/shopware/shopware-operator/internal/config"
	"github.com/shopware/shopware-operator/internal/logging"
	"github.com/shopware/shopware-operator/internal/util"
	"go.uber.org/zap"
)

var copyBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 256*1024)
		return &buf
	},
}

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
	// nolint: prealloc
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}
	if err := joinSnapshotErrors(errs); err != nil {
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
	if err := s.precheckTempStorage(ctx, cfg, snapshotCtx); err != nil {
		return err
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	if snapshotCtx.IncludeDB {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := s.createDatabaseBackup(cancelCtx, cfg, snapshotCtx)
			if err != nil {
				cancel()
				logger.Errorw("failed to create database backup", zap.Error(err))
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
				logger.Errorw("failed to create asset backup", zap.Error(err))
				errChan <- fmt.Errorf("failed to create asset backup: %w", err)
			}
		}()
	}

	wg.Wait()
	close(errChan)

	// nolint: prealloc
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}
	if err := joinSnapshotErrors(errs); err != nil {
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

	cred, err := getAWSKeysWithAsumeRole(ctx, cfg.S3)
	if err != nil {
		return fmt.Errorf("failed to get AWS credentials: %w", err)
	}

	workers := util.S3WorkerCount()
	logger.Infow("Init minio client", zap.String("endpoint", cfg.S3.Endpoint), zap.Int("workers", workers))
	minioClient, err := util.NewMinioClient(cfg.S3.Endpoint, cred, true, workers)
	if err != nil {
		return fmt.Errorf("failed to create minio client: %w", err)
	}

	jobs := []util.BucketDownload{
		{Bucket: cfg.S3.PrivateBucket, DestDir: filepath.Join(snapshotCtx.TempArchiveDir, "private")},
		{Bucket: cfg.S3.PublicBucket, DestDir: filepath.Join(snapshotCtx.TempArchiveDir, "public")},
	}
	for _, job := range jobs {
		if job.Bucket == "" {
			continue
		}
		if err := os.MkdirAll(job.DestDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory for bucket backup: %w", err)
		}
	}

	logger.Info("Starting S3 bucket read")
	stats := util.NewTransferStats(logger, "download", workers)
	err = util.DownloadBuckets(ctx, minioClient, workers, jobs, processDownloadObject(logger, stats))
	stats.Finish()
	if err != nil {
		logger.Errorw("bucket backup failed", zap.Error(err))
		return fmt.Errorf("bucket backup: %w", err)
	}
	logger.Info("Done S3 bucket read")
	return nil
}

func processDownloadObject(logger *zap.SugaredLogger, stats *util.TransferStats) func(destDir string, i minio.ObjectInfo, o *minio.Object) error {
	return func(destDir string, i minio.ObjectInfo, o *minio.Object) error {
		//nolint: errcheck
		defer o.Close()

		file := filepath.Join(destDir, i.Key)
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		if strings.HasSuffix(i.Key, "/") && i.Size == 0 {
			return nil
		}

		f, err := os.Create(file)
		if err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}
		defer func() {
			if err := f.Close(); err != nil {
				logger.Warnw("failed to close bucket file", zap.String("file", file), zap.Error(err))
			}
		}()

		bufPtr := copyBufferPool.Get().(*[]byte)
		defer copyBufferPool.Put(bufPtr)

		n, err := io.CopyBuffer(f, o, *bufPtr)
		if err != nil {
			return fmt.Errorf("failed to copy object to file: %w", err)
		}
		stats.Add(n)
		return nil
	}
}

func (s *SnapshotService) createDatabaseBackup(
	ctx context.Context, cfg *config.SnapshotConfig, snapshotCtx *SnapshotContext,
) error {
	logger := logging.FromContext(ctx)

	logger.Info("Starting database backup")
	targetDir := filepath.Join(snapshotCtx.TempArchiveDir, "database")
	databaseSpec, err := databaseSpecFromConfig(cfg.Database)
	if err != nil {
		return err
	}

	dump := util.NewMySQLShell(cfg.Database.MysqlShellBinaryPath)
	out, err := dump.Dump(ctx, util.DumpInput{
		DumpFilePath: targetDir,
		DatabaseSpec: databaseSpec,
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
	databaseSpec, err := databaseSpecFromConfig(cfg.Database)
	if err != nil {
		return err
	}

	shell := util.NewMySQLShell(cfg.Database.MysqlShellBinaryPath)
	err = shell.RestoreDump(ctx, util.RestoreInput{
		DatabaseSpec: databaseSpec,
		DumpFilePath: dumpFilePath,
	})
	if err != nil {
		return fmt.Errorf("failed to restore database: %w", err)
	}

	logger.Info("Finished database restore")
	return nil
}

func databaseSpecFromConfig(database config.DatabaseConfig) (util.DatabaseSpec, error) {
	hasCertificate := database.SSLCert != ""
	hasKey := database.SSLKey != ""
	if hasCertificate != hasKey {
		return util.DatabaseSpec{}, fmt.Errorf("database TLS client certificate and key must either both be set or both be empty")
	}

	return util.DatabaseSpec{
		Host:                           database.Host,
		Password:                       []byte(database.Password),
		User:                           database.User,
		Port:                           database.Port,
		Name:                           database.Database,
		Version:                        database.Version,
		Options:                        database.Options,
		TLSCA:                          []byte(database.SSLCA),
		TLSCert:                        []byte(database.SSLCert),
		TLSKey:                         []byte(database.SSLKey),
		TLSClientCertificate:           hasCertificate,
		TLSDontVerifyServerCertificate: database.SSLDontVerifyServerCert,
	}, nil
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

func (s *SnapshotService) uploadToS3(ctx context.Context, cfg *config.SnapshotConfig, snapshotCtx *SnapshotContext, contentType string, reader io.ReadCloser) error {
	defer func() {
		if err := reader.Close(); err != nil {
			logging.FromContext(ctx).Warnw("failed to close S3 upload reader", zap.Error(err))
		}
	}()

	bucket, objectFile, err := parseS3URL(snapshotCtx.BackupFile)
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

	userTags := map[string]string{
		"object-type": "store-snapshot-backup",
	}
	for k, v := range snapshotCtx.Labels {
		userTags[k] = v
	}

	_, err = minioClient.PutObject(ctx, bucket, objectFile, reader, -1,
		minio.PutObjectOptions{
			ContentType: contentType,
			UserTags:    userTags,
		})
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
