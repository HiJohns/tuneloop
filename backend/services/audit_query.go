package services

import (
	"fmt"
	"strings"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"gorm.io/gorm"
)

type AuditLogQuery struct {
	ResourceType string
	Action       string
	UserID       string
	DateFrom     string
	DateTo       string
	Keyword      string
	Page         int
	PageSize     int

	Role     string
	ActorID  string
	TenantID string
	OrgID    string
}

type AuditLogResult struct {
	List     []models.AuditLog `json:"list"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"pageSize"`
}

func QueryAuditLogs(q *AuditLogQuery) (*AuditLogResult, error) {
	query := database.GetDB().Model(&models.AuditLog{})

	query = applyAuditRBAC(query, q)

	if q.ResourceType != "" {
		query = query.Where("resource_type = ?", q.ResourceType)
	}
	if q.Action != "" {
		query = query.Where("action = ?", q.Action)
	}
	if q.UserID != "" {
		query = query.Where("user_id = ?", q.UserID)
	}
	if q.DateFrom != "" {
		query = query.Where("created_at >= ?", q.DateFrom)
	}
	if q.DateTo != "" {
		query = query.Where("created_at < ?", q.DateTo)
	}
	if q.Keyword != "" {
		like := "%" + q.Keyword + "%"
		query = query.Where(
			query.Where("resource_type ILIKE ?", like).
				Or("resource_id ILIKE ?", like).
				Or("action ILIKE ?", like),
		)
	}

	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count audit logs: %w", err)
	}

	offset := (page - 1) * pageSize
	var logs []models.AuditLog
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		return nil, fmt.Errorf("query audit logs: %w", err)
	}

	return &AuditLogResult{
		List:     logs,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func GetAuditLogByID(id, role, userID, tenantID string) (*models.AuditLog, error) {
	query := database.GetDB().Model(&models.AuditLog{}).Where("id = ?", id)

	q := &AuditLogQuery{Role: role, ActorID: userID, TenantID: tenantID}
	query = applyAuditRBAC(query, q)

	var log models.AuditLog
	if err := query.First(&log).Error; err != nil {
		return nil, err
	}
	return &log, nil
}

func ExportAuditLogs(q *AuditLogQuery) (string, error) {
	query := database.GetDB().Model(&models.AuditLog{})

	query = applyAuditRBAC(query, q)

	if q.ResourceType != "" {
		query = query.Where("resource_type = ?", q.ResourceType)
	}
	if q.Action != "" {
		query = query.Where("action = ?", q.Action)
	}
	if q.UserID != "" {
		query = query.Where("user_id = ?", q.UserID)
	}
	if q.DateFrom != "" {
		query = query.Where("created_at >= ?", q.DateFrom)
	}
	if q.DateTo != "" {
		query = query.Where("created_at < ?", q.DateTo)
	}
	if q.Keyword != "" {
		like := "%" + q.Keyword + "%"
		query = query.Where(
			query.Where("resource_type ILIKE ?", like).
				Or("resource_id ILIKE ?", like).
				Or("action ILIKE ?", like),
		)
	}

	var logs []models.AuditLog
	if err := query.Order("created_at DESC").Find(&logs).Error; err != nil {
		return "", fmt.Errorf("export audit logs: %w", err)
	}

	var buf strings.Builder
	buf.WriteString("Time,UserID,ActorRole,Action,ResourceType,ResourceID,IPAddress\n")
	for _, l := range logs {
		buf.WriteString(fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s\n",
			l.CreatedAt.Format("2006-01-02 15:04:05"),
			l.UserID,
			l.ActorRole,
			l.Action,
			l.ResourceType,
			l.ResourceID,
			l.IPAddress,
		))
	}
	return buf.String(), nil
}

func applyAuditRBAC(query *gorm.DB, q *AuditLogQuery) *gorm.DB {
	role := q.Role
	userID := q.ActorID
	tenantID := q.TenantID
	orgID := q.OrgID

	switch role {
	case "ADMIN", "OWNER":
		if tenantID != "" {
			return query.Where("tenant_id = ?", tenantID)
		}
	case "site_admin":
		if orgID != "" {
			return query.Where("org_id = ?", orgID)
		}
		return query.Where("user_id = ?", userID)
	default:
		return query.Where("user_id = ?", userID)
	}

	return query
}
