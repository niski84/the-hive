package embeddings

import (
	"context"
	"hash/fnv"
	"math"
)

// MockEmbedder generates deterministic mock embeddings for testing.
type MockEmbedder struct {
	dim int
}

// NewMockEmbedder creates a new mock embedder with the specified dimension.
func NewMockEmbedder(dim int) *MockEmbedder {
	return &MockEmbedder{dim: dim}
}

// Dimension returns the embedding dimension.
func (e *MockEmbedder) Dimension() int {
	return e.dim
}

// EmbedText generates a deterministic mock embedding based on text hash.
func (e *MockEmbedder) EmbedText(ctx context.Context, text string) ([]float32, error) {
	// Generate deterministic "embeddings" based on text hash
	h := fnv.New32a()
	h.Write([]byte(text))
	seed := h.Sum32()

	embedding := make([]float32, e.dim)
	for i := 0; i < e.dim; i++ {
		// Use a simple hash-based pseudo-random function
		val := float32(math.Sin(float64(seed*uint32(i+1)) * 0.1))
		embedding[i] = val
	}

	// Normalize the vector
	var sum float32
	for _, v := range embedding {
		sum += v * v
	}
	norm := float32(math.Sqrt(float64(sum)))
	if norm > 0 {
		for i := range embedding {
			embedding[i] /= norm
		}
	}

	return embedding, nil
}

// EmbedBatch generates embeddings for multiple texts.
func (e *MockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i, text := range texts {
		embedding, err := e.EmbedText(ctx, text)
		if err != nil {
			return nil, err
		}
		result[i] = embedding
	}
	return result, nil
}

