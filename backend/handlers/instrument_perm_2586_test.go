package handlers

// #2586：乐器数据进/出端点的 cus_perm 守卫（handler 级行为验证）。
//
//   R1 导出 GET /api/instruments/export → instrument:read
//      无 read 位 → 403/40305 且响应体不是 xlsx；有 read 位 → 200 且 SN 集合 = 2
//      （#2578 可见性语义不回退）；cus_perm=0（legacy）与 NAMESPACE_ADMIN 放行。
//   R2 导入族 6 端点 → instrument:create；无 create 位 → 6/6 403/40305；
//      有 create 位 → 6/6 非 403（守卫不误伤）。
//
// 测试必须切到真实 PermissionRegistry：默认 MockPermissionRegistry.GetCusPermBit
// 恒返回 -1（直通），否则守卫永远 PASS（#2586 Input Truth 工具盲区）。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

const (
	permBitCreate2586 = 1 << 0 // instrument:create
	permBitRead2586   = 1 << 1 // instrument:read
	permBitMaintain   = 1 << 5 // instrument:maintain（无 read/create）
)

func installRealPermRegistry2586(t *testing.T) {
	t.Helper()
	prev := middleware.PermissionRegistry
	middleware.PermissionRegistry = services.NewPermissionRegistry()
	t.Cleanup(func() { middleware.PermissionRegistry = prev })
}

func permRouter2586(t *testing.T, actor testutil.TestActor, mount func(r *gin.Engine)) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.GetTestDB(t)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Set("db", db)
		c.Next()
	})
	mount(r)
	return r
}

func permGet2586(t *testing.T, r *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func permPost2586(t *testing.T, r *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(nil))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// respCode2586 解析 JSON 响应体的 code（xlsx 二进制返回 0）。
func respCode2586(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
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

// exportSNs2586 解析 xlsx 导出的 SN 列。
func exportSNs2586(t *testing.T, w *httptest.ResponseRecorder) []string {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	rows, err := f.GetRows("Instruments")
	require.NoError(t, err)
	sns := make([]string, 0, len(rows))
	for _, row := range rows[1:] {
		if len(row) > 0 {
			sns = append(sns, row[0])
		}
	}
	sort.Strings(sns)
	return sns
}

func TestExportPerm2586(t *testing.T) {
	testfixtures.SetupTestDB(t)
	installRealPermRegistry2586(t)
	fx := seedExportFixture2578(t)
	const path = "/api/instruments/export?fields=sn,category_name,status"

	base := testutil.TestActor{
		TenantID: fx.tenantID, OrgID: fx.siteAOrgID, UserID: fx.siteAdmin, Role: "ADMIN",
	}

	// 1) 无 read 位（仅有 maintain）→ 403/40305，且响应体不是 xlsx。
	noRead := base
	noRead.CusPerm = permBitMaintain
	w := permGet2586(t, permRouter2586(t, noRead, func(r *gin.Engine) {
		r.GET("/api/instruments/export",
			middleware.RequireCusPerm("instrument:read"), ExportInstruments)
	}), path)
	require.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
	require.Equal(t, 40305, respCode2586(t, w))
	noReadXLSX := 0
	if _, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes())); err == nil {
		noReadXLSX = 1
	}

	// 2) 有 read 位 → 200，且 SN 集合 = siteA 的 2 台（#2578 可见性语义）。
	withRead := base
	withRead.CusPerm = permBitRead2586
	w = permGet2586(t, permRouter2586(t, withRead, func(r *gin.Engine) {
		r.GET("/api/instruments/export",
			middleware.RequireCusPerm("instrument:read"), ExportInstruments)
	}), path)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	sns := exportSNs2586(t, w)
	require.Equal(t, []string{"CVZ-104", "CVZ-105"}, sns, "有 read 位时导出必须与 #2578 列表可见性一致")

	// 3) cus_perm = 0（legacy JWT）→ 放行（D5 兼容语义）。
	legacy := base
	legacy.CusPerm = 0
	w = permGet2586(t, permRouter2586(t, legacy, func(r *gin.Engine) {
		r.GET("/api/instruments/export",
			middleware.RequireCusPerm("instrument:read"), ExportInstruments)
	}), path)
	require.Equal(t, http.StatusOK, w.Code, "legacy cus_perm=0 必须直通; body=%s", w.Body.String())

	// 4) NAMESPACE_ADMIN（system 级）→ 放行（#1964）。
	sysAdmin := testutil.TestActor{TenantID: fx.tenantID, UserID: fx.siteAdmin, Role: "NAMESPACE_ADMIN"}
	w = permGet2586(t, permRouter2586(t, sysAdmin, func(r *gin.Engine) {
		r.GET("/api/instruments/export",
			middleware.RequireCusPerm("instrument:read"), ExportInstruments)
	}), path)
	require.Equal(t, http.StatusOK, w.Code, "NAMESPACE_ADMIN 必须放行; body=%s", w.Body.String())

	t.Logf("EXPORT_NO_READ_PERM_STATUS=%d", http.StatusForbidden)
	t.Logf("EXPORT_NO_READ_PERM_CODE=40305")
	t.Logf("EXPORT_NO_READ_PERM_XLSX=%d", noReadXLSX)
	t.Logf("EXPORT_WITH_READ_PERM_STATUS=%d", http.StatusOK)
	t.Logf("EXPORT_WITH_READ_PERM_SNS=%d", len(sns))
	t.Logf("EXPORT_ZERO_CUSPERM_STATUS=%d", http.StatusOK)
	t.Logf("EXPORT_SYSTEM_ADMIN_STATUS=%d", http.StatusOK)
}

func TestImportFamilyPerm2586(t *testing.T) {
	testfixtures.SetupTestDB(t)
	installRealPermRegistry2586(t)
	fx := seedExportFixture2578(t)

	mount := func(r *gin.Engine) {
		guard := middleware.RequireCusPerm("instrument:create")
		r.POST("/api/instruments/import", guard, ImportInstruments)
		r.GET("/api/instruments/import/template", guard, DownloadCSVTemplate)
		r.GET("/api/instruments/batch-import/template", guard, DownloadCSVTemplate)
		r.POST("/api/instruments/batch-import", guard, ExecuteBatchImport)
		r.POST("/api/instruments/batch-import/preview", guard, PreviewBatchImport)
		r.POST("/api/instruments/batch-import/media", guard, UploadBatchMedia)
	}
	type endpoint struct{ method, path string }
	endpoints := []endpoint{
		{http.MethodPost, "/api/instruments/import"},
		{http.MethodGet, "/api/instruments/import/template"},
		{http.MethodGet, "/api/instruments/batch-import/template"},
		{http.MethodPost, "/api/instruments/batch-import"},
		{http.MethodPost, "/api/instruments/batch-import/preview"},
		{http.MethodPost, "/api/instruments/batch-import/media"},
	}

	call := func(r *gin.Engine, e endpoint) *httptest.ResponseRecorder {
		if e.method == http.MethodGet {
			return permGet2586(t, r, e.path)
		}
		return permPost2586(t, r, e.path)
	}

	noCreate := testutil.TestActor{
		TenantID: fx.tenantID, OrgID: fx.siteAOrgID, UserID: fx.siteAdmin,
		Role: "ADMIN", CusPerm: permBitRead2586, // 只读
	}
	withCreate := noCreate
	withCreate.CusPerm = permBitCreate2586

	denied, deniedCode, allowed := 0, 0, 0
	for _, e := range endpoints {
		w := call(permRouter2586(t, noCreate, mount), e)
		require.Equal(t, http.StatusForbidden, w.Code, "%s 无 create 位必须 403; body=%s", e.path, w.Body.String())
		denied++
		if respCode2586(t, w) == 40305 {
			deniedCode++
		}

		w = call(permRouter2586(t, withCreate, mount), e)
		require.NotEqual(t, http.StatusForbidden, w.Code, "%s 有 create 位不得被守卫拦截; body=%s", e.path, w.Body.String())
		allowed++
	}

	t.Logf("IMPORT_FAMILY_ENDPOINTS=%d", len(endpoints))
	t.Logf("IMPORT_FAMILY_403=%d", denied)
	t.Logf("IMPORT_FAMILY_40305=%d", deniedCode)
	t.Logf("IMPORT_FAMILY_WITH_CREATE_NOT403=%d", allowed)
}
