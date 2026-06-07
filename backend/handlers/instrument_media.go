package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
		BatchType string `json:"batch_type"`
		IsDisplay bool   `json:"is_display"`
		Files     []struct {
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
		media := models.InstrumentMedia{
			TenantID:     tenantID,
			OrgID:        middleware.GetOrgID(ctx),
			InstrumentID: instrumentID,
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
		InstrumentID: instrumentID,
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
	if err := db.Where("instrument_id = ? AND tenant_id = ?", instrumentID, tenantID).
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
