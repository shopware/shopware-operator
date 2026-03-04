package util

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunDownloadWorkers_ProcessesAllObjectsWithBoundedConcurrency(t *testing.T) {
	const (
		objectCount = 200
		workers     = 8
	)

	objects := make(chan minio.ObjectInfo, objectCount)
	for i := 0; i < objectCount; i++ {
		objects <- minio.ObjectInfo{Key: "obj"}
	}
	close(objects)

	var processed int64
	var inFlight int64
	var maxInFlight int64

	err := runDownloadWorkers(context.Background(), workers, objects, func(_ context.Context, _ minio.ObjectInfo) error {
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
	assert.Equal(t, int64(objectCount), atomic.LoadInt64(&processed))
	assert.LessOrEqual(t, atomic.LoadInt64(&maxInFlight), int64(workers))
}

func TestRunDownloadWorkers_ReturnsWorkerError(t *testing.T) {
	objects := make(chan minio.ObjectInfo, 2)
	objects <- minio.ObjectInfo{Key: "ok"}
	objects <- minio.ObjectInfo{Key: "fail"}
	close(objects)

	err := runDownloadWorkers(context.Background(), 2, objects, func(_ context.Context, obj minio.ObjectInfo) error {
		if obj.Key == "fail" {
			return errors.New("boom")
		}
		return nil
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "boom")
}

func TestRunDownloadWorkers_NoDeadlockOnWorkerError(t *testing.T) {
	t.Parallel()

	objects := make(chan minio.ObjectInfo, 200)
	for i := 0; i < cap(objects); i++ {
		objects <- minio.ObjectInfo{Key: fmt.Sprintf("obj-%d", i)}
	}
	close(objects)

	errCh := make(chan error, 1)
	go func() {
		errCh <- runDownloadWorkers(context.Background(), 4, objects, func(_ context.Context, _ minio.ObjectInfo) error {
			return errors.New("boom")
		})
	}()

	select {
	case err := <-errCh:
		require.Error(t, err)
		assert.ErrorContains(t, err, "boom")
	case <-time.After(2 * time.Second):
		t.Fatal("runDownloadWorkers did not return; possible deadlock")
	}
}

func TestRunDownloadWorkers_ReturnsListError(t *testing.T) {
	objects := make(chan minio.ObjectInfo, 1)
	objects <- minio.ObjectInfo{Err: io.EOF}
	close(objects)

	err := runDownloadWorkers(context.Background(), 4, objects, func(_ context.Context, _ minio.ObjectInfo) error {
		return nil
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "error listing objects")
}

func BenchmarkRunDownloadWorkers_Memory(b *testing.B) {
	const (
		objectCount = 10000
		workers     = 30
	)

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		objects := make(chan minio.ObjectInfo, workers)
		go func() {
			for j := 0; j < objectCount; j++ {
				objects <- minio.ObjectInfo{Key: "obj"}
			}
			close(objects)
		}()

		var processed int64
		err := runDownloadWorkers(context.Background(), workers, objects, func(_ context.Context, _ minio.ObjectInfo) error {
			atomic.AddInt64(&processed, 1)
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}
		if atomic.LoadInt64(&processed) != objectCount {
			b.Fatalf("expected %d processed objects, got %d", objectCount, processed)
		}
	}
}

func BenchmarkRunDownloadWorkers_WorkerCounts(b *testing.B) {
	const objectCount = 10000

	b.ReportAllocs()

	for _, workers := range []int{5, 15, 30, 60} {
		workers := workers
		b.Run(fmt.Sprintf("workers_%d", workers), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				objects := make(chan minio.ObjectInfo, workers)
				go func() {
					for j := 0; j < objectCount; j++ {
						objects <- minio.ObjectInfo{Key: "obj"}
					}
					close(objects)
				}()

				var processed int64
				err := runDownloadWorkers(context.Background(), workers, objects, func(_ context.Context, _ minio.ObjectInfo) error {
					atomic.AddInt64(&processed, 1)
					return nil
				})
				if err != nil {
					b.Fatal(err)
				}
				if atomic.LoadInt64(&processed) != objectCount {
					b.Fatalf("expected %d processed objects, got %d", objectCount, processed)
				}
			}
		})
	}
}
