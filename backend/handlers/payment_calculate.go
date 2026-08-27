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
	PromoPoints   float64 `json:"promo_points"`    // cents (1 点 = 1 分, #1757)
	MaxGiftRatio  float64 `json:"max_gift_ratio"`  // ratio, unchanged
	MaxGiftAmount float64 `json:"max_gift_amount"` // cents
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
	case "refund":
		loadRefundPayment(db, req.ID, &resp)
	case "deposit-refund":
		loadDepositRefund(db, req.ID, &resp)
	case "renewal":
		loadRenewalPayment(db, req.ID, &resp)
	case "payment_shortfall":
		if msg := loadShortfallPayment(db, req.ID, &resp); msg != "" {
			// #1785: explicit error instead of a silent HTTP 200 + Amount:0.
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": msg})
			return
		}
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
		PromoPoints:   float64(user.PromoPoints),
		MaxGiftRatio:  maxGiftRatio,
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
				resp.Amount = v // cents (P3 契约)
			}
		}
	}
	// #1758: Deposit/ShippingFee are Cents — add directly (cents), never
	// ToYuan() (yuan) which mixed units (3 分 + 0.01 元 = 3.01 元 → 多扣).
	resp.Amount += float64(order.Deposit + order.ShippingFee)
	if order.PricingBreakdown != nil && *order.PricingBreakdown != "" {
		resp.Details = map[string]interface{}{
			"pricing_breakdown": *order.PricingBreakdown,
			"deposit":           float64(order.Deposit),
			"shipping_fee":      float64(order.ShippingFee),
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
		paid := models.Cents(0)
		if req.PaidAmount != nil {
			paid = *req.PaidAmount
		}
		// #1758: cents contract (previously ToYuan → yuan mixed units).
		resp.Amount = math.Max(0, float64(newTotal-paid))
		resp.Details = map[string]interface{}{}

		// Old quote (the previously accepted quote, before the requote) for
		// the receipt comparison view (#1577).
		var oldQuote models.RepairQuote
		if err := db.Where("repair_request_id = ? AND id != ? AND status = ?", req.ID, quote.ID, "accepted").
			Order("created_at DESC").First(&oldQuote).Error; err == nil {
			oldTotal := oldQuote.MaterialFee + oldQuote.ServiceFee + oldQuote.LogisticsFee
			resp.Details["old_quote"] = map[string]interface{}{
				"material_fee":  float64(oldQuote.MaterialFee),
				"service_fee":   float64(oldQuote.ServiceFee),
				"logistics_fee": float64(oldQuote.LogisticsFee),
				"total":         float64(oldTotal),
			}
		}
	} else {
		resp.Title = "报修支付"
		// #1758: cents contract (previously ToYuan → yuan).
		resp.Amount = float64(quote.MaterialFee + quote.ServiceFee + quote.LogisticsFee)
		resp.Details = map[string]interface{}{}
	}
	resp.Details["material_fee"] = float64(quote.MaterialFee)
	resp.Details["service_fee"] = float64(quote.ServiceFee)
	resp.Details["logistics_fee"] = float64(quote.LogisticsFee)
	resp.Details["total"] = resp.Amount
}

func loadDamagePayment(db *gorm.DB, id string, resp *PaymentCalculateResponse) {
	var report models.DamageReport
	if err := db.Where("id = ?", id).First(&report).Error; err != nil {
		return
	}
	damageAmount := models.Cents(0)
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

	// #1758: cents contract — damageAmount/Deposit are Cents; the payable
	// excess and every breakdown field stay in cents (frontend /100).
	payAmount := math.Max(0, float64(damageAmount-order.Deposit))
	resp.Title = "定损赔偿"
	resp.Amount = payAmount
	resp.Details = map[string]interface{}{
		"paid_breakdown": map[string]float64{
			"rent_subtotal": rentSubtotal,
			"deposit":       float64(order.Deposit),
			"shipping_fee":  float64(order.ShippingFee),
			"paid_total":    rentSubtotal + float64(order.Deposit) + float64(order.ShippingFee),
		},
		"damage_amount":     float64(damageAmount),
		"deposit_deduction": math.Min(float64(order.Deposit), float64(damageAmount)),
		"pay_amount":        payAmount,
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
	// #1758: cents contract (previously ToYuan → yuan).
	resp.Amount = float64(settlement.CashRefundable)
	details := map[string]interface{}{
		"cash_refundable":  float64(settlement.CashRefundable),
		"prepaid_refunded": float64(settlement.PrepaidRefunded),
		"gift_refunded":    float64(settlement.GiftPointsRefunded),
	}
	// Pass through gift policy info from the settlement breakdown for
	// receipt display: A1 (gift cap at refund time) and pay_ratio (L-06).
	if settlement.Breakdown != "" {
		var bd map[string]interface{}
		if json.Unmarshal([]byte(settlement.Breakdown), &bd) == nil {
			if v, ok := bd["gift_cap"].(float64); ok {
				details["gift_cap"] = v
			}
			if v, ok := bd["pay_ratio"].(float64); ok {
				details["pay_ratio"] = v
			}
		}
	}
	resp.Details = details
}

// loadCancelledOrderRefund serves the refund page after cancel-by-customer:
// the full original payment is refunded via original channels
// (cash → WeChat original-path, prepaid/gift → wallet).
func loadCancelledOrderRefund(db *gorm.DB, id string, resp *PaymentCalculateResponse) {
	var order models.Order
	if err := db.Where("id = ? AND status = ?", id, models.OrderStatusCancelled).First(&order).Error; err != nil {
		return
	}
	// #1758: cents contract — all payment fields stay cents (frontend /100).
	total := float64(order.CashPaid + order.PrepaidPointsUsed + order.GiftPointsUsed)
	resp.Title = "取消订单退款"
	resp.Amount = total
	resp.Details = map[string]interface{}{
		"total_paid":       total,
		"cash_paid":        float64(order.CashPaid),
		"prepaid_used":     float64(order.PrepaidPointsUsed),
		"gift_used":        float64(order.GiftPointsUsed),
		"total_refund":     total,
		"cash_refundable":  float64(order.CashPaid),
		"prepaid_refunded": float64(order.PrepaidPointsUsed),
		"gift_refunded":    float64(order.GiftPointsUsed),
		"cancel_refund":    true,
	}
}

func loadDepositRefund(db *gorm.DB, id string, resp *PaymentCalculateResponse) {
	var order models.Order
	if err := db.Where("id = ?", id).First(&order).Error; err != nil {
		return
	}
	resp.Title = "押金退款"
	// #1758: cents contract (previously ToYuan → yuan).
	resp.Amount = float64(order.Deposit)
	resp.Details = map[string]interface{}{
		"deposit":  float64(order.Deposit),
		"refunded": order.DepositRefunded,
	}
}

func loadRenewalPayment(db *gorm.DB, id string, resp *PaymentCalculateResponse) {
	var record models.OrderPaymentRecord
	if err := db.Where("order_id = ? AND order_type = ? AND type = ?", id, "renewal", "payment").Order("created_at desc").First(&record).Error; err != nil {
		return
	}
	resp.Title = "续期支付"
	// #1758: record.Amount is Cents — pass through as cents (frontend /100).
	resp.Amount = float64(record.Amount)
}

// loadShortfallPayment (#1746/#1748 L-04C 流程 3)：总账补缴支付确认页数据。
// 金额与明细全部来自服务端（补缴记录 + computeSettlement），前端禁止自算。
// #1785: returns a non-empty error message on failure (order/shortfall missing
// or no payable shortfall) so the caller can surface an explicit 40002 instead
// of silently returning HTTP 200 with Amount:0.
func loadShortfallPayment(db *gorm.DB, id string, resp *PaymentCalculateResponse) string {
	var order models.Order
	if err := db.Where("id = ?", id).First(&order).Error; err != nil {
		return "补缴单不存在"
	}
	var record models.OrderPaymentRecord
	if err := db.Where("order_id = ? AND order_type = ? AND status = ?", order.ID, "payment_shortfall", "pending").
		Order("created_at desc").First(&record).Error; err != nil {
		return "补缴单不存在"
	}
	result := computeSettlement(order, db)
	if result.PayableShortfall <= 0 {
		return "无需补缴"
	}
	resp.Title = "补缴差额"
	// #1758: cents contract — record.Amount and settlement fields are Cents;
	// payable shortfall from computeSettlement is yuan → convert to cents.
	resp.Amount = float64(record.Amount)
	resp.Details = map[string]interface{}{
		"shortfall_amount": float64(record.Amount),
		"rent":             result.RentPayable * 100,
		"shipping_fee":     float64(order.ShippingFee),
		"overdue_fee":      result.OverdueChargesTotal * 100,
		"damage_amount":    result.DamageDeducted * 100,
		"paid_total":       float64(order.CashPaid + order.PrepaidPointsUsed + order.GiftPointsUsed),
	}
	return ""
}
