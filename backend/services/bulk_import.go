package services

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"
)

// BulkImportResult holds the outcome of a batch import operation.
type BulkImportResult struct {
	Summary struct {
		Total   int `json:"total"`
		Created int `json:"created"`
		Updated int `json:"updated"`
		Failed  int `json:"failed"`
	} `json:"summary"`
	Details []BulkImportDetail `json:"details"`
}

// BulkImportDetail records the result for a single row.
type BulkImportDetail struct {
	Row    int    `json:"row"`
	Key    string `json:"key"`    // email for accounts, code for orgs
	Action string `json:"action"` // created, updated, skipped, failed
	Reason string `json:"reason,omitempty"`
}

// CSVRecord represents a single parsed CSV row with column access.
type CSVRecord struct {
	RowNum int
	Fields map[string]string
}

// ParseCSV parses a CSV reader into headers and records, skipping UTF-8 BOM.
func ParseCSV(r io.Reader) ([]string, []CSVRecord, error) {
	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true

	allRows, err := reader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CSV: %w", err)
	}
	if len(allRows) < 2 {
		return nil, nil, fmt.Errorf("CSV file is empty or has no data rows")
	}

	headers := allRows[0]
	if len(headers) > 0 {
		headers[0] = strings.TrimPrefix(headers[0], "\ufeff")
	}

	// Normalize headers to lowercase with underscores
	normalizedHeaders := make([]string, len(headers))
	for i, h := range headers {
		normalizedHeaders[i] = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(h, " ", "_")))
	}

	var records []CSVRecord
	for i, row := range allRows[1:] {
		if isEmptyRow(row) {
			continue
		}
		fields := make(map[string]string)
		for j, val := range row {
			if j >= len(normalizedHeaders) {
				break
			}
			fields[normalizedHeaders[j]] = strings.TrimSpace(val)
		}
		records = append(records, CSVRecord{
			RowNum: i + 2, // 1-indexed header, so data starts at row 2
			Fields: fields,
		})
	}

	return normalizedHeaders, records, nil
}

// isEmptyRow checks if all cells in a row are empty.
func isEmptyRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

// DeduplicateRecords keeps the last occurrence when duplicate keys exist.
func DeduplicateRecords(records []CSVRecord, keyField string) []CSVRecord {
	seen := make(map[string]int) // key -> last index
	for i, r := range records {
		key := strings.TrimSpace(r.Fields[keyField])
		if key != "" {
			seen[key] = i
		}
	}

	var result []CSVRecord
	for i, r := range records {
		key := strings.TrimSpace(r.Fields[keyField])
		if key == "" {
			result = append(result, r)
			continue
		}
		if seen[key] == i {
			result = append(result, r)
		}
	}
	return result
}

// ValidateRequired checks that all required fields are present in a record.
func ValidateRequired(record CSVRecord, required []string) []string {
	var errs []string
	for _, field := range required {
		if strings.TrimSpace(record.Fields[field]) == "" {
			errs = append(errs, fmt.Sprintf("missing required field: %s", field))
		}
	}
	return errs
}

// ValidateEmail performs a basic email format check.
func ValidateEmail(email string) error {
	if email == "" {
		return fmt.Errorf("email is empty")
	}
	if !strings.Contains(email, "@") || !strings.Contains(email, ".") {
		return fmt.Errorf("invalid email format: %s", email)
	}
	return nil
}

// SplitTags splits a pipe-separated tag string into a slice.
func SplitTags(tags string) []string {
	if tags == "" {
		return []string{}
	}
	parts := strings.Split(tags, "|")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// GenerateImportReport creates a summary report from import results.
func GenerateImportReport(result *BulkImportResult) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("Bulk Import Report — %s", time.Now().Format("2006-01-02 15:04:05")))
	parts = append(parts, fmt.Sprintf("Total: %d | Created: %d | Updated: %d | Failed: %d",
		result.Summary.Total, result.Summary.Created, result.Summary.Updated, result.Summary.Failed))
	if result.Summary.Failed > 0 {
		parts = append(parts, "Failed rows:")
		for _, d := range result.Details {
			if d.Action == "failed" {
				parts = append(parts, fmt.Sprintf("  Row %d (%s): %s", d.Row, d.Key, d.Reason))
			}
		}
	}
	return strings.Join(parts, "\n")
}

// WriteCSVTemplate writes a CSV template with headers and sample rows to a writer.
func WriteCSVTemplate(w io.Writer, headers []string, sampleRows [][]string) error {
	// Write UTF-8 BOM for Excel compatibility
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return fmt.Errorf("failed to write BOM: %w", err)
	}

	writer := csv.NewWriter(w)
	if err := writer.Write(headers); err != nil {
		return fmt.Errorf("failed to write headers: %w", err)
	}
	for _, row := range sampleRows {
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write sample row: %w", err)
		}
	}
	writer.Flush()
	return writer.Error()
}

// SafeAtoi parses an integer with a default fallback.
func SafeAtoi(s string, defaultValue int) int {
	if s == "" {
		return defaultValue
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultValue
	}
	return v
}

// LogImportError logs an import error with context.
func LogImportError(entityType string, row int, key string, err error) {
	log.Printf("[BulkImport][%s] Row %d (key=%s): %v", entityType, row, key, err)
}
