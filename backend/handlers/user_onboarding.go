package handlers

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
)

type UserOnboardingHandler struct{}

func NewUserOnboardingHandler() *UserOnboardingHandler {
	return &UserOnboardingHandler{}
}

func (h *UserOnboardingHandler) GetOnboardingStatus(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	userID, err := middleware.EnsureLocalUser(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "user sync failed: " + err.Error()})
		return
	}

	var user models.User
	if err := db.Select("id, name, onboarding_completed, prepaid_points, promo_points").
		Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"name":                user.Name,
			"onboarding_completed": user.OnboardingCompleted,
			"prepaid_points":       user.PrepaidPoints,
			"promo_points":         user.PromoPoints,
		},
	})
}

func (h *UserOnboardingHandler) CompleteOnboarding(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	userID, err := middleware.EnsureLocalUser(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "user sync failed: " + err.Error()})
		return
	}

	var req struct {
		Name *string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request: " + err.Error()})
		return
	}

	updates := map[string]interface{}{
		"onboarding_completed": true,
		"updated_at":           time.Now(),
	}
	if req.Name != nil {
		updates["name"] = *req.Name
	}

	if err := db.Model(&models.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to complete onboarding: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "onboarding completed",
	})
}

func (h *UserOnboardingHandler) UploadIDPhoto(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	userID, err := middleware.EnsureLocalUser(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "user sync failed: " + err.Error()})
		return
	}

	c.Request.ParseMultipartForm(10 << 20)
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "no file uploaded"})
		return
	}

	mimeType := file.Header.Get("Content-Type")
	if mimeType != "image/jpeg" && mimeType != "image/png" && mimeType != "image/webp" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "only JPEG, PNG, WebP allowed"})
		return
	}

	if file.Size > 5*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "file too large, max 5MB"})
		return
	}

	ext := filepath.Ext(file.Filename)
	filename := fmt.Sprintf("id_photos/%s_%d%s", userID, time.Now().UnixNano(), ext)

	reader, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50002, "message": "failed to open file"})
		return
	}
	defer reader.Close()

	storage := services.NewMediaStorage()
	if err := storage.Upload(ctx, filename, reader, mimeType); err != nil {
		log.Printf("id photo upload failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50003, "message": "failed to save file"})
		return
	}

	fileURL, _ := storage.GetURL(ctx, filename)
	if fileURL == "" {
		fileURL = fmt.Sprintf("/uploads/media/%s", filename)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "upload success",
		"data": gin.H{
			"url": fileURL,
		},
	})
}
