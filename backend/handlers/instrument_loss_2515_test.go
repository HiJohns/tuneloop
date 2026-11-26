package handlers

// #2515：丢失台账不得跨网点泄漏。
//
// 缺陷：`GET /instrument-loss` 只按 tenant_id 过滤，网点级账号（site_admin /
// site_member 都持 instrument:read）能看到本商户全部网点的丢失/赔偿台账。
//
// 口径（D1）：台账可见性 = 乐器可见性（#1911 `applyInstrumentListScope`），按
// `instruments.site_id`（归属站点）过滤；**不使用** `current_site_id`（调拨改写会漂移）。
//
// Evidence rows: R1（网点隔离 + 红证据）、R2（商户/平台不回归）、R3（多站点员工）、
// R4（跨站指名 + 漂移口径排除）。
// Token lines use fmt.Printf so scripts/verify-2515.sh can grep them from `go test -v`.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type lossFixture2515 struct {
	tid, aID, aOrg, bID, bOrg string
	userA, instA, instB       string
	recA, recB                string
}

// seedLoss2515：ID 域强制分离（sites.id ≠ sites.org_id ≠ tenant_id，#2508/#2510），
// 且 instA.current_site_id 故意指向 B 站，用于证明调拨字段不参与判定。
func seedLoss2515(t *testing.T, db *gorm.DB) lossFixture2515 {
	t.Helper()
	f := lossFixture2515{
		tid: uuid.NewString(), aID: uuid.NewString(), aOrg: uuid.NewString(),
		bID: uuid.NewString(), bOrg: uuid.NewString(), userA: uuid.NewString(),
	}
	testfixtures.SeedTenantFK(t, db, f.tid)
	require.NoError(t, db.Create(&models.Site{ID: f.aID, TenantID: f.tid, OrgID: f.aOrg, Name: "丢失网点A", Status: "active"}).Error)
	require.NoError(t, db.Create(&models.Site{ID: f.bID, TenantID: f.tid, OrgID: f.bOrg, Name: "丢失网点B", Status: "active"}).Error)
	require.NoError(t, db.Create(&models.User{
		ID: f.userA, IAMSub: f.userA, TenantID: f.tid, OrgID: f.aOrg,
		Username: "loss2515a", Name: "网点A员工", Email: "loss2515a@example.com",
		Role: "site_member", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.NewString(), TenantID: f.tid, SiteID: f.aID, UserID: f.userA,
		Role: "STAFF", Roles: []string{"STAFF"}, Status: "active",
	}).Error)

	instAID := uuid.NewString()
	bID := f.bID
	require.NoError(t, db.Create(&models.Instrument{
		ID: instAID, TenantID: f.tid, OrgID: &f.aOrg, SN: "LOSS-2515-A", StockStatus: "lost",
		SiteID: uuidPtr2515(f.aID), CurrentSiteID: uuidPtr2515(bID),
	}).Error)
	instBID := uuid.NewString()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instBID, TenantID: f.tid, OrgID: &f.bOrg, SN: "LOSS-2515-B", StockStatus: "lost",
		SiteID: uuidPtr2515(f.bID),
	}).Error)

	recA := uuid.NewString()
	recB := uuid.NewString()
	require.NoError(t, db.Create(&models.InstrumentLossRecord{
		ID: recA, TenantID: f.tid, InstrumentID: instAID, ResponsibleParty: "user",
		CompensationCents: models.FromYuan(100), Description: "A 站丢失",
	}).Error)
	require.NoError(t, db.Create(&models.InstrumentLossRecord{
		ID: recB, TenantID: f.tid, InstrumentID: instBID, ResponsibleParty: "user",
		CompensationCents: models.FromYuan(200), Description: "B 站丢失",
	}).Error)

	f.instA, f.instB, f.recA, f.recB = instAID, instBID, recA, recB
	return f
}

func uuidPtr2515(v string) *uuid.UUID {
	parsed := uuid.MustParse(v)
	return &parsed
}

func lossRouter2515(actor testutil.TestActor) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	// 生产路由挂 middleware.ExplicitTenantScope()（backend/main.go:442）→ 关闭 GORM 自动租户注入，
	// handler 必须全靠显式过滤。测试必须镜像同一模式，否则「隔离成立」可能只是自动作用域的副作用。
	r.Use(middleware.ExplicitTenantScope())
	h := &InstrumentLossHandler{}
	r.GET("/instrument-loss", h.List)
	return r
}

type lossList2515 struct {
	status int
	ids    []string
}

func serveLoss2515(t *testing.T, r *gin.Engine, query string) lossList2515 {
	t.Helper()
	url := "/instrument-loss"
	if query != "" {
		url += "?" + query
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				ID string `json:"id"`
			} `json:"list"`
			Total int `json:"total"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	out := lossList2515{status: w.Code}
	for _, row := range resp.Data.List {
		out.ids = append(out.ids, row.ID)
	}
	return out
}

func contains2515(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func flag2515(cond bool) int {
	if cond {
		return 1
	}
	return 0
}

// R1：网点 A 账号只看到 A 站记录；同时复现旧谓词（tenant-only）的泄漏计数作为红证据。
func TestInstrumentLoss2515_SiteScope(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	f := seedLoss2515(t, db)

	actor := testutil.TestActor{TenantID: f.tid, OrgID: f.aOrg, UserID: f.userA, Role: "STAFF"}
	got := serveLoss2515(t, lossRouter2515(actor), "")

	leak := contains2515(got.ids, f.recB)
	// 红证据：旧谓词（仅 tenant_id）在本夹具下会返回 2 条。
	var tenantOnly int64
	require.NoError(t, db.Model(&models.InstrumentLossRecord{}).Where("tenant_id = ?", f.tid).Count(&tenantOnly).Error)

	fmt.Printf("R1 STATUS=%d SITE_A_IDS=[%s] CROSS_SITE_LEAK=%d OLD_PREDICATE_LEAK=%d RED_CONFIRMED=%d\n",
		got.status, joinIDs2515(got.ids), flag2515(leak), tenantOnly, flag2515(tenantOnly == 2 && !leak))
	require.Equal(t, http.StatusOK, got.status)
	require.Equal(t, []string{f.recA}, got.ids)
	require.False(t, leak, "site A must not see site B loss records")
	require.EqualValues(t, 2, tenantOnly, "old tenant-only predicate leaks both sites")
}

// R1a：网点管理员（Role="ADMIN" 且 oid≠tid → business_role=site_admin，同样持
// instrument:read）必须与 site_member 同口径收敛到本网点归属站点。
func TestInstrumentLoss2515_SiteAdminScope(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	f := seedLoss2515(t, db)

	actor := testutil.TestActor{TenantID: f.tid, OrgID: f.aOrg, UserID: f.userA, Role: "ADMIN"}
	got := serveLoss2515(t, lossRouter2515(actor), "")

	leak := contains2515(got.ids, f.recB)
	fmt.Printf("R1A STATUS=%d SITE_ADMIN_ROWS=%d SITE_ADMIN_CROSS_SITE_LEAK=%d SITE_ADMIN_IDS=[%s]\n",
		got.status, len(got.ids), flag2515(leak), joinIDs2515(got.ids))
	require.Equal(t, http.StatusOK, got.status)
	require.False(t, leak, "site admin of site A must not see site B loss records")
	require.Equal(t, []string{f.recA}, got.ids)
}

// R2：商户管理员（tid==oid）与平台管理员仍看全量（不回归）。
func TestInstrumentLoss2515_MerchantAndPlatform(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	f := seedLoss2515(t, db)

	merchantActor := testutil.TestActor{TenantID: f.tid, OrgID: f.tid, UserID: uuid.NewString(), Role: "ADMIN"}
	merchant := serveLoss2515(t, lossRouter2515(merchantActor), "")

	// 平台/系统管理员：business role = system_admin（GetBusinessRole 依 role=NAMESPACE_ADMIN），
	// 不进入站点收敛分支；tenant 上下文与既有 handler 的 tid 门禁保持一致。
	platformActor := testutil.TestActor{TenantID: f.tid, OrgID: f.aOrg, UserID: uuid.NewString(), Role: "NAMESPACE_ADMIN"}
	platform := serveLoss2515(t, lossRouter2515(platformActor), "")

	fmt.Printf("R2 MERCHANT_SEES=%d PLATFORM_SEES=%d MERCHANT_IDS=%s PLATFORM_IDS=%s\n",
		len(merchant.ids), len(platform.ids), joinIDs2515(merchant.ids), joinIDs2515(platform.ids))
	require.Equal(t, 2, len(merchant.ids))
	require.Equal(t, 2, len(platform.ids))
}

// R3：同一用户两条 site_members → 两个站点记录都可见（#1911 多站点语义）。
func TestInstrumentLoss2515_MultiSiteStaff(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	f := seedLoss2515(t, db)

	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.NewString(), TenantID: f.tid, SiteID: f.bID, UserID: f.userA,
		Role: "STAFF", Roles: []string{"STAFF"}, Status: "active",
	}).Error)

	actor := testutil.TestActor{TenantID: f.tid, OrgID: f.aOrg, UserID: f.userA, Role: "STAFF"}
	got := serveLoss2515(t, lossRouter2515(actor), "")
	fmt.Printf("R3 MULTISITE_SEES=%d MULTISITE_IDS=%s\n", len(got.ids), joinIDs2515(got.ids))
	require.Equal(t, 2, len(got.ids))
}

// R4：网点 A 指名 B 站乐器 → 0 条；且新过滤不引用 current_site_id（漂移口径排除）。
func TestInstrumentLoss2515_NegativeCrossSiteInstrument(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	f := seedLoss2515(t, db)

	actor := testutil.TestActor{TenantID: f.tid, OrgID: f.aOrg, UserID: f.userA, Role: "STAFF"}
	router := lossRouter2515(actor)
	cross := serveLoss2515(t, router, "instrument_id="+f.instB)
	same := serveLoss2515(t, router, "instrument_id="+f.instA)

	// DRIFT_SCOPE（计划语义，期望 0）：返回行中按「归属站点」判定为错归属
	// （instrument.site_id 不在操作者可见站点）的条数——调拨漂移不得改写凭证归属。
	var driftScope int64
	if len(same.ids) > 0 {
		require.NoError(t, db.Model(&models.InstrumentLossRecord{}).
			Joins("JOIN instruments ON instruments.id = instrument_loss_records.instrument_id").
			Where("instrument_loss_records.id IN ?", same.ids).
			Where("instruments.site_id IS DISTINCT FROM ?", f.aID).
			Count(&driftScope).Error)
	}
	// NO_DRIFT_PROOF=1：recA 的 current_site_id 指向 B 站却仍被返回 → 证明判定未使用 current_site_id。
	noDriftProof := flag2515(len(same.ids) == 1 && same.ids[0] == f.recA)

	fmt.Printf("R4 CROSS_INSTRUMENT_ROWS=%d SAME_SITE_ROWS=%d DRIFT_SCOPE=%d NO_DRIFT_PROOF=%d\n",
		len(cross.ids), len(same.ids), driftScope, noDriftProof)
	require.Equal(t, 0, len(cross.ids), "site A must not read site B records by instrument id")
	require.Equal(t, 1, len(same.ids))
	require.Zero(t, driftScope, "no returned row may be misattributed by site ownership drift")
	require.Equal(t, 1, noDriftProof, "site ownership must not be judged by current_site_id")
}

func joinIDs2515(ids []string) string {
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += ","
		}
		out += id
	}
	return out
}
