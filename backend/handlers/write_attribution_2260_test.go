package handlers

// #2260（B2b）：S4 写入使用**数据归属**，不使用操作者 JWT tid。
//   1) 乐器促销覆盖 → 乐器商户 org（且范围校验，跨商户 404）
//   2) 商户结算配置 → 商户 org（merchants.tenant_id 可能多商户共享）

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

func TestPromoOverride_WriteAnchoredToInstrumentOrg_2260(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	shared := uuid.NewString()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	siteOrgA := uuid.NewString()
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
	r.GET("/instruments/:id/promo-overrides", GetInstrumentPromoOverrides)
	r.PUT("/instruments/:id/promo-overrides", UpdateInstrumentPromoOverride)

	// 平台管理员（tid 空）：旧实现在 uuid 列写入 '' → 500；修复后锚定乐器商户 org
	platformAdmin := testutil.TestActor{TenantID: "", OrgID: "", UserID: uuid.NewString(), Role: "NAMESPACE_ADMIN"}
	code, resp := waPost(t, r, http.MethodPut, "/instruments/"+instID+"/promo-overrides", platformAdmin,
		map[string]interface{}{"override_type": "rent_to_own", "enabled": true, "content": "可租转售"})
	require.Equal(t, http.StatusOK, code, "%v", resp)

	var row models.InstrumentPromoOverride
	require.NoError(t, db.Where("instrument_id = ? AND override_type = ?", instID, "rent_to_own").First(&row).Error)
	assert.Equal(t, orgA, row.TenantID, "覆盖配置必须锚定乐器商户 org（数据归属）")

	// 读回（同锚）
	code, resp = waPost(t, r, http.MethodGet, "/instruments/"+instID+"/promo-overrides", platformAdmin, nil)
	require.Equal(t, http.StatusOK, code, "%v", resp)
	if list, ok := resp["data"].([]interface{}); ok {
		assert.Len(t, list, 1)
	}

	// 跨商户：商户 B 管理员对 A 的乐器 → 404，且不得写入 B 名下
	merchantBAdmin := testutil.TestActor{TenantID: orgB, OrgID: uuid.NewString(), UserID: uuid.NewString(), Role: "OWNER"}
	code, _ = waPost(t, r, http.MethodPut, "/instruments/"+instID+"/promo-overrides", merchantBAdmin,
		map[string]interface{}{"override_type": "rent_to_own", "enabled": true})
	assert.Equal(t, http.StatusNotFound, code, "跨商户覆盖必须 404")
	var crossCnt int64
	db.Model(&models.InstrumentPromoOverride{}).Where("tenant_id = ? AND instrument_id = ?", orgB, instID).Count(&crossCnt)
	assert.Equal(t, int64(0), crossCnt, "不得在他商户 org 下留下覆盖行")
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
	admin := testutil.TestActor{TenantID: orgA, OrgID: orgA, UserID: uuid.NewString(), Role: "OWNER"}

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
