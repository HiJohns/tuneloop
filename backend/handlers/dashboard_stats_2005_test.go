package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

func dashboardStatsRouter(role, tid, oid string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, role)
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tid)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, oid)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	dh := NewDashboardHandler(database.GetDB())
	r.GET("/stats", dh.GetDashboardStats)
	return r
}

func callDashboardStats(t *testing.T, role, tid, oid string) (int, map[string]interface{}) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stats", nil)
	dashboardStatsRouter(role, tid, oid).ServeHTTP(w, req)
	var body struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return w.Code, body.Data
}

func asInt(t *testing.T, data map[string]interface{}, key string) int {
	t.Helper()
	v, ok := data[key]
	require.Truef(t, ok, "missing key %s", key)
	f, ok := v.(float64)
	require.Truef(t, ok, "key %s not a number: %T", key, v)
	return int(f)
}

// #2005 S1: system_admin → 平台治理指标（全平台，无经营 KPI）
func TestDashboardStats_2005_Governance(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.Tenant{}, &models.Merchant{}, &models.User{}))

	tid := uuid.New().String()
	require.NoError(t, db.Create(&models.Tenant{ID: tid, Name: "S1-" + tid[:8]}).Error)
	require.NoError(t, db.Create(&models.Merchant{ID: uuid.New().String(), TenantID: tid, OrgID: tid, AdminUID: uuid.New().String(), Name: "S1商户"}).Error)
	require.NoError(t, db.Create(&models.User{
		ID: uuid.New().String(), IAMSub: uuid.New().String(), TenantID: tid, OrgID: tid,
		Name: "S1用户", Username: "s1-" + tid[:8], Status: "active",
	}).Error)

	code, data := callDashboardStats(t, "NAMESPACE_ADMIN", "", "")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "system_admin", data["role"])
	require.GreaterOrEqual(t, asInt(t, data, "merchants_count"), 1)
	require.GreaterOrEqual(t, asInt(t, data, "users_count"), 1)
	_, hasAssets := data["total_assets"]
	require.False(t, hasAssets, "治理视图不应返回经营 KPI（total_assets）")

	recent, ok := data["recent_merchants"].([]interface{})
	require.True(t, ok)
	found := false
	for _, it := range recent {
		m, _ := it.(map[string]interface{})
		if m["name"] == "S1商户" {
			found = true
		}
	}
	require.True(t, found, "recent_merchants 应包含新建商户")
}

// #2005 S1: merchant_admin 按 tenant_id 作用域；租约源 = lease_sessions
func TestDashboardStats_2005_MerchantScope(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.Instrument{}, &models.LeaseSession{}, &models.Order{}))

	tid := uuid.New().String()
	otherTid := uuid.New().String()

	// #2009 口径统一：租约指标取自 orders（in_lease / expired），不再用 lease_sessions
	todayStr := time.Now().Format("2006-01-02")
	seedScope := func(tenant string) {
		require.NoError(t, db.Create(&models.Instrument{TenantID: tenant, OrgID: &tenant, SN: "A-" + uuid.New().String()[:8], StockStatus: "rented"}).Error)
		require.NoError(t, db.Create(&models.Instrument{TenantID: tenant, OrgID: &tenant, SN: "B-" + uuid.New().String()[:8], StockStatus: "available"}).Error)
		// 3 条在租（今天/昨天/未来到期）+ 1 条逾期
		for _, ed := range []string{todayStr, time.Now().AddDate(0, 0, -1).Format("2006-01-02"), time.Now().AddDate(0, 0, 3).Format("2006-01-02")} {
			end := ed
			require.NoError(t, db.Create(&models.Order{
				TenantID: tenant, OrgID: tenant, UserID: uuid.New().String(), InstrumentID: uuid.New().String(),
				Level: "standard", LeaseTerm: 1, MonthlyRent: models.FromYuan(100),
				Status: models.OrderStatusInLease, EndDate: &end, CashPaid: models.FromYuan(100),
				CreatedAt: time.Now().AddDate(0, 0, -10), UpdatedAt: time.Now(),
			}).Error)
		}
		require.NoError(t, db.Create(&models.Order{
			TenantID: tenant, OrgID: tenant, UserID: uuid.New().String(), InstrumentID: uuid.New().String(),
			Level: "standard", LeaseTerm: 1, MonthlyRent: models.FromYuan(100),
			Status: models.OrderStatusExpired, CreatedAt: time.Now().AddDate(0, 0, -10), UpdatedAt: time.Now(),
		}).Error)
		// 1 条今日完成（今日新订单 + 收入）
		require.NoError(t, db.Create(&models.Order{
			TenantID: tenant, OrgID: tenant, UserID: uuid.New().String(), InstrumentID: uuid.New().String(),
			Level: "standard", LeaseTerm: 1, MonthlyRent: models.FromYuan(100),
			Status: "completed", CashPaid: models.FromYuan(100), CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}).Error)
	}
	seedScope(tid)
	seedScope(otherTid) // 不应计入

	code, data := callDashboardStats(t, "ADMIN", tid, tid) // tid==oid → merchant_admin
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "merchant_admin", data["role"])
	require.Equal(t, 2, asInt(t, data, "total_assets"))
	require.Equal(t, 1, asInt(t, data, "rented_assets"))
	require.Equal(t, 1, asInt(t, data, "available_assets"))
	require.Equal(t, 3, asInt(t, data, "active_leases"), "in_lease 订单")
	require.Equal(t, 1, asInt(t, data, "expiring_today"), "in_lease 且 end_date=今天")
	require.Equal(t, 1, asInt(t, data, "overdue"), "expired 订单")
	require.Equal(t, 1, asInt(t, data, "new_orders_today"))
}

// #2005 S1: site_admin 按 org_id 作用域
func TestDashboardStats_2005_SiteScope(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.Instrument{}))

	tid := uuid.New().String()
	oid := uuid.New().String()
	otherOid := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{TenantID: tid, OrgID: &oid, SN: "S-" + uuid.New().String()[:8], StockStatus: "rented"}).Error)
	require.NoError(t, db.Create(&models.Instrument{TenantID: tid, OrgID: &otherOid, SN: "X-" + uuid.New().String()[:8], StockStatus: "rented"}).Error)

	code, data := callDashboardStats(t, "ADMIN", tid, oid) // tid!=oid → site_admin
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "site_admin", data["role"])
	require.Equal(t, 1, asInt(t, data, "total_assets"), "仅计本网点")
}

// #2005 S1: 无作用域身份（customer）→ 403，避免空作用域全量泄漏
func TestDashboardStats_2005_CustomerForbidden(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	code, _ := callDashboardStats(t, "USER", "", "")
	require.Equal(t, http.StatusForbidden, code)
}

// #2006 QA 返工：聚合查询失败必须 500，不得静默返回 0（HTTP 200）。
// 注入方式：删除 merchants 表 → 治理支首个聚合失败。
func TestDashboardStats_2005_AggregationErrorReturns500(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.Merchant{}))
	require.NoError(t, db.Migrator().DropTable(&models.Merchant{}))
	t.Cleanup(func() { _ = db.AutoMigrate(&models.Merchant{}) })

	code, _ := callDashboardStats(t, "NAMESPACE_ADMIN", "", "")
	require.Equal(t, http.StatusInternalServerError, code, "聚合失败必须 500，不得静默 0")
}
