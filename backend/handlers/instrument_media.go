package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/image/draw"
	"gorm.io/gorm"
)

var validBatchTypes = map[string]bool{
	"shipping":   true,
	"forwarding": true,
	"accepting":  true,
	"returning":  true,
	"relaying":   true,
	"receiving":  true,
	"repaired":   true,
	"repair":     true,
}

func buildStructuredKey(ctx context.Context, originalKey string, batchType string, seq int) string {
	tenantID := middleware.GetTenantID(ctx)
	orgID := middleware.GetOrgID(ctx)
	if orgID == "" {
		orgID = "_namespace"
	}
	ext := filepath.Ext(originalKey)
	if ext == "" {
		ext = ".bin"
	}
	return fmt.Sprintf("%s/%s/%s_%s_%d_%d%s",
		tenantID, orgID,
		uuid.New().String()[:8], batchType, time.Now().Unix(), seq, ext)
}

func CreateInstrumentMedia(c *gin.Context) {
	instrumentID := c.Param("id")
	if instrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "instrument id is required"})
		return
	}

	var req struct {
		BatchType  string `json:"batch_type"`
		IsDisplay  bool   `json:"is_display"`
		ObjectType string `json:"object_type"`
		ObjectID   string `json:"object_id"`
		Files      []struct {
			FileKey   string `json:"file_key"`
			FileType  string `json:"file_type"`
			SortOrder int    `json:"sort_order"`
		} `json:"files"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request: " + err.Error()})
		return
	}

	if !validBatchTypes[req.BatchType] {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "invalid batch_type: " + req.BatchType})
		return
	}

	if len(req.Files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "message": "at least one file is required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)
	storage := services.MediaStorageFromContext(c)

	var instrument models.Instrument
	if err := db.Where("id = ? AND tenant_id = ?", instrumentID, tenantID).First(&instrument).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "instrument not found"})
		return
	}

	batchID := uuid.New().String()

	tx := db.Begin()

	if req.IsDisplay {
		if err := tx.Model(&models.InstrumentMedia{}).
			Where("instrument_id = ? AND tenant_id = ?", instrumentID, tenantID).
			Update("is_display", false).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to reset display"})
			return
		}
	}

	var hasVideo bool
	var videoCount int
	for _, f := range req.Files {
		if f.FileType == "video" {
			videoCount++
		}
	}
	if videoCount > 1 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40005, "message": "at most 1 video per batch"})
		return
	}
	for _, f := range req.Files {
		if f.FileType == "video" {
			hasVideo = true
			var existing models.InstrumentMedia
			if err := tx.Where("instrument_id = ? AND tenant_id = ? AND file_type = 'video'", instrumentID, tenantID).
				First(&existing).Error; err == nil {
				if existing.StorageKey != "" {
					if err := storage.Delete(ctx, existing.StorageKey); err != nil {
						log.Printf("[InstrumentMedia] Failed to delete old video file: %v", err)
					}
					thumbKey := strings.TrimSuffix(existing.StorageKey, filepath.Ext(existing.StorageKey)) + "_thumb.jpg"
					storage.Delete(ctx, thumbKey)
				}
				tx.Delete(&existing)
			}
			var existingThumb models.InstrumentMedia
			if err := tx.Where("instrument_id = ? AND tenant_id = ? AND file_type = 'video_thumb'", instrumentID, tenantID).
				First(&existingThumb).Error; err == nil {
				tx.Delete(&existingThumb)
			}
		}
	}

	for i, f := range req.Files {
		newKey := buildStructuredKey(ctx, f.FileKey, req.BatchType, i+1)
		if err := storage.Rename(ctx, f.FileKey, newKey); err != nil {
			log.Printf("[InstrumentMedia] Rename failed, using original key: %v", err)
			newKey = f.FileKey
		}
		var objectID *string
		if req.ObjectID != "" {
			objectID = &req.ObjectID
		}
		media := models.InstrumentMedia{
			TenantID:     tenantID,
			OrgID:        middleware.GetOrgID(ctx),
			InstrumentID: &instrumentID,
			ObjectType:   req.ObjectType,
			ObjectID:     objectID,
			BatchID:      batchID,
			BatchType:    req.BatchType,
			FileName:     filepath.Base(f.FileKey),
			FileType:     f.FileType,
			StorageKey:   newKey,
			IsDisplay:    req.IsDisplay,
			SortOrder:    f.SortOrder,
		}
		if err := tx.Create(&media).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create media record"})
			return
		}
	}

	tx.Commit()

	// Generate thumbnails for all image types
	for i, f := range req.Files {
		if f.FileType == "image" {
			newKey := buildStructuredKey(ctx, f.FileKey, req.BatchType, i+1)
			thumbKey := strings.TrimSuffix(newKey, filepath.Ext(newKey)) + "_thumb.jpg"
			srcPath := filepath.Join(".", "uploads", "media", newKey)
			if data, err := os.ReadFile(srcPath); err == nil {
				if thumbData, err := services.GenerateThumbnail(data, 128); err == nil {
					storage.Upload(ctx, thumbKey, bytes.NewReader(thumbData), "image/jpeg")
				}
			}
		}
	}

	if hasVideo {
		go generateVideoThumbnail(c, db, tenantID, instrumentID, batchID, storage)
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"batch_id": batchID}})
}

func generateVideoThumbnail(c *gin.Context, db *gorm.DB, tenantID, instrumentID, batchID string, storage services.MediaStorage) {
	ffmpegPath := "ffmpeg"
	if _, err := exec.LookPath(ffmpegPath); err != nil {
		log.Printf("[InstrumentMedia] FFmpeg not found, skipping thumbnail for batch %s", batchID)
		return
	}

	var video models.InstrumentMedia
	if err := db.Where("instrument_id = ? AND tenant_id = ? AND batch_id = ? AND file_type = 'video'",
		instrumentID, tenantID, batchID).First(&video).Error; err != nil {
		log.Printf("[InstrumentMedia] Video not found for thumbnail: %v", err)
		return
	}

	srcPath := filepath.Join("./uploads/media", video.StorageKey)
	thumbExt := "_thumb.jpg"
	thumbKey := strings.TrimSuffix(video.StorageKey, filepath.Ext(video.StorageKey)) + thumbExt
	dstPath := filepath.Join("./uploads/media", thumbKey)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffmpegPath, "-i", srcPath, "-ss", "00:00:01", "-vframes", "1", "-f", "image2", dstPath)
	if err := cmd.Run(); err != nil {
		log.Printf("[InstrumentMedia] FFmpeg thumbnail failed for %s: %v", video.StorageKey, err)
		return
	}

	thumb := models.InstrumentMedia{
		TenantID:     tenantID,
		OrgID:        video.OrgID,
		InstrumentID: &instrumentID,
		BatchID:      batchID,
		BatchType:    video.BatchType,
		FileName:     filepath.Base(thumbKey),
		FileType:     "video_thumb",
		StorageKey:   thumbKey,
		IsDisplay:    video.IsDisplay,
		SortOrder:    video.SortOrder + 1,
	}
	if err := db.Create(&thumb).Error; err != nil {
		log.Printf("[InstrumentMedia] Failed to save thumbnail record: %v", err)
	}
}

func SetMediaDisplay(c *gin.Context) {
	instrumentID := c.Param("id")
	if instrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "instrument id is required"})
		return
	}

	var req struct {
		BatchID string `json:"batch_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request"})
		return
	}
	if req.BatchID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "batch_id is required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)

	tx := db.Begin()
	if err := tx.Model(&models.InstrumentMedia{}).
		Where("instrument_id = ? AND tenant_id = ?", instrumentID, tenantID).
		Update("is_display", false).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to reset display"})
		return
	}
	if err := tx.Model(&models.InstrumentMedia{}).
		Where("instrument_id = ? AND tenant_id = ? AND batch_id = ?", instrumentID, tenantID, req.BatchID).
		Update("is_display", true).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to set display"})
		return
	}
	tx.Commit()

	// Sync back to Instrument.Images/Video for backward compatibility
	syncBackwardCompat(db, tenantID, instrumentID)

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "display updated"})
}

func DeleteMediaBatch(c *gin.Context) {
	instrumentID := c.Param("id")
	batchID := c.Param("batch_id")
	if instrumentID == "" || batchID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "instrument id and batch id are required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)
	storage := services.MediaStorageFromContext(c)

	var mediaList []models.InstrumentMedia
	if err := db.Where("instrument_id = ? AND tenant_id = ? AND batch_id = ?", instrumentID, tenantID, batchID).
		Find(&mediaList).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query media"})
		return
	}
	if len(mediaList) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "batch not found"})
		return
	}

	for _, m := range mediaList {
		if err := storage.Delete(ctx, m.StorageKey); err != nil {
			log.Printf("[InstrumentMedia] Failed to delete file %s: %v", m.StorageKey, err)
		}
	}

	if err := db.Where("instrument_id = ? AND tenant_id = ? AND batch_id = ?", instrumentID, tenantID, batchID).
		Delete(&models.InstrumentMedia{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to delete media"})
		return
	}

	syncBackwardCompat(db, tenantID, instrumentID)

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "batch deleted"})
}

func GetInstrumentMedia(c *gin.Context) {
	instrumentID := c.Param("id")
	if instrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "instrument id is required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)
	storage := services.MediaStorageFromContext(c)

	var mediaList []models.InstrumentMedia
	if err := db.Where("tenant_id = ? AND (instrument_id = ? OR (object_type = 'instrument' AND object_id = ?))", tenantID, instrumentID, instrumentID).
		Order("sort_order asc, created_at desc").
		Find(&mediaList).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query media"})
		return
	}

	type mediaItem struct {
		BatchID   string `json:"batch_id"`
		BatchType string `json:"batch_type"`
		FileType  string `json:"file_type"`
		URL       string `json:"url"`
		ThumbURL  string `json:"thumb_url,omitempty"`
		SortOrder int    `json:"sort_order"`
		CreatedAt string `json:"created_at"`
	}

	type batchInfo struct {
		BatchID   string `json:"batch_id"`
		BatchType string `json:"batch_type"`
		Count     int    `json:"count"`
		CreatedAt string `json:"created_at"`
	}

	type batchGroup struct {
		BatchID   string      `json:"batch_id"`
		BatchType string      `json:"batch_type"`
		CreatedAt string      `json:"created_at"`
		Items     []mediaItem `json:"items"`
	}

	var displayItems []mediaItem
	var videoItem *mediaItem
	batchesMap := make(map[string]*batchInfo)
	batchGroupsMap := make(map[string]*batchGroup)
	thumbMap := make(map[string]string)

	for _, m := range mediaList {
		url, err := storage.GetURL(ctx, m.StorageKey)
		if err != nil {
			url = "/uploads/media/" + m.StorageKey
		}

		if m.FileType == "video_thumb" {
			thumbMap[m.BatchID] = url
			continue
		}

		item := mediaItem{
			BatchID:   m.BatchID,
			BatchType: m.BatchType,
			FileType:  m.FileType,
			URL:       url,
			SortOrder: m.SortOrder,
			CreatedAt: m.CreatedAt.Format(time.RFC3339),
		}

		if m.IsDisplay && m.FileType != "video" {
			displayItems = append(displayItems, item)
		}

		if m.FileType == "video" {
			videoItem = &item
		}

		if _, ok := batchesMap[m.BatchID]; !ok {
			batchesMap[m.BatchID] = &batchInfo{
				BatchID:   m.BatchID,
				BatchType: m.BatchType,
				CreatedAt: m.CreatedAt.Format(time.RFC3339),
			}
		}
		batchesMap[m.BatchID].Count++

		if m.FileType != "video_thumb" {
			if _, ok := batchGroupsMap[m.BatchID]; !ok {
				batchGroupsMap[m.BatchID] = &batchGroup{
					BatchID:   m.BatchID,
					BatchType: m.BatchType,
					CreatedAt: m.CreatedAt.Format(time.RFC3339),
					Items:     []mediaItem{},
				}
			}
			batchGroupsMap[m.BatchID].Items = append(batchGroupsMap[m.BatchID].Items, item)
		}
	}

	if videoItem != nil {
		if thumbURL, ok := thumbMap[videoItem.BatchID]; ok {
			videoItem.ThumbURL = thumbURL
		}
	}

	var batches []batchInfo
	for _, b := range batchesMap {
		batches = append(batches, *b)
	}

	var groups []batchGroup
	for _, g := range batchGroupsMap {
		groups = append(groups, *g)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"display": displayItems,
			"batches": batches,
			"video":   videoItem,
			"groups":  groups,
		},
	})
}

// UploadDisplayImage uploads/replaces the display image for an instrument
// Resizes to max 1920px width, clears previous display flags
func UploadDisplayImage(c *gin.Context) {
	instrumentID := c.Param("id")
	if instrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "instrument id is required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)

	var instrument models.Instrument
	if err := db.Where("id = ? AND tenant_id = ?", instrumentID, tenantID).First(&instrument).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "instrument not found"})
		return
	}

	file, header, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "image file is required"})
		return
	}
	defer file.Close()

	// Validate file type
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "unsupported image format, use jpg/png/webp"})
		return
	}

	// Decode image
	src, _, err := image.Decode(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "message": "failed to decode image: " + err.Error()})
		return
	}

	// Resize if width > 1920px
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	var final image.Image
	if width > 1920 {
		ratio := float64(1920) / float64(width)
		newWidth := 1920
		newHeight := int(float64(height) * ratio)
		dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
		final = dst
	} else {
		final = src
	}

	// Encode to JPEG buffer
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, final, &jpeg.Options{Quality: 90}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to encode image"})
		return
	}

	// Upload to storage
	storage := services.MediaStorageFromContext(c)
	storageKey := buildStructuredKey(ctx, fmt.Sprintf("%s%s", uuid.New().String()[:8], ".jpg"), "display", 1)
	if err := storage.Upload(ctx, storageKey, &buf, "image/jpeg"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "failed to upload image"})
		return
	}

	// Generate thumbnail
	thumbData, err := services.GenerateThumbnail(buf.Bytes(), 128)
	if err == nil {
		thumbKey := strings.TrimSuffix(storageKey, filepath.Ext(storageKey)) + "_thumb.jpg"
		storage.Upload(ctx, thumbKey, bytes.NewReader(thumbData), "image/jpeg")
	}

	orgID := middleware.GetOrgID(ctx)
	now := time.Now()

	tx := db.Begin()

	// Create new display image record
	displayMedia := models.InstrumentMedia{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		OrgID:        orgID,
		InstrumentID: &instrumentID,
		BatchID:      uuid.New().String(),
		BatchType:    "display",
		FileName:     header.Filename,
		FileType:     "image",
		FileSize:     int64(buf.Len()),
		StorageKey:   storageKey,
		IsDisplay:    true,
		SortOrder:    0,
		CreatedAt:    now,
	}
	if err := tx.Create(&displayMedia).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50003, "message": "failed to create media record"})
		return
	}

	tx.Commit()

	// Get URL for response
	url, _ := storage.GetURL(ctx, storageKey)
	if url == "" {
		url = "/uploads/media/" + storageKey
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"id":         displayMedia.ID,
			"url":        url,
			"width":      width,
			"height":     height,
			"file_size":  displayMedia.FileSize,
		},
	})
}

func syncBackwardCompat(db *gorm.DB, tenantID, instrumentID string) {
	var displayMedia []models.InstrumentMedia
	if err := db.Where("instrument_id = ? AND tenant_id = ? AND is_display = ? AND file_type != ?",
		instrumentID, tenantID, true, "video_thumb").
		Order("sort_order asc").
		Find(&displayMedia).Error; err != nil {
		return
	}

	var images []string
	var video string
	for _, m := range displayMedia {
		if m.FileType == "video" {
			video = "/uploads/media/" + m.StorageKey
		} else {
			images = append(images, "/uploads/media/"+m.StorageKey)
		}
	}

	imagesJSONBytes, _ := json.Marshal(images)
	imagesJSON := string(imagesJSONBytes)

	db.Model(&models.Instrument{}).
		Where("id = ? AND tenant_id = ?", instrumentID, tenantID).
		Updates(map[string]interface{}{
			"images": imagesJSON,
			"video":  video,
		})
}
