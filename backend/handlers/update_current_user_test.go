package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
	"tuneloop-backend/services"
)

// setupUpdateCurrentUserTest spins up a mock IAM server that handles
// PUT /api/v1/users/:id (the endpoint UpdateCurrentUser proxies to) and
// returns the given status. It returns the tuneloop router wired to the
// handler plus cleanup.
func setupUpdateCurrentUserTest(t *testing.T, iamStatus int) (*gorm.DB, http.Handler, string, string) {
	cleanup := setupMockIAMAndDB(t)
	t.Cleanup(cleanup)

	iamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/token" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"access_token":"mock-token","expires_in":3600,"token_type":"Bearer"}`)
			return
		}
		if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/users/") {
			if iamStatus == http.StatusOK {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, `{"code":20000,"data":{"id":"`+r.URL.Path[10:]+`"}}`)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(iamStatus)
			fmt.Fprint(w, `{"code":40900,"message":"phone already registered"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(iamSrv.Close)
	services.SetIAMInternalURLForTesting(iamSrv.URL)

	db := database.GetDB()
	db.Exec("ALTER TABLE users ADD COLUMN IF NOT EXISTS iam_sub VARCHAR(255) NOT NULL DEFAULT ''")

	tenantID := uuid.New().String()
	userID := uuid.New().String()
	user := models.User{
		ID:        userID,
		IAMSub:    userID,
		TenantID:  tenantID,
		OrgID:     tenantID,
		Name:      "Original Name",
		Phone:     "13900000000",
		Email:     "orig@example.com",
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&user).Error)

	handler := &UserStaffHandler{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/users/me", handler.UpdateCurrentUser)

	return db, router, tenantID, userID
}

// TestUpdateCurrentUser_IAMConflict_AbortsLocalUpdate verifies #1600: when
// IAM rejects the profile update (409, e.g. duplicate phone/email), the
// handler must abort and NOT write the local DB cache.
func TestUpdateCurrentUser_IAMConflict_AbortsLocalUpdate(t *testing.T) {
	db, router, _, userID := setupUpdateCurrentUserTest(t, http.StatusConflict)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"name":  "Conflicting Name",
		"phone": "13911112222",
	})
	req := httptest.NewRequest("PUT", "/api/users/me", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 40900, resp.Code, "conflict must be surfaced to the frontend")

	// Local cache must NOT be updated (the IAM call precedes local writes).
	var user models.User
	require.NoError(t, db.Where("id = ?", userID).First(&user).Error)
	assert.Equal(t, "Original Name", user.Name, "local cache must stay stale on IAM conflict")
	assert.Equal(t, "13900000000", user.Phone)
}

// TestUpdateCurrentUser_IAMSuccess_UpdatesLocalCache verifies the positive
// path of #1600: IAM accepts → local cache is synced.
func TestUpdateCurrentUser_IAMSuccess_UpdatesLocalCache(t *testing.T) {
	db, router, _, userID := setupUpdateCurrentUserTest(t, http.StatusOK)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"name":  "New Name",
		"phone": "13933334444",
	})
	req := httptest.NewRequest("PUT", "/api/users/me", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 20000, resp.Code)

	var user models.User
	require.NoError(t, db.Where("id = ?", userID).First(&user).Error)
	assert.Equal(t, "New Name", user.Name)
	assert.Equal(t, "13933334444", user.Phone)
}
