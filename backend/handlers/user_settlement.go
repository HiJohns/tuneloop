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

	// Parse tier segments from pricing_breakdown
	var tierSegments []services.TierSegment
	if order.PricingBreakdown != nil && *order.PricingBreakdown != "" {
		var pb struct {
			TierSegments []services.TierSegment `json:"tier_segments"`
		}
		if err := json.Unmarshal([]byte(*order.PricingBreakdown), &pb); err == nil {
			tierSegments = pb.TierSegments
		}
	}

	// Get overdue charges
	var overdueCharges []models.OverdueCharge
	db.Where("order_id = ?", orderID).Find(&overdueCharges)

	var overdueDays []string
	for _, oc := range overdueCharges {
		overdueDays = append(overdueDays, oc.ChargeDate)
	}

	// Compute rent payable excluding overdue days (tier-based)
	rentPayable := 0.0
	startDate := parseDate(order.StartDate)
	var overdueDayPositions []int
	if startDate != nil {
		epoch := *startDate
		for _, d := range overdueDays {
			dt, err := time.Parse("2006-01-02", d)
			if err == nil {
				pos := int(dt.Sub(epoch).Hours() / 24) + 1 // 1-indexed day number
				overdueDayPositions = append(overdueDayPositions, pos)
			}
		}
	}

	if len(tierSegments) > 0 {
		cursor := 1 // current day position (1-indexed)
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
			cursor = segEnd + 1
		}
	} else {
		// Fallback: flat rate
		actualDays := 0
		if startDate != nil {
			endDate := parseDate(order.EndDate)
			if endDate != nil {
				actualDays = int(endDate.Sub(*startDate).Hours() / 24)
			} else {
				now := time.Now()
				actualDays = int(now.Sub(*startDate).Hours() / 24)
			}
		}
		if actualDays < 1 {
			actualDays = 1
		}
		rentPayable = finalDailyRent * float64(actualDays)
	}
	rentPayable = math.Round(rentPayable*100) / 100

	// Total amount customer paid (rent only, excluding deposit)
	totalRentPaid := order.CashPaid + order.PrepaidPointsUsed
	if totalRentPaid == 0 {
		if order.PricingBreakdown != nil && *order.PricingBreakdown != "" {
			var pb map[string]interface{}
			if json.Unmarshal([]byte(*order.PricingBreakdown), &pb) == nil {
				if v, ok := pb["total_amount"].(float64); ok {
					totalRentPaid = v
				}
			}
		}
	}

	// refund calculation: R = totalRentPaid + deposit - Dd - rentPayable
	// Dd = deposit_deducted from DamageReport (provided in query or 0)
	var damageDeducted float64
	var report models.DamageReport
	if err := db.Where("lease_id = ?", orderID).First(&report).Error; err == nil {
		damageDeducted = report.DepositDeducted
	}

	totalRefund := totalRentPaid + order.Deposit - damageDeducted - rentPayable
	if totalRefund < 0 {
		totalRefund = 0
	}

	var overdueChargesTotal float64
	db.Model(&models.OverdueCharge{}).
		Select("COALESCE(SUM(remaining_balance), 0)").
		Where("order_id = ? AND status IN ?", orderID, []string{"failed", "partial"}).
		Scan(&overdueChargesTotal)

	// Deduct overdue charges total from refund
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
		"original_total":         order.CashPaid + order.PrepaidPointsUsed + order.GiftPointsUsed,
		"total_rent_paid":       totalRentPaid,
		"deposit":               order.Deposit,
		"damage_deducted":       damageDeducted,
		"rent_payable":          rentPayable,
		"total_refund":          totalRefund,
		"cash_refundable":       cashRefundable,
		"prepaid_refunded":      prepaidRefunded,
		"overdue_charges_total": overdueChargesTotal,
		"gift_points_used":      order.GiftPointsUsed,
		"gift_cap":              giftCap,
		"gift_points_refunded":  giftPointsRefunded,
		"cash_paid":             order.CashPaid,
		"prepaid_points_used":   order.PrepaidPointsUsed,
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": breakdown,
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

	_, finalDailyRent, _ := parsePricingBreakdown(order.PricingBreakdown)
	_, capRate := parsePointsPolicySnapshot(order.PointsPolicySnapshot)

	startDate := parseDate(order.StartDate)
	endDate := parseDate(order.EndDate)
	actualDays := 0
	if startDate != nil && endDate != nil {
		actualDays = int(endDate.Sub(*startDate).Hours() / 24)
	} else if startDate != nil {
		actualDays = int(time.Now().Sub(*startDate).Hours() / 24)
	}
	if actualDays < 1 {
		actualDays = 1
	}

	actualRentAmount := finalDailyRent * float64(actualDays)
	giftCap := math.Floor(actualRentAmount * capRate / 100)
	giftPointsRefunded := 0.0
	if order.GiftPointsUsed > giftCap {
		giftPointsRefunded = order.GiftPointsUsed - giftCap
	}

	totalPaid := order.CashPaid + order.PrepaidPointsUsed
	if totalPaid == 0 {
		rentPaid := 0.0
		if order.PricingBreakdown != nil && *order.PricingBreakdown != "" {
			var pb map[string]interface{}
			if json.Unmarshal([]byte(*order.PricingBreakdown), &pb) == nil {
				if v, ok := pb["total_amount"].(float64); ok {
					rentPaid = v
				}
			}
		}
		totalPaid = rentPaid + order.Deposit + order.ShippingFee
	}
	totalRefund := 0.0
	if totalPaid > actualRentAmount {
		totalRefund = totalPaid - actualRentAmount
	}
	cashRefundable := math.Min(totalRefund, order.CashPaid)
	prepaidRefunded := totalRefund - cashRefundable

	var overdueChargesTotal float64
	db.Model(&models.OverdueCharge{}).
		Select("COALESCE(SUM(remaining_balance), 0)").
		Where("order_id = ? AND status IN ?", orderID, []string{"failed", "partial"}).
		Scan(&overdueChargesTotal)

	breakdownJSON, _ := json.Marshal(map[string]interface{}{
		"original_total":       order.CashPaid + order.PrepaidPointsUsed + order.GiftPointsUsed,
		"total_paid":           totalPaid,
		"actual_rent_amount":   actualRentAmount,
		"actual_rent_days":     actualDays,
		"final_daily_rent":     finalDailyRent,
		"gift_points_used":     order.GiftPointsUsed,
		"gift_cap":             giftCap,
		"gift_points_refunded": giftPointsRefunded,
		"total_refund":         totalRefund,
		"cash_refundable":      cashRefundable,
		"prepaid_refunded":     prepaidRefunded,
		"cash_paid":            order.CashPaid,
		"prepaid_points_used":  order.PrepaidPointsUsed,
	})

	tx := db.Begin()

	settlement := models.Settlement{
		ID:                 uuid.New().String(),
		OrderID:            orderID,
		ActualRentDays:     actualDays,
		ActualRentAmount:   actualRentAmount,
		OriginalRentAmount: totalPaid + order.GiftPointsUsed,
		GiftPointsRefunded: giftPointsRefunded,
		CashRefundable:     cashRefundable,
		PrepaidRefunded:    prepaidRefunded,
		RefundMethod:       req.RefundMethod,
		RefundStatus:       "pending",
		OverdueChargesTotal: overdueChargesTotal,
		Breakdown:          string(breakdownJSON),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if err := tx.Create(&settlement).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create settlement"})
		return
	}

	if giftPointsRefunded > 0 {
		var user models.User
		if err := tx.Where("id = ?", userID).First(&user).Error; err == nil {
			if err := tx.Model(&user).Updates(map[string]interface{}{
				"promo_points": gorm.Expr("promo_points + ?", giftPointsRefunded),
				"updated_at":   time.Now(),
			}).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to refund gift points"})
				return
			}
		}
	}

	if prepaidRefunded > 0 {
		if err := tx.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
			"prepaid_points": gorm.Expr("prepaid_points + ?", prepaidRefunded),
			"updated_at":     time.Now(),
		}).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to refund prepaid points"})
			return
		}
	}

	// Cash refund via WeChat Pay (if cash was paid on this order)
	if cashRefundable > 0 {
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
			Amount:          cashRefundable,
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
			result, err := client.Refund(c.Request.Context(), wechatpay.RefundParams{
				OutTradeNo:   outTradeNo,
				OutRefundNo:  outRefundNo,
				TotalAmount:  cfg.AmountToCents(paymentRecord.Amount),
				RefundAmount: cfg.AmountToCents(cashRefundable),
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
				refundRecord.RefundID = &result.RefundID
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
		"total_spending": gorm.Expr("total_spending + ?", actualRentAmount),
		"updated_at":     time.Now(),
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update total spending"})
		return
	}

	if err := tx.Model(&models.Order{}).Where("id = ?", orderID).Update("status", models.OrderStatusReturned).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update order status"})
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
			"settlement_id":      settlement.ID,
			"cash_refundable":    cashRefundable,
			"prepaid_refunded":   prepaidRefunded,
			"gift_points_refunded": giftPointsRefunded,
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
