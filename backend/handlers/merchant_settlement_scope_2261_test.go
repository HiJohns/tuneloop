package handlers

// #2261（B3 域3 前置）：结算配置读写必须带显式商户范围。
// 商户管理员只能访问自己商户；平台管理员（无法解析商户）不受限。
// 父提交缺陷：商户管理员因共享 tenant 与自动范围不匹配，连自己商户都 404。

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

func TestSettlementConfig_ExplicitMerchantScope_2261(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	shared := uuid.NewString()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	merchantA := uuid.NewString()
	merchantB := uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: merchantA, TenantID: shared, OrgID: orgA, Name: "SC-A",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)
	require.NoError(t, db.Create(&models.Merchant{
		ID: merchantB, TenantID: shared, OrgID: orgB, Name: "SC-B",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.TestActor{
			TenantID: c.GetHeader("X-Tenant"), OrgID: c.GetHeader("X-Org"),
			UserID: uuid.NewString(), Role: c.GetHeader("X-Role"),
		}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.GET("/admin/merchant/:id/settlement", GetSettlementConfig)
	r.PUT("/admin/merchant/:id/settlement", UpsertSettlementConfig)

	call := func(method, path, tenant, org, role string, body interface{}) (int, map[string]interface{}) {
		var raw *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			raw = bytes.NewReader(b)
		} else {
			raw = bytes.NewReader(nil)
		}
		req := httptest.NewRequest(method, path, raw)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Tenant", tenant)
		req.Header.Set("X-Org", org)
		req.Header.Set("X-Role", role)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp
	}

	// 商户 A 管理员：本商户读/写 200；他商户读/写 404
	code, _ := call(http.MethodGet, "/admin/merchant/"+merchantA+"/settlement", orgA, orgA, "OWNER", nil)
	assert.Equal(t, http.StatusOK, code, "本商户结算配置必须可读（修复共享 tenant 下 404）")

	code, _ = call(http.MethodPut, "/admin/merchant/"+merchantA+"/settlement", orgA, orgA, "OWNER", map[string]interface{}{
		"receiver_type": "bank", "receiver_account": "6222", "profit_share_ratio": 0.5, "is_enabled": true,
	})
	assert.Equal(t, http.StatusOK, code, "本商户结算配置必须可写")

	code, _ = call(http.MethodGet, "/admin/merchant/"+merchantB+"/settlement", orgA, orgA, "OWNER", nil)
	assert.Equal(t, http.StatusNotFound, code, "跨商户读取必须 404")
	code, _ = call(http.MethodPut, "/admin/merchant/"+merchantB+"/settlement", orgA, orgA, "OWNER", map[string]interface{}{
		"receiver_type": "bank", "receiver_account": "6223", "profit_share_ratio": 0.6, "is_enabled": true,
	})
	assert.Equal(t, http.StatusNotFound, code, "跨商户写入必须 404")

	// 平台管理员（无法解析商户）：可访问任意商户
	code, _ = call(http.MethodGet, "/admin/merchant/"+merchantB+"/settlement", "", "", "NAMESPACE_ADMIN", nil)
	assert.Equal(t, http.StatusOK, code, "平台管理员可读任意商户结算配置")

	// 落库锚定 A 的 org
	var cfg models.MerchantSettlementConfig
	require.NoError(t, db.Where("tenant_id = ?", orgA).First(&cfg).Error)
	assert.Equal(t, "6222", cfg.ReceiverAccount)
}
