// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package parser

import (
	"fmt"
	"strings"

	"github.com/gen2brain/go-fitz"
)

// parsePDF extracts text from a PDF file using go-fitz (MuPDF)
// API reference: https://pkg.go.dev/github.com/gen2brain/go-fitz
func parsePDF(filePath string) (string, error) {
	// New creates a new Document from a file path
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
