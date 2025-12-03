// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package processor

import (
	"strings"
)

// Chunker handles text chunking with sentence-aware splitting
type Chunker struct {
	chunkSize    int
	chunkOverlap int
}

// NewChunker creates a new chunker with default settings
// Default: ~1000 characters per chunk with 100 character overlap
func NewChunker() *Chunker {
	return &Chunker{
		chunkSize:    1000,
		chunkOverlap: 100,
	}
}

// ChunkText splits text into overlapping chunks, trying to avoid cutting sentences
func (c *Chunker) ChunkText(text string) ([]string, error) {
	if len(text) == 0 {
		return []string{}, nil
	}

	var chunks []string
	start := 0
	textLen := len(text)

	for start < textLen {
		end := start + c.chunkSize
		if end > textLen {
			end = textLen
		}

		// If we're not at the end, try to find a sentence boundary
		if end < textLen {
			// Look for sentence endings within the last 200 characters
			searchStart := end - 200
			if searchStart < start {
				searchStart = start
			}

			// Try to find a good break point (period, exclamation, question mark followed by space)
			bestBreak := end
			for i := end - 1; i >= searchStart; i-- {
				if i < len(text) {
					char := text[i]
					// Check for sentence endings
					if (char == '.' || char == '!' || char == '?') && i+1 < len(text) {
						// Check if followed by space or newline
						nextChar := text[i+1]
						if nextChar == ' ' || nextChar == '\n' || nextChar == '\r' {
							bestBreak = i + 1
							break
						}
					}
					// Also check for paragraph breaks (double newline)
					if i+1 < len(text) && char == '\n' && text[i+1] == '\n' {
						bestBreak = i + 2
						break
					}
				}
			}

			// If we found a good break point, use it
			if bestBreak > start {
				end = bestBreak
			}
		}

		chunk := strings.TrimSpace(text[start:end])
		if len(chunk) > 0 {
			chunks = append(chunks, chunk)
		}

		// Move start position with overlap
		if end >= textLen {
			break
		}

		start = end - c.chunkOverlap
		if start < 0 {
			start = 0
		}
		// Ensure we don't get stuck in a loop
		if start >= end {
			start = end
		}
	}

	return chunks, nil
}
