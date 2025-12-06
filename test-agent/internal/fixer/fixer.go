// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package fixer

import (
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/the-hive/test-agent/internal/analyzer"
)

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Fix represents a code change to apply
type Fix struct {
	FilePath    string
	OldCode     string
	NewCode     string
	LineNumbers []int
	Description string
}

// Fixer generates code fixes from analysis results
type Fixer struct {
	projectRoot string
}

// NewFixer creates a new fix generator
func NewFixer(projectRoot string) *Fixer {
	return &Fixer{
		projectRoot: projectRoot,
	}
}

// GenerateFix creates a Fix from an analysis result
func (f *Fixer) GenerateFix(analysis *analyzer.AnalysisResult, testFailure string) ([]Fix, error) {
	var fixes []Fix

	// Check if this is a console error (JavaScript/HTML error)
	isConsoleError := strings.Contains(testFailure, "Console Error:") ||
		strings.Contains(testFailure, "SyntaxError") ||
		strings.Contains(testFailure, "ReferenceError") ||
		strings.Contains(testFailure, "TypeError") ||
		strings.Contains(testFailure, "is not defined")

	// Check if this is a server-side issue (API returning wrong content type)
	// Look for JSON parsing errors that indicate server is returning HTML instead of JSON
	hasJSONError := strings.Contains(testFailure, "JSON") &&
		(strings.Contains(testFailure, "Unexpected") ||
			strings.Contains(testFailure, "parse") ||
			strings.Contains(testFailure, "position"))

	// Check if error mentions API endpoints or server-side issues
	hasAPIEndpoint := strings.Contains(testFailure, "/api/") ||
		strings.Contains(testFailure, "Failed to load") ||
		strings.Contains(testFailure, "tenant AI key") ||
		strings.Contains(testFailure, "loadAPIKeys")

	isServerSideIssue := hasJSONError && hasAPIEndpoint

	if isConsoleError || isServerSideIssue {
		// For server-side issues, try to find Go handler/middleware files first
		if isServerSideIssue {
			goFiles := f.findGoFilesFromError(testFailure, analysis.Fix)
			for _, goFile := range goFiles {
				// Extract line number from error if available
				lineNum := f.extractLineNumberFromError(testFailure, goFile)

				// Read the file to get the old code
				oldCode := ""
				if lineNum > 0 {
					oldCode = f.extractOldCodeFromFile(goFile, lineNum)
				} else {
					// Read relevant sections of the file
					content, err := os.ReadFile(goFile)
					if err == nil {
						oldCode = string(content)
					}
				}

				// Extract new code from AI response
				newCode := f.extractNewCodeForFile(analysis.Fix, filepath.Base(goFile))

				// If no code block found, try to extract from the explanation
				if newCode == "" && analysis.Fix != "" {
					newCode = f.extractCodeFromText(analysis.Fix, oldCode)
				}

				// If still no code, try to extract from the full analysis response
				if newCode == "" && analysis.Fix != "" {
					newCode = f.extractCodeFromFullResponse(analysis.Fix, oldCode, filepath.Base(goFile))
				}

				// Validate that newCode is actually Go code, not HTML/JS
				if newCode != "" {
					// Check if newCode looks like Go code (not HTML/JS)
					isGoCode := strings.Contains(newCode, "func ") ||
						strings.Contains(newCode, "package ") ||
						strings.Contains(newCode, "http.ResponseWriter") ||
						strings.Contains(newCode, "w.Header()") ||
						strings.Contains(newCode, "json.NewEncoder")

					// Reject if it looks like HTML/JS
					isHTMLJS := strings.Contains(newCode, "<script") ||
						strings.Contains(newCode, "async function") ||
						strings.Contains(newCode, "document.") ||
						strings.Contains(newCode, "fetch(")

					if isGoCode && !isHTMLJS {
						fixes = append(fixes, Fix{
							FilePath:    goFile,
							OldCode:     oldCode,
							NewCode:     newCode,
							LineNumbers: []int{lineNum},
							Description: analysis.Explanation,
						})
					}
				} else if oldCode != "" {
					// If we have old code but no new code, still create a fix entry
					// but mark it as needing manual review
					fixes = append(fixes, Fix{
						FilePath:    goFile,
						OldCode:     oldCode,
						NewCode:     analysis.Fix, // Use full fix text
						LineNumbers: []int{lineNum},
						Description: analysis.Explanation,
					})
				}
			}
		}

		// For console errors that are NOT server-side issues, try to find HTML files
		if !isServerSideIssue {
			htmlFiles := f.findHTMLFilesFromError(testFailure, analysis.Fix)
			for _, htmlFile := range htmlFiles {
				// Extract line number from error if available (e.g., "analyst.html:307")
				lineNum := f.extractLineNumberFromError(testFailure, htmlFile)

				// Read the file to get the old code
				oldCode := ""
				if lineNum > 0 {
					oldCode = f.extractOldCodeFromFile(htmlFile, lineNum)
				} else {
					// If no line number, read the entire file (for HTML/JS, we might need full context)
					content, err := os.ReadFile(htmlFile)
					if err == nil {
						oldCode = string(content)
					}
				}

				// Extract new code from AI response
				newCode := f.extractNewCodeForFile(analysis.Fix, filepath.Base(htmlFile))

				// If no code block found, try to extract from the explanation
				if newCode == "" && analysis.Fix != "" {
					newCode = f.extractCodeFromText(analysis.Fix, oldCode)
				}

				// If still no code, try to extract from the full analysis response
				if newCode == "" && analysis.Fix != "" {
					// Look for code patterns in the fix text
					newCode = f.extractCodeFromFullResponse(analysis.Fix, oldCode, filepath.Base(htmlFile))
				}

				// Validate that newCode is actually HTML/JS code, not Go code
				if newCode != "" {
					// Check if newCode looks like Go code (should NOT be in HTML files)
					isGoCode := strings.Contains(newCode, "func ") ||
						strings.Contains(newCode, "package ") ||
						strings.Contains(newCode, "http.ResponseWriter") ||
						strings.Contains(newCode, "w.Header()") ||
						strings.Contains(newCode, "json.NewEncoder")

					// Reject Go code in HTML files
					if isGoCode {
						log.Printf("⚠️  Skipping fix: Go code detected in HTML file %s", htmlFile)
						continue
					}

					fixes = append(fixes, Fix{
						FilePath:    htmlFile,
						OldCode:     oldCode,
						NewCode:     newCode,
						LineNumbers: []int{lineNum},
						Description: analysis.Explanation,
					})
				} else if oldCode != "" {
					// Check if the analysis fix contains Go code
					if strings.Contains(analysis.Fix, "func ") || strings.Contains(analysis.Fix, "http.ResponseWriter") {
						log.Printf("⚠️  Skipping fix: Analysis contains Go code for HTML file %s", htmlFile)
						continue
					}

					// Even if we can't extract new code, create a fix with the old code
					// so the AI can see what needs to be changed
					// This helps when the AI response doesn't have code blocks
					fixes = append(fixes, Fix{
						FilePath:    htmlFile,
						OldCode:     oldCode,
						NewCode:     analysis.Fix, // Use the full fix text as new code
						LineNumbers: []int{lineNum},
						Description: analysis.Explanation,
					})
				}
			}
		}

		// Return fixes if we found any
		if len(fixes) > 0 {
			return fixes, nil
		}
	}

	// For compilation errors, extract file paths and line numbers directly from the error output
	if strings.Contains(testFailure, "CompilationErrors") || strings.Contains(testFailure, "# github.com/") {
		// Extract file paths with line numbers (format: path/to/file.go:line:col: message)
		filePathRegex := regexp.MustCompile(`([^\s]+\.go):(\d+):\d+:`)
		fileMatches := filePathRegex.FindAllStringSubmatch(testFailure, -1)

		for _, match := range fileMatches {
			if len(match) >= 3 {
				filePath := match[1]
				lineNum, _ := strconv.Atoi(match[2])

				// Make path absolute if relative
				if !filepath.IsAbs(filePath) {
					filePath = filepath.Join(f.projectRoot, filePath)
				}

				// Read the file to get the old code around the error line
				oldCode := f.extractOldCodeFromFile(filePath, lineNum)

				// Validate that the AI response doesn't contain placeholder code
				if f.containsPlaceholderCode(analysis.Fix) {
					// Skip this fix - it contains placeholders
					continue
				}

				// Try to extract new code from AI response
				newCode := f.extractNewCodeForFile(analysis.Fix, filepath.Base(filePath))

				fixes = append(fixes, Fix{
					FilePath:    filePath,
					OldCode:     oldCode,
					NewCode:     newCode,
					LineNumbers: []int{lineNum},
					Description: analysis.Explanation,
				})
			}
		}

		// Return fixes if we found any
		if len(fixes) > 0 {
			return fixes, nil
		}
	}

	// For test failures, extract file path from test output (format: file_test.go:line: message)
	// Pattern: "    redis_queue_test.go:59: Expected payload..."
	testFileRegex := regexp.MustCompile(`\s+([^\s]+_test\.go):(\d+):`)
	testFileMatches := testFileRegex.FindAllStringSubmatch(testFailure, -1)

	if len(testFileMatches) > 0 {
		// Extract the first test file mentioned
		testFilePath := testFileMatches[0][1]
		lineNum, _ := strconv.Atoi(testFileMatches[0][2])

		// Try to find the full path - check common locations
		possiblePaths := []string{
			filepath.Join(f.projectRoot, "internal", "queue", testFilePath),
			filepath.Join(f.projectRoot, testFilePath),
			testFilePath,
		}

		var actualPath string
		for _, path := range possiblePaths {
			if _, err := os.Stat(path); err == nil {
				actualPath = path
				break
			}
		}

		if actualPath == "" {
			// If file not found, try to construct from package path in test failure
			// Look for package path like "github.com/the-hive/internal/queue"
			pkgRegex := regexp.MustCompile(`github\.com/the-hive/([^\s]+)`)
			pkgMatches := pkgRegex.FindAllStringSubmatch(testFailure, -1)
			if len(pkgMatches) > 0 {
				pkgPath := strings.ReplaceAll(pkgMatches[0][1], "/", string(filepath.Separator))
				actualPath = filepath.Join(f.projectRoot, pkgPath, testFilePath)
			}
		}

		if actualPath != "" {
			// Read the file to get the old code around the error line
			oldCode := f.extractOldCodeFromFile(actualPath, lineNum)

			// Try to extract new code from AI response
			newCode := f.extractNewCodeForFile(analysis.Fix, filepath.Base(actualPath))

			// If no code block found, try to extract from the explanation
			if newCode == "" && analysis.Fix != "" {
				// Look for code snippets in the fix text
				newCode = f.extractCodeFromText(analysis.Fix, oldCode)
			}

			// If still no new code, try to infer from the error message and old code
			if newCode == "" && oldCode != "" {
				newCode = f.inferFixFromError(oldCode, testFailure, lineNum)
			}

			// Always create a fix if we have a file path and line number, even if newCode is empty
			// The applier can handle line-number-based fixes
			if actualPath != "" {
				fixes = append(fixes, Fix{
					FilePath:    actualPath,
					OldCode:     oldCode,
					NewCode:     newCode,
					LineNumbers: []int{lineNum},
					Description: analysis.Explanation,
				})
			}
		}

		// Return fixes if we found any
		if len(fixes) > 0 {
			return fixes, nil
		}
	}

	// Try to extract file path and code changes from the fix text
	// Look for code blocks with file paths
	codeBlockRegex := regexp.MustCompile("(?s)```(?:go|)?\\s*(?:file:)?([^\\n]+)?\\n(.*?)```")
	matches := codeBlockRegex.FindAllStringSubmatch(analysis.Fix, -1)

	if len(matches) == 0 {
		// Try to find file references in the text
		fileRefRegex := regexp.MustCompile(`(?:file|File|FILE):\s*([^\s\n]+\.go)`)
		fileMatches := fileRefRegex.FindAllStringSubmatch(analysis.Fix, -1)

		if len(fileMatches) > 0 {
			filePath := fileMatches[0][1]
			// Validate it's a reasonable file path
			if strings.Contains(filePath, "/") && strings.HasSuffix(filePath, ".go") {
				fixes = append(fixes, Fix{
					FilePath:    filePath,
					NewCode:     analysis.Fix,
					Description: analysis.Explanation,
				})
			}
		}
		// If no valid fixes found, return empty (don't create invalid fixes)
		return fixes, nil
	}

	// Process each code block
	for _, match := range matches {
		fileHint := strings.TrimSpace(match[1])
		codeContent := strings.TrimSpace(match[2])

		// Try to determine file path
		filePath := f.determineFilePath(fileHint, testFailure)

		if filePath != "" {
			fixes = append(fixes, Fix{
				FilePath:    filePath,
				NewCode:     codeContent,
				Description: analysis.Explanation,
			})
		}
	}

	return fixes, nil
}

// extractFilePathsFromCompilationErrors extracts file paths from compilation error output
func (f *Fixer) extractFilePathsFromCompilationErrors(errorOutput string) []string {
	var filePaths []string
	seen := make(map[string]bool)

	// Pattern: path/to/file.go:line:col: error message
	filePathRegex := regexp.MustCompile(`([^\s]+\.go):\d+:\d+:`)
	matches := filePathRegex.FindAllStringSubmatch(errorOutput, -1)

	for _, match := range matches {
		if len(match) > 1 {
			filePath := match[1]
			// Make absolute if relative
			if !filepath.IsAbs(filePath) {
				filePath = filepath.Join(f.projectRoot, filePath)
			}
			// Deduplicate
			if !seen[filePath] {
				filePaths = append(filePaths, filePath)
				seen[filePath] = true
			}
		}
	}

	return filePaths
}

// determineFilePath tries to figure out which file to modify
func (f *Fixer) determineFilePath(hint, testFailure string) string {
	// If hint contains a path, use it
	if hint != "" && (strings.Contains(hint, "/") || strings.Contains(hint, ".go")) {
		// Clean up the hint
		hint = strings.Trim(hint, "`\"'")
		if !strings.HasPrefix(hint, "/") && !strings.HasPrefix(hint, "./") {
			hint = "./" + hint
		}
		return filepath.Join(f.projectRoot, strings.TrimPrefix(hint, "./"))
	}

	// Try to extract from test failure
	// Look for file paths in stack traces
	fileRegex := regexp.MustCompile(`([^\s]+\.go):\d+`)
	matches := fileRegex.FindAllStringSubmatch(testFailure, -1)
	if len(matches) > 0 {
		filePath := matches[0][1]
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(f.projectRoot, filePath)
		}
		return filePath
	}

	return ""
}

// extractOldCodeFromFile reads a file and extracts code around a specific line
func (f *Fixer) extractOldCodeFromFile(filePath string, lineNum int) string {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "" // Can't read file, will skip old code requirement
	}

	lines := strings.Split(string(content), "\n")
	if lineNum < 1 || lineNum > len(lines) {
		return ""
	}

	// Extract a few lines around the error (context window)
	// Use a wider window for better context
	start := max(0, lineNum-5)        // 5 lines before
	end := min(len(lines), lineNum+5) // 5 lines after

	context := strings.Join(lines[start:end], "\n")
	return context
}

// extractNewCodeForFile tries to extract new code from AI response for a specific file
func (f *Fixer) extractNewCodeForFile(aiResponse, fileName string) string {
	// Try to find code blocks
	codeBlockRegex := regexp.MustCompile("(?s)```(?:go|)?\\s*(?:file:)?([^\\n]+)?\\n(.*?)```")
	matches := codeBlockRegex.FindAllStringSubmatch(aiResponse, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			codeContent := strings.TrimSpace(match[2])
			// If code block mentions the file name, use it
			if strings.Contains(codeContent, fileName) ||
				strings.Contains(match[1], fileName) ||
				strings.Contains(aiResponse, fileName) {
				return codeContent
			}
		}
	}

	// If no code block found, try to extract from text mentioning the file
	if idx := strings.Index(aiResponse, fileName); idx > 0 {
		// Extract surrounding text that might contain code
		start := max(0, idx-300)
		end := min(len(aiResponse), idx+1000)
		extracted := aiResponse[start:end]

		// Look for code-like patterns
		if strings.Contains(extracted, "func ") ||
			strings.Contains(extracted, "if ") ||
			strings.Contains(extracted, "return ") {
			return extracted
		}
	}

	return ""
}

// extractCodeFromText tries to extract code from plain text that might contain code snippets
func (f *Fixer) extractCodeFromText(text, oldCode string) string {
	// REJECT explanatory text - this is the main source of corruption
	rejectPatterns := []string{
		"To ensure that",
		"Assuming the",
		"Here's the",
		"Here is the",
		"This fix",
		"The fix",
		"Explanation:",
		"Root Cause:",
	}

	firstLine := strings.TrimSpace(strings.Split(text, "\n")[0])
	for _, pattern := range rejectPatterns {
		if strings.HasPrefix(firstLine, pattern) || strings.Contains(firstLine, pattern) {
			log.Printf("⚠️  Rejecting fix: Contains explanatory text pattern '%s'", pattern)
			return "" // Reject explanatory text
		}
	}

	// If oldCode is provided, try to find a similar pattern in the text
	if oldCode != "" {
		// Look for lines that might be replacements
		oldLines := strings.Split(oldCode, "\n")
		if len(oldLines) > 0 {
			// Try to find the key line (usually the one with the error)
			keyLine := strings.TrimSpace(oldLines[len(oldLines)/2])

			// Look for similar patterns in the text
			if strings.Contains(text, keyLine) {
				// Extract surrounding context
				idx := strings.Index(text, keyLine)
				start := max(0, idx-200)
				end := min(len(text), idx+len(keyLine)+300)
				extracted := text[start:end]

				// Try to extract just the code-like parts
				lines := strings.Split(extracted, "\n")
				var codeLines []string
				for _, line := range lines {
					trimmed := strings.TrimSpace(line)
					// Look for lines that look like code
					if strings.Contains(trimmed, "if ") ||
						strings.Contains(trimmed, "return ") ||
						strings.Contains(trimmed, "=") ||
						strings.Contains(trimmed, ":=") ||
						strings.Contains(trimmed, "func ") {
						codeLines = append(codeLines, trimmed)
					}
				}
				if len(codeLines) > 0 {
					return strings.Join(codeLines, "\n")
				}
			}
		}
	}

	// Fallback: look for any code-like patterns
	lines := strings.Split(text, "\n")
	var codeLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip markdown code fences and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "```") {
			continue
		}
		// Look for Go code patterns
		if strings.Contains(trimmed, "t.Errorf") ||
			strings.Contains(trimmed, "Expected") ||
			strings.Contains(trimmed, "got") {
			codeLines = append(codeLines, trimmed)
		}
	}

	if len(codeLines) > 0 {
		return strings.Join(codeLines, "\n")
	}

	return ""
}

// inferFixFromError tries to infer a fix from the error message and old code
func (f *Fixer) inferFixFromError(oldCode, errorMsg string, lineNum int) string {
	// Look for common error patterns and infer fixes
	lines := strings.Split(oldCode, "\n")
	if len(lines) == 0 {
		return ""
	}

	// Find the line with the error (usually the middle line)
	errorLine := ""
	if len(lines) > 0 {
		errorLine = strings.TrimSpace(lines[len(lines)/2])
	}

	// Pattern 1: JSON formatting mismatch (spaces vs no spaces)
	// Error: "Expected payload {"test": "data"}, got {"test":"data"}"
	if strings.Contains(errorMsg, "Expected") && strings.Contains(errorMsg, "got") &&
		(strings.Contains(errorMsg, "payload") || strings.Contains(errorMsg, "Payload")) {
		// Look for string comparison of JSON payloads
		if strings.Contains(errorLine, "string(") &&
			(strings.Contains(errorLine, "Payload") || strings.Contains(errorLine, "payload")) {
			// Generate a fix that uses JSON unmarshaling for comparison
			// Find the comparison block
			oldLines := strings.Split(oldCode, "\n")
			var newLines []string
			for i, line := range oldLines {
				if strings.Contains(line, "string(") &&
					(strings.Contains(line, "Payload") || strings.Contains(line, "payload")) {
					// Replace with JSON comparison
					newLines = append(newLines, "\t// Compare JSON payloads by normalizing whitespace")
					newLines = append(newLines, "\tvar expectedPayload, actualPayload map[string]interface{}")
					newLines = append(newLines, "\tif err := json.Unmarshal(job.Payload, &expectedPayload); err != nil {")
					newLines = append(newLines, "\t\tt.Fatalf(\"Failed to unmarshal expected payload: %v\", err)")
					newLines = append(newLines, "\t}")
					newLines = append(newLines, "\tif err := json.Unmarshal(dequeued.Payload, &actualPayload); err != nil {")
					newLines = append(newLines, "\t\tt.Fatalf(\"Failed to unmarshal actual payload: %v\", err)")
					newLines = append(newLines, "\t}")
					newLines = append(newLines, "\texpectedJSON, _ := json.Marshal(expectedPayload)")
					newLines = append(newLines, "\tactualJSON, _ := json.Marshal(actualPayload)")
					newLines = append(newLines, "\tif string(expectedJSON) != string(actualJSON) {")
					newLines = append(newLines, "\t\tt.Errorf(\"Expected payload %s, got %s\", string(expectedJSON), string(actualJSON))")
					newLines = append(newLines, "\t}")
					// Skip the old lines that we're replacing
					skipCount := 0
					for j := i; j < len(oldLines) && skipCount < 2; j++ {
						if strings.Contains(oldLines[j], "t.Errorf") {
							skipCount++
						}
					}
					break
				} else {
					newLines = append(newLines, line)
				}
			}
			if len(newLines) > 0 {
				return strings.Join(newLines, "\n")
			}
		}
	}

	// Pattern 2: Try to find the actual line that needs fixing
	// Look for the line that's being tested
	for _, line := range lines {
		if strings.Contains(line, "if ") && strings.Contains(line, "!=") {
			// This might be the comparison line - try to fix it
			if strings.Contains(errorMsg, "Expected") {
				// Maybe we need to normalize the comparison
				return line
			}
		}
	}

	return ""
}

// containsPlaceholderCode checks if the fix contains placeholder code that shouldn't be used
func (f *Fixer) containsPlaceholderCode(fix string) bool {
	placeholders := []string{
		"someFunction",
		"someVariable",
		"exampleFunction",
		"exampleCode",
		"yourFunction",
		"yourVariable",
		"TODO",
		"FIXME",
		"// ...",
		"// ... more code ...",
	}

	lowerFix := strings.ToLower(fix)
	for _, placeholder := range placeholders {
		if strings.Contains(lowerFix, strings.ToLower(placeholder)) {
			return true
		}
	}

	return false
}

// findHTMLFilesFromError tries to find HTML files mentioned in the error or analysis
func (f *Fixer) findHTMLFilesFromError(errorMsg, analysis string) []string {
	var files []string

	// Common template directories
	templateDirs := []string{
		filepath.Join(f.projectRoot, "internal", "server", "templates"),
		filepath.Join(f.projectRoot, "frontend", "template"),
		filepath.Join(f.projectRoot, "web", "templates"),
	}

	// Extract HTML file names from error message (e.g., "analyst.html:307")
	htmlFileRegex := regexp.MustCompile(`([^\s]+\.html):(\d+)`)
	matches := htmlFileRegex.FindAllStringSubmatch(errorMsg, -1)

	for _, match := range matches {
		if len(match) >= 2 {
			htmlFileName := match[1]
			// Try to find the file in template directories
			for _, dir := range templateDirs {
				filePath := filepath.Join(dir, htmlFileName)
				if _, err := os.Stat(filePath); err == nil {
					files = append(files, filePath)
					break
				}
			}
		}
	}

	// If no files found from error, check analysis for file mentions
	if len(files) == 0 {
		// Look for HTML file names in the analysis text
		htmlNameRegex := regexp.MustCompile(`([a-z_]+\.html)`)
		nameMatches := htmlNameRegex.FindAllStringSubmatch(analysis+errorMsg, -1)
		for _, match := range nameMatches {
			if len(match) >= 2 {
				htmlFileName := match[1]
				for _, dir := range templateDirs {
					filePath := filepath.Join(dir, htmlFileName)
					if _, err := os.Stat(filePath); err == nil {
						files = append(files, filePath)
						break
					}
				}
			}
		}
	}

	// If still no files, return common template files that might be relevant
	if len(files) == 0 {
		commonFiles := []string{"base.html", "index.html", "settings.html", "login.html", "analyst.html", "super_admin.html"}
		for _, dir := range templateDirs {
			for _, name := range commonFiles {
				filePath := filepath.Join(dir, name)
				if _, err := os.Stat(filePath); err == nil {
					files = append(files, filePath)
				}
			}
		}
	}

	return files
}

// extractLineNumberFromError extracts line number from error message
func (f *Fixer) extractLineNumberFromError(errorMsg, filePath string) int {
	fileName := filepath.Base(filePath)
	// Look for pattern like "analyst.html:307"
	regex := regexp.MustCompile(regexp.QuoteMeta(fileName) + `:(\d+)`)
	matches := regex.FindStringSubmatch(errorMsg)
	if len(matches) >= 2 {
		if lineNum, err := strconv.Atoi(matches[1]); err == nil {
			return lineNum
		}
	}
	return 0
}

// findGoFilesFromError tries to find Go handler/middleware files mentioned in the error or analysis
func (f *Fixer) findGoFilesFromError(errorMsg, analysis string) []string {
	var files []string

	// Common handler/middleware directories
	handlerDirs := []string{
		filepath.Join(f.projectRoot, "internal", "server", "middleware"),
		filepath.Join(f.projectRoot, "internal", "server"),
		filepath.Join(f.projectRoot, "internal", "middleware"),
	}

	// Extract API endpoint from error (e.g., "/api/v1/keys")
	endpointRegex := regexp.MustCompile(`/api/v[0-9]+/[a-z-]+`)
	endpoints := endpointRegex.FindAllString(errorMsg+analysis, -1)

	// Look for handler files
	for _, dir := range handlerDirs {
		if _, err := os.Stat(dir); err != nil {
			continue
		}

		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			// Check if file name suggests it's a handler or middleware
			fileName := filepath.Base(path)
			if strings.Contains(fileName, "handler") ||
				strings.Contains(fileName, "middleware") ||
				strings.Contains(fileName, "auth") {
				files = append(files, path)
			}

			// Also check file content for endpoint mentions
			content, err := os.ReadFile(path)
			if err == nil {
				contentStr := string(content)
				for _, endpoint := range endpoints {
					if strings.Contains(contentStr, endpoint) {
						files = append(files, path)
						break
					}
				}
				// Also check for common patterns that cause this issue
				if strings.Contains(contentStr, "http.Redirect") &&
					strings.Contains(contentStr, "RequireLogin") {
					files = append(files, path)
				}
			}

			return nil
		})

		if err != nil {
			continue
		}
	}

	return files
}

// extractCodeFromFullResponse tries to extract code from the full AI response when no code blocks are found
func (f *Fixer) extractCodeFromFullResponse(response, oldCode, fileName string) string {
	// REJECT explanatory text that starts with "To ensure", "Assuming", "Here's", etc.
	// These are AI explanations, not code
	rejectPatterns := []string{
		"To ensure that",
		"Assuming the",
		"Here's the",
		"Here is the",
		"This fix",
		"The fix",
		"Explanation:",
		"Root Cause:",
		"we need to",
		"we should",
		"you need to",
	}

	firstLine := strings.TrimSpace(strings.Split(response, "\n")[0])
	for _, pattern := range rejectPatterns {
		if strings.HasPrefix(firstLine, pattern) || strings.Contains(firstLine, pattern) {
			log.Printf("⚠️  Rejecting fix: Contains explanatory text pattern '%s'", pattern)
			return "" // Reject explanatory text
		}
	}

	// Look for function definitions or code snippets
	lines := strings.Split(response, "\n")
	var codeLines []string
	inCodeBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip lines that are clearly explanatory text
		if strings.HasPrefix(trimmed, "To ") ||
			strings.HasPrefix(trimmed, "This ") ||
			strings.HasPrefix(trimmed, "The ") ||
			strings.HasPrefix(trimmed, "Here") ||
			strings.HasPrefix(trimmed, "Assuming") {
			continue
		}

		// Look for JavaScript/HTML code patterns
		if strings.Contains(trimmed, "async function") ||
			strings.Contains(trimmed, "function ") ||
			strings.Contains(trimmed, "const ") ||
			strings.Contains(trimmed, "let ") ||
			strings.Contains(trimmed, "var ") ||
			strings.Contains(trimmed, "fetch(") ||
			strings.Contains(trimmed, "JSON.parse") ||
			strings.Contains(trimmed, "response.") ||
			strings.Contains(trimmed, "document.") {
			inCodeBlock = true
			codeLines = append(codeLines, line)
		} else if inCodeBlock {
			// Continue collecting code until we hit a blank line or non-code
			if trimmed == "" && len(codeLines) > 5 {
				break
			}
			if trimmed != "" {
				codeLines = append(codeLines, line)
			}
		}
	}

	if len(codeLines) > 0 {
		return strings.Join(codeLines, "\n")
	}

	return ""
}
