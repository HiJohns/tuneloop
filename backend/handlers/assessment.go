package handlers

import (
	"net/http"
	"time"
	"tuneloop-backend/models"

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
	orderID := c.Param("id")

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

	// Query instrument_media for shipping (outbound) photos
	var outboundMedia []models.InstrumentMedia
	h.db.Where("instrument_id = ? AND batch_type = ? AND file_type = ?",
		order.InstrumentID, "shipping", "image").
		Order("sort_order ASC").
		Find(&outboundMedia)

	// Query instrument_media for receiving/returning (return) photos
	var returnMedia []models.InstrumentMedia
	h.db.Where("instrument_id = ? AND batch_type IN ? AND file_type = ?",
		order.InstrumentID, []string{"receiving", "returning"}, "image").
		Order("sort_order ASC").
		Find(&returnMedia)

	outboundPhotos := make([]string, 0)
	for _, m := range outboundMedia {
		outboundPhotos = append(outboundPhotos, m.StorageKey)
	}

	returnPhotos := make([]string, 0)
	for _, m := range returnMedia {
		returnPhotos = append(returnPhotos, m.StorageKey)
	}

	damageLevel := "none"
	if order.Status == models.OrderStatusCompleted {
		damageLevel = "damaged"
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"outbound_condition": gin.H{
				"photos": outboundPhotos,
			},
			"return_condition": gin.H{
				"photos":      returnPhotos,
				"damage_level": damageLevel,
			},
			"assessment_status": "pending",
		},
	})
}

// SubmitAssessment submits damage assessment and optionally creates maintenance ticket
func (h *AssessmentHandler) SubmitAssessment(c *gin.Context) {
	orderID := c.Param("id")

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

		// Update instrument status to maintenance + repair_pending
		if err := h.db.Table("instruments").Where("id = ?", order.InstrumentID).
			Updates(map[string]interface{}{"stock_status": models.StockStatusMaintenance, "repair_status": "repair_pending"}).Error; err != nil {
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
