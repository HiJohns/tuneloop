package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// #2615 丢失台账列表/详情：
//   - List 分页（page/pageSize）+ 状态过滤（open/restored/all）+ 乐器摘要 + photos 数组化
//   - Get 单条详情（不存在/他人租户 → 404/40400，不得 200 泄漏）
//
// 夹具沿用 instrument_loss_test.go 的 setupLossFixture（Postgres + 双 ID 域，
// sites.id ≠ sites.org_id，#2510），这里只替换路由与种子数据。

type lossList2615Fixture struct {
	lossFixture
	router *gin.Engine
}

func setupLossList2615Fixture(t *testing.T) lossList2615Fixture {
	t.Helper()
	base := setupLossFixture(t)
	h := NewInstrumentLossHandler()
	r := gin.New()
	staff := testutil.TestActor{
		TenantID: base.tenantID, OrgID: base.siteOrg, UserID: base.staffSub, Role: "site_member",
	}
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(staff.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.GET("/instrument-loss", h.List)
	r.GET("/instrument-loss/:id", h.Get)
	return lossList2615Fixture{lossFixture: base, router: r}
}

// seedLoss2615Record 建「乐器 + 丢失记录」两条数据（SN 各异，便于断言 brief）。
func seedLoss2615Record(
	t *testing.T, f lossList2615Fixture, sn string, restored bool, createdAt time.Time,
) (recordID, instrumentID string) {
	t.Helper()
	instrumentID = uuid.New().String()
	siteUUID, _ := uuid.Parse(f.siteID)
	tenantID := f.tenantID
	stockStatus := models.StockStatusLost
	if restored {
		stockStatus = models.StockStatusAvailable
	}
	require.NoError(t, f.db.Create(&models.Instrument{
		ID: instrumentID, TenantID: tenantID, OrgID: &tenantID,
		CurrentSiteID: &siteUUID, SN: sn, CategoryName: "吉他", LevelName: "专业",
		CoverImage: "cover-" + sn + ".jpg", StockStatus: stockStatus,
		CreatedAt: createdAt, UpdatedAt: createdAt,
	}).Error)

	recordID = uuid.New().String()
	rec := models.InstrumentLossRecord{
		ID: recordID, TenantID: tenantID, InstrumentID: instrumentID,
		ResponsibleParty: "user", UserRatio: 100,
		CompensationCents: models.FromYuan(1000), UserBurdenCents: models.FromYuan(1000),
		Description: "丢失描述 " + sn, Photos: `["k1","k2"]`,
		CreatedAt: createdAt, UpdatedAt: createdAt,
	}
	if restored {
		restoredAt := createdAt.Add(30 * time.Minute)
		rec.RestoredAt = &restoredAt
		rec.RestoredDamaged = true
		rec.RestoreDescription = "已找回"
		rec.RestorePhotos = `["r1"]`
	}
	require.NoError(t, f.db.Create(&rec).Error)
	return recordID, instrumentID
}

func lossGet2615(t *testing.T, r *gin.Engine, path string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "response must be JSON: %s", w.Body.String())
	return w.Code, resp
}

func lossData2615(resp map[string]interface{}) map[string]interface{} {
	d, _ := resp["data"].(map[string]interface{})
	if d == nil {
		return map[string]interface{}{}
	}
	return d
}

func lossList2615(resp map[string]interface{}) []interface{} {
	list, _ := lossData2615(resp)["list"].([]interface{})
	return list
}

func TestInstrumentLoss2615_ListAndGet(t *testing.T) {
	f := setupLossList2615Fixture(t)
	now := time.Now().UTC().Truncate(time.Second)
	restoredID, _ := seedLoss2615Record(t, f, "SN-2615-1", true, now.Add(-2*time.Hour))
	openID2, _ := seedLoss2615Record(t, f, "SN-2615-2", false, now.Add(-1*time.Hour))
	openID1, _ := seedLoss2615Record(t, f, "SN-2615-3", false, now)

	// ---- 分页 + 真实 total ----
	code, resp := lossGet2615(t, f.router, "/instrument-loss?page=1&pageSize=2&status=all")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, float64(20000), resp["code"], "list must return 20000: %v", resp)
	data := lossData2615(resp)
	total, _ := data["total"].(float64)
	pageLen := len(lossList2615(resp))
	t.Logf("LOSS_LIST_TOTAL=%d", int(total))
	t.Logf("LOSS_LIST_PAGE_LEN=%d", pageLen)
	if int(total) != 3 || pageLen != 2 {
		t.Fatalf("pagination broken: total=%v pageLen=%d (want 3/2)", data["total"], pageLen)
	}
	if ps, _ := data["page_size"].(float64); int(ps) != 2 {
		t.Fatalf("page_size echo wrong: %v", data["page_size"])
	}

	// ---- 状态过滤 ----
	_, openResp := lossGet2615(t, f.router, "/instrument-loss?status=open&pageSize=100")
	openTotal, _ := lossData2615(openResp)["total"].(float64)
	t.Logf("LOSS_LIST_STATUS_OPEN=%d", int(openTotal))
	_, restoredResp := lossGet2615(t, f.router, "/instrument-loss?status=restored&pageSize=100")
	restoredTotal, _ := lossData2615(restoredResp)["total"].(float64)
	t.Logf("LOSS_LIST_STATUS_RESTORED=%d", int(restoredTotal))
	if int(openTotal) != 2 || int(restoredTotal) != 1 {
		t.Fatalf("status filter broken: open=%v restored=%v (want 2/1)", openTotal, restoredTotal)
	}
	// 兼容层：open=true 等价 status=open
	_, legacyResp := lossGet2615(t, f.router, "/instrument-loss?open=true&pageSize=100")
	legacyTotal, _ := lossData2615(legacyResp)["total"].(float64)
	if int(legacyTotal) != 2 {
		t.Fatalf("open=true compatibility broken: total=%v (want 2)", legacyTotal)
	}

	// ---- 非法 status → 400/40002（不得静默当 all）----
	badCode, badResp := lossGet2615(t, f.router, "/instrument-loss?status=bogus")
	if badCode == http.StatusOK || badResp["code"] != float64(40002) {
		t.Fatalf("invalid status must be 400/40002, got %d/%v", badCode, badResp["code"])
	}

	// ---- 乐器摘要 + photos 数组化 ----
	briefSN := 1
	photosArray := 1
	for _, item := range lossList2615(resp) {
		row, _ := item.(map[string]interface{})
		brief, _ := row["instrument_brief"].(map[string]interface{})
		if brief == nil || brief["sn"] == nil || brief["sn"] == "" {
			briefSN = 0
			t.Logf("missing instrument_brief.sn: %v", row["instrument_id"])
		}
		photos, ok := row["photos"].([]interface{})
		if !ok || len(photos) != 2 {
			photosArray = 0
			t.Logf("photos not normalized to array: %v", row["photos"])
		}
	}
	t.Logf("LOSS_LIST_BRIEF_SN=%d", briefSN)
	t.Logf("LOSS_LIST_PHOTOS_ARRAY=%d", photosArray)
	if briefSN != 1 || photosArray != 1 {
		t.Fatalf("brief/photos normalization broken: briefSN=%d photosArray=%d", briefSN, photosArray)
	}

	// ---- 响应字段全保留（含已作废的冲正列；#2615 计划「原始字段全保留」）----
	reversedFields := []string{
		"reversed_at", "reversed_amount_cents", "deducted_damage_cents",
		"deducted_idle_cents", "reverse_note",
	}
	keepsReversed := 1
	for _, item := range lossList2615(resp) {
		row, _ := item.(map[string]interface{})
		for _, k := range reversedFields {
			if _, ok := row[k]; !ok {
				keepsReversed = 0
				t.Logf("list row drops legacy field %q: %v", k, row["id"])
			}
		}
	}
	_, detailResp := lossGet2615(t, f.router, "/instrument-loss/"+openID1)
	detailRow := lossData2615(detailResp)
	for _, k := range reversedFields {
		if _, ok := detailRow[k]; !ok {
			keepsReversed = 0
			t.Logf("detail drops legacy field %q", k)
		}
	}
	t.Logf("LOSS_RESPONSE_KEEPS_REVERSED_FIELDS=%d", keepsReversed)
	if keepsReversed != 1 {
		t.Fatalf("legacy reversed_* fields must be preserved in both list and detail")
	}

	// ---- 详情：命中 ----
	code, getResp := lossGet2615(t, f.router, "/instrument-loss/"+openID1)
	require.Equal(t, http.StatusOK, code)
	getCode, _ := getResp["code"].(float64)
	t.Logf("LOSS_GET_CODE=%d", int(getCode))
	getBrief, _ := lossData2615(getResp)["instrument_brief"].(map[string]interface{})
	getBriefSN := 0
	if getBrief != nil && getBrief["sn"] == "SN-2615-3" {
		getBriefSN = 1
	}
	t.Logf("LOSS_GET_BRIEF_SN=%d", getBriefSN)
	if int(getCode) != 20000 || getBriefSN != 1 {
		t.Fatalf("GET detail broken: code=%v briefSN=%d", getCode, getBriefSN)
	}

	// ---- 详情：已恢复记录也带恢复留痕 ----
	_, restoredDetail := lossGet2615(t, f.router, "/instrument-loss/"+restoredID)
	if restoredDetail["code"] != float64(20000) {
		t.Fatalf("restored record detail must be 20000: %v", restoredDetail)
	}
	rd := lossData2615(restoredDetail)
	if rd["restored_at"] == nil || rd["restored_damaged"] != true {
		t.Fatalf("restored trace missing: %v", rd)
	}

	// ---- 详情：不存在 / 非法 id → 404/40400 ----
	missingCode, missingResp := lossGet2615(t, f.router, "/instrument-loss/"+uuid.New().String())
	missingBody, _ := missingResp["code"].(float64)
	t.Logf("LOSS_GET_MISSING_CODE=%d", int(missingBody))
	if missingCode != http.StatusNotFound || int(missingBody) != 40400 {
		t.Fatalf("missing record must be 404/40400, got %d/%v", missingCode, missingBody)
	}
	notUUIDCode, notUUIDResp := lossGet2615(t, f.router, "/instrument-loss/not-a-uuid")
	if notUUIDCode != http.StatusNotFound || notUUIDResp["code"] != float64(40400) {
		t.Fatalf("malformed id must be 404/40400, got %d/%v", notUUIDCode, notUUIDResp["code"])
	}

	// ---- 详情：跨租户 → 404/40400（不得 200 泄漏）----
	otherTenant := uuid.New().String()
	testfixtures.SeedTenantFK(t, f.db, otherTenant)
	otherRouter := gin.New()
	other := testutil.TestActor{
		TenantID: otherTenant, UserID: uuid.New().String(), OrgID: uuid.New().String(), Role: "site_member",
	}
	otherRouter.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(other.InjectContext(c.Request.Context()))
		c.Next()
	})
	h := NewInstrumentLossHandler()
	otherRouter.GET("/instrument-loss/:id", h.Get)
	// 跨租户同 id：用本租户的 openID2 访问另一租户上下文
	crossCode, crossResp := lossGet2615(t, otherRouter, "/instrument-loss/"+openID2)
	crossBody, _ := crossResp["code"].(float64)
	t.Logf("LOSS_GET_CROSS_TENANT_CODE=%d", int(crossBody))
	if crossCode != http.StatusNotFound || int(crossBody) != 40400 {
		t.Fatalf("cross-tenant read must be 404/40400, got %d/%v", crossCode, crossBody)
	}
}
