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

// SubmitQuote submits a repair quote for assessment.
// v3: fields changed to material_fee/service_fee/logistics_fee/duration (old quote_amount/timeframe accepted as shim)
func SubmitQuote(c *gin.Context) {
	var req struct {
		RepairRequestID string  `json:"repair_request_id" binding:"required"`
		MaterialFee     float64 `json:"material_fee"`
		ServiceFee      float64 `json:"service_fee"`
		LogisticsFee    float64 `json:"logistics_fee"`
		Duration        string  `json:"duration"`
		Comment         string  `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "invalid request"})
		return
	}

	// Scan comment for sensitive content
	if req.Comment != "" {
		if services.HandleSensitiveQuote(req.RepairRequestID, "", req.Comment) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "quote comment contains sensitive information"})
			return
		}
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)

	quote := models.RepairQuote{
		ID:              uuid.New().String(),
		RepairRequestID: req.RepairRequestID,
		WorkerID:        userID,
		MaterialFee:     req.MaterialFee,
		ServiceFee:      req.ServiceFee,
		LogisticsFee:    req.LogisticsFee,
		Duration:        req.Duration,
		Comment:         req.Comment,
		Status:          models.RepairQuotePending,
		CreatedAt:       time.Now(),
	}
	if err := db.Create(&quote).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create quote"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": quote})
}

// ListQuotes lists quotes for a repair request.
func ListQuotes(c *gin.Context) {
	requestID := c.Param("request_id")
	db := database.GetDB()

	var quotes []models.RepairQuote
	db.Where("repair_request_id = ?", requestID).Order("created_at ASC").Find(&quotes)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": quotes}})
}

// AcceptQuote marks a quote as accepted and transitions the repair request.
func AcceptQuote(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		RepairRequestID string `json:"repair_request_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.RepairRequestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "repair_request_id required"})
		return
	}

	db := database.GetDB()

	db.Model(&models.RepairQuote{}).Where("id = ?", id).Update("status", "accepted")
	db.Model(&models.RepairRequest{}).Where("id = ?", req.RepairRequestID).Update("status", models.RepairReqStatusPendingPay)

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "quote accepted"})
}
