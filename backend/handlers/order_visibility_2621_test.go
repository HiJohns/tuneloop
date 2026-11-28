package handlers

// #2621 R4 回归：员工订单可见范围 = **当前组织上下文（JWT oid，登录时选定的网点）**，
// 不是该用户 site_members 的全部网点；自查询失败或 oid 为空 → fail-closed（空列表），
// 禁止退化为商户/租户范围（现状 bug 即放宽）。
//
// Owner 2026-11-28 裁定（Rev.4 D8/D9）：
//   - 多网点员工登录时必须选网点（IAM select-org 已具备），订单只看选中的网点；
//   - 身份自查询失败 / oid 缺失 → 空列表，绝不按 tenant 放宽。
//
// 测试保真（D6）：上下文由 testutil.TestActor.InjectContext 构造——同时写 middleware 与
// database 两套 key，GORM 自动租户过滤（addTenantScope）真实执行。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

type orderVisibility2621Env struct {
	tenantID string // JWT tid（商户租户）
	siteOrgA string // 网点 A 的组织 id（员工登录时选定的 oid）
	siteOrgB string // 网点 B 的组织 id
	staffSub string // 员工 IAM sub
	orderA   string // 网点 A 的订单
	orderB   string // 网点 B 的订单
}

// setupOrderVisibility2621 构造「双网点员工 + 每网点 1 单」，且本地 users.tenant_id
// 是 IAM 关系缓存（cacheTenant ≠ JWT tid）——预生产故障形态。
func setupOrderVisibility2621(t *testing.T) (*gin.Engine, orderVisibility2621Env) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cleanup := setupMockIAMAndDB(t)
	t.Cleanup(cleanup)
	db := dbForOrderVisibility2621(t)

	env := orderVisibility2621Env{
		tenantID: uuid.NewString(),
		siteOrgA: uuid.NewString(),
		siteOrgB: uuid.NewString(),
		staffSub: "iam-sub-2621-order-staff",
	}
	cacheTenant := uuid.NewString()

	// site_members.tenant_id 有 FK → tenant 行必须先存在。
	require.NoError(t, db.Create(&models.Tenant{ID: env.tenantID, Name: "T2621", Status: "active"}).Error)

	siteA := models.Site{ID: uuid.NewString(), TenantID: env.tenantID, OrgID: env.siteOrgA, Name: "网点A-2621"}
	siteB := models.Site{ID: uuid.NewString(), TenantID: env.tenantID, OrgID: env.siteOrgB, Name: "网点B-2621"}
	require.NoError(t, db.Create(&siteA).Error)
	require.NoError(t, db.Create(&siteB).Error)

	staff := models.User{
		ID:       uuid.NewString(),
		IAMSub:   env.staffSub,
		TenantID: cacheTenant, // IAM 关系缓存，故意 ≠ JWT tid
		OrgID:    env.siteOrgA,
		Name:     "双网点员工2621",
		Status:   "active",
		Role:     "site_member",
	}
	require.NoError(t, db.Create(&staff).Error)

	customer := models.User{
		ID:       uuid.NewString(),
		IAMSub:   "iam-sub-2621-order-customer",
		TenantID: cacheTenant,
		OrgID:    uuid.NewString(),
		Name:     "顾客2621",
		Status:   "active",
	}
	require.NoError(t, db.Create(&customer).Error)

	// 同一员工在两个网点都有成员关系（旧实现会把两网点的订单都返回）。
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.NewString(), TenantID: env.tenantID, SiteID: siteA.ID, UserID: staff.ID,
		Role: "Staff", Roles: []string{"site_member"}, Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.NewString(), TenantID: env.tenantID, SiteID: siteB.ID, UserID: staff.ID,
		Role: "Staff", Roles: []string{"site_member"}, Status: "active",
	}).Error)

	instA := models.Instrument{
		ID: uuid.NewString(), TenantID: env.tenantID, OrgID: strPtrOrderVisibility(env.siteOrgA),
		SiteID: uuidPtrOrderVisibility(siteA.ID), SN: "SN-2621-A", StockStatus: "available",
	}
	instB := models.Instrument{
		ID: uuid.NewString(), TenantID: env.tenantID, OrgID: strPtrOrderVisibility(env.siteOrgB),
		SiteID: uuidPtrOrderVisibility(siteB.ID), SN: "SN-2621-B", StockStatus: "available",
	}
	require.NoError(t, db.Create(&instA).Error)
	require.NoError(t, db.Create(&instB).Error)

	mkOrder := func(instrumentID, no string) string {
		id := uuid.NewString()
		require.NoError(t, db.Create(&models.Order{
			ID: id, TenantID: env.tenantID, OrgID: env.siteOrgA,
			UserID: customer.ID, InstrumentID: instrumentID,
			Level: "standard", LeaseTerm: 3,
			MonthlyRent: models.FromYuan(100), Deposit: models.FromYuan(500),
			Status: models.OrderStatusReserved, OrderNo: no,
		}).Error)
		return id
	}
	env.orderA = mkOrder(instA.ID, "YL26210001")
	env.orderB = mkOrder(instB.ID, "YL26210002")

	router := gin.New()
	router.GET("/orders", GetOrders)
	return router, env
}

func dbForOrderVisibility2621(t *testing.T) *gorm.DB {
	t.Helper()
	return database.GetDB()
}

func strPtrOrderVisibility(s string) *string { return &s }
func uuidPtrOrderVisibility(s string) *uuid.UUID {
	u := uuid.MustParse(s)
	return &u
}

// listOrders2621 以指定 actor 请求订单列表，返回订单 ID 集合（解析 JSON 响应）。
func listOrders2621(t *testing.T, router *gin.Engine, actor testutil.TestActor) []string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/orders?page=1&page_size=50", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				ID string `json:"id"`
			} `json:"list"`
			Total int `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), w.Body.String())
	ids := make([]string, 0, len(resp.Data.List))
	for _, item := range resp.Data.List {
		ids = append(ids, item.ID)
	}
	return ids
}

func contains2621(list []string, id string) bool {
	for _, v := range list {
		if v == id {
			return true
		}
	}
	return false
}

// TestOrderVisibility2621：双网点员工以 A 网点 oid 请求 → 只看到 A 网点订单；
// 自查询失败 / oid 为空 → 空列表（fail-closed），不按 tenant 放宽。
func TestOrderVisibility2621(t *testing.T) {
	router, env := setupOrderVisibility2621(t)

	t.Run("staff_sees_only_current_site", func(t *testing.T) {
		actor := testutil.TestActor{
			TenantID: env.tenantID, OrgID: env.siteOrgA, UserID: env.staffSub, Role: "STAFF",
		}
		ids := listOrders2621(t, router, actor)

		staffScoped := 0
		if contains2621(ids, env.orderA) {
			staffScoped = 1
		}
		crossSiteHidden := 0
		if !contains2621(ids, env.orderB) {
			crossSiteHidden = 1
		}
		t.Logf("STAFF_ORDER_SITE_SCOPED=%d", staffScoped)
		t.Logf("CROSS_SITE_ORDER_HIDDEN=%d", crossSiteHidden)
		require.Equal(t, 1, staffScoped, "当前网点订单必须可见；实际=%v", ids)
		require.Equal(t, 1, crossSiteHidden, "跨网点订单必须隐藏（不得按 site_members 全量或其域名放开）；实际=%v", ids)
	})

	t.Run("self_query_failure_fail_closed", func(t *testing.T) {
		// 本地无该 sub 的 users 行 → 身份自查询失败；旧实现会跳过网点过滤返回全租户订单。
		actor := testutil.TestActor{
			TenantID: env.tenantID, OrgID: env.siteOrgA, UserID: "iam-sub-2621-order-missing", Role: "STAFF",
		}
		ids := listOrders2621(t, router, actor)
		failClosed := 0
		if len(ids) == 0 {
			failClosed = 1
		}
		t.Logf("SELF_QUERY_FAIL_CLOSED=%d", failClosed)
		t.Logf("SELF_QUERY_FAIL_LIST_LEN=%d", len(ids))
		require.Equal(t, 1, failClosed, "自查询失败必须 fail-closed（空列表），实际=%v", ids)
	})

	t.Run("no_oid_empty_list", func(t *testing.T) {
		// oid 为空（未选网点/上下文缺失）→ 空列表，不得按商户范围返回。
		actor := testutil.TestActor{
			TenantID: env.tenantID, OrgID: "", UserID: env.staffSub, Role: "STAFF",
		}
		ids := listOrders2621(t, router, actor)
		noOidEmpty := 0
		if len(ids) == 0 {
			noOidEmpty = 1
		}
		t.Logf("NO_OID_EMPTY_LIST=%d", noOidEmpty)
		t.Logf("NO_OID_LIST_LEN=%d", len(ids))
		require.Equal(t, 1, noOidEmpty, "oid 为空必须返回空列表，实际=%v", ids)
	})

	t.Run("tenant_wide_fallback_absent", func(t *testing.T) {
		// 两条失败分支（自查询失败 / oid 空）都不得退化为租户全量。
		fallback := 0
		for _, actor := range []testutil.TestActor{
			{TenantID: env.tenantID, OrgID: env.siteOrgA, UserID: "iam-sub-2621-order-missing", Role: "STAFF"},
			{TenantID: env.tenantID, OrgID: "", UserID: env.staffSub, Role: "STAFF"},
		} {
			ids := listOrders2621(t, router, actor)
			if len(ids) > 0 {
				fallback++
			}
		}
		t.Logf("TENANT_WIDE_FALLBACK=%d", fallback)
		require.Equal(t, 0, fallback, "失败分支退化为租户全量")
	})
}
