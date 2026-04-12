package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
)

func TestGetSiteTreeReturnsManager(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("Test database not available, skipping:", err)
		return
	}
	database.SetDB(db)

	tenantID := uuid.New().String()
	now := time.Now()

	// 1. 创建测试管理员用户
	managerID := uuid.New()
	db.Exec(`INSERT INTO users (id, iam_sub, tenant_id, org_id, name, email, phone, is_shadow, created_at, updated_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?, false, ?, ?)`,
		managerID, managerID.String(), tenantID, tenantID, "测试管理员", "test@example.com", "13800138000", now, now)

	// 2. 创建测试网点（带 manager_id）
	siteID := uuid.New().String()
	db.Exec(`INSERT INTO sites (id, name, address, tenant_id, manager_id, status, created_at, updated_at) 
		VALUES (?, '测试网点', '测试地址', ?, ?, 'active', ?, ?)`,
		siteID, tenantID, managerID, now, now)

	// 3. 设置路由
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	siteHandler := NewSiteHandler()
	router.GET("/api/sites/tree", siteHandler.GetSiteTree)

	// 4. 调用 GET /api/sites/tree
	req := httptest.NewRequest("GET", "/api/sites/tree", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 5. 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				Manager *struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"manager"`
			} `json:"list"`
		} `json:"data"`
	}

	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// 6. 验证 manager 不为 null 且包含正确的 name
	require.NotEmpty(t, response.Data.List, "Should return sites list")
	foundSite := false
	for _, site := range response.Data.List {
		if site.ID == siteID {
			foundSite = true
			require.NotNil(t, site.Manager, "Manager should not be null")
			assert.Equal(t, managerID.String(), site.Manager.ID)
			assert.Equal(t, "测试管理员", site.Manager.Name)
			break
		}
	}
	assert.True(t, foundSite, "Created site should be in the list")

	// 清理
	db.Exec(`DELETE FROM sites WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM users WHERE tenant_id = ?`, tenantID)
}

// TestGetSiteTreeReturnsNullManagerWhenNoManager tests that manager is null when no manager is set
func TestGetSiteTreeReturnsNullManagerWhenNoManager(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("Test database not available, skipping:", err)
		return
	}
	database.SetDB(db)

	tenantID := uuid.New().String()
	now := time.Now()

	// Create test site without manager_id
	siteID := uuid.New().String()
	db.Exec(`INSERT INTO sites (id, name, address, tenant_id, status, created_at, updated_at) 
		VALUES (?, '测试网点无管理员', '测试地址', ?, 'active', ?, ?)`,
		siteID, tenantID, now, now)

	// Setup route
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	siteHandler := NewSiteHandler()
	router.GET("/api/sites/tree", siteHandler.GetSiteTree)

	// Call API
	req := httptest.NewRequest("GET", "/api/sites/tree", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Verify
	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				Manager *struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"manager"`
			} `json:"list"`
		} `json:"data"`
	}

	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify manager is null for site without manager
	require.NotEmpty(t, response.Data.List, "Should return sites list")
	for _, site := range response.Data.List {
		if site.ID == siteID {
			assert.Nil(t, site.Manager, "Manager should be null when no manager is set")
			break
		}
	}

	// Cleanup
	db.Exec(`DELETE FROM sites WHERE tenant_id = ?`, tenantID)
}
