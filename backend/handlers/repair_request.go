package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
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

	// Batch lookup RepairTransitOrder for transit fees and order number
	reqIDs := make([]string, len(requests))
	for i, r := range requests {
		reqIDs[i] = r.ID
	}
	var transitOrders []models.RepairTransitOrder
	db.Where("repair_request_id IN ?", reqIDs).Find(&transitOrders)
	transitByReqID := make(map[string]models.RepairTransitOrder)
	for _, to := range transitOrders {
		transitByReqID[to.RepairRequestID] = to
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
		siteAddress := ""
		sitePhone := ""
		if s, ok := siteMap[r.SiteID]; ok {
			siteName = s.Name
			siteAddress = s.Address
			sitePhone = s.Phone
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
			"site_address":           siteAddress,
			"site_phone":             sitePhone,
			"merchant_name":          merchantName,
			"reporter_name":          reporterName,
			"merchant_type":          r.MerchantType,
			"accepted_quote_id":      r.AcceptedQuoteID,
			"check_fee_snapshot":     r.CheckFeeSnapshot,
			"paid_amount":            r.PaidAmount,
		}
		if to, ok := transitByReqID[r.ID]; ok {
			result[i]["transit_service_fee"] = to.TransitServiceFee
			result[i]["transit_logistics_fee"] = to.TransitLogisticsFee
			result[i]["transit_order_number"] = to.TransitOrderNumber
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

	instrumentSN, instrumentType, brand, model, siteName, siteAddress, sitePhone, merchantName, reporterName := resolveRepairMeta(db, req)

	// Look up transit order for this repair request
	var transitOrder models.RepairTransitOrder
	db.Where("repair_request_id = ?", req.ID).Limit(1).Find(&transitOrder)

	// Look up transit site info for controlled repairs
	var transitSiteName, transitSiteAddress, transitSitePhone string
	if req.TransitSiteID != nil && *req.TransitSiteID != "" {
		var transitSite models.Site
		if err := db.Where("id = ?", *req.TransitSiteID).First(&transitSite).Error; err == nil {
			transitSiteName = transitSite.Name
			transitSiteAddress = transitSite.Address
			transitSitePhone = transitSite.Phone
		}
	}

	// Look up reporter's default address for return shipping
	var reporterPhone, reporterAddress, reporterPostalCode string
	if req.UserID != "" {
		var localUser models.User
		if err := db.Where("iam_sub = ?", req.UserID).First(&localUser).Error; err == nil {
			reporterPhone = localUser.Phone
			var addr models.UserAddress
			if err := db.Where("user_id = ?", localUser.ID).Order("is_default DESC, created_at DESC").First(&addr).Error; err == nil {
				reporterPhone = addr.Phone
				reporterAddress = addr.Province + addr.City + addr.District + addr.Detail
				reporterPostalCode = addr.PostalCode
				if addr.RecipientName != "" {
					reporterName = addr.RecipientName
				}
			}
		}
	}

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
		"site_address":           siteAddress,
		"site_phone":             sitePhone,
		"merchant_name":          merchantName,
		"reporter_name":          reporterName,
		"reporter_phone":         reporterPhone,
		"reporter_address":       reporterAddress,
		"reporter_postal_code":   reporterPostalCode,
		"merchant_type":          req.MerchantType,
		"accepted_quote_id":      req.AcceptedQuoteID,
		"check_fee_snapshot":     req.CheckFeeSnapshot,
		"paid_amount":            req.PaidAmount,
		"transit_service_fee":    transitOrder.TransitServiceFee,
		"transit_logistics_fee":  transitOrder.TransitLogisticsFee,
		"transit_order_number":   transitOrder.TransitOrderNumber,
		"transit_site_name":      transitSiteName,
		"transit_site_address":   transitSiteAddress,
		"transit_site_phone":     transitSitePhone,
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

	if body.SiteID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "site_id is required"})
		return
	}

	if tenantID == "" {
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
		ExpireAt:         expireAt,
		Description:      body.Description,
		Photos:           string(photosJSON),
		VideoURL:         body.VideoURL,
		TrackingCompany:  body.TrackingCompany,
		TrackingNumber:   body.TrackingNumber,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if body.TransitSiteID != "" {
		req.TransitSiteID = &body.TransitSiteID
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
	createRepairRecord(db, id, middleware.GetUserID(ctx), "shipped", "已发货（"+body.TrackingCompany+" "+body.TrackingNumber+"）", nil)

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

	// Batch lookup worker names
	workerIDs := make([]string, 0)
	for _, r := range records {
		if r.WorkerID != "" {
			workerIDs = append(workerIDs, r.WorkerID)
		}
	}
	workerNameMap := make(map[string]string)
	if len(workerIDs) > 0 {
		var users []models.User
		db.Where("iam_sub IN ?", workerIDs).Find(&users)
		for _, u := range users {
			name := u.Name
			if name == "" {
				name = u.Username
			}
			if name == "" {
				name = u.Phone
			}
			workerNameMap[u.IAMSub] = name
		}
	}

	result := make([]gin.H, len(records))
	for i, r := range records {
		item := gin.H{
			"id":           r.ID,
			"repair_request_id": r.RepairRequestID,
			"worker_id":    r.WorkerID,
			"worker_name":  workerNameMap[r.WorkerID],
			"comment":      r.Comment,
			"photos":       r.Photos,
			"record_type":  r.RecordType,
			"created_at":   r.CreatedAt,
		}
		result[i] = item
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"records": result}})
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

// TransitProcess handles transit process for a controlled repair request (v3).
// Transit employee fills transit_service_fee and transit_logistics_fee, then fans out to controlled sites.
func (h *RepairRequestHandler) TransitProcess(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var req models.RepairRequest
	if err := db.Where("id = ?", id).First(&req).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "not found"})
		return
	}
	if req.Status != models.RepairReqStatusTransitProcessing {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "not in transit_processing status"})
		return
	}
	if req.MerchantType != models.MerchantTypeControlled {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "not a controlled repair request"})
		return
	}

	var body struct {
		TransitServiceFee   float64 `json:"transit_service_fee"`
		TransitLogisticsFee float64 `json:"transit_logistics_fee"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request"})
		return
	}

	// Create inbound transit order (pending_activation, activated when user ships)
	transitOrder := models.RepairTransitOrder{
		ID:                  uuid.New().String(),
		RepairRequestID:     id,
		TransitSiteID:       *req.TransitSiteID,
		Direction:           models.RepairTransitDirIn,
		Status:              models.RepairTransitPendingActivation,
		TransitServiceFee:   &body.TransitServiceFee,
		TransitLogisticsFee: &body.TransitLogisticsFee,
		CreatedAt:           time.Now(),
	}
	if err := db.Create(&transitOrder).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create transit order"})
		return
	}

	// Transition to pending_assessment (fanned out to all controlled sites)
	now := time.Now()
	exp := now.AddDate(0, 0, 7)
	db.Model(&req).Updates(map[string]interface{}{
		"status":     models.RepairReqStatusPendingAssessment,
		"expire_at":  &exp,
		"updated_at": now,
	})

	// Notify technicians at all controlled sites associated with this transit site
	var routes []models.TransitRoute
	if err := db.Where("transit_site_id = ?", req.TransitSiteID).Find(&routes).Error; err == nil {
		title := "有新报修单待报价"
		content := "中转网点已处理完成，请查看并提交报价。"
		for _, route := range routes {
			services.NotifyTechniciansOfSite(db, req.TenantID, route.ControlledSiteID, "new_repair", title, content, req.ID, "repair_request")
		}
	}

	createRepairRecord(db, id, middleware.GetUserID(ctx), "transit_processed", "中转处理完成", nil)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "transit process completed, repair request fanned out to controlled sites"})
}

// Receive handles receiving a repair request at a site (v3).
func (h *RepairRequestHandler) Receive(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var req models.RepairRequest
	if err := db.Where("id = ?", id).First(&req).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "not found"})
		return
	}
	if req.Status != models.RepairReqStatusShipping && req.Status != models.RepairReqStatusTransitIn {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "not in shipping or transit_in status"})
		return
	}

	newStatus := models.RepairReqStatusRepairing
	if req.Status == models.RepairReqStatusShipping && req.MerchantType == models.MerchantTypeControlled {
		// Full-authority site receives → repairing directly
		// Controlled: shipping → transit_in (transit site processes later)
		newStatus = models.RepairReqStatusTransitIn
	}

	db.Model(&req).Updates(map[string]interface{}{
		"status":     newStatus,
		"updated_at": time.Now(),
	})

	createRepairRecord(db, id, middleware.GetUserID(ctx), "received", "已收货", nil)

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "repair request received", "data": gin.H{"status": newStatus}})
}

// TransitRelay handles relay (scan/unpack/photograph/repack/forward) for a transit order (v3).
func (h *RepairRequestHandler) TransitRelay(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var req models.RepairRequest
	if err := db.Where("id = ?", id).First(&req).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "not found"})
		return
	}

	var body struct {
		Direction          string   `json:"direction"` // in/out
		TransitOrderNumber string   `json:"transit_order_number"`
		UnpackPhotos       []string `json:"unpack_photos"`
		RepackCompany      string   `json:"repack_company"`
		RepackTrackingNum  string   `json:"repack_tracking_number"`
		Note               string   `json:"note"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request"})
		return
	}

	var transitOrder models.RepairTransitOrder
	if err := db.Where("repair_request_id = ? AND direction = ?", id, body.Direction).First(&transitOrder).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "transit order not found"})
		return
	}

	photosJSON, _ := json.Marshal(body.UnpackPhotos)
	updates := map[string]interface{}{
		"status":                 models.RepairTransitRelayed,
		"transit_order_number":   body.TransitOrderNumber,
		"unpack_photos":          string(photosJSON),
		"repack_company":         body.RepackCompany,
		"repack_tracking_number": body.RepackTrackingNum,
		"note":                   body.Note,
		"updated_at":             time.Now(),
	}
	db.Model(&transitOrder).Updates(updates)

	// Advance the repair request status based on direction
	var reqStatus string
	if body.Direction == models.RepairTransitDirIn {
		reqStatus = models.RepairReqStatusTransitIn // inbound relay done → transit_in
	} else {
		reqStatus = models.RepairReqStatusTransitOut // outbound relay done → transit_out
	}
	db.Model(&req).Updates(map[string]interface{}{
		"status":     reqStatus,
		"updated_at": time.Now(),
	})

	createRepairRecord(db, id, middleware.GetUserID(ctx), "transit_relayed", "中转转发完成", body.UnpackPhotos)

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "transit relay completed"})
}

// ConfirmReceipt handles customer confirming receipt of returned instrument (returned → closed).
func (h *RepairRequestHandler) ConfirmReceipt(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var req models.RepairRequest
	if err := db.Where("id = ?", id).First(&req).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "not found"})
		return
	}

	if req.Status != models.RepairReqStatusReturned {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "only returned requests can be confirmed"})
		return
	}

	now := time.Now()
	if err := db.Model(&req).Updates(map[string]interface{}{
		"status":     models.RepairReqStatusClosed,
		"closed_at":  now,
		"updated_at": now,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to confirm receipt"})
		return
	}

	createRepairRecord(db, id, middleware.GetUserID(ctx), "receipt_confirmed", "确认收货", nil)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "receipt confirmed"})
}

// CompleteRepairRequest transitions a repair request from repairing to return_pending (v3).
func (h *RepairRequestHandler) CompleteRepairRequest(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var req models.RepairRequest
	if err := db.Where("id = ?", id).First(&req).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "not found"})
		return
	}

	if req.Status != models.RepairReqStatusRepairing {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "only repairing requests can be completed"})
		return
	}

	if err := db.Model(&req).Updates(map[string]interface{}{
		"status":     models.RepairReqStatusReturnPend,
		"updated_at": time.Now(),
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to complete repair"})
		return
	}

	createRepairRecord(db, id, middleware.GetUserID(ctx), "completed", "维修完成", nil)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "repair completed"})
}

// ReturnShipping sets return tracking info, transitions repair request to returned/transit_out, and activates outbound transit order (v3).
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

	var newStatus string
	if req.MerchantType == models.MerchantTypeControlled {
		// Controlled path: create and activate outbound transit order
		transitOrder := models.RepairTransitOrder{
			ID:               uuid.New().String(),
			RepairRequestID:  id,
			TransitSiteID:    *req.TransitSiteID,
			ControlledSiteID: *req.ControlledSiteID,
			Direction:        models.RepairTransitDirOut,
			Status:           models.RepairTransitActive, // activated immediately
			CreatedAt:        time.Now(),
		}
		db.Create(&transitOrder)
		newStatus = models.RepairReqStatusTransitOut
	} else {
		newStatus = models.RepairReqStatusReturned
	}

	db.Model(&req).Updates(map[string]interface{}{
		"return_company":         body.ReturnCompany,
		"return_tracking_number": body.ReturnTrackingNumber,
		"status":                 newStatus,
		"updated_at":             time.Now(),
	})

	// Notify customer that instrument has been shipped back
	var customerUser models.User
	if err := database.GetDB().Where("iam_sub = ?", req.UserID).First(&customerUser).Error; err == nil {
		title := "乐器已发回"
		content := "您的报修乐器已发回，请注意查收。"
		services.Notify(db, req.TenantID, customerUser.ID, "returned", title, content, req.ID, "repair_request")
	}

	createRepairRecord(db, id, middleware.GetUserID(ctx), "return_shipped", "已发还（"+body.ReturnCompany+" "+body.ReturnTrackingNumber+"）", nil)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "return shipping updated"})
}

// PayRepairRequest processes payment for a repair request.
// v3: handles first payment (pending_payment → pending_ship) and requote supplement.
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

	if req.Status != models.RepairReqStatusPendingPay {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "message": "not in pending_payment status"})
		return
	}

	if req.AcceptedQuoteID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "no accepted quote"})
		return
	}

	// Load accepted quote
	var quote models.RepairQuote
	if err := db.Where("id = ?", req.AcceptedQuoteID).First(&quote).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "accepted quote not found"})
		return
	}

	var amount float64
	var newStatus string
	checkFeeSnapshot := float64(0)

	if quote.IsRenegotiation {
		// Requote supplement: pay the difference
		newTotal := quote.MaterialFee + quote.ServiceFee + quote.LogisticsFee
		if req.PaidAmount != nil {
			amount = newTotal - *req.PaidAmount
			if amount < 0 {
				amount = 0
			}
		} else {
			amount = newTotal
		}
		newStatus = models.RepairReqStatusRepairing
	} else {
		// First payment: material + service + logistics
		amount = quote.MaterialFee + quote.ServiceFee + quote.LogisticsFee

		// Add transit fees if controlled path
		if req.MerchantType == models.MerchantTypeControlled {
			var transitOrder models.RepairTransitOrder
			if err := db.Where("repair_request_id = ? AND direction = ?", id, models.RepairTransitDirIn).
				First(&transitOrder).Error; err == nil {
				if transitOrder.TransitServiceFee != nil {
					amount += *transitOrder.TransitServiceFee
				}
				if transitOrder.TransitLogisticsFee != nil {
					amount += *transitOrder.TransitLogisticsFee
				}
			}
		}

		// Snapshot the system check_fee (unscoped to bypass tenant auto-scoping)
		var checkFeeSetting models.SystemSetting
		database.GetDB().Where("tenant_id = ? AND setting_key = ?", systemTenantID, keyRepairCheckFee).First(&checkFeeSetting)
		if checkFeeSetting.SettingValue != "" {
			if v, err := strconv.ParseFloat(checkFeeSetting.SettingValue, 64); err == nil {
				checkFeeSnapshot = v
			}
		}

		newStatus = models.RepairReqStatusPendingShip
	}

	if amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "payment amount must be positive"})
		return
	}

	// Update user total_spending
	var localUser models.User
	if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}
	db.Model(&localUser).Update("total_spending", gorm.Expr("total_spending + ?", amount))

	// Update repair request
	updates := map[string]interface{}{
		"paid_amount": gorm.Expr("COALESCE(paid_amount, 0) + ?", amount),
		"status":      newStatus,
		"updated_at":  time.Now(),
	}
	if newStatus == models.RepairReqStatusPendingShip {
		updates["check_fee_snapshot"] = checkFeeSnapshot
	}
	db.Model(&req).Updates(updates)

	createRepairRecord(db, id, middleware.GetUserID(ctx), "paid", "支付完成", nil)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"amount_paid": amount, "status": newStatus}})
}

// Requote allows a technician to submit a renegotiation quote during repair (v3, once per request).
func (h *RepairRequestHandler) Requote(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)

	var req models.RepairRequest
	if err := db.Where("id = ?", id).First(&req).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "not found"})
		return
	}
	if req.Status != models.RepairReqStatusRepairing {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "not in repairing status"})
		return
	}

	// Check that a renegotiation hasn't already been submitted
	var existingRenegotiation int64
	db.Model(&models.RepairQuote{}).
		Where("repair_request_id = ? AND is_renegotiation = ?", id, true).
		Count(&existingRenegotiation)
	if existingRenegotiation > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "renegotiation already submitted (once only)"})
		return
	}

	var body struct {
		MaterialFee  float64 `json:"material_fee"`
		ServiceFee   float64 `json:"service_fee"`
		LogisticsFee float64 `json:"logistics_fee"`
		Duration     string  `json:"duration"`
		Comment      string  `json:"comment"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "invalid request"})
		return
	}

	// Scan for sensitive content
	if body.Comment != "" {
		if services.HandleSensitiveQuote(id, "", body.Comment) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "quote comment contains sensitive information"})
			return
		}
	}

	// Determine technician's site
	var siteID string
	var localUser models.User
	if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err == nil {
		var members []models.SiteMember
		db.Where("user_id = ? AND role = ?", localUser.ID, "repair_technician").Limit(1).Find(&members)
		if len(members) > 0 {
			siteID = members[0].SiteID
		}
	}

	quoteNo := "RQ" + uuid.New().String()[:8]
	quote := models.RepairQuote{
		ID:              uuid.New().String(),
		RepairRequestID: id,
		SiteID:          siteID,
		WorkerID:        userID,
		QuoteNo:         quoteNo,
		MaterialFee:     body.MaterialFee,
		ServiceFee:      body.ServiceFee,
		LogisticsFee:    body.LogisticsFee,
		Duration:        body.Duration,
		Comment:         body.Comment,
		IsRenegotiation: true,
		Status:          models.RepairQuotePending,
		CreatedAt:       time.Now(),
	}
	if err := db.Create(&quote).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create requote"})
		return
	}

	// Notify customer of requote
	var customerUser models.User
	if err := database.GetDB().Where("iam_sub = ?", req.UserID).First(&customerUser).Error; err == nil {
		title := "维修师傅重新报价"
		content := "维修师傅给出了新的报价，请查看并确认。"
		services.Notify(db, req.TenantID, customerUser.ID, "requote", title, content, req.ID, "repair_request")
	}

	createRepairRecord(db, id, middleware.GetUserID(ctx), "requoted", "师傅重新报价", nil)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": quote})
}

// RejectRequote rejects a renegotiation quote and triggers rollback settlement (v3).
func (h *RepairRequestHandler) RejectRequote(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var req models.RepairRequest
	if err := db.Where("id = ?", id).First(&req).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "not found"})
		return
	}
	if req.Status != models.RepairReqStatusRepairing {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "not in repairing status"})
		return
	}

	// Rollback settlement: refund = max(0, material+service - check_fee_snapshot)
	// Logistics and transit fees are retained (lock-in at shipping time)
	if req.AcceptedQuoteID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "no accepted quote for rollback calculation"})
		return
	}
	var quote models.RepairQuote
	if err := db.Where("id = ?", req.AcceptedQuoteID).First(&quote).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "accepted quote not found"})
		return
	}

	checkFee := float64(0)
	if req.CheckFeeSnapshot != nil {
		checkFee = *req.CheckFeeSnapshot
	}
	refund := (quote.MaterialFee + quote.ServiceFee) - checkFee
	if refund < 0 {
		refund = 0
	}

	// Update repair request: transition to return_pending, adjust paid_amount
	// refund reduces the effective paid amount
	updates := map[string]interface{}{
		"status":     models.RepairReqStatusReturnPend,
		"updated_at": time.Now(),
	}

	// If there's a paid_amount, reduce it by the refund
	if req.PaidAmount != nil {
		newPaid := *req.PaidAmount - refund
		if newPaid < 0 {
			newPaid = 0
		}
		updates["paid_amount"] = newPaid
	}

	db.Model(&req).Updates(updates)

	createRepairRecord(db, id, middleware.GetUserID(ctx), "requote_rejected", "拒绝重新报价", nil)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "rollback settlement complete", "data": gin.H{
		"refund":        refund,
		"retained_fees": checkFee + quote.LogisticsFee,
	}})
}

// resolveRepairMeta enriches a RepairRequest with resolved names for instrument, site,
// merchant, and reporter. All lookups are best-effort (errors silently return empty strings).
// Shared by the Get and List handlers.
func resolveRepairMeta(db *gorm.DB, req models.RepairRequest) (instrumentSN, instrumentType, brand, model, siteName, siteAddress, sitePhone, merchantName, reporterName string) {
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
			siteAddress = site.Address
			sitePhone = site.Phone
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

// createRepairRecord inserts an automated repair_request_record entry.
func createRepairRecord(db *gorm.DB, reqID, workerID, recordType, comment string, photos []string) {
	photosJSON := []byte("[]")
	if len(photos) > 0 {
		pj, err := json.Marshal(photos)
		if err == nil {
			photosJSON = pj
		}
	}
	rec := models.RepairRequestRecord{
		ID:              uuid.New().String(),
		RepairRequestID: reqID,
		WorkerID:        workerID,
		Comment:         comment,
		Photos:          string(photosJSON),
		RecordType:      recordType,
		CreatedAt:       time.Now(),
	}
	db.Create(&rec)
}
