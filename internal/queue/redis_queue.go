package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisQueue implements Queue using Redis Lists.
type RedisQueue struct {
	client *redis.Client
	key    string
}

// NewRedisQueue creates a new Redis-backed queue.
// client: the Redis client to use
// key: the Redis key name for the queue (e.g., "jobs:default")
func NewRedisQueue(client *redis.Client, key string) (Queue, error) {
	if key == "" {
		key = "jobs:default"
	}

	log.Printf("NewRedisQueue: key=%s", key)

	// Test connection
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("NewRedisQueue: failed to ping Redis: %v", err)
		return nil, err
	}

	return &RedisQueue{
		client: client,
		key:    key,
	}, nil
}

// Enqueue adds a job to the queue using RPUSH.
func (r *RedisQueue) Enqueue(ctx context.Context, job Job) error {
	log.Printf("Enqueue: job type=%s createdAt=%s", job.Type, job.CreatedAt.Format(time.RFC3339))

	data, err := json.Marshal(job)
	if err != nil {
		log.Printf("Enqueue: failed to marshal job: %v", err)
		return err
	}

	log.Printf("Enqueue: key=%s payloadSize=%d", r.key, len(data))

	if err := r.client.RPush(ctx, r.key, data).Err(); err != nil {
		log.Printf("Enqueue: failed to push to Redis: %v", err)
		return err
	}

	log.Printf("Enqueue: successfully enqueued job type=%s", job.Type)
	return nil
}

// Dequeue blocks until a job is available using BLPOP, then returns it.
func (r *RedisQueue) Dequeue(ctx context.Context) (Job, error) {
	log.Printf("Dequeue: waiting for job from key=%s", r.key)

	// Use a channel to handle context cancellation
	type result struct {
		val []string
		err error
	}
	resultChan := make(chan result, 1)

	go func() {
		val, err := r.client.BLPop(ctx, 0, r.key).Result()
		resultChan <- result{val: val, err: err}
	}()

	select {
	case <-ctx.Done():
		log.Printf("Dequeue: context cancelled")
		return Job{}, ctx.Err()
	case res := <-resultChan:
		if res.err != nil {
			if res.err == redis.Nil {
				log.Printf("Dequeue: context cancelled or timeout")
				return Job{}, ctx.Err()
			}
			log.Printf("Dequeue: failed to pop from Redis: %v", res.err)
			return Job{}, res.err
		}

		if len(res.val) < 2 {
			log.Printf("Dequeue: invalid result from Redis, expected 2 elements, got %d", len(res.val))
			return Job{}, fmt.Errorf("invalid result from Redis")
		}

		data := res.val[1]
		log.Printf("Dequeue: received job payloadSize=%d", len(data))

		var job Job
		if err := json.Unmarshal([]byte(data), &job); err != nil {
			log.Printf("Dequeue: failed to unmarshal job: %v", err)
			return Job{}, err
		}

		log.Printf("Dequeue: successfully dequeued job type=%s createdAt=%s", job.Type, job.CreatedAt.Format(time.RFC3339))
		return job, nil
	}
}

