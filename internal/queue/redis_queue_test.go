// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package queue

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/the-hive/internal/config"
)

func TestRedisQueue_EnqueueDequeue(t *testing.T) {
	// Skip if Redis is not available
	ctx := context.Background()
	client, err := config.NewRedisClient(ctx)
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	// Use a unique queue key for this test
	queueKey := "test:queue:" + time.Now().Format("20060102150405")
	q, err := NewRedisQueue(client, queueKey)
	if err != nil {
		t.Fatalf("NewRedisQueue failed: %v", err)
	}

	// Clean up test key after test
	defer func() {
		client.Del(ctx, queueKey)
	}()

	// Test enqueue
	job := Job{
		Type:      "test_job",
		Payload:   []byte(`{"test": "data"}`),
		CreatedAt: time.Now(),
	}

	if err := q.Enqueue(ctx, job); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Test dequeue with timeout
	dequeueCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	dequeued, err := q.Dequeue(dequeueCtx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	if dequeued.Type != job.Type {
		t.Errorf("Expected job type %s, got %s", job.Type, dequeued.Type)
	}

	// Compare JSON payloads by normalizing whitespace (JSON can have different formatting)
	// Unmarshal both to ensure they're equivalent JSON
	var expectedPayload, actualPayload map[string]interface{}
	if err := json.Unmarshal(job.Payload, &expectedPayload); err != nil {
		t.Fatalf("Failed to unmarshal expected payload: %v", err)
	}
	if err := json.Unmarshal(dequeued.Payload, &actualPayload); err != nil {
		t.Fatalf("Failed to unmarshal actual payload: %v", err)
	}

	// Compare the unmarshaled objects
	expectedJSON, _ := json.Marshal(expectedPayload)
	actualJSON, _ := json.Marshal(actualPayload)
	if string(expectedJSON) != string(actualJSON) {
		t.Errorf("Expected payload %s, got %s", string(expectedJSON), string(actualJSON))
	}
}

func TestRedisQueue_MultipleJobs(t *testing.T) {
	// Skip if Redis is not available
	ctx := context.Background()
	client, err := config.NewRedisClient(ctx)
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	// Use a unique queue key for this test
	queueKey := "test:queue:multi:" + time.Now().Format("20060102150405")
	q, err := NewRedisQueue(client, queueKey)
	if err != nil {
		t.Fatalf("NewRedisQueue failed: %v", err)
	}

	// Clean up test key after test
	defer func() {
		client.Del(ctx, queueKey)
	}()

	// Enqueue multiple jobs
	numJobs := 5
	for i := 0; i < numJobs; i++ {
		job := Job{
			Type:      "test_job",
			Payload:   []byte(`{"index": ` + strconv.Itoa(i) + `}`),
			CreatedAt: time.Now(),
		}
		if err := q.Enqueue(ctx, job); err != nil {
			t.Fatalf("Enqueue failed for job %d: %v", i, err)
		}
	}

	// Dequeue all jobs
	dequeueCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	for i := 0; i < numJobs; i++ {
		dequeued, err := q.Dequeue(dequeueCtx)
		if err != nil {
			t.Fatalf("Dequeue failed for job %d: %v", i, err)
		}
		if dequeued.Type != "test_job" {
			t.Errorf("Expected job type test_job, got %s", dequeued.Type)
		}
	}
}

func TestRedisQueue_ContextCancellation(t *testing.T) {
	// Skip if Redis is not available
	ctx := context.Background()
	client, err := config.NewRedisClient(ctx)
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	// Use a unique queue key for this test
	queueKey := "test:queue:cancel:" + time.Now().Format("20060102150405")
	q, err := NewRedisQueue(client, queueKey)
	if err != nil {
		t.Fatalf("NewRedisQueue failed: %v", err)
	}

	// Clean up test key after test
	defer func() {
		client.Del(ctx, queueKey)
	}()

	// Test that dequeue respects context cancellation
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel() // Cancel immediately

	_, err = q.Dequeue(cancelCtx)
	if err == nil {
		t.Error("Expected error on cancelled context, got nil")
	}
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}
