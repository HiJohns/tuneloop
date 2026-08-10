package handlers

import (
	"encoding/json"
	"log"
	"math"
	"net/http"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PaymentCalculateRequest struct {
	Type string `json:"type" binding:"required"`
	ID   string `json:"id"`
}

type PaymentCalculateResponse struct {
	Type    string                 `json:"type"`
	Title   string                 `json:"title"`
	Amount  float64                `json:"amount"`
	Wallet  *WalletInfo            `json:"wallet"`
	Details map[string]interface{} `json:"details,omitempty"`
}

type WalletInfo struct {
	PromoPoints  float64 `json:"promo_points"`
	MaxGiftRatio float64 `json:"max_gift_ratio"`
	MaxGiftAmount float64 `json:"max_gift_amount"`
}

func CalculatePayment(c *gin.Context) {
	var req PaymentCalculateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)
	tenantID := middleware.GetTenantID(ctx)
	db := database.GetDB().WithContext(ctx)

	var resp PaymentCalculateResponse
	resp.Type = req.Type

	wallet, err := getWalletInfo(db, userID, tenantID, 0)
	if err != nil {
		log.Printf("[CalculatePayment] wallet error: %v", err)
	}
	resp.Wallet = wallet

	switch req.Type {
	case "rent":
		loadRentPayment(db, userID, req.ID, &resp)
	case "repair", "requote":
		loadRepairPayment(db, req.ID, req.Type, &resp)
	case "damage":
		loadDamagePayment(db, req.ID, &resp)
	case "points":
		resp.Title = "预付点充值"
	case "refund":
		loadRefundPayment(db, req.ID, &resp)
	case "deposit-refund":
		loadDepositRefund(db, req.ID, &resp)
	case "renewal":
		loadRenewalPayment(db, req.ID, &resp)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid type"})
		return
	}

	if resp.Wallet != nil && resp.Amount > 0 {
		resp.Wallet.MaxGiftAmount = math.Floor(resp.Amount*resp.Wallet.MaxGiftRatio*100) / 100
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": resp})
}

func getWalletInfo(db *gorm.DB, userID, tenantID string, amount float64) (*WalletInfo, error) {
	var user models.User
	if err := db.Where("iam_sub = ?", userID).First(&user).Error; err != nil {
		return nil, err
	}

	// Gift policy per membership level (default row fallback) (#1605, L-05).
	// Old points_policies max_pay_ratio remains a fallback for legacy configs.
	maxGiftRatio := 0.3
	if policy := services.GetGiftPolicyByLevel(db, levelIDOrZero(user.MembershipLevelID)); policy != nil {
		maxGiftRatio = policy.PayRatio
	} else {
		policies, err := queryApplicablePointsPolicies(db, tenantID, "")
		if err == nil && len(policies) > 0 {
			maxGiftRatio = policies[0].MaxPayRatio
		}
	}

	return &WalletInfo{
		PromoPoints:  user.PromoPoints,
		MaxGiftRatio: maxGiftRatio,
		MaxGiftAmount: math.Floor(amount * maxGiftRatio * 100 / 100),
	}, nil
}

// levelIDOrZero dereferences an optional membership level ID.
func levelIDOrZero(id *int) int {
	if id == nil {
		return 0
	}
	return *id
}

func loadRentPayment(db *gorm.DB, userID, id string, resp *PaymentCalculateResponse) {
	var order models.Order
	if err := db.Where("id = ?", id).First(&order).Error; err != nil {
		return
	}
	resp.Title = "租赁支付"
	resp.Amount = 0
	if order.PricingBreakdown != nil && *order.PricingBreakdown != "" {
		var pb map[string]interface{}
		if json.Unmarshal([]byte(*order.PricingBreakdown), &pb) == nil {
			if v, ok := pb["total_amount"].(float64); ok {
				resp.Amount = v
			}
		}
	}
	resp.Amount += order.Deposit + order.ShippingFee
	if order.PricingBreakdown != nil && *order.PricingBreakdown != "" {
		resp.Details = map[string]interface{}{
			"pricing_breakdown": *order.PricingBreakdown,
			"deposit":           order.Deposit,
			"shipping_fee":      order.ShippingFee,
			"total":             resp.Amount,
		}
	}
}

func loadRepairPayment(db *gorm.DB, id, ptype string, resp *PaymentCalculateResponse) {
	var req models.RepairRequest
	if err := db.Where("id = ?", id).First(&req).Error; err != nil {
		return
	}
	if req.AcceptedQuoteID == nil {
		return
	}
	var quote models.RepairQuote
	if err := db.Where("id = ?", req.AcceptedQuoteID).First(&quote).Error; err != nil {
		return
	}
	if ptype == "requote" {
		resp.Title = "报修增补差价"
		newTotal := quote.MaterialFee + quote.ServiceFee + quote.LogisticsFee
		paid := float64(0)
		if req.PaidAmount != nil {
			paid = *req.PaidAmount
		}
		resp.Amount = math.Max(0, newTotal-paid)
		resp.Details = map[string]interface{}{}

		// Old quote (the previously accepted quote, before the requote) for
		// the receipt comparison view (#1577).
		var oldQuote models.RepairQuote
		if err := db.Where("repair_request_id = ? AND id != ? AND status = ?", req.ID, quote.ID, "accepted").
			Order("created_at DESC").First(&oldQuote).Error; err == nil {
			oldTotal := oldQuote.MaterialFee + oldQuote.ServiceFee + oldQuote.LogisticsFee
			resp.Details["old_quote"] = map[string]interface{}{
				"material_fee":  oldQuote.MaterialFee,
				"service_fee":   oldQuote.ServiceFee,
				"logistics_fee": oldQuote.LogisticsFee,
				"total":         oldTotal,
			}
		}
	} else {
		resp.Title = "报修支付"
		resp.Amount = quote.MaterialFee + quote.ServiceFee + quote.LogisticsFee
		resp.Details = map[string]interface{}{}
	}
	resp.Details["material_fee"] = quote.MaterialFee
	resp.Details["service_fee"] = quote.ServiceFee
	resp.Details["logistics_fee"] = quote.LogisticsFee
	resp.Details["total"] = resp.Amount
}

func loadDamagePayment(db *gorm.DB, id string, resp *PaymentCalculateResponse) {
	var report models.DamageReport
	if err := db.Where("id = ?", id).First(&report).Error; err != nil {
		return
	}
	damageAmount := float64(0)
	if report.DamageAmount != nil {
		damageAmount = *report.DamageAmount
	}
	var order models.Order
	if err := db.Where("id = ?", report.LeaseID).First(&order).Error; err != nil {
		return
	}

	// Compute pricing breakdown from order
	pricingBreakdown := make(map[string]interface{})
	if order.PricingBreakdown != nil && *order.PricingBreakdown != "" {
		json.Unmarshal([]byte(*order.PricingBreakdown), &pricingBreakdown)
	}
	rentSubtotal := 0.0
	if pb, ok := pricingBreakdown["total_amount"]; ok {
		if v, ok2 := pb.(float64); ok2 {
			rentSubtotal = v
		}
	}

	payAmount := math.Max(0, damageAmount-order.Deposit)
	resp.Title = "定损赔偿"
	resp.Amount = payAmount
	resp.Details = map[string]interface{}{
		"paid_breakdown": map[string]float64{
			"rent_subtotal":  rentSubtotal,
			"deposit":        order.Deposit,
			"shipping_fee":   order.ShippingFee,
			"paid_total":     rentSubtotal + order.Deposit + order.ShippingFee,
		},
		"damage_amount":    damageAmount,
		"deposit_deduction": math.Min(order.Deposit, damageAmount),
		"pay_amount":       payAmount,
	}
}

func loadRefundPayment(db *gorm.DB, id string, resp *PaymentCalculateResponse) {
	var settlement models.Settlement
	if err := db.Where("id = ?", id).First(&settlement).Error; err != nil {
		// Fallback: cancelled-order refund (cancel-by-customer flow has no
		// settlement record — refund the full original payment).
		loadCancelledOrderRefund(db, id, resp)
		return
	}
	resp.Title = "结算退款"
	resp.Amount = settlement.CashRefundable
	resp.Details = map[string]interface{}{
		"cash_refundable":  settlement.CashRefundable,
		"prepaid_refunded": settlement.PrepaidRefunded,
		"gift_refunded":    settlement.GiftPointsRefunded,
	}
}

// loadCancelledOrderRefund serves the refund page after cancel-by-customer:
// the full original payment is refunded via original channels
// (cash → WeChat original-path, prepaid/gift → wallet).
func loadCancelledOrderRefund(db *gorm.DB, id string, resp *PaymentCalculateResponse) {
	var order models.Order
	if err := db.Where("id = ? AND status = ?", id, models.OrderStatusCancelled).First(&order).Error; err != nil {
		return
	}
	total := order.CashPaid + order.PrepaidPointsUsed + order.GiftPointsUsed
	resp.Title = "取消订单退款"
	resp.Amount = total
	resp.Details = map[string]interface{}{
		"total_paid":        total,
		"cash_paid":         order.CashPaid,
		"prepaid_used":      order.PrepaidPointsUsed,
		"gift_used":         order.GiftPointsUsed,
		"total_refund":      total,
		"cash_refundable":   order.CashPaid,
		"prepaid_refunded":  order.PrepaidPointsUsed,
		"gift_refunded":     order.GiftPointsUsed,
		"cancel_refund":     true,
	}
}

func loadDepositRefund(db *gorm.DB, id string, resp *PaymentCalculateResponse) {
	var order models.Order
	if err := db.Where("id = ?", id).First(&order).Error; err != nil {
		return
	}
	resp.Title = "押金退款"
	resp.Amount = order.Deposit
	resp.Details = map[string]interface{}{
		"deposit":  order.Deposit,
		"refunded": order.DepositRefunded,
	}
}

func loadRenewalPayment(db *gorm.DB, id string, resp *PaymentCalculateResponse) {
	var record models.OrderPaymentRecord
	if err := db.Where("order_id = ? AND order_type = ? AND type = ?", id, "renewal", "payment").Order("created_at desc").First(&record).Error; err != nil {
		return
	}
	resp.Title = "续期支付"
	resp.Amount = record.Amount
}
