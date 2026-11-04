package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/services/wechatpay"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2308: 支付成功但 WxBind 失败时：
// 1) order_payment_records.status 必须保持 paid（不能回滚后无限重试）
// 2) side_effect_status=needs_rebind；session=needs_rebind
// 3) POST /auth/registration-sessions/:id/rebind 可用新 token 完成注册

type rebindMock2308 struct {
	srv     *httptest.Server
	bindOK  int32
	bindHit int32
}

func newRebindMock2308(t *testing.T) *rebindMock2308 {
	t.Helper()
	m := &rebindMock2308{}
	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"access_token": "t2308", "expires_in": 7200, "token_type": "Bearer"})
	})
	mux.HandleFunc("/api/v1/auth/wx-bind", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&m.bindHit, 1)
		if atomic.LoadInt32(&m.bindOK) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, map[string]interface{}{"error": "invalid or expired exchange_token"})
			return
		}
		writeJSON(w, map[string]interface{}{"user_id": "u2308", "wx_openid": "openid2308"})
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			writeJSON(w, map[string]interface{}{"ok": true})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/customer-role") {
			writeJSON(w, map[string]interface{}{"ok": true})
			return
		}
		writeJSON(w, map[string]interface{}{"ok": true})
	})
	mux.HandleFunc("/api/v1/perm-version/increment", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"ok": true})
	})
	m.srv = httptest.NewServer(mux)
	return m
}

func (m *rebindMock2308) close() { m.srv.Close() }

func setupPaidRegistration2308(t *testing.T, m *rebindMock2308) (*models.User, *models.RegistrationSession, *models.OrderPaymentRecord) {
	t.Helper()
	db := testfixtures.SetupTestDB(t)
	services.SetIAMInternalURLForTesting(m.srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)

	localUser := &models.User{
		ID: uuid.New().String(), TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID: "00000000-0000-0000-0000-000000000000", Username: "13900002308",
		Name: "待完成注册", Phone: "13900002308", Status: "init",
	}
	require.NoError(t, db.Create(localUser).Error)

	form, _ := json.Marshal(registerForm{Name: "待完成注册", Phone: "13900002308", IDPhotos: map[string]string{}})
	iamUserID := uuid.New().String()
	session := &models.RegistrationSession{
		ID: uuid.New().String(), IAMUserID: &iamUserID, LocalUserID: &localUser.ID,
		FormData: string(form), Amount: models.FromYuan(99), Status: "pending",
		ExchangeToken: "expired-token",
	}
	require.NoError(t, db.Create(session).Error)

	outTradeNo := "membership2308"
	record := &models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: transactionZeroUUID2308(), UserID: localUser.ID,
		SessionID: &session.ID, OrderType: "membership", OutTradeNo: &outTradeNo,
		Amount: models.FromYuan(99), Status: "pending", Type: "payment",
	}
	require.NoError(t, db.Create(record).Error)
	return localUser, session, record
}

func transactionZeroUUID2308() string { return "00000000-0000-0000-0000-000000000000" }

func TestProcessPaymentCallback2308_WxBindExpired_NeedsRebind(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := newRebindMock2308(t)
	defer m.close()
	_, session, record := setupPaidRegistration2308(t, m)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	ok := processPaymentCallback(c, &wechatpay.CallbackResult{
		OutTradeNo: *record.OutTradeNo, TransactionID: "tx2308",
		Amount: int64(record.Amount), Success: true,
	})
	require.True(t, ok, "callback must return success after persisting paid")

	var got models.OrderPaymentRecord
	require.NoError(t, database.GetDB().First(&got, "id = ?", record.ID).Error)
	assert.Equal(t, "paid", got.Status, "paid must survive side-effect failure")
	assert.Equal(t, "needs_rebind", got.SideEffectStatus)
	assert.EqualValues(t, 1, got.SideEffectAttempts)
	require.NotNil(t, got.SideEffectError)

	var gotSession models.RegistrationSession
	require.NoError(t, database.GetDB().First(&gotSession, "id = ?", session.ID).Error)
	assert.Equal(t, "needs_rebind", gotSession.Status)
}

func TestRebindRegistration2308_CompletesAfterFreshToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := newRebindMock2308(t)
	defer m.close()
	_, session, record := setupPaidRegistration2308(t, m)
	// Simulate the callback having persisted paid before side-effect failure.
	require.NoError(t, database.GetDB().Model(&models.OrderPaymentRecord{}).Where("id = ?", record.ID).
		Updates(map[string]interface{}{"status": "paid", "side_effect_status": "needs_rebind"}).Error)

	// First mark session needs_rebind (as the callback failure would).
	require.NoError(t, database.GetDB().Model(&models.RegistrationSession{}).Where("id = ?", session.ID).
		Updates(map[string]interface{}{"status": "needs_rebind", "error": "invalid or expired exchange_token"}).Error)

	atomic.StoreInt32(&m.bindOK, 1)
	h := NewRegistrationSessionHandler(database.GetDB())
	r := gin.New()
	r.POST("/api/auth/registration-sessions/:id/rebind", h.RebindRegistrationSession)

	body, _ := json.Marshal(map[string]string{"exchange_token": "fresh-token"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/registration-sessions/"+session.ID+"/rebind", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var gotSession models.RegistrationSession
	require.NoError(t, database.GetDB().First(&gotSession, "id = ?", session.ID).Error)
	assert.Equal(t, "completed", gotSession.Status)
}
