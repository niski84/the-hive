package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaEmbedder uses a local Ollama instance for embeddings.
type OllamaEmbedder struct {
	baseURL string
	model   string
	client  *http.Client
	dim     int
}

// NewOllamaEmbedder creates a new Ollama embedder.
func NewOllamaEmbedder(baseURL, model string) (*OllamaEmbedder, error) {
	// Default dimension for nomic-embed-text
	dim := 768

	return &OllamaEmbedder{
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 60 * time.Second}, // Ollama can be slower
		dim:     dim,
	}, nil
}

// Dimension returns the embedding dimension.
func (e *OllamaEmbedder) Dimension() int {
	return e.dim
}

// EmbedText generates an embedding for a single text.
func (e *OllamaEmbedder) EmbedText(ctx context.Context, text string) ([]float32, error) {
	type requestPayload struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}

	payload := requestPayload{
		Model:  e.model,
		Prompt: text,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/embeddings", e.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, string(body))
	}

	type responsePayload struct {
		Embedding []float64 `json:"embedding"`
	}

	var response responsePayload
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert float64 to float32
	result := make([]float32, len(response.Embedding))
	for i, v := range response.Embedding {
		result[i] = float32(v)
	}

	return result, nil
}

// EmbedBatch generates embeddings for multiple texts (sequential for Ollama).
func (e *OllamaEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i, text := range texts {
		embedding, err := e.EmbedText(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("failed to embed text %d: %w", i, err)
		}
		result[i] = embedding
	}
	return result, nil
}

