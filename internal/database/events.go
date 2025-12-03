// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package database

import (
	"database/sql"
	"fmt"
	"time"
)

// Event represents a processing event
type Event struct {
	ID           int64     `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	EventType    string    `json:"event_type"` // ingest, update, alert
	DocumentName string    `json:"document_name"`
	Details      string    `json:"details"`
}

// EventLogger handles event logging to SQLite
type EventLogger struct {
	db *sql.DB
}

// NewEventLogger creates a new event logger
func NewEventLogger(db *sql.DB) (*EventLogger, error) {
	logger := &EventLogger{db: db}
	if err := logger.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize events schema: %w", err)
	}
	return logger, nil
}

// initSchema creates the events table if it doesn't exist
func (e *EventLogger) initSchema() error {
	const schema = `
	CREATE TABLE IF NOT EXISTS events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		event_type TEXT NOT NULL,
		document_name TEXT NOT NULL,
		details TEXT
	);
	
	CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_events_document_name ON events(document_name);
	`
	_, err := e.db.Exec(schema)
	return err
}

// LogEvent logs a new event
func (e *EventLogger) LogEvent(eventType, documentName, details string) error {
	_, err := e.db.Exec(
		"INSERT INTO events (timestamp, event_type, document_name, details) VALUES (?, ?, ?, ?)",
		time.Now(),
		eventType,
		documentName,
		details,
	)
	return err
}

// GetRecentEvents returns the last N events, sorted by timestamp descending
func (e *EventLogger) GetRecentEvents(limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := e.db.Query(
		"SELECT id, timestamp, event_type, document_name, details FROM events ORDER BY timestamp DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.Timestamp, &event.EventType, &event.DocumentName, &event.Details); err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	return events, nil
}

// GetEventsByDocument returns all events for a specific document
func (e *EventLogger) GetEventsByDocument(documentName string) ([]Event, error) {
	rows, err := e.db.Query(
		"SELECT id, timestamp, event_type, document_name, details FROM events WHERE document_name = ? ORDER BY timestamp DESC",
		documentName,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.Timestamp, &event.EventType, &event.DocumentName, &event.Details); err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	return events, nil
}
