package services

import (
	"log"
	"time"
	"tuneloop-backend/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Notify creates a notification for a user (dual-channel: system message + WeChat).
// WeChat sending is a log stub until template IDs and WeChat API client are configured.
func Notify(db *gorm.DB, userID, ntype, title, content, refID, refType string) {
	notif := models.Notification{
		ID:        uuid.New().String(),
		UserID:    userID,
		Type:      ntype,
		Title:     title,
		Content:   content,
		RefID:     refID,
		RefType:   refType,
		Status:    "unread",
		CreatedAt: time.Now(),
	}
	if err := db.Create(&notif).Error; err != nil {
		log.Printf("[Notify] Failed to create notification for user %s: %v", userID, err)
	}
	log.Printf("[Notify] WeChat stub: would send template message to user %s (type=%s, ref=%s)", userID, ntype, refID)
}

// NotifyUsersBySite sends a notification to all site_members with the given roles at a site.
func NotifyUsersBySite(db *gorm.DB, siteID, ntype, title, content, refID, refType string, roles []string) {
	var members []struct {
		UserID string
	}
	if err := db.Table("site_members").
		Select("user_id").
		Where("site_id = ? AND role IN ?", siteID, roles).
		Find(&members).Error; err != nil {
		log.Printf("[NotifyUsersBySite] Failed to query site_members for site %s: %v", siteID, err)
		return
	}
	for _, m := range members {
		Notify(db, m.UserID, ntype, title, content, refID, refType)
	}
}

// NotifyTechniciansOfSite sends a notification to all repair_technicians at a site.
func NotifyTechniciansOfSite(db *gorm.DB, siteID, ntype, title, content, refID, refType string) {
	NotifyUsersBySite(db, siteID, ntype, title, content, refID, refType, []string{"repair_technician"})
}

// NotifyMerchantAdmins sends a notification to all merchant admin users within a tenant.
func NotifyMerchantAdmins(db *gorm.DB, tenantID, ntype, title, content, refID, refType string) {
	var admins []struct {
		ID string
	}
	if err := db.Table("users").
		Select("id").
		Where("tenant_id = ? AND role IN ?", tenantID, []string{"OWNER", "ADMIN"}).
		Find(&admins).Error; err != nil {
		log.Printf("[NotifyMerchantAdmins] Failed to query admins for tenant %s: %v", tenantID, err)
		return
	}
	for _, a := range admins {
		Notify(db, a.ID, ntype, title, content, refID, refType)
	}
}
