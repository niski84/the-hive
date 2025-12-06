// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/the-hive/internal/proto"
	"github.com/the-hive/internal/vectordb"
)

// HiveService implements the gRPC Hive service.
type HiveService struct {
	proto.UnimplementedHiveServer
	db       *sql.DB
	vectorDB vectordb.VectorDB
}

// NewHiveService wires database and vector storage dependencies.
func NewHiveService(db *sql.DB, vectorDB vectordb.VectorDB) *HiveService {
	return &HiveService{
		db:       db,
		vectorDB: vectorDB,
	}
}

// Ingest persists chunk metadata and forwards the vector payload to the vector DB.
func (s *HiveService) Ingest(ctx context.Context, req *proto.Chunk) (*proto.Status, error) {
	if req == nil {
		return &proto.Status{Success: false, Message: "chunk payload missing"}, nil
	}

	const insertChunk = `
		INSERT OR REPLACE INTO chunks (id, document_id, content, chunk_index)
		VALUES (?, ?, ?, ?);
	`

	if _, err := s.db.ExecContext(ctx, insertChunk, req.Id, req.DocumentId, req.Content, 0); err != nil {
		return &proto.Status{
			Success: false,
			Message: fmt.Sprintf("failed to store chunk: %v", err),
		}, nil
	}

	if req.Vector != nil && len(req.Vector) > 0 {
		if err := s.vectorDB.Upsert(ctx, req.Id, req.Vector, req.Metadata); err != nil {
			log.Printf("vector upsert failed for chunk %s: %v", req.Id, err)
		}
	}

	return &proto.Status{
		Success: true,
		Message: "chunk ingested",
		ChunkId: req.Id,
	}, nil
}
