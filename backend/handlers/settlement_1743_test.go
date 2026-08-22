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

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

// #1743 regression tests for the settlement chain unit contract:
// T2 rent callback writes back the discounted cash_paid,
// T3 overflow days go to the overdue fee (Ca − C), never into rent,
// T4 zero-cash settlements complete instead of staying pending forever,
// T1 payment/refund record amounts are serialized as Cents.

func str1743Ptr(s string) *string { return &s }

func float1743Ptr(f float64) *float64 { return &f }

// TestComputeSettlement_TierOverflowDays (#1743 业务裁决): Re is priced by
// the COVERED days C = ΣC0..Cn (pricing_breakdown.rent_days), NOT by the
// actual days Ca. A lease that outlasts C (Ca=2 vs C=1) does NOT extend the
// tier billing — the excess goes to the overdue fee (Ca−C) at inspection.
func TestComputeSettlement_TierOverflowDays(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()

	returnedAt := time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC) // 47h → ceil = 2 days
	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: uuid.New().String(),
		StartDate:    str1743Ptr("2026-08-01"),
		EndDate:      str1743Ptr("2026-08-02"),
		LeaseTerm:    1,
		Status:       models.OrderStatusReturned,
		ReturnedAt:   &returnedAt,
		Deposit:      0,
		CashPaid:     3600,
		// Snapshot: single 1-day segment at ¥36/day (=3600 cents)
		PricingBreakdown: str1743Ptr(`{"base_daily_rent":3600,"rent_days":1,"tiers":[{"days_max":1,"discount_percent":0,"daily_rate":3600}],"tier_segments":[{"tier":1,"days":1,"rate":3600,"discount":1,"subtotal":3600}],"total_amount":3600}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-TIER-OVF", StockStatus: "rented",
	}).Error)

	result := computeSettlement(order, db)
	require.Equal(t, 2, result.ActualDays)
	require.Equal(t, 36.0, result.RentPayable, "Re is priced by covered days C (1), NOT extended to Ca (2)")
	// #1743 审计: C (cover_days) must be persisted with the calculation.
	require.Equal(t, 1, result.Breakdown["cover_days"], "audit must record C = cover_days")
	require.Equal(t, 2, result.Breakdown["actual_rent_days"], "audit must record Ca")
}

// TestInspectReturn_OverdueDays_CaMinusC (#1743 业务裁决): overdue days =
// Ca − C (covered days), not endDate+1 → scanTime.
func TestInspectReturn_OverdueDays_CaMinusC(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()

	orderID := uuid.New().String()
	instrumentID := uuid.New().String()
	deliveredAt := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	// C = 1 covered day; Ca = ceil(8-01 10:00 → 8-02 23:00 = 37h) = 2 →
	// overdue 1 day.
	require.NoError(t, db.Create(&models.Order{
		ID: orderID, TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: instrumentID, Status: models.OrderStatusReturning,
		StartDate: str1743Ptr("2026-08-01"), EndDate: str1743Ptr("2026-08-02"),
		LeaseTerm: 1, DeliveredAt: &deliveredAt,
		CashPaid:         3600,
		PricingBreakdown: str1743Ptr(`{"base_daily_rent":3600,"rent_days":1,"tier_segments":[{"tier":1,"days":1,"rate":3600,"discount":1,"subtotal":3600}]}`),
	}).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: instrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-OVD-CAMINUSC", StockStatus: "rented",
		Pricing: `{"daily_rent":36,"deposit":0,"overdue_daily_fee":15}`,
	}).Error)

	handler := &WarehouseHandler{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/warehouse/orders/:id/return-inspect", handler.InspectReturn)

	body, _ := json.Marshal(map[string]interface{}{
		"instrument_sn": "SN-OVD-CAMINUSC",
		"scan_time":     time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC),
		"condition":     "good",
		"notes":         "完好归还",
		"photos":        []string{},
	})
	req := httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/return-inspect", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var report models.DamageReport
	require.NoError(t, db.Where("lease_id = ?", orderID).First(&report).Error)
	require.Equal(t, 1, report.OverdueDays, "overdue days = Ca(2) − C(1)")
	require.Equal(t, models.Cents(1500), report.OverdueFee, "overdue fee = Ro(15) × 1 day")
}

// TestExecuteRefund_ZeroCashRefund_CompletesSettlement: nothing to refund →
// settlement.refund_status = completed (never stuck on pending / 处理中).
func TestExecuteRefund_ZeroCashRefund_CompletesSettlement(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "zero-refund", Status: "active",
	}).Error)

	// Rent fully consumed: paid ¥36 for 1 day, returned within the term.
	returnedAt := time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC) // 23h → 1 day
	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID:     uuid.New().String(),
		StartDate:        str1743Ptr("2026-08-01"),
		EndDate:          str1743Ptr("2026-08-02"),
		LeaseTerm:        1,
		Status:           models.OrderStatusCompleted,
		ReturnedAt:       &returnedAt,
		Deposit:          0,
		CashPaid:         3600,
		PricingBreakdown: str1743Ptr(`{"base_daily_rent":3600,"rent_days":1,"tiers":[{"days_max":1,"discount_percent":0,"daily_rate":3600}],"tier_segments":[{"tier":1,"days":1,"rate":3600,"discount":1,"subtotal":3600}],"total_amount":3600}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-ZERO-RFD", StockStatus: "rented",
	}).Error)

	result, err := executeRefund(db, order)
	require.NoError(t, err)
	require.Equal(t, 0.0, result.CashRefundable)

	var settlement models.Settlement
	require.NoError(t, db.Where("order_id = ?", order.ID).First(&settlement).Error)
	require.Equal(t, "completed", settlement.RefundStatus, "zero cash refund must complete the settlement")

	var refundCount int64
	db.Model(&models.OrderRefundRecord{}).Where("order_id = ?", order.ID).Count(&refundCount)
	require.Zero(t, refundCount, "no refund record may be created when there is nothing to refund")
}

// TestApplySideEffects_RentWritesBackCashPaid: the rent callback must write
// the server-recomputed discounted amount into orders.cash_paid (#1743 T2).
func TestApplySideEffects_RentWritesBackCashPaid(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()

	orderID := uuid.New().String()
	instrumentID := uuid.New().String()
	require.NoError(t, db.Create(&models.Order{
		ID: orderID, TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: instrumentID, Status: models.OrderStatusReserved,
		CashPaid: 3600, // pre-order full-price snapshot
	}).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: instrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-CB-WB", StockStatus: "available",
	}).Error)

	// ENO-style discounted actual payment recomputed at prepay: 36 cents.
	record := models.OrderPaymentRecord{
		ID:        uuid.New().String(),
		OrderType: "rent",
		OrderID:   &orderID,
		Amount:    36,
		Status:    "paid",
	}
	require.NoError(t, applySideEffects(db, &record, time.Now()))

	var after models.Order
	require.NoError(t, db.Where("id = ?", orderID).First(&after).Error)
	require.Equal(t, models.OrderStatusPaid, after.Status)
	require.Equal(t, models.Cents(36), after.CashPaid, "cash_paid must equal the discounted actual payment")

	// Waive scenario: 100% discount writes back 0.
	waiveRecord := models.OrderPaymentRecord{
		ID:        uuid.New().String(),
		OrderType: "rent",
		OrderID:   &orderID,
		Amount:    0,
		Status:    "paid",
	}
	require.NoError(t, applySideEffects(db, &waiveRecord, time.Now()))
	var waived models.Order
	require.NoError(t, db.Where("id = ?", orderID).First(&waived).Error)
	require.Equal(t, models.Cents(0), waived.CashPaid, "fully-waived payment writes cash_paid=0")
}

// TestGetOrder_PaymentRecordsCentsContract: payment_records[].amount and
// refund_records[].amount are Cents; the frontends divide by exactly 100.
func TestGetOrder_PaymentRecordsCentsContract(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	userID := uuid.New().String()

	orderID := uuid.New().String()
	require.NoError(t, db.Create(&models.Order{
		ID: orderID, TenantID: tenantID, OrgID: tenantID, UserID: userID,
		InstrumentID: uuid.New().String(), Status: models.OrderStatusPaid,
	}).Error)
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &tenantID, UserID: userID, OrderType: "rent",
		OrderID: &orderID, Amount: 36, Method: str1743Ptr("jsapi"), Status: "paid",
	}).Error)

	router := setupTestRouter(t, tenantID, userID)
	router.GET("/orders/:id", GetOrder)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/orders/"+orderID, nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			PaymentRecords []struct {
				Amount float64 `json:"amount"`
			} `json:"payment_records"`
			RefundRecords []struct {
				Amount float64 `json:"amount"`
			} `json:"refund_records"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Len(t, resp.Data.PaymentRecords, 1)
	require.Equal(t, 36.0, resp.Data.PaymentRecords[0].Amount, "amount must be raw Cents, NOT converted to yuan")
}

// TestSettlement_UserScenarioA_CouponWaiveDepositOnly (#1743 用户验证例 A):
// OREZ 全免租金，实付=押金 10000；实际 10 天 → Re=100，物流 100 →
// 退款 = 10000 − 100 − 100 = 9800。
func TestSettlement_UserScenarioA_CouponWaiveDepositOnly(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "scen-a", Status: "active",
	}).Error)

	// T2 回写后: cash_paid = 实付 10000（押金，OREZ 免租金）, Deposit=10000
	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: uuid.New().String(),
		StartDate:    str1743Ptr("2026-08-01"),
		EndDate:      str1743Ptr("2026-09-05"), // 35 天租期
		LeaseTerm:    35,
		Status:       models.OrderStatusReturned,
		ReturnedAt:   timePtr1743(time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)),
		Deposit:      models.FromYuan(10000),
		CashPaid:     models.FromYuan(10000),
		ShippingFee:  models.FromYuan(100),
		// 阶梯: 30 天 @10 元 + 5 天 @8 元 → 合同租金 340
		PricingBreakdown: str1743Ptr(`{"base_daily_rent":1000,"rent_days":35,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":1000},{"days_max":35,"discount_percent":20,"daily_rate":800}],"tier_segments":[{"tier":1,"days":30,"rate":1000,"discount":1,"subtotal":30000},{"tier":2,"days":5,"rate":1000,"discount":0.8,"subtotal":4000}],"total_amount":34000}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-SCEN-A", StockStatus: "rented",
	}).Error)

	result := computeSettlement(order, db)

	// 实际 10 天 → Re = 10 × 10 = 100
	require.Equal(t, 100.0, result.RentPayable, "Re for 10 actual days")
	require.Equal(t, 9800.0, result.TotalRefund, "refund = 10000 − 100 − 100")
	require.Equal(t, 0.0, result.PayableShortfall)
}

// TestSettlement_UserScenarioB_NoDepositNoCoupon (#1743 用户验证例 B):
// 免押 + 无优惠，实付 340；实际 10 天 → Re=100，物流 100 →
// 退款 = 340 − 100 − 100 = 140。押金 0 时扣项缺口照扣（旧实现漏扣物流）。
func TestSettlement_UserScenarioB_NoDepositNoCoupon(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "scen-b", Status: "active",
	}).Error)

	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID:     uuid.New().String(),
		StartDate:        str1743Ptr("2026-08-01"),
		EndDate:          str1743Ptr("2026-09-05"),
		LeaseTerm:        35,
		Status:           models.OrderStatusReturned,
		ReturnedAt:       timePtr1743(time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)),
		Deposit:          0, // 免押
		CashPaid:         models.FromYuan(340),
		ShippingFee:      models.FromYuan(100),
		PricingBreakdown: str1743Ptr(`{"base_daily_rent":1000,"rent_days":35,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":1000},{"days_max":35,"discount_percent":20,"daily_rate":800}],"tier_segments":[{"tier":1,"days":30,"rate":1000,"discount":1,"subtotal":30000},{"tier":2,"days":5,"rate":1000,"discount":0.8,"subtotal":4000}],"total_amount":34000}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-SCEN-B", StockStatus: "rented",
	}).Error)

	result := computeSettlement(order, db)

	require.Equal(t, 100.0, result.RentPayable)
	require.Equal(t, 140.0, result.TotalRefund, "refund = 340 − 100 − 100 (物流费在押金为 0 时仍全额扣除)")
	require.Equal(t, 0.0, result.PayableShortfall)
}

// TestSettlement_UserScenarioC_Overdue_Shortfall (#1743 用户验证例 C):
// 无优惠实付 340，实际 40 天 → Re 封顶 340（覆盖 35 天），逾期 5×15=75，
// 物流 100 → 应付 515 → 补缴 175（负退款透传）。
func TestSettlement_UserScenarioC_Overdue_Shortfall(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "scen-c", Status: "active",
	}).Error)

	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID:     uuid.New().String(),
		StartDate:        str1743Ptr("2026-08-01"),
		EndDate:          str1743Ptr("2026-09-05"),
		LeaseTerm:        35,
		Status:           models.OrderStatusReturned,
		ReturnedAt:       timePtr1743(time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)), // 实际 40 天
		Deposit:          0,
		CashPaid:         models.FromYuan(340),
		ShippingFee:      models.FromYuan(100),
		PricingBreakdown: str1743Ptr(`{"base_daily_rent":1000,"rent_days":35,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":1000},{"days_max":35,"discount_percent":20,"daily_rate":800}],"tier_segments":[{"tier":1,"days":30,"rate":1000,"discount":1,"subtotal":30000},{"tier":2,"days":5,"rate":1000,"discount":0.8,"subtotal":4000}],"total_amount":34000}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-SCEN-C", StockStatus: "rented",
	}).Error)

	// 验收时按 Ca−C=5 天持久化逾期费 75 元到 damage report（#1743 新口径）
	require.NoError(t, db.Create(&models.DamageReport{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		OrgID:        orgID,
		LeaseID:      order.ID,
		InstrumentID: order.InstrumentID,
		UserID:       userID,
		Condition:    "good",
		Status:       "completed",
		OverdueDays:  5,
		OverdueFee:   models.FromYuan(75),
	}).Error)

	result := computeSettlement(order, db)

	// Re 封顶在覆盖天数 35 → 340（绝不按 40 天展开 = 380）
	require.Equal(t, 340.0, result.RentPayable, "Re caps at covered days C")
	// 应付 = 340 + 75 + 100 = 515；已付 340 → 补缴 175
	require.Equal(t, 0.0, result.TotalRefund, "no refund when shortfall")
	require.Equal(t, 175.0, result.PayableShortfall, "shortfall = 515 − 340")
	require.Equal(t, 75.0, result.OverdueChargesTotal)
}

func timePtr1743(t time.Time) *time.Time { return &t }
