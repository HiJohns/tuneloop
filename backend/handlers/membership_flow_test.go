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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/services/wechatpay"
	"tuneloop-backend/testutil"
)

// TestMembershipFlow covers TC #1553: membership registration fee.
// Register → membership_fee returned → prepay (order_type=membership) in
// mock mode → payment record booked as paid. Membership payment has no
// side effects on activation (gift points are credited at registration).
func TestMembershipFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testfixtures.SetupWechatPayMock(t)
	db := testfixtures.SetupTestDB(t)

	// Membership fee setting: 99 (default, made explicit).
	require.NoError(t, db.Create(&models.SystemSetting{
		ID:           uuid.New().String(),
		TenantID:     "00000000-0000-0000-0000-000000000000",
		SettingKey:   "membership_fee",
		SettingValue: "99",
	}).Error)

	// ------------------------------------------------------------------
	// Step 1: Register → membership_fee returned.
	// ------------------------------------------------------------------
	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")

	newUserID := "6d1e2c3a-0000-4000-8000-0000000000d1"
	srv := newRegisterMockServer(t, newUserID, "MembershipUser")
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)

	registerBody, _ := json.Marshal(map[string]interface{}{
		"name":     "Membership User",
		"nickname": "会员用户",
		"phone":    "13800138000",
		"password": "secret123",
		"wx_code":  "mem-wx-code",
	})
	req := httptest.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(registerBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router := gin.New()
	router.POST("/api/auth/register", NewAuthHandler(db).PostRegister)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "register: %s", w.Body.String())

	var registerResp struct {
		Code int `json:"code"`
		Data struct {
			AccessToken   string  `json:"access_token"`
			MembershipFee float64 `json:"membership_fee"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &registerResp))
	require.Equal(t, 20000, registerResp.Code)
	assert.Equal(t, 99.0, registerResp.Data.MembershipFee, "membership_fee from system_settings")

	// Local user created (needed for prepay user resolution).
	var localUser models.User
	require.NoError(t, db.Where("iam_sub = ?", newUserID).First(&localUser).Error)

	// ------------------------------------------------------------------
	// Step 2: Prepay membership (real JSAPI path with stubbed client).
	// ------------------------------------------------------------------
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
	customer := testutil.MakeCustomer("", localUser.ID)
	prepayRouter := gin.New()
	prepayRouter.Use(func(c *gin.Context) {
		ctx := customer.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	prepayRouter.POST("/api/pay/prepay", PrepayOrder)

	prepayBody, _ := json.Marshal(map[string]interface{}{
		"order_type": "membership",
		"amount":     99.0,
		"open_id":    "mem-openid",
	})
	req = httptest.NewRequest("POST", "/api/pay/prepay", bytes.NewBuffer(prepayBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	prepayRouter.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "prepay: %s", w.Body.String())

	var prepayResp struct {
		Code int `json:"code"`
		Data struct {
			Success bool `json:"success"`
			Data    struct {
				OutTradeNo string `json:"out_trade_no"`
				PrepayID   string `json:"prepay_id"`
			} `json:"data"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &prepayResp))
	require.Equal(t, 20000, prepayResp.Code)
	assert.True(t, prepayResp.Data.Success, "prepay success flag")
	require.NotEmpty(t, prepayResp.Data.Data.PrepayID, "real JSAPI prepay returns prepay_id")

	// ------------------------------------------------------------------
	// Step 3: Payment record booked as pending (real flow), amount 99.
	// ------------------------------------------------------------------
	var record models.OrderPaymentRecord
	require.NoError(t, db.Where("order_type = ? AND amount = ?", "membership", 9900).
		Order("created_at desc").First(&record).Error)
	require.Equal(t, "pending", record.Status, "real flow books payment as pending")
	assert.Equal(t, "membership", record.OrderType)
	assert.Equal(t, models.Cents(9900), record.Amount)
	assert.Equal(t, "jsapi", *record.Method)
	assert.Equal(t, prepayResp.Data.Data.OutTradeNo, *record.OutTradeNo)

	// ------------------------------------------------------------------
	// Step 4: Membership callback → activate level, no error.
	// ------------------------------------------------------------------
	// applySideEffects (membership case) activates the highest level whose
	// MinAmount <= paid amount (#1575).
	require.NoError(t, db.Create(&models.MembershipLevel{
		ID:        1,
		Name:      "初级会员",
		MinAmount: 9900,
	}).Error)
	require.NoError(t, db.Create(&models.MembershipLevel{
		ID:        2,
		Name:      "高级会员",
		MinAmount: 19900,
	}).Error)

	err := applySideEffects(db, &record, time.Now())
	require.NoError(t, err, "membership payment side effects must not error")

	// Level 1 (99 <= 99) activated, not level 2 (99 < 199).
	var userAfter models.User
	require.NoError(t, db.Where("id = ?", localUser.ID).First(&userAfter).Error)
	require.NotNil(t, userAfter.MembershipLevelID, "membership level set after payment")
	require.Equal(t, 1, *userAfter.MembershipLevelID, "highest level with MinAmount <= 9900 cents activated")

	// Payment record unchanged after side effects (no status flip).
	var after models.OrderPaymentRecord
	require.NoError(t, db.Where("id = ?", record.ID).First(&after).Error)
	assert.Equal(t, "pending", after.Status)
}

// Ensure unused imports compile even when the test DB is skipped.
var _ = context.Background()
var _ = uuid.New

// stubJSAPIClient drives the non-mock JSAPI branch of PrepayOrder without
// real WeChat credentials (#1656 membership prepay regression guard).
type stubJSAPIClient struct{}

func (stubJSAPIClient) CreateJSAPIOrder(_ context.Context, p wechatpay.JSAPIParams) (*wechatpay.JSAPIResult, error) {
	return &wechatpay.JSAPIResult{
		PrepayID:  "stub_prepay_id_001",
		Package:   "prepay_id=stub_prepay_id_001",
		TimeStamp: "1750000000",
		NonceStr:  "stub_nonce",
		SignType:  "RSA",
		Sign:      "stub_sign",
	}, nil
}
func (stubJSAPIClient) CreateNativeOrder(context.Context, wechatpay.NativeParams) (*wechatpay.NativeResult, error) {
	return &wechatpay.NativeResult{CodeURL: "stub"}, nil
}
func (stubJSAPIClient) CreateH5Order(context.Context, wechatpay.H5Params) (*wechatpay.H5Result, error) {
	return &wechatpay.H5Result{}, nil
}
func (stubJSAPIClient) QueryOrder(context.Context, string) (*wechatpay.QueryResult, error) {
	return &wechatpay.QueryResult{TradeState: "SUCCESS"}, nil
}
func (stubJSAPIClient) CloseOrder(context.Context, string) error { return nil }
func (stubJSAPIClient) Refund(context.Context, wechatpay.RefundParams) (*wechatpay.RefundResult, error) {
	return &wechatpay.RefundResult{}, nil
}
func (stubJSAPIClient) QueryRefund(context.Context, string) (*wechatpay.RefundResult, error) {
	return &wechatpay.RefundResult{}, nil
}
func (stubJSAPIClient) VerifyPaymentCallback(context.Context, []byte, string, string, string, string) (*wechatpay.CallbackResult, error) {
	return &wechatpay.CallbackResult{}, nil
}

// TestPrepayMembership_NonMock verifies the membership branch of PrepayOrder
// (#1656): in non-mock mode the request must reach the JSAPI switch arm and
// return a prepay_id — previously the missing branch produced an empty 200
// (silent no-op on the client's "发起支付" button in production).
func TestPrepayMembership_NonMock(t *testing.T) {
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

	user := models.User{
		ID:       uuid.New().String(),
		IAMSub:   "6d1e2c3a-0000-4000-8000-0000000000d2",
		TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID:    "00000000-0000-0000-0000-000000000000",
		Name:     "NonMockMember",
		Phone:    "13800138001",
		Role:     "USER",
		Status:   "active",
	}
	require.NoError(t, db.Create(&user).Error)

	customer := testutil.MakeCustomer("", user.ID)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := customer.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/pay/prepay", PrepayOrder)

	prepayBody, _ := json.Marshal(map[string]interface{}{
		"order_type": "membership",
		"amount":     99.0,
		"open_id":    "mock_openid",
	})
	req := httptest.NewRequest("POST", "/api/pay/prepay", bytes.NewBuffer(prepayBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "prepay: %s", w.Body.String())
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Mock    bool `json:"mock"`
			Success bool `json:"success"`
			Data    struct {
				PrepayID string `json:"prepay_id"`
			} `json:"data"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	assert.False(t, resp.Data.Mock, "non-mock mode")
	assert.True(t, resp.Data.Success)
	assert.Equal(t, "stub_prepay_id_001", resp.Data.Data.PrepayID,
		"membership must reach the JSAPI switch arm and return prepay_id")
}

// recordingJSAPIClient records the last JSAPI params for openid backfill
// assertions (#1678).
type recordingJSAPIClient struct {
	lastParams wechatpay.JSAPIParams
}

func (r *recordingJSAPIClient) CreateJSAPIOrder(_ context.Context, p wechatpay.JSAPIParams) (*wechatpay.JSAPIResult, error) {
	r.lastParams = p
	return &wechatpay.JSAPIResult{
		PrepayID:  "stub_prepay_id_002",
		Package:   "prepay_id=stub_prepay_id_002",
		TimeStamp: "1750000001",
		NonceStr:  "stub_nonce",
		SignType:  "RSA",
		Sign:      "stub_sign",
	}, nil
}
func (r *recordingJSAPIClient) CreateNativeOrder(context.Context, wechatpay.NativeParams) (*wechatpay.NativeResult, error) {
	return &wechatpay.NativeResult{CodeURL: "stub"}, nil
}
func (r *recordingJSAPIClient) CreateH5Order(context.Context, wechatpay.H5Params) (*wechatpay.H5Result, error) {
	return &wechatpay.H5Result{}, nil
}
func (r *recordingJSAPIClient) QueryOrder(context.Context, string) (*wechatpay.QueryResult, error) {
	return &wechatpay.QueryResult{TradeState: "SUCCESS"}, nil
}
func (r *recordingJSAPIClient) CloseOrder(context.Context, string) error { return nil }
func (r *recordingJSAPIClient) Refund(context.Context, wechatpay.RefundParams) (*wechatpay.RefundResult, error) {
	return &wechatpay.RefundResult{}, nil
}
func (r *recordingJSAPIClient) QueryRefund(context.Context, string) (*wechatpay.RefundResult, error) {
	return &wechatpay.RefundResult{}, nil
}
func (r *recordingJSAPIClient) VerifyPaymentCallback(context.Context, []byte, string, string, string, string) (*wechatpay.CallbackResult, error) {
	return &wechatpay.CallbackResult{}, nil
}

// TestPrepayRent_OpenIDBackfillFromLocalUser verifies #1684: rent prepay
// without open_id backfills it from the local users cache (wx_openid) so the
// weapp payment reaches the JSAPI branch (prepay_id) instead of silently
// falling into the PC-only Native QR branch.
func TestPrepayRent_OpenIDBackfillFromLocalUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	rec := &recordingJSAPIClient{}
	wechatpay.ResetGlobalForTesting()
	wechatpay.SetClientForTesting(rec, &wechatpay.Config{
		AppID:           "wxcb44a1be70e356ed",
		NotifyURL:       "http://localhost:5553/api/wechatpay/notify",
		RefundNotifyURL: "http://localhost:5553/api/wechatpay/notify",
	})
	t.Cleanup(func() {
		wechatpay.ResetGlobalForTesting()
		testfixtures.SetupWechatPayMock(t)
	})

	user := models.User{
		ID:       uuid.New().String(),
		IAMSub:   "11111111-2222-4333-8444-555555555555",
		TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID:    "00000000-0000-0000-0000-000000000000",
		Name:     "RentUser",
		Phone:    "13800138001",
		Role:     "USER",
		Status:   "active",
		WxOpenid: "rent_openid_from_local_001",
	}
	require.NoError(t, db.Create(&user).Error)

	// Production shape: the JWT sub is the IAM user UUID (user.IAMSub), which
	// differs from the local users.id. Inject the IAM sub so the backfill query
	// must match via iam_sub (regression guard for the prod incident 2026-08-18
	// where id= matched nothing and rent prepay fell back to Native QR).
	customer := testutil.MakeCustomer("", user.IAMSub)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := customer.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/pay/prepay", PrepayOrder)

	prepayBody, _ := json.Marshal(map[string]interface{}{
		"order_id":   uuid.New().String(),
		"order_type": "rent",
		"amount":     0.02,
	})
	req := httptest.NewRequest("POST", "/api/pay/prepay", bytes.NewBuffer(prepayBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "prepay: %s", w.Body.String())
	assert.Equal(t, "rent_openid_from_local_001", rec.lastParams.OpenID,
		"openid must be backfilled from local users.wx_openid (#1684)")
}

// TestPrepayMembership_SessionOpenIDBackfill verifies (#1678) that in the
// two-phase registration flow the prepay handler backfills openid from the
// pending session — the frontend no longer calls /api/wechat/openid.
func TestPrepayMembership_SessionOpenIDBackfill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	rec := &recordingJSAPIClient{}
	wechatpay.ResetGlobalForTesting()
	wechatpay.SetClientForTesting(rec, &wechatpay.Config{
		AppID:           "wxcb44a1be70e356ed",
		NotifyURL:       "http://localhost:5553/api/wechatpay/notify",
		RefundNotifyURL: "http://localhost:5553/api/wechatpay/notify",
	})
	t.Cleanup(func() {
		wechatpay.ResetGlobalForTesting()
		testfixtures.SetupWechatPayMock(t)
	})

	session := models.RegistrationSession{
		ID:       uuid.New().String(),
		OpenID:   "session_openid_backfill_001",
		FormData: `{"name":"T","phone":"13800138002"}`,
		Amount:   99.0,
		Status:   "pending",
	}
	require.NoError(t, db.Create(&session).Error)

	router := gin.New()
	router.POST("/api/pay/prepay", PrepayOrder)

	// No open_id in the request: must be backfilled from the session.
	prepayBody, _ := json.Marshal(map[string]interface{}{
		"order_type": "membership",
		"amount":     99.0,
		"session_id": session.ID,
	})
	req := httptest.NewRequest("POST", "/api/pay/prepay", bytes.NewBuffer(prepayBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "prepay: %s", w.Body.String())
	assert.Equal(t, "session_openid_backfill_001", rec.lastParams.OpenID,
		"openid must be backfilled from the registration session")
}

// TestPrepayRentWithCoupon_OREZ verifies #1719: the waive coupon (OREZ) is
// generalised to all payment types — a rent prepay with OREZ re-prices to 0
// and books the record paid (method=waived) with side effects, without
// calling WeChat Pay.
func TestPrepayRentWithCoupon_OREZ(t *testing.T) {
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

	db.Exec("DELETE FROM coupons") // fixed code, isolated per test
	require.NoError(t, db.Create(&models.Coupon{
		ID:     uuid.New().String(),
		Code:   "OREZ",
		Type:   "waive",
		Value:  0,
		Active: true,
	}).Error)

	user := models.User{
		ID:       uuid.New().String(),
		IAMSub:   "6d1e2c3a-0000-4000-8000-0000000000e9",
		TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID:    "00000000-0000-0000-0000-000000000000",
		Name:     "RentOREZ",
		Phone:    "13800138009",
		Role:     "USER",
		Status:   "active",
	}
	require.NoError(t, db.Create(&user).Error)

	order := models.Order{
		ID:           uuid.New().String(),
		TenantID:     "00000000-0000-0000-0000-000000000000",
		OrgID:        user.OrgID,
		UserID:       user.ID,
		InstrumentID: uuid.New().String(),
		Status:       models.OrderStatusReserved,
		CashPaid:     models.FromYuan(1),
	}
	require.NoError(t, db.Create(&order).Error)

	customer := testutil.MakeCustomer("", user.IAMSub)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := customer.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/pay/prepay", PrepayOrder)

	body, _ := json.Marshal(map[string]interface{}{
		"order_id":    order.ID,
		"order_type":  "rent",
		"amount":      100.0,
		"coupon_code": "OREZ",
	})
	req := httptest.NewRequest("POST", "/api/pay/prepay", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "prepay: %s", w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Success bool `json:"success"`
			Data    struct {
				OutTradeNo string `json:"out_trade_no"`
			} `json:"data"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	assert.True(t, resp.Data.Success)

	// Payment record re-priced to 0 and booked paid (waived).
	var record models.OrderPaymentRecord
	require.NoError(t, db.Where("out_trade_no = ?", resp.Data.Data.OutTradeNo).First(&record).Error)
	assert.Equal(t, models.Cents(0), record.Amount, "OREZ re-priced rent to 0")
	assert.Equal(t, "paid", record.Status)
	assert.Equal(t, "waived", *record.Method)

	// Side effects applied: order → paid.
	var orderAfter models.Order
	require.NoError(t, db.Where("id = ?", order.ID).First(&orderAfter).Error)
	assert.Equal(t, models.OrderStatusPaid, orderAfter.Status, "rent waive side effects mark order paid")
}

// TestPrepayRentWithCoupon_ENO verifies #1719: the percent coupon (ENO) is
// generalised to all payment types — a rent prepay with ENO re-prices the
// amount to 1% and reaches the real JSAPI branch (prepay_id).
func TestPrepayRentWithCoupon_ENO(t *testing.T) {
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

	db.Exec("DELETE FROM coupons")
	require.NoError(t, db.Create(&models.Coupon{
		ID:     uuid.New().String(),
		Code:   "ENO",
		Type:   "percent",
		Value:  10,
		Active: true,
	}).Error)

	user := models.User{
		ID:       uuid.New().String(),
		IAMSub:   "6d1e2c3a-0000-4000-8000-0000000000ea",
		TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID:    "00000000-0000-0000-0000-000000000000",
		Name:     "RentENO",
		Phone:    "13800138010",
		Role:     "USER",
		Status:   "active",
		WxOpenid: "rent_eno_openid_001",
	}
	require.NoError(t, db.Create(&user).Error)

	customer := testutil.MakeCustomer("", user.IAMSub)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := customer.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/pay/prepay", PrepayOrder)

	body, _ := json.Marshal(map[string]interface{}{
		"order_id":    uuid.New().String(),
		"order_type":  "rent",
		"amount":      100.0,
		"coupon_code": "ENO",
	})
	req := httptest.NewRequest("POST", "/api/pay/prepay", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "prepay: %s", w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Data struct {
				OutTradeNo string `json:"out_trade_no"`
				PrepayID   string `json:"prepay_id"`
			} `json:"data"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.NotEmpty(t, resp.Data.Data.PrepayID, "ENO rent prepay reaches real JSAPI branch")

	// Payment record re-priced to 1% of 100 = 1.0, pending real payment.
	var record models.OrderPaymentRecord
	require.NoError(t, db.Where("out_trade_no = ?", resp.Data.Data.OutTradeNo).First(&record).Error)
	assert.Equal(t, models.Cents(100), record.Amount, "ENO re-priced rent to 1% of 100 yuan = 100 cents")
	assert.Equal(t, "pending", record.Status)
	assert.Equal(t, "jsapi", *record.Method)
}
