package handlers

import (
	"encoding/json"
	"net/http"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

func GetNotifications(c *gin.Context) {
	ctx := c.Request.Context()

	db := database.GetDB().WithContext(ctx)

	// #1742: notifications.user_id stores the LOCAL users.id; the JWT sub is
	// the IAM-side id. Resolve via read-only reverse lookup. No local record
	// → empty list (never a shadow-user write on a read path).
	userID, err := middleware.LocalUserID(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to resolve user",
		})
		return
	}

	notifications := []models.Notification{}
	if userID != "" {
		if err := db.Where("user_id = ?", userID).Order("created_at DESC").Find(&notifications).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "Failed to fetch notifications",
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list": notifications,
		},
	})
}

func GetUnreadCount(c *gin.Context) {
	ctx := c.Request.Context()

	db := database.GetDB().WithContext(ctx)

	// #1742: resolve local id (see GetNotifications).
	userID, err := middleware.LocalUserID(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to resolve user",
		})
		return
	}

	count := int64(0)
	if userID != "" {
		if err := db.Model(&models.Notification{}).Where("user_id = ? AND status = ?", userID, "unread").Count(&count).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "Failed to count unread notifications",
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"count": count,
		},
	})
}

func MarkAllNotificationsRead(c *gin.Context) {
	ctx := c.Request.Context()

	db := database.GetDB().WithContext(ctx)

	// #1742: resolve local id (see GetNotifications). No local record →
	// nothing to mark; respond idempotent success.
	userID, err := middleware.LocalUserID(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to resolve user",
		})
		return
	}

	if userID != "" {
		if err := db.Model(&models.Notification{}).Where("user_id = ? AND status = ?", userID, "unread").Update("status", "read").Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "Failed to mark all notifications as read",
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
	})
}

func GetNotificationDetail(c *gin.Context) {
	notificationID := c.Param("id")
	ctx := c.Request.Context()

	db := database.GetDB().WithContext(ctx)

	// #1742: resolve local id (see GetNotifications).
	userID, err := middleware.LocalUserID(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to resolve user",
		})
		return
	}

	if userID == "" {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "notification not found"})
		return
	}

	var notification models.Notification
	if err := db.Where("id = ? AND user_id = ?", notificationID, userID).First(&notification).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "notification not found"})
		return
	}

	ref := gin.H{"type": notification.RefType}

	switch notification.RefType {
	case "damage_report":
		var damageReport models.DamageReport
		if err := db.Where("id = ?", notification.RefID).First(&damageReport).Error; err == nil {
			ref["damage_report"] = damageReport

			var order models.Order
			if err := db.Where("id = ?", damageReport.LeaseID).First(&order).Error; err == nil {
				ref["order"] = order
			}
		}
	case "appeal":
		var appeal models.Appeal
		if err := db.Where("id = ?", notification.RefID).First(&appeal).Error; err == nil {
			ref["appeal"] = appeal

			var damageReport models.DamageReport
			if err := db.Where("id = ?", appeal.DamageReportID).First(&damageReport).Error; err == nil {
				ref["damage_report"] = damageReport
			}
		}
	}

	if notification.ActionData != nil && *notification.ActionData != "" {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(*notification.ActionData), &parsed); err == nil {
			for k, v := range parsed {
				ref[k] = v
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"notification": notification,
			"ref":          ref,
		},
	})
}

func MarkNotificationRead(c *gin.Context) {
	notificationID := c.Param("id")
	ctx := c.Request.Context()

	db := database.GetDB().WithContext(ctx)

	// #1742: resolve local id (see GetNotifications). No local record →
	// nothing to mark; respond idempotent success.
	userID, err := middleware.LocalUserID(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to resolve user",
		})
		return
	}

	if userID != "" {
		if err := db.Model(&models.Notification{}).Where("id = ? AND user_id = ?", notificationID, userID).Update("status", "read").Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "Failed to mark notification as read",
			})
			return
		}
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
