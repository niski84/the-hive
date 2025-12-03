// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// ConfigRequest represents the configuration update request
type ConfigRequest struct {
	AIProvider string `json:"ai_provider"`
	APIKey     string `json:"api_key"`
	QdrantURL  string `json:"qdrant_url"`
	RedisURL   string `json:"redis_url"`
}

// ConfigResponse represents the current configuration
type ConfigResponse struct {
	AIProvider string `json:"ai_provider"`
	APIKey     string `json:"api_key"` // Masked
	QdrantURL  string `json:"qdrant_url"`
	RedisURL   string `json:"redis_url"`
}

// HandleGetConfig returns the current configuration
func HandleGetConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	config := ConfigResponse{
		AIProvider: getEnvOrDefault("EMBEDDER_TYPE", "openai"),
		APIKey:     maskAPIKey(os.Getenv("OPENAI_API_KEY")),
		QdrantURL:  getEnvOrDefault("QDRANT_URL", "localhost:6334"),
		RedisURL:   getEnvOrDefault("REDIS_URL", "localhost:6379"),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// HandleSaveConfig updates the configuration and saves to .env file
func HandleSaveConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Update environment variables
	if req.AIProvider != "" {
		os.Setenv("EMBEDDER_TYPE", req.AIProvider)
	}
	if req.APIKey != "" {
		os.Setenv("OPENAI_API_KEY", req.APIKey)
	}
	if req.QdrantURL != "" {
		os.Setenv("QDRANT_URL", req.QdrantURL)
	}
	if req.RedisURL != "" {
		os.Setenv("REDIS_URL", req.RedisURL)
	}

	// Write to .env file
	envContent := buildEnvContent(req)
	if err := os.WriteFile(".env", []byte(envContent), 0644); err != nil {
		log.Printf("Failed to write .env file: %v", err)
		http.Error(w, fmt.Sprintf("Failed to save configuration: %v", err), http.StatusInternalServerError)
		return
	}

	// Reload .env file to ensure it's in sync
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: Failed to reload .env file: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "Configuration saved successfully",
	})
}

// buildEnvContent builds the .env file content
func buildEnvContent(req ConfigRequest) string {
	var lines []string

	if req.AIProvider != "" {
		lines = append(lines, fmt.Sprintf("EMBEDDER_TYPE=%s", req.AIProvider))
	}
	if req.APIKey != "" {
		lines = append(lines, fmt.Sprintf("OPENAI_API_KEY=%s", req.APIKey))
	}
	if req.QdrantURL != "" {
		lines = append(lines, fmt.Sprintf("QDRANT_URL=%s", req.QdrantURL))
	}
	if req.RedisURL != "" {
		lines = append(lines, fmt.Sprintf("REDIS_URL=%s", req.RedisURL))
	}

	// Preserve other existing env vars if .env exists
	if existing, err := os.ReadFile(".env"); err == nil {
		existingLines := strings.Split(string(existing), "\n")
		for _, line := range existingLines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			// Skip lines we're updating
			if strings.HasPrefix(line, "EMBEDDER_TYPE=") ||
				strings.HasPrefix(line, "OPENAI_API_KEY=") ||
				strings.HasPrefix(line, "QDRANT_URL=") ||
				strings.HasPrefix(line, "REDIS_URL=") {
				continue
			}
			lines = append(lines, line)
		}
	}

	return strings.Join(lines, "\n") + "\n"
}

// getEnvOrDefault returns the environment variable or a default value
func getEnvOrDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// maskAPIKey masks the API key for display (shows first 4 and last 4 chars)
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

