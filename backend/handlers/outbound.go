package handlers

import (
	"net/http"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// OutboundHandler handles outbound confirmation for mini-program
type OutboundHandler struct {
	db *gorm.DB
}

// NewOutboundHandler creates a new outbound handler
func NewOutboundHandler(db *gorm.DB) *OutboundHandler {
	return &OutboundHandler{db: db}
}

// GetOutboundPhotos returns outbound photos for confirmation
func (h *OutboundHandler) GetOutboundPhotos(c *gin.Context) {
	orderID := c.Param("id")

	// Fetch order with instrument
	var order struct {
		ID           string `json:"id"`
		InstrumentID string `json:"instrument_id"`
	}

	if err := h.db.Table("orders").Where("id = ?", orderID).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "订单不存在",
		})
		return
	}

	// Query instrument_media for shipping (outbound) photos
	var media []models.InstrumentMedia
	h.db.Where("instrument_id = ? AND batch_type = ? AND file_type = ?",
		order.InstrumentID, "shipping", "image").
		Order("sort_order ASC").
		Find(&media)

	outboundPhotos := make([]map[string]interface{}, 0)
	for _, m := range media {
		outboundPhotos = append(outboundPhotos, map[string]interface{}{
			"url":      m.StorageKey,
			"batch_id": m.BatchID,
			"taken_at": m.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"outbound_photos":   outboundPhotos,
			"assessment_photos": []interface{}{},
		},
	})
}

// ConfirmOutbound handles outbound confirmation from mini-program
func (h *OutboundHandler) ConfirmOutbound(c *gin.Context) {
	orderID := c.Param("id")

	var req struct {
		Confirmed bool   `json:"confirmed"`
		UserId    string `json:"userId"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "无效的请求数据",
		})
		return
	}

	if !req.Confirmed {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40001,
			"message": "需要确认出库",
		})
		return
	}

	// Update order status
	if err := h.db.Table("orders").Where("id = ?", orderID).Update("status", "outbound_confirmed").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "更新订单状态失败",
		})
		return
	}

	// Update instrument status to rented
	var order struct {
		InstrumentID string `json:"instrument_id"`
	}

	if err := h.db.Table("orders").Where("id = ?", orderID).First(&order).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "获取订单信息失败",
		})
		return
	}

	if err := h.db.Table("instruments").Where("id = ?", order.InstrumentID).Update("stock_status", "rented").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "更新乐器状态失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "出库确认成功",
		"data": gin.H{
			"orderId":      orderID,
			"status":       "outbound_confirmed",
			"instrumentId": order.InstrumentID,
		},
	})
}
