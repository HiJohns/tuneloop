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

	// 跨商户详情 404
	w = httptest.NewRecorder()
	router(true).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/merchants/"+merchantB.ID, nil))
	assert.Equal(t, http.StatusNotFound, w.Code)

	// 跨租户网点加成员 404
	body, _ := json.Marshal(map[string]interface{}{"user_ids": []map[string]string{{"user_id": uuid.NewString(), "role": "site_member"}}})
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/sites/"+siteB.ID+"/members", bytes.NewReader(body))
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
