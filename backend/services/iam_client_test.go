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
