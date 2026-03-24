package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type UploadHandler struct {
	uploadDir string
	baseURL   string
}

func NewUploadHandler() *UploadHandler {
	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	
	// Ensure upload directory exists
	os.MkdirAll(uploadDir, 0755)
	
	baseURL := os.Getenv("UPLOAD_BASE_URL")
	if baseURL == "" {
		baseURL = "/uploads"
	}
	
	return &UploadHandler{
		uploadDir: uploadDir,
		baseURL:   baseURL,
	}
}

// HandleUpload - POST /api/upload
func (h *UploadHandler) HandleUpload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "No file uploaded: " + err.Error(),
		})
		return
	}
	
	// Validate file type (only images and videos)
	allowedTypes := []string{
		"image/jpeg", "image/jpg", "image/png", "image/gif",
		"video/mp4", "video/mov", "video/avi",
	}
	
	fileType := file.Header.Get("Content-Type")
	if !contains(allowedTypes, fileType) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "Invalid file type. Only images (JPEG, PNG, GIF) and videos (MP4, MOV, AVI) are allowed",
		})
		return
	}
	
	// Validate file size (max 10MB)
	const maxSize = 10 * 1024 * 1024 // 10MB
	if file.Size > maxSize {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "File too large. Maximum size is 10MB",
		})
		return
	}
	
	// Generate unique filename
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext == "" {
		ext = ".bin"
	}
	uniqueName := fmt.Sprintf("%d_%s%s", time.Now().Unix(), generateRandomString(8), ext)
	savePath := filepath.Join(h.uploadDir, uniqueName)
	
	// Save file
	if err := c.SaveUploadedFile(file, savePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to save file: " + err.Error(),
		})
		return
	}
	
	// Return file info
	fileURL := fmt.Sprintf("%s/%s", strings.TrimRight(h.baseURL, "/"), uniqueName)
	
	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"original_name": file.Filename,
			"saved_name":   uniqueName,
			"url":          fileURL,
			"size":         file.Size,
			"type":         fileType,
			"uploaded_at":  time.Now().Format(time.RFC3339),
		},
	})
}

// Helper functions
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}
