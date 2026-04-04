package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AssessmentHandler handles damage assessment related operations
type AssessmentHandler struct {
	db *gorm.DB
}

// NewAssessmentHandler creates a new assessment handler
func NewAssessmentHandler(db *gorm.DB) *AssessmentHandler {
	return &AssessmentHandler{db: db}
}

// GetAssessmentData returns outbound and return photos for comparison
func (h *AssessmentHandler) GetAssessmentData(c *gin.Context) {
	orderID := c.Param("order_id")

	// Fetch order with instrument and photos
	var order struct {
		ID           string    `json:"id"`
		UserID       string    `json:"user_id"`
		InstrumentID string    `json:"instrument_id"`
		StartDate    string    `json:"start_date"`
		EndDate      string    `json:"end_date"`
		Status       string    `json:"status"`
		CreatedAt    time.Time `json:"created_at"`
	}

	if err := h.db.Table("orders").Where("id = ?", orderID).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "Order not found",
		})
		return
	}

	// Fetch instrument details
	var instrument struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Brand       string `json:"brand"`
		Model       string `json:"model"`
		SN          string `json:"sn"`
		Images      string `json:"images"`
		StockStatus string `json:"stock_status"`
	}

	if err := h.db.Table("instruments").Where("id = ?", order.InstrumentID).First(&instrument).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "Instrument not found",
		})
		return
	}

	// Parse images as JSON array
	outboundPhotos := []string{}
	if instrument.Images != "" && instrument.Images != "[]" {
		// TODO: Parse JSON array properly
		outboundPhotos = append(outboundPhotos, "/uploads/default.jpg")
	}

	// For now, use placeholder return photos
	returnPhotos := []string{"/uploads/return1.jpg", "/uploads/return2.jpg"}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"order":          order,
			"instrument":     instrument,
			"outboundPhotos": outboundPhotos,
			"returnPhotos":   returnPhotos,
		},
	})
}

// SubmitAssessment submits damage assessment and optionally creates maintenance ticket
func (h *AssessmentHandler) SubmitAssessment(c *gin.Context) {
	orderID := c.Param("order_id")

	var req struct {
		HasDamage  bool   `json:"hasDamage"`
		Signature  string `json:"signature"`
		AssessedAt string `json:"assessedAt"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "Invalid request data",
		})
		return
	}

	// Fetch order to get instrument ID
	var order struct {
		InstrumentID string `json:"instrument_id"`
		Status       string `json:"status"`
	}

	if err := h.db.Table("orders").Where("id = ?", orderID).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "Order not found",
		})
		return
	}

	// If has damage, create maintenance ticket and update instrument status
	if req.HasDamage {
		// Create maintenance ticket
		ticket := map[string]interface{}{
			"order_id":            orderID,
			"instrument_id":       order.InstrumentID,
			"status":              "PENDING",
			"problem_description": "归还定损发现损坏",
			"created_at":          time.Now(),
		}

		if err := h.db.Table("maintenance_tickets").Create(&ticket).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "Failed to create maintenance ticket",
			})
			return
		}

		// Update instrument status to repairing
		if err := h.db.Table("instruments").Where("id = ?", order.InstrumentID).Update("stock_status", "repairing").Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "Failed to update instrument status",
			})
			return
		}
	}

	// Update order status to assessed
	if err := h.db.Table("orders").Where("id = ?", orderID).Update("status", "assessed").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to update order status",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "Assessment submitted successfully",
		"data": gin.H{
			"maintenance_ticket_created": req.HasDamage,
			"instrument_status_updated":  req.HasDamage,
		},
	})
}
