package snapshot

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/shopware/shopware-operator/internal/config"
	"github.com/shopware/shopware-operator/internal/logging"
	"go.uber.org/zap"
)

func (s *SnapshotService) ExtractArchive(ctx context.Context, cfg *config.SnapshotConfig, snapshotCtx *SnapshotContext) error {
	logger := logging.FromContext(ctx)
	logger.Infow("Extracting archive", zap.String("source", snapshotCtx.BackupFile), zap.String("destDir", snapshotCtx.TempArchiveDir))

	// Check if BackupFile is an S3 URL
	_, _, parseErr := parseS3URL(snapshotCtx.BackupFile)
	isS3URL := parseErr == nil

	if isS3URL {
		logger.Infow("Detected S3 URL, downloading archive from S3", zap.String("s3URL", snapshotCtx.BackupFile))
		return s.extractArchiveFromS3(ctx, cfg, snapshotCtx)
	}

	logger.Infow("Extracting from local archive file", zap.String("localFile", snapshotCtx.BackupFile))
	return s.extractLocalArchive(ctx, snapshotCtx)
}

func (s *SnapshotService) extractLocalArchive(ctx context.Context, snapshotCtx *SnapshotContext) error {
	logger := logging.FromContext(ctx)
	r, err := zip.OpenReader(snapshotCtx.BackupFile)
	if err != nil {
		return fmt.Errorf("failed to open zip file %s: %w", snapshotCtx.BackupFile, err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			logger.Warnw("failed to close zip reader", zap.Error(err))
		}
	}()

	return s.extractZipFiles(r.File, snapshotCtx.TempArchiveDir)
}

func (s *SnapshotService) extractArchiveFromS3(ctx context.Context, cfg *config.SnapshotConfig, snapshotCtx *SnapshotContext) error {
	logger := logging.FromContext(ctx)

	// Download archive from S3
	reader, err := s.downloadFromS3(ctx, cfg, snapshotCtx.BackupFile)
	if err != nil {
		return fmt.Errorf("failed to download archive from S3: %w", err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			logger.Warnw("failed to close S3 reader", zap.Error(err))
		}
	}()

	// Create temporary file to stream the archive to disk instead of memory
	tempFile, err := os.CreateTemp("", "snapshot-archive-*.zip")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		if err := os.Remove(tempFile.Name()); err != nil {
			logger.Warnw("failed to remove temp file", zap.String("file", tempFile.Name()), zap.Error(err))
		}
	}() // Clean up temp file
	defer func() {
		if err := tempFile.Close(); err != nil {
			logger.Warnw("failed to close temp file", zap.Error(err))
		}
	}()

	// Stream from S3 to temp file
	written, err := io.Copy(tempFile, reader)
	if err != nil {
		return fmt.Errorf("failed to stream archive to temp file: %w", err)
	}

	logger.Infow("Downloaded archive from S3 to temp file", zap.String("s3URL", snapshotCtx.BackupFile), zap.Int64("archiveSize", written), zap.String("tempFile", tempFile.Name()))

	// Close the temp file so we can open it with zip.OpenReader
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Open zip file from temp file
	zipReader, err := zip.OpenReader(tempFile.Name())
	if err != nil {
		return fmt.Errorf("failed to open temp zip file: %w", err)
	}
	defer func() {
		if err := zipReader.Close(); err != nil {
			logger.Warnw("failed to close zip reader", zap.Error(err))
		}
	}()

	return s.extractZipFiles(zipReader.File, snapshotCtx.TempArchiveDir)
}

func (s *SnapshotService) extractZipFiles(files []*zip.File, destDir string) error {
	for _, f := range files {
		fpath := filepath.Join(destDir, f.Name)

		// Prevent ZipSlip (https://snyk.io/research/zip-slip-vulnerability)
		if !strings.HasPrefix(fpath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", fpath)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, 0755); err != nil {
				return fmt.Errorf("failed to create dir %s: %w", fpath, err)
			}
			continue
		}

		// Ensure parent directories exist
		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return fmt.Errorf("failed to create parent dir for %s: %w", fpath, err)
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open file in zip %s: %w", f.Name, err)
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			//nolint: errcheck
			rc.Close()
			return fmt.Errorf("failed to create file %s: %w", fpath, err)
		}

		_, err = io.Copy(outFile, rc)

		// Close in correct order
		if err := outFile.Close(); err != nil {
			//nolint: errcheck
			rc.Close()
			return fmt.Errorf("failed to close output file %s: %w", fpath, err)
		}
		if err := rc.Close(); err != nil {
			return fmt.Errorf("failed to close zip file reader for %s: %w", f.Name, err)
		}

		if err != nil {
			return fmt.Errorf("failed to extract file %s: %w", fpath, err)
		}
	}

	return nil
}

func (s *SnapshotService) CreateArchive(ctx context.Context, cfg *config.SnapshotConfig, snapshotCtx *SnapshotContext) error {
	logger := logging.FromContext(ctx)
	logger.Infow("Creating archive", zap.String("sourceDir", snapshotCtx.TempArchiveDir), zap.String("targetFile", snapshotCtx.BackupFile))

	// Check if BackupFile is an S3 URL
	_, _, parseErr := parseS3URL(snapshotCtx.BackupFile)
	isS3URL := parseErr == nil

	if isS3URL {
		logger.Infow("Detected S3 URL, creating archive and uploading to S3", zap.String("s3URL", snapshotCtx.BackupFile))
		return s.createArchiveAndUploadToS3(ctx, cfg, snapshotCtx)
	}

	logger.Infow("Creating local archive file", zap.String("localFile", snapshotCtx.BackupFile))
	return s.createLocalArchive(ctx, snapshotCtx)
}

func (s *SnapshotService) createLocalArchive(ctx context.Context, snapshotCtx *SnapshotContext) error {
	logger := logging.FromContext(ctx)

	zipFile, err := os.Create(snapshotCtx.BackupFile)
	if err != nil {
		logger.Errorw("failed to create zip file", zap.Error(err))
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	//nolint: errcheck
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	//nolint: errcheck
	defer zipWriter.Close()

	return s.addFilesToZip(zipWriter, snapshotCtx.TempArchiveDir)
}

func (s *SnapshotService) createArchiveAndUploadToS3(ctx context.Context, cfg *config.SnapshotConfig, snapshotCtx *SnapshotContext) error {
	logger := logging.FromContext(ctx)

	// Create archive in memory
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	err := s.addFilesToZip(zipWriter, snapshotCtx.TempArchiveDir)
	if err != nil {
		return fmt.Errorf("failed to create zip archive: %w", err)
	}

	err = zipWriter.Close()
	if err != nil {
		return fmt.Errorf("failed to close zip writer: %w", err)
	}

	// Upload to S3
	reader := io.NopCloser(bytes.NewReader(buf.Bytes()))
	logger.Infow("Uploading archive to S3", zap.String("s3URL", snapshotCtx.BackupFile), zap.Int("archiveSize", buf.Len()))

	err = s.uploadToS3(ctx, cfg, snapshotCtx.BackupFile, "application/zip", reader)
	if err != nil {
		return fmt.Errorf("failed to upload archive to S3: %w", err)
	}

	logger.Infow("Successfully uploaded archive to S3", zap.String("s3URL", snapshotCtx.BackupFile))
	return nil
}

func (s *SnapshotService) addFilesToZip(zipWriter *zip.Writer, sourceDir string) error {
	return filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, errWalk error) error {
		if d.IsDir() {
			return nil
		}

		if errWalk != nil {
			return nil
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", path, err)
		}

		relPath = filepath.ToSlash(relPath)

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("failed to get file info for %s: %w", path, err)
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return fmt.Errorf("failed to create zip header for %s: %w", path, err)
		}
		header.Name = relPath
		header.Method = zip.Deflate // Compression

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("failed to create zip header for %s: %w", relPath, err)
		}

		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", path, err)
		}
		//nolint: errcheck
		defer file.Close()

		_, err = io.Copy(writer, file)
		if err != nil {
			return fmt.Errorf("failed to copy file %s to zip: %w", path, err)
		}
		return nil
	})
}
