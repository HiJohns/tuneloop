package services

// #2457 R1：IAM organizations.name 是组织名的权威源。UpdateOrganizationName 必须
// 以服务凭据（client-credentials Bearer）PUT /api/v1/organizations/{id}，请求体只含
// name，并回读 IAM 返回的 org.name（供 handler 记录/校验）。

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

const updateOrgID2457 = "76d46658-0000-0000-0000-000000000001"

func TestUpdateOrganizationName_PutsNameWithServiceCredential_2457(t *testing.T) {
	var mu sync.Mutex
	var method, path, body, auth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/auth/token" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "svc-token", "expires_in": 3600, "token_type": "Bearer",
			})
			return
		}
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		method, path, body = r.Method, r.URL.Path, string(raw)
		auth = r.Header.Get("Authorization")
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": updateOrgID2457, "name": "海淀店-rename",
		})
	}))
	defer srv.Close()
	SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { SetIAMInternalURLForTesting("") })

	client := NewIAMClientWithCredentials("tuneloop", "secret")
	org, err := client.UpdateOrganizationName(updateOrgID2457, "  海淀店-rename  ")
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, http.MethodPut, method)
	require.Equal(t, "/api/v1/organizations/"+updateOrgID2457, path)
	require.JSONEq(t, `{"name":"海淀店-rename"}`, body, "请求体只能携带 name（trim 后）")
	require.Equal(t, "Bearer svc-token", auth, "必须使用服务凭据 Bearer，而非操作者 token")
	require.Equal(t, updateOrgID2457, org.ID)
	require.Equal(t, "海淀店-rename", org.Name)

	t.Logf("METHOD=%s", method)
	t.Logf("PATH=%s", path)
	t.Logf("BODY=%s", body)
	t.Logf("AUTH=Bearer")
	t.Logf("RESULT=IAM_UPDATE_ORG_OK")
}

func TestUpdateOrganizationName_ConflictIsSentinel_2457(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/auth/token" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "svc-token", "expires_in": 3600, "token_type": "Bearer",
			})
			return
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"organization name exists"}`))
	}))
	defer srv.Close()
	SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { SetIAMInternalURLForTesting("") })

	client := NewIAMClientWithCredentials("tuneloop", "secret")
	_, err := client.UpdateOrganizationName(updateOrgID2457, "海淀店")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrIAMOrgNameConflict), "409 必须映射为 ErrIAMOrgNameConflict: %v", err)
}

func TestUpdateOrganizationName_RejectsEmptyNameWithoutHTTP_2457(t *testing.T) {
	var calls int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/auth/token" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "svc-token", "expires_in": 3600, "token_type": "Bearer",
			})
			return
		}
		mu.Lock()
		calls++
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": updateOrgID2457, "name": "x"})
	}))
	defer srv.Close()
	SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { SetIAMInternalURLForTesting("") })

	client := NewIAMClientWithCredentials("tuneloop", "secret")
	_, err := client.UpdateOrganizationName(updateOrgID2457, "   ")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrIAMInvalidRequest), "空名必须本地拒绝（IAM 侧无空名校验）: %v", err)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 0, calls, "空名不得发起 IAM 写请求")
}

func TestUpdateOrganizationName_RejectsMissingOrgID_2457(t *testing.T) {
	client := NewIAMClientWithCredentials("tuneloop", "secret")
	_, err := client.UpdateOrganizationName("  ", "海淀店")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrIAMInvalidRequest))
}

// R9：状态码 → 可判别错误映射（403/404/409/500）+ 调用者令牌路径。
func TestUpdateOrganizationName_StatusMapping_2457(t *testing.T) {
	cases := []struct {
		name   string
		status int
		check  func(t *testing.T, err error)
	}{
		{"forbidden", http.StatusForbidden, func(t *testing.T, err error) {
			require.True(t, errors.Is(err, ErrIAMForbidden), "403 必须映射为 ErrIAMForbidden: %v", err)
		}},
		{"not_found", http.StatusNotFound, func(t *testing.T, err error) {
			require.True(t, errors.Is(err, ErrIAMOrgNotFound), "404 必须映射为 ErrIAMOrgNotFound: %v", err)
		}},
		{"conflict", http.StatusConflict, func(t *testing.T, err error) {
			require.True(t, errors.Is(err, ErrIAMOrgNameConflict), "409 必须映射为 ErrIAMOrgNameConflict: %v", err)
		}},
		{"server_error", http.StatusInternalServerError, func(t *testing.T, err error) {
			require.Error(t, err)
			require.False(t, errors.Is(err, ErrIAMForbidden))
			require.False(t, errors.Is(err, ErrIAMOrgNotFound))
			require.False(t, errors.Is(err, ErrIAMOrgNameConflict))
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/api/v1/auth/token" {
					_ = json.NewEncoder(w).Encode(map[string]interface{}{
						"access_token": "svc-token", "expires_in": 3600, "token_type": "Bearer",
					})
					return
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"error":"mapped"}`))
			}))
			defer srv.Close()
			SetIAMInternalURLForTesting(srv.URL)
			t.Cleanup(func() { SetIAMInternalURLForTesting("") })

			client := NewIAMClientWithCredentials("tuneloop", "secret")
			_, err := client.UpdateOrganizationName(updateOrgID2457, "海淀店")
			tc.check(t, err)
			t.Logf("STATUS=%d MAPPED_OK=1", tc.status)
		})
	}
}

// R9：调用者令牌路径必须使用该令牌（而非服务凭据）。
func TestUpdateOrganizationNameWithToken_UsesUserToken_2457(t *testing.T) {
	var mu sync.Mutex
	var auth, path string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/auth/token" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "svc-token", "expires_in": 3600, "token_type": "Bearer",
			})
			return
		}
		mu.Lock()
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": updateOrgID2457, "name": "海淀店"})
	}))
	defer srv.Close()
	SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { SetIAMInternalURLForTesting("") })

	client := NewIAMClientWithCredentials("tuneloop", "secret")
	_, err := client.UpdateOrganizationNameWithToken("user-token-abc", updateOrgID2457, "海淀店")
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "Bearer user-token-abc", auth)
	require.Equal(t, "/api/v1/organizations/"+updateOrgID2457, path)
	t.Logf("METHOD=PUT")
	t.Logf("PATH=%s", path)
	t.Logf("AUTH=user-token")
}
