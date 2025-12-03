// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"log"
	"net/http"
	"strings"

	"github.com/the-hive/internal/database"
)

// AuthMiddleware creates an authentication middleware that validates API keys
func AuthMiddleware(apiKeyStore *database.APIKeyStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract API key from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"missing authorization header"}`))
				return
			}

			// Support both "Bearer <key>" and just "<key>" formats
			key := strings.TrimSpace(authHeader)
			if strings.HasPrefix(key, "Bearer ") {
				key = strings.TrimPrefix(key, "Bearer ")
			}

			// Validate the key
			isValid, err := apiKeyStore.ValidateKey(key)
			if err != nil {
				log.Printf("Error validating API key: %v", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":"internal server error"}`))
				return
			}

			if !isValid {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"invalid or inactive API key"}`))
				return
			}

			// Update last_seen_at timestamp for this key
			if err := apiKeyStore.UpdateLastSeen(key); err != nil {
				log.Printf("Warning: Failed to update last_seen_at for key: %v", err)
				// Don't fail the request, just log the warning
			}

			// Key is valid, proceed to next handler
			next.ServeHTTP(w, r)
		})
	}
}

