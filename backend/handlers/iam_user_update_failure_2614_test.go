package handlers

// #2614：`IAMClient.UpdateUser` 的两处后端调用点（`UpdateIAMUser` / `UpdateCurrentUser`）
// 必须把上游失败映射成 #2598 的四类三元组，而不是把持久失败压成可重试（500/50000）
// 或把瞬时故障压成数据冲突（409/40900）。
//
// 矩阵：上游 400 / 401 / 403 / 404 / 409(email) / 409(phone) / 409(username) /
// 409(unknown) / 500 / 不可达。断言 `(status, code, message)` 三元组 + 负例
// （持久类不含「请稍后再试」、瞬时类不得 409、message 不含上游原始串 50000）+
// 失败时本地缓存未被改写。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"
)

// rawMarker 出现在上游正文里：任何响应 message 含它即视为原始串外溢。
const rawMarker2614 = "UPSTREAM_RAW_MARKER_2614"

type iamUpdateMock2614 struct {
	mu     sync.Mutex
	status int
	body   string
	calls  int
}

func (m *iamUpdateMock2614) set(status int, body string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = status
	m.body = body
}

func (m *iamUpdateMock2614) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// newIAMUpdateMock2614 serves the client-credentials token endpoint (without it
// every request dies before reaching the update call) and the typed PUT failure.
func newIAMUpdateMock2614(t *testing.T) (*iamUpdateMock2614, *httptest.Server) {
	t.Helper()
	mock := &iamUpdateMock2614{status: http.StatusOK, body: `{"code":20000}`}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "svc-2614", "expires_in": 3600})
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		mock.mu.Lock()
		mock.calls++
		status, body := mock.status, mock.body
		mock.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")
	return mock, srv
}

type triple2614 struct {
	status  int
	code    int
	message string
	rawBody string
}

func (tr triple2614) retryHint() bool {
	return strings.Contains(tr.message, "请稍后再试")
}

func decodeTriple2614(t *testing.T, body []byte) (int, int, string) {
	t.Helper()
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &resp), "response must be JSON: %s", string(body))
	return 0, resp.Code, resp.Message
}

// seedUpdateUser2614 creates the local cache row used by both endpoints.
func seedUpdateUser2614(t *testing.T) (tenantID, userID string) {
	t.Helper()
	db := testfixtures.SetupTestDB(t)
	require.NotNil(t, db)
	testutil.SetDB(t, db)

	tenantID = uuid.NewString()
	userID = uuid.NewString()
	require.NoError(t, db.Create(&models.Tenant{ID: tenantID, Name: "T2614", Status: "active"}).Error)
	require.NoError(t, db.Create(&models.User{
		ID: uuid.NewString(), IAMSub: userID, TenantID: tenantID, OrgID: tenantID,
		Username: "u2614", Name: "Old Name", Email: "old2614@example.com",
		Phone: "13700002614", Role: "site_member", Status: "active",
	}).Error)
	return tenantID, userID
}

func iamUserUpdateRouter2614(tenantID, userID string) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		ctx = context.WithValue(ctx, middleware.ContextKeyNamespaceID, "ns-2614")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := NewIAMProxyHandler()
	r.PUT("/api/iam/users/:id", h.UpdateIAMUser)
	return r
}

func currentUserRouter2614(tenantID, userID string) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		ctx = context.WithValue(ctx, middleware.ContextKeyNamespaceID, "ns-2614")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := &UserStaffHandler{}
	r.PUT("/api/users/me", h.UpdateCurrentUser)
	return r
}

func runUpdateIAMUser2614(t *testing.T, r *gin.Engine, userID string) triple2614 {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{"name": "New Name", "email": "new2614@example.com"})
	req := httptest.NewRequest(http.MethodPut, "/api/iam/users/"+userID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	_, code, msg := decodeTriple2614(t, w.Body.Bytes())
	return triple2614{status: w.Code, code: code, message: msg, rawBody: w.Body.String()}
}

func runUpdateCurrentUser2614(t *testing.T, r *gin.Engine) triple2614 {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{"name": "New Name", "username": "newname2614"})
	req := httptest.NewRequest(http.MethodPut, "/api/users/me", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	_, code, msg := decodeTriple2614(t, w.Body.Bytes())
	return triple2614{status: w.Code, code: code, message: msg, rawBody: w.Body.String()}
}

func jsonBody2614(reason string) string {
	return fmt.Sprintf(`{"error":%q,"trace":%q}`, reason, rawMarker2614)
}

// R1: UpdateIAMUser 的九种上游状态 → 四类三元组。
func TestUpdateIAMUser2614_Triples(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantID, userID := seedUpdateUser2614(t)
	r := iamUserUpdateRouter2614(tenantID, userID)

	cases := []struct {
		label       string
		status      int
		body        string
		unreachable bool
		wantStatus  int
		wantCode    int
		wantMsgPart string
		persistent  bool
		transient   bool
	}{
		{label: "400", status: http.StatusBadRequest, body: jsonBody2614("invalid payload"), wantStatus: 400, wantCode: 40002},
		{label: "401", status: http.StatusUnauthorized, body: jsonBody2614("unauthorized"), wantStatus: 500, wantCode: 50015, persistent: true},
		{label: "403", status: http.StatusForbidden, body: jsonBody2614("permission denied"), wantStatus: 500, wantCode: 50015, persistent: true},
		{label: "404", status: http.StatusNotFound, body: jsonBody2614("user not found"), wantStatus: 500, wantCode: 50015, persistent: true},
		{label: "409email", status: http.StatusConflict, body: jsonBody2614("email already exists"), wantStatus: 409, wantCode: 40900, wantMsgPart: "邮箱已被其他账户使用"},
		{label: "409phone", status: http.StatusConflict, body: jsonBody2614("phone already exists"), wantStatus: 409, wantCode: 40900, wantMsgPart: "手机号已被其他账户使用"},
		{label: "409unknown", status: http.StatusConflict, body: jsonBody2614("some other conflict"), wantStatus: 409, wantCode: 40900, wantMsgPart: "邮箱或手机号已被其他账户占用"},
		{label: "500", status: http.StatusInternalServerError, body: jsonBody2614("boom"), wantStatus: 502, wantCode: 50200, transient: true},
		{label: "dead", unreachable: true, wantStatus: 502, wantCode: 50200, transient: true},
	}

	got := map[string]triple2614{}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			var srv *httptest.Server
			if tc.unreachable {
				var mock *iamUpdateMock2614
				mock, srv = newIAMUpdateMock2614(t)
				_ = mock
				srv.Close() // transport failure → transient
			} else {
				mock, _ := newIAMUpdateMock2614(t)
				mock.set(tc.status, tc.body)
			}
			tr := runUpdateIAMUser2614(t, r, userID)
			assert.Equal(t, tc.wantStatus, tr.status, "%s status (body=%s)", tc.label, tr.rawBody)
			assert.Equal(t, tc.wantCode, tr.code, "%s code", tc.label)
			if tc.wantMsgPart != "" {
				assert.Contains(t, tr.message, tc.wantMsgPart, "%s message", tc.label)
			}
			assert.NotContains(t, tr.rawBody, rawMarker2614, "%s must not leak the upstream body", tc.label)
			assert.NotEqual(t, 50000, tr.code, "%s must not fall back to the legacy 50000", tc.label)
			if tc.persistent {
				assert.False(t, tr.retryHint(), "%s persistent failure must not hint retry", tc.label)
			}
			if tc.transient {
				assert.NotEqual(t, http.StatusConflict, tr.status, "%s transient failure must not be 409", tc.label)
			}
			got[tc.label] = tr
		})
	}

	// 失败路径不得改写本地缓存（#2249 顺序不变：本地同步只在 IAM 成功后）。
	var cached models.User
	require.NoError(t, database.GetDB().Where("iam_sub = ?", userID).First(&cached).Error)
	localKept := 0
	if cached.Name == "Old Name" {
		localKept = 1
	}

	// token 语义：UPDATEIAM_PERSISTENT_NO_RETRY=0 表示「持久类不含请稍后再试」。
	persistentNoRetry := 0
	for _, label := range []string{"401", "403", "404"} {
		if got[label].retryHint() {
			persistentNoRetry = 1
		}
	}

	t.Logf("UPDATEIAM_400_CODE=%d UPDATEIAM_401_CODE=%d UPDATEIAM_403_STATUS=%d UPDATEIAM_403_CODE=%d "+
		"UPDATEIAM_404_CODE=%d UPDATEIAM_409_EMAIL_CODE=%d UPDATEIAM_409_PHONE_CODE=%d "+
		"UPDATEIAM_409_UNKNOWN_CODE=%d UPDATEIAM_500_STATUS=%d UPDATEIAM_500_CODE=%d UPDATEIAM_DEAD_CODE=%d "+
		"UPDATEIAM_LOCAL_CACHE_KEPT=%d",
		got["400"].code, got["401"].code, got["403"].status, got["403"].code,
		got["404"].code, got["409email"].code, got["409phone"].code,
		got["409unknown"].code, got["500"].status, got["500"].code, got["dead"].code, localKept)

	textOK := 1
	if !strings.Contains(got["409email"].message, "邮箱已被其他账户使用") ||
		!strings.Contains(got["409phone"].message, "手机号已被其他账户使用") {
		textOK = 0
	}
	rawLeak := 0
	for _, tr := range got {
		if strings.Contains(tr.rawBody, rawMarker2614) {
			rawLeak = 1
		}
	}
	legacyGone := 1
	for _, tr := range got {
		if tr.code == 50000 {
			legacyGone = 0
		}
	}

	t.Logf("UPDATEIAM_409_EMAIL_TEXT=%d UPDATEIAM_409_PHONE_TEXT=%d UPDATEIAM_RAW_LEAK=%d "+
		"UPDATEIAM_LEGACY_50000_GONE=%d UPDATEIAM_PERSISTENT_NO_RETRY=%d",
		textOK, textOK, rawLeak, legacyGone, persistentNoRetry)

	require.Equal(t, 1, localKept, "UPDATEIAM_LOCAL_CACHE_KEPT=1")
	require.Equal(t, 1, textOK, "UPDATEIAM_409_*_TEXT=1")
	require.Equal(t, 0, rawLeak, "UPDATEIAM_RAW_LEAK=0")
	require.Equal(t, 1, legacyGone, "UPDATEIAM_LEGACY_50000_GONE=1")
	require.Equal(t, 0, persistentNoRetry, "UPDATEIAM_PERSISTENT_NO_RETRY=0")
}

// R2: UpdateCurrentUser 的十种上游状态 → 四类三元组（含 username 文案逐字保持）。
func TestUpdateCurrentUser2614_Triples(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantID, userID := seedUpdateUser2614(t)
	r := currentUserRouter2614(tenantID, userID)

	cases := []struct {
		label       string
		status      int
		body        string
		unreachable bool
		wantStatus  int
		wantCode    int
		wantMsgPart string
		persistent  bool
		transient   bool
	}{
		{label: "400", status: http.StatusBadRequest, body: jsonBody2614("invalid payload"), wantStatus: 400, wantCode: 40002},
		{label: "401", status: http.StatusUnauthorized, body: jsonBody2614("unauthorized"), wantStatus: 500, wantCode: 50015, persistent: true},
		{label: "403", status: http.StatusForbidden, body: jsonBody2614("permission denied"), wantStatus: 500, wantCode: 50015, persistent: true},
		{label: "404", status: http.StatusNotFound, body: jsonBody2614("user not found"), wantStatus: 500, wantCode: 50015, persistent: true},
		{label: "409username", status: http.StatusConflict, body: jsonBody2614("username already exists"), wantStatus: 409, wantCode: 40900, wantMsgPart: "用户名已被占用，请换一个"},
		{label: "409email", status: http.StatusConflict, body: jsonBody2614("email already exists"), wantStatus: 409, wantCode: 40900, wantMsgPart: "邮箱已被其他账户使用"},
		{label: "409phone", status: http.StatusConflict, body: jsonBody2614("phone already exists"), wantStatus: 409, wantCode: 40900, wantMsgPart: "手机号已被其他账户使用"},
		{label: "500", status: http.StatusInternalServerError, body: jsonBody2614("boom"), wantStatus: 502, wantCode: 50200, transient: true},
		{label: "dead", unreachable: true, wantStatus: 502, wantCode: 50200, transient: true},
	}

	got := map[string]triple2614{}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			if tc.unreachable {
				_, srv := newIAMUpdateMock2614(t)
				srv.Close()
			} else {
				mock, _ := newIAMUpdateMock2614(t)
				mock.set(tc.status, tc.body)
			}
			tr := runUpdateCurrentUser2614(t, r)
			assert.Equal(t, tc.wantStatus, tr.status, "%s status (body=%s)", tc.label, tr.rawBody)
			assert.Equal(t, tc.wantCode, tr.code, "%s code", tc.label)
			if tc.wantMsgPart != "" {
				assert.Contains(t, tr.message, tc.wantMsgPart, "%s message", tc.label)
			}
			assert.NotContains(t, tr.rawBody, rawMarker2614, "%s must not leak the upstream body", tc.label)
			assert.NotEqual(t, 50000, tr.code, "%s must not fall back to the legacy 50000", tc.label)
			if tc.persistent {
				assert.False(t, tr.retryHint(), "%s persistent failure must not hint retry", tc.label)
			}
			if tc.transient {
				assert.NotEqual(t, http.StatusConflict, tr.status, "%s transient failure must not be 409", tc.label)
			}
			got[tc.label] = tr
		})
	}

	var cached models.User
	require.NoError(t, database.GetDB().Where("iam_sub = ?", userID).First(&cached).Error)
	localKept := 0
	if cached.Name == "Old Name" {
		localKept = 1
	}

	transientNotConflict := 1
	for _, label := range []string{"500", "dead"} {
		if got[label].status == http.StatusConflict {
			transientNotConflict = 0
		}
	}
	persistentNoRetry := 0
	for _, label := range []string{"401", "403", "404"} {
		if got[label].retryHint() {
			persistentNoRetry = 1
		}
	}
	rawLeak := 0
	for _, tr := range got {
		if strings.Contains(tr.rawBody, rawMarker2614) {
			rawLeak = 1
		}
	}
	usernameTextKept := 0
	if got["409username"].message == "用户名已被占用，请换一个" {
		usernameTextKept = 1
	}
	textOK := 1
	if !strings.Contains(got["409email"].message, "邮箱已被其他账户使用") ||
		!strings.Contains(got["409phone"].message, "手机号已被其他账户使用") {
		textOK = 0
	}

	// `%w` 包裹后仍必须命中类型判据（不依赖 err.Error() 的子串匹配）。
	wrapped := fmt.Errorf("outer context: %w", &services.IAMClientHTTPError{
		Op: "UpdateUser", Status: http.StatusConflict, Body: `{"error":"username already exists"}`,
	})
	wrappedTyped := 0
	if iamUserUpdateConflictText(wrapped) == "用户名已被占用，请换一个" {
		wrappedTyped = 1
	}

	t.Logf("UPDATECURRENT_400_CODE=%d UPDATECURRENT_401_CODE=%d UPDATECURRENT_403_STATUS=%d UPDATECURRENT_403_CODE=%d "+
		"UPDATECURRENT_404_CODE=%d UPDATECURRENT_409_USERNAME_STATUS=%d UPDATECURRENT_409_USERNAME_CODE=%d "+
		"UPDATECURRENT_500_STATUS=%d UPDATECURRENT_500_CODE=%d UPDATECURRENT_DEAD_CODE=%d",
		got["400"].code, got["401"].code, got["403"].status, got["403"].code,
		got["404"].code, got["409username"].status, got["409username"].code,
		got["500"].status, got["500"].code, got["dead"].code)
	t.Logf("UPDATECURRENT_409_USERNAME_TEXT_KEPT=%d UPDATECURRENT_409_EMAIL_TEXT=%d UPDATECURRENT_409_PHONE_TEXT=%d "+
		"UPDATECURRENT_RAW_LEAK=%d UPDATECURRENT_TRANSIENT_NOT_CONFLICT=%d UPDATECURRENT_LOCAL_CACHE_KEPT=%d "+
		"UPDATECURRENT_WRAPPED_TYPED=%d UPDATECURRENT_PERSISTENT_NO_RETRY=%d",
		usernameTextKept, textOK, textOK, rawLeak, transientNotConflict, localKept, wrappedTyped, persistentNoRetry)

	require.Equal(t, 1, usernameTextKept, "UPDATECURRENT_409_USERNAME_TEXT_KEPT=1")
	require.Equal(t, 1, textOK, "UPDATECURRENT_409_*_TEXT=1")
	require.Equal(t, 0, rawLeak, "UPDATECURRENT_RAW_LEAK=0")
	require.Equal(t, 1, transientNotConflict, "UPDATECURRENT_TRANSIENT_NOT_CONFLICT=1")
	require.Equal(t, 1, localKept, "UPDATECURRENT_LOCAL_CACHE_KEPT=1")
	require.Equal(t, 1, wrappedTyped, "UPDATECURRENT_WRAPPED_TYPED=1")
	require.Equal(t, 0, persistentNoRetry, "UPDATECURRENT_PERSISTENT_NO_RETRY=0")
}
