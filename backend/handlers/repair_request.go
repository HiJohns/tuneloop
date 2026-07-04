package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

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
		query = query.Where("user_id = ?", userID)
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
	enriched := enrichRepairRequestList(db, requests)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": enriched}})
}

// enrichRepairRequestList enriches a slice of RepairRequests with resolved names
// for instrument, site, and merchant using batch lookups to avoid N+1 queries.
func enrichRepairRequestList(db *gorm.DB, requests []models.RepairRequest) []gin.H {
	if len(requests) == 0 {
		return []gin.H{}
	}

	// Collect unique IDs for batch lookups
	uiIDs := make(map[string]bool)
	siteIDs := make(map[string]bool)
	tenantIDs := make(map[string]bool)
	userIDs := make(map[string]bool)
	for _, r := range requests {
		if r.UserInstrumentID != "" {
			uiIDs[r.UserInstrumentID] = true
		}
		if r.SiteID != "" {
			siteIDs[r.SiteID] = true
		}
		if r.TenantID != "" {
			tenantIDs[r.TenantID] = true
		}
		if r.UserID != "" {
			userIDs[r.UserID] = true
		}
	}

	// Batch lookups
	uiMap := make(map[string]models.UserInstrument)
	if len(uiIDs) > 0 {
		var uis []models.UserInstrument
		db.Where("id IN ?", keys(uiIDs)).Find(&uis)
		for _, ui := range uis {
			uiMap[ui.ID] = ui
		}
	}

	siteMap := make(map[string]models.Site)
	if len(siteIDs) > 0 {
		var sites []models.Site
		db.Where("id IN ?", keys(siteIDs)).Find(&sites)
		for _, s := range sites {
			siteMap[s.ID] = s
		}
	}

	tenantMap := make(map[string]models.Tenant)
	if len(tenantIDs) > 0 {
		var tenants []models.Tenant
		db.Where("id IN ?", keys(tenantIDs)).Find(&tenants)
		for _, t := range tenants {
			tenantMap[t.ID] = t
		}
	}

	userMap := make(map[string]models.User)
	if len(userIDs) > 0 {
		var users []models.User
		database.GetDB().Where("iam_sub IN ?", keys(userIDs)).Find(&users)
		for _, u := range users {
			userMap[u.IAMSub] = u
		}
	}

	// Build enriched response
	result := make([]gin.H, len(requests))
	for i, r := range requests {
		instrumentSN := ""
		instrumentType := ""
		brand := ""
		model := ""
		if ui, ok := uiMap[r.UserInstrumentID]; ok {
			instrumentSN = ui.SN
			instrumentType = ui.InstrumentType
			brand = ui.Brand
			model = ui.Model
		}

		siteName := ""
		if s, ok := siteMap[r.SiteID]; ok {
			siteName = s.Name
		}

		merchantName := ""
		if t, ok := tenantMap[r.TenantID]; ok {
			merchantName = t.Name
		}

		reporterName := ""
		if u, ok := userMap[r.UserID]; ok {
			reporterName = u.Name
			if reporterName == "" {
				reporterName = u.Username
			}
			if reporterName == "" {
				reporterName = u.Phone
			}
		}

		result[i] = gin.H{
			"id":                     r.ID,
			"tenant_id":              r.TenantID,
			"site_id":                r.SiteID,
			"user_id":                r.UserID,
			"user_instrument_id":     r.UserInstrumentID,
			"status":                 r.Status,
			"description":            r.Description,
			"photos":                 r.Photos,
			"video_url":              r.VideoURL,
			"quote_amount":           r.QuoteAmount,
			"inspection_fee":         r.InspectionFee,
			"shipping_fee":           r.ShippingFee,
			"tracking_company":       r.TrackingCompany,
			"tracking_number":        r.TrackingNumber,
			"return_company":         r.ReturnCompany,
			"return_tracking_number": r.ReturnTrackingNumber,
			"worker_id":              r.WorkerID,
			"created_at":             r.CreatedAt,
			"updated_at":             r.UpdatedAt,
			"closed_at":              r.ClosedAt,
			"instrument_sn":          instrumentSN,
			"instrument_type":        instrumentType,
			"brand":                  brand,
			"model":                  model,
			"site_name":              siteName,
			"merchant_name":          merchantName,
			"reporter_name":          reporterName,
		}
	}
	return result
}

// keys returns the keys of a map as a slice.
func keys[K comparable, V any](m map[K]V) []K {
	result := make([]K, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
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

	instrumentSN, instrumentType, brand, model, siteName, merchantName, reporterName := resolveRepairMeta(db, req)

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
		"id":                     req.ID,
		"tenant_id":              req.TenantID,
		"site_id":                req.SiteID,
		"user_id":                req.UserID,
		"user_instrument_id":     req.UserInstrumentID,
		"status":                 req.Status,
		"description":            req.Description,
		"photos":                 req.Photos,
		"video_url":              req.VideoURL,
		"quote_amount":           req.QuoteAmount,
		"inspection_fee":         req.InspectionFee,
		"shipping_fee":           req.ShippingFee,
		"tracking_company":       req.TrackingCompany,
		"tracking_number":        req.TrackingNumber,
		"return_company":         req.ReturnCompany,
		"return_tracking_number": req.ReturnTrackingNumber,
		"worker_id":              req.WorkerID,
		"created_at":             req.CreatedAt,
		"updated_at":             req.UpdatedAt,
		"closed_at":              req.ClosedAt,
		"instrument_sn":          instrumentSN,
		"instrument_type":        instrumentType,
		"brand":                  brand,
		"model":                  model,
		"site_name":              siteName,
		"merchant_name":          merchantName,
		"reporter_name":          reporterName,
	}})
}

// Create creates a new repair request.
func (h *RepairRequestHandler) Create(c *gin.Context) {
	var body struct {
		UserInstrumentID string   `json:"user_instrument_id"`
		SiteID           string   `json:"site_id"`
		MerchantType     string   `json:"merchant_type"`
		TransitSiteID    string   `json:"transit_site_id"`
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
			if err := db.Create(&newUI).Error; err != nil {
				log.Printf("[RepairRequest.Create] user_instrument create failed: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create user instrument"})
				return
			}
			userInstrumentID = newUI.ID
		} else {
			userInstrumentID = existing.ID
		}
	}

	if userInstrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "user_instrument_id or sn is required"})
		return
	}

	tenantID := middleware.GetTenantID(ctx)
	orgID := middleware.GetOrgID(ctx)
	if tenantID == "" {
		if body.SiteID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "site_id is required"})
			return
		}
		var site models.Site
		if err := db.Where("id = ?", body.SiteID).First(&site).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "site not found"})
			return
		}
		tenantID = site.TenantID
		orgID = site.OrgID
	}

	merchantType := body.MerchantType
	if merchantType == "" {
		merchantType = models.MerchantTypeFull
	}

	status := models.RepairReqStatusPendingAssessment
	var expireAt *time.Time
	if merchantType == models.MerchantTypeControlled && body.TransitSiteID != "" {
		status = models.RepairReqStatusTransitProcessing
	} else {
		// Full authority: enter pending_assessment directly, set 5-business-day expiry
		now := time.Now()
		exp := now.AddDate(0, 0, 7) // simple approximation: 5 business days ≈ 7 calendar days
		expireAt = &exp
	}
	if body.TrackingNumber != "" && status != models.RepairReqStatusTransitProcessing {
		status = models.RepairReqStatusShipping
	}

	photosJSON, _ := json.Marshal(body.Photos)

	req := models.RepairRequest{
		ID:               uuid.New().String(),
		TenantID:         tenantID,
		SiteID:           body.SiteID,
		UserID:           userID,
		UserInstrumentID: userInstrumentID,
		Status:           status,
		MerchantType:     merchantType,
		TransitSiteID:    body.TransitSiteID,
		ExpireAt:         expireAt,
		Description:      body.Description,
		Photos:           string(photosJSON),
		VideoURL:         body.VideoURL,
		TrackingCompany:  body.TrackingCompany,
		TrackingNumber:   body.TrackingNumber,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if err := db.Create(&req).Error; err != nil {
		log.Printf("[RepairRequest.Create] insert failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create repair request"})
		return
	}

	initRecord := models.RepairRequestRecord{
		ID:              uuid.New().String(),
		RepairRequestID: req.ID,
		WorkerID:        userID,
		Comment:         "报修单已创建",
		Photos:          string(photosJSON),
		RecordType:      "created",
		CreatedAt:       time.Now(),
	}
	db.Create(&initRecord)

	objectID := req.ID
	if len(body.Photos) > 0 || body.VideoURL != "" {
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

func (h *RepairRequestHandler) AddRecord(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)

	var body struct {
		Comment  string   `json:"comment"`
		Photos   []string `json:"photos"`
		VideoURL string   `json:"video_url"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request"})
		return
	}

	if body.Comment == "" && len(body.Photos) == 0 && body.VideoURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "comment, photos, or video_url is required"})
		return
	}

	var req models.RepairRequest
	if err := db.Where("id = ?", id).First(&req).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "repair request not found"})
		return
	}

	orgID := middleware.GetOrgID(ctx)
	if orgID == "" {
		var site models.Site
		if err := db.Where("id = ?", req.SiteID).First(&site).Error; err == nil {
			orgID = site.OrgID
		}
	}

	photosJSON, _ := json.Marshal(body.Photos)
	record := models.RepairRequestRecord{
		ID:              uuid.New().String(),
		RepairRequestID: id,
		WorkerID:        userID,
		Comment:         body.Comment,
		Photos:          string(photosJSON),
		RecordType:      "progress",
		CreatedAt:       time.Now(),
	}
	if err := db.Create(&record).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create record"})
		return
	}

	// Video replacement logic
	if body.VideoURL != "" {
		if req.VideoURL != "" && req.VideoURL != body.VideoURL {
			if err := services.NewMediaStorage().Delete(ctx, req.VideoURL); err != nil {
				log.Printf("[RepairRequest.AddRecord] failed to delete old video %s: %v", req.VideoURL, err)
			}
		}
		db.Model(&req).Update("video_url", body.VideoURL)
		db.Model(&req).Update("updated_at", time.Now())
	}

	// Create InstrumentMedia entries
	batchID := uuid.New().String()
	seq := 0
	for _, url := range body.Photos {
		media := models.InstrumentMedia{
			TenantID:   req.TenantID,
			OrgID:      orgID,
			ObjectType: "repair_request",
			ObjectID:   &id,
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
			TenantID:   req.TenantID,
			OrgID:      orgID,
			ObjectType: "repair_request",
			ObjectID:   &id,
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

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": record})
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

// resolveRepairMeta enriches a RepairRequest with resolved names for instrument, site,
// merchant, and reporter. All lookups are best-effort (errors silently return empty strings).
// Shared by the Get and List handlers.
func resolveRepairMeta(db *gorm.DB, req models.RepairRequest) (instrumentSN, instrumentType, brand, model, siteName, merchantName, reporterName string) {
	if req.UserInstrumentID != "" {
		var ui models.UserInstrument
		if err := db.Where("id = ?", req.UserInstrumentID).First(&ui).Error; err == nil {
			instrumentSN = ui.SN
			instrumentType = ui.InstrumentType
			brand = ui.Brand
			model = ui.Model
		}
	}

	if req.SiteID != "" {
		var site models.Site
		if err := db.Where("id = ?", req.SiteID).First(&site).Error; err == nil {
			siteName = site.Name
		}
	}

	if req.TenantID != "" {
		var tenant models.Tenant
		if err := db.Where("id = ?", req.TenantID).First(&tenant).Error; err == nil {
			merchantName = tenant.Name
		}
	}

	if req.UserID != "" {
		var user models.User
		if err := database.GetDB().Where("iam_sub = ?", req.UserID).First(&user).Error; err == nil {
			reporterName = user.Name
			if reporterName == "" {
				reporterName = user.Username
			}
			if reporterName == "" {
				reporterName = user.Phone
			}
		}
	}

	return
}
