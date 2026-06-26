package handlers

import (
	"net/http"
	"strings"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func scopeTypeFromPath(path string) string {
	if strings.HasPrefix(path, "/admin/") {
		return "system"
	}
	if strings.HasPrefix(path, "/merchant/") {
		return "merchant"
	}
	if strings.HasPrefix(path, "/site/") {
		return "site"
	}
	return ""
}

func ListPointsPolicies(c *gin.Context) {
	scopeType := scopeTypeFromPath(c.Request.URL.Path)
	if scopeType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "could not determine scope from path"})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	query := db.Where("scope_type = ?", scopeType)
	tenantID := middleware.GetTenantID(ctx)
	orgID := middleware.GetOrgID(ctx)
	if scopeType == "merchant" && tenantID != "" {
		query = query.Where("scope_id = ?", tenantID)
	}
	if scopeType == "site" && orgID != "" {
		query = query.Where("scope_id = ?", orgID)
	}
	var policies []models.PointsPolicy
	if err := query.Find(&policies).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": policies})
}

func CreatePointsPolicy(c *gin.Context) {
	scopeType := scopeTypeFromPath(c.Request.URL.Path)
	if scopeType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "could not determine scope from path"})
		return
	}
	var req struct {
		ScopeID     *string `json:"scope_id"`
		MaxPayRatio float64 `json:"max_pay_ratio"`
		ValidDays   int     `json:"valid_days"`
		IsActive    bool    `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	orgID := middleware.GetOrgID(ctx)
	if req.ScopeID == nil || *req.ScopeID == "" {
		if scopeType == "merchant" && tenantID != "" {
			req.ScopeID = &tenantID
		}
		if scopeType == "site" && orgID != "" {
			req.ScopeID = &orgID
		}
	}
	policy := models.PointsPolicy{
		ScopeType:   scopeType,
		ScopeID:     req.ScopeID,
		MaxPayRatio: req.MaxPayRatio,
		ValidDays:   req.ValidDays,
		IsActive:    req.IsActive,
	}
	db := database.GetDB().WithContext(ctx)
	if err := db.Create(&policy).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": policy})
}

func UpdatePointsPolicy(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid id"})
		return
	}
	var req struct {
		MaxPayRatio *float64 `json:"max_pay_ratio"`
		ValidDays   *int     `json:"valid_days"`
		IsActive    *bool    `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	updates := map[string]interface{}{}
	if req.MaxPayRatio != nil { updates["max_pay_ratio"] = *req.MaxPayRatio }
	if req.ValidDays != nil { updates["valid_days"] = *req.ValidDays }
	if req.IsActive != nil { updates["is_active"] = *req.IsActive }
	if err := db.Model(&models.PointsPolicy{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "updated"})
}
