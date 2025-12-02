package parser

import (
	"fmt"
	"os"

	"github.com/PuerkitoBio/goquery"
)

// parseHTML extracts text from an HTML file, removing script and style tags
func parseHTML(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open HTML file: %w", err)
	}
	defer file.Close()

	// Parse HTML with goquery
	doc, err := goquery.NewDocumentFromReader(file)
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %w", err)
	}

	// Remove script, style, and noscript tags before extracting text
	doc.Find("script, style, noscript").Each(func(i int, s *goquery.Selection) {
		s.Remove()
	})

	// Extract text content
	text := doc.Text()
	if text == "" {
		return "", fmt.Errorf("no text extracted from HTML: %s", filePath)
	}

	return text, nil
}

