// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

// LicensingMiddleware creates a middleware that enforces freeware licensing
// Blocks requests with 402 Payment Required if trial period has expired
func LicensingMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if license key is present (licensed version)
			licenseKey := os.Getenv("LICENSE_KEY")
			if licenseKey != "" && licenseKey != "" {
				// Licensed version - allow all requests
				next.ServeHTTP(w, r)
				return
			}

			// Check install date
			installDateStr := os.Getenv("INSTALL_DATE")
			if installDateStr == "" {
				// No install date set - allow (will be set on first run)
				next.ServeHTTP(w, r)
				return
			}

			// Parse install date
			installDate, err := time.Parse("2006-01-02", installDateStr)
			if err != nil {
				log.Printf("Warning: Invalid INSTALL_DATE format: %s", installDateStr)
				// Allow request if date parsing fails
				next.ServeHTTP(w, r)
				return
			}

			// Check if trial period has expired (365 days)
			now := time.Now()
			daysSinceInstall := int(now.Sub(installDate).Hours() / 24)

			if daysSinceInstall > 365 {
				// Trial expired - block request
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusPaymentRequired)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error":   "trial period expired",
					"message": "This evaluation period has expired. Please contact support for a license key.",
					"days":    daysSinceInstall,
				})
				return
			}

			// Trial still valid - allow request
			next.ServeHTTP(w, r)
		})
	}
}

// GetTrialDays returns the number of days since installation
func GetTrialDays() int {
	installDateStr := os.Getenv("INSTALL_DATE")
	if installDateStr == "" {
		return 0
	}

	installDate, err := time.Parse("2006-01-02", installDateStr)
	if err != nil {
		return 0
	}

	now := time.Now()
	daysSinceInstall := int(now.Sub(installDate).Hours() / 24)
	return daysSinceInstall
}

