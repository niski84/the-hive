package parser

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// parseExcel extracts text from an Excel file using "markdownification" strategy
func parseExcel(filePath string) (string, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open Excel file: %w", err)
	}
	defer f.Close()

	var builder strings.Builder

	// Get all sheet names
	sheetList := f.GetSheetList()
	if len(sheetList) == 0 {
		return "", fmt.Errorf("no sheets found in Excel file: %s", filePath)
	}

	// Process each sheet
	for sheetIdx, sheetName := range sheetList {
		if sheetIdx > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(fmt.Sprintf("Sheet: %s\n", sheetName))

		// Get all rows
		rows, err := f.GetRows(sheetName)
		if err != nil {
			// Skip this sheet if we can't read it (e.g., password protected)
			builder.WriteString(fmt.Sprintf("(Unable to read sheet %s: %v)\n", sheetName, err))
			continue
		}

		if len(rows) == 0 {
			continue
		}

		// First row is headers
		headers := rows[0]
		if len(headers) == 0 {
			continue
		}

		// Process data rows (skip header row)
		for rowIdx := 1; rowIdx < len(rows); rowIdx++ {
			row := rows[rowIdx]
			
			// Build row text: "Row [X]: [Header 1]: [Value], [Header 2]: [Value]..."
			rowParts := []string{}
			for colIdx, header := range headers {
				if colIdx < len(row) && row[colIdx] != "" {
					value := strings.TrimSpace(row[colIdx])
					if value != "" {
						headerName := strings.TrimSpace(header)
						if headerName == "" {
							headerName = fmt.Sprintf("Column %d", colIdx+1)
						}
						rowParts = append(rowParts, fmt.Sprintf("%s: %s", headerName, value))
					}
				}
			}

			if len(rowParts) > 0 {
				builder.WriteString(fmt.Sprintf("Row %d: %s\n", rowIdx+1, strings.Join(rowParts, ", ")))
			}
		}
	}

	result := strings.TrimSpace(builder.String())
	if result == "" {
		return "", fmt.Errorf("no content extracted from Excel file: %s", filePath)
	}

	return result, nil
}

