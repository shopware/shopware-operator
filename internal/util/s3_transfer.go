package util

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"
)

const (
	minS3Workers      = 32
	transferLogEveryN = 10000
	transferLogEvery  = 60 * time.Second
)

// S3WorkerCount is the concurrency for many-object bucket download/upload.
// I/O-bound S3 GETs/PUTs should not be capped at GOMAXPROCS (often 1 in a
// CPU-limited Job). Floor is 32 to match the previous downloadBucket constant.
func S3WorkerCount() int {
	return s3WorkerCount(runtime.GOMAXPROCS(0))
}

func s3WorkerCount(maxProcs int) int {
	n := 2 * maxProcs
	if n < minS3Workers {
		return minS3Workers
	}
	return n
}

// NewMinioClient builds a MinIO client. When workers > 0 the HTTP transport
// keeps at least that many idle connections per host so concurrent GETs/PUTs
// reuse TLS sessions.
func NewMinioClient(endpoint string, creds *credentials.Credentials, secure bool, workers int) (*minio.Client, error) {
	opts := &minio.Options{
		Creds:  creds,
		Secure: secure,
	}
	if workers > 0 {
		tr, err := minioTransport(secure, workers)
		if err != nil {
			return nil, err
		}
		opts.Transport = tr
	}
	return minio.New(endpoint, opts)
}

func minioTransport(secure bool, workers int) (*http.Transport, error) {
	tr, err := minio.DefaultTransport(secure)
	if err != nil {
		return nil, fmt.Errorf("minio transport: %w", err)
	}
	if workers > tr.MaxIdleConnsPerHost {
		tr.MaxIdleConnsPerHost = workers
	}
	if idle := workers * 2; idle > tr.MaxIdleConns {
		tr.MaxIdleConns = idle
	}
	return tr, nil
}

// TransferStats logs many-object S3 throughput (objects, bytes, MiB/s).
type TransferStats struct {
	logger  *zap.SugaredLogger
	op      string
	workers int
	start   time.Time
	objects int64
	bytes   int64
	lastLog atomic.Int64
}

func NewTransferStats(logger *zap.SugaredLogger, op string, workers int) *TransferStats {
	now := time.Now()
	s := &TransferStats{
		logger:  logger,
		op:      op,
		workers: workers,
		start:   now,
	}
	s.lastLog.Store(now.UnixNano())
	logger.Infow("Starting S3 transfer",
		zap.String("op", op),
		zap.Int("workers", workers),
	)
	return s
}

func (s *TransferStats) Add(n int64) {
	if s == nil {
		return
	}
	objects := atomic.AddInt64(&s.objects, 1)
	total := atomic.AddInt64(&s.bytes, n)
	now := time.Now()
	last := s.lastLog.Load()
	if objects%transferLogEveryN != 0 && now.Sub(time.Unix(0, last)) < transferLogEvery {
		return
	}
	if !s.lastLog.CompareAndSwap(last, now.UnixNano()) {
		return
	}
	s.log(now, objects, total, "S3 transfer progress")
}

func (s *TransferStats) Finish() {
	if s == nil {
		return
	}
	s.log(time.Now(), atomic.LoadInt64(&s.objects), atomic.LoadInt64(&s.bytes), "S3 transfer finished")
}

func (s *TransferStats) log(now time.Time, objects, bytes int64, msg string) {
	elapsed := now.Sub(s.start)
	mibps := 0.0
	if secs := elapsed.Seconds(); secs > 0 {
		mibps = math.Round((float64(bytes)/secs)/(1024*1024)*100) / 100
	}
	s.logger.Infow(msg,
		zap.String("op", s.op),
		zap.Int("workers", s.workers),
		zap.Int64("objects", objects),
		zap.String("bytes", formatTransferBytes(bytes)),
		zap.Float64("miBps", mibps),
		zap.Duration("elapsed", elapsed),
	)
}

func formatTransferBytes(n int64) string {
	if n < 0 {
		return "-" + formatTransferBytes(-n)
	}
	const (
		ki = 1024
		mi = ki * 1024
		gi = mi * 1024
		ti = gi * 1024
	)
	switch {
	case n >= ti:
		return formatTransferByteUnit(n, ti, "Ti")
	case n >= gi:
		return formatTransferByteUnit(n, gi, "Gi")
	case n >= mi:
		return formatTransferByteUnit(n, mi, "Mi")
	case n >= ki:
		return formatTransferByteUnit(n, ki, "Ki")
	default:
		return fmt.Sprintf("%dB", n)
	}
}

func formatTransferByteUnit(n, unit int64, suffix string) string {
	v := float64(n) / float64(unit)
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d%s", int64(v), suffix)
	}
	return fmt.Sprintf("%.1f%s", v, suffix)
}

// BucketDownload is one bucket to pull into DestDir with a shared worker pool.
type BucketDownload struct {
	Bucket  string
	DestDir string
}

type downloadTask struct {
	Bucket  string
	DestDir string
	Object  minio.ObjectInfo
}

// DownloadBuckets lists every bucket and processes objects with a single shared
// worker pool so a small private bucket does not idle half the workers.
func DownloadBuckets(
	ctx context.Context,
	client *minio.Client,
	workers int,
	jobs []BucketDownload,
	f func(destDir string, info minio.ObjectInfo, obj *minio.Object) error,
) error {
	if workers <= 0 {
		workers = 1
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	taskCh := make(chan downloadTask, workers)
	var listErr atomic.Value
	var listWg sync.WaitGroup

	for _, job := range jobs {
		if job.Bucket == "" {
			continue
		}
		job := job
		listWg.Add(1)
		go func() {
			defer listWg.Done()
			objects := client.ListObjects(ctx, job.Bucket, minio.ListObjectsOptions{
				Recursive: true,
			})
			for object := range objects {
				if object.Err != nil {
					listErr.Store(fmt.Errorf("error listing objects in %s: %w", job.Bucket, object.Err))
					cancel()
					return
				}
				select {
				case <-ctx.Done():
					return
				case taskCh <- downloadTask{Bucket: job.Bucket, DestDir: job.DestDir, Object: object}:
				}
			}
		}()
	}

	go func() {
		listWg.Wait()
		close(taskCh)
	}()

	workerErr := runWorkers(ctx, workers, taskCh, func(ctx context.Context, task downloadTask) error {
		r, err := client.GetObject(ctx, task.Bucket, task.Object.Key, minio.GetObjectOptions{})
		if err != nil {
			return fmt.Errorf("error downloading %s/%s: %w", task.Bucket, task.Object.Key, err)
		}
		if err := f(task.DestDir, task.Object, r); err != nil {
			return fmt.Errorf("error processing object %s/%s: %w", task.Bucket, task.Object.Key, err)
		}
		return nil
	})

	stored, _ := listErr.Load().(error)
	if workerErr != nil {
		if errors.Is(workerErr, context.Canceled) && stored != nil {
			return stored
		}
		return workerErr
	}
	return stored
}
