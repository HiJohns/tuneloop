package handlers

import (
	"log"
	"net/http"
	"regexp"
	"tuneloop-backend/middleware"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
)

func ChangePasswordSelf(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)

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
	hasUpper, _ := regexp.MatchString(`[A-Z]`, req.NewPassword)
	hasLower, _ := regexp.MatchString(`[a-z]`, req.NewPassword)
	hasDigit, _ := regexp.MatchString(`[0-9]`, req.NewPassword)
	if !hasUpper || !hasLower || !hasDigit {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "密码必须包含大写字母、小写字母和数字"})
		return
	}

	iamClient := services.NewIAMClient()
	if err := iamClient.UpdateUserPassword(userID, req.NewPassword); err != nil {
		log.Printf("[ChangePasswordSelf] Failed to update password for user %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "密码修改失败，请稍后重试"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "密码修改成功",
	})
}
