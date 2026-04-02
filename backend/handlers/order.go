package handlers

import (
	"net/http"
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
			"message": "invalid parameters",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"order_id":             "order_" + req.InstrumentID,
			"payment_url":          "https://pay.example.com/mock",
			"first_payment_amount": 760,
			"created_at":           "2026-03-21T10:30:00Z",
		},
	})
}
