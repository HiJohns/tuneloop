package handlers

import (
	"encoding/json"
	"io"
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

// newRegisterMockServer serves the IAM endpoints PostRegister needs:
// client-credentials token, user creation, and wx-login returning an HS256
// token so ValidateToken accepts it locally.
func newRegisterMockServer(t *testing.T, newUserID, name string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		// client_credentials (IAMClient.CreateUser) and password grant
		// (IAMService.IAMLogin fallback) both hit this endpoint.
		body, _ := io.ReadAll(r.Body)
		var req map[string]string
		json.Unmarshal(body, &req)
		if req["grant_type"] == "password" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token":  signHS256Token(t, newUserID, name),
				"expires_in":    7200,
				"token_type":    "Bearer",
				"refresh_token": "",
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-client-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/api/v1/auth/wx-login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  signHS256Token(t, newUserID, name),
			"expires_in":    7200,
			"token_type":    "Bearer",
			"refresh_token": "",
		})
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"user_id": newUserID,
					"status":  "active",
				},
			})
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})
	return httptest.NewServer(mux)
}

// TestPostRegister_RefConsumption verifies #1496: the ref parameter is only
// consumed (referrals row created) when wx_code is provided.
func TestPostRegister_RefConsumption(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.Referral{}))
	db.Exec("DELETE FROM referrals")

	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")

	// Existing referrer with a ref_code
	referrerID := "6d1e2c3a-0000-4000-8000-0000000000aa"
	require.NoError(t, db.Create(&models.User{
		ID:       referrerID,
		IAMSub:   referrerID,
		TenantID: "00000000-0000-0000-0000-000000000001",
		OrgID:    "00000000-0000-0000-0000-000000000000",
		Name:     "Referrer",
		Role:     "USER",
		Status:   "active",
		RefCode:  "abc12345",
	}).Error)

	newUserID := "6d1e2c3a-0000-4000-8000-0000000000bb"
	srv := newRegisterMockServer(t, newUserID, "NewUser")
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)

	router := gin.New()
	router.POST("/api/auth/register", NewAuthHandler(db).PostRegister)

	register := func(wxCode, ref string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]interface{}{
			"name":     "New User",
			"phone":    "13900139000",
			"password": "secret123",
			"wx_code":  wxCode,
			"ref":      ref,
		})
		req := httptest.NewRequest("POST", "/api/auth/register", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	t.Run("wx_code_with_ref_creates_referral", func(t *testing.T) {
		w := register("test-wx-code", "abc12345")
		require.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Code int `json:"code"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, 20000, resp.Code)

		var count int64
		require.NoError(t, db.Model(&models.Referral{}).
			Where("referrer_id = ? AND ref_code = ? AND status = ?", referrerID, "abc12345", "registered").
			Count(&count).Error)
		require.Equal(t, int64(1), count, "referral row must be created when wx_code + ref are provided")
	})

	t.Run("no_ref_skips_referral", func(t *testing.T) {
		// different wx code → new local user
		otherUser := "6d1e2c3a-0000-4000-8000-0000000000cc"
		srv2 := newRegisterMockServer(t, otherUser, "OtherUser")
		defer srv2.Close()
		services.SetIAMInternalURLForTesting(srv2.URL)

		w := register("test-wx-code-2", "")
		require.Equal(t, http.StatusOK, w.Code)

		var count int64
		require.NoError(t, db.Model(&models.Referral{}).Count(&count).Error)
		require.Equal(t, int64(1), count, "no referral row created without ref param")
	})

	t.Run("no_wx_code_skips_ref_consumption", func(t *testing.T) {
		// H5 register: no wx_code → ref must NOT be consumed.
		// PostRegister falls to the IAMLogin-after-register path; use an
		// independent mock user so iam_sub is fresh.
		h5User := "6d1e2c3a-0000-4000-8000-0000000000dd"
		srv3 := newRegisterMockServer(t, h5User, "H5User")
		defer srv3.Close()
		services.SetIAMInternalURLForTesting(srv3.URL)

		w := register("", "abc12345")
		require.Equal(t, http.StatusOK, w.Code)

		var count int64
		require.NoError(t, db.Model(&models.Referral{}).Count(&count).Error)
		require.Equal(t, int64(1), count, "ref must not be consumed when wx_code is empty")
	})
}
