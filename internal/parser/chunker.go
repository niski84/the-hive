// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package parser

import (
	"strings"
)

// Chunker handles text chunking with configurable size and overlap
type Chunker struct {
	chunkSize    int
	chunkOverlap int
}

// NewChunker creates a new chunker with default settings
func NewChunker() *Chunker {
	return &Chunker{
		chunkSize:    1000, // characters per chunk
		chunkOverlap: 200,  // overlap between chunks
	}
}

// ChunkText splits text into overlapping chunks
func (c *Chunker) ChunkText(text string) ([]string, error) {
	if len(text) == 0 {
		return []string{}, nil
	}

	var chunks []string
	start := 0

	for start < len(text) {
		end := start + c.chunkSize
		if end > len(text) {
			end = len(text)
		}

		chunk := text[start:end]
		chunks = append(chunks, strings.TrimSpace(chunk))

		if end >= len(text) {
			break
		}

		start = end - c.chunkOverlap
		if start < 0 {
			start = 0
		}
	}

	return chunks, nil
}

