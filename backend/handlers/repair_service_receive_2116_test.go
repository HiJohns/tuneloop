package handlers

import (
	"encoding/json"
	"testing"

	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

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
