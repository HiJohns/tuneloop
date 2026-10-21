package handlers

// #2255（P3 批次1，A5 ISO-13 补强）：维修服务单操作面跨商户隔离——
// 商户 B 的员工对商户 A 的单报价必须 403，本商户员工可正常报价（正向对照）。

import (
	"net/http"
	"testing"

	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestRepairService_Quote_CrossMerchantForbidden_2255(t *testing.T) {
	f := setupRepairServiceFixture(t)
	rrID := rsSeedOrder2229(t, f, models.RepairReqStatusPendingQuote, f.customerID, "Q2255")

	// 跨商户：不同 tenant/org 的员工 → 404（loadRepairService 按商户范围隐藏）或 403
	otherActor := testutil.TestActor{
		TenantID: f.otherTenantID, OrgID: uuid.NewString(), UserID: uuid.NewString(), Role: "site_admin",
	}
	code, _ := svcPost(t, f, otherActor, "/repair-services/"+rrID+"/quote",
		map[string]interface{}{"quote_repair_cents": 1000})
	assert.Contains(t, []int{http.StatusNotFound, http.StatusForbidden}, code, "跨商户报价必须被拒（404/403）")

	var after models.RepairRequest
	assert.NoError(t, f.db.Where("id = ?", rrID).First(&after).Error)
	assert.Equal(t, models.RepairReqStatusPendingQuote, after.Status, "越权请求不得改变状态")

	// 正向对照：本商户维修师可报价
	ownActor := testutil.TestActor{TenantID: f.tenantID, OrgID: f.orgID, UserID: f.techID, Role: "repair_technician"}
	code, resp := svcPost(t, f, ownActor, "/repair-services/"+rrID+"/quote",
		map[string]interface{}{"quote_repair_cents": 1000})
	assert.Equal(t, http.StatusOK, code, "%v", resp)
}
