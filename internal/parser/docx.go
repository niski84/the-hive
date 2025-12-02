package parser

import (
	"fmt"
	"strings"

	"github.com/nguyenthenguyen/docx"
)

// parseDOCX extracts text from a DOCX file
func parseDOCX(filePath string) (string, error) {
	doc, err := docx.ReadDocxFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open DOCX file: %w", err)
	}
	defer doc.Close()

	// Extract text content
	text := doc.Editable().GetContent()
	
	// Strip XML tags and clean up
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("no text extracted from DOCX: %s", filePath)
	}

	return text, nil
}

