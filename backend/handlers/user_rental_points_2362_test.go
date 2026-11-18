package handlers

// #2362 Rev.2（A1/B1/D4）：支付端乐币额度与足额收款。
// R1 快照整数比；R2 cap 与 SettlementCalculator 同源；R3 越界/负数拒绝；
// R4 乐币覆盖押金（首付不足）拒绝。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"
)

func TestPointsRatioSnapshot_2362(t *testing.T) {
	cases := []struct {
		in       float64
		num, den int64
	}{
		{0.3, 3, 10},
		{0.25, 1, 4},
		{0.5, 1, 2},
		{1, 1, 1},
		{0.335, 67, 200},
		{0, 0, 1},
	}
	for _, tc := range cases {
		num, den, ok := pointsRatioFromDecimal(tc.in)
		require.True(t, ok, "ratio %v must parse", tc.in)
		assert.EqualValues(t, tc.num, num, "ratio %v num", tc.in)
		assert.EqualValues(t, tc.den, den, "ratio %v den", tc.in)
	}

	// R1 断言：券后租金 100000 分 + 3/10 的 cap（整数运算）
	capPoints, err := services.CalcPointsCap(100000, 3, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 300, capPoints, "floor(100000*3/(100*10))")
}

func TestPointsCapParity_2362(t *testing.T) {
	// R2：券 30000、券后租金 250000、900 点；支付端与 Calculator 必须完全一致。
	calc := services.NewSettlementCalculator()
	res, err := calc.Calculate(services.SettlementInput{
		SettlementID: "s-2362", OrderID: "o-2362", SettlementVersion: 1,
		RatioNum: 3, RatioDen: 10, RatioSource: services.RatioSourceSnapshot,
		DailyRateCents: 10000,
		PricingTiers:   []services.PricingTier{{DaysMax: 30, DiscountPercent: 0}, {DaysMax: -1, DiscountPercent: 0}},
		ActualDays:     28, CouponDiscountCents: 30000,
		GiftPointsUsedStoredCents:    90000,
		CashPaidCents:                210000,
		RentCashPaidNetCents:         160000,
		DepositCashCollectedNetCents: 50000,
		DepositHeldCents:             50000,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 750, res.GiftPointsCap)

	payCap, err := services.CalcPointsCap(res.RAfterCouponCents, 3, 10)
	require.NoError(t, err)
	assert.Equal(t, res.GiftPointsCap, payCap, "payment cap must equal calculator cap")

	// 基数不含押金：若把押金 50000 计入，cap 会变成 900（错误值）。
	withDeposit, err := services.CalcPointsCap(res.RAfterCouponCents+50000, 3, 10)
	require.NoError(t, err)
	assert.NotEqual(t, int64(750), withDeposit, "deposit must not enter the cap base")
}

func TestPointsUsageLimits_2362(t *testing.T) {
	// R3：cap=750（250000*3/1000）；用满 75000 分通过，超 1 点拒绝。
	if _, err := validatePointsUsage(250000, 75000, 3, 10); err != nil {
		t.Fatalf("exact cap must pass: %v", err.Message)
	}
	_, over := validatePointsUsage(250000, 75100, 3, 10)
	require.NotNil(t, over)
	assert.EqualValues(t, 40002, over.Code)

	_, negative := validatePointsUsage(250000, -100, 3, 10)
	require.NotNil(t, negative)
	assert.EqualValues(t, 40002, negative.Code)

	_, frac := validatePointsUsage(250000, 75050, 3, 10)
	require.NotNil(t, frac, "fractional points must be rejected")
}

func TestInsufficientPointsPayment_2362(t *testing.T) {
	// R4：全额比例 1/1、券后租金 100000（=1000 点）、押金 50000；
	// 用 1200 点会侵入押金 → 首付不足；支付端必须在建单前拒绝（400，不落 orders）。
	// 注：ratio<=1 时 cap<=券后租金，因此该场景由 cap 校验拦下（而不是靠后续现金流）。
	_, err := validatePointsUsage(100000, 120000, 1, 1)
	require.NotNil(t, err, "points covering deposit must be rejected")
	assert.EqualValues(t, 40002, err.Code)
}

// ---- Rev.3：入口级（POST /api/user/orders）证据 ----

func newPointsOrderRouter2362(t *testing.T) (*gin.Engine, string, string, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	testfixtures.SetupWechatPayMock(t)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	instID := uuid.New().String()

	baseRate := 100.0
	deposit := 500.0
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID, StockStatus: models.StockStatusAvailable,
		BaseDailyRate: models.ToCentsPtr(&baseRate), Deposit: models.ToCentsPtr(&deposit),
		Pricing: `{"daily_rent":100.0,"deposit":500.0}`,
	}).Error)
	require.NoError(t, db.Where("is_system_default = ?", true).Delete(&models.PricingTemplate{}).Error)
	require.NoError(t, db.Create(&models.PricingTemplate{
		ID: uuid.New().String(), Code: "default-2362", Name: "Default2362",
		IsSystemDefault: true, IsActive: true,
		ConfigSchema: `{"tiers":[{"days_max":30,"discount_percent":0},{"days_max":365,"discount_percent":20}],"deposit_ratio":0}`,
	}).Error)
	testutil.SeedVerifiedCustomer(t, db, tenantID, orgID, userID)
	require.NoError(t, db.Create(&models.PointsPolicy{
		ID: uuid.New().String(), ScopeType: "system", MaxPayRatio: 0.3, IsActive: true,
	}).Error)
	require.NoError(t, db.Create(&models.PointBatch{
		ID: uuid.New().String(), UserID: userID, SourceType: "manual",
		AmountCents: models.FromYuan(2000), RemainingCents: models.FromYuan(2000),
		AcquiredAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)

	customer := testutil.MakeCustomer("", userID)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := customer.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := NewUserRentalHandler()
	router.POST("/api/user/orders", h.CreateOrder)

	cleanup := func() {}
	return router, instID, userID, cleanup
}

func postOrder2362(t *testing.T, r *gin.Engine, instID string, body map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/user/orders", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

func TestOrderSnapshotRatio_2362(t *testing.T) {
	r, instID, _, _ := newPointsOrderRouter2362(t)
	code, resp := postOrder2362(t, r, instID, map[string]interface{}{
		"instrument_id": instID, "start_date": "2026-07-01", "end_date": "2026-07-30", "rent_days": 30,
	})
	require.Equal(t, http.StatusCreated, code, "resp=%v", resp)
	data := resp["data"].(map[string]interface{})
	orderID := data["order_id"].(string)
	var order models.Order
	require.NoError(t, testfixtures.GetTestDB(t).Where("id = ?", orderID).First(&order).Error)
	require.NotNil(t, order.PointsPolicySnapshot)
	var snap map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(*order.PointsPolicySnapshot), &snap))
	assert.EqualValues(t, 3, int64(snap["ratio_num"].(float64)))
	assert.EqualValues(t, 10, int64(snap["ratio_den"].(float64)))
	assert.Equal(t, "points_policy", snap["source"])
}

func TestOrderRejectsBadPoints_2362(t *testing.T) {
	r, instID, _, _ := newPointsOrderRouter2362(t)
	db := testfixtures.GetTestDB(t)
	var before int64
	require.NoError(t, db.Model(&models.Order{}).Count(&before).Error)

	cases := []map[string]interface{}{
		{"gift_points_used": 901},
		{"gift_points_used": -1},
		{"gift_points_used": 900.5},
	}
	for _, extra := range cases {
		body := map[string]interface{}{"instrument_id": instID, "start_date": "2026-07-01", "end_date": "2026-07-30", "rent_days": 30}
		for k, v := range extra {
			body[k] = v
		}
		code, resp := postOrder2362(t, r, instID, body)
		assert.Equal(t, http.StatusBadRequest, code, "body=%v resp=%v", body, resp)
	}
	var after int64
	require.NoError(t, db.Model(&models.Order{}).Count(&after).Error)
	assert.Equal(t, before, after, "rejected points must not create orders")
}

func TestOrderAmountConsistency_2362(t *testing.T) {
	r, instID, _, _ := newPointsOrderRouter2362(t)
	db := testfixtures.GetTestDB(t)
	require.NoError(t, db.Create(&models.DiscountPolicy{
		ID: uuid.New().String(), Name: "P2362", RentDiscount: 0.9, IsActive: true,
	}).Error)
	policyID := func() string {
		var p models.DiscountPolicy
		require.NoError(t, db.Where("name = ?", "P2362").First(&p).Error)
		return p.ID
	}()
	require.NoError(t, db.Create(&models.DiscountCode{
		ID: uuid.New().String(), Code: "CPN2362", PolicyID: policyID, IsActive: true,
	}).Error)

	code, resp := postOrder2362(t, r, instID, map[string]interface{}{
		"instrument_id": instID, "start_date": "2026-07-01", "end_date": "2026-07-30", "rent_days": 30,
		"discount_code": "CPN2362", "gift_points_used": 800,
	})
	require.Equal(t, http.StatusCreated, code, "resp=%v", resp)
	data := resp["data"].(map[string]interface{})
	orderID := data["order_id"].(string)
	var order models.Order
	require.NoError(t, db.Where("id = ?", orderID).First(&order).Error)
	// rent 30d×100 = 300000; coupon 10% → 270000; gift 800 points = 80000; deposit 50000.
	assert.EqualValues(t, 80000, order.GiftPointsUsed)
	assert.EqualValues(t, 50000, order.Deposit)
	assert.EqualValues(t, 240000, order.CashPaid, "cash = rentAfterCoupon - points + deposit")
}
