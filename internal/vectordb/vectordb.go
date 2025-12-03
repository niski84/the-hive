// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package vectordb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	qdrant "github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
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
	GetPointCount(ctx context.Context) (int, error)
	UpdatePayload(ctx context.Context, id string, tags []string) error
}

// QdrantVectorDB is a thin wrapper around the Qdrant service clients.
type QdrantVectorDB struct {
	collectionsSvc qdrant.CollectionsClient
	pointsSvc      qdrant.PointsClient
	collection     string
	dimension      int
}

// NewQdrantVectorDB constructs a new wrapper and ensures the collection exists.
// It accepts the gRPC connection to create service clients directly.
func NewQdrantVectorDB(conn *grpc.ClientConn) (*QdrantVectorDB, error) {
	if conn == nil {
		return nil, errors.New("gRPC connection is required")
	}

	collectionName := "the_hive"
	// Default dimension - will be updated when first vector is inserted
	defaultDim := 1536

	// Create service clients from the gRPC connection
	collectionsSvc := qdrant.NewCollectionsClient(conn)
	pointsSvc := qdrant.NewPointsClient(conn)

	vdb := &QdrantVectorDB{
		collectionsSvc: collectionsSvc,
		pointsSvc:      pointsSvc,
		collection:     collectionName,
		dimension:      defaultDim,
	}

	// Ensure collection exists
	if err := vdb.ensureCollection(context.Background(), defaultDim); err != nil {
		return nil, fmt.Errorf("failed to ensure collection: %w", err)
	}

	return vdb, nil
}

// ensureCollection creates the collection if it doesn't exist.
func (q *QdrantVectorDB) ensureCollection(ctx context.Context, dim int) error {
	log.Printf("Ensuring Qdrant collection %s exists with dimension %d", q.collection, dim)

	// Check if collection exists
	collections, err := q.collectionsSvc.List(ctx, &qdrant.ListCollectionsRequest{})
	if err != nil {
		return fmt.Errorf("failed to list collections: %w", err)
	}

	// Check if our collection already exists
	exists := false
	for _, coll := range collections.Collections {
		if coll.Name == q.collection {
			exists = true
			break
		}
	}

	if !exists {
		// Create the collection
		_, err = q.collectionsSvc.Create(ctx, &qdrant.CreateCollection{
			CollectionName: q.collection,
			VectorsConfig: &qdrant.VectorsConfig{
				Config: &qdrant.VectorsConfig_Params{
					Params: &qdrant.VectorParams{
						Size:     uint64(dim),
						Distance: qdrant.Distance_Cosine,
					},
				},
			},
		})
		if err != nil {
			return fmt.Errorf("failed to create collection: %w", err)
		}
		log.Printf("Created Qdrant collection %s with dimension %d", q.collection, dim)
	}

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

	// Convert UUID string to Qdrant PointId
	// Qdrant requires a valid UUID string format
	// Validate that id is a valid UUID format before passing to Qdrant
	pointID := &qdrant.PointId{
		PointIdOptions: &qdrant.PointId_Uuid{
			Uuid: id, // id should already be a valid UUID string from deterministic generation
		},
	}

	// Create point struct
	point := &qdrant.PointStruct{
		Id: pointID,
		Vectors: &qdrant.Vectors{
			VectorsOptions: &qdrant.Vectors_Vector{
				Vector: &qdrant.Vector{
					Data: vector,
				},
			},
		},
		Payload: payload,
	}

	// Upsert the point
	_, err := q.pointsSvc.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: q.collection,
		Points:         []*qdrant.PointStruct{point},
	})
	if err != nil {
		return fmt.Errorf("failed to upsert point: %w", err)
	}

	// Log success for watchdog monitoring
	log.Printf("vector upsert success for chunk %s", id)

	return nil
}

// Search performs a similarity search.
func (q *QdrantVectorDB) Search(ctx context.Context, queryVector []float32, topK int) ([]Match, error) {
	if len(queryVector) == 0 {
		return nil, errors.New("query vector cannot be empty")
	}

	if topK <= 0 {
		topK = 10
	}

	// Perform search
	searchResult, err := q.pointsSvc.Search(ctx, &qdrant.SearchPoints{
		CollectionName: q.collection,
		Vector:         queryVector,
		Limit:          uint64(topK),
		WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
		WithVectors:    &qdrant.WithVectorsSelector{SelectorOptions: &qdrant.WithVectorsSelector_Enable{Enable: false}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}

	// Convert results to Match structs
	matches := make([]Match, 0, len(searchResult.Result))
	for _, scoredPoint := range searchResult.Result {
		// Extract point ID
		var pointID string
		if scoredPoint.Id != nil {
			if uuid := scoredPoint.Id.GetUuid(); uuid != "" {
				pointID = uuid
			} else if num := scoredPoint.Id.GetNum(); num != 0 {
				pointID = fmt.Sprintf("%d", num)
			}
		}

		// Extract metadata from payload
		metadata := make(map[string]string)
		var documentID string
		if scoredPoint.Payload != nil {
			for key, value := range scoredPoint.Payload {
				if strValue := value.GetStringValue(); strValue != "" {
					metadata[key] = strValue
					if key == "document_id" {
						documentID = strValue
					}
				} else {
					// Log if we're skipping non-string values (for debugging)
					log.Printf("Skipping non-string payload key: %s (type: %T)", key, value)
				}
			}
		} else {
			log.Printf("Warning: Payload is nil for point %s", pointID)
		}
		
		// Log if content is missing (for debugging)
		if metadata["content"] == "" {
			log.Printf("Warning: Content missing in metadata for point %s. Available keys: %v", pointID, getMetadataKeys(metadata))
		}

		matches = append(matches, Match{
			ID:         pointID,
			DocumentID: documentID,
			Score:      scoredPoint.Score,
			Metadata:   metadata,
		})
	}

	return matches, nil
}

// GetPointCount returns the number of points in the collection
func (q *QdrantVectorDB) GetPointCount(ctx context.Context) (int, error) {
	// Get collection info
	info, err := q.collectionsSvc.Get(ctx, &qdrant.GetCollectionInfoRequest{
		CollectionName: q.collection,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get collection info: %w", err)
	}

	if info.Result == nil {
		return 0, nil
	}

	// Extract point count from collection info
	// PointsCount is a *uint64
	if info.Result.PointsCount != nil {
		return int(*info.Result.PointsCount), nil
	}

	// Fallback: collection exists but may be empty
	return 0, nil
}

// UpdatePayload updates the payload (metadata) of an existing point
func (q *QdrantVectorDB) UpdatePayload(ctx context.Context, id string, tags []string) error {
	// Convert string ID to Qdrant PointId
	var pointID *qdrant.PointId

	// Check if it's a valid UUID format (simplified check)
	if len(id) > 20 {
		// Assume UUID format
		pointID = &qdrant.PointId{
			PointIdOptions: &qdrant.PointId_Uuid{
				Uuid: id,
			},
		}
	} else {
		// Try numeric
		var numID uint64
		if _, err := fmt.Sscanf(id, "%d", &numID); err == nil {
			pointID = &qdrant.PointId{
				PointIdOptions: &qdrant.PointId_Num{
					Num: numID,
				},
			}
		} else {
			// Fallback: use as string UUID
			pointID = &qdrant.PointId{
				PointIdOptions: &qdrant.PointId_Uuid{
					Uuid: id,
				},
			}
		}
	}

	// Prepare tags payload
	payload := make(map[string]*qdrant.Value)

	// Convert tags to JSON array string for storage
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	payload["tags"] = &qdrant.Value{
		Kind: &qdrant.Value_StringValue{StringValue: string(tagsJSON)},
	}

	// Also add individual tag fields for easier querying
	for i, tag := range tags {
		payload[fmt.Sprintf("tag_%d", i)] = &qdrant.Value{
			Kind: &qdrant.Value_StringValue{StringValue: tag},
		}
	}

	// Use SetPayload to update existing point
	_, err = q.pointsSvc.SetPayload(ctx, &qdrant.SetPayloadPoints{
		CollectionName: q.collection,
		Payload:        payload,
		PointsSelector: &qdrant.PointsSelector{PointsSelectorOneOf: &qdrant.PointsSelector_Points{Points: &qdrant.PointsIdsList{Ids: []*qdrant.PointId{pointID}}}},
	})
	if err != nil {
		return fmt.Errorf("failed to update payload: %w", err)
	}

	return nil
}

// Delete removes a vector from the collection.
func (q *QdrantVectorDB) Delete(ctx context.Context, id string) error {
	// Convert UUID string to Qdrant PointId
	pointID := &qdrant.PointId{
		PointIdOptions: &qdrant.PointId_Uuid{
			Uuid: id,
		},
	}

	// Delete the point
	_, err := q.pointsSvc.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: q.collection,
		Points:         &qdrant.PointsSelector{PointsSelectorOneOf: &qdrant.PointsSelector_Points{Points: &qdrant.PointsIdsList{Ids: []*qdrant.PointId{pointID}}}},
	})
	if err != nil {
		return fmt.Errorf("failed to delete point: %w", err)
	}

	return nil
}

// getMetadataKeys returns all keys from metadata map (helper for debugging)
func getMetadataKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

