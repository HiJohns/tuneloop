package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
)

var (
	resetRateLimit sync.Map
)

const (
	maxResetPerWindow = 3
	resetWindow       = 30 * time.Minute
)

type rateEntry struct {
	count    int
	windowAt time.Time
}

func checkResetRateLimit(userID string) bool {
	val, _ := resetRateLimit.Load(userID)
	entry, ok := val.(*rateEntry)
	now := time.Now()

	if !ok || now.Sub(entry.windowAt) > resetWindow {
		resetRateLimit.Store(userID, &rateEntry{count: 1, windowAt: now})
		return true
	}

	if entry.count >= maxResetPerWindow {
		return false
	}

	entry.count++
	return true
}

func maskEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return email
	}
	local := parts[0]
	if len(local) <= 1 {
		return email
	}
	return local[:1] + strings.Repeat("*", len(local)-1) + "@" + parts[1]
}

func ResetPasswordSelf(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)

	if !checkResetRateLimit(userID) {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"code":    42900,
			"message": "操作过于频繁，请 30 分钟后再试",
		})
		return
	}

	db := database.GetDB().WithContext(ctx)

	var user models.User
	if err := db.Where("iam_sub = ? AND deleted_at IS NULL", userID).First(&user).Error; err != nil {
		if err2 := db.Where("id = ? AND deleted_at IS NULL", userID).First(&user).Error; err2 != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    40400,
				"message": "用户不存在",
			})
			return
		}
	}

	if user.Email == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "您的账户未绑定邮箱，请联系管理员",
		})
		return
	}

	iamClient := services.NewIAMClient()
	if err := iamClient.RequestPasswordReset(userID); err != nil {
		log.Printf("[ResetPasswordSelf] Failed to request password reset for user %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50002,
			"message": "邮件发送失败，请稍后重试",
		})
		return
	}

	emailMasked := maskEmail(user.Email)

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": fmt.Sprintf("密码重置邮件已发送至 %s，请查收", emailMasked),
		"data": gin.H{
			"email_masked":      emailMasked,
			"expires_in_minutes": 60,
		},
	})
}
