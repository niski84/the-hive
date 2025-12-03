// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/the-hive/internal/ai"
	"github.com/the-hive/internal/database"
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
	vectorDB      vectordb.VectorDB
	embedder      embeddings.Embedder
	auditLogStore *database.AuditLogStore
}

// NewSearchHandler creates a new search handler with dependencies
func NewSearchHandler(vectorDB vectordb.VectorDB, embedder embeddings.Embedder, auditLogStore *database.AuditLogStore) *SearchHandler {
	return &SearchHandler{
		vectorDB:      vectorDB,
		embedder:      embedder,
		auditLogStore: auditLogStore,
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
			// Log for debugging
			log.Printf("Warning: No content found in metadata for chunk %s. Available keys: %v", match.ID, getMapKeys(match.Metadata))
			content = "No content available"
		}

		// Extract tags from metadata (stored as JSON string in "tags" field)
		tags := []string{}
		if tagsJSON := match.Metadata["tags"]; tagsJSON != "" {
			if err := json.Unmarshal([]byte(tagsJSON), &tags); err != nil {
				log.Printf("Failed to parse tags for chunk %s: %v", match.ID, err)
			}
		}

		// Add tags to metadata for frontend
		metadata := make(map[string]string)
		for k, v := range match.Metadata {
			metadata[k] = v
		}
		// Store tags as comma-separated string for easier frontend access
		if len(tags) > 0 {
			tagsStr := ""
			for i, tag := range tags {
				if i > 0 {
					tagsStr += ","
				}
				tagsStr += tag
			}
			metadata["tags_list"] = tagsStr
		}

		response.Matches = append(response.Matches, SearchMatch{
			ChunkID:    match.ID,
			DocumentID: match.DocumentID,
			Content:    content,
			Score:      match.Score,
			Metadata:   metadata,
		})
	}

	// Log audit entry
	if h.auditLogStore != nil {
		clientIP := getClientIP(r)
		details := fmt.Sprintf("Client [%s] searched for [%s]", clientIP, req.Query)
		if err := h.auditLogStore.LogAction(clientIP, database.AuditActionSearch, details); err != nil {
			log.Printf("Failed to log search audit entry: %v", err)
		}
	}

	// Return results
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// getClientIP extracts the client IP address from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxies)
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		ips := strings.Split(forwarded, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}
	// Check X-Real-IP header
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}
	// Fallback to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// getMapKeys returns all keys from a map (helper for debugging)
func getMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
