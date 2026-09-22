package handlers

import (
	"log"
	"net/http"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
)

// RegisterAsCustomer grants the caller the customer identity (#2031 A, B1).
// The customer identity is a zero-permission functional role attached to the
// user's root-org member relation — no org is created. Only the authenticated
// user may grant it to themselves (no admin代挂), and the namespace is derived
// by beaconiam from the user's existing relations.
func RegisterAsCustomer(c *gin.Context) {
	ctx := c.Request.Context()
	localUserID := middleware.GetUserID(ctx)
	if localUserID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "未登录"})
		return
	}

	var user models.User
	if err := database.GetDB().WithContext(ctx).Where("id = ?", localUserID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "用户不存在"})
		return
	}
	if user.IAMSub == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "账户未与 IAM 同步，请重新登录后再试"})
		return
	}

	if err := services.NewIAMClient().GrantCustomerRole(user.IAMSub); err != nil {
		log.Printf("[RegisterAsCustomer] IAM grant failed for user=%s: %v", user.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "注册为顾客失败，请稍后重试"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"is_customer": true},
	})
}
