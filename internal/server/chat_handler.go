// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/the-hive/internal/ai"
	"github.com/the-hive/internal/database"
	"github.com/the-hive/internal/embeddings"
	"github.com/the-hive/internal/vectordb"
)

// ChatHandler handles chat/Q&A requests
type ChatHandler struct {
	vectorDB      vectordb.VectorDB
	embedder      embeddings.Embedder
	auditLogStore *database.AuditLogStore
	chatStore     *database.ChatStore
	orgStore      *database.OrganizationStore
	usageStore    *database.UsageStore
}

// NewChatHandler creates a new chat handler
func NewChatHandler(vectorDB vectordb.VectorDB, embedder embeddings.Embedder, auditLogStore *database.AuditLogStore, chatStore *database.ChatStore, orgStore *database.OrganizationStore, usageStore *database.UsageStore) *ChatHandler {
	return &ChatHandler{
		vectorDB:      vectorDB,
		embedder:      embedder,
		auditLogStore: auditLogStore,
		chatStore:     chatStore,
		orgStore:      orgStore,
		usageStore:    usageStore,
	}
}

// ChatRequest represents a chat request
type ChatRequest struct {
	Query     string `json:"query"`
	SessionID string `json:"session_id,omitempty"`
}

// ChatResponse represents a chat response
type ChatResponse struct {
	Answer    string                   `json:"answer"`
	SessionID string                   `json:"session_id"`
	Citations []map[string]interface{} `json:"citations,omitempty"`
}

// HandleChat handles POST /api/v1/chat
func (h *ChatHandler) HandleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Query == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "query is required"})
		return
	}

	// Get user from context
	user := r.Context().Value("user")
	if user == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "not authenticated"})
		return
	}

	dbUser, ok := user.(*database.User)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid user type"})
		return
	}

	// Get organization ID from context
	orgID := ""
	if orgIDVal := r.Context().Value("organization_id"); orgIDVal != nil {
		if orgIDStr, ok := orgIDVal.(string); ok {
			orgID = orgIDStr
		}
	}

	// Generate query embedding
	ctx := r.Context()
	var queryVector []float32
	var err error

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

	// Search for relevant context
	matches, err := h.vectorDB.Search(ctx, queryVector, 5, orgID)
	if err != nil {
		log.Printf("Failed to search: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to search"})
		return
	}

	// Build context from matches
	contextText := ""
	for _, match := range matches {
		// Extract content from metadata
		content := match.Metadata["content"]
		if content == "" {
			content = "No content available"
		}
		contextText += content + "\n\n"
	}

	// Generate answer using AI (simple implementation - just return the query for now)
	// TODO: Implement proper AI answer generation
	answer := fmt.Sprintf("Based on the context: %s", req.Query)
	if contextText != "" {
		answer = fmt.Sprintf("Based on the search results, here's what I found related to your question: %s", req.Query)
	}

	// Create or get session
	sessionID := req.SessionID
	if sessionID == "" {
		// Create new session
		session, err := h.chatStore.CreateSession(dbUser.ID, orgID, req.Query)
		if err != nil {
			log.Printf("Failed to create session: %v", err)
		} else {
			sessionID = session.ID
		}
	}

	// Save messages to session
	if sessionID != "" {
		// Save user message
		if err := h.chatStore.AddMessage(sessionID, "user", req.Query, nil); err != nil {
			log.Printf("Failed to save user message: %v", err)
		}

		// Save assistant message with citations
		citations := make([]map[string]interface{}, 0)
		for _, match := range matches {
			content := match.Metadata["content"]
			if content == "" {
				content = "No content available"
			}
			chunkID := match.Metadata["chunk_id"]
			if chunkID == "" {
				chunkID = match.ID
			}
			citations = append(citations, map[string]interface{}{
				"document_id": match.DocumentID,
				"chunk_id":    chunkID,
				"content":     content,
				"score":       match.Score,
			})
		}

		if err := h.chatStore.AddMessage(sessionID, "assistant", answer, map[string]interface{}{
			"citations": citations,
		}); err != nil {
			log.Printf("Failed to save assistant message: %v", err)
		}
	}

	// Build response
	response := ChatResponse{
		Answer:    answer,
		SessionID: sessionID,
		Citations: make([]map[string]interface{}, 0),
	}

	for _, match := range matches {
		content := match.Metadata["content"]
		if content == "" {
			content = "No content available"
		}
		chunkID := match.Metadata["chunk_id"]
		if chunkID == "" {
			chunkID = match.ID
		}
		response.Citations = append(response.Citations, map[string]interface{}{
			"document_id": match.DocumentID,
			"chunk_id":    chunkID,
			"content":     content,
			"score":       match.Score,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleChatPage serves the chat page
func HandleChatPage(w http.ResponseWriter, r *http.Request, metadataStore *database.SystemMetadataStore, orgStore *database.OrganizationStore) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := renderTemplate(w, "chat.html", nil); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleAnalystPage serves the analyst page
func HandleAnalystPage(w http.ResponseWriter, r *http.Request, metadataStore *database.SystemMetadataStore, orgStore *database.OrganizationStore) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := renderTemplate(w, "analyst.html", nil); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

