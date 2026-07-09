package handlers

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rwcarlsen/goexif/exif"
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
		blurFile := strings.Replace(filePath, filepath.Ext(filePath), "_blur.jpg", 1)
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

	// Fix EXIF orientation before processing
	if err := FixEXIFFile(srcPath); err != nil {
		log.Printf("[GenerateBlurBanner] FixEXIFFile warning for %s: %v", srcPath, err)
	}

	src, err := imaging.Open(srcPath)
	if err != nil {
		return fmt.Errorf("imaging.Open failed for %s: %w", srcPath, err)
	}
	blur := imaging.Blur(src, 8)
	blurPath := strings.Replace(srcPath, filepath.Ext(srcPath), "_blur.jpg", 1)
	if err := imaging.Save(blur, blurPath, imaging.JPEGQuality(80)); err != nil {
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

	// Compute aspect ratio for each banner from image file
	type BannerWithRatio struct {
		models.Banner
		AspectRatio float64 `json:"aspect_ratio"`
	}
	result := make([]BannerWithRatio, len(banners))
	for i, b := range banners {
		result[i].Banner = b
		if b.ImageURL != "" && strings.HasPrefix(b.ImageURL, "/uploads/media/") {
			key := strings.TrimPrefix(b.ImageURL, "/uploads/media/")
			filePath := filepath.Join("./uploads/media", key)
			f, err := os.Open(filePath)
			if err == nil {
				cfg, _, err := image.DecodeConfig(f)
				f.Close()
				if err == nil && cfg.Height > 0 {
					result[i].AspectRatio = float64(cfg.Width) / float64(cfg.Height)
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"list": result,
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

// FixEXIFFile opens a JPEG file, checks EXIF orientation, and rotates the file in-place if needed.
func FixEXIFFile(path string) error {
	if !strings.HasSuffix(strings.ToLower(path), ".jpg") && !strings.HasSuffix(strings.ToLower(path), ".jpeg") {
		return nil
	}
	f, err := os.Open(path)
	if err != nil { return err }
	orient := readEXIFOrientation(f)
	f.Close()
	if orient == 0 || orient == 1 { return nil }
	img, err := imaging.Open(path)
	if err != nil { return err }
	switch orient {
	case 3:  img = imaging.Rotate180(img)
	case 6:  img = imaging.Rotate270(img) // CW 90°
	case 8:  img = imaging.Rotate90(img)  // CCW 90°
	default: return nil
	}
	return imaging.Save(img, path)
}

// readEXIFOrientation returns the EXIF Orientation value (0 if not found).
func readEXIFOrientation(f *os.File) int {
	if f == nil { return 0 }
	x, err := exif.Decode(f)
	if err != nil { return 0 }
	tag, err := x.Get(exif.Orientation)
	if err != nil { return 0 }
	val, err := tag.Int(0)
	if err != nil { return 0 }
	return val
}

// RotateByEXIF decodes EXIF orientation from raw JPEG bytes and rotates the image accordingly.
func RotateByEXIF(img image.Image, rawData []byte) image.Image {
	r := bytes.NewReader(rawData)
	x, err := exif.Decode(r)
	if err != nil { return img }
	tag, err := x.Get(exif.Orientation)
	if err != nil { return img }
	val, err := tag.Int(0)
	if err != nil { return img }
	switch val {
	case 3:  return imaging.Rotate180(img)
	case 6:  return imaging.Rotate270(img)
	case 8:  return imaging.Rotate90(img)
	default: return img
	}
}
