package handlers

import (
	"encoding/json"
	"net/http"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type RepairRequestHandler struct{}

func NewRepairRequestHandler() *RepairRequestHandler {
	return &RepairRequestHandler{}
}

// List returns repair requests visible to the current user.
func (h *RepairRequestHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)
	role := middleware.GetRole(c.Request.Context())

	var requests []models.RepairRequest
	query := db.Model(&models.RepairRequest{})

	if role == "USER" {
		var localUser models.User
		if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err == nil {
			query = query.Where("user_id = ?", localUser.ID)
		} else {
			query = query.Where("user_id = ?", userID)
		}
	}

	query.Order("created_at DESC").Find(&requests)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": requests}})
}

// Get returns a single repair request by ID.
func (h *RepairRequestHandler) Get(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "id is required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var req models.RepairRequest
	if err := db.Where("id = ?", id).First(&req).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": req})
}

// Create creates a new repair request.
func (h *RepairRequestHandler) Create(c *gin.Context) {
	var body struct {
		UserInstrumentID string   `json:"user_instrument_id"`
		SiteID           string   `json:"site_id"`
		Description      string   `json:"description"`
		Photos           []string `json:"photos"`
		TrackingCompany  string   `json:"tracking_company"`
		TrackingNumber   string   `json:"tracking_number"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)

	status := models.RepairReqStatusPendingShip
	if body.TrackingNumber != "" {
		status = models.RepairReqStatusShipping
	}

	photosJSON, _ := json.Marshal(body.Photos)

	req := models.RepairRequest{
		ID:               uuid.New().String(),
		TenantID:         middleware.GetTenantID(ctx),
		SiteID:           body.SiteID,
		UserID:           userID,
		UserInstrumentID: body.UserInstrumentID,
		Status:           status,
		Description:      body.Description,
		Photos:           string(photosJSON),
		TrackingCompany:  body.TrackingCompany,
		TrackingNumber:   body.TrackingNumber,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if err := db.Create(&req).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": req})
}

// UpdateTracking updates the tracking info for a pending_ship request.
func (h *RepairRequestHandler) UpdateTracking(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		TrackingCompany string `json:"tracking_company"`
		TrackingNumber  string `json:"tracking_number"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.TrackingNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "tracking_number required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var req models.RepairRequest
	if err := db.Where("id = ?", id).First(&req).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "not found"})
		return
	}

	if req.Status != models.RepairReqStatusPendingShip {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "only pending_ship requests can update tracking"})
		return
	}

	db.Model(&req).Updates(map[string]interface{}{
		"tracking_company": body.TrackingCompany,
		"tracking_number":  body.TrackingNumber,
		"status":           models.RepairReqStatusShipping,
		"updated_at":       time.Now(),
	})

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "updated"})
}

// ListRecords returns records for a repair request.
func (h *RepairRequestHandler) ListRecords(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var records []models.RepairRequestRecord
	if err := db.Where("repair_request_id = ?", id).Order("created_at ASC").Find(&records).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"records": records}})
}

// UserInstrumentLookup looks up user instruments by SN.
func (h *RepairRequestHandler) UserInstrumentLookup(c *gin.Context) {
	sn := c.Query("sn")
	if sn == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "sn is required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)

	var localUser models.User
	if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"instrument": nil}})
		return
	}

	var instr models.UserInstrument
	if err := db.Where("user_id = ? AND sn = ?", localUser.ID, sn).First(&instr).Error; err == nil {
		c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"instrument": instr}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"instrument": nil}})
}
