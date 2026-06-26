package handlers

import (
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func GetInstrumentPromoOverrides(c *gin.Context) {
	instrumentID := c.Param("id")
	if _, err := uuid.Parse(instrumentID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid instrument id"})
		return
	}
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	db := database.GetDB().WithContext(ctx)
	var overrides []models.InstrumentPromoOverride
	if err := db.Where("tenant_id = ? AND instrument_id = ?", tenantID, instrumentID).Find(&overrides).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": overrides})
}

func UpdateInstrumentPromoOverride(c *gin.Context) {
	instrumentID := c.Param("id")
	if _, err := uuid.Parse(instrumentID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid instrument id"})
		return
	}
	var req struct {
		OverrideType string `json:"override_type" binding:"required"`
		Enabled      bool   `json:"enabled" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	if req.OverrideType != "discount" && req.OverrideType != "rebate" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "override_type must be discount or rebate"})
		return
	}
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	db := database.GetDB().WithContext(ctx)

	var existing models.InstrumentPromoOverride
	result := db.Where("tenant_id = ? AND instrument_id = ? AND override_type = ?", tenantID, instrumentID, req.OverrideType).First(&existing)
	if result.Error != nil {
		override := models.InstrumentPromoOverride{
			TenantID:     tenantID,
			InstrumentID: instrumentID,
			OverrideType: req.OverrideType,
			Enabled:      req.Enabled,
		}
		if err := db.Create(&override).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
			return
		}
	} else {
		if err := db.Model(&existing).Update("enabled", req.Enabled).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "updated"})
}
