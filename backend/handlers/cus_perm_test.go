package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupCusPermTestDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
	}
	database.SetDB(db)
	_ = db
}

func Test_CreateMerchant_CusPerm(t *testing.T) {
	skipIfNoIAM(t)
	setupCusPermTestDB(t)

	iamClient := services.NewIAMClient()
	nsID := os.Getenv("IAM_NAMESPACE")
	if nsID == "" {
		t.Skip("IAM_NAMESPACE not set")
	}

	ts := time.Now().Format("150405")
	name := "e2e_m_" + ts
	email := "e2e_ma_" + ts + "@tuneloop.com"

	body := map[string]interface{}{
		"name":            name,
		"admin_name":      "E2E Admin",
		"admin_email":     email,
		"admin_phone":     "13800000000",
		"skip_activation": true,
	}
	jsonBody, _ := json.Marshal(body)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, nsID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, nsID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, nsID)
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, "NAMESPACE_ADMIN")
		ctx = context.WithValue(ctx, middleware.ContextKeyGid, nsID)
		ctx = context.WithValue(ctx, middleware.ContextKeyNamespaceID, nsID)
		ctx = context.WithValue(ctx, middleware.ContextKeyIsOwner, true)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	handler := NewMerchantHandler()
	router.POST("/api/merchants", handler.CreateMerchant)
	req := httptest.NewRequest("POST", "/api/merchants", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, 201, w.Code, "CreateMerchant should return 201, body: %s", w.Body.String())

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	require.Equal(t, float64(20100), resp["code"], "response code should be 20100")

	data := resp["data"].(map[string]interface{})
	orgID := data["iam_org_id"].(string)
	require.NotEmpty(t, orgID)
	initialPassword, hasPassword := data["initial_password"]
	require.True(t, hasPassword, "initial_password should be present when skip_activation=true")
	require.NotEmpty(t, initialPassword, "initial_password should not be empty")

	users, err := iamClient.ListUsers()
	require.NoError(t, err)

	var adminID string
	for _, u := range users {
		if u.Email == email {
			adminID = u.ID
			break
		}
	}
	require.NotEmpty(t, adminID, "admin user %s should exist in IAM", email)

	_, cusPermInt, err := iamClient.GetUserCustomerPermissions(orgID, adminID)
	require.NoError(t, err, "GetUserCustomerPermissions for %s in org %s should succeed", adminID, orgID)
	require.NotEqual(t, int64(0), cusPermInt, "cus_perm should not be 0 for merchant_admin, got %d", cusPermInt)

	t.Logf("PASS: cus_perm=%d for %s", cusPermInt, email)
}

func Test_CreateSite_CusPerm(t *testing.T) {
	skipIfNoIAM(t)
	setupCusPermTestDB(t)

	iamClient := services.NewIAMClient()
	nsID := os.Getenv("IAM_NAMESPACE")
	if nsID == "" {
		t.Skip("IAM_NAMESPACE not set")
	}

	orgs, err := iamClient.ListOrganizations()
	require.NoError(t, err)
	require.NotEmpty(t, orgs, "need at least one org in IAM")
	parentOrgID := orgs[0].ID

	ts := time.Now().Format("150405")
	siteName := "e2e_s_" + ts
	email := "e2e_sa_" + ts + "@tuneloop.com"

	body := map[string]interface{}{
		"name":          siteName,
		"manager_name":  "E2E SiteAdmin",
		"manager_email": email,
		"manager_phone": "13800000001",
	}
	jsonBody, _ := json.Marshal(body)

	users, _ := iamClient.ListUsers()
	require.NotEmpty(t, users)
	userID := users[0].ID

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, parentOrgID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, parentOrgID)
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, "OWNER")
		ctx = context.WithValue(ctx, middleware.ContextKeyGid, nsID)
		ctx = context.WithValue(ctx, middleware.ContextKeyNamespaceID, nsID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	siteHandler := NewSiteHandler()
	router.POST("/api/merchant/sites", siteHandler.CreateSite)
	req := httptest.NewRequest("POST", "/api/merchant/sites", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, 201, w.Code, "CreateSite should return 201, body: %s", w.Body.String())

	users2, err := iamClient.ListUsers()
	require.NoError(t, err)

	var adminID string
	for _, u := range users2 {
		if u.Email == email {
			adminID = u.ID
			break
		}
	}
	require.NotEmpty(t, adminID, "site_admin should exist in IAM")

	_, cusPermInt, err := iamClient.GetUserCustomerPermissions(parentOrgID, adminID)
	require.NoError(t, err, "GetUserCustomerPermissions should succeed")
	require.NotEqual(t, int64(0), cusPermInt, "cus_perm should not be 0 for site_admin")

	t.Logf("PASS: cus_perm=%d for %s", cusPermInt, email)
}

func Test_AddMember_CusPerm(t *testing.T) {
	skipIfNoIAM(t)
	t.Skip("AddMember test pending — requires site + member handler wiring")
}
