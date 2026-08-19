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

type UserSettlementHandler struct{}

func NewUserSettlementHandler() *UserSettlementHandler {
	return &UserSettlementHandler{}
}

func parsePricingBreakdown(pbJSON *string) (map[string]interface{}, float64, error) {
	result := map[string]interface{}{}
	if pbJSON == nil || *pbJSON == "" {
		return result, 0, fmt.Errorf("no pricing breakdown")
	}
	if err := json.Unmarshal([]byte(*pbJSON), &result); err != nil {
		return result, 0, err
	}
	finalDaily, _ := result["final_daily_rent"].(float64)
	return result, finalDaily, nil
}

func parsePointsPolicySnapshot(ppsJSON *string) (map[string]interface{}, float64, float64) {
	result := map[string]interface{}{}
	if ppsJSON == nil || *ppsJSON == "" {
		return result, 0, 0
	}
	json.Unmarshal([]byte(*ppsJSON), &result)
	capRate, _ := result["cap_rate"].(float64)
	payRatio, _ := result["pay_ratio"].(float64)
	return result, capRate, payRatio
}

func (h *UserSettlementHandler) CalculateSettlement(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	orderID := c.Param("id")
	var order models.Order
	if err := db.Where("id = ?", orderID).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "order not found"})
		return
	}

	result := computeSettlement(order, db)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": result.Breakdown,
	})
}

func (h *UserSettlementHandler) ConfirmSettlement(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	userID, err := middleware.EnsureLocalUser(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "user sync failed"})
		return
	}

	orderID := c.Param("id")

	var req struct {
		RefundMethod string `json:"refund_method"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.RefundMethod = "prepaid"
	}

	var order models.Order
	if err := db.Where("id = ? AND user_id = ?", orderID, userID).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "order not found"})
		return
	}

	if order.Status != models.OrderStatusInLease && order.Status != models.OrderStatusReturning && order.Status != models.OrderStatusDepositRefunding {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "order not in settlement status"})
		return
	}

	var existing models.Settlement
	if err := db.Where("order_id = ?", orderID).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "settlement already exists"})
		return
	}

	result := computeSettlement(order, db)

	breakdownJSON, _ := json.Marshal(result.Breakdown)

	tx := db.Begin()

	settlement := models.Settlement{
		ID:                  uuid.New().String(),
		OrderID:             orderID,
		ActualRentDays:      result.ActualDays,
		ActualRentAmount:    result.RentPayable,
		OriginalRentAmount:  result.TotalRentPaid + order.GiftPointsUsed,
		GiftPointsRefunded:  result.GiftPointsRefunded,
		CashRefundable:      result.CashRefundable,
		RefundMethod:        req.RefundMethod,
		RefundStatus:        "pending",
		OverdueChargesTotal: result.OverdueChargesTotal,
		Breakdown:           string(breakdownJSON),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	if err := tx.Create(&settlement).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create settlement"})
		return
	}

	if result.GiftPointsRefunded > 0 {
		var user models.User
		if err := tx.Where("id = ?", userID).First(&user).Error; err == nil {
			if err := tx.Model(&user).Updates(map[string]interface{}{
				"promo_points": gorm.Expr("promo_points + ?", result.GiftPointsRefunded),
				"updated_at":   time.Now(),
			}).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to refund gift points"})
				return
			}
		}
	}

	// Cash refund via WeChat Pay (if cash was paid on this order)
	if result.CashRefundable > 0 {
		var paymentRecord models.OrderPaymentRecord
		paymentFound := false
		var outTradeNo string
		if err := tx.Where("order_id = ? AND order_type = ? AND status = ?", orderID, "rent", "paid").First(&paymentRecord).Error; err == nil && paymentRecord.ID != "" {
			paymentFound = true
			if paymentRecord.OutTradeNo != nil {
				outTradeNo = *paymentRecord.OutTradeNo
			}
		}

		cfg := wechatpay.GetConfig()
		outRefundNo := fmt.Sprintf("sttl_%s_%d", orderID[:8], time.Now().Unix())

		refundRecord := models.OrderRefundRecord{
			ID:        uuid.New().String(),
			TenantID:  order.TenantID,
			Amount:    result.CashRefundable,
			Reason:    strPtr("租赁结算退款"),
			Status:    "pending",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if cfg.MockMode || !paymentFound {
			refundRecord.Status = "refunded"
			settlement.RefundStatus = "completed"
		} else {
			refundRecord.PaymentRecordID = &paymentRecord.ID
			client := wechatpay.GetClient()
			refundResp, err := client.Refund(c.Request.Context(), wechatpay.RefundParams{
				OutTradeNo:   outTradeNo,
				OutRefundNo:  outRefundNo,
				TotalAmount:  cfg.AmountToCents(paymentRecord.Amount),
				RefundAmount: cfg.AmountToCents(result.CashRefundable),
				Reason:       "租赁结算退款",
				NotifyURL:    cfg.RefundNotifyURL,
			})
			if err != nil {
				refundRecord.Status = "failed"
				fr := err.Error()
				refundRecord.FailReason = &fr
				log.Printf("[ConfirmSettlement] refund failed for order %s: %v", orderID, err)
				settlement.RefundStatus = "failed"
			} else {
				refundRecord.RefundID = &refundResp.RefundID
				settlement.RefundStatus = "refunding"
			}
		}
		refundRecord.OutRefundNo = &outRefundNo

		if err := tx.Create(&refundRecord).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create refund record"})
			return
		}
		if err := tx.Model(&settlement).Update("refund_status", settlement.RefundStatus).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update settlement status"})
			return
		}
	}

	// Increment total spending by the CASH portion (C1, L-06)
	spendingBasis := result.CashBasis
	if spendingBasis <= 0 {
		spendingBasis = result.RentPayable
	}
	if err := tx.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"total_spending": gorm.Expr("total_spending + ?", spendingBasis),
		"updated_at":     time.Now(),
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update total spending"})
		return
	}

	// Close the order (L-06): manual settlement confirmation marks the
	// order completed, matching the auto-refund path.
	if err := tx.Model(&models.Order{}).Where("id = ?", orderID).
		Update("status", models.OrderStatusCompleted).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to close order"})
		return
	}

	tx.Commit()

	// Check and upgrade membership level after settlement
	if err := services.CheckAndUpgradeLevel(userID, nil); err != nil {
		log.Printf("[WARN] Membership upgrade check failed for user %s: %v", userID, err)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "settlement confirmed",
		"data": gin.H{
			"settlement_id":        settlement.ID,
			"cash_refundable":      result.CashRefundable,
			"gift_points_refunded": result.GiftPointsRefunded,
		},
	})
}

// executeRefund computes the settlement for an order and performs the
// actual refund (gift points → prepaid points → cash) inside the given
// transaction. Called automatically when an order reaches "completed"
// (good inspection, appeal resolution, agree-damage, damage payment
// callback) so refunds never depend on a manual step (#1530).
//
// Idempotent: if a settlement already exists for the order it returns
// the existing result without refunding twice.
func executeRefund(tx *gorm.DB, order models.Order) (*settlementResult, error) {
	var existing models.Settlement
	if err := tx.Where("order_id = ?", order.ID).First(&existing).Error; err == nil {
		var result settlementResult
		json.Unmarshal([]byte(existing.Breakdown), &result.Breakdown)
		result.RentPayable = existing.ActualRentAmount
		result.TotalRentPaid = existing.OriginalRentAmount
		result.GiftPointsRefunded = existing.GiftPointsRefunded
		result.CashRefundable = existing.CashRefundable
		result.OverdueChargesTotal = existing.OverdueChargesTotal
		result.ActualDays = existing.ActualRentDays
		return &result, nil
	}

	result := computeSettlement(order, tx)

	breakdownJSON, _ := json.Marshal(result.Breakdown)

	settlement := models.Settlement{
		ID:                  uuid.New().String(),
		OrderID:             order.ID,
		ActualRentDays:      result.ActualDays,
		ActualRentAmount:    result.RentPayable,
		OriginalRentAmount:  result.TotalRentPaid + order.GiftPointsUsed,
		GiftPointsRefunded:  result.GiftPointsRefunded,
		CashRefundable:      result.CashRefundable,
		PrepaidRefunded:     0, // no prepaid-points deduction logic yet (#1636)
		RefundMethod:        "wechat_pay",
		RefundStatus:        "pending",
		OverdueChargesTotal: result.OverdueChargesTotal,
		Breakdown:           string(breakdownJSON),
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	if err := tx.Create(&settlement).Error; err != nil {
		return nil, fmt.Errorf("failed to create settlement: %w", err)
	}

	// Refund gift points (over cap portion) to promo_points
	if result.GiftPointsRefunded > 0 {
		if err := tx.Model(&models.User{}).Where("id = ?", order.UserID).Updates(map[string]interface{}{
			"promo_points": gorm.Expr("promo_points + ?", result.GiftPointsRefunded),
			"updated_at":   time.Now(),
		}).Error; err != nil {
			return nil, fmt.Errorf("failed to refund gift points: %w", err)
		}
	}

	// Cash refund via WeChat Pay (mock mode books directly)
	if result.CashRefundable > 0 {
		cfg := wechatpay.GetConfig()
		mockMode := cfg != nil && cfg.MockMode

		outRefundNo := fmt.Sprintf("sttl_%s_%d", order.ID[:8], time.Now().Unix())

		refundRecord := models.OrderRefundRecord{
			ID:        uuid.New().String(),
			TenantID:  order.TenantID,
			Amount:    result.CashRefundable,
			Reason:    strPtr("租赁结算退款"),
			Status:    "pending",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		var paymentRecord models.OrderPaymentRecord
		paymentFound := false
		var outTradeNo string
		if err := tx.Where("order_id = ? AND order_type = ? AND status = ?", order.ID, "rent", "paid").First(&paymentRecord).Error; err == nil && paymentRecord.ID != "" {
			paymentFound = true
			if paymentRecord.OutTradeNo != nil {
				outTradeNo = *paymentRecord.OutTradeNo
			}
		}

		if mockMode || !paymentFound || cfg == nil {
			refundRecord.Status = "refunded"
			settlement.RefundStatus = "completed"
		} else {
			refundRecord.PaymentRecordID = &paymentRecord.ID
			client := wechatpay.GetClient()
			refundResp, err := client.Refund(nil, wechatpay.RefundParams{
				OutTradeNo:   outTradeNo,
				OutRefundNo:  outRefundNo,
				TotalAmount:  cfg.AmountToCents(paymentRecord.Amount),
				RefundAmount: cfg.AmountToCents(result.CashRefundable),
				Reason:       "租赁结算退款",
				NotifyURL:    cfg.RefundNotifyURL,
			})
			if err != nil {
				refundRecord.Status = "failed"
				fr := err.Error()
				refundRecord.FailReason = &fr
				log.Printf("[executeRefund] refund failed for order %s: %v", order.ID, err)
				settlement.RefundStatus = "failed"
			} else {
				refundRecord.RefundID = &refundResp.RefundID
				settlement.RefundStatus = "refunding"
			}
		}
		refundRecord.OutRefundNo = &outRefundNo

		if err := tx.Create(&refundRecord).Error; err != nil {
			return nil, fmt.Errorf("failed to create refund record: %w", err)
		}
	}

	if err := tx.Model(&settlement).Update("refund_status", settlement.RefundStatus).Error; err != nil {
		return nil, fmt.Errorf("failed to update settlement status: %w", err)
	}

	// Mark deposit as refunded only after actual refund executes
	if err := tx.Model(&models.Order{}).Where("id = ?", order.ID).Update("deposit_refunded", true).Error; err != nil {
		return nil, fmt.Errorf("failed to mark deposit refunded: %w", err)
	}

	// Increment total spending by the CASH portion of actual rental amount
	// (C1 = R1 − A1, gift points excluded — prevents gift-point feedback
	// loops, L-06). Industry practice: growth values count real spend.
	spendingBasis := result.CashBasis
	if spendingBasis <= 0 {
		spendingBasis = result.RentPayable
	}
	if err := tx.Model(&models.User{}).Where("id = ?", order.UserID).Updates(map[string]interface{}{
		"total_spending": gorm.Expr("total_spending + ?", spendingBasis),
		"updated_at":     time.Now(),
	}).Error; err != nil {
		return nil, fmt.Errorf("failed to update total spending: %w", err)
	}

	// Rebate gift points (L-06): A2 = floor(C1 × refund_ratio) credited on
	// refund completion. refund_ratio from the user's level gift policy;
	// legacy fallback: membership_gift_ratios.SelfSpendRatio on RentPayable.
	var orderUser models.User
	if err := tx.Where("id = ?", order.UserID).First(&orderUser).Error; err == nil {
		rebatePoints := 0.0
		rebateDesc := ""
		if policy := services.GetGiftPolicyByLevel(tx, levelIDOrZero(orderUser.MembershipLevelID)); policy != nil && policy.RefundRatio > 0 {
			rebatePoints = math.Floor(result.CashBasis * policy.RefundRatio)
			rebateDesc = fmt.Sprintf("退款返赠点: 实付现金 ¥%.2f × %.2f%%", result.CashBasis, policy.RefundRatio*100)
		}
		if rebatePoints <= 0 {
			var selfRatio float64
			if orderUser.MembershipLevelID != nil {
				if ratios := services.GetGiftRatios(*orderUser.MembershipLevelID); ratios != nil {
					selfRatio = ratios.SelfSpendRatio
				}
			}
			if selfRatio > 0 {
				rebatePoints = math.Floor(result.RentPayable * selfRatio)
				rebateDesc = fmt.Sprintf("消费返赠点: 租金 ¥%.2f × %.2f%%", result.RentPayable, selfRatio*100)
			}
		}
		if rebatePoints > 0 {
			if err := tx.Model(&models.User{}).Where("id = ?", order.UserID).Updates(map[string]interface{}{
				"promo_points": gorm.Expr("promo_points + ?", rebatePoints),
				"updated_at":   time.Now(),
			}).Error; err != nil {
				log.Printf("[executeRefund] rebate points credit failed for %s: %v", order.UserID, err)
			} else {
				tx.Create(&models.PointsTransaction{
					ID:          uuid.New().String(),
					UserID:      order.UserID,
					TenantID:    order.TenantID,
					Type:        "refund_rebate",
					Amount:      rebatePoints,
					OrderID:     &order.ID,
					Description: rebateDesc,
					CreatedAt:   time.Now(),
				})
			}
		}

		// Referral commission (#1542 + #1535): referrer gets gift points
		// proportional to the referred user's rent × referrer-level ratio.
		if referrer := services.FindReferrer(order.UserID); referrer != nil {
			var refRatio float64
			if referrer.MembershipLevelID != nil {
				if ratios := services.GetGiftRatios(*referrer.MembershipLevelID); ratios != nil {
					refRatio = ratios.ReferralSpendRatio
				}
			}
			if refRatio > 0 {
				refPoints := math.Floor(result.RentPayable * refRatio)
				if refPoints > 0 {
					if err := tx.Model(&models.User{}).Where("id = ?", referrer.ID).Updates(map[string]interface{}{
						"promo_points": gorm.Expr("promo_points + ?", refPoints),
						"updated_at":   time.Now(),
					}).Error; err != nil {
						log.Printf("[executeRefund] referral points credit failed for %s: %v", referrer.ID, err)
					} else {
						tx.Create(&models.PointsTransaction{
							ID:          uuid.New().String(),
							UserID:      referrer.ID,
							TenantID:    order.TenantID,
							Type:        "referral",
							Amount:      refPoints,
							OrderID:     &order.ID,
							Description: fmt.Sprintf("介绍人返赠点: 被介绍人订单租金 ¥%.2f × %.2f%%", result.RentPayable, refRatio*100),
							CreatedAt:   time.Now(),
						})
					}
				}
			}
		}
	}

	return &result, nil
}

func (h *UserSettlementHandler) GetSettlement(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	orderID := c.Param("id")

	var settlement models.Settlement
	if err := db.Where("order_id = ?", orderID).First(&settlement).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "settlement not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": settlement,
	})
}

func parseDate(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	// GORM scans gorm:"type:date" columns into *string as RFC3339
	// (e.g. "2026-07-01T00:00:00Z"), not bare "2006-01-02". Parse all
	// layouts used by the codebase so settlement math works on
	// DB-loaded orders (discovered by TestSettlementFlow, #1563).
	for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, *s); err == nil {
			return &t
		}
	}
	return nil
}

// settlementResult holds the canonical settlement calculation for an order.
type settlementResult struct {
	RentPayable            float64
	TotalRentPaid          float64
	RemainingDeposit       float64
	DepositDeductedOverdue float64
	DamageDeducted         float64
	TotalRefund            float64
	CashRefundable         float64
	GiftPointsRefunded     float64
	OverdueChargesTotal    float64
	ActualDays             int
	CashBasis              float64 // C1: cash actually paid for rent (R1 − A1), spending/rebate basis
	Breakdown              map[string]interface{}
}

// computeSettlement performs the tier-based rent calculation and refund math.
// It mirrors docs/cases.md §2.7.
func computeSettlement(order models.Order, db *gorm.DB) settlementResult {
	_, finalDailyRent, _ := parsePricingBreakdown(order.PricingBreakdown)
	if finalDailyRent == 0 && order.PricingBreakdown != nil && *order.PricingBreakdown != "" {
		var pb map[string]interface{}
		if json.Unmarshal([]byte(*order.PricingBreakdown), &pb) == nil {
			if v, ok := pb["base_daily_rent"].(float64); ok && v > 0 {
				finalDailyRent = v
			}
		}
	}
	_, capRate, snapPayRatio := parsePointsPolicySnapshot(order.PointsPolicySnapshot)

	var tierSegments []services.TierSegment
	if order.PricingBreakdown != nil && *order.PricingBreakdown != "" {
		var pb struct {
			TierSegments []services.TierSegment `json:"tier_segments"`
		}
		if err := json.Unmarshal([]byte(*order.PricingBreakdown), &pb); err == nil {
			tierSegments = pb.TierSegments
		}
	}

	// Overdue fee for staged settlement: collected once at return inspection
	// (#1493) and persisted on the DamageReport (#1708; legacy data on
	// DamageAssessment until the DDL migration). Legacy per-day overdue_charges
	// are ignored (daily deduction removed, #1492).
	var reportOverdue models.DamageReport
	if err := db.Where("lease_id = ?", order.ID).Order("created_at desc").First(&reportOverdue).Error; err != nil {
		reportOverdue = models.DamageReport{}
	}
	overdueFee := reportOverdue.OverdueFee
	if overdueFee == 0 {
		// Legacy orders: overdue persisted on DamageAssessment.
		var assessment models.DamageAssessment
		if err := db.Where("order_id = ?", order.ID).Order("created_at desc").First(&assessment).Error; err == nil {
			overdueFee = assessment.OverdueFee
		}
	}
	if overdueFee < 0 {
		overdueFee = 0
	}

	startDate := parseDate(order.StartDate)

	rentPayable := 0.0
	actualDays := 0

	// Derive actual lease period: returned_at for returned/completed orders,
	// end_date otherwise. Start from delivered_at (实际收货) when available so
	// 当天收货当天归还 (北京同日, 不足 24h) 计 1 天而非自然日 2 天 (#1665 口径).
	actualLeaseEnd := parseDate(order.EndDate)
	if order.ReturnedAt != nil && (order.Status == "returned" || order.Status == "completed" || order.Status == "returning") {
		rt := *order.ReturnedAt
		actualLeaseEnd = &rt
	}
	actualLeaseStart := startDate
	if order.DeliveredAt != nil && (order.Status == "returned" || order.Status == "completed" || order.Status == "returning") {
		actualLeaseStart = order.DeliveredAt
	}
	if actualLeaseStart != nil && actualLeaseEnd != nil {
		hours := actualLeaseEnd.Sub(*actualLeaseStart).Hours()
		actualDays = int(math.Ceil(hours / 24))
	}
	if actualDays < 1 {
		actualDays = 1
	}

	if len(tierSegments) > 0 {
		cursor := 1
		for _, seg := range tierSegments {
			if cursor > actualDays {
				break
			}
			segEnd := cursor + seg.Days - 1
			// Cap segment days at actual lease days
			effectiveSegDays := seg.Days
			if cursor+effectiveSegDays-1 > actualDays {
				effectiveSegDays = actualDays - cursor + 1
			}
			if effectiveSegDays > 0 {
				rentPayable += float64(effectiveSegDays) * seg.Rate * seg.Discount
			}
			cursor = segEnd + 1
		}
	} else {
		if startDate != nil && actualLeaseEnd != nil {
			rentPayable = finalDailyRent * float64(actualDays)
		}
	}
	rentPayable = math.Round(rentPayable*100) / 100

	totalRentPaid := order.CashPaid + order.PrepaidPointsUsed + order.GiftPointsUsed - order.Deposit - order.ShippingFee
	if totalRentPaid == 0 && order.PricingBreakdown != nil && *order.PricingBreakdown != "" {
		var pb map[string]interface{}
		if json.Unmarshal([]byte(*order.PricingBreakdown), &pb) == nil {
			if v, ok := pb["total_amount"].(float64); ok {
				totalRentPaid = v
			}
		}
	}

	var damageDeducted float64
	var report models.DamageReport
	if err := db.Where("lease_id = ?", order.ID).First(&report).Error; err == nil {
		damageDeducted = report.DepositDeducted
	}

	// Deposit deduction: overdue fee (charged once at return, #1493) +
	// damage deduction + logistics fee (filled by staff at SHIPPING page,
	// #1541/#1621 — design moved fee entry to dispatch, not inspection).
	// All come off the deposit; remainder participates in the refund.
	shippingFee := order.ShippingFee
	if shippingFee < 0 {
		shippingFee = 0
	}
	totalDepositDeducted := overdueFee + damageDeducted + shippingFee
	remainingDeposit := order.Deposit - totalDepositDeducted
	if remainingDeposit < 0 {
		remainingDeposit = 0
	}

	totalRefund := totalRentPaid + remainingDeposit - rentPayable
	if totalRefund < 0 {
		totalRefund = 0
	}

	// Early-return rebate: rent paid for days not actually used.
	earlyReturnRebate := totalRentPaid - rentPayable
	if earlyReturnRebate < 0 {
		earlyReturnRebate = 0
	}

	// Refund order (#1537, L-06): differential gift-point split.
	// A1 = floor(R1 × pay_ratio) — the gift-point usage cap recomputed at
	// refund time against the adjusted payable R1 and the user's current
	// membership level gift policy (snapshot pay_ratio first, then
	// current-level policy, then legacy cap_rate, then default 0).
	//
	//   A1 < A0 → refund (A0 − A1) gift points to promo_points,
	//             refund (C0 − C1) cash to WeChat (C1 = R1 − A1)
	//   A1 ≥ A0 → gift points stay at A0, refund cash C0 − (R1 − A0)
	//
	// Conservation: gift_refunded + cash_refunded = R0 − R1.
	payRatio := snapPayRatio
	levelID := 0
	if order.UserID != "" {
		var u models.User
		if err := db.Select("membership_level_id").Where("id = ?", order.UserID).First(&u).Error; err == nil && u.MembershipLevelID != nil {
			levelID = *u.MembershipLevelID
		}
	}
	if payRatio <= 0 {
		if policy := services.GetGiftPolicyByLevel(db, levelID); policy != nil {
			payRatio = policy.PayRatio
		}
	}
	if payRatio <= 0 {
		// legacy fallback: cap_rate (percent) → ratio
		payRatio = capRate / 100
	}
	if payRatio <= 0 {
		payRatio = 0.3
	}

	a0 := order.GiftPointsUsed
	a1 := math.Floor(rentPayable * payRatio)
	if a1 < 0 {
		a1 = 0
	}

	var giftPointsRefunded, cashRefundable float64
	if a1 < a0 {
		giftPointsRefunded = a0 - a1
		// C1 = R1 − A1; cash refund = C0 − C1 = (R0 − A0) − (R1 − A1)
		cashRefundable = totalRefund - giftPointsRefunded
	} else {
		// gift stays at A0 → cash refund = R0 − (A0 + R1 − A0) = R0 − R1
		giftPointsRefunded = 0
		cashRefundable = totalRefund
	}
	if cashRefundable < 0 {
		cashRefundable = 0
	}

	breakdown := map[string]interface{}{
		"original_total":            order.CashPaid + order.PrepaidPointsUsed + order.GiftPointsUsed,
		"total_rent_paid":           totalRentPaid,
		"deposit":                   order.Deposit,
		"deposit_deducted_overdue":  overdueFee,
		"deposit_deducted_damage":   damageDeducted,
		"deposit_deducted_shipping": shippingFee,
		"remaining_deposit":         remainingDeposit,
		"damage_deducted":           damageDeducted,
		"overdue_fee":               overdueFee,
		"overdue_days":              reportOverdue.OverdueDays,
		"early_return_rebate":       earlyReturnRebate,
		"rent_payable":              rentPayable,
		"actual_rent_amount":        rentPayable, // backward-compatible alias
		"actual_rent_days":          actualDays,
		"final_daily_rent":          finalDailyRent,
		"total_refund":              totalRefund,
		"cash_refundable":           cashRefundable,
		"gift_points_used":          order.GiftPointsUsed,
		"gift_cap":                  a1,
		"gift_points_refunded":      giftPointsRefunded,
		"cash_paid":                 order.CashPaid,
		"pay_ratio":                 payRatio,
		"tier_segments":             tierSegments,
	}

	// C1 = R1 − min(A1, A0): the cash portion of the adjusted payable rent.
	// When A1 ≥ A0 the gift actually used is A0, so the cash basis is
	// R1 − A0. Basis for total_spending accumulation and rebate points (L-06).
	effectiveGift := a1
	if a0 < a1 {
		effectiveGift = a0
	}
	cashBasis := rentPayable - effectiveGift
	if cashBasis < 0 {
		cashBasis = 0
	}

	return settlementResult{
		RentPayable:            rentPayable,
		TotalRentPaid:          totalRentPaid,
		RemainingDeposit:       remainingDeposit,
		DepositDeductedOverdue: overdueFee,
		DamageDeducted:         damageDeducted,
		TotalRefund:            totalRefund,
		CashRefundable:         cashRefundable,
		GiftPointsRefunded:     giftPointsRefunded,
		OverdueChargesTotal:    overdueFee,
		ActualDays:             actualDays,
		CashBasis:              cashBasis,
		Breakdown:              breakdown,
	}
}

// buildRefundReceipt generates the standard receipt text for refund
// notifications (cases.md 退货-定损-申诉-退款用例). Fields:
//
//	rent = actual rent paid, shipping_fee = order.ShippingFee,
//	overdue = assessment overdue, damage = damage deducted,
//	renewal = sum of paid renewal payments, total_paid = all payments,
//	actual_refund = settlement refund amount.
func buildRefundReceipt(db *gorm.DB, order models.Order, s *settlementResult) string {
	var renewalTotal float64
	db.Model(&models.OrderPaymentRecord{}).
		Where("order_id = ? AND order_type = ? AND status = ? AND type = ?", order.ID, "renewal", "paid", "payment").
		Select("COALESCE(SUM(amount),0)").Scan(&renewalTotal)

	// Instrument SN (category) for the receipt header (L-06)
	instrumentLabel := ""
	if order.InstrumentID != "" {
		var inst struct {
			SN           string
			CategoryName string
		}
		if err := db.Model(&models.Instrument{}).
			Select("sn, category_name").
			Where("id = ?", order.InstrumentID).
			First(&inst).Error; err == nil && inst.SN != "" {
			if inst.CategoryName != "" {
				instrumentLabel = fmt.Sprintf("%s（%s）", inst.SN, inst.CategoryName)
			} else {
				instrumentLabel = inst.SN
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("租赁结算明细\n")
	if instrumentLabel != "" {
		sb.WriteString(fmt.Sprintf("乐器：%s\n", instrumentLabel))
	}
	sb.WriteString(fmt.Sprintf("实际租期：%d 天\n", s.ActualDays))
	sb.WriteString("——\n")
	sb.WriteString(fmt.Sprintf("租金：¥%.2f\n", s.RentPayable))
	if order.ShippingFee > 0 {
		sb.WriteString(fmt.Sprintf("物流费：¥%.2f\n", order.ShippingFee))
	}
	if s.OverdueChargesTotal > 0 {
		sb.WriteString(fmt.Sprintf("逾期费：¥%.2f\n", s.OverdueChargesTotal))
	}
	if s.DamageDeducted > 0 {
		sb.WriteString(fmt.Sprintf("损坏赔偿：¥%.2f\n", s.DamageDeducted))
	}
	if renewalTotal > 0 {
		sb.WriteString(fmt.Sprintf("续期费用：¥%.2f\n", renewalTotal))
	}
	sb.WriteString("——\n")
	sb.WriteString(fmt.Sprintf("应付合计：¥%.2f\n", s.TotalRefund+s.RentPayable+s.DamageDeducted+s.OverdueChargesTotal))
	sb.WriteString(fmt.Sprintf("其中赠点抵扣：%.0f 点\n", order.GiftPointsUsed))
	sb.WriteString(fmt.Sprintf("现金应付：¥%.2f\n", s.CashBasis))
	sb.WriteString(fmt.Sprintf("已收（含押金）：¥%.2f\n", order.CashPaid+order.PrepaidPointsUsed+order.GiftPointsUsed+order.Deposit))
	sb.WriteString(fmt.Sprintf("押金退还：¥%.2f\n", s.RemainingDeposit))
	sb.WriteString("——\n")
	if s.GiftPointsRefunded > 0 {
		sb.WriteString(fmt.Sprintf("退回赠点：%.0f 点\n", s.GiftPointsRefunded))
	}
	sb.WriteString(fmt.Sprintf("退回微信：¥%.2f\n", s.CashRefundable))
	sb.WriteString(fmt.Sprintf("实际退款合计：¥%.2f\n", s.CashRefundable+s.GiftPointsRefunded))
	sb.WriteString("返点赠点到账：详见会员中心")
	return sb.String()
}

// StaffRefundOrder POST /orders/:id/refund — staff-triggered refund for an
// order in deposit_refunding (L-04 path 2/3). Executes the differential
// settlement (L-06), closes the order (completed) and returns the receipt.
func (h *UserSettlementHandler) StaffRefundOrder(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	orderID := c.Param("id")

	// Staff-only: site_admin / site_member (merchant_admin & system admin
	// also allowed for platform-level operations).
	role := middleware.GetBusinessRole(ctx)
	switch role {
	case middleware.BusinessRoleSiteAdmin, middleware.BusinessRoleSiteMember,
		middleware.BusinessRoleMerchantAdmin, middleware.BusinessRoleSystemAdmin:
		// allowed
	default:
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "no permission to refund orders"})
		return
	}

	var order models.Order
	if err := db.Where("id = ?", orderID).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "order not found"})
		return
	}

	// Org isolation: site staff may only refund orders in their org.
	if role == middleware.BusinessRoleSiteAdmin || role == middleware.BusinessRoleSiteMember {
		orgID := middleware.GetOrgID(ctx)
		if orgID == "" || order.OrgID != orgID {
			c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "order does not belong to your site"})
			return
		}
	}

	if order.Status != models.OrderStatusDepositRefunding {
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "order is not in refunding status"})
		return
	}

	result, err := executeRefund(db, order)
	if err != nil {
		log.Printf("[StaffRefundOrder] executeRefund failed for %s: %v", orderID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "refund failed: " + err.Error()})
		return
	}

	// Close the order (L-04/L-06): deposit_refunding → completed
	if err := db.Model(&models.Order{}).Where("id = ?", orderID).
		Update("status", models.OrderStatusCompleted).Error; err != nil {
		log.Printf("[StaffRefundOrder] failed to close order %s: %v", orderID, err)
	}

	receipt := buildRefundReceipt(db, order, result)

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "refund processed",
		"data": gin.H{
			"order_id":             orderID,
			"cash_refundable":      result.CashRefundable,
			"gift_points_refunded": result.GiftPointsRefunded,
			"receipt":              receipt,
		},
	})
}
