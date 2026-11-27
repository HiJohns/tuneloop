package handlers

// #2598 R4：微信绑定 / 合并 / 二维码合并三条路径的错误三元组分流。
//
// 缺陷（项 2/3/4）：三条路径都把 `WxBind` / `WxMerge` / `WxMergeByTicket` 的
// **任何**错误（含 IAM 不可达/超时）写成 409/40900——瞬时故障被伪装成「数据冲突」，
// 前端按冲突处理（「数据冲突，请刷新后重试」、重试按钮、监控口径全错）。
//
// 期望（Decision Lock D5）：
//   上游 409 → 409/40900（可行动：已绑定其他账户 / 绑定关系已变更）
//   上游 400/422 → 400/40002（可行动：重新扫码/重新发起绑定，**不含**「请稍后再试」）
//   上游 401/403/404 → 500/50015（「身份服务配置异常，请联系平台管理员处理」）
//   传输层/429/5xx → 502/50200（含「请稍后再试」，**不得**为 409）
//   needs_merge（200）→ 20000 + data.needs_merge=true（保持不变）
//
// Token lines are printed with fmt.Printf so scripts/verify-2598.sh can grep them
// regardless of go test -v indentation.

import (
	"bytes"
	"encoding/json"
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
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// wxFailureIAM2598 是三条微信路径共用的 fake IAM：按配置返回指定上游状态。
type wxFailureIAM2598 struct {
	bindStatus    int // /api/v1/auth/wx-bind
	mergeStatus   int // /api/v1/auth/wx-merge
	confirmStatus int // /api/v1/auth/wx-merge-confirm
}

func (f *wxFailureIAM2598) handler() http.Handler {
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/api/v1/auth/wx-bind", func(w http.ResponseWriter, r *http.Request) {
		if f.bindStatus != http.StatusOK {
			w.WriteHeader(f.bindStatus)
			writeJSON(w, map[string]string{"error": "upstream rejects bind"})
			return
		}
		writeJSON(w, map[string]interface{}{
			"needs_merge":      true,
			"openid":           "openid-2598",
			"existing_user_id": "user-Y-2598",
			"target_user_id":   "user-X-2598",
			"merge_ticket":     "ticket-2598",
		})
	})
	mux.HandleFunc("/api/v1/auth/wx-merge", func(w http.ResponseWriter, r *http.Request) {
		if f.mergeStatus != http.StatusOK {
			w.WriteHeader(f.mergeStatus)
			writeJSON(w, map[string]string{"error": "upstream rejects merge"})
			return
		}
		writeJSON(w, map[string]interface{}{"merged": true})
	})
	mux.HandleFunc("/api/v1/auth/wx-merge-confirm", func(w http.ResponseWriter, r *http.Request) {
		if f.confirmStatus != http.StatusOK {
			w.WriteHeader(f.confirmStatus)
			writeJSON(w, map[string]string{"error": "upstream rejects merge ticket"})
			return
		}
		writeJSON(w, map[string]interface{}{"merged": true, "from_user_id": "iam-X-2598", "to_user_id": "iam-Y-2598"})
	})
	return mux
}

// installWxFailureIAM2598 启动 fake IAM 并把它设为后端上游，返回其 URL。
func installWxFailureIAM2598(t *testing.T, f *wxFailureIAM2598) string {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() {
		services.SetIAMInternalURLForTesting("")
		srv.Close()
	})
	return srv.URL
}

func wxTriple2598(t *testing.T, rec *httptest.ResponseRecorder) (int, int, string) {
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

func callWxBind2598(t *testing.T, router *gin.Engine, code string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"code": code})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/users/me/wx-bind", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func callWxMerge2598(t *testing.T, router *gin.Engine) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/users/me/wx-merge",
		strings.NewReader(`{"to_user_id":"user-Y-2598","openid":"openid-2598"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer user-X-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestWxFailure2598_Matrix 覆盖三条路径 × 上游状态矩阵，并输出 R4 合同 token。
func TestWxFailure2598_Matrix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	if db == nil {
		t.Skip("no test database available")
	}

	tenantID := uuid.NewString()
	orgID := uuid.NewString()
	require.NoError(t, db.Create(&models.Tenant{ID: tenantID, Name: "T2598", Status: "active"}).Error)
	siteID := uuid.NewString()
	require.NoError(t, db.Create(&models.Site{ID: siteID, TenantID: tenantID, OrgID: orgID, Name: "S2598", Type: "normal"}).Error)

	bindUser := models.User{
		ID: uuid.NewString(), IAMSub: "iam-X-2598", TenantID: tenantID, OrgID: orgID,
		Name: "X2598", Phone: "13700002598", Status: "active",
	}
	require.NoError(t, db.Create(&bindUser).Error)

	router := wxMergeRouter2228("user-X-2598")
	persistentNoRetry := 0 // 0 = 持久/重做类文案均不含「请稍后再试」

	// ---- 微信绑定：上游 400（用户可重做）----
	iam := &wxFailureIAM2598{bindStatus: http.StatusBadRequest, mergeStatus: http.StatusOK, confirmStatus: http.StatusOK}
	installWxFailureIAM2598(t, iam)
	status, code, msg := wxTriple2598(t, callWxBind2598(t, router, "c2598"))
	bind400Code := code
	require.Equal(t, http.StatusBadRequest, status, "上游 400 必须 400（非 409）: %s", msg)
	require.Equal(t, 40002, code, "上游 400 必须 40002")
	require.Contains(t, msg, "微信授权已失效", "上游 400 文案必须可行动")
	if strings.Contains(msg, "请稍后再试") {
		persistentNoRetry = 1
		t.Errorf("上游 400 文案不得含「请稍后再试」: %q", msg)
	}

	// ---- 微信绑定：上游 403（持久配置/权限）----
	iam.bindStatus = http.StatusForbidden
	status, code, msg = wxTriple2598(t, callWxBind2598(t, router, "c2598"))
	bind403Code := code
	require.Equal(t, http.StatusInternalServerError, status, "上游 403 必须 500")
	require.Equal(t, 50015, code, "上游 403 必须 50015")
	require.Equal(t, iamConfigErrorMessage, msg, "持久配置文案必须与码表逐字一致")
	if strings.Contains(msg, "请稍后再试") {
		persistentNoRetry = 1
		t.Errorf("上游 403 文案不得含「请稍后再试」: %q", msg)
	}

	// ---- 微信绑定：上游 409（真冲突）----
	iam.bindStatus = http.StatusConflict
	status, code, msg = wxTriple2598(t, callWxBind2598(t, router, "c2598"))
	bind409Code := code
	require.Equal(t, http.StatusConflict, status, "上游 409 必须 409")
	require.Equal(t, 40900, code, "上游 409 必须 40900")
	require.Contains(t, msg, "该微信已绑定其他账户", "冲突文案必须指明冲突对象")
	require.NotContains(t, msg, "请稍后再试", "冲突不是瞬时故障")

	// ---- 微信绑定：IAM 不可达（瞬时）----
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	services.SetIAMInternalURLForTesting(deadURL)
	status, code, msg = wxTriple2598(t, callWxBind2598(t, router, "c2598"))
	transientStatus := status
	transientCode := code
	require.Equal(t, http.StatusBadGateway, status, "瞬时故障必须 502（不得 409）")
	require.Equal(t, 50200, code, "瞬时故障必须 50200")
	require.Contains(t, msg, "请稍后再试", "瞬时故障文案必须提示可重试")

	// ---- 微信绑定：needs_merge 分支保持不变（20000 + data.needs_merge）----
	iam.bindStatus = http.StatusOK
	services.SetIAMInternalURLForTesting("")
	installWxFailureIAM2598(t, iam)
	rec := callWxBind2598(t, router, "c2598")
	require.Equal(t, http.StatusOK, rec.Code, "needs_merge 必须 200: %s", rec.Body.String())
	var merged map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &merged))
	respCode := 0
	if raw, ok := merged["code"].(float64); ok {
		respCode = int(raw)
	}
	needsMerge := 0
	if data, ok := merged["data"].(map[string]interface{}); ok && data["needs_merge"] == true && respCode == 20000 {
		needsMerge = 1
	}
	require.Equal(t, 1, needsMerge, "needs_merge 分支必须保持 20000 + data.needs_merge=true")

	// ---- 账户合并：上游 400（用户可重做）----
	iam.mergeStatus = http.StatusBadRequest
	status, code, msg = wxTriple2598(t, callWxMerge2598(t, router))
	merge400Code := code
	require.Equal(t, http.StatusBadRequest, status, "上游 400 必须 400（非 409）: %s", msg)
	require.Equal(t, 40002, code, "上游 400 必须 40002")
	require.Contains(t, msg, "账户合并失败", "文案必须保留「账户合并失败」关键词")
	if strings.Contains(msg, "请稍后再试") {
		persistentNoRetry = 1
		t.Errorf("账户合并上游 400 文案不得含「请稍后再试」: %q", msg)
	}

	// ---- 二维码合并（MergeBind）：上游 400（合并凭证失效）----
	iam.mergeStatus = http.StatusOK
	iam.confirmStatus = http.StatusBadRequest
	const bindToken = "0123456789abcdfe"
	bindTokensMu.Lock()
	bindTokens[bindToken] = &bindTokenEntry{
		UserID: bindUser.ID, Status: "needs_merge", MergeTicket: "ticket-2598", CreatedAt: time.Now(),
	}
	bindTokensMu.Unlock()
	t.Cleanup(func() {
		bindTokensMu.Lock()
		delete(bindTokens, bindToken)
		bindTokensMu.Unlock()
	})

	bindRouter := bindMergeTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/wechat-bind/merge",
		strings.NewReader(`{"token":"`+bindToken+`","merge_ticket":"ticket-2598"}`))
	req.Header.Set("Content-Type", "application/json")
	wRec := httptest.NewRecorder()
	bindRouter.ServeHTTP(wRec, req)
	status, code, msg = wxTriple2598(t, wRec)
	mergeBind400Code := code
	require.Equal(t, http.StatusBadRequest, status, "MergeBind 上游 400 必须 400（非 409）: %s", msg)
	require.Equal(t, 40002, code, "MergeBind 上游 400 必须 40002")
	require.Contains(t, msg, "合并凭证已失效", "MergeBind 400 文案必须可行动")
	if strings.Contains(msg, "请稍后再试") {
		persistentNoRetry = 1
		t.Errorf("MergeBind 上游 400 文案不得含「请稍后再试」: %q", msg)
	}

	fmt.Printf("WXBIND_400_CODE=%d\n", bind400Code)
	fmt.Printf("WXBIND_403_CODE=%d\n", bind403Code)
	fmt.Printf("WXBIND_409_CODE=%d\n", bind409Code)
	fmt.Printf("WX_TRANSIENT_STATUS=%d\n", transientStatus)
	fmt.Printf("WX_TRANSIENT_CODE=%d\n", transientCode)
	fmt.Printf("WXMERGE_400_CODE=%d\n", merge400Code)
	fmt.Printf("MERGEBIND_400_CODE=%d\n", mergeBind400Code)
	fmt.Printf("PERSISTENT_NO_RETRY_HINT=%d\n", persistentNoRetry)
	fmt.Printf("WXBIND_NEEDS_MERGE=%d\n", needsMerge)
}
