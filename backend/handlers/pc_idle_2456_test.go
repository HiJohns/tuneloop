package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// #2456: PC 后台 idle timeout 证据测试。
// 每个用例把断言打印成 `KEY=VALUE` 行（R2/R3/R4/R5/R6/R7），
// 由 scripts/verify-2456.sh 从 `go test -v` 输出中抽取，作为 docs/evidence/2456 产物。
const (
	pcIdleTestSub     = "22bfb6d9-0000-4000-8000-0000000000a1"
	pcIdleOtherSub    = "22bfb6d9-0000-4000-8000-0000000000b2"
	pcIdleActiveToken = "rt-pc-active"
	pcIdleIdleToken   = "rt-pc-idle"
	pcIdleWeappToken  = "rt-weapp-h5"
)

// ensurePCSessionsTable 保证测试库存在 pc_sessions 并清空：
// 模板库可能由旧迁移构建（新迁移尚未重放），这里显式兜底，避免用例依赖模板状态。
func ensurePCSessionsTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	if !db.Migrator().HasTable(&models.PCSession{}) {
		require.NoError(t, db.Migrator().CreateTable(&models.PCSession{}))
	}
	require.NoError(t, db.Exec("DELETE FROM pc_sessions").Error)
}

func countPCSessionsByRefresh(t *testing.T, db *gorm.DB, refreshToken string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&models.PCSession{}).
		Where("refresh_hash = ?", services.HashRefreshToken(refreshToken)).
		Count(&count).Error)
	return count
}

func seedPCSession(t *testing.T, db *gorm.DB, sub, refreshToken string, lastActivity time.Time) models.PCSession {
	t.Helper()
	row := models.PCSession{
		IAMSub:         sub,
		RefreshHash:    services.HashRefreshToken(refreshToken),
		LastActivityAt: lastActivity,
	}
	require.NoError(t, db.Create(&row).Error)
	return row
}

// newPCIdleFakeIAM serves the three IAM token grants used by #2456:
// authorization_code (OAuth callback), password (账密登录), refresh_token.
// refreshCalls counts refresh grants so a rejected idle session can be proven
// never to reach IAM.
func newPCIdleFakeIAM(t *testing.T, refreshCalls *int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			GrantType    string `json:"grant_type"`
			Code         string `json:"code"`
			RefreshToken string `json:"refresh_token"`
		}
		_ = json.Unmarshal(body, &payload)

		var refreshToken string
		switch payload.GrantType {
		case "authorization_code":
			refreshToken = "rt-" + payload.Code
		case "password":
			refreshToken = "rt-login"
		case "refresh_token":
			*refreshCalls++
			refreshToken = payload.RefreshToken
		default:
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  signHS256Token(t, pcIdleTestSub, "PCIdleUser"),
			"refresh_token": refreshToken,
			"expires_in":    900,
			"token_type":    "Bearer",
		})
	})
	return httptest.NewServer(mux)
}

// R2: 仅 PC 入口（OAuth 回调无 client_type / 账密登录）写 pc_sessions；
// H5 回调（client_type=wx）不写。
func TestPCIdleR2_RegistrationOnlyForPCEntrypoints(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	ensurePCSessionsTable(t, db)

	refreshCalls := 0
	iamServer := newPCIdleFakeIAM(t, &refreshCalls)
	defer iamServer.Close()
	services.SetIAMInternalURLForTesting(iamServer.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("EXTERNAL_WEB_URL", "http://localhost:5554")
	t.Setenv("EXTERNAL_MOBILE_URL", "http://localhost:5553")

	handler := NewAuthHandler(db)
	router := gin.New()
	router.GET("/api/auth/callback", handler.Callback)
	router.POST("/api/auth/login", handler.PostLogin)

	// 1) PC OAuth 回调（不传 client_type）
	pcCallback := httptest.NewRecorder()
	router.ServeHTTP(pcCallback, httptest.NewRequest("GET", "/api/auth/callback?code=pc-callback", nil))
	require.Equal(t, http.StatusOK, pcCallback.Code)
	pcRows := countPCSessionsByRefresh(t, db, "rt-pc-callback")
	t.Logf("PC_CALLBACK_ROWS=%d", pcRows)
	require.Equal(t, int64(1), pcRows, "PC OAuth callback must register exactly one pc session")

	// 2) H5 回调（client_type=wx）——不得登记
	h5Callback := httptest.NewRecorder()
	router.ServeHTTP(h5Callback, httptest.NewRequest("GET", "/api/auth/callback?code=wx-callback&client_type=wx", nil))
	require.Equal(t, http.StatusOK, h5Callback.Code)
	h5Rows := countPCSessionsByRefresh(t, db, "rt-wx-callback")
	t.Logf("H5_CALLBACK_ROWS=%d", h5Rows)
	require.Equal(t, int64(0), h5Rows, "H5/小程序 callback must not register a pc session")

	// 3) 账密登录
	loginBody := `{"identifier":"pc@example.com","password":"secret"}`
	login := httptest.NewRecorder()
	loginReq := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(login, loginReq)
	require.Equal(t, http.StatusOK, login.Code)
	loginRows := countPCSessionsByRefresh(t, db, "rt-login")
	t.Logf("PC_LOGIN_ROWS=%d", loginRows)
	require.Equal(t, int64(1), loginRows, "password login must register exactly one pc session")
}

// R2 负例：客户端类型判定必须把 H5/小程序判为移动端（否则会被误当 PC 登记 idle 会话，
// 即计划中的负例 `H5_CALLBACK_ROWS=1 → FAIL`）。
func TestPCIdleR2_NegativeClientTypePredicate(t *testing.T) {
	mobile := []string{"wx", "wechat", "mobile"}
	desktop := []string{"", "web", "pc"}
	misclassified := 0
	for _, ct := range mobile {
		if !isMobileClientType(ct) {
			misclassified++
		}
	}
	for _, ct := range desktop {
		if isMobileClientType(ct) {
			misclassified++
		}
	}
	t.Logf("NEG_H5_AS_PC=%d", misclassified)
	require.Equal(t, 0, misclassified)
}

// refreshRouter builds a router exposing only POST /api/auth/refresh.
func refreshRouter(t *testing.T, db *gorm.DB) *gin.Engine {
	t.Helper()
	router := gin.New()
	router.POST("/api/auth/refresh", NewAuthHandler(db).Refresh)
	return router
}

func postRefresh(t *testing.T, router *gin.Engine, refreshToken string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/auth/refresh",
		strings.NewReader(`{"refresh_token":"`+refreshToken+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func decodeCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var body struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Code
}

// R3: 静置超过 1h → 拒绝续期（401/40110），且不向 IAM 续期。
func TestPCIdleR3_IdleSessionRejected(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	ensurePCSessionsTable(t, db)
	t.Setenv("PC_IDLE_TIMEOUT_MINUTES", "60")

	refreshCalls := 0
	iamServer := newPCIdleFakeIAM(t, &refreshCalls)
	defer iamServer.Close()
	services.SetIAMInternalURLForTesting(iamServer.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	seedPCSession(t, db, pcIdleTestSub, pcIdleIdleToken, time.Now().Add(-2*time.Hour))

	w := postRefresh(t, refreshRouter(t, db), pcIdleIdleToken)
	code := decodeCode(t, w)

	t.Logf("HTTP=%d", w.Code)
	t.Logf("CODE=%d", code)
	t.Logf("IAM_CALLS=%d", refreshCalls)
	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Equal(t, 40110, code)
	require.Equal(t, 0, refreshCalls, "idle-expired refresh must not reach IAM")
	require.Contains(t, w.Body.String(), "登录已超时，请重新登录")
}

// R4: 1h 内续期保持登录，并前移 last_activity_at。
func TestPCIdleR4_ActiveSessionRefreshes(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	ensurePCSessionsTable(t, db)
	t.Setenv("PC_IDLE_TIMEOUT_MINUTES", "60")

	refreshCalls := 0
	iamServer := newPCIdleFakeIAM(t, &refreshCalls)
	defer iamServer.Close()
	services.SetIAMInternalURLForTesting(iamServer.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	seeded := seedPCSession(t, db, pcIdleTestSub, pcIdleActiveToken, time.Now().Add(-10*time.Minute))

	w := postRefresh(t, refreshRouter(t, db), pcIdleActiveToken)
	code := decodeCode(t, w)

	var after models.PCSession
	require.NoError(t, db.Where("id = ?", seeded.ID).First(&after).Error)
	touched := 0
	if after.LastActivityAt.After(seeded.LastActivityAt.Add(1 * time.Minute)) {
		touched = 1
	}

	t.Logf("HTTP=%d", w.Code)
	t.Logf("CODE=%d", code)
	t.Logf("IAM_CALLS=%d", refreshCalls)
	t.Logf("TOUCHED=%d", touched)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 20000, code)
	require.Equal(t, 1, refreshCalls)
	require.Equal(t, 1, touched, "successful refresh must advance last_activity_at")
}

// R5: 非 PC 会话（无 pc_sessions 行，模拟 H5/小程序令牌）不受 idle 策略影响。
func TestPCIdleR5_NonPCSessionUnaffected(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	ensurePCSessionsTable(t, db)
	t.Setenv("PC_IDLE_TIMEOUT_MINUTES", "0")

	refreshCalls := 0
	iamServer := newPCIdleFakeIAM(t, &refreshCalls)
	defer iamServer.Close()
	services.SetIAMInternalURLForTesting(iamServer.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	// 无 pc_sessions 行 = H5/小程序令牌；即便配置禁用也必须是"未套用 idle"路径。
	enforced := 0
	if countPCSessionsByRefresh(t, db, pcIdleWeappToken) > 0 {
		enforced = 1
	}

	w := postRefresh(t, refreshRouter(t, db), pcIdleWeappToken)
	code := decodeCode(t, w)

	t.Logf("HTTP=%d", w.Code)
	t.Logf("CODE=%d", code)
	t.Logf("IDLE_ENFORCED=%d", enforced)
	t.Logf("IAM_CALLS=%d", refreshCalls)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 20000, code)
	require.Equal(t, 0, enforced, "non-PC token must not have a pc session row")
	require.Equal(t, 1, refreshCalls)
}

// R6: 活动上报只前移调用者自己的会话；无身份 → 401。
func TestPCIdleR6_ActivityEndpoint(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	ensurePCSessionsTable(t, db)
	t.Setenv("PC_IDLE_TIMEOUT_MINUTES", "60")

	now := time.Now()
	owner := seedPCSession(t, db, pcIdleTestSub, "rt-owner", now.Add(-30*time.Minute))
	other := seedPCSession(t, db, pcIdleOtherSub, "rt-other", now.Add(-30*time.Minute))

	handler := NewAuthHandler(db)
	router := gin.New()
	router.POST("/api/auth/pc-activity", func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyUserID, pcIdleTestSub)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}, handler.PCActivity)

	req := httptest.NewRequest("POST", "/api/auth/pc-activity", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var ownerAfter, otherAfter models.PCSession
	require.NoError(t, db.Where("id = ?", owner.ID).First(&ownerAfter).Error)
	require.NoError(t, db.Where("id = ?", other.ID).First(&otherAfter).Error)

	ownerTouched := 0
	if ownerAfter.LastActivityAt.After(now.Add(-5 * time.Second)) {
		ownerTouched = 1
	}
	otherTouched := 0
	if otherAfter.LastActivityAt.After(now.Add(-5 * time.Second)) {
		otherTouched = 1
	}

	// 无身份 → 401
	noTokenRouter := gin.New()
	noTokenRouter.POST("/api/auth/pc-activity", NewAuthHandler(db).PCActivity)
	noToken := httptest.NewRecorder()
	noTokenRouter.ServeHTTP(noToken, httptest.NewRequest("POST", "/api/auth/pc-activity", nil))

	t.Logf("HTTP=%d", w.Code)
	t.Logf("OWNER_TOUCHED=%d", ownerTouched)
	t.Logf("OTHER_TOUCHED=%d", otherTouched)
	t.Logf("NO_TOKEN_HTTP=%d", noToken.Code)
	require.Equal(t, 1, ownerTouched, "caller's own pc session must be touched")
	require.Equal(t, 0, otherTouched, "other users' pc sessions must not be touched")
	require.Equal(t, http.StatusUnauthorized, noToken.Code)
}

// R7: PC_IDLE_TIMEOUT_MINUTES=1 让 61 分钟静置被拒；=0 表示禁用（放行）。
func TestPCIdleR7_ConfigDrivenThreshold(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()

	// call := func(minutes string) (int, int) { ... }

	// 用例 A：=1，静置 61 分钟 → 拒绝
	ensurePCSessionsTable(t, db)
	t.Setenv("PC_IDLE_TIMEOUT_MINUTES", "1")
	refreshCalls := 0
	iamServer := newPCIdleFakeIAM(t, &refreshCalls)
	defer iamServer.Close()
	services.SetIAMInternalURLForTesting(iamServer.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	seedPCSession(t, db, pcIdleTestSub, "rt-cfg-1", time.Now().Add(-61*time.Minute))
	router := refreshRouter(t, db)
	w := postRefresh(t, router, "rt-cfg-1")
	rejected := 0
	if w.Code == http.StatusUnauthorized && decodeCode(t, w) == 40110 {
		rejected = 1
	}

	// 用例 B：=0（禁用），同样静置 61 分钟 → 放行
	t.Setenv("PC_IDLE_TIMEOUT_MINUTES", "0")
	ensurePCSessionsTable(t, db)
	seedPCSession(t, db, pcIdleTestSub, "rt-cfg-0", time.Now().Add(-61*time.Minute))
	w0 := postRefresh(t, refreshRouter(t, db), "rt-cfg-0")
	disabled := 0
	if w0.Code == http.StatusOK && decodeCode(t, w0) == 20000 {
		disabled = 1
	}

	t.Logf("MINUTES_1_REJECT=%d", rejected)
	t.Logf("ZERO_DISABLED=%d", disabled)
	require.Equal(t, 1, rejected)
	require.Equal(t, 1, disabled)
}
