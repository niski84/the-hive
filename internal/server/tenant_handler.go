// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"encoding/json"
	"net/http"

	"github.com/the-hive/internal/database"
)

// HandleListOrganizations handles GET /api/v1/organizations
func HandleListOrganizations(w http.ResponseWriter, r *http.Request, orgStore *database.OrganizationStore, userStore *database.UserStore, usageStore *database.UsageStore) {
	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	// Get all organizations
	orgs, err := orgStore.GetAllOrganizations()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orgs)
}

// HandleCreateOrganization handles POST /api/v1/organizations
func HandleCreateOrganization(w http.ResponseWriter, r *http.Request, orgStore *database.OrganizationStore, userStore *database.UserStore) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}

	// Create organization with default subscription status
	org, err := orgStore.CreateOrganization(req.Name, "active")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(org)
}

// HandleUpdateOrganization handles PUT /api/v1/organizations/{id}
func HandleUpdateOrganization(w http.ResponseWriter, r *http.Request, orgStore *database.OrganizationStore) {
	if r.Method != http.MethodPut && r.Method != http.MethodPatch {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	var req struct {
		Name               string `json:"name"`
		SubscriptionStatus string `json:"subscription_status,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}

	// Extract ID from path
	id := r.URL.Path[len("/api/v1/admin/organizations/"):]

	// Update organization (name and/or subscription status)
	var subscriptionStatus string
	if req.SubscriptionStatus != "" {
		subscriptionStatus = req.SubscriptionStatus
	}
	
	err := orgStore.UpdateOrganization(id, req.Name, subscriptionStatus)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Get updated organization
	org, err := orgStore.GetOrganizationByID(id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(org)
}

