package handlers

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

// MallImageHandler 商城页图片（#2210）——平台级全站配置，与租户无关。
// 读 = 公开 status='active'；写 = namespace_admin（mall_image:manage cus_perm）。
type MallImageHandler struct{}

func NewMallImageHandler() *MallImageHandler {
	return &MallImageHandler{}
}

func (h *MallImageHandler) ListMallImages(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var images []models.MallImage
	if err := db.Order("sort_order asc").Find(&images).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to list mall images"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"list": images,
		},
	})
}

func (h *MallImageHandler) CreateMallImage(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var req struct {
		ImageURL  string `json:"image_url" binding:"required"`
		LinkURL   string `json:"link_url"`
		Title     string `json:"title"`
		SortOrder int    `json:"sort_order"`
		Status    string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	status := req.Status
	if status == "" {
		status = "active"
	}

	image := models.MallImage{
		ID:        uuid.New().String(),
		ImageURL:  req.ImageURL,
		LinkURL:   req.LinkURL,
		Title:     req.Title,
		SortOrder: req.SortOrder,
		Status:    status,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Create(&image).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create mall image: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":    20000,
		"message": "success",
		"data":    image,
	})
}

func (h *MallImageHandler) UpdateMallImage(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "mall image id is required"})
		return
	}

	var existing models.MallImage
	if err := db.Where("id = ?", id).First(&existing).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "mall image not found"})
		return
	}

	var req struct {
		ImageURL  string `json:"image_url"`
		LinkURL   string `json:"link_url"`
		Title     string `json:"title"`
		SortOrder int    `json:"sort_order"`
		Status    string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

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

	if err := db.Model(&existing).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update mall image: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
	})
}

func (h *MallImageHandler) DeleteMallImage(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "mall image id is required"})
		return
	}

	var image models.MallImage
	if err := db.Where("id = ?", id).First(&image).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "mall image not found"})
		return
	}

	// Delete the uploaded file from disk
	if image.ImageURL != "" && strings.HasPrefix(image.ImageURL, "/uploads/media/") {
		key := strings.TrimPrefix(image.ImageURL, "/uploads/media/")
		filePath := filepath.Join("./uploads/media", key)
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			log.Printf("[MallImage] failed to delete file %s: %v", filePath, err)
		}
	}

	result := db.Where("id = ?", id).Delete(&image)
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "mall image not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
	})
}

func (h *MallImageHandler) GetPublicMallImages(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var images []models.MallImage
	if err := db.Where("status = ?", "active").Order("sort_order asc").Find(&images).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to list mall images"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"list": images,
		},
	})
}
