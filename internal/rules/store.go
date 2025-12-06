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
	if err != nil {
		return err
	}
	
	// Migration: Add organization_id column if it doesn't exist
	rows, err := s.db.Query("PRAGMA table_info(rules)")
	if err == nil {
		defer rows.Close()
		hasOrgID := false
		for rows.Next() {
			var cid int
			var name, dataType string
			var notNull, pk int
			var defaultValue interface{}
			if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err == nil {
				if name == "organization_id" {
					hasOrgID = true
					break
				}
			}
		}
		if !hasOrgID {
			_, err = s.db.Exec("ALTER TABLE rules ADD COLUMN organization_id TEXT")
			if err != nil {
				return fmt.Errorf("failed to add organization_id to rules: %w", err)
			}
			_, err = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_rules_organization_id ON rules(organization_id)")
			if err != nil {
				return fmt.Errorf("failed to create organization_id index on rules: %w", err)
			}
		} else {
			// Column exists, ensure index exists
			_, err = s.db.Exec("CREATE INDEX IF NOT EXISTS idx_rules_organization_id ON rules(organization_id)")
			if err != nil {
				return fmt.Errorf("failed to create organization_id index on rules: %w", err)
			}
		}
	}
	
	return nil
}

// refreshCache refreshes the in-memory cache of active rules
// If organizationID is provided, only caches rules for that organization
func (s *Store) refreshCache(organizationID ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Use context with timeout to prevent indefinite hanging
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var query string
	var args []interface{}
	if len(organizationID) > 0 && organizationID[0] != "" {
		query = "SELECT id, query, active FROM rules WHERE active = 1 AND organization_id = ?"
		args = []interface{}{organizationID[0]}
	} else {
		query = "SELECT id, query, active FROM rules WHERE active = 1"
		args = []interface{}{}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
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
// If organizationID is provided, only returns rules for that organization
func (s *Store) GetActiveRules(organizationID ...string) ([]Rule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// If organizationID is provided, query DB directly (cache doesn't store org_id)
	if len(organizationID) > 0 && organizationID[0] != "" {
		s.mu.RUnlock()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		rows, err := s.db.QueryContext(ctx, "SELECT id, query, active FROM rules WHERE active = 1 AND organization_id = ?", organizationID[0])
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

	// Return a copy to avoid external modification
	rules := make([]Rule, len(s.activeRules))
	copy(rules, s.activeRules)
	return rules, nil
}

// GetAllRules returns all rules
// If organizationID is provided, only returns rules for that organization
func (s *Store) GetAllRules(organizationID ...string) ([]Rule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var query string
	var args []interface{}
	if len(organizationID) > 0 && organizationID[0] != "" {
		query = "SELECT id, query, active FROM rules WHERE organization_id = ? ORDER BY id DESC"
		args = []interface{}{organizationID[0]}
	} else {
		query = "SELECT id, query, active FROM rules ORDER BY id DESC"
		args = []interface{}{}
	}

	rows, err := s.db.Query(query, args...)
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
// organizationID is optional - if provided, the rule will be scoped to that organization
func (s *Store) AddRule(ctx context.Context, query string, active bool, organizationID ...string) (*Rule, error) {
	// Use context with timeout to prevent indefinite hanging
	insertCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	orgID := ""
	if len(organizationID) > 0 {
		orgID = organizationID[0]
	}

	// Perform database insert WITHOUT holding the lock
	result, err := s.db.ExecContext(insertCtx, "INSERT INTO rules (query, active, organization_id) VALUES (?, ?, ?)", query, active, orgID)
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
		if err := s.refreshCache(orgID); err != nil {
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
	// Note: We don't know the org_id here, so refresh all (or could query first)
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
	// Note: We don't know the org_id here, so refresh all (or could query first)
	return s.refreshCache()
}

