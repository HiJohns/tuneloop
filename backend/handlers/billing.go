package handlers

import (
	"encoding/csv"
	"log"
	"math"
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
		Where("status NOT IN ?", []string{"reserved", "cancelled"})

	if startStr != "" {
		if startTime, err := time.Parse("2006-01-02", startStr); err == nil {
			baseQuery = baseQuery.Where("created_at >= ?", startTime)
		}
	}
	if endStr != "" {
		if endTime, err := time.Parse("2006-01-02", endStr); err == nil {
			baseQuery = baseQuery.Where("created_at < ?", endTime.Add(24*time.Hour))
		}
	}

	// Permission scoping
	if businessRole == middleware.BusinessRoleMerchantAdmin {
		baseQuery = baseQuery.Where("tenant_id = ?", tenantID)
	} else if businessRole != middleware.BusinessRoleSystemAdmin {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}

	// Count total
	var total int64
	baseQuery.Count(&total)

	// Compute summary
	var summary struct {
		TotalCashPaid float64 `gorm:"column:total_cash"`
		TotalPrepaid  float64 `gorm:"column:total_prepaid"`
		TotalGift     float64 `gorm:"column:total_gift"`
	}
	baseQuery.Select("COALESCE(SUM(cash_paid),0) as total_cash, COALESCE(SUM(prepaid_points_used),0) as total_prepaid, COALESCE(SUM(gift_points_used),0) as total_gift").
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
	var totalRefund float64
	refundQuery.Select("COALESCE(SUM(rr.amount),0)").Scan(&totalRefund)

	// Fetch order list
	type OrderRow struct {
		OrderID        string  `json:"order_id"`
		CreatedAt      string  `json:"created_at"`
		InstrumentName string `json:"instrument_name"`
		UserName       string  `json:"user_name"`
		CashPaid       float64 `json:"cash_paid"`
		PrepaidUsed    float64 `json:"prepaid_used"`
		GiftUsed       float64 `json:"gift_used"`
		RefundAmount   float64 `json:"refund_amount"`
		Deposit        float64 `json:"deposit"`
		Status         string  `json:"status"`
	}

	var rows []OrderRow
	offset := (page - 1) * pageSize
	isCSV := c.Query("format") == "csv"

	listQuery := baseQuery.Select(`
		orders.id as order_id,
		orders.created_at,
		COALESCE(instruments.sn, instruments.name, '') as instrument_name,
		COALESCE(users.name, '') as user_name,
		orders.cash_paid,
		orders.prepaid_points_used as prepaid_used,
		orders.gift_points_used as gift_used,
		orders.deposit,
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
		writer.Write([]string{"订单号", "时间", "用户", "乐器", "实付", "预付点抵扣", "赠点抵扣", "押金", "状态"})
		for _, r := range rows {
			writer.Write([]string{
				r.OrderID,
				r.CreatedAt[:10],
				r.UserName,
				r.InstrumentName,
				strconv.FormatFloat(r.CashPaid, 'f', 2, 64),
				strconv.FormatFloat(r.PrepaidUsed, 'f', 2, 64),
				strconv.FormatFloat(r.GiftUsed, 'f', 2, 64),
				strconv.FormatFloat(r.Deposit, 'f', 2, 64),
				r.Status,
			})
		}
		writer.Flush()
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"summary": gin.H{
				"total_orders":      total,
				"total_cash_paid":   math.Round(summary.TotalCashPaid*100) / 100,
				"total_prepaid_used": math.Round(summary.TotalPrepaid*100) / 100,
				"total_gift_used":   math.Round(summary.TotalGift*100) / 100,
				"total_refund":      math.Round(totalRefund*100) / 100,
			},
			"list":      rows,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
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
