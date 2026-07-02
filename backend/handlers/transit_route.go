package handlers

import (
	"net/http"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ListTransitRoutes returns all transit route mappings.
func ListTransitRoutes(c *gin.Context) {
	db := database.GetDB()
	var routes []models.TransitRoute
	db.Order("priority ASC").Find(&routes)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": routes}})
}

// CreateTransitRoute creates a transit route mapping.
func CreateTransitRoute(c *gin.Context) {
	var req struct {
		ControlledSiteID string `json:"controlled_site_id" binding:"required"`
		TransitSiteID    string `json:"transit_site_id" binding:"required"`
		Priority         int    `json:"priority"`
		IsDefault        bool   `json:"is_default"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "invalid request"})
		return
	}

	route := models.TransitRoute{
		ID:               uuid.New().String(),
		ControlledSiteID: req.ControlledSiteID,
		TransitSiteID:    req.TransitSiteID,
		Priority:         req.Priority,
		IsDefault:        req.IsDefault,
		CreatedAt:        time.Now(),
	}

	db := database.GetDB()
	if err := db.Create(&route).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": route})
}

// DeleteTransitRoute removes a transit route mapping.
func DeleteTransitRoute(c *gin.Context) {
	id := c.Param("id")
	db := database.GetDB()
	if err := db.Where("id = ?", id).Delete(&models.TransitRoute{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to delete"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "deleted"})
}
