package handlers

// #2276：membership/site/technician 写路由显式范围守卫。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMembershipWrite_ExplicitScope_2276(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantA, tenantB := uuid.NewString(), uuid.NewString()
	merchantA := models.Merchant{ID: uuid.NewString(), TenantID: tenantA, OrgID: tenantA, Name: "Merchant-A", AdminUID: uuid.NewString()}
	merchantB := models.Merchant{ID: uuid.NewString(), TenantID: tenantB, OrgID: tenantB, Name: "Merchant-B", AdminUID: uuid.NewString()}
	require.NoError(t, db.Create(&merchantA).Error)
	require.NoError(t, db.Create(&merchantB).Error)
	siteB := models.Site{ID: uuid.NewString(), TenantID: tenantB, OrgID: tenantB, Name: "Site-B"}
	require.NoError(t, db.Create(&siteB).Error)
	siteA := models.Site{ID: uuid.NewString(), TenantID: tenantA, OrgID: tenantA, Name: "Site-A"}
	require.NoError(t, db.Create(&siteA).Error)
	profileB := models.TechnicianProfile{ID: uuid.NewString(), UserID: uuid.NewString(), TenantID: tenantB, Status: "active"}
	require.NoError(t, db.Create(&profileB).Error)

	router := func(explicit bool) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			actor := testutil.TestActor{TenantID: tenantA, OrgID: tenantA, UserID: uuid.NewString(), Role: "ADMIN"}
			c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
			c.Next()
		})
		if explicit {
			r.Use(middleware.ExplicitTenantScope())
		}
		merchantHandler := NewMerchantHandler()
		siteMemberHandler := NewSiteMemberHandler()
		techHandler := NewTechnicianProfileHandler()
		merchantMemberHandler := NewMerchantMemberHandler()
		siteHandler := NewSiteHandler()
		r.GET("/merchants", merchantHandler.ListMerchants)
		r.GET("/merchants/:id", merchantHandler.GetMerchant)
		r.POST("/sites/:id/members", siteMemberHandler.AddMember)
		r.PUT("/technician-profiles/:id", techHandler.Update)
		r.GET("/admin/merchants/:id/members", merchantMemberHandler.ListMembers)
		r.POST("/admin/merchants/:id/members", merchantMemberHandler.AddMember)
		r.GET("/sites/tree", siteHandler.GetSiteTree)
		return r
	}

	// 商户列表：仅 A
	w := httptest.NewRecorder()
	router(true).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/merchants?page=1&pageSize=10", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var listResp struct {
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	require.Len(t, listResp.Data.List, 1)
	assert.Equal(t, merchantA.ID, listResp.Data.List[0]["id"])

	// on/off 等价（状态码 + 列表内容）
	wOff := httptest.NewRecorder()
	router(false).ServeHTTP(wOff, httptest.NewRequest(http.MethodGet, "/merchants?page=1&pageSize=10", nil))
	assert.Equal(t, w.Code, wOff.Code, "on/off 等价")
	var offResp struct {
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wOff.Body.Bytes(), &offResp))
	require.Len(t, offResp.Data.List, len(listResp.Data.List), "on/off 列表长度一致")
	for i := range offResp.Data.List {
		assert.Equal(t, listResp.Data.List[i]["id"], offResp.Data.List[i]["id"], "on/off 列表 ID 一致")
	}

	// 跨租户成员列表 404（同 shared tenant 的其他 org 仍为 403，#2274）
	w = httptest.NewRecorder()
	router(true).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/merchants/"+merchantB.ID+"/members", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)

	// 跨租户 AddMember 404（不进入 IAM）
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/merchants/"+merchantB.ID+"/members", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	router(true).ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// 站点树不含他租户站点
	w = httptest.NewRecorder()
	router(true).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/sites/tree", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), siteB.ID, "站点树不得返回他租户站点")
	assert.Contains(t, w.Body.String(), siteA.ID, "站点树应包含本租户站点")

	// 跨商户详情 404
	w = httptest.NewRecorder()
	router(true).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/merchants/"+merchantB.ID, nil))
	assert.Equal(t, http.StatusNotFound, w.Code)

	// 跨租户网点加成员 404
	body, _ := json.Marshal(map[string]interface{}{"user_ids": []map[string]string{{"user_id": uuid.NewString(), "role": "site_member"}}})
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/sites/"+siteB.ID+"/members", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router(true).ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// 跨租户维修师更新 404
	body, _ = json.Marshal(map[string]interface{}{"bio": "hack"})
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/technician-profiles/"+profileB.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router(true).ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// 说明：真实 merchant_admin claim 为 tid==oid==商户 org（#2274）；
// 这里「共享 tenant」指 merchants.tenant_id 共享，不是 JWT tid=shared。
func TestMerchantAdmin_SharedTenantOrgScope_2276(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	shared, orgA, orgB := uuid.NewString(), uuid.NewString(), uuid.NewString()
	mA := models.Merchant{ID: uuid.NewString(), TenantID: shared, OrgID: orgA, AdminUID: uuid.NewString(), Name: "A", Status: "active"}
	mB := models.Merchant{ID: uuid.NewString(), TenantID: shared, OrgID: orgB, AdminUID: uuid.NewString(), Name: "B", Status: "active"}
	require.NoError(t, db.Create(&mA).Error)
	require.NoError(t, db.Create(&mB).Error)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.TestActor{TenantID: orgA, OrgID: orgA, UserID: uuid.NewString(), Role: "ADMIN"}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	}, middleware.ExplicitTenantScope())
	h := NewMerchantHandler()
	r.GET("/merchants", h.ListMerchants)
	r.GET("/merchants/:id", h.GetMerchant)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/merchants?page=1&pageSize=10", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data.List, 1, "merchant_admin 共享 tenant 只能看本 org")
	assert.Equal(t, mA.ID, resp.Data.List[0]["id"])

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/merchants/"+mB.ID, nil))
	assert.Equal(t, http.StatusNotFound, w.Code, "跨商户 org 必须 404")
}

func TestCreateSite_ExplicitScope_2276(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantA, tenantB := uuid.NewString(), uuid.NewString()
	mA := models.Merchant{ID: uuid.NewString(), TenantID: tenantA, OrgID: tenantA, AdminUID: uuid.NewString(), Name: "A", Status: "active"}
	mB := models.Merchant{ID: uuid.NewString(), TenantID: tenantB, OrgID: tenantB, AdminUID: uuid.NewString(), Name: "B", Status: "active"}
	require.NoError(t, db.Create(&mA).Error)
	require.NoError(t, db.Create(&mB).Error)

	mockIAM := newMockIAMServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]interface{}{"id": "mock-ns-2276"})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 20000, "org_id": uuid.NewString(), "admin_id": uuid.NewString()})
	}, nil)
	defer mockIAM.Close()
	services.SetIAMInternalURLForTesting(mockIAM.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	router := func(tenant, org string) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			actor := testutil.TestActor{TenantID: tenant, OrgID: org, UserID: uuid.NewString(), Role: "OWNER"}
			c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
			c.Next()
		}, middleware.ExplicitTenantScope())
		r.POST("/merchant/sites", NewSiteHandler().CreateSite)
		return r
	}
	post := func(tenant, org string, body map[string]interface{}) (int, map[string]interface{}) {
		b, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/merchant/sites", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		router(tenant, org).ServeHTTP(w, req)
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp
	}

	code, resp := post(tenantA, tenantA, map[string]interface{}{"name": "S-A"})
	require.Equal(t, http.StatusCreated, code, resp)
	var site models.Site
	require.NoError(t, db.Where("name = ?", "S-A").First(&site).Error)
	assert.Equal(t, tenantA, site.TenantID, "创建网点必须归属当前租户")
	assert.NotEmpty(t, site.OrgID, "网点自身组织 ID 由 IAM 返回")

	// B 不得为 A 创建网点
	code, resp = post(tenantB, tenantB, map[string]interface{}{"name": "S-HACK", "merchant_id": mA.ID})
	assert.Equal(t, http.StatusForbidden, code, resp)
}
