// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package pdf

import (
	"fmt"
	"strings"

	"github.com/gen2brain/go-fitz"
)

// Processor handles PDF text extraction and chunking
type Processor struct {
	chunkSize    int
	chunkOverlap int
}

// NewProcessor creates a new PDF processor
func NewProcessor() *Processor {
	return &Processor{
		chunkSize:    1000,  // characters per chunk
		chunkOverlap: 200,   // overlap between chunks
	}
}

// ExtractText extracts text from a PDF file using go-fitz (MuPDF)
func (p *Processor) ExtractText(filePath string) (string, error) {
	doc, err := fitz.New(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open PDF: %w", err)
	}
	defer doc.Close()

	var textBuilder strings.Builder
	numPages := doc.NumPage()

	for i := 0; i < numPages; i++ {
		pageText, err := doc.Text(i)
		if err != nil {
			// Log error but continue with other pages
			fmt.Printf("warning: failed to extract text from page %d: %v\n", i, err)
			continue
		}
		textBuilder.WriteString(pageText)
		// Add a page separator for readability
		if i < numPages-1 {
			textBuilder.WriteString("\n\n")
		}
	}

	extractedText := strings.TrimSpace(textBuilder.String())
	if extractedText == "" {
		return "", fmt.Errorf("no text extracted from PDF: %s", filePath)
	}

	return extractedText, nil
}

// ChunkText splits text into overlapping chunks
func (p *Processor) ChunkText(text string) ([]string, error) {
	if len(text) == 0 {
		return []string{}, nil
	}

	var chunks []string
	start := 0

	for start < len(text) {
		end := start + p.chunkSize
		if end > len(text) {
			end = len(text)
		}

		chunk := text[start:end]
		chunks = append(chunks, strings.TrimSpace(chunk))

		if end >= len(text) {
			break
		}

		start = end - p.chunkOverlap
		if start < 0 {
			start = 0
		}
	}

	return chunks, nil
}

