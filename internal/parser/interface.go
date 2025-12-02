package parser

// Parser defines the interface for all file parsers
type Parser interface {
	// Parse extracts text content from a file
	Parse(filePath string) (string, error)
}

