package handlers

import (
	"net/http"
	"strconv"

	"tuneloop-backend/middleware"
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
		Role:         middleware.GetRole(ctx),
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

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": result,
	})
}

func GetAuditLog(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")

	log, err := services.GetAuditLogByID(id,
		middleware.GetRole(ctx),
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

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": log,
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
		Role:         middleware.GetRole(ctx),
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
