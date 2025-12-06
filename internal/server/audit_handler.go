// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/the-hive/internal/database"
)

// HandleAuditLogs handles GET /api/v1/audit requests
func HandleAuditLogs(w http.ResponseWriter, r *http.Request, auditLogStore *database.AuditLogStore) {
	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	// Get limit from query parameter (default to 50)
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		var err error
		if limit, err = parseAuditLimit(limitStr); err != nil {
			limit = 50
		}
	}

	// Get organization ID from context
	orgID := ""
	if orgIDVal := r.Context().Value("organization_id"); orgIDVal != nil {
		if orgIDStr, ok := orgIDVal.(string); ok {
			orgID = orgIDStr
		}
	}
	logs, err := auditLogStore.GetRecentLogs(limit, "", orgID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"logs":  logs,
		"count": len(logs),
	})
}

// parseAuditLimit is a simple helper to parse integer from string
func parseAuditLimit(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}
