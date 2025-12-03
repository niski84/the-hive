// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package database

import (
	"database/sql"
	"fmt"
	"time"
)

// SystemMetadataStore manages system metadata (install_date, license_key, etc.)
type SystemMetadataStore struct {
	db *sql.DB
}

// NewSystemMetadataStore creates a new system metadata store
func NewSystemMetadataStore(db *sql.DB) (*SystemMetadataStore, error) {
	store := &SystemMetadataStore{db: db}
	if err := store.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize system_metadata schema: %w", err)
	}
	return store, nil
}

// initSchema creates the system_metadata table if it doesn't exist
func (s *SystemMetadataStore) initSchema() error {
	const schema = `
	CREATE TABLE IF NOT EXISTS system_metadata (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	
	CREATE INDEX IF NOT EXISTS idx_system_metadata_key ON system_metadata(key);
	`
	_, err := s.db.Exec(schema)
	return err
}

// Get retrieves a metadata value by key
func (s *SystemMetadataStore) Get(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM system_metadata WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get metadata: %w", err)
	}
	return value, nil
}

// Set sets a metadata value by key
func (s *SystemMetadataStore) Set(key, value string) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO system_metadata (key, value) VALUES (?, ?)",
		key, value,
	)
	return err
}

// EnsureInstallDate ensures install_date exists in the database
// If it doesn't exist, sets it to the current date
func (s *SystemMetadataStore) EnsureInstallDate() error {
	existing, err := s.Get("install_date")
	if err != nil {
		return err
	}
	
	if existing == "" {
		// Set install date to current date (YYYY-MM-DD format)
		installDate := time.Now().Format("2006-01-02")
		if err := s.Set("install_date", installDate); err != nil {
			return fmt.Errorf("failed to set install_date: %w", err)
		}
	}
	
	return nil
}

// GetInstallDate returns the install date from the database
func (s *SystemMetadataStore) GetInstallDate() (time.Time, error) {
	dateStr, err := s.Get("install_date")
	if err != nil {
		return time.Time{}, err
	}
	if dateStr == "" {
		return time.Time{}, fmt.Errorf("install_date not set")
	}
	
	installDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse install_date: %w", err)
	}
	
	return installDate, nil
}

// GetDaysActive calculates the number of days since installation
func (s *SystemMetadataStore) GetDaysActive() (int, error) {
	installDate, err := s.GetInstallDate()
	if err != nil {
		return 0, err
	}
	
	days := int(time.Since(installDate).Hours() / 24)
	return days, nil
}

