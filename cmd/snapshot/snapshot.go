package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/shopware/shopware-operator/internal/config"
	"github.com/shopware/shopware-operator/internal/logging"
	"github.com/shopware/shopware-operator/internal/snapshot"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

var (
	version    = "dev"
	commit     = "none"
	date       = "unknown"
	backupFile = fmt.Sprintf("shopware-snapshot-%s.zip", time.Now().Format("2006-01-02-15-04-05"))
	tempDir    = os.TempDir()
	includeDB  = true
	includeS3  = true
)

func main() {
	ctx := context.Background()
	cfg, err := config.LoadSnapshotConfig(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(11)
	}

	logger := logging.NewLogger(cfg.LogLevel, cfg.LogFormat).
		With(zap.String("service", "shopware-snapshot")).
		With(zap.String("operator_version", version)).
		With(zap.String("operator_date", date)).
		With(zap.String("operator_commit", commit))

	ctx = logging.WithLogger(ctx, logger)
	logger.Debugw("Configuration loaded", zap.Any("config", cfg))

	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "tempdir",
				Value:       tempDir,
				Usage:       "Directory for a temporary snapshot",
				Destination: &tempDir,
			},
			&cli.StringFlag{
				Name:        "backup-file",
				Value:       backupFile,
				Usage:       "File for the snapshot, or s3 URL",
				Destination: &backupFile,
			},
			&cli.BoolFlag{
				Name:        "include-db",
				Usage:       "Include database backup",
				Value:       true,
				Destination: &includeDB,
			},
			&cli.BoolFlag{
				Name:        "include-s3",
				Usage:       "Include S3 backup",
				Value:       true,
				Destination: &includeS3,
			},
		},
		Name:  "snpashot",
		Usage: "creates an snapshot for a given shopware store",
		Commands: []*cli.Command{
			{
				Name:  "create",
				Usage: "Create a snapshot",
				Action: func(ctx context.Context, c *cli.Command) error {
					snapshotDir, err := os.MkdirTemp(tempDir, "snapshot-")
					if err != nil {
						logger.Errorw("TempDir creation failed", zap.Error(err))
						os.Exit(14)
					}
					//nolint: errcheck
					defer os.RemoveAll(snapshotDir)

					var tempMeta any
					err = json.Unmarshal([]byte(cfg.MetaStoreJson), &tempMeta)
					if err != nil {
						logger.Warnw("MetaStoreJson unmarshal failed, skip metadata creation", zap.Error(err))
					}
					b, err := json.MarshalIndent(tempMeta, "", "  ")
					if err != nil {
						logger.Warnw("MetaStoreJson parsing failed, skip metadata creation", zap.Error(err))
					}
					err = os.WriteFile(fmt.Sprintf("%s/metadata.json", snapshotDir), b, 0644)
					if err != nil {
						logger.Warnw("Write metadata.json failed, skip metadata creation", zap.Error(err))
					}

					snapshotService := snapshot.NewSnapshotService(cfg)
					logger.Infow("Starting snapshot create process",
						zap.String("backupFile", backupFile),
						zap.String("temp", snapshotDir),
						zap.Bool("includeDB", includeDB),
						zap.Bool("includeS3", includeS3),
					)

					now := time.Now()
					if err := snapshotService.CreateBackup(ctx, cfg, &snapshot.SnapshotContext{
						BackupFile:     backupFile,
						TempArchiveDir: snapshotDir,
						IncludeDB:      includeDB,
						IncludeS3:      includeS3,
					}); err != nil {
						logger.Errorw("Backup failed", zap.Error(err))
						return fmt.Errorf("backup failed: %w", err)
					}

					logger.Infow("Backup completed successfully",
						zap.Duration("duration", time.Since(now)),
					)

					return nil
				},
			},
			{
				Name:  "restore",
				Usage: "Restore a snapshot",
				Action: func(ctx context.Context, c *cli.Command) error {
					snapshotTempDir, err := os.MkdirTemp("", "snapshot-")
					if err != nil {
						logger.Errorw("TempDir creation failed", zap.Error(err))
						os.Exit(14)
					}
					//nolint: errcheck
					defer os.RemoveAll(snapshotTempDir)

					var tempMeta any
					json.Unmarshal([]byte(cfg.MetaStoreJson), &tempMeta)
					b, err := json.MarshalIndent(tempMeta, "", "  ")
					if err != nil {
						logger.Warnw("MetaStoreJson parsing failed, skip metadata creation", zap.Error(err))
					}
					err = os.WriteFile(fmt.Sprintf("%s/metadata.json", snapshotTempDir), b, 0644)
					if err != nil {
						logger.Warnw("Write metadata.json failed, skip metadata creation", zap.Error(err))
					}

					snapshotService := snapshot.NewSnapshotService(cfg)
					logger.Infow("Starting snapshot restore process",
						zap.String("backupFile", backupFile),
						zap.String("temp", snapshotTempDir),
						zap.Bool("includeDB", includeDB),
						zap.Bool("includeS3", includeS3),
					)

					now := time.Now()
					if err := snapshotService.RestoreBackup(ctx, cfg, &snapshot.SnapshotContext{
						BackupFile:     backupFile,
						TempArchiveDir: snapshotTempDir,
						IncludeDB:      includeDB,
						IncludeS3:      includeS3,
					}); err != nil {
						logger.Errorw("Restore failed", zap.Error(err))
						return fmt.Errorf("restore failed: %w", err)
					}

					logger.Infow("Backup restore successfully",
						zap.Duration("duration", time.Since(now)),
					)
					return nil
				},
			},
		},
	}

	if err := cmd.Run(ctx, os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error running command: %v\n", err)
		os.Exit(1)
	}
}
