package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// newWxAPIStub serves both /cgi-bin/token and /wxa/getwxacodeunlimit.
// failWxacode=true makes the wxacode endpoint return a JSON error.
func newWxAPIStub(t *testing.T, failWxacode bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/cgi-bin/token"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"mock_wx_token","expires_in":7200}`))
		case strings.Contains(r.URL.Path, "/wxa/getwxacodeunlimit"):
			if failWxacode {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"errcode":40013,"errmsg":"invalid appid"}`))
				return
			}
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("fake-wxacode-image-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
}

// genBindTokenRouter wires GenBindToken with a context carrying user_id.
func genBindTokenRouter(iamSub string) *gin.Engine {
	router := gin.New()
	router.POST("/users/me/wechat-bind", func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyUserID, iamSub)
		c.Request = c.Request.WithContext(ctx)
		NewWechatBindHandler().GenBindToken(c)
	})
	return router
}

func TestGenBindToken_Success(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()

	user := models.User{ID: "6d1e2c3a-0000-4000-8000-000000000101", IAMSub: "iam-sub-0001", TenantID: "00000000-0000-0000-0000-000000000001", OrgID: "00000000-0000-0000-0000-000000000000", Username: "bindtest"}
	require.NoError(t, db.Create(&user).Error)

	wxStub := newWxAPIStub(t, false)
	defer wxStub.Close()
	services.SetWxAPIBaseURLForTesting(wxStub.URL)
	t.Setenv("WX_APPID", "wx-test-appid")
	t.Setenv("WX_SECRET", "wx-test-secret")
	t.Setenv("IAM_NAMESPACE", "test-ns")

	router := genBindTokenRouter("iam-sub-0001")
	req := httptest.NewRequest("POST", "/users/me/wechat-bind", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Token         string `json:"token"`
			WxacodeBase64 string `json:"wxacode_base64"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)

	// Token must be 16 hex chars so scene "bind_<token>" fits the 32-char limit.
	require.Len(t, resp.Data.Token, 16)
	scene := "bind_" + resp.Data.Token
	require.LessOrEqual(t, len(scene), 32)

	// wxacode_base64 must decode to the stubbed image bytes.
	raw, err := base64.StdEncoding.DecodeString(resp.Data.WxacodeBase64)
	require.NoError(t, err)
	require.Equal(t, "fake-wxacode-image-bytes", string(raw))

	// PollBindToken must report pending right after generation.
	require.Equal(t, "pending", lookupBindStatus(resp.Data.Token))
}

func TestGenBindToken_WxacodeFailure(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()

	user := models.User{ID: "6d1e2c3a-0000-4000-8000-000000000102", IAMSub: "iam-sub-0002", TenantID: "00000000-0000-0000-0000-000000000001", OrgID: "00000000-0000-0000-0000-000000000000", Username: "bindtest2"}
	require.NoError(t, db.Create(&user).Error)

	wxStub := newWxAPIStub(t, true)
	defer wxStub.Close()
	services.SetWxAPIBaseURLForTesting(wxStub.URL)
	t.Setenv("WX_APPID", "wx-test-appid")
	t.Setenv("WX_SECRET", "wx-test-secret")

	router := genBindTokenRouter("iam-sub-0002")
	req := httptest.NewRequest("POST", "/users/me/wechat-bind", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Red-line: a wxacode failure MUST surface as 500, never a partial success.
	require.Equal(t, http.StatusInternalServerError, w.Code)
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Message)
}

func TestGenBindToken_UserNotFound(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()

	router := genBindTokenRouter("iam-sub-unknown")
	req := httptest.NewRequest("POST", "/users/me/wechat-bind", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestPollBindToken_States(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()

	token := "0123456789abcdef"
	bindTokensMu.Lock()
	bindTokens[token] = &bindTokenEntry{
		UserID:    "6d1e2c3a-0000-4000-8000-000000000103",
		Status:    "pending",
		CreatedAt: time.Now(),
	}
	bindTokensMu.Unlock()

	router := gin.New()
	handler := NewWechatBindHandler()
	router.GET("/users/me/wechat-bind/:token", handler.PollBindToken)

	// pending
	req := httptest.NewRequest("GET", "/users/me/wechat-bind/"+token, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"status":"pending"`)

	// bound
	bindTokensMu.Lock()
	bindTokens[token].Status = "bound"
	bindTokensMu.Unlock()
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/users/me/wechat-bind/"+token, nil))
	require.Contains(t, w.Body.String(), `"status":"bound"`)

	// expired: old token must report expired
	bindTokensMu.Lock()
	bindTokens[token].CreatedAt = time.Now().Add(-10 * time.Minute)
	bindTokensMu.Unlock()
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/users/me/wechat-bind/"+token, nil))
	require.Contains(t, w.Body.String(), `"status":"expired"`)

	// unknown token → expired
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/users/me/wechat-bind/ffffffffffffffff", nil))
	require.Contains(t, w.Body.String(), `"status":"expired"`)

	bindTokensMu.Lock()
	delete(bindTokens, token)
	bindTokensMu.Unlock()
}

func TestConfirmBind_Success(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()

	user := models.User{ID: "6d1e2c3a-0000-4000-8000-000000000104", IAMSub: "iam-sub-0004", TenantID: "00000000-0000-0000-0000-000000000001", OrgID: "00000000-0000-0000-0000-000000000000", Username: "bindtest4"}
	require.NoError(t, db.Create(&user).Error)

	// Mock IAM so UpdateUser succeeds.
	mockIAM := newMockIAMServer(nil, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":20000}`))
	})
	defer mockIAM.Close()
	services.SetIAMInternalURLForTesting(mockIAM.URL)

	token := "abcd1234abcd1234"
	bindTokensMu.Lock()
	bindTokens[token] = &bindTokenEntry{
		UserID:    user.ID,
		Status:    "pending",
		CreatedAt: time.Now(),
	}
	bindTokensMu.Unlock()

	router := gin.New()
	router.POST("/wechat-bind/confirm", NewWechatBindHandler().ConfirmBind)

	body, _ := json.Marshal(map[string]string{"token": token, "wx_openid": "oa7cSxREALOPENID"})
	req := httptest.NewRequest("POST", "/wechat-bind/confirm", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)

	// Local cache must now carry the real openid.
	var updated models.User
	require.NoError(t, db.First(&updated, "id = ?", user.ID).Error)
	require.Equal(t, "oa7cSxREALOPENID", updated.WxOpenid)

	// Token consumed: second confirm with same token must fail.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/wechat-bind/confirm", bytes.NewReader(body)))
	require.Equal(t, http.StatusBadRequest, w.Code)

	bindTokensMu.Lock()
	delete(bindTokens, token)
	bindTokensMu.Unlock()
}

func TestConfirmBind_MissingParams(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()

	router := gin.New()
	router.POST("/wechat-bind/confirm", NewWechatBindHandler().ConfirmBind)

	// no token
	body, _ := json.Marshal(map[string]string{"wx_openid": "oa7cSxREALOPENID"})
	req := httptest.NewRequest("POST", "/wechat-bind/confirm", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	// no wx_openid
	body, _ = json.Marshal(map[string]string{"token": "abcd1234abcd1234"})
	req = httptest.NewRequest("POST", "/wechat-bind/confirm", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func lookupBindStatus(token string) string {
	bindTokensMu.RLock()
	defer bindTokensMu.RUnlock()
	entry, ok := bindTokens[token]
	if !ok {
		return "not_found"
	}
	return entry.Status
}
