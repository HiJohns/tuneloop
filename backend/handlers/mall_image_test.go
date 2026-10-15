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

// TestMallImage_PartialUpdatePreservesSortOrder 覆盖 #2210 审计 R-1：
// 状态启停（仅 {status}）不得把 sort_order 重置为 0；显式 {sort_order} 仍须生效。
func TestMallImage_PartialUpdatePreservesSortOrder(t *testing.T) {
	router := setupMallImageRouter(t)
	if router == nil {
		return
	}

	// 创建 sort_order=5
	createBody := map[string]interface{}{
		"image_url":  "http://example.com/mall-sort.jpg",
		"sort_order": 5,
		"status":     "active",
	}
	jsonBody, _ := json.Marshal(createBody)
	req := httptest.NewRequest("POST", "/api/admin/mall-images", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	var createResp struct {
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))
	imageID := createResp.Data["id"].(string)

	readSortOrder := func() float64 {
		r := httptest.NewRequest("GET", "/api/admin/mall-images", nil)
		ww := httptest.NewRecorder()
		router.ServeHTTP(ww, r)
		var resp struct {
			Data struct {
				List []map[string]interface{} `json:"list"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(ww.Body.Bytes(), &resp))
		require.Equal(t, 1, len(resp.Data.List))
		return resp.Data.List[0]["sort_order"].(float64)
	}

	// 部分更新：仅 status（模拟 PC 状态启停）→ sort_order 必须保持 5
	partial, _ := json.Marshal(map[string]interface{}{"status": "inactive"})
	req = httptest.NewRequest("PUT", "/api/admin/mall-images/"+imageID, bytes.NewBuffer(partial))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, float64(5), readSortOrder(), "partial update must preserve sort_order")

	// 显式 sort_order 更新仍生效
	explicit, _ := json.Marshal(map[string]interface{}{"sort_order": 9})
	req = httptest.NewRequest("PUT", "/api/admin/mall-images/"+imageID, bytes.NewBuffer(explicit))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, float64(9), readSortOrder(), "explicit sort_order update must apply")
}
