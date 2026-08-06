package handlers

import (
	"encoding/json"
	"fmt"
	"log"
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

func parsePointsPolicySnapshot(ppsJSON *string) (map[string]interface{}, float64) {
	result := map[string]interface{}{}
	if ppsJSON == nil || *ppsJSON == "" {
		return result, 0
	}
	json.Unmarshal([]byte(*ppsJSON), &result)
	capRate, _ := result["cap_rate"].(float64)
	return result, capRate
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
			ID:              uuid.New().String(),
			TenantID:        order.TenantID,
			Amount:          result.CashRefundable,
			Reason:          strPtr("租赁结算退款"),
			Status:          "pending",
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
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

	// Increment total spending by actual rental amount
	if err := tx.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"total_spending": gorm.Expr("total_spending + ?", result.RentPayable),
		"updated_at":     time.Now(),
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update total spending"})
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
		RefundMethod:        "prepaid",
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
			ID:              uuid.New().String(),
			TenantID:        order.TenantID,
			Amount:          result.CashRefundable,
			Reason:          strPtr("租赁结算退款"),
			Status:          "pending",
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
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

	// Increment total spending by actual rental amount
	if err := tx.Model(&models.User{}).Where("id = ?", order.UserID).Updates(map[string]interface{}{
		"total_spending": gorm.Expr("total_spending + ?", result.RentPayable),
		"updated_at":     time.Now(),
	}).Error; err != nil {
		return nil, fmt.Errorf("failed to update total spending: %w", err)
	}

	// Loyalty gift points (#1542): order completed → credit gift points
	// proportional to actual rent × level ratio.
	var orderUser models.User
	if err := tx.Where("id = ?", order.UserID).First(&orderUser).Error; err == nil {
		var selfRatio float64
		if orderUser.MembershipLevelID != nil {
			if ratios := services.GetGiftRatios(*orderUser.MembershipLevelID); ratios != nil {
				selfRatio = ratios.SelfSpendRatio
			}
		}
		if selfRatio > 0 {
			loyaltyPoints := math.Floor(result.RentPayable * selfRatio)
			if loyaltyPoints > 0 {
				if err := tx.Model(&models.User{}).Where("id = ?", order.UserID).Updates(map[string]interface{}{
					"promo_points": gorm.Expr("promo_points + ?", loyaltyPoints),
					"updated_at":   time.Now(),
				}).Error; err != nil {
					log.Printf("[executeRefund] loyalty points credit failed for %s: %v", order.UserID, err)
				} else {
					tx.Create(&models.PointsTransaction{
						ID:          uuid.New().String(),
						UserID:      order.UserID,
						TenantID:    order.TenantID,
						Type:        "loyalty",
						Amount:      loyaltyPoints,
						OrderID:     &order.ID,
						Description: fmt.Sprintf("消费返赠点: 租金 ¥%.2f × %.2f%%", result.RentPayable, selfRatio*100),
						CreatedAt:   time.Now(),
					})
				}
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
	t, err := time.Parse("2006-01-02", *s)
	if err != nil {
		return nil
	}
	return &t
}

// settlementResult holds the canonical settlement calculation for an order.
type settlementResult struct {
	RentPayable           float64
	TotalRentPaid         float64
	RemainingDeposit      float64
	DepositDeductedOverdue float64
	DamageDeducted        float64
	TotalRefund           float64
	CashRefundable        float64
	GiftPointsRefunded    float64
	OverdueChargesTotal   float64
	ActualDays            int
	Breakdown             map[string]interface{}
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
	_, capRate := parsePointsPolicySnapshot(order.PointsPolicySnapshot)

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
	// (#1493) and persisted on the DamageAssessment. Legacy per-day
	// overdue_charges are ignored (daily deduction removed, #1492).
	var assessment models.DamageAssessment
	if err := db.Where("order_id = ?", order.ID).Order("created_at desc").First(&assessment).Error; err != nil {
		assessment = models.DamageAssessment{}
	}
	overdueFee := assessment.OverdueFee
	if overdueFee < 0 {
		overdueFee = 0
	}

	startDate := parseDate(order.StartDate)

	rentPayable := 0.0
	actualDays := 0

	// Derive actual lease period: returned_at for returned/completed orders, end_date otherwise
	actualLeaseEnd := parseDate(order.EndDate)
	if order.ReturnedAt != nil && (order.Status == "returned" || order.Status == "completed" || order.Status == "returning") {
		rt := *order.ReturnedAt
		actualLeaseEnd = &rt
	}
	if startDate != nil && actualLeaseEnd != nil {
		actualDays = services.CalculateDays(*startDate, *actualLeaseEnd)
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
	// damage deduction. Both come off the deposit; remainder participates
	// in the refund.
	totalDepositDeducted := overdueFee + damageDeducted
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

	// Refund order (#1537): gift points (over cap) first via promo_points,
	// then remaining cash via order_refund_records. Prepaid points removed.
	cashRefundable := totalRefund
	if cashRefundable < 0 {
		cashRefundable = 0
	}

	giftCap := math.Floor(rentPayable * capRate / 100)
	giftPointsRefunded := 0.0
	if order.GiftPointsUsed > giftCap {
		giftPointsRefunded = order.GiftPointsUsed - giftCap
	}

	breakdown := map[string]interface{}{
		"original_total":           order.CashPaid + order.PrepaidPointsUsed + order.GiftPointsUsed,
		"total_rent_paid":          totalRentPaid,
		"deposit":                  order.Deposit,
		"deposit_deducted_overdue": overdueFee,
		"deposit_deducted_damage":  damageDeducted,
		"remaining_deposit":        remainingDeposit,
		"damage_deducted":          damageDeducted,
		"overdue_fee":              overdueFee,
		"overdue_days":             assessment.OverdueDays,
		"early_return_rebate":      earlyReturnRebate,
		"rent_payable":             rentPayable,
		"actual_rent_amount":       rentPayable, // backward-compatible alias
		"actual_rent_days":         actualDays,
		"final_daily_rent":         finalDailyRent,
		"total_refund":             totalRefund,
		"cash_refundable":          cashRefundable,
		"gift_points_used":         order.GiftPointsUsed,
		"gift_cap":                 giftCap,
		"gift_points_refunded":     giftPointsRefunded,
		"cash_paid":                order.CashPaid,
		"tier_segments":            tierSegments,
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
		Breakdown:              breakdown,
	}
}
