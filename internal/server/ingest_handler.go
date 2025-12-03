// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/the-hive/internal/ai"
	"github.com/the-hive/internal/processor"
	"github.com/the-hive/internal/vectordb"
)

// IngestRequest represents the ingestion request payload
type IngestRequest struct {
	FilePath string            `json:"file_path"`
	Content  string            `json:"content"`
	Metadata map[string]string `json:"metadata"`
}

// IngestHandler holds dependencies for the ingest handler
type IngestHandler struct {
	vectorDB vectordb.VectorDB
	chunker  *processor.Chunker
}

// NewIngestHandler creates a new ingest handler with dependencies
func NewIngestHandler(vectorDB vectordb.VectorDB) *IngestHandler {
	return &IngestHandler{
		vectorDB: vectorDB,
		chunker:  processor.NewChunker(),
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
	for i, chunk := range chunks {
		// Generate embedding
		embedding, err := ai.GenerateEmbedding(chunk)
		if err != nil {
			log.Printf("Failed to generate embedding for chunk %d: %v", i, err)
			continue
		}

		// Create unique chunk ID
		chunkID := fmt.Sprintf("%s-%d-%s", documentID, i, uuid.New().String())

		// Prepare metadata for Qdrant
		metadata := make(map[string]string)
		metadata["document_id"] = documentID
		metadata["chunk_index"] = fmt.Sprintf("%d", i)
		metadata["content"] = chunk // Store content in metadata
		metadata["filename"] = req.Metadata["filename"]
		if req.Metadata["filetype"] != "" {
			metadata["filetype"] = req.Metadata["filetype"]
		}
		if req.Metadata["file_path"] != "" {
			metadata["file_path"] = req.Metadata["file_path"]
		}

		// Upsert to Qdrant
		if err := h.vectorDB.Upsert(ctx, chunkID, embedding, metadata); err != nil {
			log.Printf("Failed to upsert chunk %d to Qdrant: %v", i, err)
			continue
		}

		successCount++
	}

	fmt.Printf(" [INGESTED] %s: %d/%d chunks stored\n", req.FilePath, successCount, len(chunks))

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
