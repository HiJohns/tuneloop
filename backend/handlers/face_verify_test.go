package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services/tencentcloud"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// fakeFaceProvider is a test double for FaceVerifyProvider.
type fakeFaceProvider struct {
	token   string
	passed  bool
	sim     float64
	wantErr bool
}

func (f *fakeFaceProvider) GetToken(name, idCard string) (string, error) {
	if f.wantErr {
		return "", tencentcloud.ErrNotConfigured
	}
	return f.token, nil
}

func (f *fakeFaceProvider) GetResult(bizToken string) (bool, float64, error) {
	if f.wantErr {
		return false, 0, tencentcloud.ErrNotConfigured
	}
	return f.passed, f.sim, nil
}

func faceRouter(provider tencentcloud.FaceVerifyProvider, userID, tenantID, orgID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := NewFaceVerifyHandler(provider)
	router.POST("/api/user/face-verify/token", h.Token)
	router.POST("/api/user/face-verify/result", h.Result)
	return router
}

func setupFaceVerifyTestUser(t *testing.T, tenantID, userID, orgID string) {
	t.Helper()
	db := database.GetDB()
	user := models.User{
		IAMSub:   userID,
		TenantID: tenantID,
		OrgID:    orgID,
		Name:     "test-user",
		Phone:    "13800138000",
		Status:   "active",
	}
	require.NoError(t, db.Create(&user).Error)
}

func TestFaceVerify_NotConfigured(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000101"
	userID := "00000000-0000-0000-0000-000000000102"
	orgID := "00000000-0000-0000-0000-000000000103"

	setupFaceVerifyTestUser(t, tenantID, userID, orgID)
	router := faceRouter(tencentcloud.NullFaceProvider{}, userID, tenantID, orgID)

	// Token should return 40012
	body, _ := json.Marshal(map[string]string{"name": "张三", "id_card_no": "110101199001011234"})
	req := httptest.NewRequest("POST", "/api/user/face-verify/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, float64(40012), resp["code"])

	// Result should also return 40012
	body2, _ := json.Marshal(map[string]string{"biz_token": "fake-token"})
	req2 := httptest.NewRequest("POST", "/api/user/face-verify/result", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	require.Equal(t, http.StatusBadRequest, w2.Code)
	var resp2 map[string]interface{}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp2))
	require.Equal(t, float64(40012), resp2["code"])
}

func TestFaceVerify_TokenSuccess(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000201"
	userID := "00000000-0000-0000-0000-000000000202"
	orgID := "00000000-0000-0000-0000-000000000203"

	setupFaceVerifyTestUser(t, tenantID, userID, orgID)
	router := faceRouter(&fakeFaceProvider{token: "test-biz-token-123"}, userID, tenantID, orgID)

	body, _ := json.Marshal(map[string]string{"name": "张三", "id_card_no": "110101199001011234"})
	req := httptest.NewRequest("POST", "/api/user/face-verify/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, float64(20000), resp["code"])
	data := resp["data"].(map[string]interface{})
	require.Equal(t, "test-biz-token-123", data["biz_token"])

	// Verify user record was updated with real_name and id_card_no
	db := database.GetDB()
	var user models.User
	require.NoError(t, db.Where("iam_sub = ?", userID).First(&user).Error)
	require.NotNil(t, user.RealName)
	require.Equal(t, "张三", *user.RealName)
	require.NotNil(t, user.IdCardNo)
	require.Equal(t, "110101199001011234", *user.IdCardNo)
}

func TestFaceVerify_TokenInvalidIdCard(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000301"
	userID := "00000000-0000-0000-0000-000000000302"
	orgID := "00000000-0000-0000-0000-000000000303"

	setupFaceVerifyTestUser(t, tenantID, userID, orgID)
	router := faceRouter(&fakeFaceProvider{token: "token"}, userID, tenantID, orgID)

	body, _ := json.Marshal(map[string]string{"name": "张三", "id_card_no": "12345"})
	req := httptest.NewRequest("POST", "/api/user/face-verify/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, float64(40002), resp["code"])
}

func TestFaceVerify_ResultPassed(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000401"
	userID := "00000000-0000-0000-0000-000000000402"
	orgID := "00000000-0000-0000-0000-000000000403"

	setupFaceVerifyTestUser(t, tenantID, userID, orgID)
	router := faceRouter(&fakeFaceProvider{passed: true, sim: 95.5}, userID, tenantID, orgID)

	body, _ := json.Marshal(map[string]string{"biz_token": "valid-token"})
	req := httptest.NewRequest("POST", "/api/user/face-verify/result", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, float64(20000), resp["code"])
	data := resp["data"].(map[string]interface{})
	require.Equal(t, true, data["passed"])
	require.InDelta(t, 95.5, data["similarity"], 0.1)

	// Verify user was marked as face_verified
	db := database.GetDB()
	var user models.User
	require.NoError(t, db.Where("iam_sub = ?", userID).First(&user).Error)
	require.True(t, user.FaceVerified)
	require.NotNil(t, user.FaceVerifiedAt)
}

func TestFaceVerify_ResultFailed(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000501"
	userID := "00000000-0000-0000-0000-000000000502"
	orgID := "00000000-0000-0000-0000-000000000503"

	setupFaceVerifyTestUser(t, tenantID, userID, orgID)
	router := faceRouter(&fakeFaceProvider{passed: false, sim: 40.0}, userID, tenantID, orgID)

	body, _ := json.Marshal(map[string]string{"biz_token": "valid-token"})
	req := httptest.NewRequest("POST", "/api/user/face-verify/result", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, float64(20000), resp["code"])
	data := resp["data"].(map[string]interface{})
	require.Equal(t, false, data["passed"])

	// Verify user was NOT marked as face_verified
	db := database.GetDB()
	var user models.User
	require.NoError(t, db.Where("iam_sub = ?", userID).First(&user).Error)
	require.False(t, user.FaceVerified)
}

func TestUpdateCurrentUser_IdCardNoAcceptance(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000701"
	userID := "00000000-0000-0000-0000-000000000702"
	orgID := "00000000-0000-0000-0000-000000000703"

	db := database.GetDB()
	user := models.User{
		IAMSub:   userID,
		TenantID: tenantID,
		OrgID:    orgID,
		Name:     "test-user",
		Phone:    "13800138002",
		Status:   "active",
	}
	require.NoError(t, db.Create(&user).Error)

	h := &UserStaffHandler{}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/users/me", h.UpdateCurrentUser)

	// Valid id_card_no format — should pass validation (may fail at IAM, that's OK)
	body, _ := json.Marshal(map[string]interface{}{
		"id_card_no": "110101199001011234",
	})
	req := httptest.NewRequest("PUT", "/api/users/me", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should NOT get 400 (validation error); 409 from IAM is acceptable
	require.NotEqual(t, http.StatusBadRequest, w.Code)
}


