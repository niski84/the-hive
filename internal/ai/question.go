// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// AskQuestion asks a yes/no question and returns YES or NO
// Returns the answer and usage information
func AskQuestion(ctx context.Context, prompt string) (string, *Usage, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", nil, fmt.Errorf("OPENAI_API_KEY not set")
	}

	// Use OpenAI Chat API for better yes/no answers
	url := "https://api.openai.com/v1/chat/completions"

	// Determine if this is a tagging request (contains "JSON array")
	isTaggingRequest := strings.Contains(strings.ToLower(prompt), "json array")
	
	var systemPrompt string
	var maxTokens int
	if isTaggingRequest {
		systemPrompt = "You are a helpful assistant that analyzes documents and returns JSON arrays of tags. Always return ONLY valid JSON, no other text."
		maxTokens = 100
	} else {
		systemPrompt = "You are a helpful assistant that answers yes/no questions. Always respond with only 'YES' or 'NO'."
		maxTokens = 10
	}
	
	payload := map[string]interface{}{
		"model": "gpt-3.5-turbo",
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": systemPrompt,
			},
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"max_tokens":   maxTokens,
		"temperature":  0.1, // Low temperature for consistent responses
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", nil, fmt.Errorf("OpenAI API error: %d - %s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		Model string `json:"model"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", nil, err
	}

	if len(result.Choices) == 0 {
		return "", nil, fmt.Errorf("no response from OpenAI")
	}

	answer := strings.TrimSpace(result.Choices[0].Message.Content)
	
	// Extract usage information
	usage := &Usage{
		InputTokens:  result.Usage.PromptTokens,
		OutputTokens: result.Usage.CompletionTokens,
	}
	if result.Model != "" {
		usage.Model = result.Model
	} else {
		usage.Model = "gpt-3.5-turbo" // Default model
	}
	
	// Normalize answer to YES or NO
	answerUpper := strings.ToUpper(answer)
	if strings.Contains(answerUpper, "YES") {
		return "YES", usage, nil
	}
	if strings.Contains(answerUpper, "NO") {
		return "NO", usage, nil
	}

	// Default to NO if unclear
	return "NO", usage, nil
}

