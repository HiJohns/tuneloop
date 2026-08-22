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
// tenantID must be provided explicitly (avoid auto-scoping issues with cross-tenant notifications).
// Notify creates a notification. Optional variadic arg: actionType string
// (e.g. "repair_request", "payment") — stored in ActionType for frontend
// action-button rendering (default "info").
func Notify(db *gorm.DB, tenantID, userID, ntype, title, content, refID, refType string, action ...string) {
	actionType := "info"
	if len(action) > 0 && action[0] != "" {
		actionType = action[0]
	}
	notif := models.Notification{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		UserID:     userID,
		Type:       ntype,
		Title:      title,
		Content:    content,
		RefID:      refID,
		RefType:    refType,
		ActionType: actionType,
		Status:     "unread",
		CreatedAt:  time.Now(),
	}
	if err := db.Create(&notif).Error; err != nil {
		log.Printf("[Notify] Failed to create notification for user %s: %v", userID, err)
	}
	log.Printf("[Notify] WeChat stub: would send template message to user %s (type=%s, ref=%s)", userID, ntype, refID)
}

// NotifyUsersBySite sends a notification to all site_members with the given roles at a site.
func NotifyUsersBySite(db *gorm.DB, tenantID, siteID, ntype, title, content, refID, refType string, roles []string) {
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
		Notify(db, tenantID, m.UserID, ntype, title, content, refID, refType)
	}
}

// NotifyUsersBySiteWithAction sends notifications to site members with
// ActionType and ActionData support (for action buttons on MessageDetail).
func NotifyUsersBySiteWithAction(db *gorm.DB, tenantID, siteID, ntype, title, content, refID, refType string, roles []string, actionType string, actionData *string) {
	var members []struct {
		UserID string
	}
	if err := db.Table("site_members").
		Select("user_id").
		Where("site_id = ? AND role IN ?", siteID, roles).
		Find(&members).Error; err != nil {
		log.Printf("[NotifyUsersBySiteWithAction] Failed to query site_members for site %s: %v", siteID, err)
		return
	}
	// Resolve the site's org_id for the notification (notifications.org_id
	// is a uuid column — empty string raises SQLSTATE 22P02).
	orgID := ""
	var site struct {
		OrgID string
	}
	if err := db.Table("sites").Select("org_id").Where("id = ?", siteID).First(&site).Error; err == nil {
		orgID = site.OrgID
	}
	for _, m := range members {
		notif := models.Notification{
			ID:         uuid.New().String(),
			TenantID:   tenantID,
			OrgID:      orgID,
			UserID:     m.UserID,
			Type:       ntype,
			Title:      title,
			Content:    content,
			RefID:      refID,
			RefType:    refType,
			ActionType: actionType,
			ActionData: actionData,
			Status:     "unread",
			CreatedAt:  time.Now(),
		}
		if err := db.Create(&notif).Error; err != nil {
			log.Printf("[NotifyUsersBySiteWithAction] Failed to create notification for user %s: %v", m.UserID, err)
		}
	}
}

// NotifyTechniciansOfSite sends a notification to all repair_technicians at a site.
func NotifyTechniciansOfSite(db *gorm.DB, tenantID, siteID, ntype, title, content, refID, refType string) {
	NotifyUsersBySite(db, tenantID, siteID, ntype, title, content, refID, refType, []string{"repair_technician"})
}

// NotifyMerchantAdmins sends a notification to all merchant admin users within a tenant.
// Optional variadic args (mirroring Notify): actionType string, actionData *string.
func NotifyMerchantAdmins(db *gorm.DB, tenantID, ntype, title, content, refID, refType string, action ...interface{}) {
	var admins []struct {
		ID    string
		OrgID string
	}
	if err := db.Table("users").
		Select("id, org_id").
		Where("tenant_id = ? AND role IN ?", tenantID, []string{"OWNER", "ADMIN"}).
		Find(&admins).Error; err != nil {
		log.Printf("[NotifyMerchantAdmins] Failed to query admins for tenant %s: %v", tenantID, err)
		return
	}
	actionType := "info"
	var actionData *string
	if len(action) > 0 {
		if v, ok := action[0].(string); ok && v != "" {
			actionType = v
		}
	}
	if len(action) > 1 {
		if v, ok := action[1].(*string); ok {
			actionData = v
		}
	}
	for _, a := range admins {
		orgID := a.OrgID
		if orgID == "" {
			orgID = "00000000-0000-0000-0000-000000000000"
		}
		notif := models.Notification{
			ID:         uuid.New().String(),
			TenantID:   tenantID,
			OrgID:      orgID,
			UserID:     a.ID,
			Type:       ntype,
			Title:      title,
			Content:    content,
			RefID:      refID,
			RefType:    refType,
			ActionType: actionType,
			ActionData: actionData,
			Status:     "unread",
			CreatedAt:  time.Now(),
		}
		if err := db.Create(&notif).Error; err != nil {
			log.Printf("[NotifyMerchantAdmins] Failed to create notification for user %s: %v", a.ID, err)
		}
	}
}
