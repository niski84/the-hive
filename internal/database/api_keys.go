// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package database

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// APIKey represents an API key in the database
type APIKey struct {
	Key        string     `json:"key"`
	ClientName string     `json:"client_name"`
	IsActive   bool       `json:"is_active"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	IsOnline   bool       `json:"is_online"` // Computed field: true if last_seen_at is within last 30 seconds
}

// APIKeyStore manages API keys
type APIKeyStore struct {
	db *sql.DB
}

// NewAPIKeyStore creates a new API key store
func NewAPIKeyStore(db *sql.DB) (*APIKeyStore, error) {
	store := &APIKeyStore{db: db}
	if err := store.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize API keys schema: %w", err)
	}
	return store, nil
}

// initSchema creates the api_keys table if it doesn't exist and migrates schema if needed
func (s *APIKeyStore) initSchema() error {
	// Step 1: Create base table (OLD schema - without last_seen_at)
	// This ensures existing tables are not affected
	const baseSchema = `
	CREATE TABLE IF NOT EXISTS api_keys (
		key TEXT PRIMARY KEY,
		client_name TEXT NOT NULL,
		is_active BOOLEAN NOT NULL DEFAULT TRUE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := s.db.Exec(baseSchema)
	if err != nil {
		return fmt.Errorf("failed to create base table: %w", err)
	}
	
	// Step 2: Check existing columns using PRAGMA (CRITICAL: before any index creation)
	rows, err := s.db.Query("PRAGMA table_info(api_keys)")
	if err != nil {
		return fmt.Errorf("failed to query table info: %w", err)
	}
	defer rows.Close()
	
	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue interface{}
		
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("failed to scan table info: %w", err)
		}
		columns[name] = true
	}
	
	// Step 3: Add last_seen_at column if it doesn't exist (MIGRATION)
	needsMigration := !columns["last_seen_at"]
	if needsMigration {
		log.Printf("[MIGRATION] Adding last_seen_at column to api_keys table")
		_, err = s.db.Exec("ALTER TABLE api_keys ADD COLUMN last_seen_at DATETIME")
		if err != nil {
			return fmt.Errorf("failed to add last_seen_at column: %w", err)
		}
		log.Printf("[MIGRATION] Successfully added last_seen_at column")
		// Update our column map to reflect the new column
		columns["last_seen_at"] = true
	}
	
	// Step 4: Create indexes ONLY after migration is complete
	// Verify column exists before creating index (error suppression)
	hasLastSeenAt := columns["last_seen_at"]
	
	// Create is_active index (always safe - column exists in base schema)
	_, err = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_api_keys_active ON api_keys(is_active)")
	if err != nil {
		// Suppress error - log but don't fail startup
		log.Printf("[WARNING] Failed to create is_active index (non-fatal): %v", err)
	} else {
		log.Printf("[INFO] Created/verified is_active index")
	}
	
	// Create last_seen_at index ONLY if column exists (after migration check)
	if hasLastSeenAt {
		_, err = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_api_keys_last_seen ON api_keys(last_seen_at)")
		if err != nil {
			// Suppress error - log but don't fail startup
			log.Printf("[WARNING] Failed to create last_seen_at index (non-fatal): %v", err)
		} else {
			log.Printf("[INFO] Created/verified last_seen_at index")
		}
	} else {
		log.Printf("[INFO] Skipping last_seen_at index creation (column not present)")
	}
	
	log.Printf("[INFO] Database migration complete for api_keys table")
	return nil
}

// GenerateKey generates a new API key
func (s *APIKeyStore) GenerateKey(clientName string) (string, error) {
	key := "hive_" + uuid.New().String()
	
	_, err := s.db.Exec(
		"INSERT INTO api_keys (key, client_name, is_active, created_at) VALUES (?, ?, ?, ?)",
		key,
		clientName,
		true,
		time.Now(),
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate API key: %w", err)
	}
	
	return key, nil
}

// ValidateKey checks if an API key is valid and active
func (s *APIKeyStore) ValidateKey(key string) (bool, error) {
	var isActive bool
	err := s.db.QueryRow(
		"SELECT is_active FROM api_keys WHERE key = ?",
		key,
	).Scan(&isActive)
	
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to validate API key: %w", err)
	}
	
	return isActive, nil
}

// RevokeKey revokes an API key
func (s *APIKeyStore) RevokeKey(key string) error {
	_, err := s.db.Exec(
		"UPDATE api_keys SET is_active = FALSE WHERE key = ?",
		key,
	)
	return err
}

// UpdateLastSeen updates the last_seen_at timestamp for a given API key
func (s *APIKeyStore) UpdateLastSeen(key string) error {
	_, err := s.db.Exec(
		"UPDATE api_keys SET last_seen_at = ? WHERE key = ?",
		time.Now(),
		key,
	)
	return err
}

// ListKeys returns all API keys with online status
func (s *APIKeyStore) ListKeys() ([]APIKey, error) {
	rows, err := s.db.Query(
		"SELECT key, client_name, is_active, created_at, last_seen_at FROM api_keys ORDER BY created_at DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	now := time.Now()
	onlineThreshold := 30 * time.Second // Consider online if seen within last 30 seconds

	for rows.Next() {
		var key APIKey
		var lastSeenAt sql.NullTime
		if err := rows.Scan(&key.Key, &key.ClientName, &key.IsActive, &key.CreatedAt, &lastSeenAt); err != nil {
			return nil, err
		}
		
		if lastSeenAt.Valid {
			key.LastSeenAt = &lastSeenAt.Time
			// Check if last seen within threshold
			if now.Sub(lastSeenAt.Time) <= onlineThreshold {
				key.IsOnline = true
			}
		}
		
		keys = append(keys, key)
	}

	return keys, nil
}

