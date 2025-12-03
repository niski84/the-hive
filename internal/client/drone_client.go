package client

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/the-hive/internal/proto"
)

// DroneClient wraps the generated gRPC client to expose higher-level helpers.
type DroneClient struct {
	client proto.HiveClient
}

// NewDroneClient creates a new DroneClient instance.
func NewDroneClient(client proto.HiveClient) *DroneClient {
	return &DroneClient{client: client}
}

// IngestChunk sends a chunk to the Hive server.
// metadata should include: file_hash, ingest_type (new/update), filename, path, filetype
func (c *DroneClient) IngestChunk(ctx context.Context, documentID, content string, chunkIndex int, metadata map[string]string) error {
	filename := metadata["filename"]
	if filename == "" {
		filename = documentID
	}

	log.Printf("[INFO] Uploading file to server: %s (chunk %d/%s)...", filename, chunkIndex, metadata["ingest_type"])

	// Create a deterministic UUID based on the file path and chunk index
	// This ensures if we re-ingest the same file, we update the existing vectors (Idempotency)
	filePath := metadata["file_path"]
	if filePath == "" {
		filePath = documentID
	}
	seed := fmt.Sprintf("%s-%d", filePath, chunkIndex)
	pointID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(seed)).String()

	chunk := &proto.Chunk{
		Id:         pointID, // Pure UUID string - no concatenation
		DocumentId: documentID,
		Content:    content,
		Vector:     nil, // embeddings computed server-side later
		Metadata:   metadata,
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	status, err := c.client.Ingest(ctx, chunk)
	if err != nil {
		return fmt.Errorf("failed to ingest chunk: %w", err)
	}
	if !status.Success {
		return fmt.Errorf("ingestion failed: %s", status.Message)
	}
	return nil
}

// Query performs a semantic search against the Hive.
func (c *DroneClient) Query(ctx context.Context, query string, topK int32) (*proto.Result, error) {
	request := &proto.Search{
		Query:       query,
		TopK:        topK,
		QueryVector: nil,
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	result, err := c.client.Query(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to query hive: %w", err)
	}
	return result, nil
}
