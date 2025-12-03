// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"github.com/the-hive/internal/vectordb"
)

// StatsResponse represents the system statistics
type StatsResponse struct {
	VectorsInMemory int    `json:"vectors_in_memory"`
	DatabaseStatus  string `json:"database_status"`
	CollectionName  string `json:"collection_name"`
}

// HandleStats returns system statistics
func HandleStats(w http.ResponseWriter, r *http.Request, vectorDB vectordb.VectorDB, db *sql.DB) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := StatsResponse{
		CollectionName: "the_hive",
		DatabaseStatus: "unknown",
	}

	// Get vector count from Qdrant
	if vectorDB != nil {
		count, err := getVectorCount(r.Context(), vectorDB)
		if err != nil {
			log.Printf("[ERROR] Failed to fetch Qdrant stats: %v", err)
			stats.VectorsInMemory = -1 // Error indicator
		} else {
			stats.VectorsInMemory = count
			log.Printf("[DEBUG] Qdrant stats: %d vectors in collection 'the_hive'", count)
		}
	} else {
		log.Printf("[WARN] Stats: vectorDB is nil, using mock or not initialized")
		stats.VectorsInMemory = 0
	}

	// Check database status
	if db != nil {
		if err := db.PingContext(r.Context()); err != nil {
			stats.DatabaseStatus = "disconnected"
		} else {
			// Try a simple query to verify it's working
			var count int
			if err := db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM chunks").Scan(&count); err != nil {
				stats.DatabaseStatus = "error"
			} else {
				stats.DatabaseStatus = "connected"
			}
		}
	} else {
		stats.DatabaseStatus = "not_initialized"
	}

	// Get trial days from metadata store (if available)
	// For now, fallback to old method if metadataStore is not available
	trialDays := GetTrialDays()

	response := map[string]interface{}{
		"vectors_in_memory": stats.VectorsInMemory,
		"database_status":   stats.DatabaseStatus,
		"trial_days":        trialDays,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// getVectorCount gets the point count from the VectorDB
func getVectorCount(ctx context.Context, vectorDB vectordb.VectorDB) (int, error) {
	if vectorDB == nil {
		return 0, nil
	}
	return vectorDB.GetPointCount(ctx)
}
