// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package worker

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/the-hive/internal/ai"
	"github.com/the-hive/internal/rules"
	"github.com/the-hive/internal/vectordb"
)

// AnalystJob represents a job for the analyst worker
type AnalystJob struct {
	FilePath    string
	Content     string
	Metadata    map[string]string
	ClientID    string
}

// NotificationSender is an interface for sending notifications
type NotificationSender interface {
	SendNotification(clientID string, notificationType, message, level string) error
}

// GraphStore interface for storing document relationships
type GraphStore interface {
	AddEdge(ctx context.Context, sourceDocID, targetDocID, relationshipType, description string) error
}

// AnalystPool manages a pool of analyst workers
type AnalystPool struct {
	jobQueue         chan AnalystJob
	ruleStore        *rules.Store
	notificationSender NotificationSender
	graphStore       GraphStore
	vectorDB         vectordb.VectorDB
	embedder         interface {
		EmbedText(ctx context.Context, text string) ([]float32, error)
	}
	workerCount      int
	ctx              context.Context
	cancel           context.CancelFunc
}

// NewAnalystPool creates a new analyst worker pool
func NewAnalystPool(ruleStore *rules.Store, notificationSender NotificationSender, graphStore GraphStore, vectorDB vectordb.VectorDB, embedder interface {
	EmbedText(ctx context.Context, text string) ([]float32, error)
}, workerCount int) *AnalystPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &AnalystPool{
		jobQueue:          make(chan AnalystJob, 100), // Buffered channel
		ruleStore:         ruleStore,
		notificationSender: notificationSender,
		graphStore:        graphStore,
		vectorDB:          vectorDB,
		embedder:          embedder,
		workerCount:       workerCount,
		ctx:               ctx,
		cancel:            cancel,
	}
}

// Start starts the analyst worker pool
func (p *AnalystPool) Start() {
	for i := 0; i < p.workerCount; i++ {
		go p.worker(i)
	}
	log.Printf("Started %d analyst workers", p.workerCount)
}

// Stop stops the analyst worker pool
func (p *AnalystPool) Stop() {
	p.cancel()
	close(p.jobQueue)
	log.Printf("Stopped analyst worker pool")
}

// Enqueue adds a job to the queue (non-blocking)
func (p *AnalystPool) Enqueue(job AnalystJob) {
	select {
	case p.jobQueue <- job:
		// Job enqueued successfully
	default:
		log.Printf("Warning: Analyst job queue full, dropping job for %s", job.FilePath)
	}
}

// worker processes jobs from the queue
func (p *AnalystPool) worker(id int) {
	log.Printf("Analyst worker %d started", id)
	defer log.Printf("Analyst worker %d stopped", id)

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

// processJob processes a single job against all active rules
func (p *AnalystPool) processJob(job AnalystJob) {
	// Get all active rules
	activeRules, err := p.ruleStore.GetActiveRules()
	if err != nil {
		log.Printf("Failed to get active rules: %v", err)
		return
	}

	if len(activeRules) == 0 {
		log.Printf("[ANALYST] No active rules to check for file %s", job.FilePath)
		return // No rules to check
	}
	
	log.Printf("[ANALYST] Processing file %s against %d active rule(s)", job.FilePath, len(activeRules))

	// Get a snippet of the content (first 2000 chars for AI analysis)
	snippet := job.Content
	if len(snippet) > 2000 {
		snippet = snippet[:2000] + "..."
	}

	// Check for contradictions with existing documents
	if p.graphStore != nil && p.vectorDB != nil && p.embedder != nil {
		p.checkContradictions(job, snippet)
	}

	// Check each rule
	for _, rule := range activeRules {
		if !rule.Active {
			continue
		}

		// Ask AI the question
		answer, err := p.askAI(rule.Query, snippet)
		if err != nil {
			log.Printf("[ERROR] Job failed: Failed to ask AI for rule %d: %v", rule.ID, err)
			continue
		}

		// If AI answers YES, send notification
		if strings.ToUpper(strings.TrimSpace(answer)) == "YES" {
			filename := job.Metadata["filename"]
			if filename == "" {
				filename = job.FilePath
			}

			message := fmt.Sprintf("⚠️ Rule Hit: '%s' detected in %s", rule.Query, filename)

			if job.ClientID != "" && p.notificationSender != nil {
				if err := p.notificationSender.SendNotification(job.ClientID, "ALERT", message, "warning"); err != nil {
					log.Printf("Failed to send notification for rule %d: %v", rule.ID, err)
				} else {
					log.Printf("[ANALYST] Rule %d triggered for %s, notification sent to client %s", rule.ID, filename, job.ClientID)
				}
			} else {
				log.Printf("[ANALYST] Rule %d triggered for %s, but no client ID or notification sender available", rule.ID, filename)
			}
		}
	}
}

// askAI asks the AI a yes/no question about the document content
func (p *AnalystPool) askAI(question, content string) (string, error) {
	// Construct the prompt
	prompt := fmt.Sprintf(`Given the following document content snippet, answer this yes/no question:

Document content:
%s

Question: %s

Answer with only "YES" or "NO":`, content, question)

	// Use the AI service to get an answer
	// For now, we'll use a simple approach: call OpenAI API directly
	// In a production system, you might want to use a dedicated LLM service
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try to use OpenAI if available
	answer, err := ai.AskQuestion(ctx, prompt)
	if err != nil {
		// Fallback: simple keyword matching for testing
		log.Printf("AI service unavailable, using fallback: %v", err)
		return p.fallbackAnswer(question, content), nil
	}

	return answer, nil
}

// fallbackAnswer provides a simple fallback when AI is unavailable
func (p *AnalystPool) fallbackAnswer(question, content string) string {
	// Simple keyword matching fallback
	questionLower := strings.ToLower(question)
	contentLower := strings.ToLower(content)

	// Check for common keywords
	if strings.Contains(questionLower, "confidential") {
		if strings.Contains(contentLower, "confidential") {
			return "YES"
		}
	}
	if strings.Contains(questionLower, "pricing") {
		if strings.Contains(contentLower, "pricing") || strings.Contains(contentLower, "price") {
			return "YES"
		}
	}
	if strings.Contains(questionLower, "secret") {
		if strings.Contains(contentLower, "secret") {
			return "YES"
		}
	}

	return "NO"
}

// checkContradictions checks if the new document contradicts existing documents
func (p *AnalystPool) checkContradictions(job AnalystJob, snippet string) {
	// Get document ID from metadata
	sourceDocID := job.Metadata["filename"]
	if sourceDocID == "" {
		sourceDocID = job.FilePath
	}

	// Generate embedding for the new document
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	queryVector, err := p.embedder.EmbedText(ctx, snippet)
	if err != nil {
		log.Printf("[ERROR] Job failed: Failed to generate embedding for contradiction check: %v", err)
		return
	}

	// Search for similar documents (top 5)
	matches, err := p.vectorDB.Search(ctx, queryVector, 5)
	if err != nil {
		log.Printf("Failed to search for similar documents: %v", err)
		return
	}

	// Check each similar document for contradictions
	for _, match := range matches {
		targetDocID := match.DocumentID
		if targetDocID == sourceDocID {
			continue // Skip self
		}

		// Get content from metadata
		targetContent := match.Metadata["content"]
		if targetContent == "" {
			continue
		}

		// Limit target content snippet
		if len(targetContent) > 2000 {
			targetContent = targetContent[:2000] + "..."
		}

		// Ask AI if documents contradict
		contradictionPrompt := fmt.Sprintf(`Compare these two document snippets and determine if they contradict each other.

Document A:
%s

Document B:
%s

Answer with ONLY "YES" if they contradict, or "NO" if they do not contradict. If they contradict, provide a brief explanation in one sentence after "YES".`, snippet, targetContent)

		answer, err := ai.AskQuestion(ctx, contradictionPrompt)
		if err != nil {
			log.Printf("Failed to check contradiction: %v", err)
			continue
		}

		// Check if AI detected a contradiction
		answerUpper := strings.ToUpper(strings.TrimSpace(answer))
		if strings.HasPrefix(answerUpper, "YES") {
			// Extract description (everything after "YES")
			description := strings.TrimSpace(strings.TrimPrefix(answer, "YES"))
			if description == "" {
				description = "Documents contain contradictory information"
			}

			// Store contradiction edge (use the same context from checkContradictions)
			if err := p.graphStore.AddEdge(ctx, sourceDocID, targetDocID, "contradicts", description); err != nil {
				log.Printf("Failed to store contradiction edge: %v", err)
			} else {
				log.Printf("Detected contradiction: %s contradicts %s", sourceDocID, targetDocID)
			}
		}
	}
}

