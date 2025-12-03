// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package parser

import (
	"fmt"
	"os"
)

// parseText extracts text from plain text files (.txt, .md)
func parseText(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read text file: %w", err)
	}

	text := string(content)
	if text == "" {
		return "", fmt.Errorf("no content in text file: %s", filePath)
	}

	return text, nil
}

