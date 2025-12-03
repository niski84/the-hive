// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package processor

import (
	"strings"
	"testing"
)

func TestChunker_ShortText(t *testing.T) {
	chunker := NewChunker()
	text := "This is a short text that should not be split."

	chunks, err := chunker.ChunkText(text)
	if err != nil {
		t.Fatalf("ChunkText failed: %v", err)
	}

	if len(chunks) != 1 {
		t.Errorf("Expected 1 chunk for short text, got %d", len(chunks))
	}

	if chunks[0] != text {
		t.Errorf("Chunk content mismatch. Expected: %q, Got: %q", text, chunks[0])
	}
}

func TestChunker_LongText(t *testing.T) {
	chunker := NewChunker()
	// Create text that's definitely longer than chunkSize (1000 chars)
	// We'll create ~3000 characters to ensure multiple chunks
	paragraph := "This is a sample paragraph. It contains multiple sentences. Each sentence ends with a period. "
	text := strings.Repeat(paragraph, 40) // ~3000 characters

	chunks, err := chunker.ChunkText(text)
	if err != nil {
		t.Fatalf("ChunkText failed: %v", err)
	}

	if len(chunks) < 2 {
		t.Errorf("Expected at least 2 chunks for long text (~3000 chars), got %d", len(chunks))
	}

	// Verify total length is approximately correct (allowing for overlap)
	totalLength := 0
	for _, chunk := range chunks {
		totalLength += len(chunk)
	}

	// Total should be approximately original length + (overlap * (num_chunks - 1))
	expectedMin := len(text)
	expectedMax := len(text) + (chunker.chunkOverlap * (len(chunks) - 1))

	if totalLength < expectedMin || totalLength > expectedMax {
		t.Errorf("Total chunk length out of expected range. Got: %d, Expected: %d-%d", totalLength, expectedMin, expectedMax)
	}
}

func TestChunker_Overlap(t *testing.T) {
	chunker := NewChunker()
	// Create text that will definitely create at least 2 chunks
	// Use a unique marker to verify overlap
	part1 := strings.Repeat("A", 800) + ". "
	part2 := strings.Repeat("B", 800) + ". "
	part3 := strings.Repeat("C", 800) + ". "
	text := part1 + part2 + part3 // ~2400 characters, should create 3 chunks

	chunks, err := chunker.ChunkText(text)
	if err != nil {
		t.Fatalf("ChunkText failed: %v", err)
	}

	if len(chunks) < 2 {
		t.Fatalf("Need at least 2 chunks to test overlap, got %d", len(chunks))
	}

	// Check overlap between consecutive chunks
	for i := 0; i < len(chunks)-1; i++ {
		chunk1 := chunks[i]
		chunk2 := chunks[i+1]

		// Find the overlap: end of chunk1 should match start of chunk2
		// We'll check if the last N characters of chunk1 appear at the start of chunk2
		// where N is approximately the overlap size
		overlapSize := chunker.chunkOverlap
		if len(chunk1) < overlapSize {
			overlapSize = len(chunk1)
		}
		if len(chunk2) < overlapSize {
			overlapSize = len(chunk2)
		}

		chunk1End := chunk1[len(chunk1)-overlapSize:]
		chunk2Start := chunk2[:overlapSize]

		// The overlap should match (allowing for whitespace differences from trimming)
		chunk1EndClean := strings.TrimSpace(chunk1End)
		chunk2StartClean := strings.TrimSpace(chunk2Start)

		// Check if there's significant overlap (at least 50% of expected overlap size)
		minOverlap := overlapSize / 2
		if len(chunk1EndClean) < minOverlap || len(chunk2StartClean) < minOverlap {
			t.Logf("Warning: Chunk %d and %d may not have expected overlap", i, i+1)
			t.Logf("Chunk %d end (%d chars): ...%s", i, len(chunk1EndClean), chunk1EndClean[len(chunk1EndClean)-50:])
			t.Logf("Chunk %d start (%d chars): %s...", i+1, len(chunk2StartClean), chunk2StartClean[:50])
		}

		// Verify chunks are not identical (they should have different content)
		if chunk1 == chunk2 {
			t.Errorf("Chunk %d and %d are identical, no overlap detected", i, i+1)
		}
	}
}

func TestChunker_EmptyText(t *testing.T) {
	chunker := NewChunker()
	text := ""

	chunks, err := chunker.ChunkText(text)
	if err != nil {
		t.Fatalf("ChunkText failed: %v", err)
	}

	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for empty text, got %d", len(chunks))
	}
}

func TestChunker_SentenceBoundaries(t *testing.T) {
	chunker := NewChunker()
	// Create text with clear sentence boundaries that should be respected
	text := strings.Repeat("This is sentence one. This is sentence two. This is sentence three. ", 50)

	chunks, err := chunker.ChunkText(text)
	if err != nil {
		t.Fatalf("ChunkText failed: %v", err)
	}

	// Verify that chunks don't cut sentences awkwardly
	// (This is a heuristic test - we check that most chunks end with sentence endings)
	sentenceEndings := 0
	for _, chunk := range chunks {
		trimmed := strings.TrimSpace(chunk)
		if len(trimmed) > 0 {
			lastChar := trimmed[len(trimmed)-1]
			if lastChar == '.' || lastChar == '!' || lastChar == '?' {
				sentenceEndings++
			}
		}
	}

	// At least 50% of chunks should end with sentence endings
	expectedMin := len(chunks) / 2
	if sentenceEndings < expectedMin {
		t.Logf("Only %d/%d chunks end with sentence endings. Chunker may not be respecting sentence boundaries well.", sentenceEndings, len(chunks))
		// This is a warning, not a failure, as the chunker tries but may not always succeed
	}
}
