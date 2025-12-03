// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/the-hive/internal/rules"
)

// HandleGetRules returns all rules
func HandleGetRules(w http.ResponseWriter, r *http.Request, ruleStore *rules.Store) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	allRules, err := ruleStore.GetAllRules()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get rules: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"rules": allRules,
	})
}

// HandleAddRule adds a new rule
func HandleAddRule(w http.ResponseWriter, r *http.Request, ruleStore *rules.Store) {
	log.Printf("[RULES] Received CreateRule request: Method=%s, Content-Type=%s", r.Method, r.Header.Get("Content-Type"))
	
	if r.Method != http.MethodPost {
		log.Printf("[RULES] Method not allowed: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Query  string `json:"query"`
		Active bool   `json:"active"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[RULES] Failed to decode JSON: %v", err)
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	log.Printf("[RULES] Decoded request: Query=%s, Active=%v", req.Query, req.Active)

	if req.Query == "" {
		log.Printf("[RULES] Query is empty, returning 400")
		http.Error(w, "query is required", http.StatusBadRequest)
		return
	}

	log.Printf("[RULES] DEBUG: Attempting to insert rule into DB...")
	
	// Use context with timeout to prevent indefinite hanging (5 seconds)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	// Retry logic: Try up to 3 times for busy/locked errors
	var rule *rules.Rule
	var err error
	maxRetries := 3
	retryDelay := 500 * time.Millisecond
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		rule, err = ruleStore.AddRule(ctx, req.Query, req.Active)
		
		if err == nil {
			// Success!
			break
		}
		
		// Check if error is due to database being busy/locked
		errStr := err.Error()
		isBusy := strings.Contains(strings.ToLower(errStr), "busy") || 
		          strings.Contains(strings.ToLower(errStr), "locked") ||
		          strings.Contains(strings.ToLower(errStr), "database is locked")
		
		if !isBusy || attempt == maxRetries {
			// Not a busy error, or we've exhausted retries
			break
		}
		
		// Log retry attempt
		log.Printf("[RULES] Database busy (attempt %d/%d), retrying in %v...", attempt, maxRetries, retryDelay)
		time.Sleep(retryDelay)
	}
	
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("[RULES] Database operation timed out after 5 seconds")
			http.Error(w, "Database Busy", http.StatusServiceUnavailable)
			return
		}
		log.Printf("[RULES] Failed to add rule to database after %d attempts: %v", maxRetries, err)
		http.Error(w, fmt.Sprintf("Failed to add rule: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("[RULES] DEBUG: Rule insert successful")
	log.Printf("[RULES] Rule created successfully with ID=%d, returning JSON", rule.ID)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(rule); err != nil {
		log.Printf("[RULES] Failed to encode JSON response: %v", err)
	}
	log.Printf("[RULES] Response sent successfully")
}

// HandleUpdateRule updates an existing rule
func HandleUpdateRule(w http.ResponseWriter, r *http.Request, ruleStore *rules.Store) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get ID from URL path or query
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "id parameter is required", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var req struct {
		Query  string `json:"query"`
		Active bool   `json:"active"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	// If query is empty, fetch the existing rule to preserve the query text
	var query string
	if req.Query == "" {
		// Get existing rule to preserve query
		allRules, err := ruleStore.GetAllRules()
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get existing rule: %v", err), http.StatusInternalServerError)
			return
		}
		// Find the rule with matching ID
		found := false
		for _, rule := range allRules {
			if rule.ID == id {
				query = rule.Query
				found = true
				break
			}
		}
		if !found {
			http.Error(w, "Rule not found", http.StatusNotFound)
			return
		}
	} else {
		query = req.Query
	}
	
	if err := ruleStore.UpdateRule(ctx, id, query, req.Active); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update rule: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// HandleDeleteRule deletes a rule
func HandleDeleteRule(w http.ResponseWriter, r *http.Request, ruleStore *rules.Store) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "id parameter is required", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	
	if err := ruleStore.DeleteRule(ctx, id); err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete rule: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

