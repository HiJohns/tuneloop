package services

// #2461 R1–R3：IAM 组织角色更新契约 = JSON body（beaconiam a7e2845 起）。
// 旧实现把 role 放 query 且 body=nil；Echo 对 PUT 不绑定 query → 每次 400
// "role is required"，导致所有 role 同步静默失效。本文件用 fake IAM 断言
// 方法/路径/body/Authorization，并验证 400 不会被吞成成功。

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	roleContractServiceToken2461 = "svc-token-2461"
	roleContractOrgID2461        = "ORG1"
	roleContractUserID2461       = "USER1"
)

type roleContractRecorder2461 struct {
	method   string
	path     string
	rawQuery string
	body     string
	auth     string
	calls    int
}

func newRoleContractMock2461(t *testing.T, rec *roleContractRecorder2461, status int, respBody string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": roleContractServiceToken2461,
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/api/v1/organizations/", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		rec.method = r.Method
		rec.path = r.URL.Path
		rec.rawQuery = r.URL.RawQuery
		rec.body = string(raw)
		rec.auth = r.Header.Get("Authorization")
		rec.calls++
		if status != 0 && status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, respBody)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":20000}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { SetIAMInternalURLForTesting("") })
	return srv
}

func assertRoleBody2461(t *testing.T, body, wantRole string) {
	t.Helper()
	require.NotEmpty(t, body, "IAM role 请求体不得为空（旧实现 body=nil → 400 role is required）")
	var got map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(body), &got), "请求体必须是 JSON：%s", body)
	assert.Equal(t, wantRole, got["role"], "body.role")
}

// R1：服务凭据通路 —— PUT /organizations/{org}/users/{user}/role，JSON body，无 query。
func TestUpdateUserRoleInOrgJSONBody_2461(t *testing.T) {
	rec := &roleContractRecorder2461{}
	newRoleContractMock2461(t, rec, http.StatusOK, "")

	client := NewIAMClientWithCredentials("test-ns-2461", "test-secret-2461")
	require.NoError(t, client.UpdateUserRoleInOrg(roleContractOrgID2461, roleContractUserID2461, "STAFF"))

	assert.Equal(t, http.MethodPut, rec.method, "METHOD")
	assert.Equal(t, "/api/v1/organizations/ORG1/users/USER1/role", rec.path, "PATH")
	assert.Empty(t, rec.rawQuery, "HAS_QUERY 必须为 0（不得再带 ?role=）")
	assertRoleBody2461(t, rec.body, "STAFF")
	assert.Equal(t, "Bearer "+roleContractServiceToken2461, rec.auth, "AUTH=service")
	assert.Equal(t, 1, rec.calls)
	t.Logf("METHOD=%s PATH=%s HAS_QUERY=%d BODY=%s AUTH=service", rec.method, rec.path, len(rec.rawQuery), rec.body)
}

// R2：用户令牌通路 —— 同一 JSON body 契约，Authorization 为传入的用户 token。
func TestUpdateUserRoleInOrgWithTokenJSONBody_2461(t *testing.T) {
	rec := &roleContractRecorder2461{}
	newRoleContractMock2461(t, rec, http.StatusOK, "")

	client := NewIAMClientWithCredentials("test-ns-2461", "test-secret-2461")
	require.NoError(t, client.UpdateUserRoleInOrgWithToken("tok-abc", roleContractOrgID2461, roleContractUserID2461, "ADMIN"))

	assert.Equal(t, http.MethodPut, rec.method, "METHOD")
	assert.Equal(t, "/api/v1/organizations/ORG1/users/USER1/role", rec.path, "PATH")
	assert.Empty(t, rec.rawQuery, "HAS_QUERY 必须为 0")
	assertRoleBody2461(t, rec.body, "ADMIN")
	assert.Equal(t, "Bearer tok-abc", rec.auth, "AUTH=user-token")
	t.Logf("METHOD=%s PATH=%s HAS_QUERY=%d BODY=%s AUTH=user-token", rec.method, rec.path, len(rec.rawQuery), rec.body)
}

// R3（负例）：IAM 400 不得被吞 —— 错误串必须同时包含状态码与 IAM 原文，调用方可判别。
func TestUpdateUserRoleInOrg400NotSwallowed_2461(t *testing.T) {
	rec := &roleContractRecorder2461{}
	newRoleContractMock2461(t, rec, http.StatusBadRequest, `{"error":"role is required"}`)

	client := NewIAMClientWithCredentials("test-ns-2461", "test-secret-2461")
	err := client.UpdateUserRoleInOrg(roleContractOrgID2461, roleContractUserID2461, "STAFF")
	require.Error(t, err, "IAM 400 必须返回错误，不得降级为成功")
	assert.Contains(t, err.Error(), "400", "错误串必须含状态码")
	assert.Contains(t, err.Error(), "role is required", "错误串必须含 IAM 原文")

	err = client.UpdateUserRoleInOrgWithToken("tok-abc", roleContractOrgID2461, roleContractUserID2461, "STAFF")
	require.Error(t, err, "用户令牌通路同样不得吞错")
	assert.Contains(t, err.Error(), "400")
	assert.Contains(t, err.Error(), "role is required")
	t.Logf("ERR_CONTAINS_400=1 ERR_CONTAINS_ROLE_REQUIRED=1 IAM_STATUS=400")
}
