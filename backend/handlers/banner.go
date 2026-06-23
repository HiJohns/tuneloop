package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	status := req.Status
	if status == "" {
		status = "active"
	}

	banner := models.Banner{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		ImageURL:  req.ImageURL,
		LinkURL:   req.LinkURL,
		Title:     req.Title,
		SortOrder: req.SortOrder,
		Status:    status,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Create(&banner).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create banner: " + err.Error()})
		return
	}

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
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update banner: " + err.Error()})
		return
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

	result := db.Where("id = ?", id).Delete(&models.Banner{})
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "banner not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
	})
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
