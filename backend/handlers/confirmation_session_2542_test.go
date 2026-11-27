package handlers

// #2542: executeAction(merchant_admin) must never swallow a failed
// SetUserCustomerPermissions call. A bind that succeeds but leaves the customer
// permission bitmap unwritten produces an owner with no usable permissions, yet
// the old code only logged and returned nil → the confirm/SSO/SMS surfaces
// reported success and the session stayed "confirmed".
//
// Covered here:
//   - permissions failure → rollback unbind (no purge), HTTP 500/50000, session "failed"
//   - success path → no unbind, bitmap bits from the merchant_admin template
//   - compensating unbind failure → explicit CRITICAL log + error carrying both causes
//   - IAM callback receipt → redirect carries status=failed

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"
)

// confirmIAMMock2542 is a minimal fake IAM that records the calls the
// merchant_admin confirmation path makes and lets a test inject failures.
type confirmIAMMock2542 struct {
	mu sync.Mutex

	bindCalls        int
	unbindCalls      int
	cusPermCalls     int
	purgeCalls       int
	permVersionCalls int

	cusPermStatus int // 0 == 200
	unbindStatus  int // 0 == 200

	lastCusPermBody map[string]interface{}
}

func newConfirmIAMMock2542(t *testing.T) *confirmIAMMock2542 {
	t.Helper()
	m := &confirmIAMMock2542{}
	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	statusOf := func(code int) int {
		if code == 0 {
			return http.StatusOK
		}
		return code
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"access_token": "svc-token-2542", "expires_in": 3600})
	})
	mux.HandleFunc("/api/v1/perm-version/increment", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.permVersionCalls++
		m.mu.Unlock()
		writeJSON(w, map[string]interface{}{"code": 20000})
	})
	mux.HandleFunc("/api/v1/organizations/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/customer-permissions"):
			var body map[string]interface{}
			if r.Body != nil {
				_ = json.NewDecoder(r.Body).Decode(&body)
			}
			m.mu.Lock()
			m.cusPermCalls++
			m.lastCusPermBody = body
			status := statusOf(m.cusPermStatus)
			m.mu.Unlock()
			if status != http.StatusOK {
				w.WriteHeader(status)
				writeJSON(w, map[string]string{"error": "permission bitmap unavailable"})
				return
			}
			writeJSON(w, map[string]interface{}{"code": 20000})
		case strings.HasSuffix(r.URL.Path, "/bind"):
			// bind and unbind share one path; the JSON body's action decides.
			var body struct {
				Action string `json:"action"`
			}
			if r.Body != nil {
				_ = json.NewDecoder(r.Body).Decode(&body)
			}
			m.mu.Lock()
			status := http.StatusOK
			if body.Action == "unbind" {
				m.unbindCalls++
				status = statusOf(m.unbindStatus)
			} else {
				m.bindCalls++
			}
			m.mu.Unlock()
			if status != http.StatusOK {
				w.WriteHeader(status)
				writeJSON(w, map[string]string{"error": "unbind unavailable"})
				return
			}
			writeJSON(w, map[string]interface{}{"code": 20000})
		default:
			writeJSON(w, map[string]interface{}{"code": 20000})
		}
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		// #2542 must not purge: the principal is not created by this flow.
		if r.Method == http.MethodDelete || strings.HasSuffix(r.URL.Path, "/purge") {
			m.mu.Lock()
			m.purgeCalls++
			m.mu.Unlock()
		}
		writeJSON(w, map[string]interface{}{"code": 20000})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")
	return m
}

func (m *confirmIAMMock2542) counts() (bind, unbind, cusPerm, purge int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bindCalls, m.unbindCalls, m.cusPermCalls, m.purgeCalls
}

type confirmFixture2542 struct {
	db         *gorm.DB
	tenantID   string
	orgID      string
	merchantID string
	userID     string
	sessionID  string
	token      string
}

func setupConfirmFixture2542(t *testing.T) *confirmFixture2542 {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db := testfixtures.SetupTestDB(t)
	require.NotNil(t, db)
	testutil.SetDB(t, db)

	// The bitmap must come from the real registry, otherwise every bit is 0
	// and the success assertion would be vacuous.
	realRegistry := services.NewPermissionRegistry()
	orig := middleware.PermissionRegistry
	middleware.PermissionRegistry = realRegistry
	t.Cleanup(func() { middleware.PermissionRegistry = orig })

	f := &confirmFixture2542{
		db:        db,
		tenantID:  uuid.NewString(),
		orgID:     uuid.NewString(),
		userID:    uuid.NewString(),
		sessionID: uuid.NewString(),
		token:     "tok-2542-" + uuid.NewString()[:8],
	}
	testfixtures.SeedTenantFK(t, db, f.tenantID)

	require.NoError(t, db.Create(&models.User{
		ID: f.userID, IAMSub: uuid.NewString(), TenantID: f.tenantID, OrgID: f.orgID,
		Username: "u-" + uuid.NewString()[:8], Name: "Pending Admin",
		Email: "admin2542@example.com", Role: "merchant_admin", Status: "pending",
	}).Error)

	f.merchantID = uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: f.merchantID, TenantID: f.tenantID, OrgID: f.orgID,
		Name: "M2542", AdminUID: f.userID, Status: "active",
	}).Error)

	require.NoError(t, db.Create(&models.ConfirmationSession{
		ID: f.sessionID, TenantID: f.tenantID, OrgID: f.orgID,
		UserID:         f.userID,
		ConfirmType:    "email",
		ConfirmTarget:  "admin2542@example.com",
		MerchantID:     f.merchantID,
		ActionType:     "merchant_admin",
		ActionTargetID: f.orgID,
		Status:         "waiting",
		Token:          f.token,
		ExpiresAt:      time.Now().Add(24 * time.Hour),
	}).Error)

	return f
}

func confirmRouter2542(t *testing.T, tenantID string) *gin.Engine {
	t.Helper()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := NewConfirmationSessionHandler()
	r.POST("/api/confirmation-sessions/:id/confirm", h.Confirm)
	r.GET("/api/iam/confirmation-callback", h.IAMConfirmationCallback)
	return r
}

func (f *confirmFixture2542) loadSession(t *testing.T) models.ConfirmationSession {
	t.Helper()
	var s models.ConfirmationSession
	require.NoError(t, f.db.Where("id = ?", f.sessionID).First(&s).Error)
	return s
}

// R1: permission-bitmap failure → rollback the binding (no purge), fail the
// receipt and persist the failure instead of leaving "confirmed" behind.
func TestConfirmationSession2542_CusPermFailureRollsBack(t *testing.T) {
	f := setupConfirmFixture2542(t)
	mock := newConfirmIAMMock2542(t)
	mock.cusPermStatus = http.StatusInternalServerError

	r := confirmRouter2542(t, f.tenantID)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/api/confirmation-sessions/"+f.sessionID+"/confirm?token="+f.token, nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 50000, resp.Code, "CODE=50000")

	session := f.loadSession(t)
	assert.Equal(t, "failed", session.Status, "SESSION_STATUS=failed")
	assert.Contains(t, session.Message, "customer permissions", "SESSION_MESSAGE carries the action error")

	bind, unbind, cusPerm, purge := mock.counts()
	assert.Equal(t, 1, bind, "BIND_CALLS=1")
	assert.Equal(t, 1, cusPerm, "CUSPERM_CALLS=1")
	assert.Equal(t, 1, unbind, "UNBIND_CALLS=1（失败必须回滚绑定）")
	assert.Equal(t, 0, purge, "PURGE_CALLS=0（主体非本流程创建）")

	t.Logf("STATUS=%d CODE=%d SESSION_STATUS=%s BIND_CALLS=%d CUSPERM_CALLS=%d UNBIND_CALLS=%d PURGE_CALLS=%d",
		w.Code, resp.Code, session.Status, bind, cusPerm, unbind, purge)
}

// R2: success path keeps working — confirmed, no unbind, bitmap carries the
// merchant_admin template bits (instrument:create=0, order:read=7).
func TestConfirmationSession2542_SuccessWritesCusPerm(t *testing.T) {
	f := setupConfirmFixture2542(t)
	mock := newConfirmIAMMock2542(t)

	r := confirmRouter2542(t, f.tenantID)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/api/confirmation-sessions/"+f.sessionID+"/confirm?token="+f.token, nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 20000, resp.Code, "CODE=20000")

	session := f.loadSession(t)
	assert.Equal(t, "confirmed", session.Status, "SESSION_STATUS=confirmed")

	bind, unbind, cusPerm, _ := mock.counts()
	assert.Equal(t, 1, bind, "BIND_CALLS=1")
	assert.Equal(t, 1, cusPerm, "CUSPERM_CALLS=1")
	assert.Equal(t, 0, unbind, "UNBIND_CALLS=0（成功路径不得解绑）")

	mock.mu.Lock()
	body := mock.lastCusPermBody
	mock.mu.Unlock()
	require.NotNil(t, body, "customer-permissions body must be captured")
	bits, ok := body["cus_perm"].(float64)
	require.True(t, ok, "cus_perm must be numeric: %#v", body["cus_perm"])
	bit0 := int64(0)
	if int64(bits)&(1<<0) != 0 {
		bit0 = 1
	}
	bit7 := int64(0)
	if int64(bits)&(1<<7) != 0 {
		bit7 = 1
	}
	assert.EqualValues(t, 1, bit0, "CUS_PERM_BIT0=1（instrument:create）")
	assert.EqualValues(t, 1, bit7, "CUS_PERM_BIT7=1（order:read）")
	assert.Equal(t, true, body["raw_bits"], "raw_bits=true")

	t.Logf("STATUS=%d CODE=%d SESSION_STATUS=%s BIND_CALLS=%d CUSPERM_CALLS=%d UNBIND_CALLS=%d CUS_PERM_BIT0=%d CUS_PERM_BIT7=%d",
		w.Code, resp.Code, session.Status, bind, cusPerm, unbind, bit0, bit7)
}

// R3: the compensating unbind failing must be explicit — CRITICAL log plus an
// error message that carries both causes. Never a silent 200.
func TestConfirmationSession2542_CompensationFailureExplicit(t *testing.T) {
	f := setupConfirmFixture2542(t)
	mock := newConfirmIAMMock2542(t)
	mock.cusPermStatus = http.StatusInternalServerError
	mock.unbindStatus = http.StatusInternalServerError

	var logBuf bytes.Buffer
	origOutput := log.Writer()
	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(origOutput) })

	r := confirmRouter2542(t, f.tenantID)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/api/confirmation-sessions/"+f.sessionID+"/confirm?token="+f.token, nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 50000, resp.Code, "CODE=50000")
	assert.Contains(t, resp.Message, "rollback unbind also failed", "COMP_ERROR_PRESENT=1")

	session := f.loadSession(t)
	assert.Equal(t, "failed", session.Status, "SESSION_STATUS=failed")

	_, unbind, _, purge := mock.counts()
	assert.Equal(t, 1, unbind, "UNBIND_CALLS=1")
	assert.Equal(t, 0, purge, "PURGE_CALLS=0")

	logged := logBuf.String()
	assert.Contains(t, logged, "CRITICAL", "CRITICAL_LOGGED=1")

	t.Logf("STATUS=%d CODE=%d SESSION_STATUS=%s UNBIND_CALLS=%d UNBIND_FAILED=1 COMP_ERROR_PRESENT=1 CRITICAL_LOGGED=1 PURGE_CALLS=%d",
		w.Code, resp.Code, session.Status, unbind, purge)
}

// R4: the IAM callback receipt must say status=failed (not status=error&message=)
// and persist the failed session.
func TestConfirmationSession2542_IAMCallbackFailureReceipt(t *testing.T) {
	f := setupConfirmFixture2542(t)
	mock := newConfirmIAMMock2542(t)
	mock.cusPermStatus = http.StatusInternalServerError

	r := confirmRouter2542(t, f.tenantID)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/iam/confirmation-callback?session="+f.sessionID+"&result=accept&confirm_type=email", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusFound, w.Code, "REDIRECT_CODE=302")
	location := w.Header().Get("Location")
	assert.Contains(t, location, "status=failed", "LOCATION_HAS_STATUS_FAILED=1")
	assert.Contains(t, location, "action=merchant_admin", "LOCATION carries the action type")
	assert.NotContains(t, location, "status=success", "must not report success")

	session := f.loadSession(t)
	assert.Equal(t, "failed", session.Status, "SESSION_STATUS=failed")

	_, _, cusPerm, _ := mock.counts()
	assert.Equal(t, 1, cusPerm, "CUSPERM_CALLS=1")

	t.Logf("REDIRECT_CODE=%d LOCATION=%s SESSION_STATUS=%s CUSPERM_CALLS=%d",
		w.Code, location, session.Status, cusPerm)
}
