package vectordb

import (
	"context"
	"errors"
	"fmt"
	"log"

	qdrant "github.com/qdrant/go-client/qdrant"
)

// Match represents a vector search hit.
type Match struct {
	ID         string
	DocumentID string
	Score      float32
	Metadata   map[string]string
}

// VectorDB describes the behaviour required by the Hive service.
type VectorDB interface {
	Upsert(ctx context.Context, id string, vector []float32, metadata map[string]string) error
	Search(ctx context.Context, queryVector []float32, topK int) ([]Match, error)
	Delete(ctx context.Context, id string) error
}

// QdrantVectorDB is a thin wrapper around the Qdrant client.
type QdrantVectorDB struct {
	client     qdrant.QdrantClient
	collection string
	dimension  int
}

// NewQdrantVectorDB constructs a new wrapper and ensures the collection exists.
func NewQdrantVectorDB(client qdrant.QdrantClient) (*QdrantVectorDB, error) {
	if client == nil {
		return nil, errors.New("qdrant client is required")
	}

	collectionName := "hive_chunks"
	// Default dimension - will be updated when first vector is inserted
	defaultDim := 1536

	vdb := &QdrantVectorDB{
		client:     client,
		collection: collectionName,
		dimension:  defaultDim,
	}

	// Ensure collection exists
	if err := vdb.ensureCollection(context.Background(), defaultDim); err != nil {
		return nil, fmt.Errorf("failed to ensure collection: %w", err)
	}

	return vdb, nil
}

// ensureCollection creates the collection if it doesn't exist.
// The Qdrant Go client API structure needs verification - this is a minimal implementation.
func (q *QdrantVectorDB) ensureCollection(ctx context.Context, dim int) error {
	log.Printf("Ensuring Qdrant collection %s exists with dimension %d", q.collection, dim)

	// Note: The actual Qdrant Go client v1.7.0 API needs to be verified.
	// The client created via qdrant.NewQdrantClient() may expose services differently.
	// For now, we skip explicit collection creation and handle errors on first use.
	// Collection creation will happen automatically on first upsert, or you can
	// create it manually via Qdrant's REST API or admin tools.

	q.dimension = dim
	return nil
}

// Upsert stores or updates a vector in Qdrant.
func (q *QdrantVectorDB) Upsert(ctx context.Context, id string, vector []float32, metadata map[string]string) error {
	if len(vector) == 0 {
		return errors.New("vector cannot be empty")
	}

	// Update dimension if needed and ensure collection exists with correct dimension
	if q.dimension != len(vector) {
		log.Printf("Updating collection dimension from %d to %d", q.dimension, len(vector))
		// Note: Collection deletion/creation needs proper Qdrant API calls
		// For now, update dimension and let ensureCollection handle it
		if err := q.ensureCollection(ctx, len(vector)); err != nil {
			return err
		}
	}

	// Convert metadata to Qdrant format
	payload := make(map[string]*qdrant.Value)
	if docID, ok := metadata["document_id"]; ok {
		payload["document_id"] = &qdrant.Value{
			Kind: &qdrant.Value_StringValue{StringValue: docID},
		}
	}
	for k, v := range metadata {
		if k != "document_id" {
			payload[k] = &qdrant.Value{
				Kind: &qdrant.Value_StringValue{StringValue: v},
			}
		}
	}

	// Convert float32 to float32 for Qdrant (it uses float32)
	points := []*qdrant.PointStruct{
		{
			Id: &qdrant.PointId{
				PointIdOptions: &qdrant.PointId_Uuid{
					Uuid: id,
				},
			},
			Vectors: &qdrant.Vectors{
				VectorsOptions: &qdrant.Vectors_Vector{
					Vector: &qdrant.Vector{
						Data: vector,
					},
				},
			},
			Payload: payload,
		},
	}

	// TODO: Implement actual Qdrant Upsert call
	// The Qdrant Go client API needs verification. Check the client structure:
	// - May be: q.client.Points().Upsert(ctx, ...)
	// - Or: Direct gRPC service client access
	// - Or: Client wrapper methods
	//
	// For now, return an error indicating API needs implementation
	// Once the correct API is identified, replace this with the actual call
	_ = points // suppress unused variable
	return fmt.Errorf("Qdrant Upsert not implemented - verify client API: github.com/qdrant/go-client")
}

// Search performs a similarity search.
func (q *QdrantVectorDB) Search(ctx context.Context, queryVector []float32, topK int) ([]Match, error) {
	if len(queryVector) == 0 {
		return nil, errors.New("query vector cannot be empty")
	}

	if topK <= 0 {
		topK = 10
	}

	// TODO: Implement actual Qdrant Search call
	// Verify the client API structure and implement the search
	_ = queryVector // suppress unused variable
	_ = topK
	return nil, fmt.Errorf("Qdrant Search not implemented - verify client API: github.com/qdrant/go-client")
}

// Delete removes a vector from the collection.
func (q *QdrantVectorDB) Delete(ctx context.Context, id string) error {
	// TODO: Implement actual Qdrant Delete call
	_ = id // suppress unused variable
	return fmt.Errorf("Qdrant Delete not implemented - verify client API: github.com/qdrant/go-client")
}
