// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package database

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// AuditAction represents the type of action being audited
type AuditAction string

const (
	AuditActionSearch AuditAction = "SEARCH"
	AuditActionIngest AuditAction = "INGEST"
)

// AuditLog represents an audit log entry
type AuditLog struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	ClientIP  string    `json:"client_ip"`
	Action    string    `json:"action"` // SEARCH or INGEST
	Details   string    `json:"details"`
}

// AuditLogStore manages audit logs
type AuditLogStore struct {
	db *sql.DB
}

// NewAuditLogStore creates a new audit log store
func NewAuditLogStore(db *sql.DB) (*AuditLogStore, error) {
	store := &AuditLogStore{db: db}
	if err := store.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize audit logs schema: %w", err)
	}
	return store, nil
}

// initSchema creates the audit_logs table if it doesn't exist
func (s *AuditLogStore) initSchema() error {
	// Step 1: Create base table (may not have organization_id)
	const baseSchema = `
	CREATE TABLE IF NOT EXISTS audit_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		client_ip TEXT NOT NULL,
		action TEXT NOT NULL,
		details TEXT
	);
	
	CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs(timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
	CREATE INDEX IF NOT EXISTS idx_audit_logs_client_ip ON audit_logs(client_ip);
	`
	_, err := s.db.Exec(baseSchema)
	if err != nil {
		return fmt.Errorf("failed to create base schema: %w", err)
	}
	
	// Step 2: Check if organization_id column exists and add if missing
	rows, err := s.db.Query("PRAGMA table_info(audit_logs)")
	if err != nil {
		return fmt.Errorf("failed to query table info: %w", err)
	}
	defer rows.Close()
	
	hasOrgID := false
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue interface{}
		
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("failed to scan table info: %w", err)
		}
		if name == "organization_id" {
			hasOrgID = true
			break
		}
	}
	
	// Step 3: Add organization_id column if missing
	if !hasOrgID {
		log.Printf("[MIGRATION] Adding organization_id column to audit_logs table")
		_, err = s.db.Exec("ALTER TABLE audit_logs ADD COLUMN organization_id TEXT")
		if err != nil {
			return fmt.Errorf("failed to add organization_id column: %w", err)
		}
		log.Printf("[MIGRATION] Successfully added organization_id column")
		
		// Create index for organization_id
		_, err = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_audit_logs_organization_id ON audit_logs(organization_id)")
		if err != nil {
			// Log but don't fail - index creation is optional
			log.Printf("Warning: Failed to create organization_id index: %v", err)
		}
	}
	
	return nil
}

// LogAction logs a new audit entry
// organizationID is optional - if provided, it will be stored for multi-tenancy filtering
func (s *AuditLogStore) LogAction(clientIP string, action AuditAction, details string, organizationID string) error {
	_, err := s.db.Exec(
		"INSERT INTO audit_logs (timestamp, client_ip, action, details, organization_id) VALUES (?, ?, ?, ?, ?)",
		time.Now(),
		clientIP,
		string(action),
		details,
		organizationID,
	)
	return err
}

// GetRecentLogs returns the last N audit logs, sorted by timestamp descending
// If actionFilter is provided, filters by action type
// If organizationID is provided, filters by organization (CRITICAL for multi-tenancy)
func (s *AuditLogStore) GetRecentLogs(limit int, actionFilter string, organizationID string) ([]AuditLog, error) {
	var query string
	var args []interface{}

	// Build query with organization filter (CRITICAL for multi-tenancy)
	if organizationID != "" {
		if actionFilter != "" {
			query = "SELECT id, timestamp, client_ip, action, details FROM audit_logs WHERE action = ? AND organization_id = ? ORDER BY timestamp DESC LIMIT ?"
			args = []interface{}{actionFilter, organizationID, limit}
		} else {
			query = "SELECT id, timestamp, client_ip, action, details FROM audit_logs WHERE organization_id = ? ORDER BY timestamp DESC LIMIT ?"
			args = []interface{}{organizationID, limit}
		}
	} else {
		// Backward compatibility: if no organizationID, return all (admin view)
		if actionFilter != "" {
			query = "SELECT id, timestamp, client_ip, action, details FROM audit_logs WHERE action = ? ORDER BY timestamp DESC LIMIT ?"
			args = []interface{}{actionFilter, limit}
		} else {
			query = "SELECT id, timestamp, client_ip, action, details FROM audit_logs ORDER BY timestamp DESC LIMIT ?"
			args = []interface{}{limit}
		}
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var log AuditLog
		if err := rows.Scan(&log.ID, &log.Timestamp, &log.ClientIP, &log.Action, &log.Details); err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}

	return logs, nil
}
