package handlers

// #2260（B2b）：S4 写入使用**数据归属**，不使用操作者 JWT tid。
//   1) 乐器促销覆盖 → 乐器商户 org（且范围校验，跨商户 404）
//   2) 商户结算配置 → 商户 org（merchants.tenant_id 可能多商户共享）

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func waPost(t *testing.T, r *gin.Engine, method, path string, actor testutil.TestActor, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	var raw *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		raw = bytes.NewReader(b)
	} else {
		raw = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, raw)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

// 生产可达身份 = site_admin（唯一持有 promo:override 的角色；tid=商户 org）。
// 可观测缺陷：商户 B 的 site_admin 对商户 A 的乐器写覆盖 → 旧实现会在 B 名下
// 留下不可见脏行（跨商户写入）；修复后 404 且不落行。
func TestPromoOverride_CrossMerchantForbidden_2260(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	shared := uuid.NewString()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	siteOrgA, siteOrgB := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: shared, OrgID: orgA, Name: "WA-A",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: shared, OrgID: orgB, Name: "WA-B",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)
	instID := uuid.NewString()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: orgA, OrgID: &siteOrgA, SN: "SN-WA-1", StockStatus: "available",
	}).Error)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(c.Request.Context())
		c.Next()
	})
	r.GET("/instruments/:id/promo-overrides", GetInstrumentPromoOverrides)
	r.PUT("/instruments/:id/promo-overrides", UpdateInstrumentPromoOverride)

	call := func(actor testutil.TestActor, method string, body interface{}) (int, map[string]interface{}) {
		var raw *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			raw = bytes.NewReader(b)
		} else {
			raw = bytes.NewReader(nil)
		}
		req := httptest.NewRequest(method, "/instruments/"+instID+"/promo-overrides", raw)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp
	}

	// 商户 B 的 site_admin（真实可达身份）→ 404，且不得在 B 名下留行
	siteAdminB := testutil.MakeSiteAdmin(orgB, siteOrgB, uuid.NewString())
	code, _ := call(siteAdminB, http.MethodPut, map[string]interface{}{"override_type": "rent_to_own", "enabled": true})
	assert.Equal(t, http.StatusNotFound, code, "跨商户覆盖必须 404（旧实现会在 B 名下写脏行）")
	var crossCnt int64
	db.Model(&models.InstrumentPromoOverride{}).Where("instrument_id = ?", instID).Count(&crossCnt)
	assert.Equal(t, int64(0), crossCnt, "越权请求不得留下任何覆盖行")

	// 本商户 site_admin（同角色）→ 200，锚定乐器商户 org
	siteAdminA := testutil.MakeSiteAdmin(orgA, siteOrgA, uuid.NewString())
	code, resp := call(siteAdminA, http.MethodPut, map[string]interface{}{"override_type": "rent_to_own", "enabled": true, "content": "可租转售"})
	require.Equal(t, http.StatusOK, code, "%v", resp)
	var row models.InstrumentPromoOverride
	require.NoError(t, db.Where("instrument_id = ? AND override_type = ?", instID, "rent_to_own").First(&row).Error)
	assert.Equal(t, orgA, row.TenantID, "覆盖配置锚定乐器商户 org")

	// 读回
	code, resp = call(siteAdminA, http.MethodGet, nil)
	require.Equal(t, http.StatusOK, code, "%v", resp)
	if list, ok := resp["data"].([]interface{}); ok {
		assert.Len(t, list, 1)
	}
}

func TestSettlementConfig_AnchoredToMerchantOrg_2260(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	shared := uuid.NewString()
	orgA := uuid.NewString()
	merchantID := uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: merchantID, TenantID: shared, OrgID: orgA, Name: "WA-S",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)

	r := gin.New()
	r.GET("/admin/merchant/:id/settlement", GetSettlementConfig)
	r.PUT("/admin/merchant/:id/settlement", UpsertSettlementConfig)
	// 生产可达身份：该端点在 authRequired（平台管理员上下文 tid 为空，无租户过滤）
	admin := testutil.TestActor{TenantID: "", OrgID: "", UserID: uuid.NewString(), Role: "NAMESPACE_ADMIN"}

	code, resp := waPost(t, r, http.MethodPut, "/admin/merchant/"+merchantID+"/settlement", admin,
		map[string]interface{}{
			"receiver_type": "bank", "receiver_account": "6222000011112222",
			"profit_share_ratio": 0.5, "is_enabled": true,
		})
	require.Equal(t, http.StatusOK, code, "%v", resp)

	var cfg models.MerchantSettlementConfig
	require.NoError(t, db.Where("tenant_id = ?", orgA).First(&cfg).Error)
	assert.Equal(t, orgA, cfg.TenantID, "结算配置必须锚定商户 org（不得用共享 tenant）")
	assert.Equal(t, "6222000011112222", cfg.ReceiverAccount)

	// 关系断言：computePaymentDeadline 的读取口径（tenant_id = 订单商户 org + is_enabled）
	var n int64
	db.Model(&models.MerchantSettlementConfig{}).
		Where("tenant_id = ? AND is_enabled = ?", orgA, true).Count(&n)
	assert.Equal(t, int64(1), n, "订单侧按商户 org 读取配置必须命中（否则支付超时配置失效）")

	// 读回
	code, resp = waPost(t, r, http.MethodGet, "/admin/merchant/"+merchantID+"/settlement", admin, nil)
	require.Equal(t, http.StatusOK, code, "%v", resp)
}

// #2260 审计修复（Bug 2）：证明测试夹具在显式注入 database 键后，
// GORM 租户回调（addTenantScope）确实生效——TestActor 仅写 middleware key，
// 不含 database 键，故默认不激活回调；本测试给出「机制可被激活」的证据。
func TestTenantScopeCallback_ActiveWithDatabaseKeys_2260(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: uuid.NewString(), OrgID: orgA, Name: "SC-A",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: uuid.NewString(), OrgID: orgB, Name: "SC-B",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)

	base := context.Background()
	unscopedDB := db.WithContext(database.IdentityCtx(base))
	var all int64
	require.NoError(t, unscopedDB.Model(&models.Merchant{}).Count(&all).Error)
	require.Equal(t, int64(2), all)

	// 注入 database 租户键（生产中间件会写）→ 回调加 tenant_id 过滤
	scopedCtx := database.SetTenantID(base, uuid.NewString())
	var filtered int64
	require.NoError(t, db.WithContext(scopedCtx).Model(&models.Merchant{}).Count(&filtered).Error)
	assert.Equal(t, int64(0), filtered, "租户键生效后查询必须被过滤（证明回调机制在测试链路可激活）")

	// 控制组：清空租户键（IdentityCtx）→ 恢复全量
	var restored int64
	require.NoError(t, db.WithContext(database.IdentityCtx(scopedCtx)).Model(&models.Merchant{}).Count(&restored).Error)
	assert.Equal(t, int64(2), restored)
}

// #2260：顾客（零租户）创建申诉——归属取被申诉对象（repair_request）的商户 org；
// 无法解析时用平台零 UUID（不再写空串导致 500）。
func TestCreateAppeal_AnchorFromObject_2260(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	orgA := uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: uuid.NewString(), OrgID: orgA, Name: "AP-A",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)
	tech := uuid.NewString()
	rrID := uuid.NewString()
	require.NoError(t, db.Omit("site_id", "user_instrument_id").Create(&models.RepairRequest{
		ID: rrID, TenantID: orgA, UserID: uuid.NewString(), Type: "service",
		Status: models.RepairReqStatusQuoted, TechnicianID: &tech, Description: "申诉目标",
		Photos: "[]", ReceivePhotos: "[]",
	}).Error)

	r := gin.New()
	r.POST("/repair-appeals", CreateAppeal)

	sub := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: uuid.NewString(), IAMSub: sub, Name: "顾客AP", Phone: "13900005501", Status: "active",
		TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID:    "00000000-0000-0000-0000-000000000000",
	}).Error)
	customer := testutil.TestActor{TenantID: "", OrgID: "", UserID: sub, Role: "USER"}

	call := func(body map[string]interface{}) (int, map[string]interface{}) {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/repair-appeals", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req.WithContext(customer.InjectContext(req.Context())))
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp
	}

	code, resp := call(map[string]interface{}{
		"category": "damage", "object_type": "repair_request", "object_id": rrID, "description": "有异议",
	})
	require.Equal(t, http.StatusOK, code, "%v", resp)
	var appeal models.Appeal
	require.NoError(t, db.Where("object_id = ?", rrID).First(&appeal).Error)
	assert.Equal(t, orgA, appeal.TenantID, "申诉归属必须落被申诉对象的商户 org")

	// 无法解析对象 → 平台零 UUID 占位（不再 500）
	code, resp = call(map[string]interface{}{
		"category": "other", "object_type": "repair_request", "object_id": uuid.NewString(), "description": "未知对象",
	})
	require.Equal(t, http.StatusOK, code, "%v", resp)
	var unknown models.Appeal
	require.NoError(t, db.Where("category = ?", "other").First(&unknown).Error)
	assert.Equal(t, "00000000-0000-0000-0000-000000000000", unknown.TenantID)
}
