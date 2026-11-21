package handlers

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
	"gorm.io/gorm"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/services/wechatpay"
	"tuneloop-backend/testutil"
)

// #2327 S3b / D5 / D12：支付闸门（enforce 开关 + 订单级判定）。
// 覆盖三个支付入口共同使用的 creditGateError。

func seedPayGateOrder(t *testing.T, db *gorm.DB, tenantID, userID, tier, creditPath, creditState string) string {
	t.Helper()
	orderID := uuid.New().String()
	require.NoError(t, db.Create(&models.Order{
		ID: orderID, TenantID: tenantID, OrgID: tenantID, UserID: userID,
		InstrumentID:      uuid.New().String(),
		Status:            models.OrderStatusReserved,
		DepositTier:       tier,
		DepositCreditPath: creditPath,
		CreditState:       creditState,
		CashPaid:          models.FromYuan(100),
	}).Error)
	return orderID
}

func newPayGateRouter(t *testing.T, db *gorm.DB, iamSub string) *gin.Engine {
	t.Helper()
	var user models.User
	require.NoError(t, db.Where("iam_sub = ?", iamSub).First(&user).Error)
	actor := testutil.MakeCustomer(user.TenantID, iamSub)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := actor.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/orders/:id/pay", PayOrder)
	return router
}

func postGateJSON(t *testing.T, router *gin.Engine, path string, body interface{}) (int, int) {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp.Code
}

// TestPayGate 覆盖 PayOrder 的 enforce 开关与档位组合（R2）。
func TestPayGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID := uuid.New().String()
	iamSub := uuid.New().String()
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: iamSub, TenantID: tenantID, OrgID: tenantID,
		Username: "paygate", Status: "active",
	}).Error)

	router := newPayGateRouter(t, db, iamSub)

	cases := []struct {
		name     string
		enforce  string
		tier     string
		path     string
		state    string
		wantHTTP int
		wantCode int
	}{
		{"enforce=false 保持旧行为", "false", "reduced", "payscore", "", http.StatusOK, 20000},
		{"enforce=true reduced 未评估拒绝", "true", "reduced", "payscore", "", http.StatusForbidden, 40320},
		{"enforce=true reduced 已拒绝", "true", "reduced", "payscore", "rejected", http.StatusForbidden, 40320},
		{"enforce=true reduced 已通过放行", "true", "reduced", "payscore", "approved", http.StatusOK, 20000},
		{"enforce=true zero(payscore) 未评估拒绝", "true", "zero", "payscore", "", http.StatusForbidden, 40320},
		{"enforce=true zero(payscore) 已通过放行", "true", "zero", "payscore", "approved", http.StatusOK, 20000},
		{"enforce=true 乐器零押金放行", "true", "zero", "", "", http.StatusOK, 20000},
		{"enforce=true full 放行", "true", "full", "payscore", "", http.StatusOK, 20000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PAYSCORE_ENFORCE", tc.enforce)
			orderID := seedPayGateOrder(t, db, tenantID, userID, tc.tier, tc.path, tc.state)
			httpCode, bizCode := postGateJSON(t, router, "/api/orders/"+orderID+"/pay", map[string]interface{}{})
			assert.Equal(t, tc.wantHTTP, httpCode)
			assert.Equal(t, tc.wantCode, bizCode)
			var order models.Order
			require.NoError(t, db.Where("id = ?", orderID).First(&order).Error)
			if tc.wantHTTP == http.StatusForbidden {
				assert.Equal(t, models.OrderStatusReserved, order.Status, "被闸门拒绝的订单不得改为 paid")
			} else {
				assert.Equal(t, models.OrderStatusPaid, order.Status)
			}
		})
	}
}

// TestPrepayGate 覆盖 PrepayOrder 的闸门：拒绝时必须发生在创建支付记录之前，
// 不得留下孤儿支付记录（R2）。
func TestPrepayGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

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

	tenantID := uuid.New().String()
	iamSub := uuid.New().String()
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: iamSub, TenantID: tenantID, OrgID: tenantID,
		Username: "prepaygate", Status: "active",
	}).Error)

	actor := testutil.MakeCustomer(tenantID, iamSub)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := actor.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/pay/prepay", PrepayOrder)

	// 负例：enforce=true 且未评估 → 403，且不得创建支付记录。
	t.Setenv("PAYSCORE_ENFORCE", "true")
	blockedOrder := seedPayGateOrder(t, db, tenantID, userID, "reduced", "payscore", "")
	httpCode, bizCode := postGateJSON(t, router, "/api/pay/prepay", map[string]interface{}{
		"order_type": "rent", "order_id": blockedOrder, "amount": 100.0, "open_id": "openid-x",
	})
	assert.Equal(t, http.StatusForbidden, httpCode)
	assert.Equal(t, 40320, bizCode)
	var blockedRecords int64
	require.NoError(t, db.Model(&models.OrderPaymentRecord{}).
		Where("order_id = ?", blockedOrder).Count(&blockedRecords).Error)
	assert.Zero(t, blockedRecords, "被闸门拒绝时不得留下孤儿支付记录")

	// 正例：enforce=true 且 approved → 放行到建单。
	approvedOrder := seedPayGateOrder(t, db, tenantID, userID, "reduced", "payscore", "approved")
	httpCode, _ = postGateJSON(t, router, "/api/pay/prepay", map[string]interface{}{
		"order_type": "rent", "order_id": approvedOrder, "amount": 100.0, "open_id": "openid-x",
	})
	assert.Equal(t, http.StatusOK, httpCode)

	// 正例：enforce=false 时未评估也放行（保持现有行为）。
	t.Setenv("PAYSCORE_ENFORCE", "false")
	legacyOrder := seedPayGateOrder(t, db, tenantID, userID, "reduced", "payscore", "")
	httpCode, _ = postGateJSON(t, router, "/api/pay/prepay", map[string]interface{}{
		"order_type": "rent", "order_id": legacyOrder, "amount": 100.0, "open_id": "openid-x",
	})
	assert.Equal(t, http.StatusOK, httpCode)
}

// TestCreditPayloadPackage 覆盖 package 契约（R3）：真实 provider 的 package
// 必须透出；mock 模式 package 为空但必须带 mock=true（前端不得当真实凭证拉起）。
func TestCreditPayloadPackage(t *testing.T) {
	expire := time.Now().Add(30 * time.Minute).Truncate(time.Second)
	mockHandler := NewPayscoreHandler(nil, wechatpay.NewMockPayscoreProvider(), &wechatpay.Config{})
	order := &models.Order{
		DepositTier:           models.DepositTierReduced,
		DepositCreditPath:     models.DepositCreditPathPayscore,
		CreditState:           services.CreditStatePending,
		CreditPackage:         "",
		CreditPackageExpireAt: nil,
	}
	payload := mockHandler.creditPayload(order)
	assert.Equal(t, "", payload["credit_package"])
	assert.Nil(t, payload["credit_package_expire_at"])
	assert.Equal(t, true, payload["mock"], "mock 模式必须标注，前端不得拉起真实业务视图")
	assert.Equal(t, true, payload["requires_assessment"])

	realHandler := NewPayscoreHandler(nil, &fakePayscoreProvider{}, &wechatpay.Config{})
	order.CreditPackage = "pkg-real-001"
	order.CreditPackageExpireAt = &expire
	payload = realHandler.creditPayload(order)
	assert.Equal(t, "pkg-real-001", payload["credit_package"])
	assert.Equal(t, expire.Format(time.RFC3339), payload["credit_package_expire_at"])
	assert.Equal(t, false, payload["mock"])

	// 负例：乐器零押金（internal）不应再要求评估。
	freeOrder := &models.Order{DepositTier: models.DepositTierZero, DepositCreditPath: ""}
	assert.Equal(t, false, mockHandler.creditPayload(freeOrder)["requires_assessment"])
}
