package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

func setupUserStaffTest(t *testing.T) (db *gorm.DB, handler *UserStaffHandler, tenantID, userID string) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	dbConn, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(dbConn)

	db = database.GetDB()

	handler = &UserStaffHandler{}
	tenantID = uuid.New().String()
	userID = uuid.New().String()

	return db, handler, tenantID, userID
}

var testUserSeq int

func createTestUser(t *testing.T, db *gorm.DB, tenantID string) string {
	testUserSeq++
	userID := uuid.New().String()
	user := models.User{
		ID:        userID,
		IAMSub:    uuid.New().String(),
		TenantID:  tenantID,
		OrgID:     tenantID,
		Name:      "Test User",
		Phone:     fmt.Sprintf("138%08d", testUserSeq),
		Email:     fmt.Sprintf("test%d@example.com", testUserSeq),
		Status:    "active",
		Position:  "staff",
		Role:      "staff",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := db.Create(&user).Error
	require.NoError(t, err)
	return userID
}
func TestUpdateUser_SiteIDNoChange(t *testing.T) {
	_, handler, tenantID, _ := setupUserStaffTest(t)
	createdUserID := createTestUser(t, database.GetDB(), tenantID)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, createdUserID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/users/:id", handler.UpdateUser)

	reqBody := map[string]interface{}{
		"name": "Updated Name",
	}
	jsonBody, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/users/"+createdUserID, bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 20000, resp.Code)
}

func TestUpdateUser_UserNotFound(t *testing.T) {
	_, handler, tenantID, userID := setupUserStaffTest(t)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/users/:id", handler.UpdateUser)

	reqBody := map[string]interface{}{
		"name": "Ghost User",
	}
	jsonBody, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/users/"+uuid.New().String(), bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 40400, resp.Code)
}

func TestUpdateUser_SiteIDClear(t *testing.T) {
	db, handler, tenantID, _ := setupUserStaffTest(t)
	db.Where("iam_sub = ''").Delete(&models.User{})
	siteID := uuid.New().String()
	site := models.Site{
		ID:       siteID,
		TenantID: tenantID,
		OrgID:    tenantID,
		Name:     "Test Site",
		Status:   "active",
	}
	err := db.Create(&site).Error
	require.NoError(t, err)

	user := models.User{
		ID:        uuid.New().String(),
		IAMSub:    "",
		TenantID:  tenantID,
		OrgID:     tenantID,
		Name:      "Site User",
		Phone:     "13800000002",
		Status:    "active",
		Position:  "staff",
		Role:      "staff",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = db.Create(&user).Error
	require.NoError(t, err)

	siteMember := models.SiteMember{
		ID:     uuid.New().String(),
		SiteID: siteID,
		UserID: user.ID,
		Role:   "staff",
	}
	err = db.Create(&siteMember).Error
	require.NoError(t, err)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, user.ID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/users/:id", handler.UpdateUser)

	reqBody := map[string]interface{}{
		"site_id": uuid.Nil.String(),
	}
	jsonBody, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/users/"+user.ID, bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 20000, resp.Code)

	var memberCount int64
	db.Model(&models.SiteMember{}).Where("user_id = ?", user.ID).Count(&memberCount)
	assert.Equal(t, int64(0), memberCount)
}

func TestUpdateUser_SiteIDChanged_SiteNotFound(t *testing.T) {
	_, handler, tenantID, _ := setupUserStaffTest(t)
	createdUserID := createTestUser(t, database.GetDB(), tenantID)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, createdUserID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/users/:id", handler.UpdateUser)

	nonexistentSiteID := uuid.New().String()
	reqBody := map[string]interface{}{
		"site_id": nonexistentSiteID,
	}
	jsonBody, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/users/"+createdUserID, bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 40001, resp.Code)
}

// TestUpdateUser_SiteIDChange_UnbindFails_NoDbUpdate verifies that when IAM
// unbind fails during site_id change, the handler returns 500 and the user's
// site_id remains unchanged in the database.
//
// NOTE: The compensation rebind test (TestUpdateUser_SiteIDChange_BindFails_RebindCompensation)
// requires IAM client dependency injection for mocking. Currently NewIAMClient()
// is hard-coded and calls a real IAM service. This test covers the unbind failure
// path; the rebind compensation path can only be tested when IAM mocking is available.
func TestUpdateUser_SiteIDChange_UnbindFails_NoDbUpdate(t *testing.T) {
	db, handler, tenantID, _ := setupUserStaffTest(t)
	oldSiteID := uuid.New().String()
	newSiteID := uuid.New().String()

	oldSite := models.Site{
		ID:       oldSiteID,
		TenantID: tenantID,
		OrgID:    tenantID,
		Name:     "Old Site",
		Status:   "active",
	}
	err := db.Create(&oldSite).Error
	require.NoError(t, err)

	newSite := models.Site{
		ID:       newSiteID,
		TenantID: tenantID,
		OrgID:    tenantID,
		Name:     "New Site",
		Status:   "active",
	}
	err = db.Create(&newSite).Error
	require.NoError(t, err)

	userID := uuid.New().String()
	user := models.User{
		ID:       userID,
		IAMSub:   uuid.New().String(),
		TenantID: tenantID,
		OrgID:    tenantID,
		Name:     "Unbind Test User",
		Phone:    "13800000003",
		Status:   "active",
		Position: "staff",
		Role:     "staff",
	}
	err = db.Create(&user).Error
	require.NoError(t, err)

	siteMember := models.SiteMember{
		ID:     uuid.New().String(),
		SiteID: oldSiteID,
		UserID: userID,
		Role:   "staff",
	}
	err = db.Create(&siteMember).Error
	require.NoError(t, err)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/users/:id", handler.UpdateUser)

	reqBody := map[string]interface{}{
		"site_id": newSiteID,
	}
	jsonBody, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/users/"+userID, bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var memberCount int64
	db.Model(&models.SiteMember{}).Where("user_id = ?", userID).Count(&memberCount)
	require.Equal(t, int64(1), memberCount)
	var updatedMember models.SiteMember
	db.Where("user_id = ?", userID).First(&updatedMember)
	assert.Equal(t, oldSiteID, updatedMember.SiteID)
}
