package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services/wechatpay"
	"tuneloop-backend/testutil"
)

// #1751 regression tests: ENO percent coupon is permille (10‰ = 1%) and
// misconfigured values (outside 1..1000) are rejected.

// TestPrepayCoupon_ENO_Permille10 (#1751): ENO value=10 → 9900 分 × 10‰
// /1000 = 99 分 (¥0.99) — NOT 9 分 (value=1 → ¥0.09, 1/10 short).
func TestPrepayCoupon_ENO_Permille10(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	// SetupTestDB's table list lacks coupons — create it here.
	require.NoError(t, db.Migrator().CreateTable(&models.Coupon{}))

	wechatpay.ResetGlobalForTesting()
	wechatpay.SetClientForTesting(stubJSAPIClient{}, &wechatpay.Config{
		AppID:           "wxcb44a1be70e356ed",
		NotifyURL:       "http://localhost:5553/api/wechatpay/notify",
		RefundNotifyURL: "http://localhost:5553/api/wechatpay/notify",
	})
	t.Cleanup(func() {
		wechatpay.ResetGlobalForTesting()
		testfixtures.SetupWechatPayMock(t)
	})

	require.NoError(t, db.Create(&models.Coupon{
		ID: uuid.New().String(), Code: "ENO", Type: "percent", Value: 10, Active: true,
	}).Error)

	user := models.User{
		ID:       uuid.New().String(),
		IAMSub:   "6d1e2c3a-0000-4000-8000-0000000000f4",
		TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID:    "00000000-0000-0000-0000-000000000000",
		Name:     "ENOUser",
		Role:     "USER",
		Status:   "active",
		WxOpenid: "eno_openid_001",
	}
	require.NoError(t, db.Create(&user).Error)

	router := gin.New()
	customer := testutil.MakeCustomer("", user.ID)
	router.Use(func(c *gin.Context) {
		ctx := customer.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/pay/prepay", PrepayOrder)

	body, _ := json.Marshal(map[string]interface{}{
		"order_type": "membership",
		"amount":     99.0, // ¥99 = 9900 分
		"open_id":    "eno-openid",
		"coupon_code": "ENO",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/pay/prepay", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Payment record amount: 9900 × 10‰ / 1000 = 99 分 (¥0.99).
	var record models.OrderPaymentRecord
	require.NoError(t, db.Where("order_type = ? AND user_id = ?", "membership", user.ID).
		Order("created_at desc").First(&record).Error)
	require.Equal(t, models.Cents(99), record.Amount, "9900 × 10‰ = 99 分 (¥0.99), not 9 分")
}

// TestPrepayCoupon_InvalidPermille_40002 (#1751): percent value outside
// 1..1000‰ is rejected — prevents future misconfiguration.
func TestPrepayCoupon_InvalidPermille_40002(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	require.NoError(t, db.Migrator().CreateTable(&models.Coupon{}))

	wechatpay.ResetGlobalForTesting()
	wechatpay.SetClientForTesting(stubJSAPIClient{}, &wechatpay.Config{
		AppID:           "wxcb44a1be70e356ed",
		NotifyURL:       "http://localhost:5553/api/wechatpay/notify",
		RefundNotifyURL: "http://localhost:5553/api/wechatpay/notify",
	})
	t.Cleanup(func() {
		wechatpay.ResetGlobalForTesting()
		testfixtures.SetupWechatPayMock(t)
	})

	require.NoError(t, db.Create(&models.Coupon{
		ID: uuid.New().String(), Code: "BAD", Type: "percent", Value: 1, Active: true, // 1‰ 非法（应为 ≥1，此处用 1001 测上界；1 是下界合法）
	}).Error)
	require.NoError(t, db.Create(&models.Coupon{
		ID: uuid.New().String(), Code: "BADHI", Type: "percent", Value: 1001, Active: true, // >1000‰ 非法
	}).Error)

	user := models.User{
		ID:       uuid.New().String(),
		IAMSub:   "6d1e2c3a-0000-4000-8000-0000000000f5",
		TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID:    "00000000-0000-0000-0000-000000000000",
		Name:     "BadCouponUser",
		Role:     "USER",
		Status:   "active",
		WxOpenid: "bad_openid_001",
	}
	require.NoError(t, db.Create(&user).Error)

	router := gin.New()
	customer := testutil.MakeCustomer("", user.ID)
	router.Use(func(c *gin.Context) {
		ctx := customer.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/pay/prepay", PrepayOrder)

	body, _ := json.Marshal(map[string]interface{}{
		"order_type": "membership",
		"amount":     99.0,
		"open_id":    "bad-openid",
		"coupon_code": "BADHI",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/pay/prepay", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 40002, resp.Code)
}
