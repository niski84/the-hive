// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"net/http"

	"github.com/the-hive/internal/database"
)

// HandleCheckDomain handles the domain validation endpoint for Caddy
// This is called by Caddy to check if a domain is allowed before issuing SSL
// GET /api/v1/infra/check-domain?domain=example.com
func HandleCheckDomain(w http.ResponseWriter, r *http.Request, domainStore *database.CustomDomainStore) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	domain := r.URL.Query().Get("domain")
	if domain == "" {
		http.Error(w, "domain parameter required", http.StatusBadRequest)
		return
	}

	// Check if domain exists and is verified
	customDomain, err := domainStore.GetDomainByHost(domain)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if customDomain == nil || !customDomain.Verified {
		// Domain not found or not verified - return 404 to tell Caddy to reject
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Domain is valid and verified - return 200 OK
	w.WriteHeader(http.StatusOK)
}

