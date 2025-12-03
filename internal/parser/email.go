// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package parser

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mnako/letters"
)

// parseEmail extracts text from an EML email file
func parseEmail(filePath string) (string, error) {
	// Open the EML file
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open EML file: %w", err)
	}
	defer file.Close()

	// Parse the EML file using letters.ParseEmail
	email, err := letters.ParseEmail(file)
	if err != nil {
		return "", fmt.Errorf("failed to parse EML file: %w", err)
	}

	var builder strings.Builder

	// Format email metadata
	if email.Headers.Subject != "" {
		builder.WriteString(fmt.Sprintf("Subject: %s\n", email.Headers.Subject))
	}

	if len(email.Headers.From) > 0 {
		from := email.Headers.From[0]
		sender := ""
		if from.Name != "" {
			sender = fmt.Sprintf("%s <%s>", from.Name, from.Address)
		} else {
			sender = from.Address
		}
		builder.WriteString(fmt.Sprintf("Sender: %s\n", sender))
	}

	if !email.Headers.Date.IsZero() {
		builder.WriteString(fmt.Sprintf("Date: %s\n", email.Headers.Date.Format(time.RFC3339)))
	}

	// Add body text
	builder.WriteString("\n")

	// Prefer text body, fall back to HTML body if needed
	bodyText := ""
	if email.Text != "" {
		bodyText = email.Text
	} else if email.HTML != "" {
		// For HTML emails, extract text (HTML tags will be stripped by the vector DB context)
		// In a production system, you might want to use goquery to strip HTML tags here
		bodyText = email.HTML
	}

	if bodyText != "" {
		builder.WriteString(bodyText)
	}

	result := strings.TrimSpace(builder.String())
	if result == "" {
		return "", fmt.Errorf("no content extracted from EML: %s", filePath)
	}

	return result, nil
}
