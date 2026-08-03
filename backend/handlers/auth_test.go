package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

const testIAMSecret = "test-secret-1489"

// signHS256Token builds an HS256 JWT signed with the IAM client secret so that
// IAMService.ValidateToken (HS256 branch) accepts it.
func signHS256Token(t *testing.T, userID, name string) string {
	t.Helper()
	claims := services.JWTClaims{
		UserID:   userID,
		TenantID: "00000000-0000-0000-0000-000000000001",
		Name:     name,
		Role:     "USER",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testIAMSecret))
	require.NoError(t, err)
	return signed
}

func newWxLoginMockServer(t *testing.T, userID, name string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/wx-login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  signHS256Token(t, userID, name),
			"expires_in":    7200,
			"token_type":    "Bearer",
			"refresh_token": "",
		})
	})
	return httptest.NewServer(mux)
}

// TestWxLogin_Channel3_NewUser_CreatesLocalCache verifies #1489: when IAM
// recognizes the WeChat user but the local users table has no record, the
// handler must create the local cache and return is_new=true so the client
// proceeds to the registration page instead of a 409 binding error.
func TestWxLogin_Channel3_NewUser_CreatesLocalCache(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()

	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")

	userID := "6d1e2c3a-0000-4000-8000-000000000001"
	srv := newWxLoginMockServer(t, userID, "TestUser")
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)

	router := gin.New()
	router.POST("/api/auth/wx-login", NewAuthHandler(db).WxLogin)

	postLogin := func() *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]interface{}{"code": "test-code-123"})
		req := httptest.NewRequest("POST", "/api/auth/wx-login", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// First login: local cache missing → create + is_new=true
	w := postLogin()
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Token string `json:"token"`
			IsNew bool   `json:"is_new"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.True(t, resp.Data.IsNew, "first login of a new IAM user must return is_new=true")
	require.NotEmpty(t, resp.Data.Token)

	var local models.User
	require.NoError(t, db.Where("iam_sub = ?", userID).First(&local).Error)
	require.Equal(t, "TestUser", local.Name)
	require.Equal(t, "USER", local.Role)
	require.Equal(t, "active", local.Status)

	// Second login: local cache exists → is_new=false
	w2 := postLogin()
	var resp2 struct {
		Code int `json:"code"`
		Data struct {
			Token string `json:"token"`
			IsNew bool   `json:"is_new"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp2))
	require.Equal(t, 20000, resp2.Code)
	require.False(t, resp2.Data.IsNew, "second login with existing local cache must return is_new=false")
}

// TestWxLogin_Channel3_ExistingLocalUser_IsNewFalse verifies the existing
// local cache path returns is_new=false (no 409).
func TestWxLogin_Channel3_ExistingLocalUser_IsNewFalse(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()

	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")

	userID := "6d1e2c3a-0000-4000-8000-000000000002"
	require.NoError(t, db.Create(&models.User{
		IAMSub:   userID,
		TenantID: "00000000-0000-0000-0000-000000000001",
		OrgID:    "00000000-0000-0000-0000-000000000000",
		Name:     "Existing",
		Role:     "USER",
		Status:   "active",
		IsShadow: true,
	}).Error)

	srv := newWxLoginMockServer(t, userID, "Existing")
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)

	router := gin.New()
	router.POST("/api/auth/wx-login", NewAuthHandler(db).WxLogin)

	body, _ := json.Marshal(map[string]interface{}{"code": "test-code-456"})
	req := httptest.NewRequest("POST", "/api/auth/wx-login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Token string `json:"token"`
			IsNew bool   `json:"is_new"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.False(t, resp.Data.IsNew)
}
