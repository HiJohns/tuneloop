package handlers

import (
	"net/http"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetOrders retrieves order list with pagination and status filter
func GetOrders(c *gin.Context) {
	page := 1
	pageSize := 10
	status := c.Query("status")

	// Parse pagination parameters
	// Note: Simplified for brevity, should parse from query params in real implementation

	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	var req struct {
		PaymentMethod   string `json:"payment_method"`
		DeliveryAddress string `json:"delivery_address"`
	}
	_ = c.ShouldBindJSON(&req)

	// Find order and check status
	var order models.Order
	query := db.Where("id = ?", orderID)
	if tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}
	if middleware.GetRole(c.Request.Context()) == "USER" {
		query = query.Where("user_id = ?", middleware.GetUserID(c.Request.Context()))
	}
	if err := query.First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "order not found",
		})
		return
	}

	if order.Status != models.OrderStatusReserved {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "order can only be paid when status is reserved",
		})
		return
	}

	// Update order status to paid
	if err := db.Model(&order).Update("status", models.OrderStatusPaid).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to update order status: " + err.Error(),
		})
		return
	}

	// Update delivery_address if provided
	if req.DeliveryAddress != "" {
		if err := db.Table("lease_sessions").Where("order_id = ?", orderID).Update("delivery_address", req.DeliveryAddress).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "failed to update delivery address: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"order_id":   orderID,
			"old_status": models.OrderStatusReserved,
			"new_status": models.OrderStatusPaid,
			"updated_at": time.Now().Format(time.RFC3339),
		},
	})
}

// PickupOrder confirms order pickup (paid -> in_lease)
func PickupOrder(c *gin.Context) {
	orderID := c.Param("id")
	if orderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "order_id is required",
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())
	tenantID := middleware.GetTenantID(c.Request.Context())

	// Find order and check status
	var order models.Order
	query := db.Where("id = ?", orderID)
	if tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}
	if middleware.GetRole(c.Request.Context()) == "USER" {
		query = query.Where("user_id = ?", middleware.GetUserID(c.Request.Context()))
	}
	if err := query.First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "order not found",
		})
		return
	}

	if order.Status != models.OrderStatusPaid {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "order can only be picked up when status is paid",
		})
		return
	}

	// Update order status to in_lease
	if err := db.Model(&order).Update("status", models.OrderStatusInLease).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to update order status: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"order_id":   orderID,
			"old_status": models.OrderStatusPaid,
			"new_status": models.OrderStatusInLease,
			"updated_at": time.Now().Format(time.RFC3339),
		},
	})
}

// ReturnOrder initiates order return (in_lease -> returning)
func ReturnOrder(c *gin.Context) {
	orderID := c.Param("id")
	if orderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "order_id is required",
		})
		return
	}

	var req struct {
		CourierCompany string   `json:"courier_company"`
		TrackingNumber string   `json:"tracking_number"`
		Photos         []string `json:"photos"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	// Find order
	var order models.Order
	if err := db.Where("id = ?", orderID).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "order not found",
		})
		return
	}

	if order.Status != models.OrderStatusInLease {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "order can only be returned when status is in_lease",
		})
		return
	}

	// Verify user ownership (customer-facing, no tenant context)
	userID := middleware.GetUserID(ctx)
	if order.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    40300,
			"message": "not your order",
		})
		return
	}

	// Update order status: in_lease -> returning
	if err := db.Model(&models.Order{}).Where("id = ? AND tenant_id = ?", orderID, order.TenantID).
		Updates(map[string]interface{}{
			"status":          models.OrderStatusReturning,
			"courier_company": req.CourierCompany,
			"tracking_number": req.TrackingNumber,
		}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to update order status: " + err.Error(),
		})
		return
	}

	// Instrument stays rented during return transit
	if err := db.Model(&models.Instrument{}).Where("id = ?", order.InstrumentID).
		Update("stock_status", models.StockStatusRented).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to update instrument status: " + err.Error(),
		})
		return
	}

	// Record status history
	history := models.OrderStatusHistory{
		ID:         uuid.New().String(),
		TenantID:   order.TenantID,
		OrderID:    orderID,
		StatusFrom: models.OrderStatusInLease,
		StatusTo:   models.OrderStatusReturning,
		Notes:      "顾客发起归还",
		ChangedBy:  stringPtr(userID),
		ChangedAt:  time.Now(),
	}
	if err := db.Create(&history).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to record status history: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"order_id":   orderID,
			"old_status": models.OrderStatusInLease,
			"new_status": models.OrderStatusReturning,
		},
	})
}

// CancelOrder cancels an order (pending -> cancelled)
func CancelOrder(c *gin.Context) {
	orderID := c.Param("id")
	if orderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "order_id is required",
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())

	// Find order and check status
	var order models.Order
	if err := db.Where("id = ?", orderID).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "order not found",
		})
		return
	}

	if order.Status != models.OrderStatusReserved && order.Status != models.OrderStatusPaid {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "order can only be cancelled when status is reserved or paid",
		})
		return
	}

	// Restore inventory when cancelling order
	if err := db.Model(&models.Instrument{}).Where("id = ?", order.InstrumentID).Update("stock_status", models.StockStatusAvailable).Error; err != nil {
		// Log error but continue with cancellation
	}

	// Update order status to cancelled
	if err := db.Model(&order).Update("status", models.OrderStatusCancelled).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to update order status: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"order_id":   orderID,
			"old_status": order.Status,
			"new_status": models.OrderStatusCancelled,
			"updated_at": time.Now().Format(time.RFC3339),
		},
	})
}

// GetOrderByInstrumentSN GET /api/orders/by-instrument-sn - Find active order by instrument SN
func GetOrderByInstrumentSN(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	sn := c.Query("sn")

	if sn == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "sn parameter is required"})
		return
	}

	db := database.GetDB().WithContext(ctx)

	var instrument models.Instrument
	if tenantID != "" {
		if err := db.Where("sn = ? AND tenant_id = ?", sn, tenantID).First(&instrument).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "instrument not found"})
			return
		}
	} else {
		if err := db.Where("sn = ?", sn).First(&instrument).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "instrument not found"})
			return
		}
	}

	var order models.Order
	if err := db.Where("instrument_id = ? AND status NOT IN ?",
		instrument.ID, []string{models.OrderStatusCancelled, models.OrderStatusCompleted}).
		Order("created_at DESC").First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "未找到该乐器的活跃订单"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"order_id":       order.ID,
			"order_status":   order.Status,
			"instrument_id":  instrument.ID,
			"instrument_sn":  sn,
			"start_date":     order.StartDate,
			"end_date":       order.EndDate,
			"monthly_rent":   order.MonthlyRent,
			"deposit":        order.Deposit,
		},
	})
}
