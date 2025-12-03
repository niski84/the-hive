// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package database

import (
	"context"
	"database/sql"
	"fmt"
)

// GraphEdge represents a relationship between two documents
type GraphEdge struct {
	SourceDocID     string `json:"source_doc_id"`
	TargetDocID     string `json:"target_doc_id"`
	RelationshipType string `json:"relationship_type"` // "contradicts", "references", etc.
	Description     string `json:"description"`
}

// GraphStore manages graph relationships
type GraphStore struct {
	db *sql.DB
}

// NewGraphStore creates a new graph store
func NewGraphStore(db *sql.DB) (*GraphStore, error) {
	store := &GraphStore{db: db}
	if err := store.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize graph schema: %w", err)
	}
	return store, nil
}

// initSchema creates the graph_edges table if it doesn't exist
func (g *GraphStore) initSchema() error {
	const schema = `
	CREATE TABLE IF NOT EXISTS graph_edges (
		source_doc_id TEXT NOT NULL,
		target_doc_id TEXT NOT NULL,
		relationship_type TEXT NOT NULL,
		description TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (source_doc_id, target_doc_id, relationship_type)
	);
	
	CREATE INDEX IF NOT EXISTS idx_graph_edges_source ON graph_edges(source_doc_id);
	CREATE INDEX IF NOT EXISTS idx_graph_edges_target ON graph_edges(target_doc_id);
	CREATE INDEX IF NOT EXISTS idx_graph_edges_type ON graph_edges(relationship_type);
	`
	_, err := g.db.Exec(schema)
	return err
}

// AddEdge adds a new edge to the graph
func (g *GraphStore) AddEdge(ctx context.Context, sourceDocID, targetDocID, relationshipType, description string) error {
	_, err := g.db.ExecContext(ctx,
		"INSERT OR REPLACE INTO graph_edges (source_doc_id, target_doc_id, relationship_type, description) VALUES (?, ?, ?, ?)",
		sourceDocID,
		targetDocID,
		relationshipType,
		description,
	)
	return err
}

// GetEdges returns all edges, optionally filtered by relationship type
func (g *GraphStore) GetEdges(relationshipType string) ([]GraphEdge, error) {
	var rows *sql.Rows
	var err error

	if relationshipType != "" {
		rows, err = g.db.Query(
			"SELECT source_doc_id, target_doc_id, relationship_type, description FROM graph_edges WHERE relationship_type = ? ORDER BY created_at DESC",
			relationshipType,
		)
	} else {
		rows, err = g.db.Query(
			"SELECT source_doc_id, target_doc_id, relationship_type, description FROM graph_edges ORDER BY created_at DESC",
		)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []GraphEdge
	for rows.Next() {
		var edge GraphEdge
		if err := rows.Scan(&edge.SourceDocID, &edge.TargetDocID, &edge.RelationshipType, &edge.Description); err != nil {
			return nil, err
		}
		edges = append(edges, edge)
	}

	return edges, nil
}

// GetEdgesForDocument returns all edges connected to a specific document
func (g *GraphStore) GetEdgesForDocument(docID string) ([]GraphEdge, error) {
	rows, err := g.db.Query(
		"SELECT source_doc_id, target_doc_id, relationship_type, description FROM graph_edges WHERE source_doc_id = ? OR target_doc_id = ? ORDER BY created_at DESC",
		docID,
		docID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []GraphEdge
	for rows.Next() {
		var edge GraphEdge
		if err := rows.Scan(&edge.SourceDocID, &edge.TargetDocID, &edge.RelationshipType, &edge.Description); err != nil {
			return nil, err
		}
		edges = append(edges, edge)
	}

	return edges, nil
}

// GetAllDocuments returns a list of all unique document IDs in the graph
func (g *GraphStore) GetAllDocuments() ([]string, error) {
	rows, err := g.db.Query(
		"SELECT DISTINCT source_doc_id FROM graph_edges UNION SELECT DISTINCT target_doc_id FROM graph_edges",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var documents []string
	for rows.Next() {
		var docID string
		if err := rows.Scan(&docID); err != nil {
			return nil, err
		}
		documents = append(documents, docID)
	}

	return documents, nil
}

