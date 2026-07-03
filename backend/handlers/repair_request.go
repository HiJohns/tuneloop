package handlers

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
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

	// Status filter (comma-separated)
	if statusParam := c.Query("status"); statusParam != "" {
		statuses := strings.Split(statusParam, ",")
		query = query.Where("status IN ?", statuses)
	}

	if role == "USER" {
		var localUser models.User
		if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err == nil {
			query = query.Where("user_id = ?", localUser.ID)
		} else {
			query = query.Where("user_id = ?", userID)
		}
	} else {
		// Staff: filter by current user's sites
		var localUser models.User
		if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err == nil {
			var siteIDs []string
			db.Table("site_members").Where("user_id = ?", localUser.ID).Pluck("site_id", &siteIDs)
			if len(siteIDs) > 0 {
				query = query.Where("site_id IN ?", siteIDs)
			}
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
		VideoURL         string   `json:"video_url"`
		TrackingCompany  string   `json:"tracking_company"`
		TrackingNumber   string   `json:"tracking_number"`
		SN               string   `json:"sn"`
		InstrumentType   string   `json:"instrument_type"`
		Brand            string   `json:"brand"`
		Model            string   `json:"model"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)

	userInstrumentID := body.UserInstrumentID
	if userInstrumentID == "" && body.SN != "" {
		var existing models.UserInstrument
		if err := db.Where("sn = ? AND user_id = ?", body.SN, userID).First(&existing).Error; err != nil {
			newUI := models.UserInstrument{
				ID:             uuid.New().String(),
				UserID:         userID,
				SN:             body.SN,
				InstrumentType: body.InstrumentType,
				Brand:          body.Brand,
				Model:          body.Model,
				CreatedAt:      time.Now(),
			}
			db.Create(&newUI)
			userInstrumentID = newUI.ID
		} else {
			userInstrumentID = existing.ID
		}
	}

	status := models.RepairReqStatusPendingAssessment
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
		VideoURL:         body.VideoURL,
		TrackingCompany:  body.TrackingCompany,
		TrackingNumber:   body.TrackingNumber,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if err := db.Create(&req).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create"})
		return
	}

	objectID := req.ID
	if len(body.Photos) > 0 || body.VideoURL != "" {
		tenantID := middleware.GetTenantID(ctx)
		orgID := middleware.GetOrgID(ctx)
		batchID := uuid.New().String()
		seq := 0
		for _, url := range body.Photos {
			media := models.InstrumentMedia{
				TenantID:   tenantID,
				OrgID:      orgID,
				ObjectType: "repair_request",
				ObjectID:   &objectID,
				BatchID:    batchID,
				BatchType:  "repair",
				FileName:   filepath.Base(url),
				FileType:   "image",
				StorageKey: url,
				IsDisplay:  false,
				SortOrder:  seq,
			}
			db.Create(&media)
			seq++
		}
		if body.VideoURL != "" {
			media := models.InstrumentMedia{
				TenantID:   tenantID,
				OrgID:      orgID,
				ObjectType: "repair_request",
				ObjectID:   &objectID,
				BatchID:    batchID,
				BatchType:  "repair",
				FileName:   filepath.Base(body.VideoURL),
				FileType:   "video",
				StorageKey: body.VideoURL,
				IsDisplay:  false,
				SortOrder:  seq,
			}
			db.Create(&media)
		}
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

// ReturnShipping sets return tracking info and transitions repair request to returned.
func (h *RepairRequestHandler) ReturnShipping(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		ReturnCompany        string `json:"return_company"`
		ReturnTrackingNumber string `json:"return_tracking_number"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.ReturnTrackingNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "return_tracking_number required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var req models.RepairRequest
	if err := db.Where("id = ?", id).First(&req).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "not found"})
		return
	}

	if req.Status != models.RepairReqStatusReturnPend {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "only return_pending requests can update return shipping"})
		return
	}

	db.Model(&req).Updates(map[string]interface{}{
		"return_company":         body.ReturnCompany,
		"return_tracking_number": body.ReturnTrackingNumber,
		"status":                 models.RepairReqStatusReturned,
		"updated_at":             time.Now(),
	})

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "return shipping updated"})
}

// PayRepairRequest processes payment for a repair request and updates user total_spending.
// Accept quote (pending_payment → repairing): adds quote_amount
// Reject quote (pending_cancel → return_pending): adds inspection_fee
func (h *RepairRequestHandler) PayRepairRequest(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "id required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)

	var req models.RepairRequest
	if err := db.Where("id = ?", id).First(&req).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "not found"})
		return
	}

	var amount float64
	var newStatus string

	switch req.Status {
	case models.RepairReqStatusPendingPay:
		if req.QuoteAmount == nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "no quote amount"})
			return
		}
		amount = *req.QuoteAmount
		newStatus = models.RepairReqStatusRepairing
	case models.RepairReqStatusPendingCancel:
		if req.InspectionFee == nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "no inspection fee"})
			return
		}
		amount = *req.InspectionFee
		newStatus = models.RepairReqStatusReturnPend
	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "message": "not in payable status"})
		return
	}

	// Update user total_spending (excludes shipping fee)
	var localUser models.User
	if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}
	db.Model(&localUser).Update("total_spending", gorm.Expr("total_spending + ?", amount))

	// Update repair request status
	db.Model(&req).Updates(map[string]interface{}{
		"status":     newStatus,
		"updated_at": time.Now(),
	})

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "payment processed"})
}
