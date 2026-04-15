package handlers

import (
	"net/http"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/internal/service"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

type PreviewOrderRequest struct {
	InstrumentID string `json:"instrument_id" binding:"required"`
	Level        string `json:"level" binding:"required"`
	LeaseTerm    int    `json:"lease_term" binding:"required"`
	DepositMode  string `json:"deposit_mode"`
}

func PreviewOrder(c *gin.Context) {
	var req PreviewOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	if req.DepositMode == "" {
		req.DepositMode = "standard"
	}

	creditScore := 600
	if userID := c.GetString("user_id"); userID != "" {
		db := database.GetDB().WithContext(c.Request.Context())
		var user models.User
		if err := db.First(&user, "id = ?", userID).Error; err == nil {
			creditScore = user.CreditScore
		}
	}

	pricingReq := &service.PricingRequest{
		InstrumentID: req.InstrumentID,
		Level:        req.Level,
		LeaseTerm:    req.LeaseTerm,
		DepositMode:  req.DepositMode,
		CreditScore:  creditScore,
	}

	resp, err := pricingService.CalculatePrice(c.Request.Context(), pricingReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "pricing calculation failed: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": resp,
	})
}

type CreateOrderRequest struct {
	InstrumentID    string `json:"instrument_id" binding:"required"`
	Level           string `json:"level" binding:"required"`
	LeaseTerm       int    `json:"lease_term" binding:"required"`
	DepositMode     string `json:"deposit_mode"`
	DeliveryType    string `json:"delivery_type"`
	AgreementSigned bool   `json:"agreement_signed"`
}

func CreateOrder(c *gin.Context) {
	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	// Generate order ID
	orderID := "order_" + req.InstrumentID + "_" + time.Now().Format("20060102150405")

	// Get tenant ID from context
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		tenantID = "default_tenant"
	}

	// Calculate pricing
	creditScore := 600
	if userID := c.GetString("user_id"); userID != "" {
		db := database.GetDB().WithContext(c.Request.Context())
		var user models.User
		if err := db.First(&user, "id = ?", userID).Error; err == nil {
			creditScore = user.CreditScore
		}
	}

	pricingReq := &service.PricingRequest{
		InstrumentID: req.InstrumentID,
		Level:        req.Level,
		LeaseTerm:    req.LeaseTerm,
		DepositMode:  req.DepositMode,
		CreditScore:  creditScore,
	}

	resp, err := pricingService.CalculatePrice(c.Request.Context(), pricingReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "pricing calculation failed: " + err.Error(),
		})
		return
	}

	// Check inventory availability
	db := database.GetDB().WithContext(c.Request.Context())
	var instrument models.Instrument
	if err := db.First(&instrument, "id = ?", req.InstrumentID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "instrument not found",
		})
		return
	}

	// Check if instrument is available
	if instrument.StockStatus != "available" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "instrument not available",
		})
		return
	}

	// Create order record
	order := models.Order{
		ID:           orderID,
		TenantID:     tenantID,
		UserID:       c.GetString("user_id"),
		InstrumentID: req.InstrumentID,
		Level:        req.Level,
		LeaseTerm:    req.LeaseTerm,
		DepositMode:  req.DepositMode,
		MonthlyRent:  resp.FirstMonthRent,
		Deposit:      resp.Deposit,
		Status:       "pending", // pending, paid, in_lease, completed, cancelled
		StartDate:    "",
		EndDate:      "",
	}

	if err := db.Create(&order).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to create order: " + err.Error(),
		})
		return
	}

	// Update inventory status to unavailable
	if err := db.Model(&instrument).Update("stock_status", "unavailable").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to update inventory: " + err.Error(),
		})
		return
	}

	// Generate payment URL (mock)
	paymentURL := "https://pay.example.com/order/" + orderID

	c.JSON(http.StatusCreated, gin.H{
		"code": 20000,
		"data": gin.H{
			"order_id":             orderID,
			"payment_url":          paymentURL,
			"first_payment_amount": resp.TotalAmount,
			"created_at":           time.Now().Format(time.RFC3339),
		},
	})
}
