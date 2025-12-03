// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package middleware

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/the-hive/internal/database"
)

// LicenseMiddleware creates a middleware that enforces commercial licensing
func LicenseMiddleware(metadataStore *database.SystemMetadataStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get install date and calculate days active
			daysActive, err := metadataStore.GetDaysActive()
			if err != nil {
				log.Printf("[ERROR] Failed to get days active: %v", err)
				// On error, allow the request (fail open for now)
				next.ServeHTTP(w, r)
				return
			}

			// Trial Check: If days_active < 365, ALLOW the request (no Authorization header required)
			if daysActive < 365 {
				next.ServeHTTP(w, r)
				return
			}

			// License Check: If days_active >= 365, require valid license
			licenseKey := os.Getenv("NORTHBOUND_LICENSE_KEY")
			if licenseKey == "" {
				// Also check database
				dbLicenseKey, err := metadataStore.Get("license_key")
				if err == nil && dbLicenseKey != "" {
					licenseKey = dbLicenseKey
				}
			}

			// Validate license key (basic check - can be enhanced)
			if licenseKey == "" || !isValidLicenseKey(licenseKey) {
				log.Printf("[WARN] License expired or invalid. Days active: %d", daysActive)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusPaymentRequired) // 402 Payment Required
				json.NewEncoder(w).Encode(map[string]string{
					"error":   "LICENSE_EXPIRED",
					"message": "Evaluation ended. Contact Northbound System.",
				})
				return
			}

			// License is valid, proceed
			next.ServeHTTP(w, r)
		})
	}
}

// isValidLicenseKey performs basic validation of the license key
// This can be enhanced with cryptographic verification, online validation, etc.
func isValidLicenseKey(key string) bool {
	// Basic validation: non-empty and reasonable length
	if len(key) < 10 {
		return false
	}
	// TODO: Add more sophisticated validation (cryptographic signature, online check, etc.)
	return true
}

