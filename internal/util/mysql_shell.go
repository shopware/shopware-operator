package util

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/inhies/go-bytesize"
	"github.com/shopware/shopware-operator/internal/logging"
	"go.uber.org/zap"
)

type MySQLShell struct {
	binaryPath string
}

func NewMySQLShell(binaryPath string) MySQLShell {
	return MySQLShell{binaryPath: binaryPath}
}

// func (h MySQLShell) Copy(ctx context.Context, snap v1.StoreSnapshot) error {
// 	dbName := "shopware"
// 	if input.DatabaseName == "" {
// 		return fmt.Errorf("empty database name")
// 	}
//
// 	if input.Credentials.Host == "" {
// 		return fmt.Errorf("empty source database host")
// 	}
//
// 	if input.TargetDatabaseHost == "" {
// 		return fmt.Errorf("empty target database host")
// 	}
//
// 	ctx = logging.WithLogger(ctx, logging.FromContext(ctx).With(
// 		zap.String("database_name", input.DatabaseName),
// 	))
//
// 	tmpl, err := template.New("cmd").Parse(`
//         util.copySchemas(["{{.DatabaseName}}"], '{{.TargetDatabaseUser}}:{{.TargetDatabasePassword}}@{{.TargetDatabaseHost}}', {"showProgress": "false"});
// 	`)
// 	if err != nil {
// 		return fmt.Errorf("parse template: %w", err)
// 	}
//
// 	var tpl bytes.Buffer
//
// 	err = tmpl.Execute(&tpl, input)
// 	if err != nil {
// 		return fmt.Errorf("execute template: %w", err)
// 	}
//
// 	_, err = h.run(ctx, input.Credentials, tpl.Bytes())
// 	if err != nil {
// 		return fmt.Errorf("run command: %w", err)
// 	}
//
// 	return nil
// }
//
// func (h MySQLShell) RestoreDump(
// 	ctx context.Context,
// 	input command.RestoreInput,
// ) (command.RestoreResponse, error) {
// 	var resp command.RestoreResponse
// 	if input.Credentials.DatabaseName == "" {
// 		return resp, fmt.Errorf("empty database name")
// 	}
//
// 	if input.S3Bucket == "" {
// 		return resp, fmt.Errorf("empty s3 bucket")
// 	}
//
// 	if input.DumpPath == "" {
// 		return resp, fmt.Errorf("empty dump name")
// 	}
//
// 	ctx = logging.WithLogger(ctx, logging.FromContext(ctx).With(
// 		zap.String("database_name", input.DatabaseName),
// 		zap.String("dump_path", input.DumpPath),
// 	))
//
// 	tmpl, err := template.New("cmd").Parse(`
//     session.runSql('SET FOREIGN_KEY_CHECKS = 0');
// 	tables = session.runSql('SHOW TABLES').fetchAll();
//     for(var index in tables) {
//         session.runSql('DROP TABLE IF EXISTS ' +  mysql.quoteIdentifier(tables[index][0]));
//     }
//     session.runSql('SET FOREIGN_KEY_CHECKS = 1');
//     util.loadDump("{{.DumpPath}}", {
//         "s3BucketName": "{{.S3Bucket}}", "s3Region": "` + h.region + `",
//         "showProgress": "false", "resetProgress": "true",
// 		"progressFile": "",
//         "schema": "{{.Credentials.DatabaseName}}"});
// 	`)
// 	if err != nil {
// 		return resp, fmt.Errorf("parse template: %w", err)
// 	}
//
// 	var tpl bytes.Buffer
// 	err = tmpl.Execute(&tpl, input)
// 	if err != nil {
// 		return resp, fmt.Errorf("execute template: %w", err)
// 	}
//
// 	start := time.Now()
//
// 	_, err = h.run(ctx, input.Credentials, tpl.Bytes())
// 	if err != nil {
// 		return resp, fmt.Errorf("run command: %w", err)
// 	}
//
// 	resp.Duration = time.Since(start)
//
// 	return resp, nil
// }

type DatabaseSpec struct {
	Host     string
	Password []byte
	User     string
	Port     int32
	Name     string
	Version  string
	SSLMode  string
	Options  string
}

type DumpInput struct {
	DumpFilePath string
	DatabaseSpec
}

type DumpOutput struct {
	Duration          time.Duration
	UncompressedSize  int64
	CompressedSize    int64
	CompressionRation float64
}

type RestoreInput struct {
	DatabaseSpec
	DumpFilePath string
}

func (h MySQLShell) Dump(
	ctx context.Context,
	input DumpInput,
) (DumpOutput, error) {
	var resp DumpOutput
	if input.Name == "" {
		return resp, fmt.Errorf("empty database name")
	}

	// 	tmpl, err := template.New("cmd").Parse(`
	// util.dumpSchemas(["{{.Name}}"], "{{.DumpFilePath}}", {
	// 	"s3BucketName": "{{.S3Bucket}}", "s3Region": "` + h.region + `",
	//      "consistent": {{.Consistent}}, compression: "zstd", "showProgress": "false"});
	// `)
	tmpl, err := template.New("cmd").Parse(`
util.dumpSchemas(["{{.Name}}"], "{{.DumpFilePath}}", {
     "consistent": true, compression: "zstd", "showProgress": false});
`)
	if err != nil {
		return resp, fmt.Errorf("parse template: %w", err)
	}

	var tpl bytes.Buffer
	err = tmpl.Execute(&tpl, input)
	if err != nil {
		return resp, fmt.Errorf("execute template: %w", err)
	}

	incompleteDump := true

	defer func() {
		if !incompleteDump {
			return
		}
		logging.FromContext(ctx).Warn("database dump is incomplete. Deleting dump directory")
		err := os.RemoveAll(input.DumpFilePath)
		if err != nil {
			logging.FromContext(ctx).Warnw("Delete dump directory failed", zap.Error(err))
		}
	}()

	output, err := h.run(ctx, input.DatabaseSpec, tpl.Bytes())
	if err != nil {
		return resp, fmt.Errorf("run command: %w", err)
	}

	outputStr := string(output)
	logging.FromContext(ctx).Debugw("mysql-shell dump output",
		zap.String("output", outputStr),
		zap.Int("length", len(outputStr)))

	resp, err = parseDumpOutput(outputStr)
	if err != nil {
		// Output parsing failed but dump may have succeeded - check if dump directory was created
		if _, statErr := os.Stat(input.DumpFilePath); statErr == nil {
			logging.FromContext(ctx).Debugw("Dump directory exists, treating as success despite parse failure", zap.Error(err))
			incompleteDump = false
			return DumpOutput{}, nil
		}
		return resp, fmt.Errorf("parse dump output: %w", err)
	}

	incompleteDump = false

	return resp, nil
}

func (h MySQLShell) RestoreDump(
	ctx context.Context,
	input RestoreInput,
) error {
	tmpl, err := template.New("cmd").Parse(`
    session.runSql('CREATE DATABASE IF NOT EXISTS ` + "`{{.Name}}`" + `');
    session.runSql('USE ` + "`{{.Name}}`" + `');
    util.loadDump("{{.DumpFilePath}}", {
        "schema": "{{.Name}}", 
        "showProgress": "false"
    });
	`)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	var tpl bytes.Buffer
	err = tmpl.Execute(&tpl, input)
	if err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	_, err = h.run(ctx, input.DatabaseSpec, tpl.Bytes())
	if err != nil {
		return fmt.Errorf("run command: %w", err)
	}

	return nil
}

func (h MySQLShell) run(
	ctx context.Context,
	db DatabaseSpec,
	jsCmd []byte,
) ([]byte, error) {
	var err error
	var binaryPath string
	if strings.Contains(h.binaryPath, "/") {
		binaryPath, err = filepath.Abs(h.binaryPath)
		if err != nil {
			return nil, fmt.Errorf("get absolute path of binary: %w", err)
		}
		logging.FromContext(ctx).Debugw("use relative path for mysqlsh binary", zap.String("path", binaryPath))
	} else {
		binaryPath, err = exec.LookPath(h.binaryPath)
		if err != nil {
			return nil, fmt.Errorf("find binary in PATH: %w", err)
		}
		logging.FromContext(ctx).Debugw("use command for mysqlsh binary", zap.String("path", binaryPath))
	}

	// Build MySQL URI with password
	// Format: mysql://user:password@host:port/schema
	port := db.Port
	if port == 0 {
		port = 3306
	}
	uri := fmt.Sprintf("mysql://%s:%s@%s:%d/%s", db.User, string(db.Password), db.Host, port, db.Name)

	//nolint:gosec
	cmd := exec.CommandContext(ctx,
		binaryPath,
		uri,
		"--js",
	)

	// Set stdin to the JavaScript command
	cmd.Stdin = bytes.NewReader(append(jsCmd, '\n'))

	// CombinedOutput captures both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		logging.FromContext(ctx).Errorw("mysql-shell",
			zap.String("output", string(output)),
			zap.Error(err))
		return nil, fmt.Errorf("run command: %w, output: %s", err, string(output))
	}

	logging.FromContext(ctx).Debugw("mysql-shell",
		zap.String("host", db.Host),
		zap.String("user", db.User),
		zap.String("schema", db.Name),
		zap.String("output", string(output)),
		zap.Int("output_len", len(output)))

	return output, nil
}

//	func (h MySQLShell) deleteRecursive(ctx context.Context, bucket string, path string) {
//		var nextContinuationToken *string
//		for {
//			resp, err := h.s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
//				Bucket:            aws.String(bucket),
//				Prefix:            aws.String(path),
//				ContinuationToken: nextContinuationToken,
//			})
//			if err != nil {
//				logging.FromContext(ctx).
//					Error("list objects for dump cleanup", zap.NamedError("error.message", err))
//				return
//			}
//
//			var objects []types.ObjectIdentifier
//			for _, obj := range resp.Contents {
//				objects = append(objects, types.ObjectIdentifier{
//					Key: obj.Key,
//				})
//			}
//
//			if len(objects) == 0 {
//				break
//			}
//
//			_, err = h.s3Client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
//				Bucket: aws.String(bucket),
//				Delete: &types.Delete{
//					Objects: objects,
//				},
//			})
//			if err != nil {
//				logging.FromContext(ctx).
//					Error("delete objects for dump cleanup", zap.NamedError("error.message", err))
//				return
//			}
//
//			if *resp.IsTruncated {
//				nextContinuationToken = resp.NextContinuationToken
//				continue
//			}
//
//			break
//		}
//	}

func parseDumpOutput(output string) (DumpOutput, error) {
	var result DumpOutput

	durationRegexp := regexp.MustCompile(`Dump duration: (\d{2}:\d{2}:\d{2})s`)
	matches := durationRegexp.FindStringSubmatch(output)

	if len(matches) != 2 {
		return result, fmt.Errorf("no duration found")
	}

	duration, err := parseDuration(matches[1])
	if err != nil {
		return result, fmt.Errorf("parse duration: %w", err)
	}
	result.Duration = duration

	uncompressedRegexp := regexp.MustCompile(
		`Uncompressed data size: (\d+\.?\d*\s?([KMGT]B|bytes))`,
	)
	matches = uncompressedRegexp.FindStringSubmatch(output)
	if len(matches) < 2 {
		return result, fmt.Errorf("no uncompressed size found")
	}

	uncompressed, err := parseSize(matches[1])
	if err != nil {
		return result, fmt.Errorf("parse uncompressed size: %w", err)
	}
	result.UncompressedSize = uncompressed

	compressedRegexp := regexp.MustCompile(`Compressed data size: (\d+\.?\d*\s?([KMGT]B|bytes))`)
	matches = compressedRegexp.FindStringSubmatch(output)
	if len(matches) < 2 {
		return result, fmt.Errorf("no compressed size found")
	}

	compressed, err := parseSize(matches[1])
	if err != nil {
		return result, fmt.Errorf("parse compressed size: %w", err)
	}
	result.CompressedSize = compressed

	if uncompressed != 0 && compressed != 0 {
		result.CompressionRation = float64(uncompressed) / float64(compressed)
	}

	return result, nil
}

func parseDuration(str string) (time.Duration, error) {
	return time.ParseDuration(str[0:2] + "h" + str[3:5] + "m" + str[6:8] + "s")
}

func parseSize(s string) (int64, error) {
	size, err := bytesize.Parse(s)
	if err != nil {
		return 0, fmt.Errorf("parse size: %w", err)
	}
	u := size.Format("%.0f", "B", false)

	sizeInt, err := strconv.Atoi(strings.TrimSuffix(u, "B"))
	if err != nil {
		return 0, fmt.Errorf("convert to int: %w", err)
	}

	return int64(sizeInt), nil
}
