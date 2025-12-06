// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"encoding/json"
	"net/http"

	"github.com/the-hive/internal/database"
)

// HandleListAPIKeys lists all API keys for the current organization
func HandleListAPIKeys(w http.ResponseWriter, r *http.Request, apiKeyStore *database.APIKeyStore) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	keys, err := apiKeyStore.ListKeys()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(keys)
}

// HandleGenerateAPIKey generates a new API key
func HandleGenerateAPIKey(w http.ResponseWriter, r *http.Request, apiKeyStore *database.APIKeyStore) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get organization ID from context (set by RequireTenant middleware)
	orgID := r.Context().Value("organization_id")
	if orgID == nil {
		http.Error(w, "Organization not found", http.StatusInternalServerError)
		return
	}

	orgIDStr, ok := orgID.(string)
	if !ok {
		http.Error(w, "Invalid organization ID", http.StatusInternalServerError)
		return
	}

	key, err := apiKeyStore.GenerateKey(orgIDStr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(key)
}

// HandleRevokeAPIKey revokes an API key
func HandleRevokeAPIKey(w http.ResponseWriter, r *http.Request, apiKeyStore *database.APIKeyStore) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	keyID := r.URL.Query().Get("id")
	if keyID == "" {
		http.Error(w, "Key ID required", http.StatusBadRequest)
		return
	}

	err := apiKeyStore.RevokeKey(keyID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// HandleEnableAPIKey enables an API key
func HandleEnableAPIKey(w http.ResponseWriter, r *http.Request, apiKeyStore *database.APIKeyStore) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	keyID := r.URL.Query().Get("id")
	if keyID == "" {
		http.Error(w, "Key ID required", http.StatusBadRequest)
		return
	}

	err := apiKeyStore.EnableKey(keyID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// HandleDeleteAPIKey deletes an API key
func HandleDeleteAPIKey(w http.ResponseWriter, r *http.Request, apiKeyStore *database.APIKeyStore) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	keyID := r.URL.Query().Get("id")
	if keyID == "" {
		http.Error(w, "Key ID required", http.StatusBadRequest)
		return
	}

	err := apiKeyStore.DeleteKey(keyID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
