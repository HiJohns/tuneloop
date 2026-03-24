package handlers

import (
	"net/http"
	"strconv"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type DashboardHandler struct {
	db *gorm.DB
}

func NewDashboardHandler(db *gorm.DB) *DashboardHandler {
	return &DashboardHandler{db: db}
}

func (h *DashboardHandler) GetDashboardStats(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "tenant_id not found"})
		return
	}

	type StatsResult struct {
		TotalAssets  int     `json:"total_assets"`
		RentalRate   float64 `json:"rental_rate"`
		TransferRate float64 `json:"transfer_rate"`
		TotalRevenue float64 `json:"total_revenue"`
	}

	var stats StatsResult

	// Count total assets for the tenant
	var totalAssets int64
	if err := h.db.Model(&models.Instrument{}).Where("tenant_id = ?", tenantID).Count(&totalAssets).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch total assets: " + err.Error(),
		})
		return
	}
	stats.TotalAssets = int(totalAssets)

	// Calculate rental rate (percentage of assets that are rented)
	var rentedAssets int64
	h.db.Model(&models.Instrument{}).Where("tenant_id = ? AND stock_status = ?", tenantID, "rented").Count(&rentedAssets)

	if stats.TotalAssets > 0 {
		stats.RentalRate = (float64(rentedAssets) / float64(stats.TotalAssets)) * 100
	} else {
		stats.RentalRate = 0
	}

	// Placeholder for transfer rate (to be implemented when transfer data is available)
	stats.TransferRate = 8.0

	// Placeholder for total revenue (to be calculated from orders)
	var totalRevenue float64
	h.db.Model(&models.Order{}).Where("tenant_id = ? AND status = ?", tenantID, "completed").Select("SUM(monthly_rent)").Scan(&totalRevenue)
	stats.TotalRevenue = totalRevenue

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": stats,
	})
}

func (h *DashboardHandler) GetNearTransfers(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "tenant_id not found"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	var transfers []interface{}
	// For now, return empty list as we don't have near transfer data yet
	// TODO: Implement real near transfer logic when available

	var total int64
	total = 0

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":  transfers,
			"total": total,
			"page":  page,
			"size":  pageSize,
		},
	})
}
