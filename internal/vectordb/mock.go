// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package vectordb

import (
	"context"
)

// MockVectorDB is a no-op implementation for UI-only mode
type MockVectorDB struct{}

// NewMockVectorDB creates a mock vector database that does nothing
func NewMockVectorDB() VectorDB {
	return &MockVectorDB{}
}

// Upsert is a no-op for mock
func (m *MockVectorDB) Upsert(ctx context.Context, id string, vector []float32, metadata map[string]string) error {
	return nil
}

// Search returns empty results for mock
func (m *MockVectorDB) Search(ctx context.Context, queryVector []float32, topK int) ([]Match, error) {
	return []Match{}, nil
}

// Delete is a no-op for mock
func (m *MockVectorDB) Delete(ctx context.Context, id string) error {
	return nil
}

// GetPointCount returns 0 for mock
func (m *MockVectorDB) GetPointCount(ctx context.Context) (int, error) {
	return 0, nil
}
