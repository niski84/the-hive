// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/the-hive/internal/ai"
	"github.com/the-hive/internal/vectordb"
)

// TaggingJob represents a job for the tagging worker
type TaggingJob struct {
	ChunkID  string
	Content  string
	VectorDB vectordb.VectorDB
}

// TaggerPool manages a pool of tagging workers
type TaggerPool struct {
	jobQueue    chan TaggingJob
	workerCount int
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewTaggerPool creates a new tagging worker pool
func NewTaggerPool(workerCount int) *TaggerPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &TaggerPool{
		jobQueue:    make(chan TaggingJob, 100), // Buffered channel
		workerCount: workerCount,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start starts the tagging worker pool
func (p *TaggerPool) Start() {
	for i := 0; i < p.workerCount; i++ {
		go p.worker(i)
	}
	log.Printf("Started %d tagging workers", p.workerCount)
}

// Stop stops the tagging worker pool
func (p *TaggerPool) Stop() {
	p.cancel()
	close(p.jobQueue)
	log.Printf("Stopped tagging worker pool")
}

// Enqueue adds a job to the queue (non-blocking)
func (p *TaggerPool) Enqueue(job TaggingJob) {
	select {
	case p.jobQueue <- job:
		// Job enqueued successfully
	default:
		log.Printf("Warning: Tagging job queue full, dropping job for chunk %s", job.ChunkID)
	}
}

// worker processes jobs from the queue
func (p *TaggerPool) worker(id int) {
	log.Printf("Tagging worker %d started", id)
	defer log.Printf("Tagging worker %d stopped", id)

	for {
		select {
		case <-p.ctx.Done():
			return
		case job, ok := <-p.jobQueue:
			if !ok {
				return
			}
			p.processJob(job)
		}
	}
}

// processJob processes a single job to generate tags
func (p *TaggerPool) processJob(job TaggingJob) {
	// Get a snippet of the content (first 2000 chars)
	snippet := job.Content
	if len(snippet) > 2000 {
		snippet = snippet[:2000]
	}

	// Ask AI for tags
	tags, err := p.askAIForTags(snippet)
	if err != nil {
		log.Printf("[ERROR] Job failed: Failed to get tags for chunk %s: %v", job.ChunkID, err)
		return
	}

	if len(tags) == 0 {
		log.Printf("No tags generated for chunk %s", job.ChunkID)
		return
	}

	// Update Qdrant payload with tags
	if job.VectorDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := job.VectorDB.UpdatePayload(ctx, job.ChunkID, tags); err != nil {
			log.Printf("Failed to update payload for chunk %s: %v", job.ChunkID, err)
		} else {
			log.Printf("Tagged chunk %s with tags: %v", job.ChunkID, tags)
		}
	}
}

// askAIForTags asks the AI to generate tags for the document
func (p *TaggerPool) askAIForTags(content string) ([]string, error) {
	// Construct the prompt
	prompt := fmt.Sprintf(`Analyze this document and return a JSON array of up to 5 relevant tags (e.g., #legal, #invoice, #urgent, #proposal). Return ONLY the JSON array, no other text.

Document content:
%s

Return format: ["#tag1", "#tag2", "#tag3"]`, content)

	// Use the AI service to get tags
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try to use OpenAI if available
	answer, err := ai.AskQuestion(ctx, prompt)
	if err != nil {
		// Fallback: simple keyword-based tagging
		log.Printf("AI service unavailable, using fallback tagging: %v", err)
		return p.fallbackTags(content), nil
	}

	// Parse JSON response
	var tags []string
	// Clean the answer - remove markdown code blocks if present
	answer = strings.TrimSpace(answer)
	answer = strings.TrimPrefix(answer, "```json")
	answer = strings.TrimPrefix(answer, "```")
	answer = strings.TrimSuffix(answer, "```")
	answer = strings.TrimSpace(answer)

	if err := json.Unmarshal([]byte(answer), &tags); err != nil {
		log.Printf("Failed to parse AI response as JSON: %v, response: %s", err, answer)
		return p.fallbackTags(content), nil
	}

	// Ensure tags start with #
	for i, tag := range tags {
		tag = strings.TrimSpace(tag)
		if !strings.HasPrefix(tag, "#") {
			tag = "#" + tag
		}
		tags[i] = tag
	}

	// Limit to 5 tags
	if len(tags) > 5 {
		tags = tags[:5]
	}

	return tags, nil
}

// fallbackTags provides simple keyword-based tagging when AI is unavailable
func (p *TaggerPool) fallbackTags(content string) []string {
	contentLower := strings.ToLower(content)
	tags := []string{}

	// Simple keyword matching
	if strings.Contains(contentLower, "legal") || strings.Contains(contentLower, "law") || strings.Contains(contentLower, "contract") {
		tags = append(tags, "#legal")
	}
	if strings.Contains(contentLower, "invoice") || strings.Contains(contentLower, "billing") || strings.Contains(contentLower, "payment") {
		tags = append(tags, "#finance")
	}
	if strings.Contains(contentLower, "urgent") || strings.Contains(contentLower, "asap") || strings.Contains(contentLower, "immediate") {
		tags = append(tags, "#urgent")
	}
	if strings.Contains(contentLower, "proposal") || strings.Contains(contentLower, "quote") {
		tags = append(tags, "#proposal")
	}
	if strings.Contains(contentLower, "confidential") || strings.Contains(contentLower, "secret") {
		tags = append(tags, "#confidential")
	}

	return tags
}
