package handlers

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

type UploadPhotosRequest struct {
	BatchType string                  `json:"batch_type" binding:"required,oneof=outbound return maintenance"`
	Photos    []*multipart.FileHeader `form:"photos" binding:"required,min=1"`
}

type PhotoBatchResponse struct {
	BatchID      string    `json:"batch_id"`
	InstrumentID string    `json:"instrument_id"`
	BatchType    string    `json:"batch_type"`
	StoragePath  string    `json:"storage_path"`
	PhotoCount   int       `json:"photo_count"`
	CreatedAt    time.Time `json:"created_at"`
}

// UploadInstrumentPhotos handles batch photo upload for instruments
// POST /api/instruments/:id/photos/upload
func UploadInstrumentPhotos(c *gin.Context) {
	instrumentID := c.Param("id")
	if instrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "instrument id is required",
		})
		return
	}

	// Parse form (max 50MB)
	if err := c.Request.ParseMultipartForm(50 << 20); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "failed to parse multipart form: " + err.Error(),
		})
		return
	}

	batchType := c.Request.FormValue("batch_type")
	if batchType == "" {
		batchType = "outbound" // default
	}

	// Validate batch type
	validTypes := map[string]bool{"outbound": true, "return": true, "maintenance": true}
	if !validTypes[batchType] {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40003,
			"message": "invalid batch_type. Valid: outbound, return, maintenance",
		})
		return
	}

	// Get instrument details
	db := database.GetDB().WithContext(c.Request.Context())
	var instrument models.Instrument
	if err := db.Preload("Tenant").Preload("Category").First(&instrument, "id = ?", instrumentID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    40400,
				"message": "instrument not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to fetch instrument: " + err.Error(),
		})
		return
	}

	// Get uploaded files
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "failed to get multipart form: " + err.Error(),
		})
		return
	}

	files := form.File["photos"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40004,
			"message": "no photos uploaded",
		})
		return
	}

	// Generate batch ID and timestamp
	batchID := uuid.New().String()
	timestamp := time.Now().Format("20060102_150405")
	batchDir := fmt.Sprintf("batch_%s", timestamp)
	
	// Create directory structure: uploads/photos/{tenant_id}/{instrument_sn}/{batch_dir}/
	tenantID := instrument.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	instrumentSN := instrument.SN
	if instrumentSN == "" {
		instrumentSN = "unknown_sn"
	}

	photoBaseDir := filepath.Join(".", "uploads", "photos", tenantID, instrumentSN)
	batchPath := filepath.Join(photoBaseDir, batchDir)
	
	if err := os.MkdirAll(batchPath, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50001,
			"message": "failed to create photo directory: " + err.Error(),
		})
		return
	}

	// Save photos and collect metadata
	type photoMetadata struct {
		Filename  string    `json:"filename"`
		Position  string    `json:"position"`
		Timestamp time.Time `json:"timestamp"`
		Size      int64     `json:"size"`
	}
	
	var photos []photoMetadata
	
	for _, file := range files {
		// Save file
		src, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50002,
				"message": "failed to open uploaded file: " + err.Error(),
			})
			return
		}
		defer src.Close()

		dstPath := filepath.Join(batchPath, file.Filename)
		dst, err := os.Create(dstPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50003,
				"message": "failed to create destination file: " + err.Error(),
			})
			return
		}
		defer dst.Close()

		if _, err := io.Copy(dst, src); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50004,
				"message": "failed to save photo: " + err.Error(),
			})
			return
		}

		photos = append(photos, photoMetadata{
			Filename:  file.Filename,
			Position:  strings.TrimSuffix(file.Filename, filepath.Ext(file.Filename)),
			Timestamp: time.Now(),
			Size:      file.Size,
		})
	}

	// Create manifest.yaml
	manifest := map[string]interface{}{
		"version":       "1.0",
		"batch_id":      batchID,
		"instrument_id": instrumentID,
		"instrument_sn": instrumentSN,
		"batch_type":    batchType,
		"operator_id":   c.GetString("user_id"),
		"tenant_id":     tenantID,
		"created_at":    time.Now().Format(time.RFC3339),
		"photos":        photos,
	}

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50005,
			"message": "failed to generate manifest: " + err.Error(),
		})
		return
	}

	manifestPath := filepath.Join(batchPath, "manifest.yaml")
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50006,
			"message": "failed to write manifest: " + err.Error(),
		})
		return
	}

	// Create ZIP archive
	zipPath := filepath.Join(photoBaseDir, fmt.Sprintf("batch_%s.zip", timestamp))
	zipFile, err := os.Create(zipPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50007,
			"message": "failed to create zip file: " + err.Error(),
		})
		return
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Add files to ZIP
	filesToZip := []string{}
	err = filepath.Walk(batchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			filesToZip = append(filesToZip, path)
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50008,
			"message": "failed to walk batch directory: " + err.Error(),
		})
		return
	}

	for _, filePath := range filesToZip {
		relPath, _ := filepath.Rel(batchPath, filePath)
		zipFileWriter, err := zipWriter.Create(relPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50009,
				"message": "failed to create zip entry: " + err.Error(),
			})
			return
		}

		fileData, err := os.ReadFile(filePath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50010,
				"message": "failed to read file for zip: " + err.Error(),
			})
			return
		}

		if _, err := zipFileWriter.Write(fileData); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50011,
				"message": "failed to write to zip: " + err.Error(),
			})
			return
		}
	}

	// Save to database
	photoBatch := models.InstrumentPhotoBatch{
		ID:           batchID,
		InstrumentID: instrumentID,
		BatchType:    batchType,
		StoragePath:  zipPath,
		OperatorID:   c.GetString("user_id"),
		CreatedAt:    time.Now(),
	}

	if err := db.Create(&photoBatch).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50012,
			"message": "failed to save photo batch: " + err.Error(),
		})
		return
	}

	// Update latest symlink
	latestDir := filepath.Join(photoBaseDir, "latest")
	os.RemoveAll(latestDir) // Remove existing symlink/dir
	if err := os.Symlink(batchPath, latestDir); err != nil {
		// If symlink fails (Windows), copy files instead
		if err := copyDir(batchPath, latestDir); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50013,
				"message": "failed to update latest directory: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": PhotoBatchResponse{
			BatchID:      batchID,
			InstrumentID: instrumentID,
			BatchType:    batchType,
			StoragePath:  zipPath,
			PhotoCount:   len(files),
			CreatedAt:    photoBatch.CreatedAt,
		},
	})
}

// GetLatestInstrumentPhotos returns the latest photo batch for an instrument
// GET /api/instruments/:id/photos/latest
func GetLatestInstrumentPhotos(c *gin.Context) {
	instrumentID := c.Param("id")
	if instrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "instrument id is required",
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())

	// Get instrument details
	var instrument models.Instrument
	if err := db.Preload("Tenant").First(&instrument, "id = ?", instrumentID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    40400,
				"message": "instrument not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to fetch instrument: " + err.Error(),
		})
		return
	}

	// Determine tenant and instrument paths
	tenantID := instrument.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	instrumentSN := instrument.SN
	if instrumentSN == "" {
		instrumentSN = "unknown_sn"
	}

	// Check if latest symlink exists
	latestDir := filepath.Join(".", "uploads", "photos", tenantID, instrumentSN, "latest")
	if _, err := os.Stat(latestDir); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40401,
			"message": "no photo batches found for this instrument",
		})
		return
	}

	// List files in latest directory (excluding subdirectories)
	var photos []string
	entries, err := os.ReadDir(latestDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to read latest directory: " + err.Error(),
		})
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() && entry.Name() != "manifest.yaml" {
			photos = append(photos, filepath.Join("/uploads/photos", tenantID, instrumentSN, "latest", entry.Name()))
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"instrument_id": instrumentID,
			"instrument_sn": instrumentSN,
			"photos":        photos,
			"count":         len(photos),
		},
	})
}

// Helper function to copy directory (for Windows compatibility)
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(dstPath, data, info.Mode())
	})
}
