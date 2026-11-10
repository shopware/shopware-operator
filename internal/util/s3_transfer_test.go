package util

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestS3WorkerCount_FloorsAt32(t *testing.T) {
	assert.Equal(t, 32, s3WorkerCount(1))
	assert.Equal(t, 32, s3WorkerCount(16))
	assert.Equal(t, 64, s3WorkerCount(32))
	assert.Equal(t, 128, s3WorkerCount(64))
}

func TestMinioTransport_IdleConnsMatchWorkers(t *testing.T) {
	tr, err := minioTransport(true, 32)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, tr.MaxIdleConnsPerHost, 32)
	assert.GreaterOrEqual(t, tr.MaxIdleConns, 64)

	tr, err = minioTransport(true, 8)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, tr.MaxIdleConnsPerHost, 16, "default transport already has 16")
}

func TestSharedWorkerPool_TwoProducersBoundedConcurrency(t *testing.T) {
	const (
		perProducer = 100
		workers     = 8
	)

	taskCh := make(chan int, workers)
	var producers sync.WaitGroup
	producers.Add(2)
	for p := 0; p < 2; p++ {
		go func() {
			defer producers.Done()
			for i := 0; i < perProducer; i++ {
				taskCh <- i
			}
		}()
	}
	go func() {
		producers.Wait()
		close(taskCh)
	}()

	var processed int64
	var inFlight int64
	var maxInFlight int64

	err := runWorkers(context.Background(), workers, taskCh, func(_ context.Context, _ int) error {
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
	assert.Equal(t, int64(perProducer*2), atomic.LoadInt64(&processed))
	assert.LessOrEqual(t, atomic.LoadInt64(&maxInFlight), int64(workers))
}

func TestTransferStats_LogsStartAndFinish(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core).Sugar()

	stats := NewTransferStats(logger, "download", 32)
	stats.Add(1024)
	stats.Finish()

	require.GreaterOrEqual(t, logs.Len(), 2)
	assert.Equal(t, "Starting S3 transfer", logs.All()[0].Message)
	assert.Equal(t, "S3 transfer finished", logs.All()[logs.Len()-1].Message)
	assert.Equal(t, int64(1), logs.All()[logs.Len()-1].ContextMap()["objects"])
}

func TestFormatTransferBytes(t *testing.T) {
	assert.Equal(t, "95.2Gi", formatTransferBytes(102211734557))
	assert.Equal(t, "100Gi", formatTransferBytes(100*1024*1024*1024))
	assert.Equal(t, "8Ki", formatTransferBytes(8192))
}
