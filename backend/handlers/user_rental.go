package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UserRentalHandler struct{}

func strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func NewUserRentalHandler() *UserRentalHandler {
	return &UserRentalHandler{}
}

// GET /api/user/instruments - Get instrument list (user-facing)
func (h *UserRentalHandler) ListInstruments(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	categoryID := c.Query("category_id")
	siteID := c.Query("site_id")
	level := c.Query("level")
	status := c.Query("status")
	sort := c.DefaultQuery("sort", "price")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	db := database.GetDB().WithContext(ctx)

	query := db.Model(&models.Instrument{}).Where("tenant_id = ? AND stock_status = ?", tenantID, "available")

	if categoryID != "" {
		query = query.Where("category_id = ?", categoryID)
	}
	if siteID != "" {
		query = query.Where("site_id = ?", siteID)
	}
	if level != "" {
		query = query.Where("level_id = ?", level)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	// Sorting
	switch sort {
	case "price":
		query = query.Order("pricing->0->>'daily_rent' ASC")
	case "distance":
		// TODO: Implement distance-based sorting if location provided
	case "rating":
		// TODO: Implement rating-based sorting
	}

	var total int64
	query.Count(&total)

	offset := (page - 1) * pageSize
	var instruments []models.Instrument
	query.Offset(offset).Limit(pageSize).Find(&instruments)

	// Transform results to include pricing calculation
	type InstrumentResponse struct {
		models.Instrument
		DailyRent   float64 `json:"daily_rent"`
		WeeklyRent  float64 `json:"weekly_rent"`
		MonthlyRent float64 `json:"monthly_rent"`
		Deposit     float64 `json:"deposit"`
	}

	var response []InstrumentResponse
	for _, inst := range instruments {
		// Parse pricing for calculations
		var pricing []map[string]interface{}
		if inst.Pricing != "" {
			db.Raw("SELECT * FROM jsonb_to_recordset(?::jsonb) AS x(daily_rent numeric, deposit numeric)", inst.Pricing).Scan(&pricing)
		}

		dailyRent := 0.0
		deposit := 0.0
		if len(pricing) > 0 {
			if dailyRentVal, ok := pricing[0]["daily_rent"].(float64); ok {
				dailyRent = dailyRentVal
			}
			if depositVal, ok := pricing[0]["deposit"].(float64); ok {
				deposit = depositVal
			}
		}

		resp := InstrumentResponse{
			Instrument:  inst,
			DailyRent:   dailyRent,
			WeeklyRent:  dailyRent * 7 * 0.9,
			MonthlyRent: dailyRent * 30 * 0.8,
			Deposit:     deposit,
		}
		response = append(response, resp)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":     response,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		},
	})
}

// GET /api/user/instruments/:id - Get instrument details (user-facing)
func (h *UserRentalHandler) GetInstrument(c *gin.Context) {
	instrumentID := c.Param("id")
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	db := database.GetDB().WithContext(ctx)

	var instrument models.Instrument
	if err := db.Where("id = ? AND tenant_id = ?", instrumentID, tenantID).First(&instrument).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "instrument not found"})
		return
	}

	// Parse pricing
	type InstrumentDetail struct {
		models.Instrument
		DailyRent   float64 `json:"daily_rent"`
		WeeklyRent  float64 `json:"weekly_rent"`
		MonthlyRent float64 `json:"monthly_rent"`
		Deposit     float64 `json:"deposit"`
	}

	var pricing []map[string]interface{}
	dailyRent := 0.0
	deposit := 0.0
	if instrument.Pricing != "" {
		db.Raw("SELECT * FROM jsonb_to_recordset(?::jsonb) AS x(daily_rent numeric, weekly_rent numeric, monthly_rent numeric, deposit numeric)", instrument.Pricing).Scan(&pricing)
		if len(pricing) > 0 {
			if val, ok := pricing[0]["daily_rent"].(float64); ok {
				dailyRent = val
			}
			if val, ok := pricing[0]["deposit"].(float64); ok {
				deposit = val
			}
		}
	}

	response := InstrumentDetail{
		Instrument:  instrument,
		DailyRent:   dailyRent,
		WeeklyRent:  dailyRent * 7 * 0.9,
		MonthlyRent: dailyRent * 30 * 0.8,
		Deposit:     deposit,
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": response,
	})
}

// POST /api/user/orders - Create rental order
func (h *UserRentalHandler) CreateOrder(c *gin.Context) {
	var req struct {
		InstrumentID    string                 `json:"instrument_id" binding:"required"`
		StartDate       string                 `json:"start_date" binding:"required"`
		EndDate         string                 `json:"end_date" binding:"required"`
		DeliveryAddress interface{}            `json:"delivery_address"`
		Notes           string                 `json:"notes"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	orgID := middleware.GetOrgID(ctx)

	db := database.GetDB().WithContext(ctx)

	userID, err := middleware.EnsureLocalUser(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "user sync failed"})
		return
	}

	effectiveTenantID := tenantID
	effectiveOrgID := orgID

	db = database.GetDB().WithContext(ctx)

	// Look up instrument (no tenant_id filter — guest doesn't know it)
	var instrument models.Instrument
	if err := db.Where("id = ? AND stock_status = ?", req.InstrumentID, "available").First(&instrument).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "instrument not available"})
		return
	}

	// Guest (tid empty): derive tenant/org from instrument
	if effectiveTenantID == "" {
		effectiveTenantID = instrument.TenantID
		if instrument.OrgID != nil {
			effectiveOrgID = *instrument.OrgID
		} else if instrument.SiteID != nil {
			effectiveOrgID = instrument.SiteID.String()
		} else if instrument.CurrentSiteID != nil {
			effectiveOrgID = instrument.CurrentSiteID.String()
		}
	}

	// Ensure user exists locally (guest may not have a local record yet)
	var existingUser models.User
	if err := db.Where("id = ?", userID).First(&existingUser).Error; err != nil {
		nilUUID := "00000000-0000-0000-0000-000000000000"
		shadowUser := models.User{
			ID:        userID,
			IAMSub:    userID,
			TenantID:  nilUUID,
			OrgID:     nilUUID,
			IsShadow:  true,
			Status:    "active",
			Name:      "Guest",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := db.Create(&shadowUser).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create user record: " + err.Error()})
			return
		}
		// Fetch real user info from IAM to replace shadow defaults
		iamClient := services.NewIAMClient()
		if iamUser, err := iamClient.GetUser(userID); err == nil && iamUser != nil {
			db.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
				"name":  iamUser.Name,
				"email": iamUser.Email,
				"phone": iamUser.Phone,
			})
		}
	}

	// Begin transaction with row lock to prevent oversell
	tx := db.Begin()
	var lockedInstrument models.Instrument
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND tenant_id = ? AND stock_status = ?", req.InstrumentID, effectiveTenantID, "available").
		First(&lockedInstrument).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "instrument already reserved"})
		return
	}

	// Parse start and end dates
	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid start_date format"})
		return
	}

	endDate, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid end_date format"})
		return
	}

	if endDate.Before(startDate) {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "end_date must be after start_date"})
		return
	}

	// Calculate rental amount
	days := int(endDate.Sub(startDate).Hours() / 24)
	months := days / 30

	// Compute pricing via CalculatePricing (merchant defaults as fallback)
	var merchantConfigJSON string
	var config models.MerchantPricingConfig
	if err := db.Where("tenant_id = ?", effectiveTenantID).First(&config).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			var defaultTemplate models.PricingTemplate
			if err2 := db.Where("is_system_default = ? AND is_active = ?", true, true).First(&defaultTemplate).Error; err2 != nil {
				merchantConfigJSON = "{}"
			} else {
				merchantConfigJSON = defaultTemplate.ConfigSchema
			}
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query pricing config"})
			return
		}
	} else {
		merchantConfigJSON = config.Config
	}

	baseRate := 0.0
	if instrument.BaseDailyRate != nil {
		baseRate = *instrument.BaseDailyRate
	}
	pricingResult := services.CalculatePricing(baseRate, merchantConfigJSON, instrument.PricingOverrides)

	dailyRent := 0.0
	if len(pricingResult.Tiers) > 0 {
		dailyRent = pricingResult.Tiers[0].DailyRate
	}
	deposit := pricingResult.Deposit
	shippingFee := pricingResult.ShippingFee

	// monthly rent = daily_rent * 25
	monthlyRent := dailyRent * 25
	totalAmount := monthlyRent + deposit + shippingFee

	// Create order
	startDateStr := req.StartDate
	endDateStr := req.EndDate
	order := models.Order{
		ID:           uuid.New().String(),
		TenantID:     effectiveTenantID,
		OrgID:        effectiveOrgID,
		UserID:       userID,
		InstrumentID: req.InstrumentID,
		Level:        instrument.Level,
		LeaseTerm:    months,
		MonthlyRent:  monthlyRent,
		Deposit:      deposit,
		ShippingFee:  shippingFee,
		Status:       models.OrderStatusReserved,
		StartDate:    &startDateStr,
		EndDate:      &endDateStr,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create order: " + err.Error()})
		return
	}

	// Create lease session
	deliveryAddressJSON := ""
	if req.DeliveryAddress != nil {
		if b, err := json.Marshal(req.DeliveryAddress); err == nil {
			deliveryAddressJSON = string(b)
		}
	}
	leaseSession := models.LeaseSession{
		ID:               uuid.New().String(),
		TenantID:         effectiveTenantID,
		OrgID:            stringPtr(effectiveOrgID),
		OrderID:          order.ID,
		UserID:           userID,
		InstrumentID:     req.InstrumentID,
		StartDate:        startDate,
		EndDate:          endDate,
		Status:           "active",
		DeliveryAddress:  &deliveryAddressJSON,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := tx.Create(&leaseSession).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create lease session"})
		return
	}

	// Create electronic contract
	contract := models.ElectronicContract{
		ID:             uuid.New().String(),
		TenantID:       effectiveTenantID,
		OrgID:          stringPtr(effectiveOrgID),
		OrderID:        order.ID,
		UserID:         userID,
		InstrumentID:   req.InstrumentID,
		ContractNumber: fmt.Sprintf("CT-%s", order.ID[:8]),
		Status:         "active",
		GeneratedAt:    time.Now(),
		ContractURL:    "",
	}
	if err := tx.Create(&contract).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create contract"})
		return
	}

	// Update instrument stock_status
	if err := tx.Model(&models.Instrument{}).Where("id = ?", req.InstrumentID).Update("stock_status", models.StockStatusRented).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to reserve instrument"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to commit transaction"})
		return
	}

	// Build response
	respData := gin.H{
		"order_id":    order.ID,
		"amount":      totalAmount,
		"deposit":     deposit,
		"lease_id":    leaseSession.ID,
		"contract_id": contract.ID,
		"payment_url": "https://pay.example.com/" + order.ID,
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":    20000,
		"message": "success",
		"data":    respData,
	})
}

// POST /api/user/orders/batch - Batch create rental orders
func (h *UserRentalHandler) BatchCreateOrder(c *gin.Context) {
	var req struct {
		Items []struct {
			InstrumentID string `json:"instrument_id" binding:"required"`
			StartDate    string `json:"start_date" binding:"required"`
			EndDate      string `json:"end_date" binding:"required"`
		} `json:"items" binding:"required,min=1"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	orgID := middleware.GetOrgID(ctx)

	db := database.GetDB().WithContext(ctx)

	userID, err := middleware.EnsureLocalUser(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "user sync failed"})
		return
	}

	effectiveTenantID := tenantID
	effectiveOrgID := orgID

	db = database.GetDB().WithContext(ctx)

	// Verify all instruments are available (no tenant_id filter for guests)
	for _, item := range req.Items {
		var inst models.Instrument
		if err := db.Where("id = ? AND stock_status = ?", item.InstrumentID, "available").First(&inst).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "instrument " + item.InstrumentID + " not available"})
			return
		}
		// Derive tenant from first instrument if guest
		if effectiveTenantID == "" {
			effectiveTenantID = inst.TenantID
			if inst.OrgID != nil {
				effectiveOrgID = *inst.OrgID
			} else if inst.SiteID != nil {
				effectiveOrgID = inst.SiteID.String()
			} else if inst.CurrentSiteID != nil {
				effectiveOrgID = inst.CurrentSiteID.String()
			}
		}
	}

	// Ensure user exists locally (guest may not have a local record yet)
	var existingUser models.User
	if err := db.Where("id = ?", userID).First(&existingUser).Error; err != nil {
		nilUUID := "00000000-0000-0000-0000-000000000000"
		shadowUser := models.User{
			ID:        userID,
			IAMSub:    userID,
			TenantID:  nilUUID,
			OrgID:     nilUUID,
			IsShadow:  true,
			Status:    "active",
			Name:      "Guest",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := db.Create(&shadowUser).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create user record: " + err.Error()})
			return
		}
		// Fetch real user info from IAM to replace shadow defaults
		iamClient := services.NewIAMClient()
		if iamUser, err := iamClient.GetUser(userID); err == nil && iamUser != nil {
			db.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
				"name":  iamUser.Name,
				"email": iamUser.Email,
				"phone": iamUser.Phone,
			})
		}
	}

	// Resolve merchant pricing config once (used for all items)
	var merchantConfigJSON string
	var config models.MerchantPricingConfig
	if err := db.Where("tenant_id = ?", effectiveTenantID).First(&config).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			var defaultTemplate models.PricingTemplate
			if err2 := db.Where("is_system_default = ? AND is_active = ?", true, true).First(&defaultTemplate).Error; err2 != nil {
				merchantConfigJSON = "{}"
			} else {
				merchantConfigJSON = defaultTemplate.ConfigSchema
			}
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query pricing config"})
			return
		}
	} else {
		merchantConfigJSON = config.Config
	}

	// Process all orders in a single transaction
	tx := db.Begin()

	type orderResult struct {
		OrderID string  `json:"order_id"`
		Amount  float64 `json:"amount"`
		Status  string  `json:"status"`
	}

	var results []orderResult
	totalAmount := 0.0

	for _, item := range req.Items {
		startDate, err := time.Parse("2006-01-02", item.StartDate)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid start_date format for " + item.InstrumentID})
			return
		}

		endDate, err := time.Parse("2006-01-02", item.EndDate)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid end_date format for " + item.InstrumentID})
			return
		}

		if endDate.Before(startDate) {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "end_date before start_date for " + item.InstrumentID})
			return
		}

		// Lock and verify instrument
		var lockedInstrument models.Instrument
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND tenant_id = ? AND stock_status = ?", item.InstrumentID, effectiveTenantID, "available").
			First(&lockedInstrument).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "instrument " + item.InstrumentID + " already reserved"})
			return
		}

		// Calculate pricing
		days := int(endDate.Sub(startDate).Hours() / 24)
		months := days / 30

		baseRate := 0.0
		if lockedInstrument.BaseDailyRate != nil {
			baseRate = *lockedInstrument.BaseDailyRate
		}
		pricingResult := services.CalculatePricing(baseRate, merchantConfigJSON, lockedInstrument.PricingOverrides)
		dailyRent := 0.0
		if len(pricingResult.Tiers) > 0 {
			dailyRent = pricingResult.Tiers[0].DailyRate
		}
		deposit := pricingResult.Deposit
		shippingFee := pricingResult.ShippingFee

		monthlyRent := dailyRent * 25
		orderAmount := monthlyRent + deposit + shippingFee

		startDateStr := item.StartDate
		endDateStr := item.EndDate

		order := models.Order{
			ID:           uuid.New().String(),
			TenantID:     effectiveTenantID,
			OrgID:        effectiveOrgID,
			UserID:       userID,
			InstrumentID: item.InstrumentID,
			Level:        lockedInstrument.Level,
			LeaseTerm:    months,
			MonthlyRent:  monthlyRent,
			Deposit:      deposit,
			ShippingFee:  shippingFee,
			Status:       models.OrderStatusPaid,
			StartDate:    &startDateStr,
			EndDate:      &endDateStr,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}

		if err := tx.Create(&order).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create order for " + item.InstrumentID})
			return
		}

		// Create lease session
		leaseSession := models.LeaseSession{
			ID:           uuid.New().String(),
			TenantID:     effectiveTenantID,
			OrgID:        stringPtr(effectiveOrgID),
			OrderID:      order.ID,
			UserID:       userID,
			InstrumentID: item.InstrumentID,
			StartDate:    startDate,
			EndDate:      endDate,
			Status:       "active",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		if err := tx.Create(&leaseSession).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create lease session for " + item.InstrumentID})
			return
		}

		// Create electronic contract
		contract := models.ElectronicContract{
			TenantID:       effectiveTenantID,
			OrgID:          stringPtr(effectiveOrgID),
			OrderID:        order.ID,
			UserID:         userID,
			InstrumentID:   item.InstrumentID,
			ContractNumber: fmt.Sprintf("CT-%s", order.ID[:8]),
			Status:         "active",
			GeneratedAt:    time.Now(),
			ContractURL:    "",
		}
		if err := tx.Create(&contract).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create contract for " + item.InstrumentID})
			return
		}

		// Update instrument stock_status
		if err := tx.Model(&models.Instrument{}).Where("id = ?", item.InstrumentID).Update("stock_status", models.StockStatusRented).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to reserve instrument " + item.InstrumentID})
			return
		}

		results = append(results, orderResult{
			OrderID: order.ID,
			Amount:  orderAmount,
			Status:  models.OrderStatusPaid,
		})
		totalAmount += orderAmount
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to commit transaction"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"orders":       results,
			"total_amount": totalAmount,
		},
	})
}

// GET /api/user/rentals - Get my rental list
func (h *UserRentalHandler) ListRentals(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	userID := middleware.GetUserID(ctx)

	db := database.GetDB().WithContext(ctx)

	var leaseSessions []models.LeaseSession
	query := db.Where("user_id = ?", userID)
	if tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}
	if err := query.Find(&leaseSessions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query rentals: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list": leaseSessions,
		},
	})
}

// POST /api/user/rentals/:id/return - Initiate return
func (h *UserRentalHandler) ReturnRental(c *gin.Context) {
	leaseID := c.Param("id")
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	userID := middleware.GetUserID(ctx)

	var req struct {
		ReturnMethod   string `json:"return_method"`
		ReturnTracking string `json:"return_tracking"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	db := database.GetDB().WithContext(ctx)

	// Get lease session
	var leaseSession models.LeaseSession
	q := db.Where("id = ? AND user_id = ?", leaseID, userID)
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if err := q.First(&leaseSession).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "rental not found"})
		return
	}

	// Use the record's tenant for subsequent operations
	effectiveTenantID := tenantID
	if effectiveTenantID == "" {
		effectiveTenantID = leaseSession.TenantID
	}

	// Update lease session
	leaseSession.Status = "return_requested"
	leaseSession.ReturnMethod = req.ReturnMethod
	leaseSession.ReturnTracking = req.ReturnTracking
	leaseSession.UpdatedAt = time.Now()

	if err := db.Save(&leaseSession).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update rental: " + err.Error()})
		return
	}

	respData := map[string]interface{}{
		"id":              leaseSession.ID,
		"tenant_id":       leaseSession.TenantID,
		"order_id":        leaseSession.OrderID,
		"user_id":         leaseSession.UserID,
		"instrument_id":   leaseSession.InstrumentID,
		"start_date":      leaseSession.StartDate,
		"end_date":        leaseSession.EndDate,
		"status":          leaseSession.Status,
		"return_method":   leaseSession.ReturnMethod,
		"return_tracking": leaseSession.ReturnTracking,
	}

	transitInfo := GetMerchantTransitInfo(ctx, effectiveTenantID)
	if transitInfo != nil && transitInfo.MerchantType == models.MerchantTypeControlled {
		respData["return_address"] = transitInfo.Address
		respData["return_phone"] = transitInfo.Phone
		respData["transit_info"] = map[string]string{
			"address": transitInfo.Address,
			"phone":   transitInfo.Phone,
			"contact": transitInfo.ContactName,
		}
		// Auto-create return forwarding session for controlled merchants
		db2 := database.GetDB().WithContext(ctx)
		createForwardingSession(c, db2, effectiveTenantID, strVal(leaseSession.OrgID), leaseSession.ID, leaseSession.OrderID, leaseSession.InstrumentID, models.ForwardingDirectionReturn)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data":    respData,
	})
}

// GET /api/user/contracts/:id - Get electronic contract
// GET /api/user/contracts - List my contracts
func (h *UserRentalHandler) ListContracts(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)

	db := database.GetDB().WithContext(ctx)

	var contracts []models.ElectronicContract
	if err := db.Where("user_id = ?", userID).Order("generated_at DESC").Find(&contracts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query contracts: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list": contracts,
		},
	})
}

// GET /api/user/contracts/:id - Get contract with joined data
func (h *UserRentalHandler) GetContract(c *gin.Context) {
	contractID := c.Param("id")
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	userID := middleware.GetUserID(ctx)

	db := database.GetDB().WithContext(ctx)

	var contract models.ElectronicContract
	q := db.Where("id = ? AND user_id = ?", contractID, userID)
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if err := q.First(&contract).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "contract not found"})
		return
	}

	// Join with order and instrument for display data
	type ContractDetail struct {
		models.ElectronicContract
		InstrumentName string  `json:"instrument_name"`
		OrderStatus    string  `json:"order_status"`
		StartDate      string  `json:"start_date"`
		EndDate        string  `json:"end_date"`
		MonthlyRent    float64 `json:"monthly_rent"`
		Deposit        float64 `json:"deposit"`
	}

	detail := ContractDetail{
		ElectronicContract: contract,
	}

	var order struct {
		Status      string  `json:"status"`
		StartDate   string  `json:"start_date"`
		EndDate     string  `json:"end_date"`
		MonthlyRent float64 `json:"monthly_rent"`
		Deposit     float64 `json:"deposit"`
	}
	if err := db.Table("orders").Where("id = ?", contract.OrderID).First(&order).Error; err == nil {
		detail.OrderStatus = order.Status
		detail.StartDate = order.StartDate
		detail.EndDate = order.EndDate
		detail.MonthlyRent = order.MonthlyRent
		detail.Deposit = order.Deposit
	}

	var instrument struct {
		SN string `json:"sn"`
	}
	if err := db.Table("instruments").Where("id = ?", contract.InstrumentID).First(&instrument).Error; err == nil {
		detail.InstrumentName = instrument.SN
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": detail,
	})
}
