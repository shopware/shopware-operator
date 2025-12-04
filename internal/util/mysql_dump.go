package util

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/shopware/shopware-operator/internal/logging"
)

type MySQLDump struct {
	binaryPath string
}

func NewMySQLDump(binaryPath string) MySQLDump {
	return MySQLDump{binaryPath: binaryPath}
}

func (h MySQLDump) Dump(
	ctx context.Context,
	input DumpInput,
	writer io.WriteCloser,
) (*DumpOutput, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("empty database name")
	}

	if input.Host == "" {
		return nil, fmt.Errorf("empty host")
	}

	if string(input.Password) == "" {
		return nil, fmt.Errorf("empty password")
	}

	if input.User == "" {
		return nil, fmt.Errorf("empty user")
	}

	duration := time.Now()
	cmd := exec.Command(
		"mysqldump",
		"--column-statistics=0",
		"--set-gtid-purged=OFF",
		"--hex-blob",
		"--skip-set-charset",
		"-h",
		input.Host,
		"-u",
		input.User,
		fmt.Sprintf("-p%s", input.Password),
		input.Name,
	)

	logging.FromContext(ctx).Debugw("mysqldump command", "host", input.Host, "user", input.User, "database", input.Name)

	dump, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("get stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start command: %w", err)
	}

	// Read stderr concurrently to prevent deadlock
	stderrChan := make(chan []byte, 1)
	go func() {
		stderrOutput, _ := io.ReadAll(stderrPipe)
		stderrChan <- stderrOutput
	}()

	// Own writer to count the gzipped size
	counterWriter := &countingWriter{w: writer}
	gw, err := gzip.NewWriterLevel(counterWriter, gzip.BestSpeed)
	if err != nil {
		return nil, fmt.Errorf("create gzip writer: %w", err)
	}

	uncompressedCount, err := io.CopyBuffer(gw, dump, make([]byte, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("copy to gzip writer: %w", err)
	}

	err = cmd.Wait()
	stderrOutput := <-stderrChan
	if err != nil {
		if len(stderrOutput) > 0 {
			return nil, fmt.Errorf("wait command: %w, stderr: %s", err, string(stderrOutput))
		}
		return nil, fmt.Errorf("wait command: %w", err)
	}

	if len(stderrOutput) > 0 {
		logging.FromContext(ctx).Warnw("mysqldump stderr", "output", string(stderrOutput))
	}

	err = gw.Close()
	if err != nil {
		return nil, fmt.Errorf("close gzip writer: %w", err)
	}

	err = writer.Close()
	if err != nil {
		return nil, fmt.Errorf("close writer: %w", err)
	}

	return &DumpOutput{
		Duration:         time.Since(duration),
		UncompressedSize: uncompressedCount,
		CompressedSize:   counterWriter.count,
		CompressionRatio: float64(counterWriter.count) / float64(uncompressedCount),
	}, nil
}

func (h MySQLDump) Restore(
	ctx context.Context,
	input DumpInput,
	reader io.ReadCloser,
) error {
	if input.Name == "" {
		return fmt.Errorf("empty database name")
	}

	if input.Host == "" {
		return fmt.Errorf("empty host")
	}

	if string(input.Password) == "" {
		return fmt.Errorf("empty password")
	}

	if input.User == "" {
		return fmt.Errorf("empty user")
	}

	cmd := exec.Command(
		"mysql",
		"-A", // Quicker startup because skipping table sync
		"-h",
		input.Host,
		"-u",
		input.User,
		fmt.Sprintf("-p%s", input.Password),
		input.Name,
	)

	logging.FromContext(ctx).Debugw("mysql restore command", "host", input.Host, "user", input.User, "database", input.Name)

	var err error
	cmd.Stdin, err = gzip.NewReader(reader)
	if err != nil {
		return fmt.Errorf("create gzip reader: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start command: %w", err)
	}

	// Read stderr concurrently to prevent deadlock
	stderrChan := make(chan []byte, 1)
	go func() {
		stderrOutput, _ := io.ReadAll(stderrPipe)
		stderrChan <- stderrOutput
	}()

	err = cmd.Wait()
	stderrOutput := <-stderrChan
	if err != nil {
		if len(stderrOutput) > 0 {
			return fmt.Errorf("wait command: %w, stderr: %s", err, string(stderrOutput))
		}
		return fmt.Errorf("wait command: %w", err)
	}

	if len(stderrOutput) > 0 {
		logging.FromContext(ctx).Warnw("mysql restore stderr", "output", string(stderrOutput))
	}

	err = reader.Close()
	if err != nil {
		return fmt.Errorf("close gzip reader: %w", err)
	}

	return nil
}

type countingWriter struct {
	w     io.Writer
	count int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.count += int64(n)
	return n, err
}
