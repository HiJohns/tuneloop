package handlers

// #1935: 平台级中转网点管理端点测试

import (
	"bytes"
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
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

func setup1935Test(t *testing.T) (*gin.Engine, string) {
	gin.SetMode(gin.TestMode)
	testfixtures.SetupTestDB(t)
	tenantID, _, _ := testfixtures.NewTenantIDs("1935ab0c000a")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		actor := testutil.TestActor{TenantID: tenantID, UserID: uuid.New().String(), Role: "admin"}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	router.GET("/api/admin/transit-sites", ListAdminTransitSites)
	router.POST("/api/admin/transit-sites", CreateAdminTransitSite)
	router.PUT("/api/admin/transit-sites/:id", UpdateAdminTransitSite)
	router.DELETE("/api/admin/transit-sites/:id", DeleteAdminTransitSite)
	router.GET("/api/admin/transit-sites/:id/members", ListTransitSiteMembers)
	router.POST("/api/admin/transit-sites/:id/members", AddTransitSiteMember)
	router.DELETE("/api/admin/transit-sites/:id/members/:member_id", RemoveTransitSiteMember)
	return router, tenantID
}

func seedTransitSite1935(t *testing.T, tenantID string) models.Site {
	t.Helper()
	site := models.Site{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		OrgID:     tenantID,
		Name:      "北京中转站",
		Address:   "北京市朝阳区xx路1号",
		Phone:     "010-12345678",
		Type:      "transit",
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, database.GetDB().Create(&site).Error)
	return site
}

func post1935JSON(t *testing.T, router *gin.Engine, path string, body map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp
}

func put1935JSON(t *testing.T, router *gin.Engine, path string, body map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp
}

func del1935(t *testing.T, router *gin.Engine, path string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp
}

func get1935(t *testing.T, router *gin.Engine, path string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp
}

func TestCreateAdminTransitSiteValidation(t *testing.T) {
	router, _ := setup1935Test(t)

	// 缺 address → 400
	code, resp := post1935JSON(t, router, "/api/admin/transit-sites", map[string]interface{}{
		"name": "中转站", "phone": "010-1",
	})
	assert.Equal(t, 400, code)
	assert.Contains(t, resp["message"], "必填")

	// 全量 → 200 + type=transit
	code, resp = post1935JSON(t, router, "/api/admin/transit-sites", map[string]interface{}{
		"name": "北京中转站", "address": "xx路1号", "phone": "010-12345678",
	})
	require.Equal(t, 200, code, "create response: %v", resp)
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "transit", data["type"])
}

func TestDeleteTransitSiteRejectsWhenReferenced(t *testing.T) {
	router, tenantID := setup1935Test(t)
	site := seedTransitSite1935(t, tenantID)

	// 有路由引用 → 400 拒绝
	route := models.TransitRoute{ID: uuid.New().String(), ControlledSiteID: uuid.New().String(), TransitSiteID: site.ID, CreatedAt: time.Now()}
	require.NoError(t, database.GetDB().Create(&route).Error)
	code, resp := del1935(t, router, "/api/admin/transit-sites/"+site.ID)
	assert.Equal(t, 400, code)
	assert.Contains(t, resp["message"], "路由引用")

	// 解绑后 → 200（status=inactive 软停用）
	database.GetDB().Where("id = ?", route.ID).Delete(&models.TransitRoute{})
	code, resp = del1935(t, router, "/api/admin/transit-sites/"+site.ID)
	assert.Equal(t, 200, code)

	var updatedSite models.Site
	require.NoError(t, database.GetDB().Where("id = ?", site.ID).First(&updatedSite).Error)
	assert.Equal(t, "inactive", updatedSite.Status)
}

func TestTransitSiteMemberRoleRestriction(t *testing.T) {
	router, tenantID := setup1935Test(t)
	site := seedTransitSite1935(t, tenantID)

	// 非法角色 → 400
	code, resp := post1935JSON(t, router, "/api/admin/transit-sites/"+site.ID+"/members", map[string]interface{}{
		"user_id": uuid.New().String(), "role": "manager",
	})
	assert.Equal(t, 400, code)
	assert.Contains(t, resp["message"], "site_admin")

	// 合法角色 → 200
	code, resp = post1935JSON(t, router, "/api/admin/transit-sites/"+site.ID+"/members", map[string]interface{}{
		"user_id": uuid.New().String(), "role": "site_member",
	})
	assert.Equal(t, 200, code)
	member := resp["data"].(map[string]interface{})
	assert.Equal(t, "site_member", member["role"])

	// 列表
	code, resp = get1935(t, router, "/api/admin/transit-sites/"+site.ID+"/members")
	assert.Equal(t, 200, code)
	_ = resp

	// 移除
	mid := member["id"].(string)
	code, resp = del1935(t, router, "/api/admin/transit-sites/"+site.ID+"/members/"+mid)
	assert.Equal(t, 200, code)
}
