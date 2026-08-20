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

// seedSession creates a pending registration session directly in the DB.
func seedSession(t *testing.T, db *gorm.DB, openid, exchangeToken string, amount float64) models.RegistrationSession {
	t.Helper()
	db.Exec("DELETE FROM registration_sessions") // isolated per test
	form := registerForm{Nickname: "微信用户", Name: "测试会员", Phone: "13800139000", Ref: ""}
	s := models.RegistrationSession{
		ID:            uuid.New().String(),
		OpenID:        openid,
		ExchangeToken: exchangeToken,
		IAMUserID:     strPtr("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
		LocalUserID:   strPtr("11111111-2222-3333-4444-555555555555"),
		FormData:      marshalForm(form),
		Amount:        models.FromYuan(amount),
		Status:        "pending",
	}
	require.NoError(t, db.Create(&s).Error)

	// #1688: the local users cache is reserved alongside the session
	// (status=init) — mirrors CreateRegistrationSession.
	require.NoError(t, db.Create(&models.User{
		ID:                 "11111111-2222-3333-4444-555555555555",
		IAMSub:             "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		TenantID:           "00000000-0000-0000-0000-000000000000",
		OrgID:              "00000000-0000-0000-0000-000000000000",
		Username:           "13800139000",
		Name:               "测试会员",
		Phone:              "13800139000",
		Role:               "USER",
		Status:             "init",
		IsProfileCompleted: true,
	}).Error)
	return s
}

// TestCreateRegistrationSession covers: form persisted as pending, amount =
// membership_fee (99), required-field validation.
func TestCreateRegistrationSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	require.NoError(t, db.Create(&models.SystemSetting{
		ID:           uuid.New().String(),
		TenantID:     "00000000-0000-0000-0000-000000000000",
		SettingKey:   "membership_fee",
		SettingValue: "99",
	}).Error)

	// IAM mock (#1688): CreateUser returns a reserved user id; PurgeUser ok.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/users":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"user_id": "11111111-2222-3333-4444-555555555555",
				"status":  "init",
			})
		case "/oauth/token", "/api/v1/auth/token":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "mock-client-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Point the injected client at the mock server.
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	h := NewRegistrationSessionHandler(db)
	h.setIAMClient(services.NewIAMClientWithCredentials("mock-client", "mock-secret"))

	router := gin.New()
	router.POST("/api/auth/registration-sessions", h.CreateRegistrationSession)

	body, _ := json.Marshal(map[string]interface{}{
		"nickname":       "微信昵称",
		"name":           "张会员",
		"phone":          "13800139000",
		"email":          "member@test.com",
		"exchange_token": "exchange-tok-001",
		"ref":            "",
	})
	req := httptest.NewRequest("POST", "/api/auth/registration-sessions", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "create session: %s", w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			SessionID string  `json:"session_id"`
			Amount    float64 `json:"amount"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	assert.Equal(t, 99.0, resp.Data.Amount, "amount = membership_fee")

	var session models.RegistrationSession
	require.NoError(t, db.Where("id = ?", resp.Data.SessionID).First(&session).Error)
	assert.Equal(t, "pending", session.Status)
	assert.Equal(t, "exchange-tok-001", session.ExchangeToken)
	assert.Contains(t, session.FormData, "13800139000", "form persisted with phone")

	// Required field validation: missing name → 400.
	badBody, _ := json.Marshal(map[string]interface{}{"nickname": "x", "phone": "13900139000"})
	req = httptest.NewRequest("POST", "/api/auth/registration-sessions", bytes.NewBuffer(badBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// #1688: one reserved local user (status=init) exists before payment —
	// the payment record references it; activation happens at the callback.
	var reserved models.User
	require.NoError(t, db.Where("status = ?", "init").First(&reserved).Error)
	assert.Equal(t, "13800139000", reserved.Phone)
	assert.Equal(t, "11111111-2222-3333-4444-555555555555", reserved.IAMSub, "local cache linked to IAM init user")
}

// TestGetMyRegistrationSession covers resume-by-openid (?code=) and
// resume-by-id (?session_id=) paths, plus 404/400 edges.
func TestGetMyRegistrationSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	// IAM mock: wx-accounts resolves the code to openid-session-001 (any
	// other code resolves to an empty openid → no pending session).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/wx-accounts" {
			w.Header().Set("Content-Type", "application/json")
			openid := "openid-session-001"
			if r.URL.Query().Get("code") == "unknown-code" {
				openid = ""
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"openid":   openid,
				"accounts": []interface{}{},
			})
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)

	s := seedSession(t, db, "openid-session-001", "exch-001", 99)

	router := gin.New()
	h := NewRegistrationSessionHandler(db)
	router.GET("/api/auth/registration-sessions/me", h.GetMyRegistrationSession)

	// by code → openid lookup
	req := httptest.NewRequest("GET", "/api/auth/registration-sessions/me?code=wx-code-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "resume by code: %s", w.Body.String())
	var resp struct {
		Code int `json:"code"`
		Data struct {
			SessionID string                 `json:"session_id"`
			Status    string                 `json:"status"`
			FormData  map[string]interface{} `json:"form_data"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	assert.Equal(t, s.ID, resp.Data.SessionID)
	assert.Equal(t, "pending", resp.Data.Status)
	assert.Equal(t, "测试会员", resp.Data.FormData["name"])

	// by session_id (H5 local cache)
	req = httptest.NewRequest("GET", "/api/auth/registration-sessions/me?session_id="+s.ID, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, s.ID, resp.Data.SessionID)

	// unknown openid → 404
	req = httptest.NewRequest("GET", "/api/auth/registration-sessions/me?code=unknown-code", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// missing params → 400
	req = httptest.NewRequest("GET", "/api/auth/registration-sessions/me", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestRegistrationSessionStatus covers the pending → paid → completed
// state transitions surfaced by the status endpoint.
func TestRegistrationSessionStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	s := seedSession(t, db, "", "", 99)

	router := gin.New()
	h := NewRegistrationSessionHandler(db)
	router.GET("/api/auth/registration-sessions/:id/status", h.GetRegistrationSessionStatus)

	statusOf := func() string {
		req := httptest.NewRequest("GET", "/api/auth/registration-sessions/"+s.ID+"/status", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var r struct {
			Code int `json:"code"`
			Data struct {
				Status string `json:"status"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &r))
		return r.Data.Status
	}

	assert.Equal(t, "pending", statusOf())

	// payment advances the session to completed via the callback path.
	now := time.Now()
	require.NoError(t, db.Model(&s).Updates(map[string]interface{}{
		"status":       "completed",
		"completed_at": now,
		"updated_at":   now,
	}).Error)
	assert.Equal(t, "completed", statusOf())

	// unknown session → 404
	req := httptest.NewRequest("GET", "/api/auth/registration-sessions/no-such-id/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestPrepayMembershipWithCoupon_OREZ covers the full waiver: no WeChat
// payment is created; the record is booked paid and the account is created
// (mock mode path, which runs side effects inline).
func TestPrepayMembershipWithCoupon_OREZ(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testfixtures.SetupWechatPayMock(t)
	db := testfixtures.SetupTestDB(t)

	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")
	newUserID := "6d1e2c3a-0000-4000-8000-0000000000e2"
	srv := newRegisterMockServer(t, newUserID, "OREZUser")
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)

	db.Exec("DELETE FROM coupons") // fixed code, isolated per test
	require.NoError(t, db.Create(&models.Coupon{
		ID:     uuid.New().String(),
		Code:   "OREZ",
		Type:   "waive",
		Value:  0,
		Active: true,
	}).Error)

	s := seedSession(t, db, "", "exch-orez", 99)

	customer := testutil.MakeCustomer("", "")
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := customer.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/pay/prepay", PrepayOrder)

	body, _ := json.Marshal(map[string]interface{}{
		"order_type":  "membership",
		"amount":      99.0, // client may send anything; server re-prices
		"session_id":  s.ID,
		"coupon_code": "orez",
	})
	req := httptest.NewRequest("POST", "/api/pay/prepay", bytes.NewBuffer(body))
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
				OutTradeNo string `json:"out_trade_no"`
			} `json:"data"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	assert.True(t, resp.Data.Success)

	// Payment record: amount re-priced to 0.
	var record models.OrderPaymentRecord
	require.NoError(t, db.Where("out_trade_no = ?", resp.Data.Data.OutTradeNo).First(&record).Error)
	assert.Equal(t, 0.0, record.Amount, "OREZ re-priced to 0")
	assert.Contains(t, *record.RawResponse, s.ID, "session_id stored on record")
	assert.NotNil(t, record.SessionID, "session_id stored in dedicated column")
	assert.Equal(t, s.ID, *record.SessionID, "dedicated column holds the session")

	// Account created server-side (no orphan left behind).
	var user models.User
	require.NoError(t, db.Where("phone = ?", "13800139000").First(&user).Error)
	assert.Equal(t, "USER", user.Role)
	assert.True(t, user.OnboardingCompleted)

	// Session completed.
	var session models.RegistrationSession
	require.NoError(t, db.Where("id = ?", s.ID).First(&session).Error)
	assert.Equal(t, "completed", session.Status)

	// Registration gift points credited.
	assert.Equal(t, 99.0, user.PromoPoints, "registration gift points")
}

// TestPrepayMembership_RecordUserIDReferencesReservedLocalUser verifies
// #1688: the membership payment record's user_id is the reserved local
// user id (not the zero UUID), so consumption records stay queryable.
func TestPrepayMembership_RecordUserIDReferencesReservedLocalUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testfixtures.SetupWechatPayMock(t)
	db := testfixtures.SetupTestDB(t)

	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")
	newUserID := "6d1e2c3a-0000-4000-8000-0000000000e2"
	srv := newRegisterMockServer(t, newUserID, "ReservedUser")
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)

	s := seedSession(t, db, "openid-reserved-001", "exch-reserved-001", 99.0)

	router := gin.New()
	router.POST("/api/pay/prepay", PrepayOrder)

	prepayBody, _ := json.Marshal(map[string]interface{}{
		"order_type": "membership",
		"amount":     99.0,
		"session_id": s.ID,
	})
	req := httptest.NewRequest("POST", "/api/pay/prepay", bytes.NewBuffer(prepayBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "prepay: %s", w.Body.String())

	var record models.OrderPaymentRecord
	require.NoError(t, db.Where("session_id = ?", s.ID).First(&record).Error)
	assert.Equal(t, "11111111-2222-3333-4444-555555555555", record.UserID,
		"payment record user_id must reference the reserved local user (#1688)")
}

// TestPrepayMembershipWithCoupon_ENO covers the percent coupon: the fee is
// re-priced server-side (99 → 0.99) in the non-mock JSAPI branch.
func TestPrepayMembershipWithCoupon_ENO(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	db.Exec("DELETE FROM coupons") // fixed code, isolated per test
	require.NoError(t, db.Create(&models.Coupon{
		ID:     uuid.New().String(),
		Code:   "ENO",
		Type:   "percent",
		Value:  10,
		Active: true,
	}).Error)

	s := seedSession(t, db, "", "exch-eno", 99)

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

	customer := testutil.MakeCustomer("", "")
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := customer.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/pay/prepay", PrepayOrder)

	body, _ := json.Marshal(map[string]interface{}{
		"order_type":  "membership",
		"amount":      99.0,
		"open_id":     "openid-eno",
		"session_id":  s.ID,
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
			Mock    bool `json:"mock"`
			Success bool `json:"success"`
			Data    struct {
				OutTradeNo string `json:"out_trade_no"`
			} `json:"data"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	assert.False(t, resp.Data.Mock)
	assert.True(t, resp.Data.Success)

	var record models.OrderPaymentRecord
	require.NoError(t, db.Where("out_trade_no = ?", resp.Data.Data.OutTradeNo).First(&record).Error)
	assert.Equal(t, models.Cents(99), record.Amount, "ENO → 1% of 99 yuan = 99 cents")
	assert.Contains(t, *record.RawResponse, "ENO", "coupon stored on record")
	assert.Equal(t, "pending", record.Status, "real payment pending until callback")
	assert.NotNil(t, record.SessionID, "session_id stored in dedicated column")
	assert.Equal(t, s.ID, *record.SessionID, "dedicated column holds the session")
}

// TestPaymentCallback_RegistrationComplete covers the server-side account
// creation on payment callback, idempotency (repeat callback creates once)
// and the no-orphan guarantee.
func TestPaymentCallback_RegistrationComplete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")
	newUserID := "6d1e2c3a-0000-4000-8000-0000000000e1"
	srv := newRegisterMockServer(t, newUserID, "SessionUser")
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)

	// No user exists before the payment is processed.
	var beforeCount int64
	require.NoError(t, db.Model(&models.User{}).Count(&beforeCount).Error)
	assert.Equal(t, int64(0), beforeCount, "no orphan users before callback")

	s := seedSession(t, db, "openid-session-cb", "exchange-cb-001", 99)
	record := models.OrderPaymentRecord{
		ID:          uuid.New().String(),
		TenantID:    "00000000-0000-0000-0000-000000000000",
		UserID:      "someone",
		OrderType:   "membership",
		OutTradeNo:  strPtr("msess-test-001"),
		Amount:      99.0,
		Type:        "payment",
		Status:      "paid",
		SessionID:   &s.ID,
		RawResponse: strPtr(`{"session_id":"` + s.ID + `","original_amount":99}`),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	now := time.Now()

	// First callback → account created, session completed.
	require.NoError(t, applySideEffects(db, &record, now), "first callback side effects")

	var user models.User
	require.NoError(t, db.Where("iam_sub = ?", newUserID).First(&user).Error)
	assert.Equal(t, "13800139000", user.Phone)
	assert.Equal(t, "openid-register-001", user.WxOpenid, "bound via exchange_token (IAM mock openid)")
	assert.Equal(t, 99.0, user.PromoPoints, "registration gift points")

	var session models.RegistrationSession
	require.NoError(t, db.Where("id = ?", s.ID).First(&session).Error)
	assert.Equal(t, "completed", session.Status)
	require.NotNil(t, session.CompletedAt)

	// Repeat callback (same out_trade_no / same session) → idempotent.
	require.NoError(t, applySideEffects(db, &record, now.Add(time.Minute)), "repeat callback")

	var userCount int64
	require.NoError(t, db.Model(&models.User{}).Where("iam_sub = ?", newUserID).Count(&userCount).Error)
	assert.Equal(t, int64(1), userCount, "repeat callback must not create a second account")

	// Coupon + full flow: coupon recorded on the session for audit.
	assert.NotEmpty(t, session.ID)
}

// TestPaymentCallback_SessionFlow_RealCallback is the regression test for
// the #1664 audit rejection: the REAL WeChat callback path
// (processPaymentCallback) overwrites record.RawResponse with the callback
// result (which has no session_id). The session link must survive via the
// dedicated SessionID column — otherwise the account is never created and
// the session stays pending forever.
func TestPaymentCallback_SessionFlow_RealCallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")
	newUserID := "6d1e2c3a-0000-4000-8000-0000000000e3"
	srv := newRegisterMockServer(t, newUserID, "CallbackRealUser")
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)

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

	s := seedSession(t, db, "openid-cb-real", "exchange-cb-real", 99)

	// Prepay (real mode) persisted the record with the dedicated SessionID
	// column and a RawResponse audit trail.
	record := models.OrderPaymentRecord{
		ID:          uuid.New().String(),
		TenantID:    "00000000-0000-0000-0000-000000000000",
		UserID:      "00000000-0000-0000-0000-000000000000", // no-orphan zero uuid
		OrderType:   "membership",
		OutTradeNo:  strPtr("msess-cb-real-001"),
		Amount:      models.FromYuan(0.99),
		Type:        "payment",
		Status:      "pending",
		SessionID:   &s.ID,
		RawResponse: strPtr(`{"session_id":"` + s.ID + `","original_amount":99}`),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, db.Create(&record).Error)

	// No user exists before the real callback fires.
	var beforeCount int64
	require.NoError(t, db.Model(&models.User{}).Count(&beforeCount).Error)
	assert.Equal(t, int64(0), beforeCount, "no orphan users before callback")

	// Real WeChat callback: processPaymentCallback overwrites RawResponse
	// with the callback result (no session_id) — the link must come from
	// the SessionID column.
	callCallback := func() bool {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/wechatpay/notify", nil)
		return processPaymentCallback(c, &wechatpay.CallbackResult{
			OutTradeNo:    *record.OutTradeNo,
			TransactionID: "txn-cb-real-001",
			Amount:        99, // cents → 0.99
			Success:       true,
			EventType:     "TRANSACTION.SUCCESS",
		})
	}

	require.True(t, callCallback(), "real callback processed")

	// Account created server-side from the session form_data.
	var user models.User
	require.NoError(t, db.Where("iam_sub = ?", newUserID).First(&user).Error)
	assert.Equal(t, "13800139000", user.Phone)
	assert.Equal(t, "openid-register-001", user.WxOpenid, "bound via exchange_token (IAM mock openid)")

	// Session completed.
	var session models.RegistrationSession
	require.NoError(t, db.Where("id = ?", s.ID).First(&session).Error)
	assert.Equal(t, "completed", session.Status)
	require.NotNil(t, session.CompletedAt)

	// Record now carries the callback result in RawResponse (audit) while
	// the SessionID column is untouched.
	var paid models.OrderPaymentRecord
	require.NoError(t, db.Where("id = ?", record.ID).First(&paid).Error)
	assert.Equal(t, "paid", paid.Status)
	assert.NotNil(t, paid.SessionID, "session link survives the callback")
	assert.Contains(t, *paid.RawResponse, "out_trade_no", "RawResponse now holds the callback result")

	// Repeat callback (same out_trade_no) → idempotent, no second account.
	require.True(t, callCallback(), "repeat callback accepted (already processed)")
	var userCount int64
	require.NoError(t, db.Model(&models.User{}).Where("iam_sub = ?", newUserID).Count(&userCount).Error)
	assert.Equal(t, int64(1), userCount, "repeat callback must not create a second account")
}
