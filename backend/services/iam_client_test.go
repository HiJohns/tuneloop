package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func newIAMCreateUserMockServer(t *testing.T, users []User) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			if users == nil {
				users = []User{}
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"users": users})
		case http.MethodPost:
			json.NewEncoder(w).Encode(map[string]interface{}{
				"user_id": "new-user-123",
				"status":  "active",
			})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	return httptest.NewServer(mux)
}

func TestCreateOrGetUser_UsernameConflictWithDifferentEmail_Rejected(t *testing.T) {
	existing := User{
		ID:       "u-1",
		Username: "lisi",
		Name:     "李四",
		Email:    "nanjing_head@tuneloop.com",
		Phone:    "342989",
	}
	srv := newIAMCreateUserMockServer(t, []User{existing})
	defer srv.Close()
	SetIAMInternalURLForTesting(srv.URL)

	client := NewIAMClientWithCredentials("test-ns", "test-secret")
	req := &CreateUserRequest{
		Username: "lisi",
		Name:     "李四",
		Email:    "xjk_admin@tuneloop.com",
		Phone:    "555123",
	}

	_, err := client.CreateOrGetUser("user-token", req)
	require.Error(t, err, "username-only conflict with different email must be rejected")
	var conflictErr *UsernameConflictError
	require.ErrorAs(t, err, &conflictErr)
	require.Equal(t, "lisi", conflictErr.Username)
	require.Equal(t, "nanjing_head@tuneloop.com", conflictErr.Email)
	require.Contains(t, err.Error(), "already in use")
}

func TestCreateOrGetUser_EmailMatch_ReusesExistingUser(t *testing.T) {
	existing := User{
		ID:       "u-1",
		Username: "lisi",
		Name:     "李四",
		Email:    "nanjing_head@tuneloop.com",
		Phone:    "342989",
	}
	srv := newIAMCreateUserMockServer(t, []User{existing})
	defer srv.Close()
	SetIAMInternalURLForTesting(srv.URL)

	client := NewIAMClientWithCredentials("test-ns", "test-secret")
	req := &CreateUserRequest{
		Username: "lisi",
		Name:     "李四",
		Email:    "nanjing_head@tuneloop.com",
	}

	result, err := client.CreateOrGetUser("user-token", req)
	require.NoError(t, err)
	require.True(t, result.Conflict)
	require.Equal(t, "u-1", result.UserID)
	require.Len(t, result.ExistingUsers, 1)
}

func TestCreateOrGetUser_PhoneMatch_ReusesExistingUser(t *testing.T) {
	existing := User{
		ID:       "u-1",
		Username: "lisi",
		Name:     "李四",
		Email:    "nanjing_head@tuneloop.com",
		Phone:    "342989",
	}
	srv := newIAMCreateUserMockServer(t, []User{existing})
	defer srv.Close()
	SetIAMInternalURLForTesting(srv.URL)

	client := NewIAMClientWithCredentials("test-ns", "test-secret")
	req := &CreateUserRequest{
		Username: "lisi",
		Name:     "李四",
		Email:    "another@tuneloop.com",
		Phone:    "342989",
	}

	result, err := client.CreateOrGetUser("user-token", req)
	require.NoError(t, err)
	require.True(t, result.Conflict)
	require.Equal(t, "u-1", result.UserID)
}

func TestCreateOrGetUser_NoConflict_CreatesNewUser(t *testing.T) {
	srv := newIAMCreateUserMockServer(t, nil)
	defer srv.Close()
	SetIAMInternalURLForTesting(srv.URL)

	client := NewIAMClientWithCredentials("test-ns", "test-secret")
	req := &CreateUserRequest{
		Username: "wangwu",
		Name:     "王五",
		Email:    "wangwu@tuneloop.com",
		Phone:    "888999",
	}

	result, err := client.CreateOrGetUser("user-token", req)
	require.NoError(t, err)
	require.False(t, result.Conflict)
	require.Equal(t, "new-user-123", result.UserID)
}

func TestCreateOrGetUser_ErrorIncludesConflictingEmail(t *testing.T) {
	existing := User{
		ID:       "u-1",
		Username: "lisi",
		Name:     "李四",
		Email:    "nanjing_head@tuneloop.com",
	}
	srv := newIAMCreateUserMockServer(t, []User{existing})
	defer srv.Close()
	SetIAMInternalURLForTesting(srv.URL)

	client := NewIAMClientWithCredentials("test-ns", "test-secret")
	req := &CreateUserRequest{
		Username: "lisi",
		Name:     "李四",
		Email:    "xjk_admin@tuneloop.com",
	}

	_, err := client.CreateOrGetUser("user-token", req)
	require.Error(t, err)
	var conflictErr *UsernameConflictError
	require.ErrorAs(t, err, &conflictErr)
	require.Equal(t, "nanjing_head@tuneloop.com", conflictErr.Email)
	require.True(t, strings.Contains(err.Error(), "nanjing_head@tuneloop.com"),
		"error should reference the conflicting account email")
}

// #1735: GetUserAuthState must serve repeated subject checks from the
// process-local cache so the auth chain does not call IAM on every request.
func TestGetUserAuthState_CachesSubjectState(t *testing.T) {
	userHits := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/api/v1/users/u-1", func(w http.ResponseWriter, r *http.Request) {
		userHits++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"user": User{ID: "u-1", Status: "active", TokenVersion: 1724217600000},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	SetIAMInternalURLForTesting(srv.URL)

	client := NewIAMClientWithCredentials("test-ns", "test-secret")

	tv, status, err := client.GetUserAuthState("u-1")
	require.NoError(t, err)
	require.Equal(t, int64(1724217600000), tv)
	require.Equal(t, "active", status)

	tv2, status2, err := client.GetUserAuthState("u-1")
	require.NoError(t, err)
	require.Equal(t, tv, tv2, "cached token_version must match")
	require.Equal(t, status, status2, "cached status must match")
	require.Equal(t, 1, userHits, "second call must be served from cache, not hit IAM again")
}

// #1735: a deleted IAM subject (404 from GetUser) must surface as the
// ErrUserNotFound sentinel so the middleware can emit an explicit
// account_not_found 401 instead of failing open.
func TestGetUserAuthState_UserNotFound_ReturnsSentinel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/api/v1/users/gone-user", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"user not found"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	SetIAMInternalURLForTesting(srv.URL)

	client := NewIAMClientWithCredentials("test-ns", "test-secret")

	_, _, err := client.GetUserAuthState("gone-user")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUserNotFound, "404 must map to the ErrUserNotFound sentinel")
}

// #2016 S1: ResolveUserOpenid —— 绑定命中返回 openid；未绑定（404）返回空串且不报错。
func TestResolveUserOpenid_BoundAndUnbound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/api/v1/internal/users/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/bound-user/wx-openid") {
			_ = json.NewEncoder(w).Encode(map[string]string{"openid": "op-123"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "wx_binding_not_found"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	SetIAMInternalURLForTesting(srv.URL)

	client := NewIAMClientWithCredentials("test-ns", "test-secret")

	openid, err := client.ResolveUserOpenid("bound-user")
	require.NoError(t, err)
	require.Equal(t, "op-123", openid)

	empty, err := client.ResolveUserOpenid("unbound-user")
	require.NoError(t, err, "404 must be treated as no-binding, not an error")
	require.Equal(t, "", empty)
}

// #2410 D5：非空 wx_openid 不得通过 UpdateUser/UpdateUserWithToken 直接写入 IAM
// （IAM 该分支是 #483 多绑定 upsert，会绕过 #2228 needs_merge）。空值解绑保留。
func TestIAMClient_RejectsDirectWxOpenid(t *testing.T) {
	userPUT := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			userPUT++
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":20000,"message":"success"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	SetIAMInternalURLForTesting(srv.URL)
	defer SetIAMInternalURLForTesting("")

	client := NewIAMClientWithCredentials("test-ns", "test-secret")
	openid := "o-direct-must-be-rejected"

	err := client.UpdateUser("u-1", &UpdateUserRequest{WxOpenid: &openid})
	require.Error(t, err)
	require.Contains(t, err.Error(), "WxBind", "error must point callers at the controlled wx-bind path")
	require.Equal(t, 0, userPUT, "non-empty wx_openid must be rejected before any HTTP request")

	err = client.UpdateUserWithToken("user-token", "u-1", &UpdateUserRequest{WxOpenid: &openid})
	require.Error(t, err)
	require.Contains(t, err.Error(), "WxBind")
	require.Equal(t, 0, userPUT, "UpdateUserWithToken must reject before any HTTP request")

	// 空值解绑仍可用（保留 #2020 解绑语义）。
	empty := ""
	require.NoError(t, client.UpdateUser("u-1", &UpdateUserRequest{WxOpenid: &empty}))
	require.Equal(t, 1, userPUT, "empty wx_openid (unbind) must still reach IAM")
	t.Logf("user_put_requests=%d (non-empty openid rejected before HTTP; empty unbind sent)", userPUT)
}
