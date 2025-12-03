// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/the-hive/internal/database"
)

var healthAPIKeyStore *database.APIKeyStore

// SetHealthAPIKeyStore sets the API key store for health endpoint tracking
func SetHealthAPIKeyStore(store *database.APIKeyStore) {
	healthAPIKeyStore = store
}

// HandleHealth handles GET /api/v1/health requests
func HandleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	// Optionally update last_seen_at if API key is provided (for client heartbeat tracking)
	if healthAPIKeyStore != nil {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			key := strings.TrimSpace(authHeader)
			if strings.HasPrefix(key, "Bearer ") {
				key = strings.TrimPrefix(key, "Bearer ")
			}
			if key != "" {
				if err := healthAPIKeyStore.UpdateLastSeen(key); err != nil {
					log.Printf("Warning: Failed to update last_seen_at in health endpoint: %v", err)
				}
			}
		}
	}

	response := map[string]string{
		"status":  "up",
		"version": "1.0",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

