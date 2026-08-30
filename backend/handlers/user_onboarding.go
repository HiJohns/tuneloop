package handlers

import (
	"context"
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
	if err := db.Select("id, name, onboarding_completed, promo_points").
		Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"name":                 user.Name,
			"onboarding_completed": user.OnboardingCompleted,
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

	side := c.PostForm("side")
	if side != "front" && side != "back" && side != "other" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "message": "invalid side, must be front, back, or other"})
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

	if err := services.NewMediaRegistry().RegisterAsset(ctx, filename, services.SourceTypeIDPhoto, userID, file.Size, "image"); err != nil {
		log.Printf("[MediaRegistry] register id photo %s failed: %v", filename, err)
	}

	fileURL, _ := storage.GetURL(ctx, filename)
	if fileURL == "" {
		fileURL = fmt.Sprintf("/uploads/media/%s", filename)
	}

	// Persist the storage key to the user's profile (#1598). Save the raw
	// storage key so it can be re-resolved via GetURL later.
	col := "id_photo_front"
	if side == "back" {
		col = "id_photo_back"
	} else if side == "other" {
		col = "id_photo_other"
	}
	// Mark the previous id photo on this side as unreferenced before overwrite.
	var oldUser models.User
	if err := db.Select(col).Where("id = ?", userID).First(&oldUser).Error; err == nil {
		var oldKey string
		switch col {
		case "id_photo_front":
			if oldUser.IdPhotoFront != nil {
				oldKey = *oldUser.IdPhotoFront
			}
		case "id_photo_back":
			if oldUser.IdPhotoBack != nil {
				oldKey = *oldUser.IdPhotoBack
			}
		case "id_photo_other":
			if oldUser.IdPhotoOther != nil {
				oldKey = *oldUser.IdPhotoOther
			}
		}
		if oldKey != "" && oldKey != filename {
			if err := services.NewMediaRegistry().MarkUnreferenced(ctx, oldKey); err != nil {
				log.Printf("[MediaRegistry] mark old id photo %s unreferenced failed: %v", oldKey, err)
			}
		}
	}
	if err := db.Model(&models.User{}).Where("id = ?", userID).Update(col, filename).Error; err != nil {
		log.Printf("id photo persist failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50004, "message": "failed to save photo reference"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "upload success",
		"data": gin.H{
			"url":  fileURL,
			"side": side,
		},
	})
}

// GetIdPhotos returns the current user's ID photo URLs (front + back + other).
func (h *UserOnboardingHandler) GetIdPhotos(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	userID, err := middleware.EnsureLocalUser(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "user sync failed: " + err.Error()})
		return
	}

	var user models.User
	if err := db.Select("id_photo_front, id_photo_back, id_photo_other").Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}

	storage := services.NewMediaStorage()
	front := ""
	if user.IdPhotoFront != nil && *user.IdPhotoFront != "" {
		front, _ = storage.GetURL(ctx, *user.IdPhotoFront)
	}
	back := ""
	if user.IdPhotoBack != nil && *user.IdPhotoBack != "" {
		back, _ = storage.GetURL(ctx, *user.IdPhotoBack)
	}
	other := ""
	if user.IdPhotoOther != nil && *user.IdPhotoOther != "" {
		other, _ = storage.GetURL(ctx, *user.IdPhotoOther)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"front":  front,
			"back":   back,
			"other":  other,
		},
	})
}

// DeleteIdPhoto clears one side of the current user's ID photo.
func (h *UserOnboardingHandler) DeleteIdPhoto(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	userID, err := middleware.EnsureLocalUser(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "user sync failed: " + err.Error()})
		return
	}

	side := c.Query("side")
	if side != "front" && side != "back" && side != "other" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "message": "invalid side, must be front, back, or other"})
		return
	}

	col := "id_photo_front"
	if side == "back" {
		col = "id_photo_back"
	} else if side == "other" {
		col = "id_photo_other"
	}
	// Read the old storage key, delete the physical file, then clear the field.
	var user models.User
	if err := db.Select(col).Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}
	var oldKey string
	switch col {
	case "id_photo_front":
		if user.IdPhotoFront != nil {
			oldKey = *user.IdPhotoFront
		}
	case "id_photo_back":
		if user.IdPhotoBack != nil {
			oldKey = *user.IdPhotoBack
		}
	case "id_photo_other":
		if user.IdPhotoOther != nil {
			oldKey = *user.IdPhotoOther
		}
	}
	if oldKey != "" {
		storage := services.NewMediaStorage()
		if err := storage.Delete(ctx, oldKey); err != nil {
			log.Printf("[DeleteIdPhoto] failed to delete physical file %s: %v", oldKey, err)
		}
		if err := services.NewMediaRegistry().MarkUnreferenced(ctx, oldKey); err != nil {
			log.Printf("[MediaRegistry] mark id photo %s unreferenced failed: %v", oldKey, err)
		}
	}
	if err := db.Model(&models.User{}).Where("id = ?", userID).Update(col, "").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to delete photo"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "deleted",
	})
}

// GetUserIdPhotos returns a user's ID photo URLs for staff identity
// verification (shipping/receiving/repair). Tenant isolation is enforced by
// checking the caller's org scope against the target user's org binding.
// GET /api/user/:userId/id-photos
func (h *UserOnboardingHandler) GetUserIdPhotos(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	role := middleware.GetBusinessRole(ctx)
	switch role {
	case middleware.BusinessRoleSiteAdmin, middleware.BusinessRoleSiteMember, middleware.BusinessRoleMerchantAdmin, middleware.BusinessRoleSystemAdmin:
		// staff or platform-level: allowed
	default:
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "no permission to view user id photos"})
		return
	}

	targetID := c.Param("userId")
	var user models.User
	if err := db.Select("id_photo_front, id_photo_back").Where("id = ?", targetID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}

	storage := services.NewMediaStorage()
	front := ""
	if user.IdPhotoFront != nil && *user.IdPhotoFront != "" {
		front, _ = storage.GetURL(ctx, *user.IdPhotoFront)
	}
	back := ""
	if user.IdPhotoBack != nil && *user.IdPhotoBack != "" {
		back, _ = storage.GetURL(ctx, *user.IdPhotoBack)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"front": front,
			"back":  back,
		},
	})
}

// AdminUploadIDPhoto uploads an ID photo on behalf of a user (PC admin).
// POST /api/admin/user-management/:id/id-photo
func (h *UserOnboardingHandler) AdminUploadIDPhoto(c *gin.Context) {
	// Platform-level admin operation: exempt from tenant query scoping so
	// customers (empty tenant_id) can be located (see UserManagementHandler.platformDB).
	ctx := context.WithValue(c.Request.Context(), database.TenantIDKey, "")
	db := database.GetDB().WithContext(ctx)

	targetID := c.Param("id")
	var target models.User
	if err := db.Select("id").Where("id = ?", targetID).First(&target).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}

	side := c.PostForm("side")
	if side != "front" && side != "back" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "message": "invalid side, must be front or back"})
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
	filename := fmt.Sprintf("id_photos/%s_%d%s", targetID, time.Now().UnixNano(), ext)

	reader, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50002, "message": "failed to open file"})
		return
	}
	defer reader.Close()

	storage := services.NewMediaStorage()
	if err := storage.Upload(ctx, filename, reader, mimeType); err != nil {
		log.Printf("admin id photo upload failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50003, "message": "failed to save file"})
		return
	}

	if err := services.NewMediaRegistry().RegisterAsset(ctx, filename, services.SourceTypeIDPhoto, targetID, file.Size, "image"); err != nil {
		log.Printf("[MediaRegistry] register id photo %s failed: %v", filename, err)
	}

	fileURL, _ := storage.GetURL(ctx, filename)
	if fileURL == "" {
		fileURL = fmt.Sprintf("/uploads/media/%s", filename)
	}

	col := "id_photo_front"
	if side == "back" {
		col = "id_photo_back"
	}
	// Mark the previous id photo on this side as unreferenced before overwrite.
	var oldUser models.User
	if err := db.Select(col).Where("id = ?", targetID).First(&oldUser).Error; err == nil {
		var oldKey string
		if col == "id_photo_front" {
			if oldUser.IdPhotoFront != nil {
				oldKey = *oldUser.IdPhotoFront
			}
		} else {
			if oldUser.IdPhotoBack != nil {
				oldKey = *oldUser.IdPhotoBack
			}
		}
		if oldKey != "" && oldKey != filename {
			if err := services.NewMediaRegistry().MarkUnreferenced(ctx, oldKey); err != nil {
				log.Printf("[MediaRegistry] mark old id photo %s unreferenced failed: %v", oldKey, err)
			}
		}
	}
	if err := db.Model(&models.User{}).Where("id = ?", targetID).Update(col, filename).Error; err != nil {
		log.Printf("admin id photo persist failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50004, "message": "failed to save photo reference"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "upload success",
		"data": gin.H{
			"url":  fileURL,
			"side": side,
		},
	})
}

// AdminDeleteIdPhoto clears one side of a user's ID photo (PC admin).
// DELETE /api/admin/user-management/:id/id-photo?side=front|back
func (h *UserOnboardingHandler) AdminDeleteIdPhoto(c *gin.Context) {
	// Platform-level admin operation: exempt from tenant query scoping
	// (see UserManagementHandler.platformDB).
	ctx := context.WithValue(c.Request.Context(), database.TenantIDKey, "")
	db := database.GetDB().WithContext(ctx)

	targetID := c.Param("id")
	var target models.User
	if err := db.Select("id").Where("id = ?", targetID).First(&target).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}

	side := c.Query("side")
	if side != "front" && side != "back" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "message": "invalid side, must be front or back"})
		return
	}

	col := "id_photo_front"
	if side == "back" {
		col = "id_photo_back"
	}
	// Read the old storage key, delete the physical file, then clear the field.
	var oldUser models.User
	if err := db.Select(col).Where("id = ?", targetID).First(&oldUser).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}
	var oldKey string
	if col == "id_photo_front" {
		if oldUser.IdPhotoFront != nil {
			oldKey = *oldUser.IdPhotoFront
		}
	} else {
		if oldUser.IdPhotoBack != nil {
			oldKey = *oldUser.IdPhotoBack
		}
	}
	if oldKey != "" {
		storage := services.NewMediaStorage()
		if err := storage.Delete(ctx, oldKey); err != nil {
			log.Printf("[AdminDeleteIdPhoto] failed to delete physical file %s: %v", oldKey, err)
		}
		if err := services.NewMediaRegistry().MarkUnreferenced(ctx, oldKey); err != nil {
			log.Printf("[MediaRegistry] mark id photo %s unreferenced failed: %v", oldKey, err)
		}
	}
	if err := db.Model(&models.User{}).Where("id = ?", targetID).Update(col, "").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to delete photo"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "deleted",
	})
}
