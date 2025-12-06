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
	FilePath      string
	Content       string
	Metadata      map[string]string
	ClientID      string
	AllChunks     []string // Full document chunks for comprehensive analysis
	OrganizationID string  // Organization ID for multi-tenancy isolation
}

// NotificationSender is an interface for sending notifications
type NotificationSender interface {
	SendNotification(clientID string, notificationType, message, level string) error
}

// GraphStore interface for storing document relationships
type GraphStore interface {
	AddEdge(ctx context.Context, sourceDocID, targetDocID, relationshipType, description string) error
}

// RuleMatchStore interface for storing rule matches
type RuleMatchStore interface {
	AddMatch(ctx context.Context, match interface{}) error
}

// RuleEventStore interface for storing rule processing events
type RuleEventStore interface {
	AddEvent(ctx context.Context, event interface{}) error
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
	matchStore       RuleMatchStore // Store for rule match history
	eventStore       RuleEventStore // Store for rule processing events
	workerCount      int
	ctx              context.Context
	cancel           context.CancelFunc
}

// NewAnalystPool creates a new analyst worker pool
func NewAnalystPool(ruleStore *rules.Store, notificationSender NotificationSender, graphStore GraphStore, vectorDB vectordb.VectorDB, embedder interface {
	EmbedText(ctx context.Context, text string) ([]float32, error)
}, matchStore RuleMatchStore, eventStore RuleEventStore, workerCount int) *AnalystPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &AnalystPool{
		jobQueue:          make(chan AnalystJob, 100), // Buffered channel
		ruleStore:         ruleStore,
		notificationSender: notificationSender,
		graphStore:        graphStore,
		vectorDB:          vectorDB,
		embedder:          embedder,
		matchStore:        matchStore,
		eventStore:        eventStore,
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
	log.Printf("[DEBUG] AnalystPool.Enqueue called for file: %s", job.FilePath)
	select {
	case p.jobQueue <- job:
		log.Printf("[DEBUG] Analyst job successfully added to queue for file: %s", job.FilePath)
		// Job enqueued successfully
	default:
		log.Printf("[WARN] Analyst job queue full, dropping job for %s", job.FilePath)
	}
}

// worker processes jobs from the queue
func (p *AnalystPool) worker(id int) {
	log.Printf("[DEBUG] Analyst worker %d started", id)
	defer log.Printf("[DEBUG] Analyst worker %d stopped", id)

	for {
		select {
		case <-p.ctx.Done():
			log.Printf("[DEBUG] Analyst worker %d received context cancellation", id)
			return
		case job, ok := <-p.jobQueue:
			if !ok {
				log.Printf("[DEBUG] Analyst worker %d: job queue closed", id)
				return
			}
			log.Printf("[DEBUG] Analyst worker %d received job for file: %s", id, job.FilePath)
			p.processJob(job)
			log.Printf("[DEBUG] Analyst worker %d finished processing job for file: %s", id, job.FilePath)
		}
	}
}

// processJob processes a single job against all active rules
func (p *AnalystPool) processJob(job AnalystJob) {
	log.Printf("[DEBUG] processJob called for file: %s (content length: %d)", job.FilePath, len(job.Content))
	
	// Get all active rules for this organization (multi-tenancy isolation)
	var activeRules []rules.Rule
	var err error
	if job.OrganizationID != "" {
		activeRules, err = p.ruleStore.GetActiveRules(job.OrganizationID)
	} else {
		activeRules, err = p.ruleStore.GetActiveRules()
	}
	if err != nil {
		log.Printf("[ERROR] Failed to get active rules: %v", err)
		return
	}

	log.Printf("[DEBUG] Retrieved %d active rules from store", len(activeRules))

	if len(activeRules) == 0 {
		log.Printf("[ANALYST] No active rules to check for file %s", job.FilePath)
		return // No rules to check
	}
	
	log.Printf("[ANALYST] Processing file %s against %d active rule(s)", job.FilePath, len(activeRules))

	// Use FULL document content (not just snippet)
	fullContent := job.Content
	filename := job.Metadata["filename"]
	if filename == "" {
		filename = job.FilePath
	}

	// Log event: Started processing
	if p.eventStore != nil {
		log.Printf("[DEBUG] Creating 'processing started' event for document: %s", filename)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := p.eventStore.AddEvent(ctx, map[string]interface{}{
			"RuleID":        0,
			"RuleQuery":     "",
			"Document":      filename,
			"EventType":     "processing",
			"Status":        "started",
			"Message":       fmt.Sprintf("Started processing document against %d active rules", len(activeRules)),
			"ClientID":      job.ClientID,
			"OrganizationID": job.OrganizationID,
		})
		if err != nil {
			log.Printf("[ERROR] Failed to store 'processing started' event: %v", err)
		} else {
			log.Printf("[DEBUG] Successfully stored 'processing started' event for document: %s", filename)
		}
	} else {
		log.Printf("[WARN] eventStore is nil, cannot log processing started event")
	}

	// Check for contradictions with existing documents
	if p.graphStore != nil && p.vectorDB != nil && p.embedder != nil {
		p.checkContradictions(job, fullContent)
	}

	// Check each rule
	for _, rule := range activeRules {
		if !rule.Active {
			continue
		}

		// Log event: Checking rule
		if p.eventStore != nil {
			log.Printf("[DEBUG] Creating 'checking' event for rule %d (%s) on document: %s", rule.ID, rule.Query, filename)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := p.eventStore.AddEvent(ctx, map[string]interface{}{
				"RuleID":        rule.ID,
				"RuleQuery":     rule.Query,
				"Document":      filename,
				"EventType":     "checking",
				"Status":        "processing",
				"Message":       fmt.Sprintf("Checking rule: %s", rule.Query),
				"ClientID":      job.ClientID,
				"OrganizationID": job.OrganizationID,
			})
			if err != nil {
				log.Printf("[ERROR] Failed to store 'checking' event for rule %d: %v", rule.ID, err)
			} else {
				log.Printf("[DEBUG] Successfully stored 'checking' event for rule %d", rule.ID)
			}
		} else {
			log.Printf("[WARN] eventStore is nil, cannot log checking event for rule %d", rule.ID)
		}

		// Determine if rule requires cross-document comparison
		requiresCrossDoc := p.requiresCrossDocumentCheck(rule.Query)

		if requiresCrossDoc {
			// Check rule against all existing documents
			p.checkRuleCrossDocument(rule, fullContent, job, filename)
		} else {
			// Check rule against only the uploaded document
			p.checkRuleSingleDocument(rule, fullContent, job, filename)
		}
	}

	// Log event: Completed processing
	if p.eventStore != nil {
		log.Printf("[DEBUG] Creating 'processing completed' event for document: %s", filename)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := p.eventStore.AddEvent(ctx, map[string]interface{}{
			"RuleID":        0,
			"RuleQuery":     "",
			"Document":      filename,
			"EventType":     "processing",
			"Status":        "completed",
			"OrganizationID": job.OrganizationID,
			"Message":   fmt.Sprintf("Completed processing document against %d active rules", len(activeRules)),
			"ClientID":  job.ClientID,
		})
		if err != nil {
			log.Printf("[ERROR] Failed to store 'processing completed' event: %v", err)
		} else {
			log.Printf("[DEBUG] Successfully stored 'processing completed' event for document: %s", filename)
		}
	} else {
		log.Printf("[WARN] eventStore is nil, cannot log processing completed event")
	}
}

// askAI asks the AI a yes/no question about the document content (legacy method, kept for backward compatibility)
func (p *AnalystPool) askAI(question, content string) (string, error) {
	answer, _, err := p.askAIWithExplanation(question, content, false, "")
	return answer, err
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
	// Use organization_id from job metadata for multi-tenancy isolation
	orgID := job.OrganizationID
	if orgID == "" {
		// Fallback: try to get from metadata
		orgID = job.Metadata["organization_id"]
	}
	matches, err := p.vectorDB.Search(ctx, queryVector, 5, orgID)
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

		answer, _, err := ai.AskQuestion(ctx, contradictionPrompt)
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


// requiresCrossDocumentCheck determines if a rule needs cross-document comparison
func (p *AnalystPool) requiresCrossDocumentCheck(query string) bool {
	queryLower := strings.ToLower(query)
	crossDocKeywords := []string{
		"contradict", "contradiction", "contradicts",
		"agreement", "agreements", "breaks agreement",
		"existing document", "existing documents",
		"other document", "other documents",
		"previous document", "previous documents",
		"conflict", "conflicts", "conflicting",
		"violate", "violates", "violation",
		"inconsistent", "inconsistency",
	}
	
	for _, keyword := range crossDocKeywords {
		if strings.Contains(queryLower, keyword) {
			return true
		}
	}
	return false
}

// checkRuleSingleDocument checks a rule against only the uploaded document
func (p *AnalystPool) checkRuleSingleDocument(rule rules.Rule, content string, job AnalystJob, filename string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Ask AI the question with full document content
	answer, explanation, err := p.askAIWithExplanation(rule.Query, content, false, "")
	if err != nil {
		log.Printf("[ERROR] Job failed: Failed to ask AI for rule %d: %v", rule.ID, err)
		return
	}

	// If AI answers YES, send notification and store match
	if strings.ToUpper(strings.TrimSpace(answer)) == "YES" {
		message := fmt.Sprintf("⚠️ Rule Hit: '%s' detected in %s", rule.Query, filename)

		// Extract relevant chunks (first 3 chunks or all if less than 3)
		matchedChunks := p.extractRelevantChunks(content, job.AllChunks)

		// Store match in database
		if p.matchStore != nil {
			match := map[string]interface{}{
				"RuleID":        rule.ID,
				"RuleQuery":     rule.Query,
				"UploadedDoc":   filename,
				"MatchedDoc":    "", // Empty for single-doc matches
				"MatchType":     "single_doc",
				"AIExplanation": explanation,
				"MatchedChunks": matchedChunks,
				"ClientID":      job.ClientID,
				"OrganizationID": job.OrganizationID,
			}
			if err := p.matchStore.AddMatch(ctx, match); err != nil {
				log.Printf("Failed to store rule match: %v", err)
			}
		}

		// Send notification
		if job.ClientID != "" && p.notificationSender != nil {
			if err := p.notificationSender.SendNotification(job.ClientID, "ALERT", message, "warning"); err != nil {
				log.Printf("Failed to send notification for rule %d: %v", rule.ID, err)
			} else {
				log.Printf("[ANALYST] Rule %d triggered for %s, notification sent to client %s", rule.ID, filename, job.ClientID)
			}
		}
	}
}

// checkRuleCrossDocument checks a rule by comparing uploaded document against all existing documents
func (p *AnalystPool) checkRuleCrossDocument(rule rules.Rule, newDocContent string, job AnalystJob, filename string) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Generate embedding for the new document
	queryVector, err := p.embedder.EmbedText(ctx, newDocContent)
	if err != nil {
		log.Printf("[ERROR] Failed to generate embedding for cross-doc rule check: %v", err)
		return
	}

	// Search for similar/relevant documents (top 10 for comprehensive comparison)
	// Use organization_id from job metadata for multi-tenancy isolation
	orgID := job.OrganizationID
	if orgID == "" {
		// Fallback: try to get from metadata
		orgID = job.Metadata["organization_id"]
	}
	matches, err := p.vectorDB.Search(ctx, queryVector, 10, orgID)
	if err != nil {
		log.Printf("Failed to search for documents for cross-doc rule check: %v", err)
		return
	}

	if len(matches) == 0 {
		log.Printf("[ANALYST] No existing documents found for cross-document rule check")
		// Still check the rule against the uploaded document only
		p.checkRuleSingleDocument(rule, newDocContent, job, filename)
		return
	}

	// Check rule against each existing document
	for _, match := range matches {
		targetDocID := match.DocumentID
		if targetDocID == filename {
			continue // Skip self
		}

		// Get full content from metadata
		targetContent := match.Metadata["content"]
		if targetContent == "" {
			continue
		}

		// Ask AI if the rule applies when comparing both documents
		answer, explanation, err := p.askAIWithExplanation(rule.Query, newDocContent, true, targetContent)
		if err != nil {
			log.Printf("Failed to check cross-doc rule: %v", err)
			continue
		}

		// If AI answers YES, we have a cross-document match
		if strings.ToUpper(strings.TrimSpace(answer)) == "YES" {
			message := fmt.Sprintf("⚠️ Rule Hit: '%s' detected between %s and %s", rule.Query, filename, targetDocID)

			// Extract relevant chunks from both documents
			matchedChunks := []string{
				fmt.Sprintf("[%s] %s", filename, truncateString(newDocContent, 500)),
				fmt.Sprintf("[%s] %s", targetDocID, truncateString(targetContent, 500)),
			}

			// Store match in database
			if p.matchStore != nil {
				matchData := map[string]interface{}{
					"RuleID":        rule.ID,
					"RuleQuery":     rule.Query,
					"UploadedDoc":   filename,
					"MatchedDoc":    targetDocID,
					"MatchType":     "cross_doc",
					"AIExplanation": explanation,
					"MatchedChunks": matchedChunks,
					"ClientID":      job.ClientID,
					"OrganizationID": job.OrganizationID,
				}
				if err := p.matchStore.AddMatch(ctx, matchData); err != nil {
					log.Printf("Failed to store cross-doc rule match: %v", err)
				}
			}

			// Log event: Cross-doc rule matched
			if p.eventStore != nil {
				p.eventStore.AddEvent(ctx, map[string]interface{}{
					"RuleID":        rule.ID,
					"RuleQuery":     rule.Query,
					"Document":      filename,
					"EventType":     "matched",
					"Status":        "completed",
					"Message":       fmt.Sprintf("Cross-document match with %s: %s", targetDocID, explanation),
					"ClientID":      job.ClientID,
					"OrganizationID": job.OrganizationID,
				})
			}

			// Send notification
			if job.ClientID != "" && p.notificationSender != nil {
				if err := p.notificationSender.SendNotification(job.ClientID, "ALERT", message, "critical"); err != nil {
					log.Printf("Failed to send notification for cross-doc rule %d: %v", rule.ID, err)
				} else {
					log.Printf("[ANALYST] Cross-doc rule %d triggered: %s vs %s, notification sent", rule.ID, filename, targetDocID)
				}
			}
		} else {
			// Log event: Cross-doc rule did not match
			if p.eventStore != nil {
				p.eventStore.AddEvent(ctx, map[string]interface{}{
					"RuleID":        rule.ID,
					"RuleQuery":     rule.Query,
					"Document":      filename,
					"EventType":     "not_matched",
					"Status":        "completed",
					"Message":       fmt.Sprintf("No match with %s", targetDocID),
					"OrganizationID": job.OrganizationID,
					"ClientID":  job.ClientID,
				})
			}
		}
	}
}

// askAIWithExplanation asks AI a question and returns both answer and explanation
func (p *AnalystPool) askAIWithExplanation(question, content string, isCrossDoc bool, otherDocContent string) (answer, explanation string, err error) {
	var prompt string
	
	if isCrossDoc && otherDocContent != "" {
		prompt = fmt.Sprintf(`You are analyzing two documents to answer a question.

Document A (Newly Uploaded):
%s

Document B (Existing Document):
%s

Question: %s

Answer with ONLY "YES" or "NO" on the first line.
If YES, provide a brief explanation on the second line explaining why.`, content, otherDocContent, question)
	} else {
		prompt = fmt.Sprintf(`Given the following document content, answer this yes/no question:

Document content:
%s

Question: %s

Answer with ONLY "YES" or "NO" on the first line.
If YES, provide a brief explanation on the second line explaining why.`, content, question)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	response, _, err := ai.AskQuestion(ctx, prompt)
	if err != nil {
		return "", "", err
	}

	// Parse response: first line is YES/NO, rest is explanation
	lines := strings.Split(strings.TrimSpace(response), "\n")
	answer = strings.ToUpper(strings.TrimSpace(lines[0]))
	
	if len(lines) > 1 {
		explanation = strings.TrimSpace(strings.Join(lines[1:], " "))
	} else if answer == "YES" {
		explanation = "Rule condition met based on document analysis"
	}

	return answer, explanation, nil
}

// extractRelevantChunks extracts relevant chunks for match display
func (p *AnalystPool) extractRelevantChunks(fullContent string, allChunks []string) []string {
	if len(allChunks) > 0 {
		// Use actual chunks if available
		if len(allChunks) > 3 {
			return allChunks[:3] // First 3 chunks
		}
		return allChunks
	}
	
	// Fallback: split full content into ~500 char chunks
	chunks := []string{}
	chunkSize := 500
	for i := 0; i < len(fullContent); i += chunkSize {
		end := i + chunkSize
		if end > len(fullContent) {
			end = len(fullContent)
		}
		chunks = append(chunks, fullContent[i:end])
		if len(chunks) >= 3 {
			break
		}
	}
	return chunks
}

// truncateString truncates a string to max length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
