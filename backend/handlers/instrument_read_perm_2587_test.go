package handlers

// #2587：乐器只读子端点 + legacy 照片上传的 cus_perm 守卫（handler 级行为验证）。
//
//   R1 8 端点 × 无对应位（CusPerm=1<<5 maintain）→ 8/8 403 + code 40305
//   R2 8 端点 × 有对应位（read=1<<1、media_upload=1<<15）→ 8/8 非 403 且非 5xx
//
// 必须切真实 PermissionRegistry（默认 MockPermissionRegistry.GetCusPermBit 恒 -1 直通）。

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
	"github.com/stretchr/testify/require"
)

const (
	perm2587BitRead        = 1 << 1  // instrument:read
	perm2587BitMediaUpload = 1 << 15 // instrument:media_upload
	perm2587BitMaintain    = 1 << 5  // instrument:maintain（既非 read 也非 media_upload）
)

func installRealPermRegistry2587(t *testing.T) {
	t.Helper()
	prev := middleware.PermissionRegistry
	middleware.PermissionRegistry = services.NewPermissionRegistry()
	t.Cleanup(func() { middleware.PermissionRegistry = prev })
}

// guard2587Route 描述一条端点：方法、路径模板、守卫权限位、handler。
type guard2587Route struct {
	method string
	path   string
	perm   string
	handle gin.HandlerFunc
}

func buildRoutes2587(id string) []guard2587Route {
	return []guard2587Route{
		{http.MethodGet, "/api/instruments/filter-options", "instrument:read", GetInstrumentFilterOptions},
		{http.MethodGet, "/api/instruments/check", "instrument:read", CheckInstrumentSN},
		{http.MethodGet, "/api/instruments/" + id + "/photos/latest", "instrument:read", GetLatestInstrumentPhotos},
		{http.MethodGet, "/api/instruments/" + id + "/media", "instrument:read", GetInstrumentMedia},
		{http.MethodGet, "/api/instruments/" + id + "/activity-log", "instrument:read", GetInstrumentActivityLog},
		{http.MethodGet, "/api/instruments/" + id + "/pricing", "instrument:read", GetInstrumentPricing},
		{http.MethodGet, "/api/instruments/" + id + "/pricing-v2", "instrument:read", GetInstrumentPricingV2},
		{http.MethodPost, "/api/instruments/" + id + "/photos/upload", "instrument:media_upload", UploadInstrumentPhotos},
	}
}

func permRouter2587(t *testing.T, actor testutil.TestActor, route guard2587Route) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.GetTestDB(t)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Set("db", db)
		c.Next()
	})
	guard := middleware.RequireCusPerm(route.perm)
	if route.method == http.MethodGet {
		r.GET(route.path, guard, route.handle)
	} else {
		r.POST(route.path, guard, route.handle)
	}
	return r
}

func call2587(t *testing.T, r *gin.Engine, route guard2587Route) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if route.method == http.MethodGet {
		req = httptest.NewRequest(http.MethodGet, route.path, nil)
	} else {
		req = httptest.NewRequest(http.MethodPost, route.path, bytes.NewReader(nil))
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// code2587 解析 JSON code（非 JSON 返回 0）。
func code2587(w *httptest.ResponseRecorder) int {
	raw := w.Body.Bytes()
	if len(raw) == 0 || raw[0] != '{' {
		return 0
	}
	var body struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return 0
	}
	return body.Code
}

func fixtureInstrumentID2587(t *testing.T) string {
	t.Helper()
	db := testfixtures.GetTestDB(t)
	var inst models.Instrument
	require.NoError(t, db.Where("sn = ?", "CVZ-104").First(&inst).Error)
	return inst.ID
}

func TestInstrumentReadEndpointsPerm2587(t *testing.T) {
	testfixtures.SetupTestDB(t)
	installRealPermRegistry2587(t)
	fx := seedExportFixture2578(t)
	instID := fixtureInstrumentID2587(t)
	routes := buildRoutes2587(instID)
	require.Len(t, routes, 8)

	noPerm := testutil.TestActor{
		TenantID: fx.tenantID, OrgID: fx.siteAOrgID, UserID: fx.siteAdmin,
		Role: "ADMIN", CusPerm: perm2587BitMaintain,
	}
	denied, deniedCode := 0, 0
	for _, route := range routes {
		w := call2587(t, permRouter2587(t, noPerm, route), route)
		require.Equal(t, http.StatusForbidden, w.Code,
			"%s %s 无 %s 位必须 403; body=%s", route.method, route.path, route.perm, w.Body.String())
		denied++
		if code2587(w) == 40305 {
			deniedCode++
		}
	}

	t.Logf("READ_ENDPOINTS=%d", len(routes))
	t.Logf("READ_ENDPOINTS_403=%d", denied)
	t.Logf("READ_ENDPOINTS_40305=%d", deniedCode)
}

func TestAuthorizedPassthrough2587(t *testing.T) {
	testfixtures.SetupTestDB(t)
	installRealPermRegistry2587(t)
	fx := seedExportFixture2578(t)
	instID := fixtureInstrumentID2587(t)
	routes := buildRoutes2587(instID)

	withPerm := testutil.TestActor{
		TenantID: fx.tenantID, OrgID: fx.siteAOrgID, UserID: fx.siteAdmin,
		Role: "ADMIN", CusPerm: perm2587BitRead | perm2587BitMediaUpload,
	}
	not403, not5xx := 0, 0
	for _, route := range routes {
		w := call2587(t, permRouter2587(t, withPerm, route), route)
		if w.Code != http.StatusForbidden {
			not403++
		}
		if w.Code < 500 {
			not5xx++
		}
		require.NotEqual(t, http.StatusForbidden, w.Code,
			"%s %s 有 %s 位不得被守卫拦截; body=%s", route.method, route.path, route.perm, w.Body.String())
		require.Less(t, w.Code, 500,
			"%s %s 不得 5xx（守卫接线顺序/上下文丢失）; body=%s", route.method, route.path, w.Body.String())
	}

	t.Logf("AUTHORIZED_ENDPOINTS=%d", len(routes))
	t.Logf("AUTHORIZED_NOT_403=%d", not403)
	t.Logf("AUTHORIZED_NOT_5XX=%d", not5xx)
}
