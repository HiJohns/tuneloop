package handlers

import (
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

func GetMerchantRebateOptIn(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	if tenantID == "" {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "tenant required"})
		return
	}
	db := database.GetDB().WithContext(ctx)
	var merchant models.Merchant
	if err := db.Where("tenant_id = ?", tenantID).First(&merchant).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "merchant not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"rebate_opt_in": merchant.RebateOptIn}})
}

func UpdateMerchantRebateOptIn(c *gin.Context) {
	var req struct {
		OptIn bool `json:"opt_in" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	if tenantID == "" {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "tenant required"})
		return
	}
	db := database.GetDB().WithContext(ctx)
	if err := db.Model(&models.Merchant{}).Where("tenant_id = ?", tenantID).Update("rebate_opt_in", req.OptIn).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "updated"})
}
