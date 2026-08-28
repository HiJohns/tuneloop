package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services/tencentcloud"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// fakeOCRProvider is a test double for IDCardOCRProvider.
type fakeOCRProvider struct {
	info *tencentcloud.IDCardInfo
	err  error
}

func (f *fakeOCRProvider) RecognizeIDCard(_, _ string) (*tencentcloud.IDCardInfo, error) {
	return f.info, f.err
}

// guarantorOCRRouter creates a gin.Engine with OCRIDCard and CreateGuarantor routes.
func guarantorOCRRouter(provider tencentcloud.IDCardOCRProvider, userID, tenantID, orgID string) *gin.Engine {
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
	h := NewGuarantorHandler(provider)
	router.POST("/api/user/idcard-ocr", h.OCRIDCard)
	router.POST("/api/user/guarantors", h.CreateGuarantor)
	return router
}

// setupGuarantorTestUser creates a local user record for EnsureLocalUser.
func setupGuarantorTestUser(t *testing.T, tenantID, userID, orgID string) {
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

func TestIDCardOCR_NotConfigured(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000201"
	userID := "00000000-0000-0000-0000-000000000202"
	orgID := "00000000-0000-0000-0000-000000000203"

	setupGuarantorTestUser(t, tenantID, userID, orgID)
	router := guarantorOCRRouter(nil, userID, tenantID, orgID)

	// Create a fake image file in the relative path the handler expects
	imgName := uuid.New().String() + ".jpg"
	imgDir := filepath.Join("uploads", "media")
	require.NoError(t, os.MkdirAll(imgDir, 0755))
	imgPath := filepath.Join(imgDir, imgName)
	require.NoError(t, os.WriteFile(imgPath, []byte("fake-image"), 0644))
	defer os.Remove(imgPath)

	// Unset env vars to simulate "not configured"
	origID := os.Getenv("TENCENTCLOUD_SECRET_ID")
	origKey := os.Getenv("TENCENTCLOUD_SECRET_KEY")
	os.Unsetenv("TENCENTCLOUD_SECRET_ID")
	os.Unsetenv("TENCENTCLOUD_SECRET_KEY")
	defer func() {
		os.Setenv("TENCENTCLOUD_SECRET_ID", origID)
		os.Setenv("TENCENTCLOUD_SECRET_KEY", origKey)
	}()

	body, _ := json.Marshal(map[string]string{
		"storage_key": imgName,
		"side":        "FRONT",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/user/idcard-ocr", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, float64(20000), resp["code"])

	data := resp["data"].(map[string]interface{})
	require.Equal(t, false, data["available"])
}

func TestIDCardOCR_Success(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000211"
	userID := "00000000-0000-0000-0000-000000000212"
	orgID := "00000000-0000-0000-0000-000000000213"

	setupGuarantorTestUser(t, tenantID, userID, orgID)

	fakeInfo := &tencentcloud.IDCardInfo{
		Name:      "张三",
		Sex:       "男",
		Nation:    "汉",
		Birth:     "1990-01-01",
		Address:   "北京市朝阳区",
		IdNum:     "110101199001011234",
		Authority: "北京市公安局",
		ValidDate: "20200101-20300101",
		Warnings:  []string{"身份证复印件"},
	}
	router := guarantorOCRRouter(&fakeOCRProvider{info: fakeInfo}, userID, tenantID, orgID)

	// Create a fake image file in the relative path the handler expects
	imgName := uuid.New().String() + ".jpg"
	imgDir := filepath.Join("uploads", "media")
	require.NoError(t, os.MkdirAll(imgDir, 0755))
	imgPath := filepath.Join(imgDir, imgName)
	require.NoError(t, os.WriteFile(imgPath, []byte("fake-image"), 0644))
	defer os.Remove(imgPath)

	body, _ := json.Marshal(map[string]string{
		"storage_key": imgName,
		"side":        "FRONT",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/user/idcard-ocr", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, float64(20000), resp["code"])

	data := resp["data"].(map[string]interface{})
	require.Equal(t, true, data["available"])
	require.Equal(t, "张三", data["name"])
	require.Equal(t, "110101199001011234", data["id_num"])
	require.Equal(t, "北京市朝阳区", data["address"])

	warnings := data["warnings"].([]interface{})
	require.Len(t, warnings, 1)
	require.Equal(t, "身份证复印件", warnings[0])
}

func TestGuarantor_Create_WithIDFields(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000221"
	userID := "00000000-0000-0000-0000-000000000222"
	orgID := "00000000-0000-0000-0000-000000000223"

	setupGuarantorTestUser(t, tenantID, userID, orgID)
	router := guarantorOCRRouter(nil, userID, tenantID, orgID)

	body, _ := json.Marshal(map[string]string{
		"name":             "李四",
		"phone":            "13900139000",
		"id_card_no":       "110101199001011234",
		"id_photo_front":   "/uploads/media/front.jpg",
		"id_photo_back":    "/uploads/media/back.jpg",
		"other_cert_photo": "/uploads/media/other.jpg",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/user/guarantors", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, float64(20000), resp["code"])

	data := resp["data"].(map[string]interface{})
	require.Equal(t, "李四", data["name"])
	require.Equal(t, "110101199001011234", data["id_card_no"])
	require.Equal(t, "/uploads/media/front.jpg", data["id_photo_front"])
	require.Equal(t, "/uploads/media/back.jpg", data["id_photo_back"])
	require.Equal(t, "/uploads/media/other.jpg", data["other_cert_photo"])

	// Verify DB record exists
	db := database.GetDB()
	var g models.Guarantor
	require.NoError(t, db.Where("id = ?", data["id"].(string)).First(&g).Error)
	require.Equal(t, "110101199001011234", g.IDCardNo)
	// Verify user_id matches the local user created by EnsureLocalUser
	var u models.User
	require.NoError(t, db.Where("iam_sub = ?", userID).First(&u).Error)
	require.Equal(t, u.ID, g.UserID)
}

func TestGuarantor_Create_MissingIDPhoto(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000231"
	userID := "00000000-0000-0000-0000-000000000232"
	orgID := "00000000-0000-0000-0000-000000000233"

	setupGuarantorTestUser(t, tenantID, userID, orgID)
	router := guarantorOCRRouter(nil, userID, tenantID, orgID)

	// Missing id_photo_front — binding:"required" should reject
	body, _ := json.Marshal(map[string]string{
		"name":           "王五",
		"phone":          "13700137000",
		"id_card_no":     "110101199001011234",
		"id_photo_back":  "/uploads/media/back.jpg",
		"other_cert_photo": "/uploads/media/other.jpg",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/user/guarantors", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, float64(40002), resp["code"])
}

func TestGuarantor_Create_InvalidIDNum(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000241"
	userID := "00000000-0000-0000-0000-000000000242"
	orgID := "00000000-0000-0000-0000-000000000243"

	setupGuarantorTestUser(t, tenantID, userID, orgID)
	router := guarantorOCRRouter(nil, userID, tenantID, orgID)

	// 15-digit ID number — should fail 18-digit validation
	body, _ := json.Marshal(map[string]string{
		"name":             "赵六",
		"phone":            "13600136000",
		"id_card_no":       "110101199001011",
		"id_photo_front":   "/uploads/media/front.jpg",
		"id_photo_back":    "/uploads/media/back.jpg",
		"other_cert_photo": "/uploads/media/other.jpg",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/user/guarantors", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, float64(40002), resp["code"])
}
