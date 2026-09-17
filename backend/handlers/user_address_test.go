package handlers

// #1930: 用户地址端点空 userID → uuid 22P02 回归测试。
// 覆盖：匿名 5 端点 401（不再进 SQL）；登录态 List/Create；shadow 用户自动建档。

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

func setupUserAddressTest(t *testing.T, iamSub string) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyUserID, iamSub)
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, "")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := NewUserAddressHandler()
	router.GET("/api/user/addresses", h.ListAddresses)
	router.POST("/api/user/addresses", h.CreateAddress)
	router.PUT("/api/user/addresses/:id", h.UpdateAddress)
	router.PUT("/api/user/addresses/:id/default", h.SetDefaultAddress)
	router.DELETE("/api/user/addresses/:id", h.DeleteAddress)
	return router, db
}

func addrCall(t *testing.T, router *gin.Engine, method, path string, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	var buf *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	} else {
		buf = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

func TestUserAddress_Anonymous401(t *testing.T) {
	router, _ := setupUserAddressTest(t, "") // 匿名：iam_sub 为空
	id := uuid.New().String()
	validBody := map[string]interface{}{"recipient_name": "A", "phone": "13800000000", "detail": "x"}
	cases := []struct {
		method string
		path   string
		body   interface{}
	}{
		{"GET", "/api/user/addresses", nil},
		{"POST", "/api/user/addresses", validBody},
		{"PUT", "/api/user/addresses/" + id, validBody},
		{"PUT", "/api/user/addresses/" + id + "/default", nil},
		{"DELETE", "/api/user/addresses/" + id, nil},
	}
	for _, tc := range cases {
		code, resp := addrCall(t, router, tc.method, tc.path, tc.body)
		assert.Equal(t, http.StatusUnauthorized, code, "%s %s 应 401", tc.method, tc.path)
		assert.Equal(t, float64(40001), resp["code"], "%s %s code 应 40001", tc.method, tc.path)
	}
}

func TestUserAddress_LoggedIn_CRUD(t *testing.T) {
	iamSub := uuid.New().String()
	router, db := setupUserAddressTest(t, iamSub)
	require.NoError(t, db.Exec(`INSERT INTO users (id, iam_sub, tenant_id, org_id, name, status, is_shadow, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'U', 'active', false, now(), now())`,
		uuid.New().String(), iamSub, uuid.New().String(), uuid.New().String()).Error)

	// List（登录态）→ 200
	code, resp := addrCall(t, router, "GET", "/api/user/addresses", nil)
	require.Equal(t, http.StatusOK, code, "%v", resp)
	assert.Equal(t, float64(20000), resp["code"])

	// Create → 201
	code, resp = addrCall(t, router, "POST", "/api/user/addresses", map[string]interface{}{
		"recipient_name": "张三", "phone": "13800000000", "province": "北京", "city": "北京",
		"district": "朝阳", "detail": "某路1号", "is_default": true,
	})
	require.Equal(t, http.StatusCreated, code, "%v", resp)
	assert.Equal(t, float64(20000), resp["code"])
	addrID := resp["data"].(map[string]interface{})["id"].(string)

	// Update → 200
	code, _ = addrCall(t, router, "PUT", "/api/user/addresses/"+addrID, map[string]interface{}{
		"recipient_name": "李四", "phone": "13800000001", "detail": "某路2号",
	})
	assert.Equal(t, http.StatusOK, code)

	// SetDefault → 200
	code, _ = addrCall(t, router, "PUT", "/api/user/addresses/"+addrID+"/default", nil)
	assert.Equal(t, http.StatusOK, code)

	// Delete → 200
	code, _ = addrCall(t, router, "DELETE", "/api/user/addresses/"+addrID, nil)
	assert.Equal(t, http.StatusOK, code)
}

func TestUserAddress_ShadowUserAutoCreated(t *testing.T) {
	iamSub := uuid.New().String()
	router, db := setupUserAddressTest(t, iamSub) // 无本地 users 记录

	code, resp := addrCall(t, router, "POST", "/api/user/addresses", map[string]interface{}{
		"recipient_name": "王五", "phone": "13900000000", "detail": "shadow 路1号",
	})
	require.Equal(t, http.StatusCreated, code, "%v", resp)
	assert.Equal(t, float64(20000), resp["code"])

	var u models.User
	require.NoError(t, db.Where("iam_sub = ?", iamSub).First(&u).Error, "EnsureLocalUser 应自动建 shadow 用户")
	assert.True(t, u.IsShadow)
}
