package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2116：收货双路径（师傅直收→repairing / 员工代收→pending_repair→开始维修）+ Complete 收紧。

// bodyCode2116 取响应体业务码（svcPost 首个返回值为 HTTP 状态码）。
func bodyCode2116(t *testing.T, resp map[string]interface{}) int {
	t.Helper()
	c, _ := resp["code"].(float64)
	return int(c)
}

func setup2116ShippingOrder(t *testing.T, f svcFixture, status string) models.RepairRequest {
	t.Helper()
	rr := models.RepairRequest{
		ID: uuid.NewString(), TenantID: f.tenantID, UserID: f.customerID, Type: "service",
		Status: status, TechnicianID: &f.techID, SiteID: f.siteID, UserInstrumentID: uuid.NewString(),
		Description: "收货测试", Photos: "[]", ReceivePhotos: "[]",
	}
	require.NoError(t, f.db.Create(&rr).Error)
	return rr
}

func receive2116(t *testing.T, f svcFixture, actor testutil.TestActor, orderID string, photos []string) (int, map[string]interface{}) {
	t.Helper()
	return svcPost(t, f, actor, "/repair-services/"+orderID+"/receive", map[string]interface{}{"photos": photos})
}

func TestReceive2116_TechnicianSelf_GoesRepairing(t *testing.T) {
	f := setupRepairServiceFixture(t)
	rr := setup2116ShippingOrder(t, f, models.RepairReqStatusShipping)
	tech := testutil.TestActor{TenantID: f.tenantID, OrgID: f.siteID, UserID: f.techID, Role: "repair_technician"}

	_, resp := receive2116(t, f, tech, rr.ID, []string{"p1.jpg"})
	require.Equal(t, 20000, bodyCode2116(t, resp), resp)

	var got models.RepairRequest
	require.NoError(t, f.db.Where("id = ?", rr.ID).First(&got).Error)
	assert.Equal(t, models.RepairReqStatusRepairing, got.Status, "师傅本人收货直接进入维修中")
	assert.Contains(t, got.ReceivePhotos, "p1.jpg", "拍照留档落库")
	require.NotNil(t, got.ReceivedAt, "收货时间落库")

	var tl models.RepairRequestRecord
	require.NoError(t, f.db.Where("repair_request_id = ? AND record_type = ?", rr.ID, "received_by_tech").First(&tl).Error)
}

func TestReceive2116_StaffOnBehalf_GoesPendingRepairAndNotifies(t *testing.T) {
	f := setupRepairServiceFixture(t)
	rr := setup2116ShippingOrder(t, f, models.RepairReqStatusShipping)
	staff := testutil.TestActor{TenantID: f.tenantID, OrgID: f.siteID, UserID: uuid.NewString(), Role: "site_member"}
	// staff actor 的 UserID 需存在于本地 users（resolveIAMSub/时间线写入者）——补一行
	require.NoError(t, f.db.Create(&models.User{
		ID: staff.UserID, IAMSub: staff.UserID, TenantID: f.tenantID, OrgID: f.siteID,
		Username: "u-" + staff.UserID[:8], Name: "代收员工", Status: "active",
	}).Error)

	_, resp := receive2116(t, f, staff, rr.ID, []string{"p2.jpg"})
	require.Equal(t, 20000, bodyCode2116(t, resp), resp)

	var got models.RepairRequest
	require.NoError(t, f.db.Where("id = ?", rr.ID).First(&got).Error)
	assert.Equal(t, models.RepairReqStatusPendingRepair, got.Status, "员工代收进入待维修")
	assert.Contains(t, got.ReceivePhotos, "p2.jpg")

	var tl models.RepairRequestRecord
	require.NoError(t, f.db.Where("repair_request_id = ? AND record_type = ?", rr.ID, "received_by_staff").First(&tl).Error)

	var n int64
	f.db.Model(&models.Notification{}).Where("user_id = ? AND type = ?", f.techID, "repair").Count(&n)
	assert.GreaterOrEqual(t, n, int64(1), "代收后应通知维修师")
}

func TestReceive2116_RequiresPhotos(t *testing.T) {
	f := setupRepairServiceFixture(t)
	rr := setup2116ShippingOrder(t, f, models.RepairReqStatusShipping)
	tech := testutil.TestActor{TenantID: f.tenantID, OrgID: f.siteID, UserID: f.techID, Role: "repair_technician"}

	_, resp := receive2116(t, f, tech, rr.ID, nil)
	assert.Equal(t, 40002, bodyCode2116(t, resp), "无照片应拒绝")
}

func TestReceive2116_RejectsNonShipping(t *testing.T) {
	f := setupRepairServiceFixture(t)
	rr := setup2116ShippingOrder(t, f, models.RepairReqStatusRepairing)
	tech := testutil.TestActor{TenantID: f.tenantID, OrgID: f.siteID, UserID: f.techID, Role: "repair_technician"}

	_, resp := receive2116(t, f, tech, rr.ID, []string{"p.jpg"})
	assert.Equal(t, 40900, bodyCode2116(t, resp), "仅 shipping 可收货")
}

func TestStart2116_TechnicianSelf(t *testing.T) {
	f := setupRepairServiceFixture(t)
	rr := setup2116ShippingOrder(t, f, models.RepairReqStatusPendingRepair)
	tech := testutil.TestActor{TenantID: f.tenantID, OrgID: f.siteID, UserID: f.techID, Role: "repair_technician"}

	_, resp := svcPost(t, f, tech, "/repair-services/"+rr.ID+"/start", nil)
	require.Equal(t, 20000, bodyCode2116(t, resp), resp)

	var got models.RepairRequest
	require.NoError(t, f.db.Where("id = ?", rr.ID).First(&got).Error)
	assert.Equal(t, models.RepairReqStatusRepairing, got.Status, "开始维修后进入维修中")

	var tl models.RepairRequestRecord
	require.NoError(t, f.db.Where("repair_request_id = ? AND record_type = ?", rr.ID, "repair_started").First(&tl).Error)
}

func TestStart2116_OnlyAssignedTechnician(t *testing.T) {
	f := setupRepairServiceFixture(t)
	rr := setup2116ShippingOrder(t, f, models.RepairReqStatusPendingRepair)
	other := testutil.TestActor{TenantID: f.tenantID, OrgID: f.siteID, UserID: uuid.NewString(), Role: "site_member"}

	_, resp := svcPost(t, f, other, "/repair-services/"+rr.ID+"/start", nil)
	assert.Equal(t, 40300, bodyCode2116(t, resp), "非师傅本人不可开始维修")
}

func TestComplete2116_ShippingDirectBlocked(t *testing.T) {
	f := setupRepairServiceFixture(t)
	rr := setup2116ShippingOrder(t, f, models.RepairReqStatusShipping)
	tech := testutil.TestActor{TenantID: f.tenantID, OrgID: f.siteID, UserID: f.techID, Role: "repair_technician"}

	_, resp := svcPost(t, f, tech, "/repair-services/"+rr.ID+"/complete", nil)
	require.Equal(t, 40900, bodyCode2116(t, resp), resp)
	assert.Contains(t, resp["message"], "current status", "shipping 直达完成已关闭（#2116）")
}

func TestComplete2116_FromRepairing(t *testing.T) {
	f := setupRepairServiceFixture(t)
	rr := setup2116ShippingOrder(t, f, models.RepairReqStatusRepairing)
	tech := testutil.TestActor{TenantID: f.tenantID, OrgID: f.siteID, UserID: f.techID, Role: "repair_technician"}

	_, resp := svcPost(t, f, tech, "/repair-services/"+rr.ID+"/complete", nil)
	require.Equal(t, 20000, bodyCode2116(t, resp), resp)

	var got models.RepairRequest
	require.NoError(t, f.db.Where("id = ?", rr.ID).First(&got).Error)
	assert.Equal(t, models.RepairReqStatusDoneRepair, got.Status)

	var n int64
	f.db.Model(&models.Notification{}).Where("user_id = ? AND title = ?", f.customerID, "维修完成").Count(&n)
	assert.GreaterOrEqual(t, n, int64(1), "完成修理应通知顾客")
}

func TestReceive2116_PhotosPersistedAsJSON(t *testing.T) {
	f := setupRepairServiceFixture(t)
	rr := setup2116ShippingOrder(t, f, models.RepairReqStatusShipping)
	staff := testutil.TestActor{TenantID: f.tenantID, OrgID: f.siteID, UserID: uuid.NewString(), Role: "site_member"}
	require.NoError(t, f.db.Create(&models.User{
		ID: staff.UserID, IAMSub: staff.UserID, TenantID: f.tenantID, OrgID: f.siteID,
		Username: "u-" + staff.UserID[:8], Name: "代收员工", Status: "active",
	}).Error)

	_, resp := receive2116(t, f, staff, rr.ID, []string{"a.jpg", "b.jpg"})
	require.Equal(t, 20000, bodyCode2116(t, resp))

	var got models.RepairRequest
	require.NoError(t, f.db.Where("id = ?", rr.ID).First(&got).Error)
	var photos []string
	require.NoError(t, json.Unmarshal([]byte(got.ReceivePhotos), &photos))
	assert.ElementsMatch(t, []string{"a.jpg", "b.jpg"}, photos)
}

// TestAdjust2116_RejectedWhileShipping：收货前（shipping）不可加价（#2116 修订）。
func TestAdjust2116_RejectedWhileShipping(t *testing.T) {
	f := setupRepairServiceFixture(t)
	customer := testutil.MakeCustomer("", f.customerSub)
	staff := testutil.MakeSiteMember(f.tenantID, f.siteID, f.techID)

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "加价-未收货", "technician_id": f.techID})
	id := svcData(t, resp)["id"].(string)
	svcPost(t, f, customer, "/user/repair-services/"+id+"/select-technician", gin.H{"technician_id": f.techID})
	svcPost(t, f, staff, "/repair-services/"+id+"/quote", gin.H{"quote_repair_cents": 20000, "quote_logistics_cents": 5000})
	svcPost(t, f, customer, "/user/repair-services/"+id+"/accept", nil)
	svcPay(t, f, id, 25000)
	svcPost(t, f, customer, "/user/repair-services/"+id+"/ship", gin.H{"tracking_number": "SF-X"})

	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/adjust", gin.H{"new_quote_cents": 30000, "incurred_cents": 5000})
	assert.Equal(t, float64(40900), resp["code"], "收货前不可加价")
}

// TestListTasks2116_IncludesSubmitterName：工作台瘦身行需要提交人（#2116 修订）。
func TestListTasks2116_IncludesSubmitterName(t *testing.T) {
	f := setupRepairServiceFixture(t)
	customer := testutil.MakeCustomer("", f.customerSub)
	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "名单", "technician_id": f.techID})
	svcData(t, resp)
	svcPost(t, f, customer, "/user/repair-services/"+svcLastID(t, f)+"/select-technician", gin.H{"technician_id": f.techID})

	staff := testutil.MakeSiteMember(f.tenantID, f.siteID, f.techID)
	req := httptest.NewRequest(http.MethodGet, "/repair-services?scope=site", nil)
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req.WithContext(staff.InjectContext(req.Context())))
	require.Equal(t, http.StatusOK, w.Code)
	var out struct {
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.GreaterOrEqual(t, len(out.Data.List), 1)
	assert.NotEmpty(t, out.Data.List[0]["user_name"], "列表行应带提交人姓名")
}

func svcLastID(t *testing.T, f svcFixture) string {
	t.Helper()
	var ids []string
	f.db.Model(&models.RepairRequest{}).Where("type = ?", "service").Order("created_at DESC").Limit(1).Pluck("id", &ids)
	require.NotEmpty(t, ids)
	return ids[0]
}

// TestStart2116_TechnicianAssignedByLocalID（#2118 修订）：technician_id 存本地
// users.id（IAM sub 不同，#2090 形态）时 Start 不得 403——与 ListTasks 双键口径一致。
func TestStart2116_TechnicianAssignedByLocalID(t *testing.T) {
	f := setupRepairServiceFixture(t)
	// 技术员：本地 id ≠ IAM sub（管理员代建例外形态）
	localID := uuid.NewString()
	techSub := uuid.NewString()
	require.NoError(t, f.db.Create(&models.User{
		ID: localID, IAMSub: techSub, TenantID: f.tenantID, OrgID: f.orgID,
		Username: "u-" + localID[:8], Name: "双键师傅", Status: "active",
	}).Error)
	rr := models.RepairRequest{
		ID: uuid.NewString(), TenantID: f.tenantID, UserID: f.customerID, Type: "service",
		Status: models.RepairReqStatusPendingRepair, TechnicianID: &localID,
		SiteID: f.siteID, UserInstrumentID: uuid.NewString(), Description: "双键", Photos: "[]", ReceivePhotos: "[]",
	}
	require.NoError(t, f.db.Create(&rr).Error)

	tech := testutil.TestActor{TenantID: f.tenantID, OrgID: f.siteID, UserID: techSub, Role: "repair_technician"}
	_, resp := svcPost(t, f, tech, "/repair-services/"+rr.ID+"/start", nil)
	assert.Equal(t, 20000, bodyCode2116(t, resp), "本地 id 指派的师傅应可开始维修")

	var got models.RepairRequest
	require.NoError(t, f.db.Where("id = ?", rr.ID).First(&got).Error)
	assert.Equal(t, models.RepairReqStatusRepairing, got.Status)
}

// TestReceive2116_TechnicianAssignedByLocalID：同口径覆盖收货路径 1。
func TestReceive2116_TechnicianAssignedByLocalID(t *testing.T) {
	f := setupRepairServiceFixture(t)
	localID := uuid.NewString()
	techSub := uuid.NewString()
	require.NoError(t, f.db.Create(&models.User{
		ID: localID, IAMSub: techSub, TenantID: f.tenantID, OrgID: f.orgID,
		Username: "u-" + localID[:8], Name: "双键师傅", Status: "active",
	}).Error)
	rr := models.RepairRequest{
		ID: uuid.NewString(), TenantID: f.tenantID, UserID: f.customerID, Type: "service",
		Status: models.RepairReqStatusShipping, TechnicianID: &localID,
		SiteID: f.siteID, UserInstrumentID: uuid.NewString(), Description: "双键收货", Photos: "[]", ReceivePhotos: "[]",
	}
	require.NoError(t, f.db.Create(&rr).Error)

	tech := testutil.TestActor{TenantID: f.tenantID, OrgID: f.siteID, UserID: techSub, Role: "repair_technician"}
	_, resp := receive2116(t, f, tech, rr.ID, []string{"p.jpg"})
	assert.Equal(t, 20000, bodyCode2116(t, resp), "本地 id 指派的师傅应可直收")

	var got models.RepairRequest
	require.NoError(t, f.db.Where("id = ?", rr.ID).First(&got).Error)
	assert.Equal(t, models.RepairReqStatusRepairing, got.Status, "路径 1：师傅直收直接进入维修中")
}

// TestDispatch2122_ShortfallNotification：实际物流费 > 已付 → 生成可缴费补缴通知（#2122）。
func TestDispatch2122_ShortfallNotification(t *testing.T) {
	f := setupRepairServiceFixture(t)
	customer := testutil.MakeCustomer("", f.customerSub)
	staff := testutil.MakeSiteMember(f.tenantID, f.siteID, f.techID)

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "补缴通知", "technician_id": f.techID})
	id := svcData(t, resp)["id"].(string)
	svcPost(t, f, customer, "/user/repair-services/"+id+"/select-technician", gin.H{"technician_id": f.techID})
	svcPost(t, f, staff, "/repair-services/"+id+"/quote", gin.H{"quote_repair_cents": 1000, "quote_logistics_cents": 0})
	svcPost(t, f, customer, "/user/repair-services/"+id+"/accept", nil)
	svcPay(t, f, id, 1000)
	svcPost(t, f, customer, "/user/repair-services/"+id+"/ship", gin.H{"tracking_number": "SF-2122"})
	svcReceive2116(t, f, id)
	svcPost(t, f, staff, "/repair-services/"+id+"/complete", nil)
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/dispatch", gin.H{"tracking_number": "SF-2122B", "logistics_fee_cents": 12000})
	require.Equal(t, float64(20000), resp["code"], resp)

	var n models.Notification
	require.NoError(t, f.db.Where("user_id = ? AND type = ?", f.customerID, "payment_shortfall").First(&n).Error,
		"应生成补缴通知且 user_id=本地 id（消息列表口径，#2124）")
	assert.Equal(t, "payment_shortfall", n.ActionType)
	assert.Contains(t, *n.ActionData, "repair_service", "action_data 应带 order_type=repair_service")
}

// TestSelectTechnician2122_NoSiteMerchantOnly：选师只回填商户租户，不再挂靠网点（#2122）。
func TestSelectTechnician2122_NoSiteMerchantOnly(t *testing.T) {
	f := setupRepairServiceFixture(t)
	customer := testutil.MakeCustomer("", f.customerSub)
	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "直属商户", "technician_id": f.techID})
	id := svcData(t, resp)["id"].(string)
	_, resp = svcPost(t, f, customer, "/user/repair-services/"+id+"/select-technician", gin.H{"technician_id": f.techID})
	require.Equal(t, 20000, bodyCode2116(t, resp), resp)

	var rr models.RepairRequest
	require.NoError(t, f.db.Where("id = ?", id).First(&rr).Error)
	assert.Equal(t, f.tenantID, rr.TenantID, "应回填商户租户")
	assert.Empty(t, rr.SiteID, "不应再挂靠网点（site_id 空）")
}

// TestListTasks2122_SiteStaffInvisible：网点账号 scope=site 看不到维修服务；商户层级可见（#2122）。
func TestListTasks2122_SiteStaffInvisible(t *testing.T) {
	f := setupRepairServiceFixture(t)
	customer := testutil.MakeCustomer("", f.customerSub)
	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "可见性", "technician_id": f.techID})
	require.Equal(t, 20000, bodyCode2116(t, resp), resp)

	list := func(actor testutil.TestActor) int {
		req := httptest.NewRequest(http.MethodGet, "/repair-services?scope=site", nil)
		w := httptest.NewRecorder()
		f.router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))
		var out struct {
			Data struct {
				Total int `json:"total"`
			} `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return out.Data.Total
	}
	// 网点账号（oid=网点组织）→ 0
	siteStaff := testutil.MakeSiteMember(f.tenantID, f.siteID, f.techID)
	assert.Equal(t, 0, list(siteStaff), "网点账号不应看到维修服务")
	// 商户层级（oid=商户组织）→ 可见
	merchant := testutil.TestActor{TenantID: f.tenantID, OrgID: f.orgID, UserID: uuid.NewString(), Role: "STAFF"}
	assert.GreaterOrEqual(t, list(merchant), 1, "商户层级应可见")
}

// TestGet2125_MerchantFallbackByOrgID：订单 tenant_id 存的是商户 org_id 时，
// 详情仍应返回 merchant（寄出面板目标地址/电话来源）——旧实现按 tenant_id 查不到 → 缺失。
func TestGet2125_MerchantFallbackByOrgID(t *testing.T) {
	f := setupRepairServiceFixture(t)
	rr := models.RepairRequest{
		ID: uuid.NewString(), TenantID: f.orgID, // ← 商户 org_id（非 merchants.tenant_id）
		UserID: f.customerSub, Type: "service", Status: models.RepairReqStatusPendingQuote,
		TechnicianID: &f.techID, SiteID: f.siteID, UserInstrumentID: uuid.NewString(),
		Description: "商户解析容错", Photos: "[]", ReceivePhotos: "[]",
	}
	require.NoError(t, f.db.Create(&rr).Error)

	customer := testutil.MakeCustomer("", f.customerSub)
	req := httptest.NewRequest(http.MethodGet, "/user/repair-services/"+rr.ID, nil)
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req.WithContext(customer.InjectContext(req.Context())))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var out struct {
		Data struct {
			Merchant map[string]interface{} `json:"merchant"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.NotNil(t, out.Data.Merchant, "tenant_id 存 org_id 时也应解析到商户")
	assert.Equal(t, "测试商户", out.Data.Merchant["name"])
}
