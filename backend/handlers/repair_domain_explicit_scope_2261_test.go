package handlers

// #2261（P3 B3 批次3 前置）：repair 域在「显式租户范围模式」下的隔离守卫。
// 目的：证明 repair 域不依赖 addTenantScope 自动注入——即使上下文标记
// ExplicitTenantScopeOnly（关闭自动过滤），跨商户数据仍不可见。
// 这是逐域切换（repair → orders/instruments → …）的第一块就绪证据。

import (
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

func TestRepairDomain_ExplicitScopeIsolation_2261(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	orgA, orgB := uuid.NewString(), uuid.NewString()
	sharedTenant := uuid.NewString() // 刻意共享 tenant，验证不依赖它
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: sharedTenant, OrgID: orgA, Name: "RA-2261",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: sharedTenant, OrgID: orgB, Name: "RB-2261",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)

	tech := uuid.NewString()
	rowA := models.RepairRequest{ID: uuid.NewString(), TenantID: orgA, UserID: uuid.NewString(), Type: "service", Status: models.RepairReqStatusPendingQuote, TechnicianID: &tech}
	rowB := models.RepairRequest{ID: uuid.NewString(), TenantID: orgB, UserID: uuid.NewString(), Type: "service", Status: models.RepairReqStatusPendingQuote, TechnicianID: &tech}
	require.NoError(t, db.Omit("site_id", "user_instrument_id").Create(&rowA).Error)
	require.NoError(t, db.Omit("site_id", "user_instrument_id").Create(&rowB).Error)

	// 关键：请求上下文**关闭自动租户范围**（模拟逐域切换后的形态）
	actor := testutil.TestActor{TenantID: orgA, OrgID: orgA, UserID: uuid.NewString(), Role: "OWNER"}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := database.ExplicitTenantScopeOnly(actor.InjectContext(c.Request.Context()))
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.GET("/admin/repair-services", ListMerchantRepairServices)

	req := httptest.NewRequest(http.MethodGet, "/admin/repair-services?page=1&page_size=20", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	ids := map[string]bool{}
	for _, it := range resp.Data.List {
		if id, ok := it["id"].(string); ok {
			ids[id] = true
		}
	}
	assert.True(t, ids[rowA.ID], "本商户服务单必须可见（显式范围模式）")
	assert.False(t, ids[rowB.ID], "关闭自动租户范围后仍不得跨商户可见（repair 域已显式锚定）")
}
