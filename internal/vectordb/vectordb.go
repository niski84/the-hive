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
	Search(ctx context.Context, queryVector []float32, topK int, organizationID string) ([]Match, error)
	Delete(ctx context.Context, id string) error
	GetPointCount(ctx context.Context) (int, error)
	UpdatePayload(ctx context.Context, id string, tags []string) error
	PurgeCollection(ctx context.Context) error // Delete all points from the collection
	PurgeByOrganization(ctx context.Context, organizationID string) (int, error) // Delete all points for a specific organization
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
// CRITICAL: organization_id must be included in metadata for multi-tenancy isolation
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
	
	// CRITICAL: Always include organization_id in payload for multi-tenancy
	// If not provided, log a warning but don't fail (for backward compatibility during migration)
	if orgID, ok := metadata["organization_id"]; ok && orgID != "" {
		payload["organization_id"] = &qdrant.Value{
			Kind: &qdrant.Value_StringValue{StringValue: orgID},
		}
	} else {
		log.Printf("Warning: Upsert called without organization_id for point %s - multi-tenancy isolation may be compromised", id)
	}
	
	for k, v := range metadata {
		if k != "document_id" && k != "organization_id" {
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
// CRITICAL: organizationID must be provided for multi-tenancy isolation
// If organizationID is empty, search will return results from all organizations (backward compatibility)
func (q *QdrantVectorDB) Search(ctx context.Context, queryVector []float32, topK int, organizationID string) ([]Match, error) {
	if len(queryVector) == 0 {
		return nil, errors.New("query vector cannot be empty")
	}

	if topK <= 0 {
		topK = 10
	}

	// Build search request
	searchReq := &qdrant.SearchPoints{
		CollectionName: q.collection,
		Vector:         queryVector,
		Limit:          uint64(topK),
		WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
		WithVectors:    &qdrant.WithVectorsSelector{SelectorOptions: &qdrant.WithVectorsSelector_Enable{Enable: false}},
	}
	
	// CRITICAL: Apply organization filter for multi-tenancy isolation
	if organizationID != "" {
		searchReq.Filter = &qdrant.Filter{
			Must: []*qdrant.Condition{
				{
					ConditionOneOf: &qdrant.Condition_Field{
						Field: &qdrant.FieldCondition{
							Key: "organization_id",
							Match: &qdrant.Match{
								MatchValue: &qdrant.Match_Keyword{
									Keyword: organizationID,
								},
							},
						},
					},
				},
			},
		}
	} else {
		log.Printf("Warning: Search called without organizationID - results may include data from all organizations")
	}

	// Perform search
	searchResult, err := q.pointsSvc.Search(ctx, searchReq)
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

// PurgeCollection deletes all points from the collection
func (q *QdrantVectorDB) PurgeCollection(ctx context.Context) error {
	log.Printf("Purging all points from collection %s", q.collection)
	
	// Get all point IDs using Scroll API
	// Scroll with a large limit and no filter gets all points
	pointIDs := make([]*qdrant.PointId, 0)
	var offset *qdrant.PointId = nil
	limit := uint32(1000) // Get 1000 at a time
	
	for {
		scrollRequest := &qdrant.ScrollPoints{
			CollectionName: q.collection,
			Filter:         nil, // No filter = all points
			WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: false}},
			WithVectors:    &qdrant.WithVectorsSelector{SelectorOptions: &qdrant.WithVectorsSelector_Enable{Enable: false}},
		}
		
		// Set offset if we have one
		if offset != nil {
			scrollRequest.Offset = offset
		}
		
		// Set limit
		scrollRequest.Limit = &limit
		
		scrollResult, err := q.pointsSvc.Scroll(ctx, scrollRequest)
		
		if err != nil {
			return fmt.Errorf("failed to scroll points for purge: %w", err)
		}
		
		// Extract point IDs from this batch
		for _, point := range scrollResult.Result {
			if point.Id != nil {
				pointIDs = append(pointIDs, point.Id)
			}
		}
		
		// Check if we got all points (next_page_offset will be nil if done)
		if scrollResult.NextPageOffset == nil || len(scrollResult.Result) == 0 {
			break
		}
		
		// Continue scrolling from the next offset
		offset = scrollResult.NextPageOffset
	}
	
	if len(pointIDs) == 0 {
		log.Printf("No points to purge in collection %s", q.collection)
		return nil
	}
	
	log.Printf("Found %d points to delete from collection %s", len(pointIDs), q.collection)
	
	// Delete all points in batches (Qdrant may have limits on batch size)
	batchSize := 1000
	for i := 0; i < len(pointIDs); i += batchSize {
		end := i + batchSize
		if end > len(pointIDs) {
			end = len(pointIDs)
		}
		batch := pointIDs[i:end]
		
		_, err := q.pointsSvc.Delete(ctx, &qdrant.DeletePoints{
			CollectionName: q.collection,
			Points: &qdrant.PointsSelector{
				PointsSelectorOneOf: &qdrant.PointsSelector_Points{
					Points: &qdrant.PointsIdsList{
						Ids: batch,
					},
				},
			},
		})
		
		if err != nil {
			return fmt.Errorf("failed to delete batch %d-%d: %w", i, end, err)
		}
		log.Printf("Deleted batch %d-%d (%d points)", i, end, len(batch))
	}
	
	log.Printf("Successfully purged %d points from collection %s", len(pointIDs), q.collection)
	return nil
}

// PurgeByOrganization deletes all points for a specific organization
func (q *QdrantVectorDB) PurgeByOrganization(ctx context.Context, organizationID string) (int, error) {
	if organizationID == "" {
		return 0, errors.New("organizationID is required")
	}
	
	log.Printf("Purging points for organization %s from collection %s", organizationID, q.collection)
	
	// Build filter for organization_id
	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			{
				ConditionOneOf: &qdrant.Condition_Field{
					Field: &qdrant.FieldCondition{
						Key: "organization_id",
						Match: &qdrant.Match{
							MatchValue: &qdrant.Match_Keyword{
								Keyword: organizationID,
							},
						},
					},
				},
			},
		},
	}
	
	// Get all point IDs using Scroll API with filter
	pointIDs := make([]*qdrant.PointId, 0)
	var offset *qdrant.PointId = nil
	limit := uint32(1000) // Get 1000 at a time
	
	for {
		scrollRequest := &qdrant.ScrollPoints{
			CollectionName: q.collection,
			Filter:         filter, // Filter by organization_id
			WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: false}},
			WithVectors:    &qdrant.WithVectorsSelector{SelectorOptions: &qdrant.WithVectorsSelector_Enable{Enable: false}},
		}
		
		// Set offset if we have one
		if offset != nil {
			scrollRequest.Offset = offset
		}
		
		// Set limit
		scrollRequest.Limit = &limit
		
		scrollResult, err := q.pointsSvc.Scroll(ctx, scrollRequest)
		
		if err != nil {
			return 0, fmt.Errorf("failed to scroll points for purge: %w", err)
		}
		
		// Extract point IDs from this batch
		for _, point := range scrollResult.Result {
			if point.Id != nil {
				pointIDs = append(pointIDs, point.Id)
			}
		}
		
		// Check if we got all points (next_page_offset will be nil if done)
		if scrollResult.NextPageOffset == nil || len(scrollResult.Result) == 0 {
			break
		}
		
		// Continue scrolling from the next offset
		offset = scrollResult.NextPageOffset
	}
	
	if len(pointIDs) == 0 {
		log.Printf("No points to purge for organization %s in collection %s", organizationID, q.collection)
		return 0, nil
	}
	
	log.Printf("Found %d points to delete for organization %s from collection %s", len(pointIDs), organizationID, q.collection)
	
	// Delete all points in batches (Qdrant may have limits on batch size)
	batchSize := 1000
	for i := 0; i < len(pointIDs); i += batchSize {
		end := i + batchSize
		if end > len(pointIDs) {
			end = len(pointIDs)
		}
		batch := pointIDs[i:end]
		
		_, err := q.pointsSvc.Delete(ctx, &qdrant.DeletePoints{
			CollectionName: q.collection,
			Points: &qdrant.PointsSelector{
				PointsSelectorOneOf: &qdrant.PointsSelector_Points{
					Points: &qdrant.PointsIdsList{
						Ids: batch,
					},
				},
			},
		})
		
		if err != nil {
			return 0, fmt.Errorf("failed to delete batch %d-%d: %w", i, end, err)
		}
		log.Printf("Deleted batch %d-%d (%d points) for organization %s", i, end, len(batch), organizationID)
	}
	
	log.Printf("Successfully purged %d points for organization %s from collection %s", len(pointIDs), organizationID, q.collection)
	return len(pointIDs), nil
}

// getMetadataKeys returns all keys from metadata map (helper for debugging)
func getMetadataKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

