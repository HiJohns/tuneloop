package services

import (
	"log"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/google/uuid"
)

// SendWarningNotification dispatches notifications for a warning based on its level configuration.
func SendWarningNotification(warning *models.Warning) {
	level := warning.Level
	notifyRoles := getNotifyRoles(level)
	notifyMethods := getNotifyMethods(level)

	for _, role := range notifyRoles {
		for _, method := range notifyMethods {
			switch method {
			case "system_message":
				sendSystemMessage(warning, role)
			case "wechat":
				sendWechatAlert(warning, role)
			case "email":
				sendEmailAlert(warning, role)
			default:
				log.Printf("[WarningNotification] Unknown method: %s", method)
			}
		}
	}
}

func getNotifyRoles(level string) []string {
	switch level {
	case models.WarningSeverityHigh:
		return []string{"merchant_admin", "site_admin"}
	case models.WarningSeverityMedium:
		return []string{"site_admin", "site_member"}
	default:
		return []string{"site_member"}
	}
}

func getNotifyMethods(level string) []string {
	switch level {
	case models.WarningSeverityHigh:
		return []string{"system_message", "wechat", "email"}
	case models.WarningSeverityMedium:
		return []string{"system_message", "wechat"}
	default:
		return []string{"system_message"}
	}
}

func sendSystemMessage(warning *models.Warning, role string) {
	db := database.GetDB()
	var members []struct {
		UserID string
	}
	if err := db.Table("site_members").
		Select("user_id").
		Where("site_id = ? AND role = ?", warning.SiteID, role).
		Find(&members).Error; err != nil {
		log.Printf("[WarningNotify] Failed to query members for site %s: %v", warning.SiteID, err)
		return
	}
	for _, m := range members {
		notif := models.Notification{
			ID:        uuid.New().String(),
			TenantID:  warning.MerchantID,
			UserID:    m.UserID,
			Type:      "warning",
			Title:     warning.Reason,
			Content:   warning.Description,
			RefID:     warning.ID,
			RefType:   "warning",
			Status:    "unread",
			CreatedAt: time.Now(),
		}
		if err := db.Create(&notif).Error; err != nil {
			log.Printf("[WarningNotify] Failed to create notification for user %s: %v", m.UserID, err)
		}
	}
}

func sendWechatAlert(warning *models.Warning, role string) {
	log.Printf("[WarningNotify] WeChat alert to %s: warning %s (%s)", role, warning.ID, warning.Reason)
}

func sendEmailAlert(warning *models.Warning, role string) {
	log.Printf("[WarningNotify] Email to %s: warning %s (%s)", role, warning.ID, warning.Reason)
}

// InitWarningScheduler starts a background goroutine for recurring alert dispatch.
func InitWarningScheduler() {
	go func() {
		log.Println("[WarningScheduler] Started (interval: 10m, tick every 10min)")
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			resendUnresolved()
		}
	}()
}

func resendUnresolved() {
	db := database.GetDB()

	var warnings []models.Warning
	db.Where("status IN ?", []string{models.WarningStatusOpen, models.WarningStatusAcknowledged}).
		Find(&warnings)

	for _, w := range warnings {
		SendWarningNotification(&w)
	}
}
