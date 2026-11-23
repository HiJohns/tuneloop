package handlers

// #2454 R4：PC 回调必须拒绝「空组织上下文」token（50202），并且不写 auth cookie；
// H5/小程序分支（client_type=wx）不受影响（D4）。

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
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/services"
)

// signEmptyContextToken2454 签发一个「空组织上下文」token：oid/tid 为空、
// 无 fn_roles、零权限、role=USER —— 即多组织账号未选择组织时 IAM 曾返回的形状。
func signEmptyContextToken2454(t *testing.T) string {
	t.Helper()
	claims := services.JWTClaims{
		UserID: "22bfb6d9-9365-46bd-9660-c036f229ce0e",
		Name:   "test2446",
		Role:   "USER",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testIAMSecret))
	require.NoError(t, err)
	return signed
}

func newCallbackIAMMock2454(t *testing.T, accessToken string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  accessToken,
			"refresh_token": "refresh-2454",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	t.Cleanup(srv.Close)
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	return srv
}

// hasAuthCookie2454 判断响应是否下发了会话 cookie（token / refresh_token）。
// oauth_state 的清除也会产生 Set-Cookie，不计入会话 cookie。
func hasAuthCookie2454(values []string) bool {
	for _, v := range values {
		name := strings.TrimSpace(strings.SplitN(v, "=", 2)[0])
		if name == "token" || name == "refresh_token" {
			return true
		}
	}
	return false
}

func callbackRouter2454(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	r := gin.New()
	r.GET("/api/auth/callback", NewAuthHandler(db).Callback)
	return r
}

func TestPCCallbackRejectsEmptyOrgContext_2454(t *testing.T) {
	newCallbackIAMMock2454(t, signEmptyContextToken2454(t))
	r := callbackRouter2454(t)

	// PC 分支：client_type 缺失
	req := httptest.NewRequest(http.MethodGet, "/api/auth/callback?code=pc-code", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), w.Body.String())

	require.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())
	require.EqualValues(t, 50202, resp["code"])
	require.Contains(t, resp["message"], "登录上下文缺失")
	require.False(t, hasAuthCookie2454(w.Header().Values("Set-Cookie")), "拒绝时不得写 auth cookie")

	t.Logf("PC_HTTP=%d", w.Code)
	t.Logf("CODE=%v", resp["code"])
	t.Logf("COOKIE_SET=0")
}

func TestH5CallbackKeepsEmptyContextToken_2454(t *testing.T) {
	newCallbackIAMMock2454(t, signEmptyContextToken2454(t))
	r := callbackRouter2454(t)

	// H5 分支：client_type=wx —— 顾客/小程序上下文允许 oid/tid 为空
	req := httptest.NewRequest(http.MethodGet, "/api/auth/callback?code=wx-code&client_type=wx", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.True(t, hasAuthCookie2454(w.Header().Values("Set-Cookie")), "H5 分支仍应设置会话 cookie")

	t.Logf("H5_HTTP=%d", w.Code)
}
