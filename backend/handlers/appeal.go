package handlers

import (
	"net/http"
	"strconv"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AppealHandler struct{}

func NewAppealHandler() *AppealHandler {
	return &AppealHandler{}
}

// GET /api/merchant/appeals - Get appeal list
func (h *AppealHandler) ListAppeals(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	status := c.Query("status")
	siteID := c.Query("site_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	db := database.GetDB().WithContext(ctx)

	query := db.Model(&models.Appeal{}).Where("tenant_id = ?", tenantID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if siteID != "" {
		query = query.Where("site_id = ?", siteID)
	}

	var total int64
	query.Count(&total)

	offset := (page - 1) * pageSize
	var appeals []models.Appeal
	query.Offset(offset).Limit(pageSize).Find(&appeals)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":     appeals,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		},
	})
}

// GET /api/merchant/appeals/:id - Get appeal details
func (h *AppealHandler) GetAppeal(c *gin.Context) {
	appealID := c.Param("id")
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	db := database.GetDB().WithContext(ctx)

	var appeal models.Appeal
	if err := db.Where("id = ? AND tenant_id = ?", appealID, tenantID).First(&appeal).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "appeal not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": appeal,
	})
}

// PUT /api/merchant/appeals/:id/resolve - Resolve appeal (arbitration)
func (h *AppealHandler) ResolveAppeal(c *gin.Context) {
	var req struct {
		Decision     string  `json:"decision" binding:"required"` // no_damage, adjust, confirm
		AdjustAmount float64 `json:"adjust_amount"`
		Comment      string  `json:"comment" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	if req.Decision != "no_damage" && req.Decision != "adjust" && req.Decision != "confirm" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "decision must be no_damage, adjust, or confirm"})
		return
	}

	appealID := c.Param("id")
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	db := database.GetDB().WithContext(ctx)

	// Get appeal with damage report
	var appeal models.Appeal
	if err := db.Where("id = ? AND tenant_id = ?", appealID, tenantID).First(&appeal).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "appeal not found"})
		return
	}

	// Update appeal
	appeal.Status = "resolved"
	appeal.Resolution = req.Decision
	appeal.FinalAmount = &req.AdjustAmount
	appeal.ManagerComment = req.Comment
	now := time.Now()
	appeal.ResolvedAt = &now

	// Process based on decision
	switch req.Decision {
	case "no_damage":
		// Cancel damage report, no deduction
		appeal.FinalAmount = float64Ptr(0)
	case "adjust":
		// Update damage amount
		if req.AdjustAmount <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "adjust_amount required for adjust decision"})
			return
		}
	case "confirm":
		// Keep original damage amount
		// Get original damage report
		var damageReport models.DamageReport
		if err := db.Where("id = ?", appeal.DamageReportID).First(&damageReport).Error; err == nil {
			appeal.FinalAmount = damageReport.DamageAmount
		}
	}

	if err := db.Save(&appeal).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to resolve appeal: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data":    appeal,
	})
}

// POST /api/user/appeals - Submit appeal
func (h *AppealHandler) SubmitAppeal(c *gin.Context) {
	var req struct {
		DamageReportID string `json:"damage_report_id" binding:"required"`
		AppealReason   string `json:"appeal_reason" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	userID := middleware.GetUserID(ctx)

	db := database.GetDB().WithContext(ctx)

	// Verify damage report exists and belongs to user
	var damageReport models.DamageReport
	if err := db.Where("id = ? AND tenant_id = ? AND user_id = ?", req.DamageReportID, tenantID, userID).First(&damageReport).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "damage report not found"})
		return
	}

	// Create appeal
	appeal := models.Appeal{
		ID:             uuid.New().String(),
		TenantID:       tenantID,
		DamageReportID: req.DamageReportID,
		UserID:         userID,
		AppealReason:   req.AppealReason,
		Status:         "pending",
		SubmittedAt:    time.Now(),
	}

	if err := db.Create(&appeal).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create appeal: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":    20000,
		"message": "success",
		"data":    appeal,
	})
}

// POST /api/user/appeals/:damage_id/agree - Agree to damage assessment
func (h *AppealHandler) AgreeDamage(c *gin.Context) {
	damageID := c.Param("damage_id")
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	userID := middleware.GetUserID(ctx)

	db := database.GetDB().WithContext(ctx)

	// Verify damage report exists and belongs to user
	var damageReport models.DamageReport
	if err := db.Where("id = ? AND tenant_id = ? AND user_id = ?", damageID, tenantID, userID).First(&damageReport).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "damage report not found"})
		return
	}

	// Update damage report status
	damageReport.Status = "agreed"
	if err := db.Save(&damageReport).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update damage report: " + err.Error()})
		return
	}

	// TODO: Process deposit deduction and refund

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data":    damageReport,
	})
}

func float64Ptr(f float64) *float64 {
	return &f
}
