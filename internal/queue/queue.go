package queue

import (
	"context"
	"encoding/json"
	"time"
)

// Job represents a job in the queue.
type Job struct {
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"createdAt"`
}

// Queue defines the interface for job queues.
type Queue interface {
	// Enqueue adds a job to the queue.
	Enqueue(ctx context.Context, job Job) error

	// Dequeue blocks until a job is available, then returns it.
	// Returns an error if the context is cancelled or if the operation fails.
	Dequeue(ctx context.Context) (Job, error)
}

