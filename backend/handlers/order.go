package handlers

import (
	"net/http"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/internal/service"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

	// Generate order ID (UUID format)
	orderID := uuid.New().String()

	// Get tenant ID from context
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		tenantID = "default_tenant"
	}
	orgID := c.GetString("org_id")

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
		OrgID:        orgID,
		UserID:       c.GetString("user_id"),
		InstrumentID: req.InstrumentID,
		Level:        req.Level,
		LeaseTerm:    req.LeaseTerm,
		DepositMode:  req.DepositMode,
		MonthlyRent:  resp.FirstMonthRent,
		Deposit:      resp.Deposit,
		Status:       "pending", // pending, paid, in_lease, completed, cancelled
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

// GetOrders retrieves order list with pagination and status filter
func GetOrders(c *gin.Context) {
	page := 1
	pageSize := 10
	status := c.Query("status")

	// Parse pagination parameters
	// Note: Simplified for brevity, should parse from query params in real implementation

	db := database.GetDB().WithContext(c.Request.Context())
	query := db.Model(&models.Order{})

	// Filter by status if provided
	if status != "" {
		query = query.Where("status = ?", status)
	}

	// Get total count
	var total int64
	query.Count(&total)

	// Get orders with pagination
	var orders []models.Order
	offset := (page - 1) * pageSize
	query.Offset(offset).Limit(pageSize).Find(&orders)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":  orders,
			"total": total,
			"page":  page,
		},
	})
}

// GetOrder retrieves a single order by ID
func GetOrder(c *gin.Context) {
	orderID := c.Param("id")
	if orderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "order_id is required",
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())
	var order models.Order
	if err := db.Where("id = ?", orderID).First(&order).Error; err != nil {
		if err.Error() == "record not found" {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    40400,
				"message": "order not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to fetch order: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": order,
	})
}

// PayOrder handles order payment (pending -> paid)
func PayOrder(c *gin.Context) {
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

	if order.Status != "pending" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "order can only be paid when status is pending",
		})
		return
	}

	// Update order status to paid
	if err := db.Model(&order).Update("status", "paid").Error; err != nil {
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
			"old_status": "pending",
			"new_status": "paid",
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

	// Find order and check status
	var order models.Order
	if err := db.Where("id = ?", orderID).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "order not found",
		})
		return
	}

	if order.Status != "paid" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "order can only be picked up when status is paid",
		})
		return
	}

	// Update order status to in_lease
	if err := db.Model(&order).Update("status", "in_lease").Error; err != nil {
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
			"old_status": "paid",
			"new_status": "in_lease",
			"updated_at": time.Now().Format(time.RFC3339),
		},
	})
}

// ReturnOrder confirms order return (in_lease -> completed)
func ReturnOrder(c *gin.Context) {
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

	if order.Status != "in_lease" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "order can only be returned when status is in_lease",
		})
		return
	}

	// Update order status to completed
	if err := db.Model(&order).Update("status", "completed").Error; err != nil {
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
			"old_status": "in_lease",
			"new_status": "completed",
			"updated_at": time.Now().Format(time.RFC3339),
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

	if order.Status != "pending" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "order can only be cancelled when status is pending",
		})
		return
	}

	// Restore inventory when cancelling order (optional but recommended)
	// Get instrument and update stock_status back to available
	if err := db.Model(&models.Instrument{}).Where("id = ?", order.InstrumentID).Update("stock_status", "available").Error; err != nil {
		// Log error but continue with cancellation
		// In production, might want to handle this more carefully
	}

	// Update order status to cancelled
	if err := db.Model(&order).Update("status", "cancelled").Error; err != nil {
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
			"old_status": "pending",
			"new_status": "cancelled",
			"updated_at": time.Now().Format(time.RFC3339),
		},
	})
}
