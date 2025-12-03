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
	LicenseKey string `json:"license_key"`
}

// ConfigResponse represents the current configuration
type ConfigResponse struct {
	AIProvider string `json:"ai_provider"`
	APIKey     string `json:"api_key"` // Masked
	QdrantURL  string `json:"qdrant_url"`
	RedisURL   string `json:"redis_url"`
	LicenseKey string `json:"license_key"` // Masked
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
		LicenseKey: maskAPIKey(os.Getenv("NORTHBOUND_LICENSE_KEY")),
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

	// Validate API key format if provided
	if req.APIKey != "" {
		// Trim whitespace
		req.APIKey = strings.TrimSpace(req.APIKey)
		
		// Determine provider (use request provider or current env)
		provider := req.AIProvider
		if provider == "" {
			provider = getEnvOrDefault("EMBEDDER_TYPE", "openai")
		}
		
		// Validate based on provider
		if provider == "openai" {
			if !strings.HasPrefix(req.APIKey, "sk-") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{
					"error": "Invalid OpenAI Key format. OpenAI API keys must start with 'sk-'",
				})
				return
			}
		} else if provider == "gemini" {
			if !strings.HasPrefix(req.APIKey, "AI") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{
					"error": "Invalid Gemini Key format. Gemini API keys must start with 'AI'",
				})
				return
			}
		}
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
	if req.LicenseKey != "" {
		os.Setenv("NORTHBOUND_LICENSE_KEY", req.LicenseKey)
	}

	// Write to .env file (truncate and write cleanly)
	envContent := buildEnvContent(req)
	// Truncate file by opening with O_TRUNC
	file, err := os.OpenFile(".env", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		log.Printf("Failed to open .env file for writing: %v", err)
		http.Error(w, fmt.Sprintf("Failed to save configuration: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()
	
	if _, err := file.WriteString(envContent); err != nil {
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

// buildEnvContent builds the .env file content by reading existing values and updating
func buildEnvContent(req ConfigRequest) string {
	// Read existing .env file using godotenv
	envMap := make(map[string]string)
	if _, err := os.Stat(".env"); err == nil {
		// File exists, read it
		if existing, err := godotenv.Read(".env"); err == nil {
			envMap = existing
		}
	}

	// Update with new values (only if provided)
	if req.AIProvider != "" {
		envMap["EMBEDDER_TYPE"] = req.AIProvider
	}
	if req.APIKey != "" {
		envMap["OPENAI_API_KEY"] = req.APIKey
	}
	if req.QdrantURL != "" {
		envMap["QDRANT_URL"] = req.QdrantURL
	}
	if req.RedisURL != "" {
		envMap["REDIS_URL"] = req.RedisURL
	}

	// Build clean .env content
	var lines []string
	for key, value := range envMap {
		// Skip empty values
		if value == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}

	// Sort for consistency (optional, but makes file readable)
	// For now, just return in map order (Go maps are random order)
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

// maskAPIKey masks the API key for display (shows first 3 and last 4 chars)
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	key = strings.TrimSpace(key) // Ensure no whitespace
	if len(key) <= 7 {
		return "****"
	}
	// Show first 3 and last 4 characters
	return key[:3] + "****" + key[len(key)-4:]
}

