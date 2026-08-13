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
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
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
	return newWxLoginMockServerWithStatus(t, userID, name, http.StatusOK, "")
}

// newWxLoginMockServerWithStatus serves a configurable wx-login response:
// status=200 → success token; otherwise → JSON error body with errBody.
func newWxLoginMockServerWithStatus(t *testing.T, userID, name string, status int, errBody string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/wx-login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(errBody))
			return
		}
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
// client-credentials token, user creation, wx-accounts (pre-check), wx-bind,
// and wx-login returning an HS256 token so ValidateToken accepts it locally.
// accountsResp overrides the wx-accounts response (default: empty accounts).
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
	mux.HandleFunc("/api/v1/auth/wx-accounts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"openid":   "openid-register-001",
			"accounts": []interface{}{},
		})
	})
	mux.HandleFunc("/api/v1/auth/wx-bind", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"user_id":   newUserID,
			"wx_openid": "openid-register-001",
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
			"nickname": "微信昵称",
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

// TestPostRegister_NoPassword_WxBind_FullFlow verifies #1597/#1571: H5
// registration without a password must (1) generate a random IAM password,
// (2) bind the WeChat openid when wx_code is present, (3) sync the local
// users cache with onboarding flags, (4) credit registration gift points,
// and (5) return a usable token.
func TestPostRegister_NoPassword_WxBind_FullFlow(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.Referral{}, &models.PointsTransaction{}, &models.SystemSetting{}))
	db.Exec("DELETE FROM referrals")
	db.Exec("DELETE FROM points_transactions")
	db.Exec("DELETE FROM system_settings")

	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")

	newUserID := "6d1e2c3a-0000-4000-8000-0000000000ee"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]string
		json.Unmarshal(body, &req)
		if req["grant_type"] == "password" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token":  signHS256Token(t, newUserID, "NoPassUser"),
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
	mux.HandleFunc("/api/v1/auth/wx-bind", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"user_id":   newUserID,
			"wx_openid": "openid-nopass-001",
		})
	})
	mux.HandleFunc("/api/v1/auth/wx-accounts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"openid":   "openid-nopass-001",
			"accounts": []interface{}{},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)

	router := gin.New()
	router.POST("/api/auth/register", NewAuthHandler(db).PostRegister)

	body, _ := json.Marshal(map[string]interface{}{
		"name":     "无密码注册用户",
		"nickname": "无密码昵称",
		"phone":    "13900221133",
		"wx_code":  "wx-register-code",
	})
	req := httptest.NewRequest("POST", "/api/auth/register", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			AccessToken string  `json:"access_token"`
			MembershipFee float64 `json:"membership_fee"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.NotEmpty(t, resp.Data.AccessToken, "register must return a usable token")
	require.Equal(t, 99.0, resp.Data.MembershipFee, "default membership fee")

	// Local cache synced with onboarding flags + wx openid bound.
	var local models.User
	require.NoError(t, db.Where("iam_sub = ?", newUserID).First(&local).Error)
	require.Equal(t, "USER", local.Role)
	require.True(t, local.IsProfileCompleted, "registration collects all onboarding fields (#1597)")
	require.True(t, local.OnboardingCompleted)
	require.Equal(t, "openid-nopass-001", local.WxOpenid, "wx_code must bind the openid (#1597)")
	require.NotEmpty(t, local.RefCode, "ref_code derived from user id")

	// Registration gift points credited (#1533).
	require.Equal(t, 99.0, local.PromoPoints, "registration gift points")
	var pt models.PointsTransaction
	require.NoError(t, db.Where("user_id = ? AND type = ?", local.ID, "registration").First(&pt).Error)
	require.Equal(t, 99.0, pt.Amount)
}

// newWxAccountsMockServer serves GET /api/v1/auth/wx-accounts with the given
// account list (0/1/N routing tests).
func newWxAccountsMockServer(t *testing.T, accounts []map[string]interface{}) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/wx-accounts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"openid":   "openid-accounts-001",
			"accounts": accounts,
		})
	})
	return httptest.NewServer(mux)
}

// TestWxAccounts_Handler verifies GET /auth/wx-accounts: code required, and
// is_customer enrichment (org_id/tenant_id empty → customer), plus merchant/site
// display names for staff accounts (#1641).
func TestWxAccounts_Handler(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()

	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")

	// Merchant + Site rows so the staff account gets display names.
	require.NoError(t, db.Create(&models.Merchant{
		ID:       "6d1e2c3a-0000-4000-8000-0000000000f5",
		TenantID: "6d1e2c3a-0000-4000-8000-0000000000f4",
		OrgID:    "6d1e2c3a-0000-4000-8000-0000000000f3",
		AdminUID: "6d1e2c3a-0000-4000-8000-0000000000f2",
		Name:     "测试琴行",
	}).Error)
	require.NoError(t, db.Create(&models.Site{
		ID:       "6d1e2c3a-0000-4000-8000-0000000000f6",
		TenantID: "6d1e2c3a-0000-4000-8000-0000000000f4",
		OrgID:    "6d1e2c3a-0000-4000-8000-0000000000f3",
		Name:     "旗舰店",
	}).Error)

	accounts := []map[string]interface{}{
		{
			"user_id": "6d1e2c3a-0000-4000-8000-0000000000f1", "name": "顾客小张",
			"nickname": "小张", "role": "USER", "org_id": "", "tenant_id": "",
		},
		{
			"user_id": "6d1e2c3a-0000-4000-8000-0000000000f2", "name": "员工小李",
			"nickname": "小李", "role": "STAFF",
			"org_id": "6d1e2c3a-0000-4000-8000-0000000000f3",
			"tenant_id": "6d1e2c3a-0000-4000-8000-0000000000f4",
		},
		{
			"user_id": "6d1e2c3a-0000-4000-8000-0000000000f7", "name": "员工无商户",
			"nickname": "小孙", "role": "STAFF",
			"org_id": "6d1e2c3a-0000-4000-8000-0000000000f8",
			"tenant_id": "6d1e2c3a-0000-4000-8000-0000000000f9",
		},
	}
	srv := newWxAccountsMockServer(t, accounts)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)

	router := gin.New()
	router.GET("/api/auth/wx-accounts", NewAuthHandler(db).WxAccounts)

	t.Run("missing code is 400", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/auth/wx-accounts", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("zero accounts returns empty list", func(t *testing.T) {
		emptySrv := newWxAccountsMockServer(t, []map[string]interface{}{})
		defer emptySrv.Close()
		services.SetIAMInternalURLForTesting(emptySrv.URL)

		// Rebuild handler so IAMService picks up the new mock URL
		router2 := gin.New()
		router2.GET("/api/auth/wx-accounts", NewAuthHandler(db).WxAccounts)

		req := httptest.NewRequest("GET", "/api/auth/wx-accounts?code=test-code-zero", nil)
		w := httptest.NewRecorder()
		router2.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp struct {
			Code int `json:"code"`
			Data struct {
				OpenID   string                   `json:"openid"`
				Accounts []map[string]interface{} `json:"accounts"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, 20000, resp.Code)
		require.Equal(t, "openid-accounts-001", resp.Data.OpenID)
		require.Empty(t, resp.Data.Accounts, "zero accounts → empty list")
	})

	t.Run("single account returns one entry", func(t *testing.T) {
		oneSrv := newWxAccountsMockServer(t, []map[string]interface{}{
			{
				"user_id": "6d1e2c3a-0000-4000-8000-0000000000f1", "name": "顾客小张",
				"nickname": "小张", "role": "USER", "org_id": "", "tenant_id": "",
			},
		})
		defer oneSrv.Close()
		services.SetIAMInternalURLForTesting(oneSrv.URL)

		// Rebuild handler so IAMService picks up the new mock URL
		router3 := gin.New()
		router3.GET("/api/auth/wx-accounts", NewAuthHandler(db).WxAccounts)

		req := httptest.NewRequest("GET", "/api/auth/wx-accounts?code=test-code-one", nil)
		w := httptest.NewRecorder()
		router3.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp struct {
			Code int `json:"code"`
			Data struct {
				OpenID   string `json:"openid"`
				Accounts []struct {
					UserID     string `json:"user_id"`
					IsCustomer bool   `json:"is_customer"`
				} `json:"accounts"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, 20000, resp.Code)
		require.Len(t, resp.Data.Accounts, 1)
		require.True(t, resp.Data.Accounts[0].IsCustomer, "single customer account")
	})

	t.Run("accounts returned with is_customer", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/auth/wx-accounts?code=test-code", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp struct {
			Code int `json:"code"`
			Data struct {
				OpenID   string `json:"openid"`
				Accounts []struct {
					UserID       string `json:"user_id"`
					IsCustomer   bool   `json:"is_customer"`
					OrgID        string `json:"org_id"`
					TenantID     string `json:"tenant_id"`
					MerchantName string `json:"merchant_name"`
					SiteName     string `json:"site_name"`
				} `json:"accounts"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, 20000, resp.Code)
		require.Equal(t, "openid-accounts-001", resp.Data.OpenID)
		require.Len(t, resp.Data.Accounts, 3)
		require.True(t, resp.Data.Accounts[0].IsCustomer, "no org/tenant → customer")
		require.False(t, resp.Data.Accounts[1].IsCustomer, "has org/tenant → staff")
		require.False(t, resp.Data.Accounts[2].IsCustomer, "staff without merchant rows")

		// Customer account: no merchant/site enrichment.
		require.Empty(t, resp.Data.Accounts[0].MerchantName)
		require.Empty(t, resp.Data.Accounts[0].SiteName)

		// Staff account with matching merchant/site rows.
		require.Equal(t, "测试琴行", resp.Data.Accounts[1].MerchantName)
		require.Equal(t, "旗舰店", resp.Data.Accounts[1].SiteName)

		// Staff account without merchant/site rows: empty strings, no error.
		require.Empty(t, resp.Data.Accounts[2].MerchantName)
		require.Empty(t, resp.Data.Accounts[2].SiteName)
	})
}

// TestWxLoginSelect_Handler verifies POST /auth/wx-login-select: params
// required, token returned with user_id forwarded.
func TestWxLoginSelect_Handler(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()

	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")

	userID := "6d1e2c3a-0000-4000-8000-0000000000f5"
	srv := newWxLoginMockServer(t, userID, "SelectUser")
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)

	router := gin.New()
	router.POST("/api/auth/wx-login-select", NewAuthHandler(db).WxLoginSelect)

	t.Run("missing user_id is 400", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{"exchange_token": "tok-1"})
		req := httptest.NewRequest("POST", "/api/auth/wx-login-select", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing exchange_token is 400", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{"user_id": userID})
		req := httptest.NewRequest("POST", "/api/auth/wx-login-select", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("select login returns token", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{"exchange_token": "tok-1", "user_id": userID})
		req := httptest.NewRequest("POST", "/api/auth/wx-login-select", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp struct {
			Code int `json:"code"`
			Data struct {
				AccessToken string `json:"access_token"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, 20000, resp.Code)
		require.NotEmpty(t, resp.Data.AccessToken)
	})
}

// TestWxLoginSelect_FriendlyErrors verifies IAM failures are mapped to
// user-friendly messages, never raw IAM error passthrough (#1643).
func TestWxLoginSelect_FriendlyErrors(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()

	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")

	selectRouter := func(srv *httptest.Server) *gin.Engine {
		services.SetIAMInternalURLForTesting(srv.URL)
		router := gin.New()
		router.POST("/api/auth/wx-login-select", NewAuthHandler(db).WxLoginSelect)
		return router
	}
	postSelect := func(router *gin.Engine) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]interface{}{"exchange_token": "tok-1", "user_id": "6d1e2c3a-0000-4000-8000-0000000000f5"})
		req := httptest.NewRequest("POST", "/api/auth/wx-login-select", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	t.Run("403 account not active → 账户已停用", func(t *testing.T) {
		srv := newWxLoginMockServerWithStatus(t, "u1", "n1", http.StatusForbidden, `{"error":"account not active"}`)
		defer srv.Close()
		w := postSelect(selectRouter(srv))
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Contains(t, w.Body.String(), "账户已停用")
		require.NotContains(t, w.Body.String(), "IAM wx-login returned")
	})

	t.Run("404 wx_user_not_found → 注册引导", func(t *testing.T) {
		srv := newWxLoginMockServerWithStatus(t, "u1", "n1", http.StatusNotFound, `{"error":"wx_user_not_found"}`)
		defer srv.Close()
		w := postSelect(selectRouter(srv))
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Contains(t, w.Body.String(), "该微信号尚未注册")
		require.NotContains(t, w.Body.String(), "wx-login-select failed")
	})

	t.Run("other error → generic 500", func(t *testing.T) {
		srv := newWxLoginMockServerWithStatus(t, "u1", "n1", http.StatusBadGateway, `{"error":"upstream down"}`)
		defer srv.Close()
		w := postSelect(selectRouter(srv))
		require.Equal(t, http.StatusInternalServerError, w.Code)
		require.Contains(t, w.Body.String(), "微信登录失败")
		require.NotContains(t, w.Body.String(), "upstream down")
	})
}

// TestPostRegister_PhoneExists_409 verifies duplicate phone registration is
// rejected with a friendly 409 (#1644): CreateUser conflict → "该手机号或邮箱
// 已注册" instead of raw IAM error passthrough.
func TestPostRegister_PhoneExists_409(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.Referral{}, &models.PointsTransaction{}, &models.SystemSetting{}))
	db.Exec("DELETE FROM referrals")
	db.Exec("DELETE FROM points_transactions")

	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")

	// CreateUser endpoint returns 409 conflict → registration must reject
	// with the friendly duplicate message and NOT create a user.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-client-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"phone already exists"}`))
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)

	router := gin.New()
	router.POST("/api/auth/register", NewAuthHandler(db).PostRegister)

	body, _ := json.Marshal(map[string]interface{}{
		"name":     "新用户",
		"nickname": "新昵称",
		"phone":    "13900331122",
		"wx_code":  "wx-code-dup",
	})
	req := httptest.NewRequest("POST", "/api/auth/register", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code)
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 40900, resp.Code)
	require.Contains(t, resp.Message, "该手机号或邮箱已注册")
	require.NotContains(t, resp.Message, "IAM register failed")
}

// TestPostRegister_WxBindFailure_Aborts verifies the #1637 red-line fix:
// a WxBind failure must abort registration with the error surfaced (409),
// never log-and-continue with a successful registration.
func TestPostRegister_WxBindFailure_Aborts(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.Referral{}, &models.PointsTransaction{}, &models.SystemSetting{}))
	db.Exec("DELETE FROM referrals")
	db.Exec("DELETE FROM points_transactions")

	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")

	newUserID := "6d1e2c3a-0000-4000-8000-0000000000f7"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-client-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
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
	mux.HandleFunc("/api/v1/auth/wx-accounts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"openid":   "openid-bindfail-001",
			"accounts": []interface{}{},
		})
	})
	// wx-bind fails (409 already bound) — registration MUST abort (409),
	// and the freshly created user must be purged (#1644 rollback).
	mux.HandleFunc("/api/v1/auth/wx-bind", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": "openid already bound"})
	})
	purged := false
	mux.HandleFunc("/api/v1/users/"+newUserID+"/purge", func(w http.ResponseWriter, r *http.Request) {
		purged = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "purged"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)

	router := gin.New()
	router.POST("/api/auth/register", NewAuthHandler(db).PostRegister)

	body, _ := json.Marshal(map[string]interface{}{
		"name":     "绑定失败用户",
		"nickname": "绑定失败",
		"phone":    "13900442255",
		"wx_code":  "wx-code-bind-fail",
	})
	req := httptest.NewRequest("POST", "/api/auth/register", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code)
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 40900, resp.Code)
	require.Contains(t, resp.Message, "微信绑定失败")
	require.True(t, purged, "WxBind failure must purge the freshly created user")
}

// TestPostRegister_ExchangeToken_Flow verifies the #1644 register path: an
// exchange_token (minted by wx-accounts) is forwarded to IAM wx-bind instead
// of the raw WeChat code — no code re-consumption (40163).
func TestPostRegister_ExchangeToken_Flow(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.Referral{}, &models.PointsTransaction{}, &models.SystemSetting{}))
	db.Exec("DELETE FROM referrals")
	db.Exec("DELETE FROM points_transactions")

	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")

	newUserID := "6d1e2c3a-0000-4000-8000-0000000000ff"
	var bindBody map[string]string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-client-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
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
	mux.HandleFunc("/api/v1/auth/wx-bind", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&bindBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"user_id":   newUserID,
			"wx_openid": "openid-exchangetoken-001",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)

	router := gin.New()
	router.POST("/api/auth/register", NewAuthHandler(db).PostRegister)

	body, _ := json.Marshal(map[string]interface{}{
		"name":           "exchange_token 注册用户",
		"nickname":       "token 昵称",
		"phone":          "13900556677",
		"exchange_token": "exch-tok-abc123",
	})
	req := httptest.NewRequest("POST", "/api/auth/register", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotNil(t, bindBody, "wx-bind must be called")
	require.Equal(t, "exch-tok-abc123", bindBody["exchange_token"])
	require.Equal(t, "", bindBody["code"], "raw code must NOT be sent when exchange_token is used")
}

// TestPostRegister_NicknameRequired verifies the #1638 contract: nickname is
// a required field (binding:"required") — a register request without it
// must be rejected with 400 before any IAM call.
func TestPostRegister_NicknameRequired(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()

	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")

	router := gin.New()
	router.POST("/api/auth/register", NewAuthHandler(db).PostRegister)

	body, _ := json.Marshal(map[string]interface{}{
		"name":  "无昵称用户",
		"phone": "13900556677",
	})
	req := httptest.NewRequest("POST", "/api/auth/register", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 40002, resp.Code)
	require.Contains(t, resp.Message, "Nickname")
}

// TestPostRegister_UsernameDerivedFromPhone verifies the #1638 data-integrity
// contract: the legacy username input path is removed — a request that sends
// a username field must have it ignored, and the IAM CreateUser payload must
// carry username = phone.
func TestPostRegister_UsernameDerivedFromPhone(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.Referral{}, &models.PointsTransaction{}, &models.SystemSetting{}))
	db.Exec("DELETE FROM referrals")
	db.Exec("DELETE FROM points_transactions")

	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")

	newUserID := "6d1e2c3a-0000-4000-8000-0000000000f8"
	var createdUsername string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]string
		json.Unmarshal(body, &req)
		if req["grant_type"] == "password" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token":  signHS256Token(t, newUserID, "DerivedUser"),
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
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			body, _ := io.ReadAll(r.Body)
			var req struct {
				Username string `json:"username"`
			}
			json.Unmarshal(body, &req)
			createdUsername = req.Username
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
	mux.HandleFunc("/api/v1/auth/wx-accounts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"openid":   "openid-derived-001",
			"accounts": []interface{}{},
		})
	})
	mux.HandleFunc("/api/v1/auth/wx-bind", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"user_id":   newUserID,
			"wx_openid": "openid-derived-001",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)

	router := gin.New()
	router.POST("/api/auth/register", NewAuthHandler(db).PostRegister)

	// Send a legacy username field that must be IGNORED (#1638)
	body, _ := json.Marshal(map[string]interface{}{
		"username": "legacy_username_ignored",
		"name":     "派生用户",
		"nickname": "派生昵称",
		"phone":    "13900668899",
		"wx_code":  "wx-code-derived",
	})
	req := httptest.NewRequest("POST", "/api/auth/register", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "13900668899", createdUsername,
		"IAM CreateUser username must be derived from phone, not the ignored username field")
}

// TestWxBindCurrentUser verifies POST /api/users/me/wx-bind (#1639 计划 §五):
// binds the current user (JWT sub = iam_sub) to the WeChat openid via IAM,
// surfaces bind failures instead of swallowing them.
func TestWxBindCurrentUser(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()

	t.Setenv("IAM_SECRET", testIAMSecret)
	t.Setenv("IAM_NAMESPACE", "test-ns")

	userID := "6d1e2c3a-0000-4000-8000-0000000000f9"
	token := signHS256Token(t, userID, "BindUser")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/wx-bind", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]string
		json.Unmarshal(body, &req)
		if req["code"] == "bad-code" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"error": "openid already bound to another user"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"user_id":   userID,
			"wx_openid": "openid-bind-me-001",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)

	router := gin.New()
	router.POST("/api/users/me/wx-bind", (&UserStaffHandler{}).WxBindCurrentUser)

	doBind := func(code string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]interface{}{"code": code})
		req := httptest.NewRequest("POST", "/api/users/me/wx-bind", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		// Simulate IAMInterceptor: JWT sub → context user id
		ctx := context.WithValue(req.Context(), middleware.ContextKeyUserID, userID)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	t.Run("missing code is 400", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/users/me/wx-bind", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		ctx := context.WithValue(req.Context(), middleware.ContextKeyUserID, userID)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("successful bind returns wx_openid", func(t *testing.T) {
		w := doBind("good-code")
		require.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Code int `json:"code"`
			Data struct {
				WxOpenid string `json:"wx_openid"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, 20000, resp.Code)
		require.Equal(t, "openid-bind-me-001", resp.Data.WxOpenid)
	})

	t.Run("bind failure surfaces 409 not swallowed", func(t *testing.T) {
		w := doBind("bad-code")
		require.Equal(t, http.StatusConflict, w.Code)
		var resp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, 40900, resp.Code)
		require.Contains(t, resp.Message, "微信绑定失败")
	})
}
