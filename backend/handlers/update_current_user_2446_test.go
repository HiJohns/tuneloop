package handlers

// #2446: P2 credential setup — username/password update via PUT /users/me and
// needs_credentials marker on GET /users/me.

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
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

type credIAMStub struct {
	updateStatus int
	updateBody   map[string]interface{}
	getUserBody  map[string]interface{}
}

func setupCredentialTest(t *testing.T, stub *credIAMStub) (*gorm.DB, http.Handler, string, string) {
	t.Helper()
	db := testfixtures.SetupTestDB(t)
	if db == nil {
		return nil, nil, "", ""
	}
	if stub == nil {
		stub = &credIAMStub{updateStatus: http.StatusOK}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/token" {
			fmt.Fprint(w, `{"access_token":"mock-token","expires_in":3600,"token_type":"Bearer"}`)
			return
		}
		if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/users/") {
			if stub.updateBody != nil {
				_ = json.NewDecoder(r.Body).Decode(&stub.updateBody)
			}
			if stub.updateStatus != http.StatusOK {
				w.WriteHeader(stub.updateStatus)
				fmt.Fprint(w, `{"error":"username already exists"}`)
				return
			}
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"code":20000,"data":{"id":"x"}}`)
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/users/") {
			if stub.getUserBody != nil {
				_ = json.NewEncoder(w).Encode(stub.getUserBody)
			} else {
				fmt.Fprint(w, `{}`)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	tenantID := uuid.NewString()
	userID := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: tenantID,
		Name: "Cred User", Username: "13800002446", Phone: "13800002446",
		Email: "cred2446@example.test", Status: "active", IsShadow: false,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)

	h := &UserStaffHandler{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.PUT("/users/me", h.UpdateCurrentUser)
	r.GET("/users/me", h.GetCurrentUser)
	return db, r, tenantID, userID
}

func TestUpdateCurrentUser_2446_UsernamePassword(t *testing.T) {
	stub := &credIAMStub{updateStatus: http.StatusOK, updateBody: map[string]interface{}{}}
	db, router, _, userID := setupCredentialTest(t, stub)

	body := `{"username":"newuser2446","password":"Abcd1234"}`
	req := httptest.NewRequest(http.MethodPut, "/users/me", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	assert.Equal(t, "newuser2446", stub.updateBody["username"])
	assert.Equal(t, "Abcd1234", stub.updateBody["password"])

	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 20000, resp.Code)
	assert.Equal(t, true, resp.Data["relogin"])

	var user models.User
	require.NoError(t, db.Where("id = ?", userID).First(&user).Error)
	assert.Equal(t, "newuser2446", user.Username)
	assert.False(t, user.ForcePasswordChange)
}

func TestUpdateCurrentUser_2446_UsernameConflictAndValidation(t *testing.T) {
	stub := &credIAMStub{updateStatus: http.StatusConflict, updateBody: map[string]interface{}{}}
	_, router, _, userID := setupCredentialTest(t, stub)

	req := httptest.NewRequest(http.MethodPut, "/users/me", bytes.NewReader([]byte(`{"username":"taken2446"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "用户名已被占用")

	// Invalid username/password must fail before calling IAM.
	stub.updateBody = map[string]interface{}{}
	req = httptest.NewRequest(http.MethodPut, "/users/me", bytes.NewReader([]byte(`{"username":"ab","password":"short"}`)))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, stub.updateBody)

	var user models.User
	require.NoError(t, database.GetDB().Where("id = ?", userID).First(&user).Error)
	assert.Equal(t, "13800002446", user.Username, "invalid request must not mutate local cache")
}

func TestGetCurrentUser_2446_NeedsCredentials(t *testing.T) {
	stub := &credIAMStub{updateStatus: http.StatusOK}
	db, router, _, userID := setupCredentialTest(t, stub)

	req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, true, resp.Data["needs_credentials"])

	require.NoError(t, db.Model(&models.User{}).Where("id = ?", userID).Update("username", "custom2446").Error)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/users/me", nil)
	router.ServeHTTP(w, req)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, false, resp.Data["needs_credentials"])
}
