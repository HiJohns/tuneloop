package handlers

// #1935: 平台级中转网点管理端点测试

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"
)

func setup1935Test(t *testing.T) (*gin.Engine, string) {
	gin.SetMode(gin.TestMode)
	testfixtures.SetupTestDB(t)
	tenantID, _, _ := testfixtures.NewTenantIDs("1935ab0c000a")

	// #1935 fix: member endpoints now call IAM (bind/unbind/templates) —
	// stub a mock IAM server so the flow is exercisable in tests.
	mockIAM := newTransitMockIAM(t)
	services.SetIAMInternalURLForTesting(mockIAM.URL)
	t.Cleanup(func() {
		services.SetIAMInternalURLForTesting("")
		mockIAM.Close()
	})

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
	router.PUT("/api/admin/transit-sites/:id/members/:member_id", UpdateTransitSiteMemberRole)
	router.DELETE("/api/admin/transit-sites/:id/members/:member_id", RemoveTransitSiteMember)
	router.GET("/api/admin/controlled-sites", ListControlledSites)
	return router, tenantID
}

// newTransitMockIAM stubs the IAM endpoints the transit member handlers call:
// client-credentials token, org bind/unbind/role-update, role templates.
func newTransitMockIAM(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-client-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/api/v1/organizations/", func(w http.ResponseWriter, r *http.Request) {
		// bind/unbind/role-update all return 200
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 20000, "message": "success"})
	})
	mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/role-templates") {
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": uuid.New().String(), "code": "site_member"},
				{"id": uuid.New().String(), "code": "site_admin"},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"id": uuid.New().String()})
	})
	return httptest.NewServer(mux)
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

func TestListControlledSitesExcludesTransit(t *testing.T) {
	router, tenantID := setup1935Test(t)
	db := database.GetDB()

	// 受控商户 + 普通网点 → 应入选
	require.NoError(t, db.Exec(`INSERT INTO merchants (id, tenant_id, org_id, name, code, merchant_type, created_at, updated_at)
		VALUES (?, ?, ?, '受控商户', 'ctrl-1936', 'controlled', now(), now())`, uuid.New().String(), tenantID, tenantID).Error)
	controlledSite := models.Site{ID: uuid.New().String(), TenantID: tenantID, OrgID: tenantID, Name: "受控网点A", Type: "store", Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	require.NoError(t, db.Create(&controlledSite).Error)

	// 中转网点 → 必须被排除
	transitSite := seedTransitSite1935(t, tenantID)

	code, resp := get1935(t, router, "/api/admin/controlled-sites")
	require.Equal(t, 200, code, "response: %v", resp)
	list := resp["data"].(map[string]interface{})["list"].([]interface{})
	ids := map[string]bool{}
	for _, item := range list {
		ids[item.(map[string]interface{})["id"].(string)] = true
	}
	assert.True(t, ids[controlledSite.ID], "受控商户网点应入选")
	assert.False(t, ids[transitSite.ID], "中转网点必须被排除")
}

func TestCreateAdminTransitSiteValidation(t *testing.T) {
	router, _ := setup1935Test(t)

	// 缺 address → 400
	code, resp := post1935JSON(t, router, "/api/admin/transit-sites", map[string]interface{}{
		"name": "中转站", "phone": "010-1",
	})
	assert.Equal(t, 400, code)
	assert.Contains(t, resp["message"], "必填")

	// 全量（含 contact_name）→ 200 + type=transit + contact_name 落库（audit Bug4）
	code, resp = post1935JSON(t, router, "/api/admin/transit-sites", map[string]interface{}{
		"name": "北京中转站", "address": "xx路1号", "phone": "010-12345678", "contact_name": "张三",
	})
	require.Equal(t, 200, code, "create response: %v", resp)
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "transit", data["type"])
	assert.Equal(t, "张三", data["contact_name"], "contact_name 必须持久化（audit Bug4）")
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

// TestTransitSiteMemberIAMSync — 添加成员走 IAM 绑定（audit Bug1）、
// 角色可更新（audit Bug5）、跨站删除被拒（audit Bug2）。
func TestTransitSiteMemberIAMSync(t *testing.T) {
	router, tenantID := setup1935Test(t)
	db := database.GetDB()
	site := seedTransitSite1935(t, tenantID)

	// 合法角色 → 200（IAM mock bind 200 后才写本地）
	code, resp := post1935JSON(t, router, "/api/admin/transit-sites/"+site.ID+"/members", map[string]interface{}{
		"user_id": uuid.New().String(), "role": "site_member",
	})
	require.Equal(t, 200, code, "add member response: %v", resp)
	member := resp["data"].(map[string]interface{})
	assert.Equal(t, "site_member", member["role"])

	// 重复添加 → 40002
	code, resp = post1935JSON(t, router, "/api/admin/transit-sites/"+site.ID+"/members", map[string]interface{}{
		"user_id": member["user_id"], "role": "site_member",
	})
	assert.Equal(t, 400, code)

	// 改角色（audit Bug5「改」）→ 200
	code, resp = put1935JSON(t, router, "/api/admin/transit-sites/"+site.ID+"/members/"+member["id"].(string), map[string]interface{}{
		"role": "site_admin",
	})
	require.Equal(t, 200, code, "role update response: %v", resp)
	assert.Equal(t, "site_admin", resp["data"].(map[string]interface{})["role"])

	// 非法角色 → 400
	code, _ = put1935JSON(t, router, "/api/admin/transit-sites/"+site.ID+"/members/"+member["id"].(string), map[string]interface{}{
		"role": "manager",
	})
	assert.Equal(t, 400, code)

	// 跨站删除防护（audit Bug2）：普通网点成员不可经中转端点删除
	otherSite := models.Site{ID: uuid.New().String(), TenantID: tenantID, OrgID: tenantID, Name: "普通网点", Type: "store", Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	require.NoError(t, db.Create(&otherSite).Error)
	otherMember := models.SiteMember{ID: uuid.New().String(), SiteID: otherSite.ID, UserID: uuid.New().String(), TenantID: tenantID, Role: "site_member"}
	require.NoError(t, db.Create(&otherMember).Error)
	code, _ = del1935(t, router, "/api/admin/transit-sites/"+site.ID+"/members/"+otherMember.ID)
	assert.Equal(t, 404, code, "跨站成员删除必须 404")
	var stillThere models.SiteMember
	require.NoError(t, db.Where("id = ?", otherMember.ID).First(&stillThere).Error, "被跨站删除的成员必须仍存在")

	// 本站成员删除 → 200 且行已删
	code, _ = del1935(t, router, "/api/admin/transit-sites/"+site.ID+"/members/"+member["id"].(string))
	assert.Equal(t, 200, code)
	var cnt int64
	db.Model(&models.SiteMember{}).Where("id = ?", member["id"]).Count(&cnt)
	assert.Equal(t, int64(0), cnt)
}

// TestUpdateAdminTransitSiteContactName — 编辑带 contact_name 不再 500（audit Bug4）
func TestUpdateAdminTransitSiteContactName(t *testing.T) {
	router, tenantID := setup1935Test(t)
	site := seedTransitSite1935(t, tenantID)

	code, resp := put1935JSON(t, router, "/api/admin/transit-sites/"+site.ID, map[string]interface{}{
		"name": "北京中转站", "address": "xx路1号", "phone": "010-12345678", "contact_name": "李四",
	})
	require.Equal(t, 200, code, "update response: %v", resp)

	var updated models.Site
	require.NoError(t, database.GetDB().Where("id = ?", site.ID).First(&updated).Error)
	assert.Equal(t, "李四", updated.ContactName)
}
