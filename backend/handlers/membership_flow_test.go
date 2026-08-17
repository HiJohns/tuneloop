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
	// Step 2: Prepay membership (mock mode → paid immediately).
	// ------------------------------------------------------------------
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
	})
	req = httptest.NewRequest("POST", "/api/pay/prepay", bytes.NewBuffer(prepayBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	prepayRouter.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "prepay: %s", w.Body.String())

	var prepayResp struct {
		Code int `json:"code"`
		Data struct {
			Mock    bool `json:"mock"`
			Success bool `json:"success"`
			Data    struct {
				OutTradeNo string `json:"out_trade_no"`
			} `json:"data"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &prepayResp))
	require.Equal(t, 20000, prepayResp.Code)
	assert.True(t, prepayResp.Data.Mock, "mock mode active in test env")

	// ------------------------------------------------------------------
	// Step 3: Payment record booked as paid (mock), amount 99.
	// ------------------------------------------------------------------
	var record models.OrderPaymentRecord
	require.NoError(t, db.Where("order_type = ? AND amount = ?", "membership", 99.0).
		Order("created_at desc").First(&record).Error)
	require.Equal(t, "paid", record.Status, "mock mode books payment as paid")
	assert.Equal(t, "membership", record.OrderType)
	assert.Equal(t, 99.0, record.Amount)
	assert.Equal(t, "mock", *record.Method)
	assert.Equal(t, prepayResp.Data.Data.OutTradeNo, *record.OutTradeNo)

	// ------------------------------------------------------------------
	// Step 4: Membership callback → activate level, no error.
	// ------------------------------------------------------------------
	// applySideEffects (membership case) activates the highest level whose
	// MinAmount <= paid amount (#1575).
	require.NoError(t, db.Create(&models.MembershipLevel{
		ID:        1,
		Name:      "初级会员",
		MinAmount: 99,
	}).Error)
	require.NoError(t, db.Create(&models.MembershipLevel{
		ID:        2,
		Name:      "高级会员",
		MinAmount: 199,
	}).Error)

	err := applySideEffects(db, &record, time.Now())
	require.NoError(t, err, "membership payment side effects must not error")

	// Level 1 (99 <= 99) activated, not level 2 (99 < 199).
	var userAfter models.User
	require.NoError(t, db.Where("id = ?", localUser.ID).First(&userAfter).Error)
	require.NotNil(t, userAfter.MembershipLevelID, "membership level set after payment")
	require.Equal(t, 1, *userAfter.MembershipLevelID, "highest level with MinAmount <= 99 activated")

	// Payment record unchanged after side effects (no status flip).
	var after models.OrderPaymentRecord
	require.NoError(t, db.Where("id = ?", record.ID).First(&after).Error)
	assert.Equal(t, "paid", after.Status)
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
		MockMode:        false,
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

// TestPrepayMembership_SessionOpenIDBackfill verifies (#1678) that in the
// two-phase registration flow the prepay handler backfills openid from the
// pending session — the frontend no longer calls /api/wechat/openid.
func TestPrepayMembership_SessionOpenIDBackfill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	rec := &recordingJSAPIClient{}
	wechatpay.ResetGlobalForTesting()
	wechatpay.SetClientForTesting(rec, &wechatpay.Config{
		MockMode:        false,
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
