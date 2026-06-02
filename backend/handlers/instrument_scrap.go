package handlers

import (
	"log"
	"net/http"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// POST /api/instruments/:id/scrap - Scrap an instrument (admin only)
func ScrapInstrument(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)
	tenantID := middleware.GetTenantID(ctx)
	instrumentID := c.Param("id")

	db := database.GetDB().WithContext(ctx)

	var instrument models.Instrument
	if err := db.Where("id = ? AND tenant_id = ?", instrumentID, tenantID).First(&instrument).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "instrument not found"})
		return
	}

	if instrument.StockStatus != models.StockStatusAvailable && instrument.StockStatus != models.StockStatusMaintenance {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "only in_store or maintenance instruments can be scrapped"})
		return
	}

	if err := db.Model(&instrument).Updates(map[string]interface{}{
		"stock_status": models.StockStatusArchived,
		"updated_at":   time.Now(),
	}).Error; err != nil {
		log.Printf("[ScrapInstrument] Failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to scrap instrument"})
		return
	}

	// Record audit log
	detailStr := "乐器已报废（scrap）"
	db.Create(&models.AuditLog{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		UserID:     userID,
		Action:     "scrap_instrument",
		ResourceID: instrumentID,
		Details:    &detailStr,
		CreatedAt:  time.Now(),
	})

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "instrument scrapped",
		"data": gin.H{
			"instrument_id": instrumentID,
			"stock_status":  models.StockStatusArchived,
		},
	})
}
