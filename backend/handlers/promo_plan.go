package handlers

import (
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func ListPromoPlans(c *gin.Context) {
	scopeType := c.DefaultQuery("scope_type", "")
	planType := c.DefaultQuery("plan_type", "discount_policy")
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	query := db.Where("plan_type = ?", planType)
	if scopeType != "" {
		query = query.Where("scope_type = ?", scopeType)
	}
	businessRole := middleware.GetBusinessRole(ctx)
	tenantID := middleware.GetTenantID(ctx)
	if businessRole == "merchant_admin" && tenantID != "" {
		query = query.Where("(scope_type = 'system' OR (scope_type = 'merchant' AND scope_id = ?))", tenantID)
	}
	var plans []models.PromoPlan
	if err := query.Order("created_at DESC").Find(&plans).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": plans})
}

func CreatePromoPlan(c *gin.Context) {
	var req struct {
		PlanType  string  `json:"plan_type" binding:"required"`
		ScopeType string  `json:"scope_type" binding:"required"`
		ScopeID   *string `json:"scope_id"`
		Name      string  `json:"name" binding:"required"`
		StartDate *string `json:"start_date"`
		EndDate   *string `json:"end_date"`
		Stackable bool    `json:"stackable"`
		IsActive  bool    `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	if req.PlanType == "discount_policy" && req.ScopeType == "site" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "discount_policy cannot have site scope"})
		return
	}
	ctx := c.Request.Context()
	businessRole := middleware.GetBusinessRole(ctx)
	tenantID := middleware.GetTenantID(ctx)
	if req.ScopeType == "merchant" {
		if businessRole != "merchant_admin" {
			c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "only merchant_admin can create merchant-scope plans"})
			return
		}
		if req.ScopeID == nil || *req.ScopeID == "" {
			req.ScopeID = &tenantID
		}
	}
	plan := models.PromoPlan{
		PlanType:  req.PlanType,
		ScopeType: req.ScopeType,
		ScopeID:   req.ScopeID,
		Name:      req.Name,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
		Stackable: req.Stackable,
		IsActive:  req.IsActive,
	}
	db := database.GetDB().WithContext(ctx)
	if err := db.Create(&plan).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": plan})
}

func UpdatePromoPlan(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid id"})
		return
	}
	var req struct {
		Name      *string `json:"name"`
		StartDate *string `json:"start_date"`
		EndDate   *string `json:"end_date"`
		Stackable *bool   `json:"stackable"`
		IsActive  *bool   `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	updates := map[string]interface{}{}
	if req.Name != nil { updates["name"] = *req.Name }
	if req.StartDate != nil { updates["start_date"] = *req.StartDate }
	if req.EndDate != nil { updates["end_date"] = *req.EndDate }
	if req.Stackable != nil { updates["stackable"] = *req.Stackable }
	if req.IsActive != nil { updates["is_active"] = *req.IsActive }
	if err := db.Model(&models.PromoPlan{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "updated"})
}

func DeletePromoPlan(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid id"})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	if err := db.Delete(&models.PromoPlan{}, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "deleted"})
}

func GetPromoPlanDetails(c *gin.Context) {
	planID := c.Param("id")
	if _, err := uuid.Parse(planID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid id"})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	var details []models.PromoPlanDetail
	if err := db.Where("promo_plan_id = ?", planID).Find(&details).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": details})
}

func UpdatePromoPlanDetails(c *gin.Context) {
	planID := c.Param("id")
	if _, err := uuid.Parse(planID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid id"})
		return
	}
	var req struct {
		Details []struct {
			LevelID         int      `json:"level_id" binding:"required"`
			RentDiscount    *float64 `json:"rent_discount"`
			DepositDiscount *float64 `json:"deposit_discount"`
			OverdueDiscount *float64 `json:"overdue_discount"`
		} `json:"details" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tx := db.Begin()
	tx.Where("promo_plan_id = ?", planID).Delete(&models.PromoPlanDetail{})
	for _, d := range req.Details {
		detail := models.PromoPlanDetail{
			PromoPlanID:      planID,
			LevelID:          d.LevelID,
			RentDiscount:     *d.RentDiscount,
			DepositDiscount:  *d.DepositDiscount,
			OverdueDiscount:  *d.OverdueDiscount,
		}
		if err := tx.Create(&detail).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
			return
		}
	}
	tx.Commit()
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "updated"})
}
