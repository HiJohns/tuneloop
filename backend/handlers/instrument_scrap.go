package handlers

import (
	"log"
	"net/http"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
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

	// Record audit log（#2062：经统一 helper 写合法 JSON，失败不阻断）
	writeAuditLog(db, tenantID, userID, "scrap_instrument", "", instrumentID, map[string]interface{}{"event": "scrap"})

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "instrument scrapped",
		"data": gin.H{
			"instrument_id": instrumentID,
			"stock_status":  models.StockStatusArchived,
		},
	})
}
