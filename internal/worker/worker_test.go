package worker

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/the-hive/internal/config"
	"github.com/the-hive/internal/queue"
)

func TestStartWorkers(t *testing.T) {
	// Skip if Redis is not available
	ctx := context.Background()
	client, err := config.NewRedisClient(ctx)
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	// Use a unique queue key for this test
	queueKey := "test:worker:queue:" + time.Now().Format("20060102150405")
	q, err := queue.NewRedisQueue(client, queueKey)
	if err != nil {
		t.Fatalf("NewRedisQueue failed: %v", err)
	}

	// Clean up test key after test
	defer func() {
		client.Del(ctx, queueKey)
	}()

	// Track processed jobs
	var processedJobs []queue.Job
	var mu sync.Mutex

	// Create handler that records processed jobs
	handler := func(ctx context.Context, job queue.Job) error {
		mu.Lock()
		defer mu.Unlock()
		processedJobs = append(processedJobs, job)
		return nil
	}

	// Enqueue some jobs
	numJobs := 3
	for i := 0; i < numJobs; i++ {
		job := queue.Job{
			Type:      "test_job",
			Payload:   []byte(`{"index": ` + strconv.Itoa(i) + `}`),
			CreatedAt: time.Now(),
		}
		if err := q.Enqueue(ctx, job); err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}
	}

	// Start workers with timeout
	workerCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Start workers in a goroutine
	done := make(chan error, 1)
	go func() {
		done <- StartWorkers(workerCtx, q, handler, 2)
	}()

	// Wait a bit for jobs to be processed
	time.Sleep(2 * time.Second)

	// Cancel context to stop workers
	cancel()

	// Wait for workers to finish
	err = <-done
	if err != nil {
		t.Errorf("StartWorkers returned error: %v", err)
	}

	// Verify all jobs were processed
	mu.Lock()
	processedCount := len(processedJobs)
	mu.Unlock()

	if processedCount != numJobs {
		t.Errorf("Expected %d jobs processed, got %d", numJobs, processedCount)
	}
}

func TestStartWorkers_HandlerError(t *testing.T) {
	// Skip if Redis is not available
	ctx := context.Background()
	client, err := config.NewRedisClient(ctx)
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	// Use a unique queue key for this test
	queueKey := "test:worker:error:" + time.Now().Format("20060102150405")
	q, err := queue.NewRedisQueue(client, queueKey)
	if err != nil {
		t.Fatalf("NewRedisQueue failed: %v", err)
	}

	// Clean up test key after test
	defer func() {
		client.Del(ctx, queueKey)
	}()

	// Create handler that returns an error
	handler := func(ctx context.Context, job queue.Job) error {
		return nil // Return nil to continue processing
	}

	// Enqueue a job
	job := queue.Job{
		Type:      "test_job",
		Payload:   []byte(`{"test": "data"}`),
		CreatedAt: time.Now(),
	}
	if err := q.Enqueue(ctx, job); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Start workers with timeout
	workerCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Start workers in a goroutine
	done := make(chan error, 1)
	go func() {
		done <- StartWorkers(workerCtx, q, handler, 1)
	}()

	// Wait a bit for job to be processed
	time.Sleep(1 * time.Second)

	// Cancel context to stop workers
	cancel()

	// Wait for workers to finish
	err = <-done
	if err != nil {
		t.Errorf("StartWorkers returned error: %v", err)
	}
}
