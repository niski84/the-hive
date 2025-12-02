package worker

import (
	"context"
	"log"
	"sync"

	"github.com/the-hive/internal/queue"
)

// HandlerFunc processes a job. It should return an error if processing fails.
type HandlerFunc func(ctx context.Context, job queue.Job) error

// StartWorkers starts a pool of workers that process jobs from the queue.
// ctx: context for cancellation (workers will stop when context is cancelled)
// q: the queue to dequeue jobs from
// handler: function to process each job
// workerCount: number of worker goroutines to start
func StartWorkers(ctx context.Context, q queue.Queue, handler HandlerFunc, workerCount int) error {
	log.Printf("StartWorkers: workerCount=%d", workerCount)

	var wg sync.WaitGroup
	wg.Add(workerCount)

	// Start worker goroutines
	for i := 0; i < workerCount; i++ {
		workerID := i + 1
		go func() {
			defer wg.Done()
			workerLoop(ctx, q, handler, workerID)
		}()
	}

	// Wait for all workers to finish
	wg.Wait()
	log.Printf("StartWorkers: all workers stopped")
	return nil
}

// workerLoop is the main loop for a single worker.
func workerLoop(ctx context.Context, q queue.Queue, handler HandlerFunc, workerID int) {
	log.Printf("workerLoop: workerID=%d started", workerID)

	for {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			log.Printf("workerLoop: workerID=%d context cancelled, stopping", workerID)
			return
		default:
		}

		// Dequeue a job (this blocks until a job is available or context is cancelled)
		job, err := q.Dequeue(ctx)
		if err != nil {
			if err == context.Canceled || err == context.DeadlineExceeded {
				log.Printf("workerLoop: workerID=%d context cancelled during dequeue", workerID)
				return
			}
			log.Printf("workerLoop: workerID=%d dequeue error: %v, continuing", workerID, err)
			continue
		}

		log.Printf("workerLoop: workerID=%d processing job type=%s createdAt=%s", workerID, job.Type, job.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))

		// Process the job
		if err := handler(ctx, job); err != nil {
			log.Printf("workerLoop: workerID=%d handler error for job type=%s: %v", workerID, job.Type, err)
			continue
		}

		log.Printf("workerLoop: workerID=%d successfully processed job type=%s", workerID, job.Type)
	}
}
