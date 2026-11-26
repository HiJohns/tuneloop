package handlers

// #2579：导出 xlsx 的字段取值必须接单一数据源（价格/分类/级别），无数据源字段不得再导出。
//
//   金额（daily_rate / deposit / overdue_daily_fee）= instrumentListPriceFields 单一链
//     （分 → 元 2 位；nil / 不可解析 → `-`；显式 0 → `0.00`）
//   level = InstrumentLevel.Caption（LevelID 非空）→ 回落受弃用字符串 → `-`
//   category_name = instruments.category_name
//   已删除字段（brand / model / monthly_rate / stock）不再出现在表头
//
// 本用例同时是红绿取样点：修复前 getFieldValue 对这些字段返回空串 / 硬编码 "0"，
// 断言必然失败（scripts/verify-2579.sh r1 --expect-red 采集 R2-red.txt）。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

const exportFields2580 = "sn,category_name,level,daily_rate,deposit,overdue_daily_fee,status"

// seedExportFields2579 构造 A/B/C 三台乐器：
//
//	A：pricing.daily_rent=100（元）、deposit NULL、level_id → 专业、category_name=大提琴、available
//	B：pricing.daily_rent=200 + pricing_overrides.daily_rent=150（override 优先）、deposit=0（显式零）
//	C：无 pricing（金额列必须为 `-`）
func seedExportFields2579(t *testing.T) (tenantID string) {
	t.Helper()
	db := testfixtures.GetTestDB(t)
	tenantID = uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)

	level := models.InstrumentLevel{ID: uuid.New(), Caption: "专业", Code: "pro-test-2579"}
	require.NoError(t, db.Create(&level).Error)

	cat := models.Category{ID: uuid.NewString(), TenantID: tenantID, Name: "大提琴"}
	require.NoError(t, db.Create(&cat).Error)

	mk := func(sn string, pricing, overrides string, deposit *models.Cents, withLevel bool) {
		inst := models.Instrument{
			ID:               uuid.NewString(),
			TenantID:         tenantID,
			CategoryID:       &cat.ID,
			CategoryName:     cat.Name,
			SN:               sn,
			StockStatus:      "available",
			Pricing:          pricing,
			PricingOverrides: overrides,
			Deposit:          deposit,
			Images:           "[]",
			Specifications:   "{}",
			Properties:       "{}",
		}
		if withLevel {
			lid := level.ID
			inst.LevelID = &lid
			inst.Level = "pro" // 受弃用字符串：有 caption 时不得被选中
		}
		require.NoError(t, db.Create(&inst).Error)
	}

	zeroDeposit := models.FromYuan(0)
	mk("CVZ-A-2579", `{"daily_rent":100}`, "{}", nil, true)
	// B：基准价 200 + pricing_overrides.daily_rent=150（override 优先，与 pricing-v2 同链）。
	mk("CVZ-B-2579", `{"daily_rent":200}`, `{"daily_rent":150}`, &zeroDeposit, false)
	mk("CVZ-C-2579", "", "{}", nil, false)
	return tenantID
}

// exportRows2579 调用导出 handler 并解析表头 + 按 SN 索引的字段值。
func exportRows2579(t *testing.T, tenantID string) (headers []string, rows map[string]map[string]string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.GetTestDB(t)
	actor := testutil.TestActor{TenantID: tenantID, UserID: uuid.NewString(), Role: "ADMIN"}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Set("db", db)
		c.Next()
	})
	r.GET("/api/instruments/export", ExportInstruments)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/instruments/export?fields="+exportFields2580, nil))
	require.Equal(t, http.StatusOK, w.Code, "导出必须 200; body=%s", w.Body.String())

	f, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	sheet := f.GetSheetName(f.GetActiveSheetIndex())
	matrix, err := f.GetRows(sheet)
	require.NoError(t, err)
	require.NotEmpty(t, matrix, "导出必须含表头")

	headers = matrix[0]
	// 字段名顺序与请求 fields 一致
	fields := []string{"sn", "category_name", "level", "daily_rate", "deposit", "overdue_daily_fee", "status"}
	rows = map[string]map[string]string{}
	for _, row := range matrix[1:] {
		if len(row) == 0 {
			continue
		}
		rec := map[string]string{}
		for i, field := range fields {
			if i < len(row) {
				rec[field] = row[i]
			}
		}
		rows[rec["sn"]] = rec
	}
	return headers, rows
}

func TestExportFieldValues2579(t *testing.T) {
	testfixtures.SetupTestDB(t)
	tenantID := seedExportFields2579(t)
	headers, rows := exportRows2579(t, tenantID)

	require.Len(t, rows, 3, "三台乐器都必须导出")
	a, b, c := rows["CVZ-A-2579"], rows["CVZ-B-2579"], rows["CVZ-C-2579"]
	require.NotNil(t, a)
	require.NotNil(t, b)
	require.NotNil(t, c)

	// 先输出测量值（红样采集需要：断言失败前必须留下实际单元格值）
	t.Logf("MONEY_DAILY_A=%s", a["daily_rate"])
	t.Logf("MONEY_DAILY_B=%s", b["daily_rate"])
	t.Logf("MONEY_DEPOSIT_B=%s", b["deposit"])
	t.Logf("MONEY_DAILY_C=%s", c["daily_rate"])
	t.Logf("LEVEL_CAPTION_A=%s", a["level"])
	t.Logf("CATEGORY_NAME_A=%s", a["category_name"])
	t.Logf("STATUS_A=%s", a["status"])
	t.Logf("HEADER_COUNT=%d", len(headers))
	t.Logf("HEADER_JOINED=%s", strings.Join(headers, "|"))

	// 表头：不得再出现已删除字段
	for _, dead := range []string{"品牌", "型号", "月租金", "库存数量"} {
		for _, h := range headers {
			require.NotEqual(t, dead, h, "已删除字段不得出现在导出表头: %v", headers)
		}
	}

	require.Equal(t, "识别码", headers[0])
	require.Equal(t, "分类名称", headers[1])
	require.Equal(t, "级别", headers[2])
	require.Equal(t, "日租金", headers[3])
	require.Equal(t, "押金", headers[4])
	require.Equal(t, "逾期租金", headers[5])
	require.Equal(t, "状态", headers[6])

	// 金额：分 → 元 2 位；nil → `-`；显式 0 → `0.00`
	require.Equal(t, "100.00", a["daily_rate"], "A.pricing.daily_rate=100 元")
	// A：instruments.deposit=NULL → 按计划回落到商户配置比例（该乐器无 total_price，算出 0.00）；
	// 这是"有解"而非"缺失"，因此与 C 的 `-` 语义不同。
	require.Equal(t, "0.00", a["deposit"], "A.deposit=NULL 应回落配置比例（可解析 → 数值）")
	require.Equal(t, "150.00", b["daily_rate"], "B 的 pricing_overrides.daily_rent=150 必须生效")
	require.Equal(t, "0.00", b["deposit"], "B.deposit=0 显式零必须显示 0.00")
	require.Equal(t, "-", c["daily_rate"], "C 无定价必须显示 -")
	require.Equal(t, "-", c["deposit"])

	// 级别 / 分类 / 状态
	require.Equal(t, "专业", a["level"], "有 LevelID 时必须取 InstrumentLevel.Caption，而不是受弃用字符串 pro")
	require.Equal(t, "大提琴", a["category_name"])
	require.Equal(t, "available", a["status"])

}

// exportResponse2579 以指定 actor 调用导出入口，返回原始响应（供 R6/R7 复用）。
func exportResponse2579(t *testing.T, actor testutil.TestActor, query string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.GetTestDB(t)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Set("db", db)
		c.Next()
	})
	r.GET("/api/instruments/export", ExportInstruments)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/instruments/export"+query, nil))
	return w
}

// exportHeaders2579 解析导出表头（R7 角色裁剪用）。
func exportHeaders2579(t *testing.T, actor testutil.TestActor, query string) []string {
	t.Helper()
	w := exportResponse2579(t, actor, query)
	require.Equal(t, http.StatusOK, w.Code, "导出必须 200; body=%s", w.Body.String())
	f, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	sheet := f.GetSheetName(f.GetActiveSheetIndex())
	matrix, err := f.GetRows(sheet)
	require.NoError(t, err)
	require.NotEmpty(t, matrix, "导出必须含表头")
	return matrix[0]
}

// TestExportUnknownFields2579 覆盖 #2579 D10 / Owner 裁决 2：未知 fields → 400/40002（不得 200 空列）。
func TestExportUnknownFields2579(t *testing.T) {
	testfixtures.SetupTestDB(t)
	tenantID := seedExportFields2579(t)

	actor := testutil.TestActor{TenantID: tenantID, UserID: uuid.NewString(), Role: "ADMIN"}
	w := exportResponse2579(t, actor, "?fields=brand")

	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			InvalidFields []string `json:"invalid_fields"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "错误响应必须是 JSON: %s", w.Body.String())

	noFile := 1
	if strings.HasPrefix(w.Header().Get("Content-Type"), "application/octet-stream") || bytes.HasPrefix(w.Body.Bytes(), []byte("PK")) {
		noFile = 0
	}

	t.Logf("STATUS=%d", w.Code)
	t.Logf("CODE=%d", body.Code)
	t.Logf("INVALID_FIELDS=%s", strings.Join(body.Data.InvalidFields, ","))
	t.Logf("NO_FILE_BODY=%d", noFile)

	require.Equal(t, http.StatusBadRequest, w.Code, "未知字段必须是 400")
	require.Equal(t, 40002, body.Code, "未知字段错误码必须是 40002")
	require.Contains(t, body.Message, "brand")
	require.Equal(t, []string{"brand"}, body.Data.InvalidFields)
	require.Equal(t, 1, noFile, "400 响应不得携带 xlsx 文件体")
}

// TestExportRoleFields2579 覆盖 #2579 D9 / Owner 裁决 1：导出字段 ⊆ 列表可见字段
// —— 无 instrument:delete（员工）时价格三列必须被后端裁掉；有该权限（管理员）时保留。
func TestExportRoleFields2579(t *testing.T) {
	testfixtures.SetupTestDB(t)
	tenantID := seedExportFields2579(t)

	// 真实权限注册表（默认 mock 的 GetCusPermBit 恒 -1 会直通，测不出裁剪）。
	prevRegistry := middleware.PermissionRegistry
	middleware.PermissionRegistry = services.NewPermissionRegistry()
	defer func() { middleware.PermissionRegistry = prevRegistry }()

	readBit := middleware.PermissionRegistry.GetCusPermBit("instrument:read")
	deleteBit := middleware.PermissionRegistry.GetCusPermBit("instrument:delete")
	require.GreaterOrEqual(t, readBit, 0)
	require.GreaterOrEqual(t, deleteBit, 0)

	staff := testutil.TestActor{
		TenantID: tenantID,
		UserID:   uuid.NewString(),
		Role:     "STAFF",
		CusPerm:  1 << readBit,
	}
	admin := testutil.TestActor{
		TenantID: tenantID,
		UserID:   uuid.NewString(),
		Role:     "ADMIN",
		CusPerm:  (1 << readBit) | (1 << deleteBit),
	}

	const allFields = "?fields=" + exportFields2580
	staffHeaders := exportHeaders2579(t, staff, allFields)
	adminHeaders := exportHeaders2579(t, admin, allFields)

	hasPrice := func(headers []string) bool {
		for _, h := range headers {
			if h == "日租金" || h == "押金" || h == "逾期租金" {
				return true
			}
		}
		return false
	}
	staffExcludes := 0
	if !hasPrice(staffHeaders) {
		staffExcludes = 1
	}
	adminIncludes := 0
	if hasPrice(adminHeaders) {
		adminIncludes = 1
	}

	t.Logf("STAFF_FIELDS_EXCLUDE_PRICE=%d", staffExcludes)
	t.Logf("ADMIN_FIELDS_INCLUDE_PRICE=%d", adminIncludes)
	t.Logf("STAFF_HEADERS=%s", strings.Join(staffHeaders, "|"))
	t.Logf("ADMIN_HEADERS=%s", strings.Join(adminHeaders, "|"))

	require.Equal(t, 1, staffExcludes, "无 instrument:delete 的账号不得导出价格列")
	require.Equal(t, 1, adminIncludes, "有 instrument:delete 的账号必须仍能导出价格列")
	// 非价格列不受影响（内部字段仍完整）
	require.Contains(t, staffHeaders, "识别码")
	require.Contains(t, staffHeaders, "分类名称")
	require.Contains(t, staffHeaders, "状态")
}
