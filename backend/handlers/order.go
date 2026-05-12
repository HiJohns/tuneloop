package handlers

import (
	"encoding/json"
	"net/http"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/internal/service"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm/clause"
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

	// Parse instrument pricing
	db := database.GetDB().WithContext(c.Request.Context())
	var instrument models.Instrument
	if err := db.First(&instrument, "id = ?", req.InstrumentID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "instrument not found"})
		return
	}

	var monthlyRent, deposit, shippingFee float64
	var pricingList []map[string]interface{}
	if instrument.Pricing != "" {
		if err := json.Unmarshal([]byte(instrument.Pricing), &pricingList); err == nil && len(pricingList) > 0 {
			p := pricingList[0]
			dailyRent, _ := p["daily_rent"].(float64)
			deposit, _ = p["deposit"].(float64)
			shippingFee, _ = p["shipping_fee"].(float64)
			monthlyRent = dailyRent * 25
		}
	}

	var resp *service.PricingResponse
	if monthlyRent > 0 {
		resp = &service.PricingResponse{
			FirstMonthRent: monthlyRent,
			Deposit:       deposit,
			TotalAmount:   monthlyRent + deposit + shippingFee,
		}
	} else {
		// fallback to pricing service
		creditScore := 600
		if uID := c.GetString("user_id"); uID != "" {
			var user models.User
			if err := db.First(&user, "id = ?", uID).Error; err == nil {
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

		var pricingErr error
		resp, pricingErr = pricingService.CalculatePrice(c.Request.Context(), pricingReq)
		if pricingErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "pricing calculation failed: " + pricingErr.Error(),
			})
			return
		}
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

	// Get tenant ID from JWT context
	tenantID := middleware.GetTenantID(c.Request.Context())
	if tenantID == "" {
		// For customers not belonging to any tenant, use zero UUID
		tenantID = "00000000-0000-0000-0000-000000000000"
	}
	orgID := middleware.GetOrgID(c.Request.Context())
	if orgID == "" {
		orgID = "00000000-0000-0000-0000-000000000000"
	}
	userID := middleware.GetUserID(c.Request.Context())
	if userID == "" {
		userID = "00000000-0000-0000-0000-000000000000"
	}

	// Check instrument and get actual pricing
	db := database.GetDB().WithContext(c.Request.Context())
	var instrument models.Instrument
	if err := db.First(&instrument, "id = ?", req.InstrumentID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "instrument not found",
		})
		return
	}

	// 顾客无租户时继承乐器的 tenant_id
	if tenantID == "00000000-0000-0000-0000-000000000000" && instrument.TenantID != "" {
		tenantID = instrument.TenantID
	}

	// Check if instrument is available
	if instrument.StockStatus != "available" {
		c.JSON(http.StatusConflict, gin.H{
			"code":    40900,
			"message": "instrument already reserved",
		})
		return
	}

	// Begin transaction with row lock to prevent oversell
	tx := db.Begin()
	var lockedInstrument models.Instrument
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND stock_status = ?", req.InstrumentID, models.StockStatusAvailable).
		First(&lockedInstrument).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{
			"code":    40900,
			"message": "instrument already reserved",
		})
		return
	}

	// Update inventory status to reserved
	if err := tx.Model(&lockedInstrument).Update("stock_status", models.StockStatusReserved).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to reserve instrument: " + err.Error(),
		})
		return
	}

	// Parse pricing from instrument.Pricing JSON
	var monthlyRent, deposit, shippingFee float64
	var pricingList []map[string]interface{}
	if instrument.Pricing != "" {
		if err := json.Unmarshal([]byte(instrument.Pricing), &pricingList); err == nil && len(pricingList) > 0 {
			p := pricingList[0]
			dailyRent, _ := p["daily_rent"].(float64)
			deposit, _ = p["deposit"].(float64)
			shippingFee, _ = p["shipping_fee"].(float64)
			// monthly rent = daily_rent * 25 (cases.md §2.2)
			monthlyRent = dailyRent * 25
		}
	}

	// Calculate pricing result
	var pricingResp *service.PricingResponse
	if monthlyRent > 0 {
		pricingResp = &service.PricingResponse{
			FirstMonthRent: monthlyRent,
			Deposit:       deposit,
			TotalAmount:   monthlyRent + deposit + shippingFee,
		}
	} else {
		// Fallback to pricing service for legacy data
		creditScore := 600
		if userID != "00000000-0000-0000-0000-000000000000" {
			var user models.User
			if err := db.First(&user, "iam_sub = ?", userID).Error; err == nil {
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
		var err error
		pricingResp, err = pricingService.CalculatePrice(c.Request.Context(), pricingReq)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "pricing calculation failed: " + err.Error(),
			})
			return
		}
		shippingFee = 0
	}

	// Create order record
	order := models.Order{
		ID:           orderID,
		TenantID:     tenantID,
		OrgID:        orgID,
		UserID:       userID,
		InstrumentID: req.InstrumentID,
		Level:        req.Level,
		LeaseTerm:    req.LeaseTerm,
		DepositMode:  req.DepositMode,
		MonthlyRent:  pricingResp.FirstMonthRent,
		Deposit:     pricingResp.Deposit,
		ShippingFee:  shippingFee,
		Status:       "pending",
	}

	// Create order record within transaction
	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to create order: " + err.Error(),
		})
		return
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to commit transaction: " + err.Error(),
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
			"first_payment_amount": pricingResp.TotalAmount,
			"created_at":         time.Now().Format(time.RFC3339),
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
	tenantID := middleware.GetTenantID(c.Request.Context())
	query := db.Model(&models.Order{}).Where("tenant_id = ?", tenantID)

	// Filter by user_id if available (for customer profile view)
	if userID := middleware.GetUserID(c.Request.Context()); userID != "" {
		query = query.Where("user_id = ?", userID)
	}

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
	if err := db.Model(&models.Instrument{}).Where("id = ?", order.InstrumentID).Update("stock_status", models.StockStatusAvailable).Error; err != nil {
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
	if err := db.Where("sn = ? AND tenant_id = ?", sn, tenantID).First(&instrument).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "instrument not found"})
		return
	}

	var order models.Order
	if err := db.Where("instrument_id = ? AND status NOT IN ?",
		instrument.ID, []string{"cancelled", "completed"}).
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
		},
	})
}
