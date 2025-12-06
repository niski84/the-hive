// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/the-hive/internal/ai"
	"github.com/the-hive/internal/database"
	"github.com/the-hive/internal/processor"
	"github.com/the-hive/internal/vectordb"
	"github.com/the-hive/internal/worker"
)

// IngestRequest represents the ingestion request payload
type IngestRequest struct {
	FilePath string            `json:"file_path"`
	Content  string            `json:"content"`
	Metadata map[string]string `json:"metadata"`
}

// IngestHandler holds dependencies for the ingest handler
type IngestHandler struct {
	vectorDB      vectordb.VectorDB
	chunker       *processor.Chunker
	wsManager     *WebSocketManager
	analystPool   *worker.AnalystPool
	taggerPool    *worker.TaggerPool
	eventLogger   *database.EventLogger
	auditLogStore *database.AuditLogStore
}

// NewIngestHandler creates a new ingest handler with dependencies
func NewIngestHandler(vectorDB vectordb.VectorDB, wsManager *WebSocketManager, analystPool *worker.AnalystPool, taggerPool *worker.TaggerPool, eventLogger *database.EventLogger, auditLogStore *database.AuditLogStore) *IngestHandler {
	return &IngestHandler{
		vectorDB:      vectorDB,
		chunker:       processor.NewChunker(),
		wsManager:     wsManager,
		analystPool:   analystPool,
		taggerPool:    taggerPool,
		eventLogger:   eventLogger,
		auditLogStore: auditLogStore,
	}
}

// HandleIngest handles POST /api/v1/ingest requests
func (h *IngestHandler) HandleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	var req IngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("invalid JSON: %v", err)})
		return
	}

	// Dump payload to console
	fmt.Printf(" [RECEIVED] %s (%d chars)\n", req.FilePath, len(req.Content))
	if len(req.Metadata) > 0 {
		fmt.Printf(" [METADATA] %+v\n", req.Metadata)
	}

	// Chunk the content
	chunks, err := h.chunker.ChunkText(req.Content)
	if err != nil {
		log.Printf("Failed to chunk text: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to chunk text: %v", err)})
		return
	}

	fmt.Printf(" [CHUNKED] %s into %d chunks\n", req.FilePath, len(chunks))

	// Generate embeddings and upsert to Qdrant
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	documentID := req.Metadata["filename"]
	if documentID == "" {
		documentID = req.FilePath
	}

	successCount := 0
	failedChunks := 0
	var lastError error

	for i, chunk := range chunks {
		// Generate embedding
		embedding, err := ai.GenerateEmbedding(chunk)
		if err != nil {
			log.Printf("[ERROR] Job failed: Failed to generate embedding for chunk %d: %v", i, err)
			lastError = err
			failedChunks++
			continue
		}

		// Create a deterministic UUID based on the file path and chunk index
		// This ensures if we re-ingest the same file, we update the existing vectors (Idempotency)
		seed := fmt.Sprintf("%s-%d", req.FilePath, i)
		pointID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(seed)).String()

		// Prepare metadata for Qdrant
		// Ensure filename, chunk_index, and file_path are explicitly in the payload
		metadata := make(map[string]string)
		metadata["document_id"] = documentID
		metadata["chunk_index"] = fmt.Sprintf("%d", i)
		metadata["content"] = chunk // Store content in metadata
		// Explicitly add filename (preserve from request metadata)
		if req.Metadata["filename"] != "" {
			metadata["filename"] = req.Metadata["filename"]
		} else {
			metadata["filename"] = documentID // Fallback to documentID
		}
		// Explicitly add file_path (preserve from request)
		if req.Metadata["file_path"] != "" {
			metadata["file_path"] = req.Metadata["file_path"]
		} else {
			metadata["file_path"] = req.FilePath // Fallback to FilePath
		}
		// Preserve other metadata fields
		if req.Metadata["filetype"] != "" {
			metadata["filetype"] = req.Metadata["filetype"]
		}
		if req.Metadata["client_id"] != "" {
			metadata["client_id"] = req.Metadata["client_id"]
		}

		// Upsert to Qdrant
		if err := h.vectorDB.Upsert(ctx, pointID, embedding, metadata); err != nil {
			log.Printf("[ERROR] Job failed: Failed to upsert chunk %d to Qdrant (pointID: %s): %v", i, pointID, err)
			lastError = err
			failedChunks++
			continue
		}

		// Send to tagging pool for auto-tagging (non-blocking, only for first chunk)
		if h.taggerPool != nil && i == 0 {
			job := worker.TaggingJob{
				ChunkID:  pointID, // Use the deterministic UUID
				Content:  chunk,   // Use first chunk for tagging
				VectorDB: h.vectorDB,
			}
			h.taggerPool.Enqueue(job)
		}

		successCount++
	}

	fmt.Printf(" [INGESTED] %s: %d/%d chunks stored\n", req.FilePath, successCount, len(chunks))

	// Log error summary if any chunks failed
	if failedChunks > 0 {
		errorMsg := fmt.Sprintf("Failed to process %d/%d chunks", failedChunks, len(chunks))
		if lastError != nil {
			errorMsg += fmt.Sprintf(" (last error: %v)", lastError)
		}
		log.Printf("[ERROR] Job failed: %s for file %s", errorMsg, req.FilePath)
	}

	// Log ingestion event
	if h.eventLogger != nil {
		documentName := req.Metadata["filename"]
		if documentName == "" {
			documentName = req.FilePath
		}
		details := fmt.Sprintf("Ingested %d chunks", successCount)
		if failedChunks > 0 {
			details += fmt.Sprintf(" (%d failed)", failedChunks)
		}
		if err := h.eventLogger.LogEvent("ingest", documentName, details); err != nil {
			log.Printf("Failed to log ingestion event: %v", err)
		}
	}

	// Log audit entry
	if h.auditLogStore != nil {
		clientIP := getClientIPFromRequest(r)
		documentName := req.Metadata["filename"]
		if documentName == "" {
			documentName = req.FilePath
		}
		// Get organization ID from context
		orgID := ""
		if orgIDVal := r.Context().Value("organization_id"); orgIDVal != nil {
			if orgIDStr, ok := orgIDVal.(string); ok {
				orgID = orgIDStr
			}
		}
		details := fmt.Sprintf("Client [%s] uploaded file [%s] (%d chunks)", clientIP, documentName, successCount)
		if err := h.auditLogStore.LogAction(clientIP, database.AuditActionIngest, details, orgID); err != nil {
			log.Printf("Failed to log ingest audit entry: %v", err)
		}
	}

	// Send to analyst pool for rule checking (non-blocking)
	if h.analystPool != nil {
		clientID := req.Metadata["client_id"]
		job := worker.AnalystJob{
			FilePath: req.FilePath,
			Content:  req.Content,
			Metadata: req.Metadata,
			ClientID: clientID,
		}
		h.analystPool.Enqueue(job)
	}

	// Legacy notification logic: Check for "CONFIDENTIAL" keyword (keep for backward compatibility)
	if h.wsManager != nil {
		contentUpper := strings.ToUpper(req.Content)
		if strings.Contains(contentUpper, "CONFIDENTIAL") {
			// Get client_id from metadata (sent by drone-client)
			clientID := req.Metadata["client_id"]
			if clientID != "" {
				filename := req.Metadata["filename"]
				if filename == "" {
					filename = req.FilePath
				}

				notification := NotificationMessage{
					Type:    "ALERT",
					Message: fmt.Sprintf("Sensitive document detected: %s", filename),
					Level:   "critical",
				}

				if err := h.wsManager.SendNotification(clientID, notification); err != nil {
					log.Printf("Failed to send notification to client %s: %v", clientID, err)
				}

				// Log alert event
				if h.eventLogger != nil {
					details := fmt.Sprintf("Alert triggered: CONFIDENTIAL keyword detected")
					if err := h.eventLogger.LogEvent("alert", filename, details); err != nil {
						log.Printf("Failed to log alert event: %v", err)
					}
				}
			}
		}
	}

	// Return 200 OK
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "ok",
		"message":       fmt.Sprintf("Processed %s (%d chunks stored)", req.FilePath, successCount),
		"chunks_total":  len(chunks),
		"chunks_stored": successCount,
	})
}

// getClientIPFromRequest extracts the client IP address from the request
func getClientIPFromRequest(r *http.Request) string {
	// Check X-Forwarded-For header (for proxies/load balancers)
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		ips := strings.Split(forwarded, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	// Remove port if present
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}
