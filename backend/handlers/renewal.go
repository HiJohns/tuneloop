package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
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

type RenewalCalculateRequest struct {
	AdditionalDays int `json:"additional_days" binding:"required,min=1"`
}

type RenewalCalculateResponse struct {
	RenewalCost       float64                `json:"renewal_cost"`
	OverdueBalance    float64                `json:"overdue_balance"`
	TotalAmount       float64                `json:"total_amount"`
	NewEndDate        string                 `json:"new_end_date"`
	MinAdditionalDays int                    `json:"min_additional_days"`
	TierBreakdown     []services.TierSegment `json:"tier_breakdown"`
	DailyRate         float64                `json:"daily_rate"`
	OverdueDailyRate  float64                `json:"overdue_daily_rate"`
	OverdueDays       int                    `json:"overdue_days"`
}

type RenewalConfirmRequest struct {
	AdditionalDays int    `json:"additional_days" binding:"required,min=1"`
	OpenID         string `json:"open_id,omitempty"`
	CouponCode     string `json:"coupon_code,omitempty"` // #1744: 每次支付手动输入，可不同可不用
}

type RenewalConfirmResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    *PrepayData `json:"data,omitempty"`
}

type renewalMetadata struct {
	AdditionalDays int    `json:"additional_days"`
	OrderID        string `json:"order_id"`
	OutTradeNo     string `json:"out_trade_no"`
}

func loadOrderForRenewal(db *gorm.DB, orderID, userID string) (*models.Order, error) {
	var localUser models.User
	if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err == nil {
		userID = localUser.ID
	}
	var order models.Order
	if err := db.Where("id = ? AND user_id = ?", orderID, userID).First(&order).Error; err != nil {
		return nil, err
	}
	// GORM can't map PostgreSQL DATE → *string, read dates via raw query
	var sd, ed *string
	db.Raw("SELECT start_date::text, end_date::text FROM orders WHERE id = ?", orderID).Row().Scan(&sd, &ed)
	order.StartDate = sd
	order.EndDate = ed
	if order.Status != models.OrderStatusInLease && order.Status != models.OrderStatusExpired {
		return nil, fmt.Errorf("order can only be renewed when status is in_lease or expired")
	}
	return &order, nil
}

func parseDatePtr(s *string) time.Time {
	if s == nil || *s == "" {
		return time.Now().Truncate(24 * time.Hour)
	}
	for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, *s); err == nil {
			return t
		}
	}
	return time.Now().Truncate(24 * time.Hour)
}

func loadRenewalPricing(db *gorm.DB, order *models.Order) (baseRate float64, pricingTiers []services.PricingTierConfig, cumulativeDiscount float64) {
	var pb services.PricingBreakdown
	if order.PricingBreakdown != nil && *order.PricingBreakdown != "" {
		json.Unmarshal([]byte(*order.PricingBreakdown), &pb)
	}
	// #1734: 快照 base_daily_rent 存量为元语义（迁移遗漏），P3 后为分——
	// 统一走 helper 归一为分。
	baseRate = resolveBaseDailyRentCents(db, order, pb.BaseDailyRent)
	if baseRate <= 0 && order.MonthlyRent > 0 {
		// #1761: monthly rent is Cents (分) — daily = monthly/30, cents.
		// Previously this returned yuan (ToYuan()/30), breaking the cent
		// contract and showing ¥0.50/day for a ¥0.01/day instrument.
		baseRate = float64(order.MonthlyRent) / 30
	}
	if baseRate <= 0 {
		// #1761: final fallback — the instrument's own base_daily_rate
		// (cents). Previously a hardcoded 50 (¥0.50) masked real pricing.
		var inst models.Instrument
		if err := db.Select("base_daily_rate").Where("id = ?", order.InstrumentID).First(&inst).Error; err == nil && inst.BaseDailyRate != nil && *inst.BaseDailyRate > 0 {
			baseRate = float64(*inst.BaseDailyRate)
		}
	}
	if baseRate <= 0 {
		// #1781: snapshot base_daily_rent may be 0 (instrument base_daily_rate
		// unconfigured; daily rate lives in pricing JSON, e.g. CV-08 ¥0.01/day).
		// Recover from snapshot pricing_tiers[0].daily_rate (cents, P3-migrated).
		if len(pb.PricingTiers) > 0 && pb.PricingTiers[0].DailyRate > 0 {
			baseRate = pb.PricingTiers[0].DailyRate
		}
	}
	disc := 1.0
	for _, p := range pb.AppliedPolicies {
		if p.Type == "membership_discount" || p.Type == "promo_campaign" {
			disc *= p.Rate
		}
	}
	return baseRate, pb.PricingTiers, disc
}

func CalculateRenewal(c *gin.Context) {
	orderID := c.Param("id")
	var req RenewalCalculateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)

	order, err := loadOrderForRenewal(db, orderID, userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}

	startDate := parseDatePtr(order.StartDate)
	endDate := parseDatePtr(order.EndDate)
	today := time.Now().Truncate(24 * time.Hour)

	// consumedDays = original lease term (not up to today)
	// #1761/#1762: lease_term may be stale when end_date was miscomputed at
	// order creation — prefer pricing_breakdown.rent_days (same covered-days
	// semantics as settlement C, #1743), falling back to lease_term.
	leaseTerm := order.LeaseTerm
	if leaseTerm <= 0 {
		leaseTerm = services.CalculateDays(startDate, endDate)
	}
	consumedDays := leaseTerm
	if order.PricingBreakdown != nil && *order.PricingBreakdown != "" {
		var pb struct {
			RentDays int `json:"rent_days"`
		}
		if json.Unmarshal([]byte(*order.PricingBreakdown), &pb) == nil && pb.RentDays > 0 {
			consumedDays = pb.RentDays
		}
	}
	if consumedDays < 0 {
		consumedDays = 0
	}

	// Overdue days: endDate to yesterday (today not counted toward overdue)
	var overdueDays int
	if today.After(endDate) {
		yesterday := today.AddDate(0, 0, -1)
		overdueDays = services.CalculateDays(endDate, yesterday)
		if overdueDays < 0 {
			overdueDays = 0
		}
	}

	// Minimum renewal days: when overdue, renewal must cover at least the
	// overdue period so the new end date stays after today (continuous).
	minAdditionalDays := 0
	if today.After(endDate) {
		minAdditionalDays = services.CalculateDays(endDate, today)
		if minAdditionalDays < 0 {
			minAdditionalDays = 0
		}
	}
	if req.AdditionalDays < minAdditionalDays {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": fmt.Sprintf("续期天数至少为 %d 天（需覆盖逾期期）", minAdditionalDays),
		})
		return
	}

	baseDate := endDate
	newEndDate := baseDate.AddDate(0, 0, req.AdditionalDays)
	baseRate, pricingTiers, cumDisc := loadRenewalPricing(db, order)

	renewalCost, tierBreakdown := services.CalculateRenewalPricing(
		baseRate, pricingTiers, consumedDays, req.AdditionalDays, cumDisc,
	)

	var overdueBalance float64

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": RenewalCalculateResponse{
			RenewalCost:       renewalCost,
			OverdueBalance:    overdueBalance,
			TotalAmount:       renewalCost + overdueBalance,
			NewEndDate:        newEndDate.Format("2006-01-02"),
			MinAdditionalDays: minAdditionalDays,
			TierBreakdown:     tierBreakdown,
			DailyRate:         baseRate,
			OverdueDailyRate:  0,
			OverdueDays:       overdueDays,
		},
	})
}

func ConfirmRenewal(c *gin.Context) {
	orderID := c.Param("id")
	var req RenewalConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	userID := middleware.GetUserID(ctx)
	db := database.GetDB().WithContext(ctx)

	// For customer (USER) JWT, tenantID is empty — derive from the order
	if tenantID == "" {
		var ord struct{ TenantID string }
		if err := db.Table("orders").Select("tenant_id").Where("id = ?", orderID).Scan(&ord).Error; err == nil {
			tenantID = ord.TenantID
		}
	}

	order, err := loadOrderForRenewal(db, orderID, userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}

	startDate := parseDatePtr(order.StartDate)
	endDate := parseDatePtr(order.EndDate)
	today := time.Now().Truncate(24 * time.Hour)
	leaseTerm := order.LeaseTerm
	if leaseTerm <= 0 {
		leaseTerm = services.CalculateDays(startDate, endDate)
	}
	consumedDays := leaseTerm
	if consumedDays < 0 {
		consumedDays = 0
	}
	// #1781: same as CalculateRenewal — extract rent_days from the pricing
	// breakdown snapshot so consumedDays reflects the actual covered period,
	// not the potentially stale lease_term.
	if order.PricingBreakdown != nil && *order.PricingBreakdown != "" {
		var pbConsumed struct {
			RentDays int `json:"rent_days"`
		}
		if json.Unmarshal([]byte(*order.PricingBreakdown), &pbConsumed) == nil && pbConsumed.RentDays > 0 {
			consumedDays = pbConsumed.RentDays
		}
	}
	baseRate, pricingTiers, cumDisc := loadRenewalPricing(db, order)

	// Minimum renewal days: must cover the overdue period (continuous).
	if today.After(endDate) {
		minAdditionalDays := services.CalculateDays(endDate, today)
		if minAdditionalDays < 0 {
			minAdditionalDays = 0
		}
		if req.AdditionalDays < minAdditionalDays {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40002,
				"message": fmt.Sprintf("续期天数至少为 %d 天（需覆盖逾期期）", minAdditionalDays),
			})
			return
		}
	}
	renewalCost, _ := services.CalculateRenewalPricing(
		baseRate, pricingTiers, consumedDays, req.AdditionalDays, cumDisc,
	)

	// #1744: 续期支付可手动输入优惠码（可不同、可不用）——服务端整单折扣，
	// 同 prepay 语义（waive 全免 / percent 按千分比）。
	totalAmount := renewalCost
	couponApplied := ""
	if req.CouponCode != "" {
		var coupon models.Coupon
		if err := db.Where("code = ? AND active = ?", strings.ToUpper(req.CouponCode), true).First(&coupon).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid coupon code"})
			return
		}
		switch coupon.Type {
		case "waive":
			totalAmount = 0
		case "percent":
			totalAmount = math.Round(renewalCost*float64(coupon.Value)/1000*100) / 100
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "unsupported coupon type"})
			return
		}
		couponApplied = coupon.Code
	}
	// 优惠快照回写（#1744）：记录最近一次支付使用的码 + 折扣（分）。
	if couponApplied != "" {
		discountCents := models.Cents(renewalCost) - models.Cents(totalAmount)
		if discountCents < 0 {
			discountCents = 0
		}
		if err := db.Model(&models.Order{}).Where("id = ?", orderID).
			Updates(map[string]interface{}{"coupon_code": couponApplied, "coupon_discount": int64(discountCents)}).Error; err != nil {
			log.Printf("[ConfirmRenewal] failed to write coupon snapshot for order %s: %v", orderID, err)
		}
	}

	cfg := wechatpay.GetConfig()
	outTradeNo := fmt.Sprintf("renewal%s%d", uuid.New().String()[:8], time.Now().Unix())

	meta := renewalMetadata{
		AdditionalDays: req.AdditionalDays,
		OrderID:        orderID,
		OutTradeNo:     outTradeNo,
	}
	metaJSON, _ := json.Marshal(meta)
	metaStr := string(metaJSON)

	record := models.OrderPaymentRecord{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		OrgID:       &order.OrgID,
		UserID:      userID,
		OrderID:     &orderID,
		OrderType:   "renewal",
		OutTradeNo:  &outTradeNo,
		Amount:      models.Cents(totalAmount),
		Type:        "payment",
		Status:      "pending",
		RawResponse: &metaStr,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := db.Create(&record).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create payment record"})
		return
	}

	// #1760: resolve openid server-side when the client omits it — the
	// frontend cannot be trusted to know the payer's openid (prepay
	// backfills it per #1678; renewal/confirm was the only missing path).
	// Empty after fallback → explicit 40002, never a 500 from WeChat.
	if req.OpenID == "" {
		req.OpenID = openidOfUser(db, userID)
	}
	if req.OpenID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "未绑定微信，请先绑定后继续"})
		return
	}

	client := wechatpay.GetClient()
	result, err := client.CreateJSAPIOrder(ctx, wechatpay.JSAPIParams{
		OutTradeNo:  outTradeNo,
		OpenID:      req.OpenID,
		TotalAmount: int64(totalAmount),
		Description: fmt.Sprintf("TuneLoop 续期 %s", orderID[:8]),
		NotifyURL:   cfg.NotifyURL,
	})
	if err != nil {
		record.Status = "failed"
		fr := err.Error()
		record.FailReason = &fr
		db.Model(&record).Updates(map[string]interface{}{"status": "failed", "fail_reason": fr})
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create payment: " + err.Error()})
		return
	}

	db.Model(&record).Update("prepay_id", result.PrepayID)
	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": RenewalConfirmResponse{
			Success: true,
			Data: &PrepayData{
				OutTradeNo: outTradeNo,
				PrepayID:   result.PrepayID,
				AppID:      cfg.AppID,
				TimeStamp:  result.TimeStamp,
				NonceStr:   result.NonceStr,
				Package:    result.Package,
				SignType:   result.SignType,
				PaySign:    result.Sign,
			},
		},
	})
}

func applyRenewalSideEffects(tx *gorm.DB, record *models.OrderPaymentRecord, now time.Time) error {
	var meta renewalMetadata
	if record.RawResponse != nil && *record.RawResponse != "" {
		if err := json.Unmarshal([]byte(*record.RawResponse), &meta); err != nil {
			return err
		}
	}
	orderID := meta.OrderID
	if orderID == "" && record.OrderID != nil {
		orderID = *record.OrderID
	}
	if orderID == "" || meta.AdditionalDays <= 0 {
		return fmt.Errorf("invalid renewal metadata")
	}

	var order models.Order
	if err := tx.Where("id = ?", orderID).First(&order).Error; err != nil {
		return err
	}

	// Renewal continues from the original end date (not from today), so an
	// overdue order's new end date = end_date + additional_days (continuous).
	endDate := parseDatePtr(order.EndDate)
	newEndDate := endDate.AddDate(0, 0, meta.AdditionalDays)
	newEndDateStr := newEndDate.Format("2006-01-02")

	if err := tx.Model(&order).Updates(map[string]interface{}{
		"end_date":   newEndDateStr,
		"status":     models.OrderStatusInLease,
		"updated_at": now,
	}).Error; err != nil {
		return err
	}

	// Update pricing_breakdown with new tier segments
	if order.PricingBreakdown != nil && *order.PricingBreakdown != "" {
		var pb services.PricingBreakdown
		if err := json.Unmarshal([]byte(*order.PricingBreakdown), &pb); err == nil {
			originalDays := pb.RentDays
			newTotalDays := originalDays + meta.AdditionalDays
			pb.RentDays = newTotalDays

			// Recompute tier segments for the full new term
			pb.TierSegments = services.ComputeTierSegments(newTotalDays, pb.PricingTiers)

			// Build cumulative discount from applied policies
			cumulativeDiscount := 1.0
			for _, p := range pb.AppliedPolicies {
				if p.Type == "membership_discount" || p.Type == "promo_campaign" {
					if p.Rate > 0 {
						cumulativeDiscount *= p.Rate
					}
				}
			}

			// Recompute totals
			newTotalAmount := 0.0
			for i := range pb.TierSegments {
				s := &pb.TierSegments[i]
				s.Rate = pb.BaseDailyRent
				s.Discount = s.Discount * cumulativeDiscount
				s.Subtotal = s.Rate * s.Discount * float64(s.Days)
				newTotalAmount += s.Subtotal
			}

			pb.TotalAmount = math.Round(newTotalAmount*100) / 100
			newEffectiveRate := pb.TotalAmount / float64(newTotalDays)
			pb.FinalDailyRent = math.Round(newEffectiveRate*100) / 100

			updatedPBJSON, err := json.Marshal(pb)
			if err == nil {
				updatedStr := string(updatedPBJSON)
				tx.Model(&order).Update("pricing_breakdown", &updatedStr)
			}
		}
	}

	// Update cash_paid / prepaid_points_used with renewal payment
	renewalAmount := record.Amount
	if record.Status == "paid" && renewalAmount > 0 {
		tx.Model(&models.Order{}).Where("id = ?", orderID).Updates(map[string]interface{}{
			"cash_paid":           gorm.Expr("cash_paid + ?", renewalAmount),
			"prepaid_points_used": gorm.Expr("prepaid_points_used + ?", 0), // renewal is cash (WeChat Pay)
		})
	}

	tx.Create(&models.OrderLog{
		OrderID:   orderID,
		Event:     "renewed",
		CreatedAt: now,
	})

	tx.Create(&models.OrderLog{
		OrderID:   orderID,
		Event:     fmt.Sprintf("续期 %d 天, 新到期日 %s", meta.AdditionalDays, newEndDateStr),
		CreatedAt: now,
	})

	// Query instrument SN so the notification identifies the instrument.
	instrumentLabel := ""
	if order.InstrumentID != "" {
		var inst models.Instrument
		if err := tx.Where("id = ?", order.InstrumentID).First(&inst).Error; err == nil {
			if inst.SN != "" {
				instrumentLabel = inst.SN
			}
			if inst.CategoryName != "" {
				instrumentLabel = fmt.Sprintf("%s（%s）", instrumentLabel, inst.CategoryName)
			}
		}
	}
	if instrumentLabel == "" {
		instrumentLabel = orderID[:8]
	}

	// #1742: record.UserID stores the JWT sub (prepay-time GetUserID), but
	// notification.user_id must be the LOCAL users.id — reverse lookup via
	// iam_sub; skip the notification when no local user exists.
	notified := false
	var renewalUser models.User
	if err := tx.Where("iam_sub = ?", record.UserID).First(&renewalUser).Error; err == nil {
		tx.Create(&models.Notification{
			TenantID:   record.TenantID,
			OrgID:      order.OrgID,
			UserID:     renewalUser.ID,
			Type:       "renewal",
			Title:      "续期成功",
			Content:    fmt.Sprintf("乐器 %s 续期 %d 天成功，新到期日：%s", instrumentLabel, meta.AdditionalDays, newEndDateStr),
			RefID:      orderID,
			RefType:    "order",
			ActionType: "order",
			ActionData: strPtr(fmt.Sprintf(`{"order_id":"%s"}`, orderID)),
			Status:     "unread",
			CreatedAt:  now,
		})
		notified = true
	}
	if !notified {
		log.Printf("[applyRenewalSideEffects] no local user for iam_sub %s — skip renewal notification", record.UserID)
	}

	// Re-evaluate membership level after renewal payment
	if err := services.CheckAndUpgradeLevel(record.UserID, nil); err != nil {
		log.Printf("[applyRenewalSideEffects] membership level check failed: %v", err)
	}

	return nil
}
