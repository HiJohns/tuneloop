package handlers

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"tuneloop-backend/middleware"

	"github.com/gin-gonic/gin"
)

// BatchImportInstruments handles ZIP upload and CSV parsing for instrument batch import
func BatchImportInstruments(c *gin.Context) {
	// Get tenant ID from context
	tenantID := middleware.GetTenantID(c.Request.Context())
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Tenant ID is required",
		})
		return
	}

	// Parse multipart form for ZIP file
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "File upload failed: " + err.Error(),
		})
		return
	}
	defer file.Close()

	// Validate file type
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".zip") {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40003,
			"message": "Only ZIP files (.zip) are supported",
		})
		return
	}

	// Read ZIP file
	zipReader, err := zip.NewReader(file.(io.ReaderAt), header.Size)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40004,
			"message": "Failed to read ZIP file: " + err.Error(),
		})
		return
	}

	// Parse ZIP and extract data
	csvData, imageDirs, errors := parseZIPArchive(zipReader)
	if len(errors) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40005,
			"message": "ZIP parsing errors",
			"errors":  errors,
		})
		return
	}

	// Return parsed data
	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"instruments": csvData,
			"images":      imageDirs,
			"count":       len(csvData),
		},
	})
}

// parseZIPArchive parses a ZIP file and returns CSV data and image directories
func parseZIPArchive(zipReader *zip.Reader) ([]map[string]interface{}, []string, []string) {
	var csvData []map[string]interface{}
	var imageDirs []string
	var errors []string
	var csvFound bool

	// Map to track directories that contain images
	imageDirMap := make(map[string]bool)

	for _, file := range zipReader.File {
		// Check for CSV files
		if strings.HasSuffix(strings.ToLower(file.Name), ".csv") {
			csvFound = true
			data, err := parseCSVFromZIP(file)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Failed to parse CSV %s: %v", file.Name, err))
				continue
			}
			csvData = append(csvData, data...)
		}

		// Track image directories
		if isImageFile(file.Name) {
			dir := filepath.Dir(file.Name)
			if dir != "." {
				imageDirMap[dir] = true
			}
		}
	}

	// Convert image directory map to slice
	for dir := range imageDirMap {
		imageDirs = append(imageDirs, dir)
	}

	if !csvFound {
		errors = append(errors, "No CSV file found in ZIP archive")
	}

	return csvData, imageDirs, errors
}

// parseCSVFromZIP reads and parses a CSV file from ZIP
func parseCSVFromZIP(zipFile *zip.File) ([]map[string]interface{}, error) {
	csvReader, err := zipFile.Open()
	if err != nil {
		return nil, err
	}
	defer csvReader.Close()

	reader := csv.NewReader(csvReader)

	// Read header row
	headers, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV headers: %w", err)
	}

	var data []map[string]interface{}
	lineNum := 1 // Start from 1 after header

	for {
		lineNum++
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading line %d: %w", lineNum, err)
		}

		// Skip empty rows
		isEmpty := true
		for _, field := range record {
			if strings.TrimSpace(field) != "" {
				isEmpty = false
				break
			}
		}
		if isEmpty {
			continue
		}

		row := make(map[string]interface{})
		metadata := make(map[string]interface{})

		for i, header := range headers {
			if i < len(record) {
				value := strings.TrimSpace(record[i])

				// Core columns
				switch strings.ToLower(header) {
				case "sn", "识别码", "serial_number":
					row["sn"] = value
				case "category_name", "分类名称", "类别":
					row["category_name"] = value
				case "site_name", "网点名称", "存放网点":
					row["site_name"] = value
				case "level_name", "级别名称", "等级":
					row["level_name"] = value
				case "description", "描述", "备注":
					row["description"] = value
				default:
					// Non-core columns go to metadata
					if value != "" {
						metadata[header] = value
					}
				}
			}
		}

		// Add metadata if not empty
		if len(metadata) > 0 {
			row["metadata"] = metadata
		}

		data = append(data, row)
	}

	return data, nil
}

// isImageFile checks if a file is an image
func isImageFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	imageExts := []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp"}
	for _, imgExt := range imageExts {
		if ext == imgExt {
			return true
		}
	}
	return false
}
