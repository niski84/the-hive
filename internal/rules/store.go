// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package rules

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// Rule represents a semantic rule
type Rule struct {
	ID     int64  `json:"id"`
	Query  string `json:"query"`
	Active bool   `json:"active"`
}

// Store manages rules storage
type Store struct {
	db  *sql.DB
	mu  sync.RWMutex
	// In-memory cache for active rules
	activeRules []Rule
}

// NewStore creates a new rules store
func NewStore(db *sql.DB) (*Store, error) {
	store := &Store{
		db: db,
	}

	// Initialize schema
	if err := store.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize rules schema: %w", err)
	}

	// Load active rules into cache
	if err := store.refreshCache(); err != nil {
		return nil, fmt.Errorf("failed to load rules: %w", err)
	}

	return store, nil
}

// initSchema creates the rules table if it doesn't exist
func (s *Store) initSchema() error {
	const schema = `
	CREATE TABLE IF NOT EXISTS rules (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		query TEXT NOT NULL,
		active BOOLEAN NOT NULL DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := s.db.Exec(schema)
	return err
}

// refreshCache refreshes the in-memory cache of active rules
func (s *Store) refreshCache() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Use context with timeout to prevent indefinite hanging
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, "SELECT id, query, active FROM rules WHERE active = 1")
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("database query timed out: %w", err)
		}
		return err
	}
	defer rows.Close()

	var rules []Rule
	for rows.Next() {
		var rule Rule
		if err := rows.Scan(&rule.ID, &rule.Query, &rule.Active); err != nil {
			return err
		}
		rules = append(rules, rule)
	}

	s.activeRules = rules
	return nil
}

// GetActiveRules returns all active rules (from cache)
func (s *Store) GetActiveRules() ([]Rule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to avoid external modification
	rules := make([]Rule, len(s.activeRules))
	copy(rules, s.activeRules)
	return rules, nil
}

// GetAllRules returns all rules
func (s *Store) GetAllRules() ([]Rule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query("SELECT id, query, active FROM rules ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []Rule
	for rows.Next() {
		var rule Rule
		if err := rows.Scan(&rule.ID, &rule.Query, &rule.Active); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

// AddRule adds a new rule
func (s *Store) AddRule(ctx context.Context, query string, active bool) (*Rule, error) {
	// Use context with timeout to prevent indefinite hanging
	insertCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Perform database insert WITHOUT holding the lock
	result, err := s.db.ExecContext(insertCtx, "INSERT INTO rules (query, active) VALUES (?, ?)", query, active)
	if err != nil {
		if insertCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("database operation timed out: %w", err)
		}
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	rule := &Rule{
		ID:     id,
		Query:  query,
		Active: active,
	}

	// Refresh cache if rule is active (this will acquire its own lock)
	if active {
		if err := s.refreshCache(); err != nil {
			return nil, err
		}
	}

	return rule, nil
}

// UpdateRule updates an existing rule
func (s *Store) UpdateRule(ctx context.Context, id int64, query string, active bool) error {
	// Perform database update WITHOUT holding the lock
	_, err := s.db.ExecContext(ctx, "UPDATE rules SET query = ?, active = ? WHERE id = ?", query, active, id)
	if err != nil {
		return err
	}

	// Refresh cache (this will acquire its own lock)
	return s.refreshCache()
}

// DeleteRule deletes a rule
func (s *Store) DeleteRule(ctx context.Context, id int64) error {
	// Perform database delete WITHOUT holding the lock
	_, err := s.db.ExecContext(ctx, "DELETE FROM rules WHERE id = ?", id)
	if err != nil {
		return err
	}

	// Refresh cache (this will acquire its own lock)
	return s.refreshCache()
}

