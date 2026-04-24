package handlers

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
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

func hasRequiredTables() bool {
	var count int64
	database.GetDB().Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_name IN ('merchants', 'site_members')").Scan(&count)
	return count >= 2
}

func hasUsers() bool {
	var count int64
	database.GetDB().Model(&models.User{}).Count(&count)
	return count > 0
}

func uuidPtrStr(s string) *uuid.UUID {
	u := uuid.MustParse(s)
	return &u
}

func initTestDB(t *testing.T) {
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)
}

func TestSetup_GetSetupStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	initTestDB(t)
	setupHandler := NewSetupHandler()

	router := gin.New()
	router.GET("/api/setup/status", setupHandler.GetSetupStatus)

	req := httptest.NewRequest("GET", "/api/setup/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, float64(20000), response["code"])

	data := response["data"].(map[string]interface{})
	requiresSetup := data["requires_setup"].(bool)
	assert.Equal(t, !hasUsers(), requiresSetup)
}

func TestMerchant_ListMerchants(t *testing.T) {
	if !hasRequiredTables() {
		t.Skip("merchants table not available")
		return
	}

	gin.SetMode(gin.TestMode)
	initTestDB(t)
	merchantHandler := NewMerchantHandler()

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
	router.GET("/api/merchants", merchantHandler.ListMerchants)

	req := httptest.NewRequest("GET", "/api/merchants", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, float64(20000), response["code"])
}

func TestMerchant_CreateMerchant(t *testing.T) {
	if !hasRequiredTables() {
		t.Skip("merchants table not available")
		return
	}

	gin.SetMode(gin.TestMode)
	merchantHandler := NewMerchantHandler()
	db := database.GetDB()

	tenantID := uuid.New().String()
	userID := uuid.New().String()
	orgID := uuid.New().String()
	testCode := "test-merchant-" + uuid.New().String()[:8]

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, "project_admin")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/merchants", merchantHandler.CreateMerchant)

	merchantReq := map[string]interface{}{
		"name":          "Test Merchant",
		"code":          testCode,
		"org_id":        orgID,
		"admin_uid":     userID,
		"contact_name":  "Test Contact",
		"contact_email": "test@example.com",
	}

	body, _ := json.Marshal(merchantReq)
	req := httptest.NewRequest("POST", "/api/merchants", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)
		if response["code"] == float64(20000) {
			data := response["data"].(map[string]interface{})
			assert.NotEmpty(t, data["id"])
			db.Where("id = ?", data["id"]).Delete(&models.Merchant{})
		}
	}
}

func TestMerchant_DeleteMerchant_WithActiveSites(t *testing.T) {
	if !hasRequiredTables() {
		t.Skip("merchants table not available")
		return
	}

	gin.SetMode(gin.TestMode)
	merchantHandler := NewMerchantHandler()
	db := database.GetDB()

	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	merchantID := uuid.New().String()
	siteID := uuid.New().String()

	merchant := models.Merchant{
		ID:       merchantID,
		TenantID: tenantID,
		OrgID:    orgID,
		Name:     "Test Merchant",
		Code:     "test-del-" + uuid.New().String()[:8],
		Status:   "active",
	}
	db.Create(&merchant)

	site := models.Site{
		ID:       siteID,
		TenantID: tenantID,
		Name:     "Test Site",
		Status:   "active",
	}
	db.Create(&site)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, uuid.New().String())
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, "project_admin")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.DELETE("/api/merchants/:id", merchantHandler.DeleteMerchant)

	req := httptest.NewRequest("DELETE", "/api/merchants/"+merchantID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["code"] == float64(40003) {
		assert.Contains(t, response["message"].(string), "active sites")
	}

	db.Where("id = ?", siteID).Delete(&models.Site{})
	db.Where("id = ?", merchantID).Delete(&models.Merchant{})
}

func TestSiteMember_ListMembers(t *testing.T) {
	if !hasRequiredTables() {
		t.Skip("site_members table not available")
		return
	}

	gin.SetMode(gin.TestMode)
	siteMemberHandler := NewSiteMemberHandler()
	db := database.GetDB()

	tenantID := uuid.New().String()
	userID := uuid.New().String()
	siteID := uuid.New().String()

	site := models.Site{
		ID:       siteID,
		TenantID: tenantID,
		Name:     "Test Site for Members",
		Status:   "active",
	}
	db.Create(&site)

	user := models.User{
		ID:       userID,
		TenantID: tenantID,
		Name:     "Test User",
		Email:    "testmember@example.com",
	}
	db.Create(&user)

	member := models.SiteMember{
		ID:       uuid.New().String(),
		TenantID: tenantID,
		SiteID:   siteID,
		UserID:   userID,
		Role:     "Manager",
	}
	db.Create(&member)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/sites/:id/members", siteMemberHandler.ListMembers)

	req := httptest.NewRequest("GET", "/api/sites/"+siteID+"/members", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)
		assert.Equal(t, float64(20000), response["code"])
	}

	db.Where("id = ?", member.ID).Delete(&models.SiteMember{})
	db.Where("id = ?", userID).Delete(&models.User{})
	db.Where("id = ?", siteID).Delete(&models.Site{})
}

func TestSiteMember_LastManagerProtection(t *testing.T) {
	if !hasRequiredTables() {
		t.Skip("site_members table not available")
		return
	}

	gin.SetMode(gin.TestMode)
	siteMemberHandler := NewSiteMemberHandler()
	db := database.GetDB()

	tenantID := uuid.New().String()
	managerUserID := uuid.New().String()
	siteID := uuid.New().String()

	site := models.Site{
		ID:       siteID,
		TenantID: tenantID,
		Name:     "Test Site Last Manager",
		Status:   "active",
	}
	db.Create(&site)

	user := models.User{
		ID:       managerUserID,
		TenantID: tenantID,
		Name:     "Last Manager",
		Email:    "lastmanager@example.com",
	}
	db.Create(&user)

	member := models.SiteMember{
		ID:       uuid.New().String(),
		TenantID: tenantID,
		SiteID:   siteID,
		UserID:   managerUserID,
		Role:     "Manager",
	}
	db.Create(&member)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, managerUserID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/sites/:siteId/members/:uid", siteMemberHandler.UpdateMemberRole)

	updateReq := map[string]interface{}{
		"role": "Staff",
	}
	body, _ := json.Marshal(updateReq)
	req := httptest.NewRequest("PUT", "/api/sites/"+siteID+"/members/"+managerUserID, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)
		if response["code"] == float64(40003) {
			assert.Contains(t, response["message"].(string), "last manager")
		}
	}

	db.Where("id = ?", member.ID).Delete(&models.SiteMember{})
	db.Where("id = ?", managerUserID).Delete(&models.User{})
	db.Where("id = ?", siteID).Delete(&models.Site{})
}

func TestSiteMember_RemoveLastManager(t *testing.T) {
	if !hasRequiredTables() {
		t.Skip("site_members table not available")
		return
	}

	gin.SetMode(gin.TestMode)
	siteMemberHandler := NewSiteMemberHandler()
	db := database.GetDB()

	tenantID := uuid.New().String()
	managerUserID := uuid.New().String()
	siteID := uuid.New().String()

	site := models.Site{
		ID:       siteID,
		TenantID: tenantID,
		Name:     "Test Site Remove Manager",
		Status:   "active",
	}
	db.Create(&site)

	user := models.User{
		ID:       managerUserID,
		TenantID: tenantID,
		Name:     "Manager To Remove",
		Email:    "managerremove@example.com",
	}
	db.Create(&user)

	member := models.SiteMember{
		ID:       uuid.New().String(),
		TenantID: tenantID,
		SiteID:   siteID,
		UserID:   managerUserID,
		Role:     "Manager",
	}
	db.Create(&member)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, managerUserID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.DELETE("/api/sites/:siteId/members/:uid", siteMemberHandler.RemoveMember)

	req := httptest.NewRequest("DELETE", "/api/sites/"+siteID+"/members/"+managerUserID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)
		if response["code"] == float64(40003) {
			assert.Contains(t, response["message"].(string), "last manager")
		}
	}

	db.Where("id = ?", member.ID).Delete(&models.SiteMember{})
	db.Where("id = ?", managerUserID).Delete(&models.User{})
	db.Where("id = ?", siteID).Delete(&models.Site{})
}

func TestSite_DeleteWithRentedInstruments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	siteHandler := NewSiteHandler()
	db := database.GetDB()

	tenantID := uuid.New().String()
	siteID := uuid.New().String()
	instrumentID := uuid.New().String()

	site := models.Site{
		ID:       siteID,
		TenantID: tenantID,
		Name:     "Test Site With Rented",
		Status:   "active",
	}
	db.Create(&site)

	instrument := models.Instrument{
		ID:          instrumentID,
		TenantID:    tenantID,
		SiteID:      uuidPtrStr(siteID),
		Name:        "Rented Instrument",
		StockStatus: "rented",
	}
	db.Create(&instrument)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, uuid.New().String())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.DELETE("/api/merchant/sites/:id", siteHandler.DeleteSite)

	req := httptest.NewRequest("DELETE", "/api/merchant/sites/"+siteID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)
		if response["code"] == float64(40003) {
			assert.Contains(t, response["message"].(string), "在租")
		}
	}

	db.Where("id = ?", instrumentID).Delete(&models.Instrument{})
	db.Where("id = ?", siteID).Delete(&models.Site{})
}
