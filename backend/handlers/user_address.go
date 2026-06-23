package handlers

import (
	"net/http"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type UserAddressHandler struct{}

func NewUserAddressHandler() *UserAddressHandler {
	return &UserAddressHandler{}
}

// ListAddresses returns the current user's addresses, default first
func (h *UserAddressHandler) ListAddresses(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)
	db := database.GetDB().WithContext(ctx)
	var localUser models.User
	if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err == nil {
		userID = localUser.ID
	}

	var addresses []models.UserAddress
	if err := db.Where("user_id = ?", userID).Order("is_default DESC, created_at DESC").Find(&addresses).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query addresses"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list": addresses,
		},
	})
}

// CreateAddress creates a new address for the current user
func (h *UserAddressHandler) CreateAddress(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	userID, err := middleware.EnsureLocalUser(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "user sync failed: " + err.Error()})
		return
	}

	var req struct {
		RecipientName string `json:"recipient_name"`
		Phone         string `json:"phone"`
		Province      string `json:"province"`
		City          string `json:"city"`
		District      string `json:"district"`
		Detail        string `json:"detail"`
		IsDefault     bool   `json:"is_default"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request: " + err.Error()})
		return
	}

	db = database.GetDB().WithContext(ctx)

	// Check for duplicate address
	var existingCount int64
	db.Model(&models.UserAddress{}).
		Where("user_id = ? AND recipient_name = ? AND phone = ? AND detail = ?",
			userID, req.RecipientName, req.Phone, req.Detail).
		Count(&existingCount)
	if existingCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "address already exists"})
		return
	}

	tx := db.Begin()

	if req.IsDefault {
		if err := tx.Model(&models.UserAddress{}).Where("user_id = ?", userID).Update("is_default", false).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to reset defaults"})
			return
		}
	}

	addr := models.UserAddress{
		ID:            uuid.New().String(),
		UserID:        userID,
		RecipientName: req.RecipientName,
		Phone:         req.Phone,
		Province:      req.Province,
		City:          req.City,
		District:      req.District,
		Detail:        req.Detail,
		IsDefault:     req.IsDefault,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if err := tx.Create(&addr).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create address"})
		return
	}

	tx.Commit()

	c.JSON(http.StatusCreated, gin.H{
		"code": 20000,
		"message": "success",
		"data": addr,
	})
}

// UpdateAddress updates an existing address
func (h *UserAddressHandler) UpdateAddress(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)
	addrID := c.Param("id")

	db := database.GetDB().WithContext(ctx)
	var localUser models.User
	if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err == nil {
		userID = localUser.ID
	}

	var req struct {
		RecipientName string `json:"recipient_name"`
		Phone         string `json:"phone"`
		Province      string `json:"province"`
		City          string `json:"city"`
		District      string `json:"district"`
		Detail        string `json:"detail"`
		IsDefault     bool   `json:"is_default"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request: " + err.Error()})
		return
	}

	var addr models.UserAddress
	if err := db.Where("id = ? AND user_id = ?", addrID, userID).First(&addr).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "address not found"})
		return
	}

	tx := db.Begin()

	if req.IsDefault {
		if err := tx.Model(&models.UserAddress{}).Where("user_id = ? AND id != ?", userID, addrID).Update("is_default", false).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to reset defaults"})
			return
		}
	}

	updates := map[string]interface{}{
		"recipient_name": req.RecipientName,
		"phone":          req.Phone,
		"province":       req.Province,
		"city":           req.City,
		"district":       req.District,
		"detail":         req.Detail,
		"is_default":     req.IsDefault,
		"updated_at":     time.Now(),
	}
	if err := tx.Model(&models.UserAddress{}).Where("id = ? AND user_id = ?", addrID, userID).Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update address"})
		return
	}

	tx.Commit()

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"message": "success",
	})
}

// SetDefaultAddress sets an address as default (clearing others)
func (h *UserAddressHandler) SetDefaultAddress(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)
	addrID := c.Param("id")

	db := database.GetDB().WithContext(ctx)
	var localUser models.User
	if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err == nil {
		userID = localUser.ID
	}

	var addr models.UserAddress
	if err := db.Where("id = ? AND user_id = ?", addrID, userID).First(&addr).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "address not found"})
		return
	}

	tx := db.Begin()

	if err := tx.Model(&models.UserAddress{}).Where("user_id = ?", userID).Update("is_default", false).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to reset defaults"})
		return
	}
	if err := tx.Model(&models.UserAddress{}).Where("id = ?", addrID).Update("is_default", true).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to set default"})
		return
	}

	tx.Commit()

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"message": "success",
	})
}

// DeleteAddress deletes a user's address
func (h *UserAddressHandler) DeleteAddress(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)
	addrID := c.Param("id")

	db := database.GetDB().WithContext(ctx)
	var localUser models.User
	if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err == nil {
		userID = localUser.ID
	}

	result := db.Where("id = ? AND user_id = ?", addrID, userID).Delete(&models.UserAddress{})
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "address not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"message": "success",
	})
}
