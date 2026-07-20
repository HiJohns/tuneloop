package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
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
	RenewalCost    float64                `json:"renewal_cost"`
	OverdueBalance float64                `json:"overdue_balance"`
	TotalAmount    float64                `json:"total_amount"`
	NewEndDate     string                 `json:"new_end_date"`
	TierBreakdown  []services.TierSegment `json:"tier_breakdown"`
	DailyRate      float64                `json:"daily_rate"`
	OverdueDays    int                    `json:"overdue_days"`
}

type RenewalConfirmRequest struct {
	AdditionalDays int    `json:"additional_days" binding:"required,min=1"`
	OpenID         string `json:"open_id,omitempty"`
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
	var order models.Order
	if err := db.Where("id = ? AND user_id = ?", orderID, userID).First(&order).Error; err != nil {
		return nil, err
	}
	if order.Status != models.OrderStatusInLease && order.Status != models.OrderStatusExpired {
		return nil, fmt.Errorf("order can only be renewed when status is in_lease or expired")
	}
	return &order, nil
}

func parseDatePtr(s *string) time.Time {
	if s == nil || *s == "" {
		return time.Now().Truncate(24 * time.Hour)
	}
	t, err := time.Parse("2006-01-02", *s)
	if err != nil {
		return time.Now().Truncate(24 * time.Hour)
	}
	return t
}

func loadRenewalPricing(order *models.Order) (baseRate float64, pricingTiers []services.PricingTierConfig, cumulativeDiscount float64) {
	var pb services.PricingBreakdown
	if order.PricingBreakdown != nil && *order.PricingBreakdown != "" {
		json.Unmarshal([]byte(*order.PricingBreakdown), &pb)
	}
	baseRate = pb.BaseDailyRent
	if baseRate <= 0 && order.MonthlyRent > 0 {
		baseRate = order.MonthlyRent / 30
	}
	if baseRate <= 0 {
		baseRate = 50
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

	consumedDays := int(today.Sub(startDate).Hours() / 24)
	if consumedDays < 0 {
		consumedDays = 0
	}

	var overdueDays int
	if today.After(endDate) {
		overdueDays = int(today.Sub(endDate).Hours() / 24)
		if overdueDays < 0 {
			overdueDays = 0
		}
	}

	newEndDate := today.AddDate(0, 0, req.AdditionalDays)
	baseRate, pricingTiers, cumDisc := loadRenewalPricing(order)

	renewalCost, tierBreakdown := services.CalculateRenewalPricing(
		baseRate, pricingTiers, consumedDays, req.AdditionalDays, cumDisc,
	)

	var overdueBalance float64
	db.Model(&models.OverdueCharge{}).
		Select("COALESCE(SUM(remaining_balance), 0)").
		Where("order_id = ? AND status IN ?", orderID, []string{"failed", "partial"}).
		Scan(&overdueBalance)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": RenewalCalculateResponse{
			RenewalCost:    renewalCost,
			OverdueBalance: overdueBalance,
			TotalAmount:    renewalCost + overdueBalance,
			NewEndDate:     newEndDate.Format("2006-01-02"),
			TierBreakdown:  tierBreakdown,
			DailyRate:      baseRate,
			OverdueDays:    overdueDays,
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

	order, err := loadOrderForRenewal(db, orderID, userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}

	startDate := parseDatePtr(order.StartDate)
	today := time.Now().Truncate(24 * time.Hour)
	consumedDays := int(today.Sub(startDate).Hours() / 24)
	if consumedDays < 0 {
		consumedDays = 0
	}

	baseRate, pricingTiers, cumDisc := loadRenewalPricing(order)
	renewalCost, _ := services.CalculateRenewalPricing(
		baseRate, pricingTiers, consumedDays, req.AdditionalDays, cumDisc,
	)

	var overdueBalance float64
	db.Model(&models.OverdueCharge{}).
		Select("COALESCE(SUM(remaining_balance), 0)").
		Where("order_id = ? AND status IN ?", orderID, []string{"failed", "partial"}).
		Scan(&overdueBalance)

	totalAmount := renewalCost + overdueBalance
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
		Amount:      totalAmount,
		Type:        "payment",
		Status:      "pending",
		RawResponse: &metaStr,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if cfg.MockMode {
		record.Status = "paid"
		record.Method = strPtr("mock")
		now := time.Now()
		record.UpdatedAt = now
		tx := db.Begin()
		if err := tx.Create(&record).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create payment record"})
			return
		}
		if err := applyRenewalSideEffects(tx, &record, now); err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "renewal side effects failed"})
			return
		}
		tx.Commit()
		c.JSON(http.StatusOK, gin.H{
			"code": 20000,
			"data": RenewalConfirmResponse{Success: true, Data: &PrepayData{OutTradeNo: outTradeNo}},
		})
		return
	}

	if err := db.Create(&record).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create payment record"})
		return
	}

	client := wechatpay.GetClient()
	result, err := client.CreateJSAPIOrder(ctx, wechatpay.JSAPIParams{
		OutTradeNo:  outTradeNo,
		OpenID:      req.OpenID,
		TotalAmount: cfg.AmountToCents(totalAmount),
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

	newEndDate := now.AddDate(0, 0, meta.AdditionalDays)
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
			"cash_paid":          gorm.Expr("cash_paid + ?", renewalAmount),
			"prepaid_points_used": gorm.Expr("prepaid_points_used + ?", 0), // renewal is cash (WeChat Pay)
		})
	}

	tx.Model(&models.OverdueCharge{}).
		Where("order_id = ? AND status IN ?", orderID, []string{"failed", "partial"}).
		Update("status", "settled")

	tx.Create(&models.OrderLog{
		OrderID:   orderID,
		Event:     fmt.Sprintf("续期 %d 天, 新到期日 %s", meta.AdditionalDays, newEndDateStr),
		CreatedAt: now,
	})

	tx.Create(&models.Notification{
		TenantID:  record.TenantID,
		OrgID:     order.OrgID,
		UserID:    record.UserID,
		Type:      "renewal",
		Title:     "续期成功",
		Content:   fmt.Sprintf("续期 %d 天成功，新到期日：%s", meta.AdditionalDays, newEndDateStr),
		RefID:     orderID,
		RefType:   "order",
		Status:    "unread",
		CreatedAt: now,
	})

	return nil
}
