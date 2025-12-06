// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/the-hive/internal/embeddings"
	"github.com/the-hive/internal/proto"
	"github.com/the-hive/internal/vectordb"
	"github.com/the-hive/internal/worker"
)

// HiveService implements the gRPC Hive service.
type HiveService struct {
	proto.UnimplementedHiveServer
	db          *sql.DB
	vectorDB    vectordb.VectorDB
	embedder    embeddings.Embedder
	wsManager   *WebSocketManager
	analystPool AnalystPoolInterface // Interface to avoid circular dependency
	// Track documents being ingested to trigger analysis when complete
	docTrackers map[string]*documentTracker
	docMu       sync.Mutex
}

// documentTracker tracks chunks for a document to determine when it's complete
type documentTracker struct {
	documentID   string
	chunks       []string
	metadata     map[string]string
	lastChunk    time.Time
	totalChunks  int
	receivedChunks int
	mu           sync.Mutex
}

// AnalystPoolInterface is an interface to avoid circular dependency with worker package
type AnalystPoolInterface interface {
	Enqueue(job worker.AnalystJob)
}

// NewHiveService wires database and vector storage dependencies.
func NewHiveService(db *sql.DB, vectorDB vectordb.VectorDB, embedder embeddings.Embedder) *HiveService {
	return &HiveService{
		db:          db,
		vectorDB:    vectorDB,
		embedder:    embedder,
		docTrackers: make(map[string]*documentTracker),
	}
}

// SetWebSocketManager sets the WebSocket manager for notifications
func (s *HiveService) SetWebSocketManager(wsManager *WebSocketManager) {
	s.wsManager = wsManager
}

// SetAnalystPool sets the analyst pool for rule checking
func (s *HiveService) SetAnalystPool(analystPool AnalystPoolInterface) {
	s.analystPool = analystPool
}

// Ingest persists chunk metadata and forwards the vector payload to the vector DB.
func (s *HiveService) Ingest(ctx context.Context, req *proto.Chunk) (*proto.Status, error) {
	if req == nil {
		return &proto.Status{Success: false, Message: "chunk payload missing"}, nil
	}

	// Extract organization_id from metadata for multi-tenancy
	orgID := ""
	if req.Metadata != nil {
		orgID = req.Metadata["organization_id"]
	}
	
	const insertChunk = `
		INSERT OR REPLACE INTO chunks (id, document_id, content, chunk_index, organization_id)
		VALUES (?, ?, ?, ?, ?);
	`

	if _, err := s.db.ExecContext(ctx, insertChunk, req.Id, req.DocumentId, req.Content, 0, orgID); err != nil {
		return &proto.Status{
			Success: false,
			Message: fmt.Sprintf("failed to store chunk: %v", err),
		}, nil
	}

	// Generate embedding if not provided
	var vector []float32
	if req.Vector != nil && len(req.Vector) > 0 {
		vector = req.Vector
	} else if s.embedder != nil {
		embedding, err := s.embedder.EmbedText(ctx, req.Content)
		if err != nil {
			log.Printf("failed to generate embedding for chunk %s: %v", req.Id, err)
			// Continue without vector - chunk is still stored in SQLite
		} else {
			vector = embedding
		}
	}

	// Store vector in Qdrant if we have one
	if len(vector) > 0 {
		// Ensure content is in metadata for RAG retrieval
		metadata := make(map[string]string)
		if req.Metadata != nil {
			for k, v := range req.Metadata {
				metadata[k] = v
			}
		}
		// Add content to metadata if not already present
		// This is critical for chat/RAG to work - content must be in Qdrant payload
		if req.Content != "" && metadata["content"] == "" {
			metadata["content"] = req.Content
		}
		
		if err := s.vectorDB.Upsert(ctx, req.Id, vector, metadata); err != nil {
			log.Printf("[ERROR] Job failed: vector upsert failed for chunk (pointID: %s): %v", req.Id, err)
			// Don't fail the request - metadata is already stored
		}
	}

	// Track document chunks for analyst processing
	chunkIndex := 0
	if idx, ok := req.Metadata["chunk_index"]; ok {
		fmt.Sscanf(idx, "%d", &chunkIndex)
	}
	
	totalChunks := 0
	if total, ok := req.Metadata["total_chunks"]; ok {
		fmt.Sscanf(total, "%d", &totalChunks)
	}

	// Track this chunk for document analysis
	s.docMu.Lock()
	tracker, exists := s.docTrackers[req.DocumentId]
	if !exists {
		tracker = &documentTracker{
			documentID:     req.DocumentId,
			chunks:         make([]string, 0),
			metadata:       make(map[string]string),
			totalChunks:    totalChunks,
			receivedChunks: 0,
		}
		if req.Metadata != nil {
			for k, v := range req.Metadata {
				tracker.metadata[k] = v
			}
		}
		s.docTrackers[req.DocumentId] = tracker
	}
	tracker.mu.Lock()
	tracker.chunks = append(tracker.chunks, req.Content)
	tracker.receivedChunks++
	tracker.lastChunk = time.Now()
	tracker.mu.Unlock()
	s.docMu.Unlock()

	// Check if document is complete and trigger analysis
	shouldAnalyze := false
	if totalChunks > 0 && tracker.receivedChunks >= totalChunks {
		// All chunks received
		shouldAnalyze = true
		log.Printf("[DEBUG] Document %s complete: received %d/%d chunks", req.DocumentId, tracker.receivedChunks, totalChunks)
	} else if chunkIndex == 0 && totalChunks == 0 {
		// No total_chunks metadata - use timeout approach: analyze after 2 seconds of no new chunks
		go func() {
			time.Sleep(2 * time.Second)
			tracker.mu.Lock()
			lastChunk := tracker.lastChunk
			received := tracker.receivedChunks
			tracker.mu.Unlock()
			
			// If no new chunks in 2 seconds, assume document is complete
			if time.Since(lastChunk) >= 2*time.Second {
				s.docMu.Lock()
				if t, exists := s.docTrackers[req.DocumentId]; exists && t == tracker {
					shouldAnalyze = true
					log.Printf("[DEBUG] Document %s assumed complete after timeout: received %d chunks", req.DocumentId, received)
				}
				s.docMu.Unlock()
			}
		}()
	}

	// Trigger analyst if document is complete
	if shouldAnalyze && s.analystPool != nil {
		tracker.mu.Lock()
		fullContent := strings.Join(tracker.chunks, "\n\n")
		metadata := tracker.metadata
		clientID := metadata["client_id"]
		filePath := metadata["file_path"]
		if filePath == "" {
			filePath = req.DocumentId
		}
		tracker.mu.Unlock()

		log.Printf("[DEBUG] Enqueueing analyst job for gRPC document: %s (content length: %d, chunks: %d)", filePath, len(fullContent), len(tracker.chunks))
		
		// Create analyst job
		job := worker.AnalystJob{
			FilePath:  filePath,
			Content:   fullContent,
			Metadata:  metadata,
			ClientID:  clientID,
			AllChunks: tracker.chunks,
		}
		s.analystPool.Enqueue(job)
		
		// Clean up tracker
		s.docMu.Lock()
		delete(s.docTrackers, req.DocumentId)
		s.docMu.Unlock()
	}

	// Notification Logic: Check for "CONFIDENTIAL" keyword (only on first chunk to avoid duplicates)
	if s.wsManager != nil && req.Metadata != nil {
		// Only check first chunk to avoid duplicate notifications
		if chunkIndex == 0 {
			contentUpper := strings.ToUpper(req.Content)
			if strings.Contains(contentUpper, "CONFIDENTIAL") {
				clientID := req.Metadata["client_id"]
				if clientID != "" {
					filename := req.Metadata["filename"]
					if filename == "" {
						filename = req.DocumentId
					}

					notification := NotificationMessage{
						Type:    "ALERT",
						Message: fmt.Sprintf("Sensitive document detected: %s", filename),
						Level:   "critical",
					}

					if err := s.wsManager.SendNotification(clientID, notification); err != nil {
						log.Printf("Failed to send notification to client %s: %v", clientID, err)
					}
				}
			}
		}
	}

	return &proto.Status{
		Success: true,
		Message: "chunk ingested",
		ChunkId: req.Id,
	}, nil
}

// Query delegates to the vector DB and stitches the textual payload from SQLite.
func (s *HiveService) Query(ctx context.Context, req *proto.Search) (*proto.Result, error) {
	// Generate query embedding if not provided
	var queryVector []float32
	if req.QueryVector != nil && len(req.QueryVector) > 0 {
		queryVector = req.QueryVector
	} else if s.embedder != nil && req.Query != "" {
		embedding, err := s.embedder.EmbedText(ctx, req.Query)
		if err != nil {
			return &proto.Result{}, fmt.Errorf("failed to generate query embedding: %w", err)
		}
		queryVector = embedding
	} else {
		return &proto.Result{}, fmt.Errorf("query text or vector is required")
	}

	topK := int(req.TopK)
	if topK <= 0 {
		topK = 10 // default
	}

	// For gRPC calls, organization_id is not available in context
	// We'll search all orgs but filter results by organization_id from metadata
	// This is a legacy endpoint - ideally organization_id should come from gRPC metadata
	matches, err := s.vectorDB.Search(ctx, queryVector, topK, "")
	if err != nil {
		return &proto.Result{}, fmt.Errorf("vector search failed: %w", err)
	}

	protoMatches := make([]*proto.Match, 0, len(matches))
	for _, match := range matches {
		// Extract organization_id from match metadata for filtering
		matchOrgID := ""
		if match.Metadata != nil {
			matchOrgID = match.Metadata["organization_id"]
		}
		
		var content string
		// Filter chunks query by organization_id for multi-tenancy isolation
		var query string
		var args []interface{}
		if matchOrgID != "" {
			query = "SELECT content FROM chunks WHERE id = ? AND organization_id = ?"
			args = []interface{}{match.ID, matchOrgID}
		} else {
			// Fallback: if no org_id in metadata, still filter by id (backward compatibility)
			query = "SELECT content FROM chunks WHERE id = ?"
			args = []interface{}{match.ID}
		}
		
		if err := s.db.QueryRowContext(ctx, query, args...).Scan(&content); err != nil {
			// Missing row should not fail the entire request.
			log.Printf("failed to fetch chunk %s content: %v", match.ID, err)
			continue
		}
		protoMatches = append(protoMatches, &proto.Match{
			ChunkId:    match.ID,
			DocumentId: match.DocumentID,
			Content:    content,
			Score:      match.Score,
			Metadata:   match.Metadata,
		})
	}

	return &proto.Result{Matches: protoMatches}, nil
}
