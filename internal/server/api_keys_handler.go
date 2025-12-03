// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/the-hive/internal/database"
)

// HandleGenerateAPIKey handles POST /api/v1/keys/generate
func HandleGenerateAPIKey(w http.ResponseWriter, r *http.Request, apiKeyStore *database.APIKeyStore) {
	log.Printf("API Key Manager: Receiving POST /api/v1/keys/generate request")
	
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	var req struct {
		ClientName string `json:"client_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("API Key Manager: Error decoding request: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("invalid JSON: %v", err)})
		return
	}

	if req.ClientName == "" {
		req.ClientName = "Unnamed Client"
	}

	log.Printf("API Key Manager: Generating key for client: %s", req.ClientName)
	key, err := apiKeyStore.GenerateKey(req.ClientName)
	if err != nil {
		log.Printf("API Key Manager: Error generating key: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	log.Printf("API Key Manager: Successfully generated key: %s", key)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"key":         key,
		"client_name": req.ClientName,
	})
}

// HandleListAPIKeys handles GET /api/v1/keys
func HandleListAPIKeys(w http.ResponseWriter, r *http.Request, apiKeyStore *database.APIKeyStore) {
	// Removed noisy logging - this endpoint is polled frequently by the UI
	// log.Printf("API Key Manager: Receiving GET /api/v1/keys request")
	
	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	keys, err := apiKeyStore.ListKeys()
	if err != nil {
		log.Printf("API Key Manager: Error listing keys: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Removed noisy logging - this endpoint is polled frequently by the UI
	// log.Printf("API Key Manager: Returning %d keys", len(keys))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"keys": keys,
	})
}

// HandleRevokeAPIKey handles POST /api/v1/keys/revoke
func HandleRevokeAPIKey(w http.ResponseWriter, r *http.Request, apiKeyStore *database.APIKeyStore) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	var req struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("invalid JSON: %v", err)})
		return
	}

	if req.Key == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "key is required"})
		return
	}

	if err := apiKeyStore.RevokeKey(req.Key); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

