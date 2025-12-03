// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package ai

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/the-hive/internal/embeddings"
)

// GenerateEmbedding generates an embedding for the given text
// Returns a dummy vector (all zeros) if OPENAI_API_KEY is not set
func GenerateEmbedding(text string) ([]float32, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Printf("warning: OPENAI_API_KEY not set, returning dummy vector")
		// Return dummy vector of 1536 dimensions (all zeros)
		return make([]float32, 1536), nil
	}

	// Use the existing embeddings package
	embedderConfig := map[string]string{
		"api_key": apiKey,
		"model":   os.Getenv("OPENAI_MODEL"),
	}
	if embedderConfig["model"] == "" {
		embedderConfig["model"] = "text-embedding-3-small" // default
	}

	embedder, err := embeddings.NewEmbedder("openai", embedderConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedder: %w", err)
	}

	ctx := context.Background()
	vector, err := embedder.EmbedText(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	return vector, nil
}
