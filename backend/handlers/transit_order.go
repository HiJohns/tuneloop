package handlers

import (
	"encoding/json"
	"net/http"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

// TransitOrderReceive marks a transit order as arrived with photos.
func TransitOrderReceive(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Photos []string `json:"photos"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Photos) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "photos required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	photosJSON, _ := json.Marshal(req.Photos)

	db.Model(&models.TransitOrder{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":       models.TransitOrderArrived,
		"unpack_photos": string(photosJSON),
		"updated_at":   time.Now(),
	})
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "received"})
}

// TransitOrderRepack records repack info and marks as repacked.
func TransitOrderRepack(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Company string `json:"company"`
		Number  string `json:"number"`
		Photos  []string `json:"photos"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Number == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "tracking number required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	photosJSON, _ := json.Marshal(req.Photos)

	db.Model(&models.TransitOrder{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":                models.TransitOrderRepacked,
		"repack_company":        req.Company,
		"repack_tracking_number": req.Number,
		"unpack_photos":         string(photosJSON),
		"updated_at":            time.Now(),
	})
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "repacked"})
}

// TransitOrderShip marks as shipped (final transit).
func TransitOrderShip(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	db.Model(&models.TransitOrder{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":     models.TransitOrderShipped,
		"updated_at": time.Now(),
	})
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "shipped"})
}

// ListTransitOrders lists transit orders for a site.
func ListTransitOrders(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	query := db.Model(&models.TransitOrder{})
	if siteID := c.Query("site_id"); siteID != "" {
		query = query.Where("transit_site_id = ? OR controlled_site_id = ?", siteID, siteID)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	var orders []models.TransitOrder
	query.Order("created_at DESC").Find(&orders)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": orders}})
}
