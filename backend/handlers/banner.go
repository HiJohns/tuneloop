package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

type BannerHandler struct{}

func NewBannerHandler() *BannerHandler {
	return &BannerHandler{}
}

func (h *BannerHandler) ListBanners(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var banners []models.Banner
	if err := db.Order("sort_order asc").Find(&banners).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to list banners"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"list": banners,
		},
	})
}

func (h *BannerHandler) CreateBanner(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)

	var req struct {
		ImageURL  string `json:"image_url" binding:"required"`
		LinkURL   string `json:"link_url"`
		Title     string `json:"title"`
		SortOrder int    `json:"sort_order"`
		Status    string `json:"status"`
		BgColor   string `json:"bg_color"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	status := req.Status
	if status == "" {
		status = "active"
	}
	bgColor := req.BgColor
	if bgColor == "" {
		bgColor = "#915F38"
	}

	banner := models.Banner{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		ImageURL:  req.ImageURL,
		LinkURL:   req.LinkURL,
		Title:     req.Title,
		SortOrder: req.SortOrder,
		Status:    status,
		BgColor:   bgColor,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Create(&banner).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create banner: " + err.Error()})
		return
	}

	// Async blur generation
	go func() {
		if err := GenerateBlurBanner(banner.ImageURL); err != nil {
			log.Printf("[Banner] blur generation failed for %s: %v", banner.ImageURL, err)
		}
	}()

	c.JSON(http.StatusCreated, gin.H{
		"code":    20000,
		"message": "success",
		"data":    banner,
	})
}

func (h *BannerHandler) UpdateBanner(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "banner id is required"})
		return
	}

	var existing models.Banner
	if err := db.Where("id = ?", id).First(&existing).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "banner not found"})
		return
	}

	var req struct {
		ImageURL  string `json:"image_url"`
		LinkURL   string `json:"link_url"`
		Title     string `json:"title"`
		SortOrder int    `json:"sort_order"`
		Status    string `json:"status"`
		BgColor   string `json:"bg_color"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	newImage := req.ImageURL != "" && req.ImageURL != existing.ImageURL

	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}
	if req.ImageURL != "" {
		updates["image_url"] = req.ImageURL
	}
	if req.LinkURL != "" {
		updates["link_url"] = req.LinkURL
	}
	if req.Title != "" {
		updates["title"] = req.Title
	}
	updates["sort_order"] = req.SortOrder
	if req.Status != "" {
		updates["status"] = req.Status
	}
	if req.BgColor != "" {
		updates["bg_color"] = req.BgColor
	}

	if err := db.Model(&existing).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update banner: " + err.Error()})
		return
	}

	// Regenerate blur if image changed
	if newImage {
		go func() {
			if err := GenerateBlurBanner(req.ImageURL); err != nil {
				log.Printf("[Banner] blur regeneration failed for %s: %v", req.ImageURL, err)
			}
		}()
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
	})
}

func (h *BannerHandler) DeleteBanner(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "banner id is required"})
		return
	}

	var banner models.Banner
	if err := db.Where("id = ?", id).First(&banner).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "banner not found"})
		return
	}

	// Delete the uploaded file from disk
	if banner.ImageURL != "" && strings.HasPrefix(banner.ImageURL, "/uploads/media/") {
		key := strings.TrimPrefix(banner.ImageURL, "/uploads/media/")
		filePath := filepath.Join("./uploads/media", key)
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			log.Printf("[Banner] failed to delete file %s: %v", filePath, err)
		}
		// Delete blur version
		blurFile := strings.Replace(filePath, filepath.Ext(filePath), "_blur.webp", 1)
		if err := os.Remove(blurFile); err != nil && !os.IsNotExist(err) {
			log.Printf("[Banner] failed to delete blur file %s: %v", blurFile, err)
		}
	}

	result := db.Where("id = ?", id).Delete(&banner)
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "banner not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
	})
}

// GenerateBlurBanner creates a blurred _blur.webp version of the banner image.
func GenerateBlurBanner(imagePath string) error {
	if !strings.HasPrefix(imagePath, "/uploads/media/") {
		return nil // not a file-based image
	}
	key := strings.TrimPrefix(imagePath, "/uploads/media/")
	srcPath := filepath.Join("./uploads/media", key)
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return fmt.Errorf("source file not found: %s", srcPath)
	}
	src, err := imaging.Open(srcPath)
	if err != nil {
		return fmt.Errorf("imaging.Open failed for %s: %w", srcPath, err)
	}
	blur := imaging.Blur(src, 15)
	blurPath := strings.Replace(srcPath, filepath.Ext(srcPath), "_blur.webp", 1)
	if err := imaging.Save(blur, blurPath); err != nil {
		return fmt.Errorf("imaging.Save blur failed: %w", err)
	}
	return nil
}

func ValidateBannerFiles(db *gorm.DB) {
	var banners []models.Banner
	db.Find(&banners)
	for _, b := range banners {
		if !strings.HasPrefix(b.ImageURL, "/uploads/media/") {
			continue
		}
		key := strings.TrimPrefix(b.ImageURL, "/uploads/media/")
		filePath := filepath.Join("./uploads/media", key)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			log.Printf("[Banner] WARNING: file not found for banner %s: %s (DB record exists)", b.ID, filePath)
		}
	}
}

func (h *BannerHandler) GetPublicBanners(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var banners []models.Banner
	if err := db.Where("status = ?", "active").Order("sort_order asc").Find(&banners).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to list banners"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"list": banners,
		},
	})
}

// MigrateBannerBlur scans all active banners and generates _blur.webp for any that are missing it.
func MigrateBannerBlur(db *gorm.DB) {
	var banners []models.Banner
	db.Where("status = ?", "active").Find(&banners)
	count := 0
	for _, b := range banners {
		if err := GenerateBlurBanner(b.ImageURL); err != nil {
			log.Printf("[BannerBlur] migration failed for %s: %v", b.ImageURL, err)
		} else {
			count++
		}
	}
	log.Printf("[BannerBlur] migration complete: %d banners processed", count)
}
