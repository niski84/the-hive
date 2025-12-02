package parser

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ParseFile routes a file to the appropriate parser based on its extension
func ParseFile(filePath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".pdf":
		return parsePDF(filePath)
	case ".docx":
		return parseDOCX(filePath)
	case ".xlsx", ".xls":
		return parseExcel(filePath)
	case ".html", ".htm":
		return parseHTML(filePath)
	case ".eml":
		return parseEmail(filePath)
	default:
		return "", fmt.Errorf("unsupported file type: %s", ext)
	}
}

// IsSupportedFile checks if a file extension is supported
func IsSupportedFile(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	supported := []string{".pdf", ".docx", ".xlsx", ".xls", ".html", ".htm", ".eml"}
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

