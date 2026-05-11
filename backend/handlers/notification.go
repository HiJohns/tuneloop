package handlers

import (
	"net/http"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

func GetNotifications(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)

	db := database.GetDB().WithContext(ctx)

	var notifications []models.Notification
	if err := db.Where("user_id = ?", userID).Order("created_at DESC").Find(&notifications).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch notifications",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list": notifications,
		},
	})
}

func MarkNotificationRead(c *gin.Context) {
	ctx := c.Request.Context()
	notificationID := c.Param("id")

	db := database.GetDB().WithContext(ctx)

	if err := db.Model(&models.Notification{}).Where("id = ?", notificationID).Update("status", "read").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to mark notification as read",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
	})
}

func GetInstrumentPhotoSpecs(c *gin.Context) {
	categoryID := c.Param("category_id")

	db := database.GetDB()

	var spec models.InstrumentPhotoSpec
	if err := db.Where("category_id = ?", categoryID).First(&spec).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 20000,
			"data": gin.H{
				"photo_requirements": []interface{}{},
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"photo_requirements": spec.PhotoRequirements,
		},
	})
}
