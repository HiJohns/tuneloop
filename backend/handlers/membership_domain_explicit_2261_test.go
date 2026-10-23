package handlers

// #2261（B3 域3）：membership 域（technician_profiles）显式范围就绪守卫。
// 关闭自动租户范围后，列表仍必须按操作者商户隔离。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTechnicianProfilesDomain_ExplicitScopeIsolation_2261(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	shared := uuid.NewString()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: shared, OrgID: orgA, Name: "M-A",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: shared, OrgID: orgB, Name: "M-B",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)

	// #2247 写入锚：profile.tenant_id = 商户 org
	require.NoError(t, db.Create(&models.TechnicianProfile{UserID: uuid.NewString(), TenantID: orgA, Status: "active"}).Error)
	require.NoError(t, db.Create(&models.TechnicianProfile{UserID: uuid.NewString(), TenantID: orgB, Status: "active"}).Error)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.TestActor{TenantID: shared, OrgID: orgA, UserID: uuid.NewString(), Role: "OWNER"}
		ctx := database.ExplicitTenantScopeOnly(actor.InjectContext(c.Request.Context()))
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.GET("/api/technician-profiles", NewTechnicianProfileHandler().List)

	// 显式模式：中间件已通过 database.ExplicitTenantScopeOnly 关闭自动租户范围

	req := httptest.NewRequest(http.MethodGet, "/api/technician-profiles", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Data struct {
			List []struct {
				TenantID string `json:"tenant_id"`
			} `json:"list"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.EqualValues(t, 1, resp.Data.Total, "跨商户技师必须不可见")
	for _, p := range resp.Data.List {
		assert.Equal(t, orgA, p.TenantID)
	}
}

// #2261 B3-C：membership 域（sites/site_members）在真实中间件 on/off 下的隔离等价。
func TestMembershipDomain_SitesExplicitScope_2261(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	orgA, orgB := uuid.NewString(), uuid.NewString()
	siteA, siteB := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: orgA, OrgID: orgA, Name: "M-A",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: orgB, OrgID: orgB, Name: "M-B",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)
	// 真实归属：sites.tenant_id = 商户 org；sites.org_id = 站点 IAM org
	require.NoError(t, db.Create(&models.Site{ID: siteA, TenantID: orgA, OrgID: uuid.NewString(), Name: "site-A", Status: "active"}).Error)
	require.NoError(t, db.Create(&models.Site{ID: siteB, TenantID: orgB, OrgID: uuid.NewString(), Name: "site-B", Status: "active"}).Error)

	actor := testutil.TestActor{TenantID: orgA, OrgID: orgA, UserID: uuid.NewString(), Role: "OWNER"}
	newRouter := func(explicit bool) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
			c.Next()
		})
		if explicit {
			r.Use(middleware.ExplicitTenantScope())
		}
		h := NewSiteMemberHandler()
		r.GET("/sites/:id/members", h.ListMembers)
		r.PUT("/merchant/sites/:id", NewSiteHandler().UpdateSite)
		return r
	}

	run := func(explicit bool) (int, int, int, int) {
		r := newRouter(explicit)
		call := func(method, path, body string) int {
			var rd *strings.Reader
			if body != "" {
				rd = strings.NewReader(body)
			} else {
				rd = strings.NewReader("")
			}
			req := httptest.NewRequest(method, path, rd)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			return w.Code
		}
		return call(http.MethodGet, "/sites/"+siteA+"/members", ""),
			call(http.MethodGet, "/sites/"+siteB+"/members", ""),
			call(http.MethodPut, "/merchant/sites/"+siteA, `{"name":"renamed-A"}`),
			call(http.MethodPut, "/merchant/sites/"+siteB, `{"name":"renamed-B"}`)
	}

	ownOff, crossOff, updOwnOff, updCrossOff := run(false)
	ownOn, crossOn, updOwnOn, updCrossOn := run(true)

	assert.Equal(t, http.StatusOK, ownOff)
	assert.Equal(t, http.StatusNotFound, crossOff)
	assert.Equal(t, ownOff, ownOn, "成员列表 on/off 等价（本商户）")
	assert.Equal(t, crossOff, crossOn, "成员列表 on/off 等价（跨商户）")
	assert.Equal(t, http.StatusOK, updOwnOff)
	assert.Equal(t, http.StatusNotFound, updCrossOff, "自动范围：跨商户更新已被过滤")
	assert.Equal(t, updOwnOff, updOwnOn, "更新 on/off 等价（本商户）")
	assert.Equal(t, updCrossOff, updCrossOn, "更新 on/off 等价（跨商户）")
}
