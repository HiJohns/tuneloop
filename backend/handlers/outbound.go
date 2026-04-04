package handlers

import (
	"net/http"
	"time"

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
	orderID := c.Param("order_id")

	// Fetch order with instrument
	var order struct {
		ID           string    `json:"id"`
		UserID       string    `json:"user_id"`
		InstrumentID string    `json:"instrument_id"`
		Status       string    `json:"status"`
		CreatedAt    time.Time `json:"created_at"`
	}

	if err := h.db.Table("orders").Where("id = ?", orderID).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "订单不存在",
		})
		return
	}

	// Fetch instrument with photos
	var instrument struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Brand       string `json:"brand"`
		Model       string `json:"model"`
		Images      string `json:"images"`
		StockStatus string `json:"stock_status"`
	}

	if err := h.db.Table("instruments").Where("id = ?", order.InstrumentID).First(&instrument).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "乐器不存在",
		})
		return
	}

	// Parse photos from JSON
	photos := []string{}
	if instrument.Images != "" && instrument.Images != "[]" {
		// TODO: Parse actual JSON array
		// For now, return placeholder
		photos = []string{"/uploads/default.jpg"}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"orderId":        orderID,
			"instrumentName": instrument.Name,
			"instrumentId":   instrument.ID,
			"photos":         photos,
			"confirmed":      order.Status == "outbound_confirmed",
		},
	})
}

// ConfirmOutbound handles outbound confirmation from mini-program
func (h *OutboundHandler) ConfirmOutbound(c *gin.Context) {
	orderID := c.Param("order_id")

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
