package handlers

import (
	"log"
	"net/http"
	"sync"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
)

var (
	changePasswordRateLimit sync.Map
)

type changePwdRateEntry struct {
	count    int
	windowAt time.Time
}

func checkChangePasswordRateLimit(userID string) bool {
	now := time.Now()
	val, _ := changePasswordRateLimit.Load(userID)
	if entry, ok := val.(*changePwdRateEntry); ok {
		if now.Sub(entry.windowAt) < 5*time.Minute {
			entry.count++
			return entry.count <= 3
		}
	}
	changePasswordRateLimit.Store(userID, &changePwdRateEntry{count: 1, windowAt: now})
	return true
}

func ChangePasswordSelf(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)

	if !checkChangePasswordRateLimit(userID) {
		c.JSON(http.StatusTooManyRequests, gin.H{"code": 42900, "message": "操作过于频繁，请 5 分钟后再试"})
		return
	}

	var req struct {
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "缺失新密码"})
		return
	}

	if len(req.NewPassword) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "密码长度不能少于 8 位"})
		return
	}
	if err := validatePassword(req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}

	iamClient := services.NewIAMClient()
	if err := iamClient.UpdateUserPassword(userID, req.NewPassword); err != nil {
		log.Printf("[ChangePasswordSelf] Failed to update password for user %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "密码修改失败，请稍后重试"})
		return
	}

	db := database.GetDB().WithContext(ctx)
	if err := db.Model(&models.User{}).Where("(iam_sub = ? OR id = ?) AND deleted_at IS NULL", userID, userID).Update("force_password_change", false).Error; err != nil {
		log.Printf("[ChangePasswordSelf] Failed to clear force_password_change for user %s: %v", userID, err)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "密码修改成功",
	})
}
