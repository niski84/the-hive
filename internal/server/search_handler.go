// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/the-hive/internal/ai"
	"github.com/the-hive/internal/embeddings"
	"github.com/the-hive/internal/vectordb"
)

// SearchRequest represents the search request payload
type SearchRequest struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k"`
}

// SearchResponse represents the search response
type SearchResponse struct {
	Matches []SearchMatch `json:"matches"`
	Count   int           `json:"count"`
}

// SearchMatch represents a single search result
type SearchMatch struct {
	ChunkID    string            `json:"chunk_id"`
	DocumentID string            `json:"document_id"`
	Content    string            `json:"content"`
	Score      float32           `json:"score"`
	Metadata   map[string]string `json:"metadata"`
}

// SearchHandler holds dependencies for the search handler
type SearchHandler struct {
	vectorDB vectordb.VectorDB
	embedder embeddings.Embedder
}

// NewSearchHandler creates a new search handler with dependencies
func NewSearchHandler(vectorDB vectordb.VectorDB, embedder embeddings.Embedder) *SearchHandler {
	return &SearchHandler{
		vectorDB: vectorDB,
		embedder: embedder,
	}
}

// HandleSearch handles POST /api/v1/search requests
func (h *SearchHandler) HandleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("invalid JSON: %v", err)})
		return
	}

	if req.Query == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "query is required"})
		return
	}

	// Default to top 3 if not specified
	if req.TopK <= 0 {
		req.TopK = 3
	}

	ctx := r.Context()

	// Generate query embedding
	var queryVector []float32
	var err error

	// Try using the embedder if available, otherwise use ai.GenerateEmbedding
	if h.embedder != nil {
		queryVector, err = h.embedder.EmbedText(ctx, req.Query)
		if err != nil {
			log.Printf("Failed to generate embedding with embedder: %v, falling back to ai.GenerateEmbedding", err)
			queryVector, err = ai.GenerateEmbedding(req.Query)
		}
	} else {
		queryVector, err = ai.GenerateEmbedding(req.Query)
	}

	if err != nil {
		log.Printf("Failed to generate query embedding: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to generate embedding: %v", err)})
		return
	}

	// Search in Qdrant
	matches, err := h.vectorDB.Search(ctx, queryVector, req.TopK)
	if err != nil {
		log.Printf("Failed to search Qdrant: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("search failed: %v", err)})
		return
	}

	// Convert matches to response format
	response := SearchResponse{
		Matches: make([]SearchMatch, 0, len(matches)),
		Count:   len(matches),
	}

	for _, match := range matches {
		// Extract content from metadata
		content := match.Metadata["content"]
		if content == "" {
			content = "No content available"
		}

		response.Matches = append(response.Matches, SearchMatch{
			ChunkID:    match.ID,
			DocumentID: match.DocumentID,
			Content:    content,
			Score:      match.Score,
			Metadata:   match.Metadata,
		})
	}

	// Return results
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
