package services

import (
	"log"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/models"
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
	// Create a system notification record — simplified stub
	log.Printf("[WarningNotify] System message to %s: warning %s (%s)", role, warning.ID, warning.Reason)
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
