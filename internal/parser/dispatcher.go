// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package parser

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ParseFile routes a file to the appropriate parser based on its extension
func ParseFile(filePath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	var text string
	var err error

	switch ext {
	case ".pdf":
		text, err = parsePDF(filePath)
	case ".docx":
		text, err = parseDOCX(filePath)
	case ".txt", ".md":
		text, err = parseText(filePath)
	case ".xlsx", ".xls":
		text, err = parseExcel(filePath)
	case ".html", ".htm":
		text, err = parseHTML(filePath)
	case ".eml":
		text, err = parseEmail(filePath)
	default:
		return "", fmt.Errorf("unsupported file type: %s", ext)
	}

	if err != nil {
		return "", err
	}

	// Log text preview: character count and first 150 chars
	charCount := len(text)
	snippet := text
	if len(snippet) > 150 {
		snippet = snippet[:150] + "..."
	}
	fmt.Printf("[TEXT EXTRACT] %s: %d characters\n", filePath, charCount)
	fmt.Printf("[TEXT SNIPPET] %s\n", snippet)

	return text, nil
}

// IsSupportedFile checks if a file extension is supported
func IsSupportedFile(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	supported := []string{".pdf", ".docx", ".txt", ".md", ".xlsx", ".xls", ".html", ".htm", ".eml"}
	for _, s := range supported {
		if ext == s {
			return true
		}
	}
	return false
}

// IsTemporaryFile checks if a file is a temporary file (e.g., ~$doc.docx)
func IsTemporaryFile(filePath string) bool {
	base := filepath.Base(filePath)
	// Check for common temporary file patterns
	if strings.HasPrefix(base, "~$") {
		return true
	}
	if strings.HasPrefix(base, "._") {
		return true
	}
	if strings.HasSuffix(base, ".tmp") {
		return true
	}
	return false
}
