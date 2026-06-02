package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm/clause"
	"log"
)

type UserRentalHandler struct{}

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
		DeliveryAddress map[string]interface{} `json:"delivery_address"`
		Notes           string                 `json:"notes"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	userID := middleware.GetUserID(ctx)

	db := database.GetDB().WithContext(ctx)

	// Verify instrument exists and is available
	var instrument models.Instrument
	if err := db.Where("id = ? AND tenant_id = ? AND stock_status = ?", req.InstrumentID, tenantID, "available").First(&instrument).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "instrument not available"})
		return
	}

	// Begin transaction with row lock to prevent oversell
	tx := db.Begin()
	var lockedInstrument models.Instrument
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND tenant_id = ? AND stock_status = ?", req.InstrumentID, tenantID, "available").
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

	// Parse pricing for calculation
	var pricing []map[string]interface{}
	dailyRent, deposit, shippingFee := 0.0, 0.0, 0.0
	if instrument.Pricing != "" {
		if err := json.Unmarshal([]byte(instrument.Pricing), &pricing); err == nil && len(pricing) > 0 {
			if val, ok := pricing[0]["daily_rent"].(float64); ok {
				dailyRent = val
			}
			if val, ok := pricing[0]["deposit"].(float64); ok {
				deposit = val
			}
			if val, ok := pricing[0]["shipping_fee"].(float64); ok {
				shippingFee = val
			}
		}
	}

	// monthly rent = daily_rent * 25
	monthlyRent := dailyRent * 25
	totalAmount := monthlyRent + deposit + shippingFee

	// Create order
	startDateStr := req.StartDate
	endDateStr := req.EndDate
	order := models.Order{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		UserID:       userID,
		InstrumentID: req.InstrumentID,
		Level:        instrument.Level,
		LeaseTerm:    months,
		MonthlyRent:  monthlyRent,
		Deposit:      deposit,
		ShippingFee:  shippingFee,
		Status:       "reserved",
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
	leaseSession := models.LeaseSession{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		OrderID:      order.ID,
		UserID:       userID,
		InstrumentID: req.InstrumentID,
		StartDate:    startDate,
		EndDate:      endDate,
		Status:       "active",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := tx.Create(&leaseSession).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create lease session: " + err.Error()})
		return
	}

	transitInfo := GetMerchantTransitInfo(ctx, tenantID)
	if transitInfo != nil && transitInfo.MerchantType == models.MerchantTypeControlled && transitInfo.Address != "" {
		deliveryJSON, _ := json.Marshal(map[string]string{
			"address": transitInfo.Address,
			"phone":   transitInfo.Phone,
			"contact": transitInfo.ContactName,
		})
		deliveryStr := string(deliveryJSON)
		if err := tx.Model(&models.LeaseSession{}).Where("id = ?", leaseSession.ID).Update("delivery_address", deliveryStr).Error; err != nil {
			log.Printf("[CreateOrder] Warning: failed to set delivery_address: %v", err)
		}
	}

	// Update instrument stock_status to reserved
	if err := tx.Model(&models.Instrument{}).Where("id = ?", req.InstrumentID).Update("stock_status", models.StockStatusReserved).Error; err != nil {
		tx.Rollback()
		log.Printf("[ERROR] Failed to update instrument stock_status: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to reserve instrument"})
		return
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to commit transaction"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"order_id":    order.ID,
			"amount":      totalAmount,
			"deposit":     deposit,
			"lease_id":    leaseSession.ID,
			"payment_url": "https://pay.example.com/" + order.ID,
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
	if err := db.Where("tenant_id = ? AND user_id = ?", tenantID, userID).Find(&leaseSessions).Error; err != nil {
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
	if err := db.Where("id = ? AND tenant_id = ? AND user_id = ?", leaseID, tenantID, userID).First(&leaseSession).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "rental not found"})
		return
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

	transitInfo := GetMerchantTransitInfo(ctx, tenantID)
	if transitInfo != nil && transitInfo.MerchantType == models.MerchantTypeControlled {
		respData["return_address"] = transitInfo.Address
		respData["return_phone"] = transitInfo.Phone
		respData["transit_info"] = map[string]string{
			"address": transitInfo.Address,
			"phone":   transitInfo.Phone,
			"contact": transitInfo.ContactName,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data":    respData,
	})
}

// GET /api/user/contracts/:id - Get electronic contract
func (h *UserRentalHandler) GetContract(c *gin.Context) {
	contractID := c.Param("id")
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	userID := middleware.GetUserID(ctx)

	db := database.GetDB().WithContext(ctx)

	var contract models.ElectronicContract
	if err := db.Where("id = ? AND tenant_id = ? AND user_id = ?", contractID, tenantID, userID).First(&contract).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "contract not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": contract,
	})
}
