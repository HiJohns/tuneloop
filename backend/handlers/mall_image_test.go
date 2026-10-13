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
	"gorm.io/gorm"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

func setupMallImageTables(t *testing.T, db *gorm.DB) error {
	table := &models.MallImage{}
	_ = db.Migrator().DropTable(table)
	return db.Migrator().CreateTable(table)
}

func setupMallImageRouter(t *testing.T) *gin.Engine {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return nil
	}
	database.SetDB(db)
	if err := setupMallImageTables(t, db); err != nil {
		t.Fatalf("failed to setup tables: %v", err)
	}

	tenantID := uuid.New().String()
	userID := uuid.New().String()

	handler := NewMallImageHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/admin/mall-images", handler.ListMallImages)
	router.POST("/api/admin/mall-images", handler.CreateMallImage)
	router.PUT("/api/admin/mall-images/:id", handler.UpdateMallImage)
	router.DELETE("/api/admin/mall-images/:id", handler.DeleteMallImage)
	router.GET("/api/public/mall-images", handler.GetPublicMallImages)

	return router
}

func TestMallImage_CRUD(t *testing.T) {
	router := setupMallImageRouter(t)
	if router == nil {
		return
	}

	// Create
	createBody := map[string]interface{}{
		"image_url":  "http://example.com/mall1.jpg",
		"link_url":   "http://example.com/link1",
		"title":      "Mall 1",
		"sort_order": 1,
		"status":     "active",
	}
	jsonBody, _ := json.Marshal(createBody)
	req := httptest.NewRequest("POST", "/api/admin/mall-images", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var createResp struct {
		Code    int                    `json:"code"`
		Message string                 `json:"message"`
		Data    map[string]interface{} `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &createResp)
	require.NoError(t, err)
	assert.Equal(t, 20000, createResp.Code)
	assert.NotEmpty(t, createResp.Data["id"])
	imageID := createResp.Data["id"].(string)

	// List (admin)
	req = httptest.NewRequest("GET", "/api/admin/mall-images", nil)
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
		"title": "Updated Mall",
	}
	jsonBody, _ = json.Marshal(updateBody)
	req = httptest.NewRequest("PUT", "/api/admin/mall-images/"+imageID, bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// List active (public)
	req = httptest.NewRequest("GET", "/api/public/mall-images", nil)
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
	require.Equal(t, 1, len(publicResp.Data.List))
	assert.Equal(t, "Updated Mall", publicResp.Data.List[0]["title"])

	// Delete
	req = httptest.NewRequest("DELETE", "/api/admin/mall-images/"+imageID, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify empty after delete
	req = httptest.NewRequest("GET", "/api/admin/mall-images", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &listResp)
	require.NoError(t, err)
	assert.Equal(t, 20000, listResp.Code)
	assert.Equal(t, 0, len(listResp.Data.List))
}
