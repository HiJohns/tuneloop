package handlers

import (
	"net/http"
	"strings"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserAddressHandler struct{}

func NewUserAddressHandler() *UserAddressHandler {
	return &UserAddressHandler{}
}

// resolveAddressUserID 解析并校验当前登录用户的本地 user_id（#1930）。
// 返回 (userID, httpStatus, message)：成功时 httpStatus=0；失败时调用方直接返回。
// 匿名（iam_sub 为空）→ 401；iam_sub 有值但无本地记录 → EnsureLocalUser 建/激 shadow user。
func resolveAddressUserID(c *gin.Context, db *gorm.DB) (string, int, string) {
	ctx := c.Request.Context()
	iamSub := middleware.GetUserID(ctx)
	if iamSub == "" {
		return "", http.StatusUnauthorized, "login required"
	}
	var localUser models.User
	if err := db.Where("iam_sub = ?", iamSub).First(&localUser).Error; err == nil {
		return localUser.ID, 0, ""
	}
	localID, err := middleware.EnsureLocalUser(ctx, db)
	if err != nil {
		if strings.Contains(err.Error(), "no user ID") {
			return "", http.StatusUnauthorized, "login required"
		}
		return "", http.StatusInternalServerError, "user sync failed: " + err.Error()
	}
	return localID, 0, ""
}

// addressErrorCode maps a guard http status to the API error code.
func addressErrorCode(status int) int {
	if status == http.StatusUnauthorized {
		return 40001
	}
	return 50000
}

// addressGuard writes the error response itself when the guard fails.
func addressGuard(c *gin.Context, db *gorm.DB) (string, bool) {
	userID, status, msg := resolveAddressUserID(c, db)
	if status == 0 {
		return userID, true
	}
	c.JSON(status, gin.H{"code": addressErrorCode(status), "message": msg})
	return "", false
}

// ListAddresses returns the current user's addresses, default first
func (h *UserAddressHandler) ListAddresses(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID, ok := addressGuard(c, db)
	if !ok {
		return
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

	userID, ok := addressGuard(c, db)
	if !ok {
		return
	}

	var req struct {
		RecipientName string `json:"recipient_name"`
		Phone         string `json:"phone"`
		Province      string `json:"province"`
		City          string `json:"city"`
		District      string `json:"district"`
		Detail        string `json:"detail"`
		PostalCode    string `json:"postal_code"`
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
		PostalCode:    req.PostalCode,
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
		"code":    20000,
		"message": "success",
		"data":    addr,
	})
}

// UpdateAddress updates an existing address
func (h *UserAddressHandler) UpdateAddress(c *gin.Context) {
	ctx := c.Request.Context()
	addrID := c.Param("id")

	db := database.GetDB().WithContext(ctx)
	userID, ok := addressGuard(c, db)
	if !ok {
		return
	}

	var req struct {
		RecipientName string `json:"recipient_name"`
		Phone         string `json:"phone"`
		Province      string `json:"province"`
		City          string `json:"city"`
		District      string `json:"district"`
		Detail        string `json:"detail"`
		PostalCode    string `json:"postal_code"`
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
		"postal_code":    req.PostalCode,
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
		"code":    20000,
		"message": "success",
	})
}

// SetDefaultAddress sets an address as default (clearing others)
func (h *UserAddressHandler) SetDefaultAddress(c *gin.Context) {
	ctx := c.Request.Context()
	addrID := c.Param("id")

	db := database.GetDB().WithContext(ctx)
	userID, ok := addressGuard(c, db)
	if !ok {
		return
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
		"code":    20000,
		"message": "success",
	})
}

// DeleteAddress deletes a user's address
func (h *UserAddressHandler) DeleteAddress(c *gin.Context) {
	ctx := c.Request.Context()
	addrID := c.Param("id")

	db := database.GetDB().WithContext(ctx)
	userID, ok := addressGuard(c, db)
	if !ok {
		return
	}

	result := db.Where("id = ? AND user_id = ?", addrID, userID).Delete(&models.UserAddress{})
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "address not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
	})
}
