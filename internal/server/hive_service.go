// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/the-hive/internal/embeddings"
	"github.com/the-hive/internal/proto"
	"github.com/the-hive/internal/vectordb"
)

// HiveService implements the gRPC Hive service.
type HiveService struct {
	proto.UnimplementedHiveServer
	db        *sql.DB
	vectorDB  vectordb.VectorDB
	embedder  embeddings.Embedder
	wsManager *WebSocketManager
}

// NewHiveService wires database and vector storage dependencies.
func NewHiveService(db *sql.DB, vectorDB vectordb.VectorDB, embedder embeddings.Embedder) *HiveService {
	return &HiveService{
		db:       db,
		vectorDB: vectorDB,
		embedder: embedder,
	}
}

// SetWebSocketManager sets the WebSocket manager for notifications
func (s *HiveService) SetWebSocketManager(wsManager *WebSocketManager) {
	s.wsManager = wsManager
}

// Ingest persists chunk metadata and forwards the vector payload to the vector DB.
func (s *HiveService) Ingest(ctx context.Context, req *proto.Chunk) (*proto.Status, error) {
	if req == nil {
		return &proto.Status{Success: false, Message: "chunk payload missing"}, nil
	}

	const insertChunk = `
		INSERT OR REPLACE INTO chunks (id, document_id, content, chunk_index)
		VALUES (?, ?, ?, ?);
	`

	if _, err := s.db.ExecContext(ctx, insertChunk, req.Id, req.DocumentId, req.Content, 0); err != nil {
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
		if err := s.vectorDB.Upsert(ctx, req.Id, vector, req.Metadata); err != nil {
			log.Printf("[ERROR] Job failed: vector upsert failed for chunk (pointID: %s): %v", req.Id, err)
			// Don't fail the request - metadata is already stored
		}
	}

	// Notification Logic: Check for "CONFIDENTIAL" keyword (only on first chunk to avoid duplicates)
	if s.wsManager != nil && req.Metadata != nil {
		chunkIndex := 0
		if idx, ok := req.Metadata["chunk_index"]; ok {
			fmt.Sscanf(idx, "%d", &chunkIndex)
		}

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

	matches, err := s.vectorDB.Search(ctx, queryVector, topK)
	if err != nil {
		return &proto.Result{}, fmt.Errorf("vector search failed: %w", err)
	}

	protoMatches := make([]*proto.Match, 0, len(matches))
	for _, match := range matches {
		var content string
		if err := s.db.QueryRowContext(ctx, "SELECT content FROM chunks WHERE id = ?", match.ID).Scan(&content); err != nil {
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
