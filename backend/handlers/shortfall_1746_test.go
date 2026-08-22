package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

// #1746 L-04C 总账补缴：应付 > 已付 → 生成补缴记录（pending）+ 订单回退
// 等待补缴；支付回调 → completed + settlement 闭环。

// TestExecuteRefund_ShortfallCreatesPaymentRecord: 优惠码少付/免押金不足 →
// payable_shortfall > 0 → 生成 payment_shortfall 记录 + 订单回退 returning +
// settlement pending。
func TestExecuteRefund_ShortfallCreatesPaymentRecord(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "shortfall-1", Status: "active",
	}).Error)

	// 场景：租期 35 天合同 340 元（30@10 + 5@8），实付 200（不足），
	// 实际 35 天全额使用 → Re=340 > 已付 200 → 补缴 140
	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID:     uuid.New().String(),
		StartDate:        strPtr("2026-08-01"),
		EndDate:          strPtr("2026-09-05"),
		LeaseTerm:        35,
		Status:           models.OrderStatusCompleted,
		ReturnedAt:       timePtr1743(time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)),
		Deposit:          0,
		CashPaid:         models.FromYuan(200),
		PricingBreakdown: strPtr(`{"base_daily_rent":1000,"rent_days":35,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":1000},{"days_max":35,"discount_percent":20,"daily_rate":1000}],"tier_segments":[{"tier":1,"days":30,"rate":1000,"discount":1,"subtotal":30000},{"tier":2,"days":5,"rate":1000,"discount":0.8,"subtotal":4000}],"total_amount":34000}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-SHORTFALL-1", StockStatus: "rented",
	}).Error)

	result, err := executeRefund(db, order)
	require.NoError(t, err)
	require.True(t, result.PayableShortfall > 0, "shortfall must be > 0")
	require.Equal(t, 140.0, result.PayableShortfall)

	// 补缴记录生成（pending）
	var rec models.OrderPaymentRecord
	require.NoError(t, db.Where("order_id = ? AND order_type = ?", order.ID, "payment_shortfall").First(&rec).Error)
	require.Equal(t, "pending", rec.Status)
	require.Equal(t, models.Cents(14000), rec.Amount, "shortfall 140 元 = 14000 分")

	// 订单回退等待补缴（不关单）
	var after models.Order
	require.NoError(t, db.Where("id = ?", order.ID).First(&after).Error)
	require.Equal(t, models.OrderStatusReturning, after.Status, "补缴完成前订单不关单")

	// settlement 保持 pending
	var settlement models.Settlement
	require.NoError(t, db.Where("order_id = ?", order.ID).First(&settlement).Error)
	require.Equal(t, "pending", settlement.RefundStatus)
}

// TestExecuteRefund_ShortfallIdempotent: 同一订单重复结算不重复生成补缴记录。
func TestExecuteRefund_ShortfallIdempotent(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "shortfall-idem", Status: "active",
	}).Error)

	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID:     uuid.New().String(),
		StartDate:        strPtr("2026-08-01"),
		EndDate:          strPtr("2026-09-05"),
		LeaseTerm:        35,
		Status:           models.OrderStatusCompleted,
		ReturnedAt:       timePtr1743(time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)),
		Deposit:          0,
		CashPaid:         models.FromYuan(200),
		PricingBreakdown: strPtr(`{"base_daily_rent":1000,"rent_days":35,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":1000},{"days_max":35,"discount_percent":20,"daily_rate":1000}],"tier_segments":[{"tier":1,"days":30,"rate":1000,"discount":1,"subtotal":30000},{"tier":2,"days":5,"rate":1000,"discount":0.8,"subtotal":4000}],"total_amount":34000}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-SHORTFALL-IDEM", StockStatus: "rented",
	}).Error)

	// 第一次：生成记录 + 回退
	_, err := executeRefund(db, order)
	require.NoError(t, err)

	// 模拟订单再次 completed 后第二次结算（幂等分支）
	var reOrder models.Order
	require.NoError(t, db.Where("id = ?", order.ID).First(&reOrder).Error)
	reOrder.Status = models.OrderStatusCompleted
	_, err = executeRefund(db, reOrder)
	require.NoError(t, err)

	var count int64
	db.Model(&models.OrderPaymentRecord{}).
		Where("order_id = ? AND order_type = ?", order.ID, "payment_shortfall").Count(&count)
	require.Equal(t, int64(1), count, "补缴记录必须幂等（不重复生成）")
}

// TestApplySideEffects_ShortfallClosesLoop: 补缴支付回调 → 订单 completed +
// settlement completed（幂等闭环）。
func TestApplySideEffects_ShortfallClosesLoop(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "shortfall-loop", Status: "active",
	}).Error)

	orderID := uuid.New().String()
	require.NoError(t, db.Create(&models.Order{
		ID: orderID, TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID:     uuid.New().String(),
		StartDate:        strPtr("2026-08-01"),
		EndDate:          strPtr("2026-09-05"),
		LeaseTerm:        35,
		Status:           models.OrderStatusReturning, // 等待补缴状态
		Deposit:          0,
		CashPaid:         models.FromYuan(200),
		PricingBreakdown: strPtr(`{"base_daily_rent":1000,"rent_days":35,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":1000},{"days_max":35,"discount_percent":20,"daily_rate":1000}],"tier_segments":[{"tier":1,"days":30,"rate":1000,"discount":1,"subtotal":30000},{"tier":2,"days":5,"rate":1000,"discount":0.8,"subtotal":4000}],"total_amount":34000}`),
	}).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID,
		SN: "SN-SHORTFALL-LOOP", StockStatus: "available",
	}).Error)

	// 预先存在的 settlement（pending）+ 补缴记录（已付）
	require.NoError(t, db.Create(&models.Settlement{
		ID: uuid.New().String(), OrderID: orderID, RefundStatus: "pending",
	}).Error)
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &orderID, OrderType: "payment_shortfall",
		Amount: models.FromYuan(140), Status: "paid",
	}).Error)

	// 补缴支付回调副作用
	record := models.OrderPaymentRecord{
		ID: uuid.New().String(), OrderType: "payment_shortfall",
		OrderID: &orderID, Amount: models.FromYuan(140), Status: "paid",
	}
	require.NoError(t, applySideEffects(db, &record, time.Now()))

	var after models.Order
	require.NoError(t, db.Where("id = ?", orderID).First(&after).Error)
	require.Equal(t, models.OrderStatusCompleted, after.Status, "补缴完成 → 订单关单")

	var settlement models.Settlement
	require.NoError(t, db.Where("order_id = ?", orderID).First(&settlement).Error)
	require.Equal(t, "completed", settlement.RefundStatus, "settlement 闭环完成")
}

// TestPrepayShortfall_OrderTypeAccepted: prepay 接受 payment_shortfall。
func TestPrepayShortfall_OrderTypeAccepted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testfixtures.SetupWechatPayMock(t)
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	userID := uuid.New().String()
	order := models.Order{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		OrgID:        tenantID,
		UserID:       userID,
		InstrumentID: uuid.New().String(),
		Status:       models.OrderStatusReturning,
	}
	require.NoError(t, db.Create(&order).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/pay/prepay", PrepayOrder)

	// 无 openid（模拟小程序回调）→ prepay 应生成 JSAPI 或 native；这里
	// 仅验证 order_type 被接受（非 40002 invalid order_type）
	body, _ := json.Marshal(map[string]interface{}{
		"order_type": "payment_shortfall",
		"order_id":   order.ID,
		"amount":     140.0,
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/api/pay/prepay", bytes.NewReader(body)))
	require.NotEqual(t, http.StatusBadRequest, w.Code, "payment_shortfall 不得被白名单拒绝")
	// 由于无 openid，会走 native 或报 openid 错误——但不会是 invalid order_type
	require.NotContains(t, w.Body.String(), "invalid order_type")
}

var _ = database.GetDB

// TestShortfallNotification_Structured (#1747 L-04C 流程 2): 补缴通知
// content 含情况说明+明细；action_data 结构化（shortfall_amount/order_id/
// breakdown 全部分）。
func TestShortfallNotification_Structured(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "shortfall-notif", Status: "active",
	}).Error)

	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID:     uuid.New().String(),
		StartDate:        strPtr("2026-08-01"),
		EndDate:          strPtr("2026-09-05"),
		LeaseTerm:        35,
		Status:           models.OrderStatusCompleted,
		ReturnedAt:       timePtr1743(time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)),
		Deposit:          0,
		CashPaid:         models.FromYuan(200),
		ShippingFee:      models.FromYuan(10),
		PricingBreakdown: strPtr(`{"base_daily_rent":1000,"rent_days":35,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":1000},{"days_max":35,"discount_percent":20,"daily_rate":1000}],"tier_segments":[{"tier":1,"days":30,"rate":1000,"discount":1,"subtotal":30000},{"tier":2,"days":5,"rate":1000,"discount":0.8,"subtotal":4000}],"total_amount":34000}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-SHORTFALL-NOTIF", StockStatus: "rented",
	}).Error)

	_, err := executeRefund(db, order)
	require.NoError(t, err)

	var notif models.Notification
	require.NoError(t, db.Where("ref_id = ? AND action_type = ?", order.ID, "payment_shortfall").First(&notif).Error)
	require.Equal(t, "订单需补缴", notif.Title)
	require.Contains(t, notif.Content, "需补缴", "content 含情况说明")
	require.Contains(t, notif.Content, "结算明细", "content 含明细")
	require.NotNil(t, notif.ActionData)

	var ad struct {
		ShortfallAmount int64  `json:"shortfall_amount"`
		OrderID         string `json:"order_id"`
		Breakdown       struct {
			Rent         int64 `json:"rent"`
			ShippingFee  int64 `json:"shipping_fee"`
			OverdueFee   int64 `json:"overdue_fee"`
			DamageAmount int64 `json:"damage_amount"`
			PaidTotal    int64 `json:"paid_total"`
		} `json:"breakdown"`
	}
	require.NoError(t, json.Unmarshal([]byte(*notif.ActionData), &ad))
	require.Equal(t, int64(15000), ad.ShortfallAmount, "shortfall 150 元 = 15000 分")
	require.Equal(t, order.ID, ad.OrderID)
	require.Equal(t, int64(34000), ad.Breakdown.Rent, "租金 340 元（分）")
	require.Equal(t, int64(1000), ad.Breakdown.ShippingFee, "物流 10 元（分）")
	require.Equal(t, int64(20000), ad.Breakdown.PaidTotal, "已付 200 元（分）")
}

// TestRefundReceipt_ThankYouAndPayments (#1747 L-04 流程 5): 退款收据
// 含感谢语 + 收支明细行。
func TestRefundReceipt_ThankYouAndPayments(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()

	order := models.Order{
		ID:                uuid.New().String(),
		TenantID:          tenantID,
		OrgID:             orgID,
		UserID:            userID,
		InstrumentID:      uuid.New().String(),
		CashPaid:          models.FromYuan(100),
		GiftPointsUsed:    0,
		PrepaidPointsUsed: 0,
		ShippingFee:       0,
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &order.ID, OrderType: "rent", Amount: models.FromYuan(100),
		Method: strPtr("jsapi"), Type: "payment", Status: "paid",
	}).Error)

	res := &settlementResult{
		RentPayable:         100,
		TotalRefund:         0,
		CashRefundable:      0,
		GiftPointsRefunded:  0,
		OverdueChargesTotal: 0,
		DamageDeducted:      0,
		ActualDays:          1,
		CashBasis:           100,
		RemainingDeposit:    0,
		Breakdown:           map[string]interface{}{},
	}

	receipt := buildRefundReceipt(db, order, res)
	require.Contains(t, receipt, "感谢您的租赁", "收据含感谢语")
	require.Contains(t, receipt, "实付（jsapi）：¥100.00", "收据含收支明细行")

	ad := buildRefundActionData(db, order, res)
	require.Contains(t, ad, `"order_id"`)
	require.Contains(t, ad, `"payments"`)
	require.Contains(t, ad, `"amount_cents":10000`, "action_data 收支明细为分")
}

// TestCalculatePayment_Shortfall (#1748 L-04C 流程 3): /pay/calculate
// payment_shortfall 返回差额 + 明细（服务端计算）。
func TestCalculatePayment_Shortfall(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "shortfall-calc", Status: "active",
	}).Error)

	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID:     uuid.New().String(),
		StartDate:        strPtr("2026-08-01"),
		EndDate:          strPtr("2026-09-05"),
		LeaseTerm:        35,
		Status:           models.OrderStatusReturning,
		ReturnedAt:       timePtr1743(time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)),
		Deposit:          0,
		CashPaid:         models.FromYuan(200),
		ShippingFee:      models.FromYuan(10),
		PricingBreakdown: strPtr(`{"base_daily_rent":1000,"rent_days":35,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":1000},{"days_max":35,"discount_percent":20,"daily_rate":1000}],"tier_segments":[{"tier":1,"days":30,"rate":1000,"discount":1,"subtotal":30000},{"tier":2,"days":5,"rate":1000,"discount":0.8,"subtotal":4000}],"total_amount":34000}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-SHORTFALL-CALC", StockStatus: "rented",
	}).Error)

	// 生成补缴记录（结算触发）
	_, err := executeRefund(db, order)
	require.NoError(t, err)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/pay/calculate", CalculatePayment)

	body, _ := json.Marshal(map[string]interface{}{
		"type": "payment_shortfall",
		"id":   order.ID,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/pay/calculate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Type    string  `json:"type"`
			Title   string  `json:"title"`
			Amount  float64 `json:"amount"`
			Details struct {
				ShortfallAmount float64 `json:"shortfall_amount"`
				Rent            float64 `json:"rent"`
				PaidTotal       float64 `json:"paid_total"`
			} `json:"details"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Equal(t, "payment_shortfall", resp.Data.Type)
	require.Equal(t, "补缴差额", resp.Data.Title)
	// #1758: calculate output is cents (frontend /100) — 150 元 = 15000 分.
	require.Equal(t, 15000.0, resp.Data.Amount, "差额 150 元 = 15000 分")
	require.Equal(t, 15000.0, resp.Data.Details.ShortfallAmount)
	require.Equal(t, 34000.0, resp.Data.Details.Rent, "租金 340 元 = 34000 分")
	require.Equal(t, 20000.0, resp.Data.Details.PaidTotal, "已付 200 元 = 20000 分")
}
