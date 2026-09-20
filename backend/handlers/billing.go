package handlers

import (
	"encoding/csv"
	"log"
	"net/http"
	"strconv"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetBillingReport returns order billing report for merchant admin / platform admin.
// GET /api/admin/billing/report
func GetBillingReport(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	tenantID := middleware.GetTenantID(ctx)
	businessRole := middleware.GetBusinessRole(ctx)

	// Parse query params
	startStr := c.Query("start")
	endStr := c.Query("end")
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "20")

	page, _ := strconv.Atoi(pageStr)
	pageSize, _ := strconv.Atoi(pageSizeStr)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Build base query
	baseQuery := db.Model(&struct{}{}).Table("orders").
		Where("orders.status NOT IN ?", []string{"reserved", "cancelled"})

	if startStr != "" {
		if startTime, err := time.Parse("2006-01-02", startStr); err == nil {
			baseQuery = baseQuery.Where("orders.created_at >= ?", startTime)
		}
	}
	if endStr != "" {
		if endTime, err := time.Parse("2006-01-02", endStr); err == nil {
			baseQuery = baseQuery.Where("orders.created_at < ?", endTime.Add(24*time.Hour))
		}
	}

	// Permission scoping
	if businessRole == middleware.BusinessRoleMerchantAdmin {
		baseQuery = baseQuery.Where("orders.tenant_id = ?", tenantID)
	} else if businessRole != middleware.BusinessRoleSystemAdmin {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}

	// Count total
	var total int64
	baseQuery.Count(&total)

	// Compute summary
	var summary struct {
		TotalCashPaid models.Cents `gorm:"column:total_cash"`
		TotalPrepaid  models.Cents `gorm:"column:total_prepaid"`
		TotalGift     models.Cents `gorm:"column:total_gift"`
	}
	// ::bigint 必须保留：Postgres SUM(bigint) 返回 numeric，驱动以文本回传，
	// 会被 models.Cents.Scan 当作「元」再 ×100（#1999 S1 分→元修复）。
	baseQuery.Select("COALESCE(SUM(orders.cash_paid),0)::bigint as total_cash, COALESCE(SUM(orders.prepaid_points_used),0)::bigint as total_prepaid, COALESCE(SUM(orders.gift_points_used),0)::bigint as total_gift").
		Scan(&summary)

	// Compute refund total
	refundQuery := db.Model(&struct{}{}).Table("order_refund_records rr").
		Joins("JOIN order_payment_records pr ON pr.id = rr.payment_record_id").
		Where("rr.status = ?", "refunded")
	if startStr != "" {
		if startTime, err := time.Parse("2006-01-02", startStr); err == nil {
			refundQuery = refundQuery.Where("rr.created_at >= ?", startTime)
		}
	}
	if endStr != "" {
		if endTime, err := time.Parse("2006-01-02", endStr); err == nil {
			refundQuery = refundQuery.Where("rr.created_at < ?", endTime.Add(24*time.Hour))
		}
	}
	if businessRole == middleware.BusinessRoleMerchantAdmin {
		refundQuery = refundQuery.Where("pr.tenant_id = ?", tenantID)
	}
	var totalRefund models.Cents
	refundQuery.Select("COALESCE(SUM(rr.amount),0)::bigint").Scan(&totalRefund)

	// Fetch order list. 金额字段一律以 models.Cents（分）扫描，输出时经 Cents.ToYuan() 转元
	// （#1999 S1 / #1757：DB 存分，导出/展示为元，两位小数）。
	type OrderRow struct {
		OrderID         string
		OrderNo         string
		OutTradeNo      *string
		CreatedAt       string
		InstrumentName  string
		UserName        string
		CashPaid        models.Cents
		PrepaidUsed     models.Cents
		GiftUsed        models.Cents
		Deposit         models.Cents
		ShippingFee     models.Cents
		RenewalAmount   models.Cents
		OverdueAmount   models.Cents
		RefundAmount    models.Cents
		DepositRefunded bool
		Status          string
	}

	var rows []OrderRow
	offset := (page - 1) * pageSize
	isCSV := c.Query("format") == "csv"

	listQuery := baseQuery.Select(`
		orders.id as order_id,
		orders.order_no,
		(SELECT pr.out_trade_no FROM order_payment_records pr
			WHERE pr.order_id = orders.id AND pr.type = 'payment'
			ORDER BY pr.created_at DESC LIMIT 1) as out_trade_no,
		orders.created_at,
		COALESCE(instruments.sn, '') as instrument_name,
		COALESCE(users.name, '') as user_name,
		orders.cash_paid,
		orders.prepaid_points_used as prepaid_used,
		orders.gift_points_used as gift_used,
		orders.deposit,
		orders.shipping_fee,
		(SELECT COALESCE(SUM(pr.amount),0)::bigint FROM order_payment_records pr
			WHERE pr.order_id = orders.id AND pr.order_type = 'renewal'
				AND pr.type = 'payment' AND pr.status = 'paid') as renewal_amount,
		(SELECT COALESCE(SUM(dr.overdue_fee),0)::bigint FROM damage_reports dr
			WHERE dr.lease_id = orders.id) as overdue_amount,
		(SELECT COALESCE(SUM(rr.amount),0)::bigint FROM order_refund_records rr
			JOIN order_payment_records pr2 ON pr2.id = rr.payment_record_id
			WHERE pr2.order_id = orders.id AND rr.status = 'refunded') as refund_amount,
		orders.deposit_refunded,
		orders.status`).
		Joins("LEFT JOIN instruments ON instruments.id = orders.instrument_id").
		Joins("LEFT JOIN users ON users.id = orders.user_id").
		Order("orders.created_at DESC")

	if !isCSV {
		listQuery = listQuery.Offset(offset).Limit(pageSize)
	}

	if err := listQuery.Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query billing: " + err.Error()})
		return
	}

	// CSV export
	if c.Query("format") == "csv" {
		c.Header("Content-Type", "text/csv")
		c.Header("Content-Disposition", "attachment; filename=billing_report.csv")
		writer := csv.NewWriter(c.Writer)
		writer.Write([]string{
			"订单号", "订单ID", "微信支付单号", "时间", "用户", "乐器",
			"实付", "预付点抵扣", "赠点抵扣", "押金", "物流费", "续费", "逾期费", "退款", "押金已退", "状态",
		})
		yuan := func(c models.Cents) string {
			return strconv.FormatFloat(c.ToYuan(), 'f', 2, 64)
		}
		for _, r := range rows {
			orderNo := r.OrderNo
			if orderNo == "" {
				orderNo = r.OrderID
			}
			outTradeNo := ""
			if r.OutTradeNo != nil {
				outTradeNo = *r.OutTradeNo
			}
			writer.Write([]string{
				orderNo,
				r.OrderID,
				outTradeNo,
				firstN(r.CreatedAt, 10),
				r.UserName,
				r.InstrumentName,
				yuan(r.CashPaid),
				yuan(r.PrepaidUsed),
				yuan(r.GiftUsed),
				yuan(r.Deposit),
				yuan(r.ShippingFee),
				yuan(r.RenewalAmount),
				yuan(r.OverdueAmount),
				yuan(r.RefundAmount),
				strconv.FormatBool(r.DepositRefunded),
				r.Status,
			})
		}
		writer.Flush()
		return
	}

	jsonRows := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		outTradeNo := ""
		if r.OutTradeNo != nil {
			outTradeNo = *r.OutTradeNo
		}
		jsonRows = append(jsonRows, gin.H{
			"order_id":         r.OrderID,
			"order_no":         r.OrderNo,
			"out_trade_no":     outTradeNo,
			"created_at":       r.CreatedAt,
			"instrument_name":  r.InstrumentName,
			"user_name":        r.UserName,
			"cash_paid":        r.CashPaid.ToYuan(),
			"prepaid_used":     r.PrepaidUsed.ToYuan(),
			"gift_used":        r.GiftUsed.ToYuan(),
			"deposit":          r.Deposit.ToYuan(),
			"shipping_fee":     r.ShippingFee.ToYuan(),
			"renewal_amount":   r.RenewalAmount.ToYuan(),
			"overdue_amount":   r.OverdueAmount.ToYuan(),
			"refund_amount":    r.RefundAmount.ToYuan(),
			"deposit_refunded": r.DepositRefunded,
			"status":           r.Status,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"summary": gin.H{
				"total_orders":       total,
				"total_cash_paid":    summary.TotalCashPaid.ToYuan(),
				"total_prepaid_used": summary.TotalPrepaid.ToYuan(),
				"total_gift_used":    summary.TotalGift.ToYuan(),
				"total_refund":       totalRefund.ToYuan(),
			},
			"list":      jsonRows,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// firstN 返回 s 的前 n 个字符；s 短于 n 时原样返回（避免切片越界 panic）。
func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// GetSettlementConfig returns the settlement config for a merchant.
// GET /api/admin/merchant/:id/settlement
func GetSettlementConfig(c *gin.Context) {
	merchantID := c.Param("id")
	if merchantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "merchant id required"})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())

	// Resolve tenant_id from merchant record
	var merchant models.Merchant
	if err := db.Where("id = ?", merchantID).First(&merchant).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "merchant not found"})
		return
	}

	var cfg models.MerchantSettlementConfig
	if err := db.Where("tenant_id = ?", merchant.TenantID).First(&cfg).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 20000, "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": cfg})
}

// UpsertSettlementConfig creates or updates a merchant's settlement config.
// PUT /api/admin/merchant/:id/settlement
func UpsertSettlementConfig(c *gin.Context) {
	merchantID := c.Param("id")
	if merchantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "merchant id required"})
		return
	}

	var req struct {
		ReceiverType     string  `json:"receiver_type"`
		ReceiverAccount  string  `json:"receiver_account"`
		ProfitShareRatio float64 `json:"profit_share_ratio"`
		IsEnabled        *bool   `json:"is_enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request: " + err.Error()})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	// Resolve tenant_id from merchant record
	var merchant models.Merchant
	if err := db.Where("id = ?", merchantID).First(&merchant).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "merchant not found"})
		return
	}
	tenantID := merchant.TenantID

	var existing models.MerchantSettlementConfig
	found := db.Where("tenant_id = ?", tenantID).First(&existing).Error == nil

	cfg := models.MerchantSettlementConfig{
		TenantID:         tenantID,
		ReceiverType:     req.ReceiverType,
		ReceiverAccount:  req.ReceiverAccount,
		ProfitShareRatio: req.ProfitShareRatio,
		IsEnabled:        true,
		UpdatedAt:        time.Now(),
	}
	if req.IsEnabled != nil {
		cfg.IsEnabled = *req.IsEnabled
	}

	if found {
		cfg.ID = existing.ID
		cfg.CreatedAt = existing.CreatedAt
		if err := db.Save(&cfg).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update settlement: " + err.Error()})
			return
		}
	} else {
		cfg.ID = uuid.New().String()
		cfg.CreatedAt = time.Now()
		if err := db.Create(&cfg).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create settlement: " + err.Error()})
			return
		}
	}

	log.Printf("[SettlementConfig] %s merchant=%s receiver=%s/%s ratio=%.2f enabled=%v",
		map[bool]string{true: "updated", false: "created"}[found],
		merchantID, req.ReceiverType, req.ReceiverAccount, req.ProfitShareRatio, cfg.IsEnabled)

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": cfg})
}
