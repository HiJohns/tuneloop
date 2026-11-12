package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2307: init 预留清理必须跳过已支付/支付中的 session；409 时可按手机号继续。

type cleanupMock2307 struct {
	srv   *httptest.Server
	purge int32
}

func newCleanupMock2307(t *testing.T) *cleanupMock2307 {
	t.Helper()
	m := &cleanupMock2307{}
	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"access_token": "t2307", "expires_in": 7200, "token_type": "Bearer"})
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			atomic.AddInt32(&m.purge, 1)
			writeJSON(w, map[string]interface{}{"ok": true})
			return
		}
		writeJSON(w, map[string]interface{}{"ok": true})
	})
	m.srv = httptest.NewServer(mux)
	return m
}

func (m *cleanupMock2307) close() { m.srv.Close() }

func TestCleanupStaleInitReservations2307_SkipsPaid(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	m := newCleanupMock2307(t)
	defer m.close()
	services.SetIAMInternalURLForTesting(m.srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)
	// #2336: NewIAMService log.Fatal()s without a namespace; set it per test
	// instead of relying on the cached service from a previous test.
	t.Setenv("IAM_NAMESPACE", "test-ns")

	old := time.Now().Add(-48 * time.Hour)
	mkSession := func(status string, withPayment string) (*models.RegistrationSession, *models.User) {
		iamID := uuid.New().String()
		local := &models.User{
			ID: uuid.New().String(), IAMSub: iamID, TenantID: "00000000-0000-0000-0000-000000000000",
			OrgID: "00000000-0000-0000-0000-000000000000", Username: uuid.New().String(),
			Phone: "13900002307", Status: "init",
		}
		require.NoError(t, db.Create(local).Error)
		s := &models.RegistrationSession{
			ID: uuid.New().String(), IAMUserID: &iamID, LocalUserID: &local.ID,
			FormData: `{"name":"x","phone":"13900002307"}`, Amount: models.FromYuan(99),
			Status: status, CreatedAt: old, UpdatedAt: old,
		}
		require.NoError(t, db.Create(s).Error)
		if withPayment != "" {
			out := "m2307-" + uuid.New().String()[:8]
			rec := &models.OrderPaymentRecord{
				ID: uuid.New().String(), TenantID: "00000000-0000-0000-0000-000000000000",
				UserID: local.ID, SessionID: &s.ID, OrderType: "membership",
				OutTradeNo: &out, Amount: models.FromYuan(99), Status: withPayment, Type: "payment",
			}
			require.NoError(t, db.Create(rec).Error)
		}
		return s, local
	}

	unpaidSession, unpaidUser := mkSession("pending", "")
	paidSession, paidUser := mkSession("failed", "paid")
	payingSession, payingUser := mkSession("pending", "pending")

	cleanupStaleInitReservations(db)

	assert.EqualValues(t, 1, atomic.LoadInt32(&m.purge), "only the unpaid session should be purged")

	var got models.RegistrationSession
	require.NoError(t, db.First(&got, "id = ?", unpaidSession.ID).Error)
	assert.Equal(t, "failed", got.Status)
	var n int64
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", unpaidUser.ID).Count(&n).Error)
	assert.EqualValues(t, 0, n, "unpaid local user should be deleted")

	for _, c := range []struct {
		session *models.RegistrationSession
		user    *models.User
	}{{paidSession, paidUser}, {payingSession, payingUser}} {
		var s models.RegistrationSession
		require.NoError(t, db.First(&s, "id = ?", c.session.ID).Error)
		assert.Equal(t, c.session.Status, s.Status, "paid/paying session status must not change")
		var cnt int64
		require.NoError(t, db.Model(&models.User{}).Where("id = ?", c.user.ID).Count(&cnt).Error)
		assert.EqualValues(t, 1, cnt, "paid/paying local user must be kept")
	}
}

func TestCreateRegistrationSession2307_ResumesInitUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "t2307", "expires_in": 7200})
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "username already exists"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)
	// #2336: NewIAMService log.Fatal()s without a namespace; set it per test
	// instead of relying on the cached service from a previous test.
	t.Setenv("IAM_NAMESPACE", "test-ns")

	iamID := uuid.New().String()
	local := &models.User{
		ID: uuid.New().String(), IAMSub: iamID, TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID: "00000000-0000-0000-0000-000000000000", Username: "13900002308",
		Name: "待完成", Phone: "13900002308", Status: "init",
	}
	require.NoError(t, db.Create(local).Error)
	session := &models.RegistrationSession{
		ID: uuid.New().String(), IAMUserID: &iamID, LocalUserID: &local.ID,
		FormData: `{"name":"待完成","phone":"13900002308"}`, Amount: models.FromYuan(99), Status: "pending",
	}
	require.NoError(t, db.Create(session).Error)

	h := NewRegistrationSessionHandler(db)
	r := gin.New()
	r.POST("/api/auth/registration-sessions", h.CreateRegistrationSession)

	body, _ := json.Marshal(map[string]interface{}{
		"nickname": "待完成", "name": "待完成", "phone": "13900002308", "agreed_terms": true,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/registration-sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var resp struct {
		Code int `json:"code"`
		Data struct {
			ResumeSessionID string `json:"resume_session_id"`
			Action          string `json:"action"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 40901, resp.Code)
	assert.Equal(t, session.ID, resp.Data.ResumeSessionID)
	assert.Equal(t, "pay", resp.Data.Action)
}

func TestCreateRegistrationSession2307_ReusesInitUserWithoutSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "t2307", "expires_in": 7200})
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "username already exists"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)
	// #2336: NewIAMService log.Fatal()s without a namespace; set it per test
	// instead of relying on the cached service from a previous test.
	t.Setenv("IAM_NAMESPACE", "test-ns")

	iamID := uuid.New().String()
	local := &models.User{
		ID: uuid.New().String(), IAMSub: iamID, TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID: "00000000-0000-0000-0000-000000000000", Username: "13900002309",
		Name: "无会话", Phone: "13900002309", Status: "init",
	}
	require.NoError(t, db.Create(local).Error)

	h := NewRegistrationSessionHandler(db)
	r := gin.New()
	r.POST("/api/auth/registration-sessions", h.CreateRegistrationSession)

	body, _ := json.Marshal(map[string]interface{}{
		"nickname": "无会话", "name": "无会话", "phone": "13900002309", "agreed_terms": true,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/registration-sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			SessionID string `json:"session_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.NotEmpty(t, resp.Data.SessionID)

	var session models.RegistrationSession
	require.NoError(t, db.First(&session, "id = ?", resp.Data.SessionID).Error)
	require.NotNil(t, session.IAMUserID)
	assert.Equal(t, iamID, *session.IAMUserID)
	require.NotNil(t, session.LocalUserID)
	assert.Equal(t, local.ID, *session.LocalUserID)

	var userCount int64
	require.NoError(t, db.Model(&models.User{}).Count(&userCount).Error)
	assert.EqualValues(t, 1, userCount, "must reuse the existing local user")
}

func TestCreateRegistrationSession2307_CompletedUserStill409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "t2307", "expires_in": 7200})
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "username already exists"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)
	// #2336: NewIAMService log.Fatal()s without a namespace; set it per test
	// instead of relying on the cached service from a previous test.
	t.Setenv("IAM_NAMESPACE", "test-ns")

	require.NoError(t, db.Create(&models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(), TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID: "00000000-0000-0000-0000-000000000000", Username: "13900002310",
		Name: "已注册", Phone: "13900002310", Status: "active",
	}).Error)

	h := NewRegistrationSessionHandler(db)
	r := gin.New()
	r.POST("/api/auth/registration-sessions", h.CreateRegistrationSession)
	body, _ := json.Marshal(map[string]interface{}{
		"nickname": "已注册", "name": "已注册", "phone": "13900002310", "agreed_terms": true,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/registration-sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 40900, resp.Code)
}

func TestCreateRegistrationSession2307_PaidSessionActionsRebind(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "t2307", "expires_in": 7200})
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "username already exists"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)
	// #2336: NewIAMService log.Fatal()s without a namespace; set it per test
	// instead of relying on the cached service from a previous test.
	t.Setenv("IAM_NAMESPACE", "test-ns")

	iamID := uuid.New().String()
	local := &models.User{ID: uuid.New().String(), IAMSub: iamID, TenantID: "00000000-0000-0000-0000-000000000000", OrgID: "00000000-0000-0000-0000-000000000000", Username: "13900002311", Name: "已支付", Phone: "13900002311", Status: "init"}
	require.NoError(t, db.Create(local).Error)
	session := &models.RegistrationSession{ID: uuid.New().String(), IAMUserID: &iamID, LocalUserID: &local.ID, FormData: `{"name":"已支付","phone":"13900002311"}`, Amount: models.FromYuan(99), Status: "failed"}
	require.NoError(t, db.Create(session).Error)
	out := "m2307-paid"
	rec := &models.OrderPaymentRecord{ID: uuid.New().String(), TenantID: "00000000-0000-0000-0000-000000000000", UserID: local.ID, SessionID: &session.ID, OrderType: "membership", OutTradeNo: &out, Amount: models.FromYuan(99), Status: "paid", Type: "payment"}
	require.NoError(t, db.Create(rec).Error)

	h := NewRegistrationSessionHandler(db)
	r := gin.New()
	r.POST("/api/auth/registration-sessions", h.CreateRegistrationSession)
	body, _ := json.Marshal(map[string]interface{}{"nickname": "已支付", "name": "已支付", "phone": "13900002311", "agreed_terms": true})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/registration-sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Action string `json:"action"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 40901, resp.Code)
	assert.Equal(t, "rebind", resp.Data.Action)
}

func TestCreateRegistrationSession2307_FailedNoPaymentActionsRestart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "t2307", "expires_in": 7200})
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "username already exists"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)
	// #2336: NewIAMService log.Fatal()s without a namespace; set it per test
	// instead of relying on the cached service from a previous test.
	t.Setenv("IAM_NAMESPACE", "test-ns")

	iamID := uuid.New().String()
	local := &models.User{ID: uuid.New().String(), IAMSub: iamID, TenantID: "00000000-0000-0000-0000-000000000000", OrgID: "00000000-0000-0000-0000-000000000000", Username: "13900002312", Name: "失败", Phone: "13900002312", Status: "init"}
	require.NoError(t, db.Create(local).Error)
	session := &models.RegistrationSession{ID: uuid.New().String(), IAMUserID: &iamID, LocalUserID: &local.ID, FormData: `{"name":"失败","phone":"13900002312"}`, Amount: models.FromYuan(99), Status: "failed"}
	require.NoError(t, db.Create(session).Error)

	h := NewRegistrationSessionHandler(db)
	r := gin.New()
	r.POST("/api/auth/registration-sessions", h.CreateRegistrationSession)
	body, _ := json.Marshal(map[string]interface{}{"nickname": "失败", "name": "失败", "phone": "13900002312", "agreed_terms": true})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/registration-sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Action string `json:"action"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 40901, resp.Code)
	assert.Equal(t, "restart", resp.Data.Action)
}
