// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// ClientDB manages the local SQLite database for client state
type ClientDB struct {
	db *sql.DB
}

// TrackedFile represents a file being tracked in the database
type TrackedFile struct {
	FilePath    string
	FileHash     string
	LastProcessed sql.NullTime
	ServerStatus string
}

// NewClientDB creates and initializes a new client database
func NewClientDB(configDir string) (*ClientDB, error) {
	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	dbPath := filepath.Join(configDir, "client_state.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	clientDB := &ClientDB{db: db}

	if err := clientDB.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return clientDB, nil
}

// Close closes the database connection
func (c *ClientDB) Close() error {
	return c.db.Close()
}

// initSchema creates the necessary tables
func (c *ClientDB) initSchema() error {
	const schema = `
	CREATE TABLE IF NOT EXISTS tracked_files (
		file_path TEXT PRIMARY KEY,
		file_hash TEXT NOT NULL,
		last_processed DATETIME DEFAULT CURRENT_TIMESTAMP,
		server_status TEXT DEFAULT 'pending'
	);

	CREATE INDEX IF NOT EXISTS idx_tracked_files_hash ON tracked_files(file_hash);
	CREATE INDEX IF NOT EXISTS idx_tracked_files_status ON tracked_files(server_status);
	`

	_, err := c.db.Exec(schema)
	return err
}

// GetTrackedFile retrieves a tracked file by path
func (c *ClientDB) GetTrackedFile(filePath string) (*TrackedFile, error) {
	var tf TrackedFile
	var lastProcessed sql.NullTime

	err := c.db.QueryRow(
		"SELECT file_path, file_hash, last_processed, server_status FROM tracked_files WHERE file_path = ?",
		filePath,
	).Scan(&tf.FilePath, &tf.FileHash, &lastProcessed, &tf.ServerStatus)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query tracked file: %w", err)
	}

	tf.LastProcessed = lastProcessed
	return &tf, nil
}

// UpsertTrackedFile inserts or updates a tracked file
func (c *ClientDB) UpsertTrackedFile(filePath, fileHash, serverStatus string) error {
	const query = `
		INSERT INTO tracked_files (file_path, file_hash, server_status, last_processed)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(file_path) DO UPDATE SET
			file_hash = excluded.file_hash,
			server_status = excluded.server_status,
			last_processed = CURRENT_TIMESTAMP
	`

	_, err := c.db.Exec(query, filePath, fileHash, serverStatus)
	if err != nil {
		return fmt.Errorf("failed to upsert tracked file: %w", err)
	}

	return nil
}

// UpdateServerStatus updates only the server status for a file
func (c *ClientDB) UpdateServerStatus(filePath, serverStatus string) error {
	_, err := c.db.Exec(
		"UPDATE tracked_files SET server_status = ? WHERE file_path = ?",
		serverStatus, filePath,
	)
	if err != nil {
		return fmt.Errorf("failed to update server status: %w", err)
	}
	return nil
}

// DeleteTrackedFile removes a file from tracking
func (c *ClientDB) DeleteTrackedFile(filePath string) error {
	_, err := c.db.Exec("DELETE FROM tracked_files WHERE file_path = ?", filePath)
	if err != nil {
		return fmt.Errorf("failed to delete tracked file: %w", err)
	}
	return nil
}

