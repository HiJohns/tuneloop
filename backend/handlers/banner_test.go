package handlers

import (
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

func setupBannerTables(t *testing.T, db *database.DB) error {
	tables := []interface{}{
		&models.Banner{},
	}
	for _, table := range tables {
		_ = db.Migrator().DropTable(table)
		if err := db.Migrator().CreateTable(table); err != nil {
			return err
		}
	}
	return nil
}

func setupBannerRouter(t *testing.T) (*gin.Engine, string, string) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return nil, "", ""
	}
	database.SetDB(db)
	if err := setupBannerTables(t, db); err != nil {
		t.Fatalf("failed to setup tables: %v", err)
	}

	tenantID := uuid.New().String()
	userID := uuid.New().String()

	handler := NewBannerHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/admin/banners", handler.ListBanners)
	router.POST("/api/admin/banners", handler.CreateBanner)
	router.PUT("/api/admin/banners/:id", handler.UpdateBanner)
	router.DELETE("/api/admin/banners/:id", handler.DeleteBanner)
	router.GET("/api/public/banners", handler.GetPublicBanners)

	return router, tenantID, userID
}

func TestBanner_CRUD(t *testing.T) {
	router, tenantID, _ := setupBannerRouter(t)
	if router == nil {
		return
	}
	defer cleanupTestData(db, tenantID)

	// Create
	createBody := map[string]interface{}{
		"image_url":  "http://example.com/banner1.jpg",
		"link_url":   "http://example.com/link1",
		"title":      "Banner 1",
		"sort_order": 1,
		"status":     "active",
	}
	jsonBody, _ := json.Marshal(createBody)
	req := httptest.NewRequest("POST", "/api/admin/banners", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var createResp struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    map[string]interface{} `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &createResp)
	require.NoError(t, err)
	assert.Equal(t, 20000, createResp.Code)
	assert.NotEmpty(t, createResp.Data["id"])
	bannerID := createResp.Data["id"].(string)

	// List
	req = httptest.NewRequest("GET", "/api/admin/banners", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var listResp struct {
		Code int `json:"code"`
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &listResp)
	require.NoError(t, err)
	assert.Equal(t, 20000, listResp.Code)
	assert.Equal(t, 1, len(listResp.Data.List))

	// Update
	updateBody := map[string]interface{}{
		"title": "Updated Banner",
	}
	jsonBody, _ = json.Marshal(updateBody)
	req = httptest.NewRequest("PUT", "/api/admin/banners/"+bannerID, bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// List active (public)
	req = httptest.NewRequest("GET", "/api/public/banners", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var publicResp struct {
		Code int `json:"code"`
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &publicResp)
	require.NoError(t, err)
	assert.Equal(t, 20000, publicResp.Code)
	assert.Equal(t, 1, len(publicResp.Data.List))
	assert.Equal(t, "Updated Banner", publicResp.Data.List[0]["title"])

	// Delete
	req = httptest.NewRequest("DELETE", "/api/admin/banners/"+bannerID, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify empty after delete
	req = httptest.NewRequest("GET", "/api/admin/banners", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &listResp)
	require.NoError(t, err)
	assert.Equal(t, 20000, listResp.Code)
	assert.Equal(t, 0, len(listResp.Data.List))
}
