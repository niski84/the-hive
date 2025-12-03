// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package jobs

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/the-hive/internal/queue"
)

// RecalcIssuePriorityPayload represents the payload for a recalc issue priority job.
type RecalcIssuePriorityPayload struct {
	UserID      int64     `json:"userId"`
	WorkspaceID int64     `json:"workspaceId"`
	IssueID     int64     `json:"issueId"`
	Action      string    `json:"action"`
	RequestedAt time.Time `json:"requestedAt"`
}

const JobTypeRecalcIssuePriority = "recalc_issue_priority"

// NewRecalcIssuePriorityJob creates a new job for recalculating issue priority.
func NewRecalcIssuePriorityJob(payload RecalcIssuePriorityPayload) (queue.Job, error) {
	log.Printf("NewRecalcIssuePriorityJob: userId=%d workspaceId=%d issueId=%d action=%s", payload.UserID, payload.WorkspaceID, payload.IssueID, payload.Action)

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		log.Printf("NewRecalcIssuePriorityJob: failed to marshal payload: %v", err)
		return queue.Job{}, err
	}

	job := queue.Job{
		Type:      JobTypeRecalcIssuePriority,
		Payload:   payloadJSON,
		CreatedAt: time.Now(),
	}

	log.Printf("NewRecalcIssuePriorityJob: created job type=%s createdAt=%s", job.Type, job.CreatedAt.Format(time.RFC3339))
	return job, nil
}

// EnqueueRecalcIssuePriority enqueues a recalc issue priority job.
func EnqueueRecalcIssuePriority(ctx context.Context, q queue.Queue, payload RecalcIssuePriorityPayload) error {
	log.Printf("EnqueueRecalcIssuePriority: userId=%d workspaceId=%d issueId=%d action=%s", payload.UserID, payload.WorkspaceID, payload.IssueID, payload.Action)

	job, err := NewRecalcIssuePriorityJob(payload)
	if err != nil {
		log.Printf("EnqueueRecalcIssuePriority: failed to create job: %v", err)
		return err
	}

	if err := q.Enqueue(ctx, job); err != nil {
		log.Printf("EnqueueRecalcIssuePriority: failed to enqueue job: %v", err)
		return err
	}

	log.Printf("EnqueueRecalcIssuePriority: successfully enqueued job")
	return nil
}

// HandleRecalcIssuePriority processes a recalc issue priority job.
func HandleRecalcIssuePriority(ctx context.Context, job queue.Job) error {
	log.Printf("HandleRecalcIssuePriority: processing job type=%s createdAt=%s", job.Type, job.CreatedAt.Format(time.RFC3339))

	if job.Type != JobTypeRecalcIssuePriority {
		log.Printf("HandleRecalcIssuePriority: unexpected job type %s, expected %s", job.Type, JobTypeRecalcIssuePriority)
		return nil
	}

	var payload RecalcIssuePriorityPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		log.Printf("HandleRecalcIssuePriority: failed to unmarshal payload: %v", err)
		return err
	}

	log.Printf("HandleRecalcIssuePriority: userId=%d workspaceId=%d issueId=%d action=%s requestedAt=%s", payload.UserID, payload.WorkspaceID, payload.IssueID, payload.Action, payload.RequestedAt.Format(time.RFC3339))

	// Simulate work
	time.Sleep(100 * time.Millisecond)

	log.Printf("HandleRecalcIssuePriority: successfully processed job for issueId=%d", payload.IssueID)
	return nil
}
