package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strconv"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetOrders retrieves order list with pagination and status filter
func GetOrders(c *gin.Context) {
	page := 1
	pageSize := 10
	status := c.Query("status")

	// Parse pagination parameters
	if p, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil && p > 0 {
		page = p
	}
	if ps, err := strconv.Atoi(c.DefaultQuery("page_size", "10")); err == nil && ps > 0 {
		pageSize = ps
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(c.Request.Context())
	userID := middleware.GetUserID(c.Request.Context())
	query := db.Model(&models.Order{})
	if tenantID != "" {
		// 员工：按所属网点过滤（从 site_members 获取用户关联的所有网点）
		var currentUser models.User
		if err := db.Where("iam_sub = ?", userID).First(&currentUser).Error; err == nil && currentUser.ID != "" {
			var memberSiteIDs []string
			db.Table("site_members").
				Where("user_id = ?", currentUser.ID).
				Pluck("site_id", &memberSiteIDs)
			if len(memberSiteIDs) > 0 {
				query = query.Joins("JOIN instruments ON instruments.id = orders.instrument_id").
					Where("instruments.site_id IN ?", memberSiteIDs)
			}
		}
	} else {
		// 顾客无租户：只看自己的订单
		if userID != "" {
			var localUser models.User
			if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err == nil {
				query = query.Where("user_id = ?", localUser.ID)
			} else {
				query = query.Where("user_id = ?", userID)
			}
		}
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
	query.Order("updated_at DESC").Offset(offset).Limit(pageSize).Find(&orders)

	// Enrich orders with instrument info and settlement data
	type orderListItem struct {
		models.Order
		InstrumentName     string   `json:"instrument_name"`
		InstrumentCategory string   `json:"instrument_category"`
		UserName           string   `json:"user_name"`
		ActualRentAmount   *float64 `json:"actual_rent_amount,omitempty"`
		CoverImage         string   `json:"cover_image"`
	}
	list := make([]orderListItem, 0, len(orders))
	storageSvc := services.MediaStorageFromContext(c)
	for _, o := range orders {
		item := orderListItem{Order: o}
		var instr models.Instrument
		if err := db.Raw("SELECT sn, category_name FROM instruments WHERE id = ? LIMIT 1", o.InstrumentID).Scan(&instr).Error; err == nil {
			item.InstrumentName = instr.SN
			item.InstrumentCategory = instr.CategoryName
		}
		var media models.InstrumentMedia
		if err := db.Where("instrument_id = ? AND is_display = ?", o.InstrumentID, true).Order("sort_order ASC").First(&media).Error; err == nil && media.StorageKey != "" {
			url, _ := storageSvc.GetURL(ctx, media.StorageKey)
			if url != "" {
				item.CoverImage = url
			} else {
				item.CoverImage = "/uploads/media/" + media.StorageKey
			}
		}
		var user models.User
		if err := db.Raw("SELECT name FROM users WHERE id = ? LIMIT 1", o.UserID).Scan(&user).Error; err == nil {
			item.UserName = user.Name
		}
		var settlement models.Settlement
		if err := db.Where("order_id = ?", o.ID).Order("created_at DESC").First(&settlement).Error; err == nil {
			item.ActualRentAmount = &settlement.ActualRentAmount
		}
		list = append(list, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":  list,
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
	tenantID := middleware.GetTenantID(c.Request.Context())
	userID := middleware.GetUserID(c.Request.Context())
	role := middleware.GetRole(c.Request.Context())
	log.Printf("[GetOrder] orderID=%s tenantID=%q userID=%q role=%q", orderID, tenantID, userID, role)
	var order models.Order
	query := db.Where("id = ?", orderID)
	if tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}
	if middleware.GetRole(c.Request.Context()) == "USER" && userID != "" {
		// Resolve local user ID from IAM sub (order stores local UUID, not IAM sub)
		var localUser models.User
		if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err == nil {
			query = query.Where("user_id = ?", localUser.ID)
		} else {
			// Fallback to direct IAM sub comparison (shadow user where ID == iam_sub)
			query = query.Where("user_id = ?", userID)
		}
	}
	if err := query.First(&order).Error; err != nil {
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

	// Fetch user name (graceful fallback — use Raw to bypass tenant scope on users table)
	userName := ""
	userEmail := ""
	userPhone := ""
	userIAMSub := ""
	var user models.User
	if err := db.Raw("SELECT * FROM users WHERE id = ? LIMIT 1", order.UserID).Scan(&user).Error; err == nil && user.ID != "" {
		userName = user.Name
		userEmail = user.Email
		userPhone = user.Phone
		userIAMSub = user.IAMSub
	}
	// If local user has no name, try to fetch from IAM
	if userName == "" && userIAMSub != "" {
		iamClient := services.NewIAMClient()
		if iamUser, iamErr := iamClient.GetUser(userIAMSub); iamErr == nil && iamUser.Name != "" {
			userName = iamUser.Name
			userEmail = iamUser.Email
			userPhone = iamUser.Phone
			db.Model(&user).Where("id = ?", order.UserID).Updates(map[string]interface{}{
				"name":  iamUser.Name,
				"email": iamUser.Email,
				"phone": iamUser.Phone,
			})
		}
	}

	// Fetch delivery address from lease_session
	deliveryAddress := ""
	var leaseSession struct{ DeliveryAddress string }
	if err := db.Raw("SELECT COALESCE(delivery_address::text, '') as delivery_address FROM lease_sessions WHERE order_id = ? LIMIT 1", orderID).Scan(&leaseSession).Error; err == nil {
		deliveryAddress = leaseSession.DeliveryAddress
	}

	// Fetch instrument info
	var instrument models.Instrument
	instrumentName := ""
	instrumentCategory := ""
	if err := db.Raw("SELECT sn, category_name FROM instruments WHERE id = ? LIMIT 1", order.InstrumentID).Scan(&instrument).Error; err == nil {
		instrumentName = instrument.SN
		instrumentCategory = instrument.CategoryName
	}

	// Fetch settlement
	var settlement models.Settlement
	var settlementData map[string]interface{}
	if err := db.Where("order_id = ?", order.ID).Order("created_at DESC").First(&settlement).Error; err == nil {
		settlementData = map[string]interface{}{
			"id":                    settlement.ID,
			"actual_rent_days":      settlement.ActualRentDays,
			"actual_rent_amount":    settlement.ActualRentAmount,
			"original_rent_amount":  settlement.OriginalRentAmount,
			"gift_points_refunded":  settlement.GiftPointsRefunded,
			"cash_refundable":       settlement.CashRefundable,
			"prepaid_refunded":      settlement.PrepaidRefunded,
			"refund_method":         settlement.RefundMethod,
			"refund_status":         settlement.RefundStatus,
			"overdue_charges_total": settlement.OverdueChargesTotal,
		}
		if settlement.Breakdown != "" {
			var breakdown map[string]interface{}
			if err := json.Unmarshal([]byte(settlement.Breakdown), &breakdown); err == nil {
				settlementData["breakdown"] = breakdown
			}
		}
	}

	// Fetch order logs
	var orderLogs []models.OrderLog
	db.Where("order_id = ?", order.ID).Order("created_at ASC").Find(&orderLogs)

	// Parse pricing_breakdown
	var pricingBreakdownData interface{}
	if order.PricingBreakdown != nil && *order.PricingBreakdown != "" {
		var pb map[string]interface{}
		if err := json.Unmarshal([]byte(*order.PricingBreakdown), &pb); err == nil {
			// Ensure shipping_fee from order is included in pricing_breakdown
			if _, hasFee := pb["shipping_fee"]; !hasFee && order.ShippingFee > 0 {
				pb["shipping_fee"] = order.ShippingFee
			}
			pricingBreakdownData = pb
		}
	}

	orderData := map[string]interface{}{
		"id":                    order.ID,
		"tenant_id":             order.TenantID,
		"user_id":               order.UserID,
		"user_name":             userName,
		"user_email":            userEmail,
		"user_phone":            userPhone,
		"instrument_id":         order.InstrumentID,
		"instrument_name":       instrumentName,
		"instrument_category":   instrumentCategory,
		"level":                 order.Level,
		"lease_term":            order.LeaseTerm,
		"deposit_mode":          order.DepositMode,
		"monthly_rent":          order.MonthlyRent,
		"deposit":               order.Deposit,
		"shipping_fee":          order.ShippingFee,
		"accumulated_months":    order.AccumulatedMonths,
		"status":                order.Status,
		"start_date":            order.StartDate,
		"end_date":              order.EndDate,
		"tracking_number":       order.TrackingNumber,
		"courier_company":       order.CourierCompany,
		"shipped_at":            order.ShippedAt,
		"delivered_at":          order.DeliveredAt,
		"returned_at":           order.ReturnedAt,
		"delivery_address":      deliveryAddress,
		"created_at":            order.CreatedAt,
		"updated_at":            order.UpdatedAt,
		"pricing_breakdown":     pricingBreakdownData,
		"settlement":            settlementData,
		"order_logs":            orderLogs,
	}

	transitInfo := GetMerchantTransitInfo(c.Request.Context(), order.TenantID)
	if transitInfo != nil && transitInfo.MerchantType == models.MerchantTypeControlled {
		orderData["transit_info"] = map[string]string{
			"address": transitInfo.Address,
			"phone":   transitInfo.Phone,
			"contact": transitInfo.ContactName,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": orderData,
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
		iamSub := middleware.GetUserID(c.Request.Context())
		var localUser models.User
		if err := db.Where("iam_sub = ?", iamSub).First(&localUser).Error; err == nil {
			query = query.Where("user_id = ?", localUser.ID)
		} else {
			query = query.Where("user_id = ?", iamSub)
		}
	}
	if err := query.First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "order not found",
		})
		return
	}

	// Guest (tid empty): derive tenant/org from order (which inherited from instrument)
	if tenantID == "" {
		tenantID = order.TenantID
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
		iamSub := middleware.GetUserID(c.Request.Context())
		var localUser models.User
		if err := db.Where("iam_sub = ?", iamSub).First(&localUser).Error; err == nil {
			query = query.Where("user_id = ?", localUser.ID)
		} else {
			query = query.Where("user_id = ?", iamSub)
		}
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
	var localUser models.User
	if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err == nil {
		userID = localUser.ID
	}
	if order.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    40300,
			"message": "not your order",
		})
		return
	}

	now := time.Now()
	// Update order status: in_lease -> returning
	if err := db.Model(&models.Order{}).Where("id = ? AND tenant_id = ?", orderID, order.TenantID).
		Updates(map[string]interface{}{
			"status":          models.OrderStatusReturning,
			"courier_company": req.CourierCompany,
			"tracking_number": req.TrackingNumber,
			"returned_at":     now,
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
		ChangedAt:  now,
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

// GetOrderLogs retrieves ordered timeline of events for an order
func GetOrderLogs(c *gin.Context) {
	orderID := c.Param("id")
	if orderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "order_id is required"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var order models.Order
	if err := db.Where("id = ?", orderID).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "order not found"})
		return
	}

	// Data isolation: verify access rights
	tenantID := middleware.GetTenantID(ctx)
	userID := middleware.GetUserID(ctx)
	role := middleware.GetRole(ctx)

	if tenantID != "" && order.TenantID != tenantID {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "order not found"})
		return
	}
	if role == "USER" && userID != "" {
		var localUser models.User
		if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err == nil {
			if order.UserID != localUser.ID {
				c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "order not found"})
				return
			}
		} else {
			if order.UserID != userID {
				c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "order not found"})
				return
			}
		}
	}

	type logEntry struct {
		Event     string    `json:"event"`
		Time      time.Time `json:"time"`
		Operator  string    `json:"operator"`
		CreatedAt time.Time `json:"created_at"`
	}
	logs := []logEntry{}

	// 1. Order created
	logs = append(logs, logEntry{
		Event:     "created",
		Time:      order.CreatedAt,
		Operator:  "customer",
		CreatedAt: order.CreatedAt,
	})

	// 2. Status transitions from order_status_history
	var history []models.OrderStatusHistory
	db.Where("order_id = ?", orderID).Order("changed_at ASC").Find(&history)
	for _, h := range history {
		op := "customer"
		if h.ChangedBy != nil {
			var operator models.User
			if err := db.Raw("SELECT name FROM users WHERE id = ? LIMIT 1", *h.ChangedBy).Scan(&operator).Error; err == nil && operator.Name != "" {
				op = operator.Name
			}
		}
		eventLabel := h.StatusTo
		logs = append(logs, logEntry{
			Event:     eventLabel,
			Time:      h.ChangedAt,
			Operator:  op,
			CreatedAt: h.CreatedAt,
		})
	}

	// 3. Settlement confirmed (from settlements table)
	var settlement models.Settlement
	if err := db.Where("order_id = ?", orderID).Order("created_at DESC").First(&settlement).Error; err == nil {
		logs = append(logs, logEntry{
			Event:     "settlement_confirmed",
			Time:      settlement.CreatedAt,
			Operator:  "system",
			CreatedAt: settlement.CreatedAt,
		})
	}

	// Sort by time ascending
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].Time.Before(logs[j].Time)
	})

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"logs": logs,
		},
	})
}
