package handlers

// #2228 P3：绑微信冲突合并流（tuneloop 侧）。
//   - /users/me/wx-bind：IAM 返回 needs_merge → 透传前端（不再误报绑定失败）
//   - /users/me/wx-merge：本人令牌透传 IAM 完成合并（from=当前用户）
//   - 合并失败（IAM 非 200）→ 409 如实返回（不静默）

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/services"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func wxMergeMockServer2228(t *testing.T, mergeStatus int, capture *map[string]interface{}, captureAuth *string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/api/v1/auth/wx-bind", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"needs_merge":      true,
			"openid":           "openid-W-2228",
			"existing_user_id": "user-Y-2228",
			"target_user_id":   "user-X-2228",
		})
	})
	mux.HandleFunc("/api/v1/auth/wx-merge", func(w http.ResponseWriter, r *http.Request) {
		*captureAuth = r.Header.Get("Authorization")
		body, _ := json.Marshal(nil)
		_ = json.NewDecoder(r.Body).Decode(capture)
		_ = body
		if mergeStatus != http.StatusOK {
			w.WriteHeader(mergeStatus)
			writeJSON(w, map[string]string{"error": "merge rejected"})
			return
		}
		writeJSON(w, map[string]interface{}{"merged": true, "to_user_id": "user-Y-2228"})
	})
	return httptest.NewServer(mux)
}

func wxMergeRouter2228(sub string) *gin.Engine {
	r := gin.New()
	h := &UserStaffHandler{}
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{UserID: sub, TenantID: uuid.New().String(), OrgID: uuid.New().String()}.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/users/me/wx-bind", h.WxBindCurrentUser)
	r.POST("/users/me/wx-merge", h.WxMergeCurrentUser)
	return r
}

func TestWxBindCurrentUser_NeedsMergePassthrough_2228(t *testing.T) {
	gin.SetMode(gin.TestMode)
	capture := map[string]interface{}{}
	auth := ""
	srv := wxMergeMockServer2228(t, http.StatusOK, &capture, &auth)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	defer services.SetIAMInternalURLForTesting("")

	r := wxMergeRouter2228("user-X-2228")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users/me/wx-bind", bytes.NewReader([]byte(`{"code":"c2228"}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data, _ := resp["data"].(map[string]interface{})
	assert.Equal(t, true, data["needs_merge"], "冲突应透传 needs_merge 而非 409")
	assert.Equal(t, "user-Y-2228", data["existing_user_id"])
	assert.Equal(t, "openid-W-2228", data["openid"])
}

func TestWxMergeCurrentUser_2228(t *testing.T) {
	gin.SetMode(gin.TestMode)
	capture := map[string]interface{}{}
	auth := ""
	srv := wxMergeMockServer2228(t, http.StatusOK, &capture, &auth)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	defer services.SetIAMInternalURLForTesting("")

	r := wxMergeRouter2228("user-X-2228")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users/me/wx-merge",
		bytes.NewReader([]byte(`{"to_user_id":"user-Y-2228","openid":"openid-W-2228"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer user-X-token")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data, _ := resp["data"].(map[string]interface{})
	assert.Equal(t, true, data["merged"])
	// 本人令牌透传（IAM 以 claims.sub 鉴权）+ from=当前用户
	assert.Equal(t, "Bearer user-X-token", auth, "必须透传本人令牌")
	assert.Equal(t, "user-X-2228", capture["from_user_id"])
	assert.Equal(t, "user-Y-2228", capture["to_user_id"])
	assert.Equal(t, "openid-W-2228", capture["openid"])

	// 目标==本人 → 400
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/users/me/wx-merge",
		bytes.NewReader([]byte(`{"to_user_id":"user-X-2228","openid":"openid-W-2228"}`)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer user-X-token")
	r.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusBadRequest, w2.Code)
}

func TestWxMergeCurrentUser_IAMFailureMapped_2228(t *testing.T) {
	gin.SetMode(gin.TestMode)
	capture := map[string]interface{}{}
	auth := ""
	srv := wxMergeMockServer2228(t, http.StatusBadRequest, &capture, &auth)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	defer services.SetIAMInternalURLForTesting("")

	r := wxMergeRouter2228("user-X-2228")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users/me/wx-merge",
		bytes.NewReader([]byte(`{"to_user_id":"user-Y-2228","openid":"openid-W-2228"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer user-X-token")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code, "IAM 拒绝 → 409 如实返回（不得静默成功）")
	assert.Contains(t, w.Body.String(), "账户合并失败")
}
