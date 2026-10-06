package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// svcDoScoped（#2134）：注入 database 租户键，生产等价地激活 GORM addTenantScope。
// 默认 svcPost 只注入 middleware 键，不触发自动租户范围
// （见 TestLocalUserIDBySub_ZeroTenantUnderStaffScope_2090 说明）。
func svcDoScoped(t *testing.T, f svcFixture, actor testutil.TestActor, method, path string, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	ctx := actor.InjectContext(req.Context())
	ctx = database.SetTenantID(ctx, actor.TenantID) // 生产等价：激活自动租户范围
	ctx = database.SetOrgID(ctx, actor.OrgID)
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req.WithContext(ctx))
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

func svcListIDs2134(t *testing.T, resp map[string]interface{}) []string {
	t.Helper()
	ids := []string{}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		return ids
	}
	list, ok := data["list"].([]interface{})
	if !ok {
		return ids
	}
	for _, it := range list {
		if m, ok := it.(map[string]interface{}); ok {
			if id, ok := m["id"].(string); ok {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// #2134：写入链把「叶组织 id」存进 tenant_id（商户 org），staff 上下文 tid=根租户
// → 自动租户范围过滤掉该行。修复后 staff 读按 {tid, oid} 显式过滤，详情/列表均可见。
func TestRepairService_LeafOrgTenantScope_2134(t *testing.T) {
	f := setupRepairServiceFixture(t)
	rr := models.RepairRequest{
		ID: uuid.NewString(), TenantID: f.orgID, UserID: f.customerID, Type: "service",
		Status: models.RepairReqStatusClosed, TechnicianID: &f.techID, SiteID: "",
		Description: "叶组织租户口径回归", Photos: "[]", ReceivePhotos: "[]",
	}
	require.NoError(t, f.db.Omit("site_id", "user_instrument_id").Create(&rr).Error)
	tech := testutil.TestActor{TenantID: f.tenantID, OrgID: f.orgID, UserID: f.techID, Role: "repair_technician"}

	// ① 详情：能读到 tenant_id=叶组织 的行（修复前：自动范围 tenant=根租户 → 404）
	code, resp := svcDoScoped(t, f, tech, http.MethodGet, "/user/repair-services/"+rr.ID, nil)
	require.Equal(t, http.StatusOK, code, resp)
	assert.Equal(t, float64(20000), resp["code"], resp)

	// ② scope=mine 列表：包含该行（修复前 0 条）
	_, lresp := svcDoScoped(t, f, tech, http.MethodGet, "/repair-services?scope=mine&status=closed", nil)
	assert.Contains(t, svcListIDs2134(t, lresp), rr.ID, "技师工作台 scope=mine 应可见叶组织行")

	// ③ scope=site：包含该行（本商户无 site 单）
	_, sresp := svcDoScoped(t, f, tech, http.MethodGet, "/repair-services?scope=site&status=closed", nil)
	assert.Contains(t, svcListIDs2134(t, sresp), rr.ID, "scope=site 应可见本商户叶组织行")
}

// #2134：隔离——同技师但属其它租户的行不得跨租户可见。
func TestRepairService_LeafOrgScope_Isolation_2134(t *testing.T) {
	f := setupRepairServiceFixture(t)
	other := models.RepairRequest{
		ID: uuid.NewString(), TenantID: f.otherTenantID, UserID: f.customerID, Type: "service",
		Status: models.RepairReqStatusClosed, TechnicianID: &f.techID,
		Description: "异租户", Photos: "[]", ReceivePhotos: "[]",
	}
	require.NoError(t, f.db.Omit("site_id", "user_instrument_id").Create(&other).Error)
	tech := testutil.TestActor{TenantID: f.tenantID, OrgID: f.orgID, UserID: f.techID, Role: "repair_technician"}

	_, resp := svcDoScoped(t, f, tech, http.MethodGet, "/repair-services?scope=mine&status=closed", nil)
	assert.NotContains(t, svcListIDs2134(t, resp), other.ID, "异租户行不得跨租户可见")
}

// #2134 P2：写入侧规范化——师傅档案 tenant_id=叶组织时，创建单应落根租户。
func TestRepairService_CreateNormalizesTenant_2134(t *testing.T) {
	f := setupRepairServiceFixture(t)
	require.NoError(t, f.db.Model(&models.TechnicianProfile{}).
		Where("user_id = ?", f.techID).Update("tenant_id", f.orgID).Error)
	customer := testutil.MakeCustomer("", f.customerSub)

	_, resp := svcPost(t, f, customer, "/user/repair-services",
		gin.H{"description": "规范化", "technician_id": f.techID})
	require.Equal(t, float64(20000), resp["code"], resp)
	id := svcData(t, resp)["id"].(string)

	var got models.RepairRequest
	require.NoError(t, f.db.Where("id = ?", id).First(&got).Error)
	assert.Equal(t, f.tenantID, got.TenantID, "写入应把叶组织规范化为根租户")
}
