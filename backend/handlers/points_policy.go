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
	orgID := middleware.GetOrgID(ctx)
	if scopeType == "merchant" {
		// #2251：商户策略按商户 org 锚定（共享 tenant 的商户不得共用策略）
		anchor, ok := resolveMerchantConfigAnchor(ctx)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "merchant scope not found"})
			return
		}
		query = query.Where("scope_id = ?", anchor)
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
	orgID := middleware.GetOrgID(ctx)
	if scopeType == "merchant" {
		// #2251：商户策略强制锚定到本商户 org（不接受客户端传入的 scope_id）
		anchor, ok := resolveMerchantConfigAnchor(ctx)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "merchant scope not found"})
			return
		}
		req.ScopeID = &anchor
	}
	if scopeType == "site" && (req.ScopeID == nil || *req.ScopeID == "") && orgID != "" {
		req.ScopeID = &orgID
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
	// #2251：商户/网点范围更新必须命中本商户锚（防按 id 跨商户改写）
	if scopeType := scopeTypeFromPath(c.Request.URL.Path); scopeType == "merchant" {
		anchor, ok := resolveMerchantConfigAnchor(ctx)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "merchant scope not found"})
			return
		}
		db = db.Where("scope_type = ? AND scope_id = ?", "merchant", anchor)
	}
	updates := map[string]interface{}{}
	if req.MaxPayRatio != nil {
		updates["max_pay_ratio"] = *req.MaxPayRatio
	}
	if req.ValidDays != nil {
		updates["valid_days"] = *req.ValidDays
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}
	result := db.Model(&models.PointsPolicy{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": result.Error.Error()})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "policy not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "updated"})
}
