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

	if order.Status != models.OrderStatusInLease && order.Status != models.OrderStatusReturning {
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
		PrepaidRefunded:     result.PrepaidRefunded,
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

	if result.PrepaidRefunded > 0 {
		if err := tx.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
			"prepaid_points": gorm.Expr("prepaid_points + ?", result.PrepaidRefunded),
			"updated_at":     time.Now(),
		}).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to refund prepaid points"})
			return
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
			"prepaid_refunded":     result.PrepaidRefunded,
			"gift_points_refunded": result.GiftPointsRefunded,
		},
	})
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
	PrepaidRefunded       float64
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

	var overdueCharges []models.OverdueCharge
	db.Where("order_id = ?", order.ID).Find(&overdueCharges)

	var overdueDays []string
	for _, oc := range overdueCharges {
		overdueDays = append(overdueDays, oc.ChargeDate)
	}

	startDate := parseDate(order.StartDate)
	var overdueDayPositions []int
	if startDate != nil {
		epoch := *startDate
		for _, d := range overdueDays {
			dt, err := time.Parse("2006-01-02", d)
			if err == nil {
				pos := int(dt.Sub(epoch).Hours()/24) + 1
				overdueDayPositions = append(overdueDayPositions, pos)
			}
		}
	}

	rentPayable := 0.0
	actualDays := 0
	if len(tierSegments) > 0 {
		cursor := 1
		for _, seg := range tierSegments {
			segEnd := cursor + seg.Days - 1
			overdueInSeg := 0
			for _, pos := range overdueDayPositions {
				if pos >= cursor && pos <= segEnd {
					overdueInSeg++
				}
			}
			nonOverdueDays := seg.Days - overdueInSeg
			if nonOverdueDays > 0 {
				rentPayable += float64(nonOverdueDays) * seg.Rate * seg.Discount
			}
			actualDays += seg.Days
			cursor = segEnd + 1
		}
	} else {
		if startDate != nil {
			endDate := parseDate(order.EndDate)
			if endDate != nil {
				actualDays = int(endDate.Sub(*startDate).Hours() / 24)
			} else {
				actualDays = int(time.Now().Sub(*startDate).Hours() / 24)
			}
		}
		if actualDays < 1 {
			actualDays = 1
		}
		rentPayable = finalDailyRent * float64(actualDays)
	}
	rentPayable = math.Round(rentPayable*100) / 100

	totalRentPaid := order.CashPaid + order.PrepaidPointsUsed
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

	var totalDepositDeducted float64
	db.Model(&models.OverdueCharge{}).
		Select("COALESCE(SUM(deducted_from_deposit), 0)").
		Where("order_id = ?", order.ID).
		Scan(&totalDepositDeducted)
	remainingDeposit := order.Deposit - totalDepositDeducted
	if remainingDeposit < 0 {
		remainingDeposit = 0
	}

	totalRefund := totalRentPaid + remainingDeposit - damageDeducted - rentPayable
	if totalRefund < 0 {
		totalRefund = 0
	}

	var overdueChargesTotal float64
	db.Model(&models.OverdueCharge{}).
		Select("COALESCE(SUM(remaining_balance), 0)").
		Where("order_id = ? AND status IN ?", order.ID, []string{"failed", "partial"}).
		Scan(&overdueChargesTotal)

	if overdueChargesTotal > 0 {
		if totalRefund >= overdueChargesTotal {
			totalRefund -= overdueChargesTotal
		} else {
			totalRefund = 0
		}
	}

	cashRefundable := math.Min(totalRefund, order.CashPaid)
	prepaidRefunded := totalRefund - cashRefundable

	giftCap := math.Floor(rentPayable * capRate / 100)
	giftPointsRefunded := 0.0
	if order.GiftPointsUsed > giftCap {
		giftPointsRefunded = order.GiftPointsUsed - giftCap
	}

	breakdown := map[string]interface{}{
		"original_total":           order.CashPaid + order.PrepaidPointsUsed + order.GiftPointsUsed,
		"total_rent_paid":          totalRentPaid,
		"deposit":                  order.Deposit,
		"deposit_deducted_overdue": totalDepositDeducted,
		"remaining_deposit":        remainingDeposit,
		"damage_deducted":          damageDeducted,
		"rent_payable":             rentPayable,
		"actual_rent_amount":       rentPayable, // backward-compatible alias
		"actual_rent_days":         actualDays,
		"final_daily_rent":         finalDailyRent,
		"total_refund":             totalRefund,
		"cash_refundable":          cashRefundable,
		"prepaid_refunded":         prepaidRefunded,
		"overdue_charges_total":    overdueChargesTotal,
		"gift_points_used":         order.GiftPointsUsed,
		"gift_cap":                 giftCap,
		"gift_points_refunded":     giftPointsRefunded,
		"cash_paid":                order.CashPaid,
		"prepaid_points_used":      order.PrepaidPointsUsed,
	}

	return settlementResult{
		RentPayable:            rentPayable,
		TotalRentPaid:          totalRentPaid,
		RemainingDeposit:       remainingDeposit,
		DepositDeductedOverdue: totalDepositDeducted,
		DamageDeducted:         damageDeducted,
		TotalRefund:            totalRefund,
		CashRefundable:         cashRefundable,
		PrepaidRefunded:        prepaidRefunded,
		GiftPointsRefunded:     giftPointsRefunded,
		OverdueChargesTotal:    overdueChargesTotal,
		ActualDays:             actualDays,
		Breakdown:              breakdown,
	}
}
