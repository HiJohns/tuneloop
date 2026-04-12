package handlers

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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

	// Map names to IDs
	db := database.GetDB()
	mappedInstruments := mapNamesToIDs(csvData, imageDirs, db, tenantID)

	// Return parsed data
	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"instruments": mappedInstruments,
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

// lookupCategoryID finds category ID by name for given tenant
func lookupCategoryID(name string, db *gorm.DB, tenantID string) (string, error) {
	var category struct {
		ID string
	}

	// Search by exact name match in tenant's categories
	err := db.Table("categories").
		Where("name = ? AND tenant_id = ?", name, tenantID).
		Select("id").
		First(&category).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", fmt.Errorf("category not found: %s", name)
		}
		return "", err
	}

	return category.ID, nil
}

// lookupSiteID finds site ID by name for given tenant
func lookupSiteID(name string, db *gorm.DB, tenantID string) (string, error) {
	var site struct {
		ID string
	}

	// Search by exact name match in tenant's sites
	err := db.Table("sites").
		Where("name = ? AND tenant_id = ?", name, tenantID).
		Select("id").
		First(&site).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", fmt.Errorf("site not found: %s", name)
		}
		return "", err
	}

	return site.ID, nil
}

// lookupLevelID finds level ID by caption/name
func lookupLevelID(name string, db *gorm.DB, tenantID string) (string, error) {
	var level struct {
		ID string
	}

	// Search by caption or code in instrument_levels
	err := db.Table("instrument_levels").
		Where("(caption = ? OR code = ?)", name, name).
		Select("id").
		First(&level).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", fmt.Errorf("level not found: %s", name)
		}
		return "", err
	}

	return level.ID, nil
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

// mapNamesToIDs maps category/site/level names to IDs and tracks errors
func mapNamesToIDs(instruments []map[string]interface{}, imageDirs []string, db *gorm.DB, tenantID string) []map[string]interface{} {
	var mapped []map[string]interface{}

	for _, instrument := range instruments {
		mappedInst := make(map[string]interface{})

		// Copy original data
		for k, v := range instrument {
			mappedInst[k] = v
		}

		// Map category name to ID
		if categoryName, ok := instrument["category_name"].(string); ok && categoryName != "" {
			categoryID, err := lookupCategoryID(categoryName, db, tenantID)
			if err != nil {
				mappedInst["_error_category"] = fmt.Sprintf("分类不存在: %s", categoryName)
			} else {
				mappedInst["category_id"] = categoryID
			}
		}

		// Map site name to ID
		if siteName, ok := instrument["site_name"].(string); ok && siteName != "" {
			siteID, err := lookupSiteID(siteName, db, tenantID)
			if err != nil {
				mappedInst["_error_site"] = fmt.Sprintf("网点不存在: %s", siteName)
			} else {
				mappedInst["site_id"] = siteID
			}
		}

		// Map level name to ID
		if levelName, ok := instrument["level_name"].(string); ok && levelName != "" {
			levelID, err := lookupLevelID(levelName, db, tenantID)
			if err != nil {
				mappedInst["_warning_level"] = fmt.Sprintf("级别不存在: %s", levelName)
			} else {
				mappedInst["level_id"] = levelID
			}
		}

		mapped = append(mapped, mappedInst)
	}

	return mapped
}
