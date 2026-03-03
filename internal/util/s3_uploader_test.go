package util

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunUploadWorkers_ProcessesAllFilesWithBoundedConcurrency(t *testing.T) {
	const (
		fileCount = 200
		workers   = 8
	)

	files := make(chan string, fileCount)
	for i := 0; i < fileCount; i++ {
		files <- "file"
	}
	close(files)

	var processed int64
	var inFlight int64
	var maxInFlight int64

	err := runUploadWorkers(context.Background(), workers, files, func(_ context.Context, _ string) error {
		current := atomic.AddInt64(&inFlight, 1)
		for {
			prev := atomic.LoadInt64(&maxInFlight)
			if current <= prev || atomic.CompareAndSwapInt64(&maxInFlight, prev, current) {
				break
			}
		}

		time.Sleep(200 * time.Microsecond)
		atomic.AddInt64(&processed, 1)
		atomic.AddInt64(&inFlight, -1)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, int64(fileCount), atomic.LoadInt64(&processed))
	assert.LessOrEqual(t, atomic.LoadInt64(&maxInFlight), int64(workers))
}

func TestRunUploadWorkers_ReturnsWorkerError(t *testing.T) {
	files := make(chan string, 2)
	files <- "ok"
	files <- "fail"
	close(files)

	err := runUploadWorkers(context.Background(), 2, files, func(_ context.Context, file string) error {
		if file == "fail" {
			return errors.New("boom")
		}
		return nil
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "boom")
}

func TestRunUploadWorkers_UsesAtLeastOneWorker(t *testing.T) {
	files := make(chan string, 1)
	files <- "file"
	close(files)

	var processed int64
	err := runUploadWorkers(context.Background(), 0, files, func(_ context.Context, _ string) error {
		atomic.AddInt64(&processed, 1)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, int64(1), atomic.LoadInt64(&processed))
}

func BenchmarkRunUploadWorkers_Memory(b *testing.B) {
	const (
		fileCount = 10000
		workers   = 30
	)

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		files := make(chan string, workers)
		go func() {
			for j := 0; j < fileCount; j++ {
				files <- "file"
			}
			close(files)
		}()

		var processed int64
		err := runUploadWorkers(context.Background(), workers, files, func(_ context.Context, _ string) error {
			atomic.AddInt64(&processed, 1)
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}
		if atomic.LoadInt64(&processed) != fileCount {
			b.Fatalf("expected %d processed files, got %d", fileCount, processed)
		}
	}
}

func BenchmarkRunUploadWorkers_WorkerCounts(b *testing.B) {
	const fileCount = 10000

	b.ReportAllocs()

	for _, workers := range []int{5, 15, 30, 60} {
		workers := workers
		b.Run(fmt.Sprintf("workers_%d", workers), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				files := make(chan string, workers)
				go func() {
					for j := 0; j < fileCount; j++ {
						files <- "file"
					}
					close(files)
				}()

				var processed int64
				err := runUploadWorkers(context.Background(), workers, files, func(_ context.Context, _ string) error {
					atomic.AddInt64(&processed, 1)
					return nil
				})
				if err != nil {
					b.Fatal(err)
				}
				if atomic.LoadInt64(&processed) != fileCount {
					b.Fatalf("expected %d processed files, got %d", fileCount, processed)
				}
			}
		})
	}
}
