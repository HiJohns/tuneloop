package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

func skipIfNoIAM(t *testing.T) {
	if os.Getenv("BEACONIAM_INTERNAL_URL") == "" {
		t.Skip("BEACONIAM_INTERNAL_URL not set, skipping IAM integration test")
	}
	if os.Getenv("IAM_PC_CLIENT_ID") == "" || os.Getenv("IAM_PC_CLIENT_SECRET") == "" {
		t.Skip("IAM credentials not set, skipping IAM integration test")
	}
}

func setupIAMTestDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)
}

func TestIAMIntegration_MerchantCreate(t *testing.T) {
	skipIfNoIAM(t)
	setupIAMTestDB(t)

	db := database.GetDB()

	tenantID := uuid.New().String()
	userID := uuid.New().String()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, "project_admin")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	merchantHandler := NewMerchantHandler()
	router.POST("/api/merchants", merchantHandler.CreateMerchant)

	testCode := "test-merchant-" + uuid.New().String()[:8]
	merchantReq := map[string]interface{}{
		"name":           "Test Merchant IAM",
		"code":           testCode,
		"address":        "Test Address",
		"contact_phone":  "13800138000",
		"admin_name":     "Test Admin",
		"admin_username": "testadmin_" + uuid.New().String()[:8],
		"admin_email":    "test_" + uuid.New().String()[:8] + "@example.com",
		"admin_phone":    "13800138000",
	}

	body, _ := json.Marshal(merchantReq)
	req := httptest.NewRequest("POST", "/api/merchants", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusCreated || w.Code == http.StatusOK {
		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)
		assert.Equal(t, float64(20100), response["code"].(float64))

		data := response["data"].(map[string]interface{})
		assert.NotEmpty(t, data["id"])
		assert.NotEmpty(t, data["iam_org_id"], "org_id should be returned from IAM")
		assert.NotEmpty(t, data["iam_admin_id"], "admin_id should be returned from IAM")
		assert.Equal(t, "pending", data["confirmation"])

		db.Where("id = ?", data["id"]).Delete(&models.Merchant{})
	} else {
		t.Logf("Response: %s", w.Body.String())
	}
}

func TestIAMIntegration_SiteCreate(t *testing.T) {
	skipIfNoIAM(t)
	setupIAMTestDB(t)

	db := database.GetDB()

	tenantID := uuid.New().String()
	userID := uuid.New().String()
	parentOrgID := uuid.New().String()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, parentOrgID)
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, "merchant_admin")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	siteHandler := NewSiteHandler()
	router.POST("/api/merchant/sites", siteHandler.CreateSite)

	siteReq := map[string]interface{}{
		"name":           "Test Site IAM",
		"address":        "Test Site Address",
		"type":           "store",
		"latitude":       39.9042,
		"longitude":      116.4074,
		"phone":          "13800138001",
		"business_hours": "09:00-18:00",
	}

	body, _ := json.Marshal(siteReq)
	req := httptest.NewRequest("POST", "/api/merchant/sites", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusCreated || w.Code == http.StatusOK {
		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)
		assert.Equal(t, float64(20000), response["code"])

		data := response["data"].(map[string]interface{})
		site := data["site"].(map[string]interface{})
		assert.NotEmpty(t, site["id"])
		assert.NotEmpty(t, data["iam_org_id"], "site.org_id should be returned from IAM")
		assert.NotEqual(t, parentOrgID, data["iam_org_id"], "site.org_id should be different from merchant org_id")

		db.Where("id = ?", site["id"]).Delete(&models.Site{})
	} else {
		t.Logf("Response: %s", w.Body.String())
	}
}

func TestIAMIntegration_ConfirmationCallback(t *testing.T) {
	setupIAMTestDB(t)

	db := database.GetDB()

	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	userID := uuid.New().String()
	sessionID := uuid.New().String()

	session := models.ConfirmationSession{
		ID:             sessionID,
		TenantID:       tenantID,
		OrgID:          orgID,
		UserID:         userID,
		ConfirmType:    "email",
		ConfirmTarget:  "test@example.com",
		ActionType:     "site_staff",
		ActionTargetID: uuid.New().String(),
		Status:         "waiting",
		Token:          uuid.New().String(),
		ExpiresAt:      time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, db.Create(&session).Error)

	user := models.User{
		ID:          userID,
		TenantID:    tenantID,
		OrgID:       orgID,
		Name:        "Test User",
		Email:       "test@example.com",
		Status:      "pending",
		IsShadow:    true,
		CreditScore: 600,
		DepositMode: "standard",
	}
	require.NoError(t, db.Create(&user).Error)

	router := gin.New()
	confirmationHandler := NewConfirmationSessionHandler()
	router.GET("/api/iam/confirmation-callback", confirmationHandler.IAMConfirmationCallback)

	req := httptest.NewRequest("GET", "/api/iam/confirmation-callback?session="+sessionID+"&result=accept", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	location := w.Header().Get("Location")
	assert.Contains(t, location, "status=success")

	var updatedSession models.ConfirmationSession
	db.First(&updatedSession, "id = ?", sessionID)
	assert.Equal(t, "confirmed", updatedSession.Status)

	var updatedUser models.User
	db.First(&updatedUser, "id = ?", userID)
	assert.Equal(t, "active", updatedUser.Status)

	db.Where("id = ?", sessionID).Delete(&models.ConfirmationSession{})
	db.Where("id = ?", userID).Delete(&models.User{})
}

func TestIAMIntegration_ConfirmationCallback_Reject(t *testing.T) {
	setupIAMTestDB(t)

	db := database.GetDB()

	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	userID := uuid.New().String()
	sessionID := uuid.New().String()

	session := models.ConfirmationSession{
		ID:            sessionID,
		TenantID:      tenantID,
		OrgID:         orgID,
		UserID:        userID,
		ConfirmType:   "email",
		ConfirmTarget: "test@example.com",
		ActionType:    "merchant_admin",
		Status:        "waiting",
		Token:         uuid.New().String(),
		ExpiresAt:     time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, db.Create(&session).Error)

	router := gin.New()
	confirmationHandler := NewConfirmationSessionHandler()
	router.GET("/api/iam/confirmation-callback", confirmationHandler.IAMConfirmationCallback)

	req := httptest.NewRequest("GET", "/api/iam/confirmation-callback?session="+sessionID+"&result=reject", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	location := w.Header().Get("Location")
	assert.Contains(t, location, "status=rejected")

	var updatedSession models.ConfirmationSession
	db.First(&updatedSession, "id = ?", sessionID)
	assert.Equal(t, "rejected", updatedSession.Status)

	db.Where("id = ?", sessionID).Delete(&models.ConfirmationSession{})
}

func TestIAMIntegration_UserUpdate_EmailChange(t *testing.T) {
	skipIfNoIAM(t)
	setupIAMTestDB(t)

	db := database.GetDB()

	tenantID := uuid.New().String()
	userID := uuid.New().String()
	iamSub := uuid.New().String()

	user := models.User{
		ID:          userID,
		IAMSub:      iamSub,
		TenantID:    tenantID,
		Name:        "Test User",
		Email:       "old@example.com",
		Phone:       "13800138000",
		Status:      "active",
		IsShadow:    false,
		CreditScore: 600,
		DepositMode: "standard",
	}
	require.NoError(t, db.Create(&user).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	iamProxyHandler := NewIAMProxyHandler()
	router.PUT("/api/iam/users/:id", iamProxyHandler.UpdateIAMUser)

	updateReq := map[string]interface{}{
		"name":  "Updated Name",
		"email": "new@example.com",
		"phone": "13900139000",
	}

	body, _ := json.Marshal(updateReq)
	req := httptest.NewRequest("PUT", "/api/iam/users/"+iamSub, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)
		assert.Equal(t, float64(20000), response["code"])

		data := response["data"].(map[string]interface{})
		assert.Equal(t, "pending", data["email_confirmation"])
	}

	db.Where("id = ?", userID).Delete(&models.User{})
}

func setupMockIAMAndDB(t *testing.T) func() {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
	}
	database.SetDB(db)

	_ = db.Migrator().DropTable(&models.ConfirmationSession{})
	_ = db.Migrator().DropTable(&models.SiteMember{})
	_ = db.Migrator().DropTable(&models.Site{})
	_ = db.Migrator().DropTable(&models.Merchant{})
	_ = db.Migrator().DropTable(&models.User{})
	require.NoError(t, db.Migrator().CreateTable(&models.User{}))
	require.NoError(t, db.Migrator().CreateTable(&models.Merchant{}))
	require.NoError(t, db.Migrator().CreateTable(&models.Site{}))
	require.NoError(t, db.Migrator().CreateTable(&models.SiteMember{}))
	require.NoError(t, db.Migrator().CreateTable(&models.ConfirmationSession{}))

	return func() { services.SetIAMInternalURLForTesting("") }
}

func newMockIAMServer(orgHandler, userHandler http.HandlerFunc) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	if orgHandler != nil {
		mux.HandleFunc("/api/v1/namespaces/", orgHandler)
	}
	if userHandler != nil {
		mux.HandleFunc("/api/v1/users/", userHandler)
	}
	return httptest.NewServer(mux)
}

func TestIAMMock_MerchantCreate_CallsIAM(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()

	orgCalled := false
	var orgPayload map[string]interface{}

	mockIAM := newMockIAMServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			orgCalled = true
			json.NewDecoder(r.Body).Decode(&orgPayload)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"org_id":   "mock-org-id",
					"admin_id": "mock-admin-id",
				},
			})
		}
	}, nil)
	defer mockIAM.Close()
	services.SetIAMInternalURLForTesting(mockIAM.URL)

	tenantID := uuid.New().String()
	userID := uuid.New().String()

	adminUser := models.User{
		ID:       userID,
		IAMSub:   userID,
		TenantID: tenantID,
		Name:     "Admin",
		Email:    "admin@mock.com",
		Phone:    "13800000000",
	}
	require.NoError(t, db.Create(&adminUser).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "project-admin")
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, "project_admin")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	handler := NewMerchantHandler()
	router.POST("/api/merchants", handler.CreateMerchant)

	body, _ := json.Marshal(map[string]interface{}{
		"name":         "Mock Merchant",
		"code":         "MOCK" + uuid.New().String()[:8],
		"admin_uid":    userID,
		"contact_name": "Contact",
	})
	req := httptest.NewRequest("POST", "/api/merchants", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.True(t, orgCalled, "IAM CreateOrganization should be called")
	assert.Equal(t, "Mock Merchant", orgPayload["name"])
	assert.NotNil(t, orgPayload["admin_info"])

	var merchant models.Merchant
	require.NoError(t, db.Where("tenant_id = ?", tenantID).First(&merchant).Error)
	assert.Equal(t, "mock-org-id", merchant.OrgID)
}

func TestIAMMock_SiteMemberAdd_CallsBind(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()

	bindCalled := false
	var bindPayload map[string]interface{}

	mockIAM := newMockIAMServer(nil, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			bindCalled = true
			json.NewDecoder(r.Body).Decode(&bindPayload)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code":    20000,
				"message": "success",
			})
		}
	})
	defer mockIAM.Close()
	services.SetIAMInternalURLForTesting(mockIAM.URL)

	tenantID := uuid.New().String()
	siteOrgID := "site-org-mock"

	site := models.Site{
		Name:     "Mock Site",
		TenantID: tenantID,
		OrgID:    siteOrgID,
		Status:   "active",
	}
	require.NoError(t, db.Create(&site).Error)

	user := models.User{
		ID:       uuid.New().String(),
		IAMSub:   "iam-user-mock",
		TenantID: tenantID,
		Name:     "Mock User",
		Email:    "user@mock.com",
	}
	require.NoError(t, db.Create(&user).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "operator")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	handler := NewSiteMemberHandler()
	router.POST("/api/sites/:id/members", handler.AddMember)

	body, _ := json.Marshal(map[string]interface{}{
		"user_ids": []map[string]interface{}{
			{"user_id": user.ID, "role": "Staff"},
		},
	})
	req := httptest.NewRequest("POST", "/api/sites/"+site.ID+"/members", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.True(t, bindCalled, "IAM Bind API should be called")
	assert.Equal(t, "bind", bindPayload["action"])
	assert.Equal(t, "staff", bindPayload["role"])

	var member models.SiteMember
	require.NoError(t, db.Where("site_id = ? AND user_id = ?", site.ID, user.ID).First(&member).Error)
	assert.Equal(t, "Staff", member.Role)
}

func TestIAMMock_ConfirmationCallback_Failed(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()

	tenantID := uuid.New().String()
	userID := uuid.New().String()
	sessionID := uuid.New().String()

	user := models.User{
		ID:       userID,
		IAMSub:   userID,
		TenantID: tenantID,
		Name:     "Failed Test User",
		Email:    "failed@mock.com",
		Status:   "pending",
	}
	require.NoError(t, db.Create(&user).Error)

	session := models.ConfirmationSession{
		ID:            sessionID,
		TenantID:      tenantID,
		OrgID:         uuid.New().String(),
		UserID:        userID,
		ConfirmType:   "email",
		ConfirmTarget: "failed@mock.com",
		ActionType:    "merchant_admin",
		Status:        "waiting",
		Token:         uuid.New().String(),
		ExpiresAt:     time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, db.Create(&session).Error)

	handler := NewConfirmationSessionHandler()
	router := gin.New()
	router.GET("/api/iam/confirmation-callback", handler.IAMConfirmationCallback)

	req := httptest.NewRequest("GET", "/api/iam/confirmation-callback?session="+sessionID+"&result=failed", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	location := w.Header().Get("Location")
	assert.Contains(t, location, "status=failed")

	var updated models.ConfirmationSession
	require.NoError(t, db.Where("id = ?", sessionID).First(&updated).Error)
	assert.Equal(t, "failed", updated.Status)
}

func TestIAMMock_UserUpdate_EmailChange_ReturnsPending(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()

	updateCalled := false
	mockIAM := newMockIAMServer(nil, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			updateCalled = true
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code":    20000,
				"message": "success",
			})
		}
	})
	defer mockIAM.Close()
	services.SetIAMInternalURLForTesting(mockIAM.URL)

	tenantID := uuid.New().String()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "operator")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	handler := NewIAMProxyHandler()
	router.PUT("/api/iam/users/:id", handler.UpdateIAMUser)

	body, _ := json.Marshal(map[string]interface{}{
		"name":  "Updated Name",
		"email": "newemail@mock.com",
		"phone": "13900139000",
	})
	req := httptest.NewRequest("PUT", "/api/iam/users/some-iam-id", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.True(t, updateCalled, "IAM UpdateUser should be called")
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	data := response["data"].(map[string]interface{})
	assert.Equal(t, "pending", data["email_confirmation"])
}
