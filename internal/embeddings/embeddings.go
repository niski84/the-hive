// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package embeddings

import (
	"context"
	"fmt"
)

// Embedder generates vector embeddings from text.
type Embedder interface {
	// EmbedText generates an embedding vector for the given text.
	EmbedText(ctx context.Context, text string) ([]float32, error)
	
	// EmbedBatch generates embeddings for multiple texts (more efficient).
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	
	// Dimension returns the dimension of the embedding vectors.
	Dimension() int
}

// NewEmbedder creates an embedder based on the provided type and configuration.
// Supported types: "openai", "ollama", "mock" (for testing)
func NewEmbedder(embedderType string, config map[string]string) (Embedder, error) {
	switch embedderType {
	case "openai":
		apiKey := config["api_key"]
		if apiKey == "" {
			return nil, fmt.Errorf("openai api_key is required")
		}
		model := config["model"]
		if model == "" {
			model = "text-embedding-3-small" // default
		}
		return NewOpenAIEmbedder(apiKey, model)
	case "ollama":
		baseURL := config["base_url"]
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
		model := config["model"]
		if model == "" {
			model = "nomic-embed-text" // default
		}
		return NewOllamaEmbedder(baseURL, model)
	case "mock":
		dim := 384 // default mock dimension
		if dimStr := config["dimension"]; dimStr != "" {
			fmt.Sscanf(dimStr, "%d", &dim)
		}
		return NewMockEmbedder(dim), nil
	default:
		return nil, fmt.Errorf("unknown embedder type: %s", embedderType)
	}
}

