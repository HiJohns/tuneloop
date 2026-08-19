package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/services/wechatpay"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
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
		// 顾客无租户：只看自己的订单。匿名（无 token / 游客）不得查看
		// 任何订单——空 userID 不加过滤会泄漏全库订单（#1694 修复）。
		if userID == "" {
			c.JSON(http.StatusOK, gin.H{
				"code": 20000,
				"data": gin.H{
					"list":  []interface{}{},
					"total": 0,
					"page":  page,
					"page_size": pageSize,
				},
			})
			return
		}
		var localUser models.User
		if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err == nil {
			query = query.Where("user_id = ?", localUser.ID)
		} else {
			query = query.Where("user_id = ?", userID)
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
		var instr struct {
			SN           string `gorm:"column:sn"`
			CategoryName string `gorm:"column:category_name"`
			CoverImage   string `gorm:"column:cover_image"`
		}
		if err := db.Raw("SELECT sn, category_name, cover_image FROM instruments WHERE id = ? LIMIT 1", o.InstrumentID).Scan(&instr).Error; err == nil {
			item.InstrumentName = instr.SN
			item.InstrumentCategory = instr.CategoryName
			item.CoverImage = instr.CoverImage
		}
		// Fallback: if no dedicated cover, use first display media
		if item.CoverImage == "" {
			var media models.InstrumentMedia
			if err := db.Where("instrument_id = ? AND is_display = ?", o.InstrumentID, true).Order("sort_order ASC").First(&media).Error; err == nil && media.StorageKey != "" {
				url, _ := storageSvc.GetURL(ctx, media.StorageKey)
				if url != "" {
					item.CoverImage = url
				} else {
					item.CoverImage = "/uploads/media/" + media.StorageKey
				}
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
	// 匿名（无 tenant 且无 userID）不得读取订单详情（#1694 泄漏修复）。
	if tenantID == "" && userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "请先登录"})
		return
	}
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
		} else if iamErr != nil {
			log.Printf("[OrderDetail] IAM GetUser failed for iam_sub=%s: %v (local cache may be stale)", userIAMSub, iamErr)
		}
	}

	// Fetch delivery address from lease_session (JSONB: string value stored
	// with quotes — #>> '{}' unwraps it; plain text passes through)
	deliveryAddress := ""
	var leaseSession struct{ DeliveryAddress string }
	if err := db.Raw(`SELECT COALESCE(
			CASE WHEN jsonb_typeof(delivery_address) = 'string' THEN delivery_address #>> '{}'
			     ELSE delivery_address::text END, '') as delivery_address
		FROM lease_sessions WHERE order_id = ? LIMIT 1`, orderID).Scan(&leaseSession).Error; err == nil {
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
	instrumentSN := instrument.SN

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

	// Fetch order logs (paginated via logs_limit query param)
	logsLimit, _ := strconv.Atoi(c.DefaultQuery("logs_limit", "15"))
	var orderLogs []models.OrderLog
	q := db.Where("order_id = ?", order.ID).Order("created_at ASC")
	if logsLimit > 0 {
		q = q.Limit(logsLimit)
	}
	q.Find(&orderLogs)

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

	// Damage context (#1707): pending_damage_response / damage_appealing orders
	// need the damage panel data (amount, description, photos) plus a settlement
	// preview (actual rent days/tier rent/refund) so the order detail page can
	// render accept/reject — the notification may have been lost (deadlock).
	var damageData map[string]interface{}
	if order.Status == models.OrderStatusPendingDamageResponse || order.Status == models.OrderStatusDamageAppealing {
		var damageReport models.DamageReport
		reportFound := false
		if err := db.Where("lease_id = ?", order.ID).Order("created_at asc").First(&damageReport).Error; err == nil {
			reportFound = true
		}
		var assessment models.DamageAssessment
		assessmentFound := false
		photos := []string{}
		if err := db.Where("order_id = ?", order.ID).Order("created_at asc").First(&assessment).Error; err == nil {
			assessmentFound = true
			if assessment.Photos != "" {
				json.Unmarshal([]byte(assessment.Photos), &photos)
			}
		}
		// Staff photos live in instrument_media (batch_type=receiving,
		// is_display=false) — the InspectReturn photo upload writes there even
		// when assessment.photos is empty (legacy orders, #1707). Fall back to
		// it so the damage panel always shows what the staff captured.
		if len(photos) == 0 && order.InstrumentID != "" {
			var receivingMedia []models.InstrumentMedia
			db.Where("instrument_id = ? AND batch_type = ? AND file_type = ?",
				order.InstrumentID, "receiving", "image").
				Order("created_at asc").Find(&receivingMedia)
			for _, m := range receivingMedia {
				url := m.StorageKey
				if !strings.HasPrefix(url, "/uploads/") && !strings.HasPrefix(url, "http") {
					url = "/uploads/media/" + url
				}
				photos = append(photos, url)
			}
		}

		// Actual rent days/amount: settlement if present, else derive from the
		// pricing breakdown (actual tier fields) or delivered_at→returned_at.
		actualRentDays := 0
		actualRentAmount := 0.0
		if settlementData != nil {
			if v, ok := settlementData["actual_rent_days"].(int); ok {
				actualRentDays = v
			}
			if v, ok := settlementData["actual_rent_amount"].(float64); ok {
				actualRentAmount = v
			}
		}
		if actualRentDays == 0 && actualRentAmount == 0 {
			if pb, ok := pricingBreakdownData.(map[string]interface{}); ok {
				if v, ok := pb["actual_rent_days"].(float64); ok {
					actualRentDays = int(v)
				}
				if v, ok := pb["actual_rent_amount"].(float64); ok {
					actualRentAmount = v
				}
			}
		}
		if actualRentDays == 0 && actualRentAmount == 0 {
			if order.DeliveredAt != nil && order.ReturnedAt != nil {
				days := services.CalculateDays(*order.DeliveredAt, *order.ReturnedAt)
				if days < 1 {
					days = 1
				}
				actualRentDays = days
				// Daily rate from the pricing breakdown when available.
				if pb, ok := pricingBreakdownData.(map[string]interface{}); ok {
					if v, ok := pb["base_daily_rent"].(float64); ok && v > 0 {
						actualRentAmount = math.Round(v*float64(days)*100) / 100
					}
				}
			}
		}

		// Refund = paid total - damage - actual rent - shipping fee (#1707).
		paidTotal := 0.0
		var paidRecords []models.OrderPaymentRecord
		db.Where("order_id = ? AND status = ? AND type = ?", orderID, "paid", "payment").
			Find(&paidRecords)
		for _, pr := range paidRecords {
			paidTotal += pr.Amount
		}
		damageAmount := 0.0
		description := ""
		status := ""
		reportID := ""
		if reportFound {
			reportID = damageReport.ID
			status = damageReport.Status
			description = damageReport.DamageDescription
			if damageReport.DamageAmount != nil {
				damageAmount = *damageReport.DamageAmount
			}
		} else if assessmentFound {
			// Legacy orders (#1707): the damage_reports table was missing when
			// InspectReturn wrote it, so the data lives in damage_assessments
			// (estimated_cost / description / photos). Derive a pending report
			// view from it so the accept/reject panel still works.
			reportID = assessment.ID
			status = "pending"
			description = assessment.Description
			if assessment.EstimatedCost != nil {
				damageAmount = *assessment.EstimatedCost
			}
		}
		refund := math.Round((paidTotal-damageAmount-actualRentAmount-order.ShippingFee)*100) / 100
		if refund < 0 {
			refund = 0
		}

		damageData = map[string]interface{}{
			"report_id":          reportID,
			"damage_amount":      damageAmount,
			"description":        description,
			"status":             status,
			"photos":             photos,
			"actual_rent_days":   actualRentDays,
			"actual_rent_amount": actualRentAmount,
			"shipping_fee":       order.ShippingFee,
			"deposit":            order.Deposit,
			"paid_total":         paidTotal,
			"refund":             refund,
		}
	}

	// Fetch payment time
	paidAt := ""
	var paymentRecord models.OrderPaymentRecord
	if err := db.Where("order_id = ? AND status = ? AND type = ?", orderID, "paid", "payment").
		First(&paymentRecord).Error; err == nil {
		paidAt = paymentRecord.UpdatedAt.Format("2006-01-02 15:04")
	}

	// Fetch payment records for 收支明细
	type paymentEntry struct {
		ID        string    `json:"id"`
		Amount    float64   `json:"amount"`
		Method    string    `json:"method"`
		Status    string    `json:"status"`
		CreatedAt time.Time `json:"created_at"`
	}
	var paymentEntries []paymentEntry
	var paymentRecords []models.OrderPaymentRecord
	db.Where("order_id = ? AND status = ? AND type = ?", orderID, "paid", "payment").
		Order("created_at ASC").Find(&paymentRecords)
	for _, pr := range paymentRecords {
		method := ""
		if pr.Method != nil {
			method = *pr.Method
		}
		paymentEntries = append(paymentEntries, paymentEntry{
			ID:        pr.ID,
			Amount:    pr.Amount,
			Method:    method,
			Status:    pr.Status,
			CreatedAt: pr.CreatedAt,
		})
	}

	// Fetch refund records from settlements (authoritative refund data)
	type refundEntry struct {
		ID        string             `json:"id"`
		Amount    float64            `json:"amount"`
		Breakdown map[string]float64 `json:"breakdown"`
		Method    string             `json:"method"`
		Status    string             `json:"status"`
		CreatedAt time.Time          `json:"created_at"`
	}
	var refundEntries []refundEntry
	var settlements []models.Settlement
	db.Where("order_id = ?", orderID).Order("created_at DESC").Find(&settlements)
	for _, s := range settlements {
		refundEntries = append(refundEntries, refundEntry{
			ID:     s.ID,
			Amount: s.CashRefundable + s.PrepaidRefunded + s.GiftPointsRefunded,
			Breakdown: map[string]float64{
				"cash":    s.CashRefundable,
				"prepaid": s.PrepaidRefunded,
				"gift":    s.GiftPointsRefunded,
			},
			Method:    s.RefundMethod,
			Status:    s.RefundStatus,
			CreatedAt: s.CreatedAt,
		})
	}

	// Fetch guarantors for deposit-free orders (#1557)
	type guarantorEntry struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Phone   string `json:"phone"`
		Company string `json:"company"`
		Title   string `json:"title"`
		Address string `json:"address"`
	}
	var guarantors []guarantorEntry
	if order.DepositWaived {
		db.Table("order_guarantors og").
			Select("g.id, g.name, g.phone, g.company, g.title, g.address").
			Joins("JOIN guarantors g ON g.id = og.guarantor_id").
			Where("og.order_id = ?", order.ID).
			Order("g.created_at ASC").
			Scan(&guarantors)
	}

	orderData := map[string]interface{}{
		"id":                  order.ID,
		"tenant_id":           order.TenantID,
		"user_id":             order.UserID,
		"user_name":           userName,
		"user_email":          userEmail,
		"user_phone":          userPhone,
		"instrument_id":       order.InstrumentID,
		"instrument_name":     instrumentName,
		"instrument_category": instrumentCategory,
		"instrument_sn":       instrumentSN,
		"level":               order.Level,
		"lease_term":          order.LeaseTerm,
		"deposit_mode":        order.DepositMode,
		"deposit":             order.Deposit,
		"deposit_waived":      order.DepositWaived,
		"guarantors":          guarantors,
		"shipping_fee":        order.ShippingFee,
		"accumulated_months":  order.AccumulatedMonths,
		"status":              order.Status,
		"start_date":          order.StartDate,
		"end_date":            order.EndDate,
		"tracking_number":     order.TrackingNumber,
		"courier_company":     order.CourierCompany,
		"shipped_at":          order.ShippedAt,
		"delivered_at":        order.DeliveredAt,
		"returned_at":         order.ReturnedAt,
		"delivery_address":    deliveryAddress,
		"created_at":          order.CreatedAt,
		"updated_at":          order.UpdatedAt,
		"paid_at":             paidAt,
		"pricing_breakdown":   pricingBreakdownData,
		"settlement":          settlementData,
		"order_logs":          orderLogs,
		"payment_records":     paymentEntries,
		"refund_records":      refundEntries,
		"damage":              damageData,
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
	// Body is optional for weapp (will fill logistics later)
	c.ShouldBindJSON(&req)

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

	// Order timeline (order_logs) — return submission must be visible
	if err := db.Create(&models.OrderLog{
		OrderID:      orderID,
		Event:        fmt.Sprintf("已提交归还（%s，单号 %s）", req.CourierCompany, req.TrackingNumber),
		OperatorID:   stringPtr(middleware.GetUserID(ctx)),
		OperatorName: stringPtr(middleware.GetName(ctx)),
		CreatedAt:    now,
	}).Error; err != nil {
		log.Printf("[ReturnOrder] failed to write order log: %v", err)
	}

	// Save return photos to instrument_media
	if len(req.Photos) > 0 && order.InstrumentID != "" {
		batchID := uuid.New().String()
		for i, photoURL := range req.Photos {
			media := models.InstrumentMedia{
				ID:           uuid.New().String(),
				TenantID:     order.TenantID,
				OrgID:        order.OrgID,
				InstrumentID: &order.InstrumentID,
				BatchID:      batchID,
				BatchType:    "returning",
				FileName:     fmt.Sprintf("return_%d.jpg", i+1),
				FileType:     "image",
				StorageKey:   photoURL,
				IsDisplay:    false,
				SortOrder:    i,
				CreatedAt:    time.Now(),
			}
			if err := db.Create(&media).Error; err != nil {
				log.Printf("[ReturnOrder] Failed to save photo %d: %v", i, err)
			}
		}
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

	// Record audit log
	detailStr := fmt.Sprintf("customer initiated return (courier=%s tracking=%s)", req.CourierCompany, req.TrackingNumber)
	db.Create(&models.AuditLog{
		ID:         uuid.New().String(),
		TenantID:   order.TenantID,
		UserID:     userID,
		Action:     "return_order",
		ResourceID: orderID,
		Details:    &detailStr,
		CreatedAt:  time.Now(),
	})

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
			// Stage-1 settlement preview (#1494): read-only cost details so the
			// user sees updated figures without an immediate refund.
			"settlement_preview": computeSettlement(order, db).Breakdown,
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

// CancelOrderByCustomer allows a customer to cancel their own order.
// Reserved orders: direct cancel + refund points.
// Paid/pending_shipment orders: cancel + create refund session.
// Registered in userOptionalAuth — customer JWT has no tenant/org context.
func CancelOrderByCustomer(c *gin.Context) {
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

	// Verify ownership: resolve local user from IAM sub
	userID := middleware.GetUserID(ctx)
	var localUser models.User
	if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err == nil {
		userID = localUser.ID
	}
	if order.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "not your order"})
		return
	}

	// Only allow cancellation from cancellable states (aligned with frontend
	// cancel button visibility in OrderDetail.jsx). in_transit is excluded —
	// shipped orders must go through return/settlement instead.
	switch order.Status {
	case models.OrderStatusReserved, models.OrderStatusPaid,
		models.OrderStatusPendingShipment:
	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "cannot cancel order in current status"})
		return
	}

	// Remember old status before updating
	oldStatus := order.Status

	// Restore instrument availability
	db.Model(&models.Instrument{}).Where("id = ?", order.InstrumentID).Update("stock_status", models.StockStatusAvailable)

	if err := db.Model(&order).Update("status", models.OrderStatusCancelled).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to cancel order"})
		return
	}

	// Record status history
	history := models.OrderStatusHistory{
		ID:         uuid.New().String(),
		TenantID:   order.TenantID,
		OrderID:    orderID,
		StatusFrom: oldStatus,
		StatusTo:   models.OrderStatusCancelled,
		Notes:      "顾客取消订单",
		ChangedBy:  stringPtr(userID),
		ChangedAt:  time.Now(),
	}
	db.Create(&history)

	// Refund prepaid points if used
	refundOrderPoints(db, &order)

	// For paid/pending_shipment orders, create refund session and return navigation
	if oldStatus == models.OrderStatusPaid || oldStatus == models.OrderStatusPendingShipment {
		refundAmount := order.CashPaid
		refundStatus := ""
		if refundAmount > 0 {
			outRefundNo := fmt.Sprintf("can_%s_%d", orderID[:8], time.Now().Unix())
			refundRecord := models.OrderRefundRecord{
				ID:          uuid.New().String(),
				TenantID:    order.TenantID,
				Amount:      refundAmount,
				OutRefundNo: &outRefundNo,
				Reason:      strPtr("顾客取消订单"),
				Status:      "pending",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}
			if wechatpay.GetConfig().MockMode {
				refundRecord.Status = "refunded"
			} else {
				// Initiate the original-path refund with WeChat Pay
				var paymentRecord models.OrderPaymentRecord
				if err := db.Where("order_id = ? AND order_type = ? AND status = ?", orderID, "rent", "paid").
					First(&paymentRecord).Error; err == nil && paymentRecord.OutTradeNo != nil {
					cfg := wechatpay.GetConfig()
					client := wechatpay.GetClient()
					_, refundErr := client.Refund(context.Background(), wechatpay.RefundParams{
						OutTradeNo:   *paymentRecord.OutTradeNo,
						OutRefundNo:  outRefundNo,
						TotalAmount:  cfg.AmountToCents(paymentRecord.Amount),
						RefundAmount: cfg.AmountToCents(refundAmount),
						Reason:       "顾客取消订单",
						NotifyURL:    cfg.RefundNotifyURL,
					})
					if refundErr != nil {
						refundRecord.Status = "failed"
						fr := refundErr.Error()
						refundRecord.FailReason = &fr
						log.Printf("[CancelOrderByCustomer] refund failed for order %s: %v", orderID, refundErr)
					}
				}
			}
			db.Create(&refundRecord)
			refundStatus = refundRecord.Status
		}
		c.JSON(http.StatusOK, gin.H{
			"code": 20000,
			"data": gin.H{
				"order_id":      orderID,
				"old_status":    oldStatus,
				"new_status":    models.OrderStatusCancelled,
				"refund_amount": refundAmount,
				"refund_status": refundStatus,
				"breakdown": gin.H{
					"total_paid":     order.CashPaid + order.PrepaidPointsUsed + order.GiftPointsUsed,
					"cash_paid":      order.CashPaid,
					"prepaid_used":   order.PrepaidPointsUsed,
					"gift_used":      order.GiftPointsUsed,
					"total_refund":   order.CashPaid + order.PrepaidPointsUsed + order.GiftPointsUsed,
					"cash_refund":    order.CashPaid,
					"prepaid_refund": order.PrepaidPointsUsed,
					"gift_refund":    order.GiftPointsUsed,
				},
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"order_id":   orderID,
			"old_status": models.OrderStatusReserved,
			"new_status": models.OrderStatusCancelled,
		},
	})
}

// StaffCancelOrder POST /api/warehouse/orders/:id/staff-cancel
// Site staff cancels a paid/pending_shipment order (e.g. deposit-free order
// whose guarantors fail verification, #1557). Refunds the original payment
// path like the customer cancellation flow.
func StaffCancelOrder(c *gin.Context) {
	orderID := c.Param("id")
	if orderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "order_id is required"})
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var order models.Order
	if err := db.Where("id = ?", orderID).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "order not found"})
		return
	}

	if order.Status != models.OrderStatusPaid && order.Status != models.OrderStatusPendingShipment {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "order can only be cancelled by staff when status is paid or pending_shipment"})
		return
	}

	oldStatus := order.Status

	// Restore instrument availability
	db.Model(&models.Instrument{}).Where("id = ?", order.InstrumentID).Update("stock_status", models.StockStatusAvailable)

	if err := db.Model(&order).Update("status", models.OrderStatusCancelled).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to cancel order"})
		return
	}

	// Resolve operator identity (IAM sub → local user name)
	operatorID := middleware.GetUserID(ctx)
	operatorName := ""
	var localUser models.User
	if err := db.Where("iam_sub = ?", operatorID).First(&localUser).Error; err == nil && localUser.Name != "" {
		operatorName = localUser.Name
	}

	// Record status history
	history := models.OrderStatusHistory{
		ID:         uuid.New().String(),
		TenantID:   order.TenantID,
		OrderID:    orderID,
		StatusFrom: oldStatus,
		StatusTo:   models.OrderStatusCancelled,
		Notes:      "员工取消订单",
		ChangedBy:  stringPtr(operatorID),
		ChangedAt:  time.Now(),
	}
	db.Create(&history)

	// Record order log with reason
	logEvent := "员工取消订单"
	if req.Reason != "" {
		logEvent = fmt.Sprintf("员工取消订单: %s", req.Reason)
	}
	db.Create(&models.OrderLog{
		OrderID:      orderID,
		Event:        logEvent,
		OperatorID:   stringPtr(operatorID),
		OperatorName: stringPtr(operatorName),
		CreatedAt:    time.Now(),
	})

	// Refund prepaid points if used
	refundOrderPoints(db, &order)

	// Initiate original-path refund with WeChat Pay
	refundAmount := order.CashPaid
	refundStatus := ""
	if refundAmount > 0 {
		outRefundNo := fmt.Sprintf("scn_%s_%d", orderID[:8], time.Now().Unix())
		refundRecord := models.OrderRefundRecord{
			ID:          uuid.New().String(),
			TenantID:    order.TenantID,
			Amount:      refundAmount,
			OutRefundNo: &outRefundNo,
			Reason:      strPtr("员工取消订单"),
			Status:      "pending",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if wechatpay.GetConfig().MockMode {
			refundRecord.Status = "refunded"
		} else {
			var paymentRecord models.OrderPaymentRecord
			if err := db.Where("order_id = ? AND order_type = ? AND status = ?", orderID, "rent", "paid").
				First(&paymentRecord).Error; err == nil && paymentRecord.OutTradeNo != nil {
				cfg := wechatpay.GetConfig()
				client := wechatpay.GetClient()
				_, refundErr := client.Refund(context.Background(), wechatpay.RefundParams{
					OutTradeNo:   *paymentRecord.OutTradeNo,
					OutRefundNo:  outRefundNo,
					TotalAmount:  cfg.AmountToCents(paymentRecord.Amount),
					RefundAmount: cfg.AmountToCents(refundAmount),
					Reason:       "员工取消订单",
					NotifyURL:    cfg.RefundNotifyURL,
				})
				if refundErr != nil {
					refundRecord.Status = "failed"
					fr := refundErr.Error()
					refundRecord.FailReason = &fr
					log.Printf("[StaffCancelOrder] refund failed for order %s: %v", orderID, refundErr)
				}
			}
		}
		db.Create(&refundRecord)
		refundStatus = refundRecord.Status
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"order_id":      orderID,
			"old_status":    oldStatus,
			"new_status":    models.OrderStatusCancelled,
			"refund_amount": refundAmount,
			"refund_status": refundStatus,
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
			"order_id":      order.ID,
			"order_status":  order.Status,
			"instrument_id": instrument.ID,
			"instrument_sn": sn,
			"start_date":    order.StartDate,
			"end_date":      order.EndDate,
			"deposit":       order.Deposit,
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
	// 匿名（无 tenant 且无 userID）不得读取订单日志（#1694 泄漏修复）。
	if tenantID == "" && userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "请先登录"})
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

	// Resolve customer name for log display (fallback: "顾客")
	customerName := "顾客"
	var customer models.User
	if order.UserID != "" {
		if err := db.Where("id = ?", order.UserID).First(&customer).Error; err == nil && customer.Name != "" {
			customerName = customer.Name
		}
	}

	// resolveOperatorName resolves a name for a ChangedBy reference (local user id or IAM sub).
	// Priority: local by id → local by iam_sub → IAM fetch (+ cache update) → fallback.
	resolveOperatorName := func(changedBy string) string {
		// 1. Local by id (ChangedBy stores local user id in most flows)
		var operator models.User
		if err := db.Where("id = ?", changedBy).First(&operator).Error; err == nil && operator.ID != "" {
			if operator.Name != "" {
				return operator.Name
			}
			// found locally but name empty → try IAM via iam_sub
			if operator.IAMSub != "" {
				iamClient := services.NewIAMClient()
				if iamUser, iamErr := iamClient.GetUser(operator.IAMSub); iamErr == nil && iamUser.Name != "" {
					db.Model(&models.User{}).Where("id = ?", operator.ID).
						Update("name", iamUser.Name)
					return iamUser.Name
				}
			}
			return "顾客"
		}
		// 2. Local by iam_sub (some flows store IAM sub)
		if err := db.Where("iam_sub = ?", changedBy).First(&operator).Error; err == nil && operator.ID != "" {
			if operator.Name != "" {
				return operator.Name
			}
			// found locally but name empty → try IAM
			iamClient := services.NewIAMClient()
			if iamUser, iamErr := iamClient.GetUser(changedBy); iamErr == nil && iamUser.Name != "" {
				db.Model(&models.User{}).Where("id = ?", operator.ID).
					Update("name", iamUser.Name)
				return iamUser.Name
			}
			return "顾客"
		}
		// 3. Fall back to order owner (customer) name when changedBy matches the owner
		if order.UserID != "" && (changedBy == order.UserID || changedBy == customer.IAMSub) {
			return customerName
		}
		// 4. Not found locally → try IAM directly with changedBy as IAM sub
		iamClient := services.NewIAMClient()
		if iamUser, iamErr := iamClient.GetUser(changedBy); iamErr == nil && iamUser.Name != "" {
			return iamUser.Name
		}
		return "顾客"
	}

	// 1. Order created
	logs = append(logs, logEntry{
		Event:     "created",
		Time:      order.CreatedAt,
		Operator:  customerName,
		CreatedAt: order.CreatedAt,
	})

	// Explicit order logs (renewal, overdue, etc.) — loaded before the history
	// pass so the dedup logic (#1701) can compare against them.
	var orderLogs []models.OrderLog
	db.Where("order_id = ?", orderID).Order("created_at DESC").Find(&orderLogs)

	// 2. Status transitions from order_status_history
	// Dedup (#1701): skip a history entry when an explicit order_log already
	// describes the same transition (order_logs carries richer copy like
	// "已发货（顺丰，单号 xx）"). Keep order_logs; drop the bare history row.
	statusKeyword := map[string][]string{
		"shipped":     {"已发货"},
		"in_lease":    {"已签收", "租赁开始"},
		"returning":   {"归还"},
		"returned":    {"已归还"},
		"completed":   {"完成"},
		"cancelled":   {"取消"},
		"expired":     {"超期", "逾期"},
		"pending_shipment": {"发货"},
	}
	var history []models.OrderStatusHistory
	db.Where("order_id = ?", orderID).Order("changed_at ASC").Find(&history)
	for _, h := range history {
		if keywords, ok := statusKeyword[h.StatusTo]; ok {
			duplicated := false
			for _, ol := range orderLogs {
				for _, kw := range keywords {
					if strings.Contains(ol.Event, kw) {
						duplicated = true
						break
					}
				}
				if duplicated {
					break
				}
			}
			if duplicated {
				continue
			}
		}
		op := "顾客"
		if h.ChangedBy != nil {
			op = resolveOperatorName(*h.ChangedBy)
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

	// 4. Explicit order logs (renewal, overdue, etc.) — orderLogs loaded above
	// (before the history pass) for dedup; reuse it here.
	for _, ol := range orderLogs {
		op := "system"
		if ol.OperatorID != nil && *ol.OperatorID != "" {
			op = resolveOperatorName(*ol.OperatorID)
		}
		logs = append(logs, logEntry{
			Event:     ol.Event,
			Time:      ol.CreatedAt,
			Operator:  op,
			CreatedAt: ol.CreatedAt,
		})
	}

	// Sort by time descending (newest first)
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].Time.After(logs[j].Time)
	})

	total := len(logs)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "15"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 15
	}
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	paginated := logs[start:end]

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"logs":     paginated,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		},
	})
}

// AdminUpdateOrder allows merchant/site admins (in DEBUG_MODE) to override order dates and status.
func AdminUpdateOrder(c *gin.Context) {
	orderID := c.Param("id")
	if orderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "order id required"})
		return
	}

	var req struct {
		StartDate   string `json:"start_date"`
		EndDate     string `json:"end_date"`
		Status      string `json:"status"`
		DeliveredAt string `json:"delivered_at"`
		ReturnedAt  string `json:"returned_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request"})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	updates := map[string]interface{}{}
	if req.Status != "" {
		updates["status"] = req.Status
	}
	if req.StartDate != "" {
		updates["start_date"] = req.StartDate
	}
	if req.EndDate != "" {
		updates["end_date"] = req.EndDate
	}
	if req.DeliveredAt != "" {
		updates["delivered_at"] = req.DeliveredAt
	} else if c.Query("clear_delivered") == "1" {
		updates["delivered_at"] = gorm.Expr("NULL")
	}
	if req.ReturnedAt != "" {
		updates["returned_at"] = req.ReturnedAt
	} else if c.Query("clear_returned") == "1" {
		updates["returned_at"] = gorm.Expr("NULL")
	}
	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "no fields to update"})
		return
	}

	if err := db.Model(&models.Order{}).Where("id = ?", orderID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "update failed: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "updated"})
}

// refundOrderPoints returns prepaid and gift points used in an order to the user's wallet.
func refundOrderPoints(db *gorm.DB, order *models.Order) {
	if order.PrepaidPointsUsed > 0 {
		db.Model(&models.User{}).Where("id = ?", order.UserID).
			Update("prepaid_points", gorm.Expr("prepaid_points + ?", order.PrepaidPointsUsed))
	}
	if order.GiftPointsUsed > 0 {
		db.Model(&models.User{}).Where("id = ?", order.UserID).
			Update("promo_points", gorm.Expr("promo_points + ?", order.GiftPointsUsed))
	}
}

func GetOrdersByOutTradeNo(c *gin.Context) {
	ctx := c.Request.Context()
	outTradeNo := c.Param("out_trade_no")

	if outTradeNo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "out_trade_no is required"})
		return
	}

	db := database.GetDB().WithContext(ctx)

	var session models.PaymentSession
	if err := db.Where("out_trade_no = ?", outTradeNo).First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "payment session not found"})
		return
	}

	var links []models.SessionOrderLink
	if err := db.Where("session_id = ?", session.ID).Find(&links).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query order links"})
		return
	}

	if len(links) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 20000, "data": struct {
			Orders []models.Order `json:"orders"`
		}{Orders: []models.Order{}}})
		return
	}

	orderIDs := make([]string, 0, len(links))
	for _, link := range links {
		orderIDs = append(orderIDs, link.OrderID)
	}

	var orders []models.Order
	if err := db.Where("id IN ?", orderIDs).Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query orders"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
		"orders": orders,
	}})
}
