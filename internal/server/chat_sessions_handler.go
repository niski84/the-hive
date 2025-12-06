// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/the-hive/internal/database"
)

// HandleGetSessions handles GET /api/v1/chat/sessions
func HandleGetSessions(w http.ResponseWriter, r *http.Request, chatStore *database.ChatStore) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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

	// Get sessions for user
	sessions, err := chatStore.GetUserSessions(dbUser.ID, orgID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

// HandleCreateSession handles POST /api/v1/chat/sessions
func HandleCreateSession(w http.ResponseWriter, r *http.Request, chatStore *database.ChatStore) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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

	// Create session with default title
	session, err := chatStore.CreateSession(dbUser.ID, orgID, "New Chat")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(session)
}

// HandleGetSessionMessages handles GET /api/v1/chat/sessions/{id}/messages
func HandleGetSessionMessages(w http.ResponseWriter, r *http.Request, chatStore *database.ChatStore) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract session ID from path
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/api/v1/chat/sessions/")
	path = strings.TrimSuffix(path, "/messages")
	sessionID := strings.Trim(path, "/")

	if sessionID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "session ID required"})
		return
	}

	// Get messages for session
	messages, err := chatStore.GetSessionMessages(sessionID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

// HandleDeleteSession handles DELETE /api/v1/chat/sessions/{id}
func HandleDeleteSession(w http.ResponseWriter, r *http.Request, chatStore *database.ChatStore) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract session ID from path
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/api/v1/chat/sessions/")
	sessionID := strings.Trim(path, "/")

	if sessionID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "session ID required"})
		return
	}

	// Delete session
	if err := chatStore.DeleteSession(sessionID); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

