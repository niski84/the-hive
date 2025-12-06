// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"github.com/the-hive/internal/database"
	"github.com/the-hive/internal/vectordb"
)

// PurgeHandler handles database purge requests
type PurgeHandler struct {
	vectorDB      vectordb.VectorDB
	db            *sql.DB
	auditLogStore *database.AuditLogStore
}

// NewPurgeHandler creates a new purge handler
func NewPurgeHandler(vectorDB vectordb.VectorDB, db *sql.DB, auditLogStore *database.AuditLogStore) *PurgeHandler {
	return &PurgeHandler{
		vectorDB:      vectorDB,
		db:            db,
		auditLogStore: auditLogStore,
	}
}

// PurgeRequest represents a purge request
type PurgeRequest struct {
	OrganizationID string `json:"organization_id,omitempty"` // If empty, purges all
}

// HandlePurge handles POST /api/v1/purge
func (h *PurgeHandler) HandlePurge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	var req PurgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}

	// Get organization ID from context or request
	orgID := req.OrganizationID
	if orgID == "" {
		if orgIDVal := r.Context().Value("organization_id"); orgIDVal != nil {
			if orgIDStr, ok := orgIDVal.(string); ok {
				orgID = orgIDStr
			}
		}
	}

	// Purge vectors from Qdrant
	if h.vectorDB != nil {
		ctx := r.Context()
		if orgID != "" {
			// Purge by organization
			if _, err := h.vectorDB.PurgeByOrganization(ctx, orgID); err != nil {
				log.Printf("Failed to purge vectors for org %s: %v", orgID, err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "failed to purge vectors"})
				return
			}
		} else {
			// Purge all
			if err := h.vectorDB.PurgeCollection(ctx); err != nil {
				log.Printf("Failed to purge all vectors: %v", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "failed to purge vectors"})
				return
			}
		}
	}

	// Purge database records
	if h.db != nil {
		if orgID != "" {
			// Purge chunks for organization
			_, err := h.db.Exec("DELETE FROM chunks WHERE organization_id = ?", orgID)
			if err != nil {
				log.Printf("Failed to purge chunks for org %s: %v", orgID, err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "failed to purge database"})
				return
			}
		} else {
			// Purge all chunks
			_, err := h.db.Exec("DELETE FROM chunks")
			if err != nil {
				log.Printf("Failed to purge all chunks: %v", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "failed to purge database"})
				return
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

