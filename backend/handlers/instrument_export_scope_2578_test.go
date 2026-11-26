package handlers

// #2578：导出必须与列表（GET /api/instruments）使用**同一套**过滤与可见性语义——
// 识别码模糊、分类按后代展开、状态/排除状态、以及 applyInstrumentListScope 的网点范围。
// 旧导出只支持 `category`（按分类名称等值）/`search_text`/`status`，且无网点范围 → 导出
// 行数与列表不一致（跨网点泄漏 + 后代分类漏筛）。
//
// 证据行（Accepted Plan R1）：
//   EXPORT_SN_FILTER=1    ?sn=CVZ-104                → 仅 A
//   EXPORT_CATEGORY_DESC=2 ?category_id=P（父）      → A+B（C 为 P 的子分类）
//   EXPORT_EXCLUDE=3       ?exclude_status=lost,...  → A+B+E（租户级；D 为 lost 被排除）
//   EXPORT_SCOPE_SITE=2    site_admin                → 仅 A+B（siteB 的 E/D 不可见）
//   EXPORT_HEADER_SN=1     首列表头 = 识别码
//   EXPORT_ROWS_EQ_LIST=1  同一 actor/筛选下导出 SN 集合 == 列表接口 SN 集合

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

type exportFixture2578 struct {
	tenantID   string
	siteA      string
	siteB      string
	siteAOrgID string
	parentCat  string
	childCat   string
	otherCat   string
	siteAdmin  string // users.iam_sub 值（= JWT user id）
}

// seedExportFixture2578 构造 R1 的 fixture：
// 分类 P(父)→C(子)、O；乐器 A(CVZ-104,available,C,siteA) / B(CVZ-105,available,C,siteA) /
// E(CVZ-106,available,O,siteB) / D(CVZ-107,lost,O,siteB)；u 为 siteA 的 site_admin。
func seedExportFixture2578(t *testing.T) exportFixture2578 {
	t.Helper()
	db := testfixtures.GetTestDB(t)
	fx := exportFixture2578{
		tenantID:   uuid.NewString(),
		siteA:      uuid.NewString(),
		siteB:      uuid.NewString(),
		siteAOrgID: uuid.NewString(),
	}
	testfixtures.SeedTenantFK(t, db, fx.tenantID)

	// 网点：siteA 的 org 与 tenant 不同（site_admin 判定依赖 oid != tid）。
	siteA := models.Site{ID: fx.siteA, TenantID: fx.tenantID, OrgID: fx.siteAOrgID, Name: "网点A-2578", Type: "direct"}
	siteB := models.Site{ID: fx.siteB, TenantID: fx.tenantID, OrgID: uuid.NewString(), Name: "网点B-2578", Type: "direct"}
	require.NoError(t, db.Create(&siteA).Error)
	require.NoError(t, db.Create(&siteB).Error)

	parent := models.Category{ID: uuid.NewString(), TenantID: fx.tenantID, Name: "弦乐-2578"}
	require.NoError(t, db.Create(&parent).Error)
	child := models.Category{ID: uuid.NewString(), TenantID: fx.tenantID, Name: "吉他-2578", ParentID: &parent.ID}
	require.NoError(t, db.Create(&child).Error)
	other := models.Category{ID: uuid.NewString(), TenantID: fx.tenantID, Name: "键盘-2578"}
	require.NoError(t, db.Create(&other).Error)
	fx.parentCat, fx.childCat, fx.otherCat = parent.ID, child.ID, other.ID

	mk := func(sn, status, catID, catName, siteID string) {
		cid, sid := catID, siteID
		var siteUUID uuid.UUID
		require.NoError(t, siteUUID.UnmarshalText([]byte(sid)))
		inst := models.Instrument{
			ID:             uuid.NewString(),
			TenantID:       fx.tenantID,
			CategoryID:     &cid,
			CategoryName:   catName,
			SN:             sn,
			StockStatus:    status,
			SiteID:         &siteUUID,
			Images:         "[]",
			Pricing:        "{}",
			Specifications: "{}",
			Properties:     "{}",
		}
		require.NoError(t, db.Create(&inst).Error)
	}
	mk("CVZ-104", "available", fx.childCat, "吉他-2578", fx.siteA)
	mk("CVZ-105", "available", fx.childCat, "吉他-2578", fx.siteA)
	mk("CVZ-106", "available", fx.otherCat, "键盘-2578", fx.siteB)
	mk("CVZ-107", "lost", fx.otherCat, "键盘-2578", fx.siteB)

	// site_admin：users.iam_sub = JWT user id，site_members 指向 siteA。
	fx.siteAdmin = uuid.NewString()
	user := models.User{
		ID: uuid.NewString(), IAMSub: fx.siteAdmin, TenantID: fx.tenantID, OrgID: fx.siteAOrgID,
		Username: "sa2578", Name: "site-admin-2578", Role: "ADMIN", Status: "active",
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.NewString(), TenantID: fx.tenantID, SiteID: fx.siteA, UserID: user.ID,
		Role: "site_admin", Roles: []string{"site_admin"}, Status: "active",
	}).Error)

	return fx
}

// exportRouter2578 挂载导出与列表两个入口，注入同一 actor + db。
func exportRouter2578(t *testing.T, actor testutil.TestActor) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.GetTestDB(t)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Set("db", db)
		c.Next()
	})
	r.GET("/api/instruments", GetInstruments)
	r.GET("/api/instruments/export", ExportInstruments)
	return r
}

func doGet2578(t *testing.T, r *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// exportSNs2578 解析 xlsx 响应：返回表头首列与数据行 SN 集合。
func exportSNs2578(t *testing.T, w *httptest.ResponseRecorder) (string, []string) {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	f, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	rows, err := f.GetRows("Instruments")
	require.NoError(t, err)
	require.NotEmpty(t, rows, "导出必须至少含表头行")
	header := rows[0][0]

	sns := make([]string, 0, len(rows))
	for _, row := range rows[1:] {
		if len(row) > 0 {
			sns = append(sns, row[0])
		}
	}
	sort.Strings(sns)
	return header, sns
}

func listSNs2578(t *testing.T, w *httptest.ResponseRecorder) []string {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				SN string `json:"sn"`
			} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "body=%s", w.Body.String())
	require.Equal(t, 20000, resp.Code)
	sns := make([]string, 0, len(resp.Data.List))
	for _, item := range resp.Data.List {
		sns = append(sns, item.SN)
	}
	sort.Strings(sns)
	return sns
}

func TestExportScope2578(t *testing.T) {
	testfixtures.SetupTestDB(t)
	fx := seedExportFixture2578(t)

	siteActor := testutil.TestActor{
		TenantID: fx.tenantID, OrgID: fx.siteAOrgID, UserID: fx.siteAdmin, Role: "ADMIN",
	}
	// 租户级 actor：无站点归属 → 不做网点过滤（用于验证后代展开与排除状态）。
	tenantActor := testutil.TestActor{
		TenantID: fx.tenantID, UserID: uuid.NewString(), Role: "ADMIN",
	}
	siteRouter := exportRouter2578(t, siteActor)
	tenantRouter := exportRouter2578(t, tenantActor)

	const fields = "fields=sn,category_name,status"

	// EXPORT_SN_FILTER：识别码模糊匹配 → 只有 A。
	header, sns := exportSNs2578(t, doGet2578(t, siteRouter, "/api/instruments/export?sn=CVZ-104&"+fields))
	require.Equal(t, []string{"CVZ-104"}, sns)
	snFilter := 0
	if len(sns) == 1 && sns[0] == "CVZ-104" {
		snFilter = 1
	}
	headerSN := 0
	if header == "识别码" {
		headerSN = 1
	}

	// EXPORT_CATEGORY_DESC：父分类筛选须含子分类的乐器（A+B）。
	_, sns = exportSNs2578(t, doGet2578(t, tenantRouter, "/api/instruments/export?category_id="+fx.parentCat+"&"+fields))
	require.Equal(t, []string{"CVZ-104", "CVZ-105"}, sns)
	categoryDesc := len(sns)

	// EXPORT_EXCLUDE：租户级排除 lost/archived/sold → A+B+E（D 为 lost）。
	_, sns = exportSNs2578(t, doGet2578(t, tenantRouter, "/api/instruments/export?exclude_status=lost,archived,sold&"+fields))
	require.Equal(t, []string{"CVZ-104", "CVZ-105", "CVZ-106"}, sns)
	excludeCount := len(sns)

	// EXPORT_SCOPE_SITE：site_admin 只见 siteA 的 A+B（siteB 的 E/D 不可见）。
	_, sns = exportSNs2578(t, doGet2578(t, siteRouter, "/api/instruments/export?"+fields))
	require.Equal(t, []string{"CVZ-104", "CVZ-105"}, sns)
	scopeCount := len(sns)

	// EXPORT_ROWS_EQ_LIST：同一 actor + 同一筛选下，导出 SN 集合 == 列表 SN 集合。
	_, exportSNS := exportSNs2578(t, doGet2578(t, siteRouter, "/api/instruments/export?sn=CVZ-10&"+fields))
	listSNS := listSNs2578(t, doGet2578(t, siteRouter, "/api/instruments?page=1&pageSize=50&sn=CVZ-10"))
	require.Equal(t, listSNS, exportSNS, "导出与列表必须同口径（同 actor / 同筛选）")
	rowsEqList := 0
	if strings.Join(listSNS, ",") == strings.Join(exportSNS, ",") {
		rowsEqList = 1
	}

	require.Equal(t, 1, snFilter)
	require.Equal(t, 2, categoryDesc)
	require.Equal(t, 3, excludeCount)
	require.Equal(t, 2, scopeCount)
	require.Equal(t, 1, headerSN)
	require.Equal(t, 1, rowsEqList)

	t.Logf("EXPORT_SN_FILTER=%d", snFilter)
	t.Logf("EXPORT_CATEGORY_DESC=%d", categoryDesc)
	t.Logf("EXPORT_EXCLUDE=%d", excludeCount)
	t.Logf("EXPORT_SCOPE_SITE=%d", scopeCount)
	t.Logf("EXPORT_HEADER_SN=%d", headerSN)
	t.Logf("EXPORT_ROWS_EQ_LIST=%d", rowsEqList)
}
