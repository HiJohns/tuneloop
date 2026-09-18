package handlers

import (
	"context"
	"net/http"
	"strconv"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
)

func ListAuditLogs(c *gin.Context) {
	ctx := c.Request.Context()
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	q := &services.AuditLogQuery{
		ResourceType: c.Query("resource_type"),
		ResourceID:   c.Query("resource_id"),
		Action:       c.Query("action"),
		UserID:       c.Query("user_id"),
		DateFrom:     c.Query("date_from"),
		DateTo:       c.Query("date_to"),
		Keyword:      c.Query("keyword"),
		Page:         page,
		PageSize:     pageSize,
		BusinessRole: middleware.GetBusinessRole(ctx),
		ActorID:      middleware.GetUserID(ctx),
		TenantID:     middleware.GetTenantID(ctx),
		OrgID:        middleware.GetOrgID(ctx),
	}

	result, err := services.QueryAuditLogs(q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to query audit logs: " + err.Error(),
		})
		return
	}

	// #1969：展示层补充（不改库/原始字段）——姓名回填 + 动作/资源中文 + UA 摘要
	enrichAuditLogs(c.Request.Context(), result.List)
	enriched := make([]gin.H, 0, len(result.List))
	for _, lg := range result.List {
		enriched = append(enriched, auditLogView(lg))
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":     enriched,
			"total":    result.Total,
			"page":     result.Page,
			"pageSize": result.PageSize,
		},
	})
}

// enrichAuditLogs 历史行 actor_name 为空时按 users.iam_sub=user_id 回填（批量，不改库）
func enrichAuditLogs(ctx context.Context, logs []models.AuditLog) {
	missing := make([]string, 0)
	for _, lg := range logs {
		if lg.ActorName == "" && lg.UserID != "" {
			missing = append(missing, lg.UserID)
		}
	}
	if len(missing) == 0 {
		return
	}
	db := database.GetDB().WithContext(ctx)
	var users []models.User
	db.Select("iam_sub, name").Where("iam_sub IN ?", missing).Find(&users)
	nameMap := map[string]string{}
	for _, u := range users {
		if u.Name != "" {
			nameMap[u.IAMSub] = u.Name
		}
	}
	for i := range logs {
		if logs[i].ActorName == "" {
			if n, ok := nameMap[logs[i].UserID]; ok {
				logs[i].ActorName = n
			}
		}
	}
}

// auditLogView 单条日志的展示视图（原始字段 + 展示字段）
func auditLogView(lg models.AuditLog) gin.H {
	actionLabel := services.AuditActionLabel(lg.Action, lg.ResourceType)
	resourceLabel := services.AuditResourceLabel(lg.ResourceType)
	uaSummary := services.ParseUserAgent(lg.UserAgent)
	actorName := lg.ActorName
	if actorName == "" && len(lg.UserID) >= 8 {
		actorName = lg.UserID[:8] + "…"
	}
	return gin.H{
		"id": lg.ID, "created_at": lg.CreatedAt, "status": lg.Status, "status_code": lg.StatusCode,
		"actor_name": actorName, "actor_role": lg.ActorRole, "user_id": lg.UserID,
		"action": lg.Action, "action_label": actionLabel,
		"resource_type": lg.ResourceType, "resource_type_label": resourceLabel, "resource_id": lg.ResourceID,
		"ip_address": lg.IPAddress, "user_agent": lg.UserAgent, "user_agent_summary": uaSummary,
		"error_message": lg.ErrorMessage, "details": lg.Details,
	}
}

func GetAuditLog(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")

	log, err := services.GetAuditLogByID(id,
		middleware.GetBusinessRole(ctx),
		middleware.GetUserID(ctx),
		middleware.GetTenantID(ctx),
	)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "audit log not found",
		})
		return
	}

	// #1969：详情同样补充展示字段
	enrichAuditLogs(ctx, []models.AuditLog{*log})
	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": auditLogView(*log),
	})
}

func ExportAuditLogs(c *gin.Context) {
	ctx := c.Request.Context()

	var req struct {
		ResourceType string `json:"resource_type"`
		Action       string `json:"action"`
		UserID       string `json:"user_id"`
		DateFrom     string `json:"date_from"`
		DateTo       string `json:"date_to"`
		Keyword      string `json:"keyword"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "invalid request body",
		})
		return
	}

	q := &services.AuditLogQuery{
		ResourceType: req.ResourceType,
		Action:       req.Action,
		UserID:       req.UserID,
		DateFrom:     req.DateFrom,
		DateTo:       req.DateTo,
		Keyword:      req.Keyword,
		BusinessRole: middleware.GetBusinessRole(ctx),
		ActorID:      middleware.GetUserID(ctx),
		TenantID:     middleware.GetTenantID(ctx),
		OrgID:        middleware.GetOrgID(ctx),
	}

	csv, err := services.ExportAuditLogs(q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to export audit logs: " + err.Error(),
		})
		return
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=audit_logs.csv")
	c.String(http.StatusOK, csv)
}
