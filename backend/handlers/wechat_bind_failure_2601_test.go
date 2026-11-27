package handlers

// #2601 R1/R2/R3：wechat_bind.go 的 ConfirmBind / Unbind / MergeBind 状态前置
// 三条失败路径必须按 #2598 的统一三元组口径分流（禁止把持久失败表达成可重试）：
//
//	transient  → 502 / 50200 → 文案含「请稍后再试」
//	conflict   → 409 / 40900 → 指明冲突对象 + 可纠正动作
//	userAction → 400 / 40002 → 指明用户动作，不含「稍后再试」
//	config     → 500 / 50015 → iamConfigErrorMessage，不含「稍后再试」
//
// 失败分类只允许 errors.As（*services.IAMAPIError / *services.IAMClientHTTPError）。
// 本文件使用独立的 fake IAM，避免改动 #2598 的 wxFailureIAM2598 夹具。
//
// Token lines are printed with fmt.Printf so scripts/verify-2601.sh can grep
// them regardless of go test -v indentation.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// 与 wechat_bind_test.go 一致的本地用户租户/组织占位（无 FK 依赖）。
const (
	testTenantID2601 = "00000000-0000-0000-0000-000000000001"
	testOrgID2601    = "00000000-0000-0000-0000-000000000000"
)

// fakeIAM2601 同时服务 client-token、wx-bind 与 users/{id}（UpdateUser）三个端点，
// 每个端点按配置返回指定上游状态。
type fakeIAM2601 struct {
	bindStatus int // POST /api/v1/auth/wx-bind
	userStatus int // PUT  /api/v1/users/{id}
}

func (f *fakeIAM2601) handler() http.Handler {
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	// IAMClient.doRequest 先取 client token；缺该端会让所有 Unbind 用例退化成瞬时类。
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"access_token": "mock-token-2601",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/api/v1/auth/wx-bind", func(w http.ResponseWriter, r *http.Request) {
		if f.bindStatus != http.StatusOK {
			w.WriteHeader(f.bindStatus)
			writeJSON(w, map[string]string{"error": "upstream rejects bind"})
			return
		}
		writeJSON(w, map[string]interface{}{
			"needs_merge":      true,
			"openid":           "openid-2601",
			"existing_user_id": "user-Y-2601",
			"target_user_id":   "user-X-2601",
			"merge_ticket":     "ticket-2601",
		})
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		if f.userStatus != http.StatusOK {
			w.WriteHeader(f.userStatus)
			writeJSON(w, map[string]string{"error": "upstream rejects update"})
			return
		}
		writeJSON(w, map[string]interface{}{"code": 20000, "message": "success"})
	})
	return mux
}

// installIAM2601 启动 fake IAM 并把它设为后端上游，返回其 URL。
func installIAM2601(t *testing.T, f *fakeIAM2601) string {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() {
		services.SetIAMInternalURLForTesting("")
		srv.Close()
	})
	return srv.URL
}

// installUnreachableIAM2601 注入「已关闭端口」以制造传输层失败（无上游状态）。
func installUnreachableIAM2601(t *testing.T) {
	t.Helper()
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	services.SetIAMInternalURLForTesting(deadURL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
}

func triple2601(t *testing.T, rec *httptest.ResponseRecorder) (int, int, string) {
	t.Helper()
	var resp map[string]interface{}
	if rec.Body.Len() > 0 {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	}
	code := 0
	if raw, ok := resp["code"].(float64); ok {
		code = int(raw)
	}
	msg := ""
	if raw, ok := resp["message"].(string); ok {
		msg = raw
	}
	return rec.Code, code, msg
}

func confirmBindRouter2601() *gin.Engine {
	r := gin.New()
	r.POST("/wechat-bind/confirm", NewWechatBindHandler().ConfirmBind)
	return r
}

func callConfirmBind2601(t *testing.T, router *gin.Engine, token string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"token": token, "exchange_token": "ex-2601"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/wechat-bind/confirm", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestConfirmBind2601_Triples 覆盖 ConfirmBind × 上游 {400,403,404,409,500} +
// 不可达 + needs_merge(200)，并输出 R1 合同 token。
func TestConfirmBind2601_Triples(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	if db == nil {
		return
	}
	user := models.User{
		ID: uuid.NewString(), IAMSub: "iam-2601-confirm", Name: "C2601", Phone: "13900002611", Status: "active",
		TenantID: testTenantID2601, OrgID: testOrgID2601,
	}
	require.NoError(t, db.Create(&user).Error)

	const token = "2601confirm0001a"
	bindTokensMu.Lock()
	bindTokens[token] = &bindTokenEntry{UserID: user.ID, Status: "pending", CreatedAt: time.Now()}
	bindTokensMu.Unlock()
	t.Cleanup(func() {
		bindTokensMu.Lock()
		delete(bindTokens, token)
		bindTokensMu.Unlock()
	})

	router := confirmBindRouter2601()
	persistentNoRetry := 0
	rawLeak := 0
	recordNoRetry := func(label, msg string) {
		if strings.Contains(msg, "请稍后再试") {
			persistentNoRetry = 1
			t.Errorf("%s 文案不得含「请稍后再试」: %q", label, msg)
		}
	}
	recordRawLeak := func(label, msg string) {
		for _, raw := range []string{"IAM returned status", "upstream rejects bind", "upstream rejects update", "failed to bind:"} {
			if strings.Contains(msg, raw) {
				rawLeak = 1
				t.Errorf("%s 文案不得包含上游原始串 %q: %q", label, raw, msg)
			}
		}
	}

	iam := &fakeIAM2601{bindStatus: http.StatusOK, userStatus: http.StatusOK}
	installIAM2601(t, iam)

	// ---- 上游 400（用户可重做：exchange_token 失效）----
	iam.bindStatus = http.StatusBadRequest
	status400, code400, msg := triple2601(t, callConfirmBind2601(t, router, token))
	require.Equal(t, http.StatusBadRequest, status400, "上游 400 必须 400（不得 500/50000）: %s", msg)
	require.Equal(t, 40002, code400, "上游 400 必须 40002")
	require.Contains(t, msg, "微信授权已失效", "上游 400 文案必须指明用户动作")
	recordNoRetry("上游 400", msg)
	recordRawLeak("上游 400", msg)

	// ---- 上游 403（持久配置/权限）----
	iam.bindStatus = http.StatusForbidden
	status403, code403, msg := triple2601(t, callConfirmBind2601(t, router, token))
	require.Equal(t, http.StatusInternalServerError, status403, "上游 403 必须 500（不得 502）: %s", msg)
	require.Equal(t, 50015, code403, "上游 403 必须 50015（不得 50000）")
	require.Equal(t, iamConfigErrorMessage, msg, "持久配置文案必须与码表逐字一致")
	recordNoRetry("上游 403", msg)
	recordRawLeak("上游 403", msg)

	// ---- 上游 404（持久：client/user 不存在）----
	iam.bindStatus = http.StatusNotFound
	status404, code404, msg := triple2601(t, callConfirmBind2601(t, router, token))
	require.Equal(t, http.StatusInternalServerError, status404, "上游 404 必须 500: %s", msg)
	require.Equal(t, 50015, code404, "上游 404 必须 50015")
	recordNoRetry("上游 404", msg)
	recordRawLeak("上游 404", msg)

	// ---- 上游 409（真冲突）----
	iam.bindStatus = http.StatusConflict
	status409, code409, msg := triple2601(t, callConfirmBind2601(t, router, token))
	require.Equal(t, http.StatusConflict, status409, "上游 409 必须 409: %s", msg)
	require.Equal(t, 40900, code409, "上游 409 必须 40900")
	require.Contains(t, msg, "该微信已绑定其他账户", "冲突文案必须指明冲突对象")
	recordNoRetry("上游 409", msg)
	recordRawLeak("上游 409", msg)

	// ---- 上游 500（瞬时）----
	iam.bindStatus = http.StatusInternalServerError
	transientStatus, transientCode, msg := triple2601(t, callConfirmBind2601(t, router, token))
	require.Equal(t, http.StatusBadGateway, transientStatus, "上游 500 必须 502（不得 409）: %s", msg)
	require.NotEqual(t, http.StatusConflict, transientStatus, "瞬时类不得为 409")
	require.Equal(t, 50200, transientCode, "上游 500 必须 50200")
	require.Contains(t, msg, "请稍后再试", "瞬时文案必须提示可重试")
	recordRawLeak("上游 500", msg)

	// ---- IAM 不可达（无上游状态 → 瞬时）----
	installUnreachableIAM2601(t)
	deadStatus, deadCode, msg := triple2601(t, callConfirmBind2601(t, router, token))
	require.Equal(t, http.StatusBadGateway, deadStatus, "不可达必须 502（不得 409）: %s", msg)
	require.Equal(t, 50200, deadCode, "不可达必须 50200")
	require.Contains(t, msg, "请稍后再试", "不可达文案必须提示可重试")

	// ---- needs_merge（200）仍是成功分支：409/40900 + data.needs_merge=true ----
	iam.bindStatus = http.StatusOK
	installIAM2601(t, iam)
	rec := callConfirmBind2601(t, router, token)
	require.Equal(t, http.StatusConflict, rec.Code, "needs_merge 保持 409: %s", rec.Body.String())
	var mergeResp struct {
		Code int `json:"code"`
		Data struct {
			NeedsMerge  bool   `json:"needs_merge"`
			MergeTicket string `json:"merge_ticket"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &mergeResp))
	require.Equal(t, 40900, mergeResp.Code, "needs_merge 保持 40900")
	require.True(t, mergeResp.Data.NeedsMerge, "needs_merge 必须为 true")
	require.Equal(t, "ticket-2601", mergeResp.Data.MergeTicket, "merge_ticket 必须透传")
	needsMergeKept := 0
	if lookupBindStatus(token) == "needs_merge" && mergeResp.Data.NeedsMerge {
		needsMergeKept = 1
	}
	require.Equal(t, 1, needsMergeKept, "needs_merge 必须把 token 置 needs_merge")

	fmt.Printf("CONFIRMBIND_400_STATUS=%d\n", status400)
	fmt.Printf("CONFIRMBIND_400_CODE=%d\n", code400)
	fmt.Printf("CONFIRMBIND_403_STATUS=%d\n", status403)
	fmt.Printf("CONFIRMBIND_403_CODE=%d\n", code403)
	fmt.Printf("CONFIRMBIND_404_STATUS=%d\n", status404)
	fmt.Printf("CONFIRMBIND_404_CODE=%d\n", code404)
	fmt.Printf("CONFIRMBIND_409_STATUS=%d\n", status409)
	fmt.Printf("CONFIRMBIND_409_CODE=%d\n", code409)
	fmt.Printf("CONFIRMBIND_TRANSIENT_STATUS=%d\n", transientStatus)
	fmt.Printf("CONFIRMBIND_TRANSIENT_CODE=%d\n", transientCode)
	fmt.Printf("CONFIRMBIND_DEAD_STATUS=%d\n", deadStatus)
	fmt.Printf("CONFIRMBIND_PERSISTENT_NO_RETRY=%d\n", persistentNoRetry)
	fmt.Printf("CONFIRMBIND_RAW_LEAK=%d\n", rawLeak)
	fmt.Printf("CONFIRMBIND_NEEDS_MERGE_KEPT=%d\n", needsMergeKept)
}

// TestUnbind2601_Triples 覆盖 Unbind × 上游 {400,403,404,500} + 不可达，
// 并断言 IAMClient.UpdateUser 的类型化（403 文本逐字保持、404 可 errors.As）。
func TestUnbind2601_Triples(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	if db == nil {
		return
	}
	const iamSub = "iam-2601-unbind"
	require.NoError(t, db.Create(&models.User{
		ID: uuid.NewString(), IAMSub: iamSub, Name: "B2601", Phone: "13900002612", Status: "active",
		TenantID: testTenantID2601, OrgID: testOrgID2601,
	}).Error)

	router := gin.New()
	router.POST("/users/me/wechat-unbind", func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyUserID, iamSub)
		c.Request = c.Request.WithContext(ctx)
		NewWechatBindHandler().Unbind(c)
	})
	call := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/users/me/wechat-unbind", nil))
		return w
	}

	persistentNoRetry := 0
	rawLeak := 0
	recordNoRetry := func(label, msg string) {
		if strings.Contains(msg, "请稍后再试") {
			persistentNoRetry = 1
			t.Errorf("%s 文案不得含「请稍后再试」: %q", label, msg)
		}
	}
	recordRawLeak := func(label, msg string) {
		for _, raw := range []string{"IAM returned status", "upstream rejects update", "unbind failed:", "UpdateUser returned status"} {
			if strings.Contains(msg, raw) {
				rawLeak = 1
				t.Errorf("%s 文案不得包含上游原始串 %q: %q", label, raw, msg)
			}
		}
	}

	iam := &fakeIAM2601{bindStatus: http.StatusOK, userStatus: http.StatusOK}
	installIAM2601(t, iam)

	// ---- 上游 400（用户可重做）----
	iam.userStatus = http.StatusBadRequest
	status400, code400, msg := triple2601(t, call())
	require.Equal(t, http.StatusBadRequest, status400, "上游 400 必须 400（不得 500/50000）: %s", msg)
	require.Equal(t, 40002, code400, "上游 400 必须 40002")
	recordNoRetry("上游 400", msg)
	recordRawLeak("上游 400", msg)

	// ---- 上游 403（持久配置/权限）----
	iam.userStatus = http.StatusForbidden
	status403, code403, msg := triple2601(t, call())
	require.Equal(t, http.StatusInternalServerError, status403, "上游 403 必须 500: %s", msg)
	require.Equal(t, 50015, code403, "上游 403 必须 50015")
	require.Equal(t, iamConfigErrorMessage, msg, "持久配置文案必须与码表逐字一致")
	recordNoRetry("上游 403", msg)
	recordRawLeak("上游 403", msg)

	// ---- 上游 404（持久：账户已删）----
	iam.userStatus = http.StatusNotFound
	status404, code404, msg := triple2601(t, call())
	require.Equal(t, http.StatusInternalServerError, status404, "上游 404 必须 500: %s", msg)
	require.Equal(t, 50015, code404, "上游 404 必须 50015")
	recordNoRetry("上游 404", msg)
	recordRawLeak("上游 404", msg)

	// ---- 上游 500（瞬时）----
	iam.userStatus = http.StatusInternalServerError
	transientStatus, transientCode, msg := triple2601(t, call())
	require.Equal(t, http.StatusBadGateway, transientStatus, "上游 500 必须 502（不得 409）: %s", msg)
	require.NotEqual(t, http.StatusConflict, transientStatus, "瞬时类不得为 409")
	require.Equal(t, 50200, transientCode, "上游 500 必须 50200")
	require.Contains(t, msg, "请稍后再试", "瞬时文案必须提示可重试")
	recordRawLeak("上游 500", msg)

	// ---- IAM 不可达（无上游状态 → 瞬时）----
	installUnreachableIAM2601(t)
	deadStatus, deadCode, msg := triple2601(t, call())
	require.Equal(t, http.StatusBadGateway, deadStatus, "不可达必须 502: %s", msg)
	require.Equal(t, 50200, deadCode, "不可达必须 50200")
	require.Contains(t, msg, "请稍后再试", "不可达文案必须提示可重试")

	// ---- IAMClient.UpdateUser 类型化：403 文本逐字保持 ----
	installIAM2601(t, &fakeIAM2601{userStatus: http.StatusForbidden})
	err403 := services.NewIAMClient().UpdateUser(iamSub, &services.UpdateUserRequest{WxOpenid: strPtr2601("")})
	require.Error(t, err403)
	require.True(t, strings.HasPrefix(err403.Error(), "permission denied: "),
		"403 文本前缀必须逐字保持 permission denied:，实际 %q", err403.Error())
	text403Kept := 0
	if strings.HasPrefix(err403.Error(), "permission denied: ") {
		text403Kept = 1
	}

	// ---- IAMClient.UpdateUser 类型化：404 → IAMClientHTTPError(Status=404) ----
	installIAM2601(t, &fakeIAM2601{userStatus: http.StatusNotFound})
	err404 := services.NewIAMClient().UpdateUser(iamSub, &services.UpdateUserRequest{WxOpenid: strPtr2601("")})
	require.Error(t, err404)
	var httpErr *services.IAMClientHTTPError
	require.True(t, errors.As(err404, &httpErr), "404 必须以 *IAMClientHTTPError 上抛（errors.As 可命中）: %v", err404)
	require.Equal(t, http.StatusNotFound, httpErr.Status, "IAMClientHTTPError.Status 必须是上游 404")
	typed404 := 0
	if errors.As(err404, &httpErr) && httpErr.Status == http.StatusNotFound {
		typed404 = 1
	}

	fmt.Printf("UNBIND_400_STATUS=%d\n", status400)
	fmt.Printf("UNBIND_400_CODE=%d\n", code400)
	fmt.Printf("UNBIND_403_STATUS=%d\n", status403)
	fmt.Printf("UNBIND_403_CODE=%d\n", code403)
	fmt.Printf("UNBIND_404_STATUS=%d\n", status404)
	fmt.Printf("UNBIND_404_CODE=%d\n", code404)
	fmt.Printf("UNBIND_TRANSIENT_STATUS=%d\n", transientStatus)
	fmt.Printf("UNBIND_TRANSIENT_CODE=%d\n", transientCode)
	fmt.Printf("UNBIND_DEAD_STATUS=%d\n", deadStatus)
	fmt.Printf("UNBIND_PERSISTENT_NO_RETRY=%d\n", persistentNoRetry)
	fmt.Printf("UNBIND_RAW_LEAK=%d\n", rawLeak)
	fmt.Printf("UPDATEUSER_403_TEXT_KEPT=%d\n", text403Kept)
	fmt.Printf("UPDATEUSER_404_TYPED=%d\n", typed404)
}

// TestMergeBind2601_StateMismatch 断言 MergeBind 的 token 状态前置与 400/40002
// 同族（不再是 400/40902 + 内部状态串），并保持 merged 幂等分支。
func TestMergeBind2601_StateMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	if db == nil {
		return
	}
	userID := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: "iam-2601-merge", Name: "M2601", Phone: "13900002613", Status: "active",
		TenantID: testTenantID2601, OrgID: testOrgID2601,
	}).Error)

	router := bindMergeTestRouter()

	const boundToken = "2601mergebound1"
	const mergedToken = "2601mergmerged"
	bindTokensMu.Lock()
	bindTokens[boundToken] = &bindTokenEntry{UserID: userID, Status: "bound", CreatedAt: time.Now()}
	bindTokens[mergedToken] = &bindTokenEntry{
		UserID: userID, Status: "merged", MergedToUserID: "user-Y-2601", CreatedAt: time.Now(),
	}
	bindTokensMu.Unlock()
	t.Cleanup(func() {
		bindTokensMu.Lock()
		delete(bindTokens, boundToken)
		delete(bindTokens, mergedToken)
		bindTokensMu.Unlock()
	})

	// ---- token 状态 bound（非 needs_merge）→ 400/40002 + 可行动文案 ----
	req := httptest.NewRequest(http.MethodPost, "/wechat-bind/merge",
		strings.NewReader(`{"token":"`+boundToken+`","merge_ticket":"ticket-2601"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	status, code, msg := triple2601(t, w)
	require.Equal(t, http.StatusBadRequest, status, "状态前置必须 400: %s", w.Body.String())
	require.Equal(t, 40002, code, "状态前置必须 40002（与相邻 invalid/expired merge ticket 同族），不得 40902")
	actionable := 0
	if strings.Contains(msg, "绑定二维码已失效") && strings.Contains(msg, "重新生成") {
		actionable = 1
	}
	require.Equal(t, 1, actionable, "状态前置文案必须可行动（重新生成二维码），实际 %q", msg)
	require.NotContains(t, msg, "status=", "内部状态串不得外溢: %q", msg)
	require.NotContains(t, msg, "not waiting for merge", "内部状态串不得外溢: %q", msg)
	internalLeak := 0
	if strings.Contains(msg, "status=") || strings.Contains(msg, "not waiting for merge") {
		internalLeak = 1
	}

	// ---- token 状态 merged → 幂等成功（200/20000）----
	req = httptest.NewRequest(http.MethodPost, "/wechat-bind/merge",
		strings.NewReader(`{"token":"`+mergedToken+`","merge_ticket":"ticket-2601"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "merged token 必须幂等成功: %s", w.Body.String())
	var mergedResp struct {
		Code int `json:"code"`
		Data struct {
			Merged bool `json:"merged"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &mergedResp))
	require.Equal(t, 20000, mergedResp.Code)
	require.True(t, mergedResp.Data.Merged)
	idempotentKept := 0
	if w.Code == http.StatusOK && mergedResp.Code == 20000 && mergedResp.Data.Merged {
		idempotentKept = 1
	}

	fmt.Printf("MERGEBIND_STATE_STATUS=%d\n", status)
	fmt.Printf("MERGEBIND_STATE_CODE=%d\n", code)
	fmt.Printf("MERGEBIND_STATE_MSG_ACTIONABLE=%d\n", actionable)
	fmt.Printf("MERGEBIND_STATE_INTERNAL_LEAK=%d\n", internalLeak)
	fmt.Printf("MERGEBIND_IDEMPOTENT_KEPT=%d\n", idempotentKept)
}

func strPtr2601(v string) *string { return &v }
