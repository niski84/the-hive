// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
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

// OpenAIEmbedder uses OpenAI's embedding API.
type OpenAIEmbedder struct {
	apiKey string
	model  string
	client *http.Client
	dim    int
}

// NewOpenAIEmbedder creates a new OpenAI embedder.
func NewOpenAIEmbedder(apiKey, model string) (*OpenAIEmbedder, error) {
	// Determine dimension based on model
	dim := 1536 // default for text-embedding-3-small
	if model == "text-embedding-3-large" {
		dim = 3072
	} else if model == "text-embedding-ada-002" {
		dim = 1536
	}

	return &OpenAIEmbedder{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
		dim:    dim,
	}, nil
}

// Dimension returns the embedding dimension.
func (e *OpenAIEmbedder) Dimension() int {
	return e.dim
}

// EmbedText generates an embedding for a single text.
func (e *OpenAIEmbedder) EmbedText(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

// EmbedBatch generates embeddings for multiple texts.
func (e *OpenAIEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	type requestPayload struct {
		Input []string `json:"input"`
		Model string   `json:"model"`
	}

	payload := requestPayload{
		Input: texts,
		Model: e.model,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/embeddings", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e.apiKey))

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai API error (status %d): %s", resp.StatusCode, string(body))
	}

	type responsePayload struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}

	var response responsePayload
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(response.Data) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(response.Data))
	}

	// Convert float64 to float32
	result := make([][]float32, len(response.Data))
	for i, data := range response.Data {
		result[i] = make([]float32, len(data.Embedding))
		for j, v := range data.Embedding {
			result[i][j] = float32(v)
		}
	}

	return result, nil
}

