// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package database

import (
	"database/sql"
	"fmt"
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
	const schema = `
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
	_, err := s.db.Exec(schema)
	return err
}

// LogAction logs a new audit entry
func (s *AuditLogStore) LogAction(clientIP string, action AuditAction, details string) error {
	_, err := s.db.Exec(
		"INSERT INTO audit_logs (timestamp, client_ip, action, details) VALUES (?, ?, ?, ?)",
		time.Now(),
		clientIP,
		string(action),
		details,
	)
	return err
}

// GetRecentLogs returns the last N audit logs, sorted by timestamp descending
func (s *AuditLogStore) GetRecentLogs(limit int) ([]AuditLog, error) {
	rows, err := s.db.Query(
		"SELECT id, timestamp, client_ip, action, details FROM audit_logs ORDER BY timestamp DESC LIMIT ?",
		limit,
	)
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
