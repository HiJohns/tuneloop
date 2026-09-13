package handlers

import (
	"net/http"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ListWarnings returns warnings filtered by query params.
func ListWarnings(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	query := db.Model(&models.Warning{})
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if level := c.Query("level"); level != "" {
		query = query.Where("level = ?", level)
	}
	if siteID := c.Query("site_id"); siteID != "" {
		query = query.Where("site_id = ?", siteID)
	}

	var warnings []models.Warning
	query.Order("level DESC, created_at DESC").Find(&warnings)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": warnings}})
}

// GetWarning returns a single warning by ID.
func GetWarning(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var w models.Warning
	if err := db.Where("id = ?", id).First(&w).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": w})
}

// CreateWarning creates a new warning record.
func CreateWarning(c *gin.Context) {
	var req struct {
		SiteID      string `json:"site_id"`
		MerchantID  string `json:"merchant_id"`
		Reason      string `json:"reason"`
		Category    string `json:"category"`
		Level       string `json:"level"`
		ObjectType  string `json:"object_type"`
		ObjectID    string `json:"object_id"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "reason is required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	w := models.Warning{
		ID:          uuid.New().String(),
		SiteID:      req.SiteID,
		MerchantID:  req.MerchantID,
		Reason:      req.Reason,
		Category:    req.Category,
		Level:       req.Level,
		ObjectType:  req.ObjectType,
		ObjectID:    req.ObjectID,
		Description: req.Description,
		Status:      models.WarningStatusOpen,
		CreatedAt:   time.Now(),
	}
	if err := db.Create(&w).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create warning"})
		return
	}

	// #1898: notify the configured recipients by e-mail. The warning record is
	// the business outcome, so a send failure is surfaced in the response
	// (never silently swallowed) rather than failing the creation.
	notify := gin.H{"sent": false}
	if sent, nerr := services.SendWarningNotification(&w); nerr != nil {
		notify["error"] = nerr.Error()
	} else if sent {
		notify["sent"] = true
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": w, "notify": notify})
}

// GetWarningSettings returns the per-level warning notification config (#1898).
// Recipients are free-form e-mail addresses (not limited to platform users).
func GetWarningSettings(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	out := make(map[string]services.WarningNotifyConfig, len(services.WarningLevels))
	for _, lv := range services.WarningLevels {
		cfg, err := services.LoadWarningNotifyConfig(db, lv)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": err.Error()})
			return
		}
		out[lv] = cfg
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": out})
}

// UpdateWarningSettings saves the per-level warning notification config (#1898).
func UpdateWarningSettings(c *gin.Context) {
	var req map[string]services.WarningNotifyConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	if len(req) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "at least one level is required"})
		return
	}
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	for level, cfg := range req {
		if err := services.SaveWarningNotifyConfig(db, level, cfg); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "saved"})
}

// UpdateWarningStatus updates a warning's status (acknowledge/resolve).
func UpdateWarningStatus(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "status required"})
		return
	}

	valid := map[string]bool{models.WarningStatusAcknowledged: true, models.WarningStatusResolved: true}
	if !valid[req.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "invalid status"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)

	updates := map[string]interface{}{
		"status": req.Status,
	}
	if req.Status == models.WarningStatusResolved {
		now := time.Now()
		updates["resolved_at"] = &now
		updates["resolved_by"] = userID
	}

	if err := db.Model(&models.Warning{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "updated"})
}
