package handlers

// #2513 S5：维修服务（type='service'）site 过滤/守卫把 JWT oid（sites.org_id 域）
// 当 repair_requests.site_id（sites.id 域）用。
//
// 规格裁定（`docs/cases/repair-service.md:65,271` + 已关闭 #2122）：
//   - 服务单归属 = 维修师直属商户（tenant_id 回填），**无 site 维度**；
//   - scope=site 仅商户层级可见，网点账号返回空；
//   - pending-dispatch 是 `scope=site&status=done_repair` 的兼容入口（RS-05）→ 同口径。
//
// 夹具刻意让 `sites.id != sites.org_id`（#2508/#2510 口径），否则 ID 域误用不可见。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type svc2513Fixture struct {
	tenantID    string // 根 tenant（JWT tid）
	merchantOrg string // 商户组织锚（merchants.org_id，也是 JWT oid 的商户层级值）
	siteID      string // sites.id（≠ siteOrg、≠ merchantOrg）
	siteOrg     string // sites.org_id（网点账号 JWT oid）
	otherTenant string
	otherOrg    string
	techID      string
	customerID  string
	rrLegacy    string // site_id = siteID（历史带 site 单）
	rrNull      string // site_id = ''（商户级单）
	rrQuote     string // 待报价单（site_id = siteID）用于守卫用例
	router      *gin.Engine
	db          *gorm.DB
}

func setupSvc2513Fixture(t *testing.T) svc2513Fixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	if db == nil {
		t.Skip("test db unavailable")
	}

	tenantID := "11111111-1111-4111-8111-111111111111"
	merchantOrg := "22222222-2222-4222-8222-222222222222"
	otherTenant := "33333333-3333-4333-8333-333333333333"
	otherOrg := "44444444-4444-4444-8444-444444444444"
	testfixtures.SeedTenantFK(t, db, tenantID, otherTenant)

	siteID := uuid.New().String()
	siteOrg := uuid.New().String()
	require.NotEqual(t, siteID, siteOrg, "夹具必须 sites.id != sites.org_id")
	require.NotEqual(t, siteOrg, merchantOrg, "夹具必须 sites.org_id != merchants.org_id")
	require.NoError(t, db.Create(&models.Site{
		ID: siteID, TenantID: tenantID, OrgID: siteOrg, Name: "A 网点", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.Merchant{
		TenantID: tenantID, OrgID: merchantOrg, Name: "商户 M", Status: "active",
		AdminUID: uuid.New().String(),
	}).Error)
	require.NoError(t, db.Create(&models.Merchant{
		TenantID: otherTenant, OrgID: otherOrg, Name: "他租户商户", Status: "active",
		AdminUID: uuid.New().String(),
	}).Error)

	techID := uuid.New().String()
	customerID := uuid.New().String()
	for _, id := range []string{techID, customerID} {
		require.NoError(t, db.Create(&models.User{
			ID: id, IAMSub: id, TenantID: tenantID, OrgID: merchantOrg,
			Username: "u-" + id[:8], Name: "用户", Status: "active",
		}).Error)
	}

	mkRR := func(siteVal, status string) string {
		id := uuid.New().String()
		rr := models.RepairRequest{
			ID:               id,
			TenantID:         merchantOrg, // 服务单归属 = 商户 org（normalizeRepairTenant 的结果）
			UserID:           customerID,
			UserInstrumentID: uuid.New().String(),
			Status:           status,
			Type:             repairServiceTypeVal,
			Description:      "svc-2513",
			TechnicianID:     &techID,
			Photos:           "[]",
			ReceivePhotos:    "[]",
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}
		if siteVal == "" {
			// 商户级单无 site（#2122 形态）：site_id 列是 uuid，空串非法 → 必须 Omit
			require.NoError(t, db.Omit("site_id").Create(&rr).Error)
			return id
		}
		rr.SiteID = siteVal
		require.NoError(t, db.Create(&rr).Error)
		return id
	}

	f := svc2513Fixture{
		tenantID: tenantID, merchantOrg: merchantOrg,
		siteID: siteID, siteOrg: siteOrg,
		otherTenant: otherTenant, otherOrg: otherOrg,
		techID: techID, customerID: customerID,
		db: db,
	}
	f.rrLegacy = mkRR(siteID, models.RepairReqStatusDoneRepair)
	f.rrNull = mkRR("", models.RepairReqStatusDoneRepair)
	f.rrQuote = mkRR(siteID, models.RepairReqStatusPendingQuote)

	h := NewRepairServiceHandler()
	r := gin.New()
	r.GET("/repair-services/pending-dispatch", h.ListPendingDispatch)
	r.GET("/repair-services", h.ListTasks)
	r.POST("/repair-services/:id/quote", h.Quote)
	f.router = r
	return f
}

// merchantActor：商户层级（tid=租户根、oid=商户组织锚）
func (f svc2513Fixture) merchantActor() testutil.TestActor {
	return testutil.TestActor{TenantID: f.tenantID, OrgID: f.merchantOrg, UserID: f.techID, Role: "merchant_admin"}
}

// siteActor：纯网点账号（tid=租户根、oid=网点组织）——#2122 规定不参与维修服务
func (f svc2513Fixture) siteActor() testutil.TestActor {
	return testutil.TestActor{TenantID: f.tenantID, OrgID: f.siteOrg, UserID: f.techID, Role: "site_member"}
}

func (f svc2513Fixture) otherTenantActor() testutil.TestActor {
	return testutil.TestActor{TenantID: f.otherTenant, OrgID: f.otherOrg, UserID: uuid.New().String(), Role: "merchant_admin"}
}

func (f svc2513Fixture) get(t *testing.T, actor testutil.TestActor, path string) (int, []models.RepairRequest) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))
	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []models.RepairRequest `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp.Data.List
}

func containsRRID(list []models.RepairRequest, id string) bool {
	for _, rr := range list {
		if rr.ID == id {
			return true
		}
	}
	return false
}

// TestRepairService2513_PendingDispatchMerchantSeesLegacy：R1
// 红灯（旧谓词 site_id = oid）→ 0 行；修复后 → 2 行（含历史带 site_id 单）。
func TestRepairService2513_PendingDispatchMerchantSeesLegacy(t *testing.T) {
	f := setupSvc2513Fixture(t)
	actor := f.merchantActor()

	// 旧谓词红灯证据：site_id = oid（oid 是 sites.org_id 域，与 sites.id 不同）
	var oldRows int64
	f.db.WithContext(database.IdentityCtx(actor.InjectContext(context.Background()))).
		Model(&models.RepairRequest{}).
		Where("type = ? AND status = ? AND site_id = ?", repairServiceTypeVal, models.RepairReqStatusDoneRepair, f.merchantOrg).
		Count(&oldRows)

	code, list := f.get(t, actor, "/repair-services/pending-dispatch")
	require.Equal(t, http.StatusOK, code)

	// R1 补充（Round 1 审计 Semantic Challenge）：merchant_admin 在
	// repairServiceMerchantLevel 的角色分支短路，pending-dispatch 的 DB 分支
	// （merchants.org_id = oid）此前没有正向用例；这里用商户直属员工
	// （Role=STAFF + oid=商户 org）覆盖该分支。
	staffActor := f.merchantActor()
	staffActor.Role = "STAFF"
	code, staffList := f.get(t, staffActor, "/repair-services/pending-dispatch")
	require.Equal(t, http.StatusOK, code)

	legacyVisible := 0
	if containsRRID(list, f.rrLegacy) {
		legacyVisible = 1
	}
	t.Logf("MERCHANT_ROWS=%d", len(list))
	t.Logf("MERCHANT_STAFF_ROWS=%d", len(staffList))
	t.Logf("LEGACY_VISIBLE=%d", legacyVisible)
	t.Logf("OLD_SITE_EQ_OID_ROWS=%d", oldRows)

	assert.Equal(t, 2, len(list), "商户层级应看到 2 张待发回单（含历史带 site_id 行）")
	assert.Equal(t, 2, len(staffList), "商户直属员工（STAFF + oid=商户 org）的 DB 分支必须看到同样 2 张")
	assert.Equal(t, 1, legacyVisible, "历史带 site_id 单必须可见")
	assert.Equal(t, int64(0), oldRows, "旧谓词 site_id = oid 必须恒空（红证据）")
	assert.True(t, containsRRID(list, f.rrNull), "商户级无 site 单保持可见")
}

// TestRepairService2513_ListTasksSiteIncludesLegacy：R2
// 红灯（旧谓词 site_id = oid OR site_id IS NULL）→ 仅 1 行；修复后 → 2 行。
func TestRepairService2513_ListTasksSiteIncludesLegacy(t *testing.T) {
	f := setupSvc2513Fixture(t)
	actor := f.merchantActor()

	var oldRows int64
	f.db.WithContext(database.IdentityCtx(actor.InjectContext(context.Background()))).
		Model(&models.RepairRequest{}).
		Where("type = ? AND status = ? AND (site_id = ? OR site_id IS NULL)",
			repairServiceTypeVal, models.RepairReqStatusDoneRepair, f.merchantOrg).
		Count(&oldRows)

	code, list := f.get(t, actor, "/repair-services?scope=site&status="+models.RepairReqStatusDoneRepair)
	require.Equal(t, http.StatusOK, code)

	t.Logf("SITE_SCOPE_ROWS=%d", len(list))
	t.Logf("OLD_FILTER_ROWS=%d", oldRows)

	assert.Equal(t, 2, len(list), "scope=site 不得漏历史带 site_id 行")
	assert.Equal(t, int64(1), oldRows, "旧谓词只命中 NULL 行（红证据）")
	assert.True(t, containsRRID(list, f.rrLegacy), "历史带 site_id 单必须在列")
	assert.True(t, containsRRID(list, f.rrNull), "商户级无 site 单保持可见")
}

// TestRepairService2513_StaffAllowedLegacySiteRow：R3
// 红灯（旧分支 rr.SiteID != oid → false）→ 403；修复后 → 20000。
func TestRepairService2513_StaffAllowedLegacySiteRow(t *testing.T) {
	f := setupSvc2513Fixture(t)
	actor := f.merchantActor()
	ctx := actor.InjectContext(context.Background())

	var rrLegacy models.RepairRequest
	require.NoError(t, f.db.WithContext(database.IdentityCtx(ctx)).
		Where("id = ?", f.rrQuote).First(&rrLegacy).Error)

	allowed := repairServiceStaffAllowed(&rrLegacy, ctx)

	// 旧逻辑复算（仅用于红证据，不参与断言路径）
	oldAllowed := true
	if rrLegacy.SiteID != "" {
		if oid := actor.OrgID; oid != "" && rrLegacy.SiteID != oid {
			oldAllowed = false
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/repair-services/"+f.rrQuote+"/quote",
		strings.NewReader(`{"quote_repair_cents":10000,"quote_logistics_cents":0}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req.WithContext(ctx))
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	allowedLegacy := 0
	if allowed {
		allowedLegacy = 1
	}
	t.Logf("ALLOWED_LEGACY=%d", allowedLegacy)
	t.Logf("QUOTE_STATUS=%d", w.Code)
	t.Logf("OLD_ALLOWED_LEGACY=%d", boolToInt(oldAllowed))

	assert.Equal(t, 1, allowedLegacy, "归属集合内的商户层级账号必须可操作历史带 site_id 单")
	assert.Equal(t, http.StatusOK, w.Code, "quote 入口必须放行（此前恒 403）")
	assert.Equal(t, 20000, resp.Code)
	assert.Equal(t, 0, boolToInt(oldAllowed), "旧 rr.SiteID != oid 分支必须恒拒（红证据）")
}

// TestRepairService2513_Negative：R4 负例
// 他租户商户 → 两接口 0 行、守卫 false；纯网点账号 → 两接口 0 行（#2122 保持）。
func TestRepairService2513_Negative(t *testing.T) {
	f := setupSvc2513Fixture(t)

	otherActor := f.otherTenantActor()
	code, list := f.get(t, otherActor, "/repair-services/pending-dispatch")
	require.Equal(t, http.StatusOK, code)
	otherRows := len(list)

	var rr models.RepairRequest
	require.NoError(t, f.db.WithContext(database.IdentityCtx(otherActor.InjectContext(context.Background()))).
		Where("id = ?", f.rrQuote).First(&rr).Error)
	otherAllowed := 0
	if repairServiceStaffAllowed(&rr, otherActor.InjectContext(context.Background())) {
		otherAllowed = 1
	}

	siteActor := f.siteActor()
	code, siteList := f.get(t, siteActor, "/repair-services?scope=site&status="+models.RepairReqStatusDoneRepair)
	require.Equal(t, http.StatusOK, code)
	siteStaffRows := len(siteList)
	// pending-dispatch 同口径（RS-05）
	code, sitePending := f.get(t, siteActor, "/repair-services/pending-dispatch")
	require.Equal(t, http.StatusOK, code)
	sitePendingRows := len(sitePending)

	t.Logf("OTHER_TENANT_ROWS=%d", otherRows)
	t.Logf("OTHER_TENANT_ALLOWED=%d", otherAllowed)
	t.Logf("SITE_STAFF_ROWS=%d", siteStaffRows)
	t.Logf("SITE_STAFF_PENDING_ROWS=%d", sitePendingRows)

	assert.Equal(t, 0, otherRows, "他租户商户不得看到本商户服务单")
	assert.Equal(t, 0, otherAllowed, "他租户商户不得操作本商户服务单")
	assert.Equal(t, 0, siteStaffRows, "#2122：网点账号 scope=site 返回空")
	assert.Equal(t, 0, sitePendingRows, "#2122 + RS-05：网点账号 pending-dispatch 返回空")
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
