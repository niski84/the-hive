// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"encoding/json"
	"net/http"

	"github.com/the-hive/internal/database"
)

// GraphResponse represents the graph data structure
type GraphResponse struct {
	Nodes []GraphNode `json:"nodes"`
	Links []GraphLink `json:"links"`
}

// GraphNode represents a document node
type GraphNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Type  string `json:"type"` // "document"
}

// GraphLink represents a relationship edge
type GraphLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"` // "contradicts", "references"
	Label  string `json:"label"`
	Description string `json:"description,omitempty"`
}

// HandleGraph handles GET /api/v1/graph requests
func HandleGraph(w http.ResponseWriter, r *http.Request, graphStore *database.GraphStore) {
	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	// Get all edges
	edges, err := graphStore.GetEdges("")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Build node set (unique document IDs)
	nodeMap := make(map[string]bool)
	for _, edge := range edges {
		nodeMap[edge.SourceDocID] = true
		nodeMap[edge.TargetDocID] = true
	}

	// Convert to nodes array
	nodes := make([]GraphNode, 0, len(nodeMap))
	for docID := range nodeMap {
		nodes = append(nodes, GraphNode{
			ID:    docID,
			Label: docID,
			Type:  "document",
		})
	}

	// Convert edges to links
	links := make([]GraphLink, 0, len(edges))
	for _, edge := range edges {
		links = append(links, GraphLink{
			Source:      edge.SourceDocID,
			Target:      edge.TargetDocID,
			Type:        edge.RelationshipType,
			Label:       edge.RelationshipType,
			Description: edge.Description,
		})
	}

	response := GraphResponse{
		Nodes: nodes,
		Links: links,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

