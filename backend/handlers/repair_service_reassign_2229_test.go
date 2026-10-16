package handlers

// #2229：维修服务单转派（网点管理员·询价阶段·原因必填留痕）
//   - 授权：仅「该单商户的网点管理员」（site_admin 成员的站点归属该商户）
//   - 校验：reason 必填 / 仅 pending_quote / 目标为本商户在册（active 档案）维修师 / 非当前师傅
//   - 留痕：时间线（record_type=reassigned）+ audit_logs（action=reassign_repair_service）
//   - 通知：原师傅 + 新师傅（user_id 落本地 id，#2090）
//   - 可见性：商户维修服务列表对 site_admin 放开（#2129 扩展）+ 顾客信息不再脱敏

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rsFixture2229 组装转派测试所需的额外主体：
//   - tech2：本商户第二位在册维修师（active 档案）
//   - siteAdmin：本商户网点的 site_admin（本地 id ≠ iam_sub，验证双键解析）
//   - plainUser：本商户普通用户（无维修师档案，验证目标校验）
//   - techOther：他商户在册维修师（验证跨商户拒绝）
func rsFixture2229(t *testing.T, f svcFixture) (tech2Sub, siteAdminSub, plainUserID, techOtherSub string) {
	t.Helper()
	tech2Sub = uuid.New().String()
	siteAdminSub = uuid.New().String()
	plainUserID = uuid.New().String()
	techOtherSub = uuid.New().String()

	require.NoError(t, f.db.Create(&models.User{
		ID: tech2Sub, IAMSub: tech2Sub, TenantID: f.tenantID, OrgID: f.orgID,
		Username: "t2-" + tech2Sub[:8], Name: "维修师乙", Status: "active",
	}).Error)
	require.NoError(t, f.db.Create(&models.TechnicianProfile{
		UserID: tech2Sub, TenantID: f.tenantID, Photo: "t2.jpg", Bio: "钢琴维修 5 年",
		Experience: "[]", Status: "active",
	}).Error)
	// 网点管理员：本地 id ≠ iam_sub（生产形态）；site_members 挂**本地 id**
	siteAdminLocalID := uuid.New().String()
	require.NoError(t, f.db.Create(&models.User{
		ID: siteAdminLocalID, IAMSub: siteAdminSub, TenantID: f.tenantID, OrgID: f.orgID,
		Username: "sa-" + siteAdminSub[:8], Name: "网点管理员", Status: "active",
	}).Error)
	require.NoError(t, f.db.Create(&models.SiteMember{
		TenantID: f.tenantID, SiteID: f.siteID, UserID: siteAdminLocalID, Role: "site_admin", Status: "active",
	}).Error)
	require.NoError(t, f.db.Create(&models.User{
		ID: plainUserID, IAMSub: plainUserID, TenantID: f.tenantID, OrgID: f.orgID,
		Username: "pu-" + plainUserID[:8], Name: "普通用户", Status: "active",
	}).Error)
	require.NoError(t, f.db.Create(&models.User{
		ID: techOtherSub, IAMSub: techOtherSub, TenantID: f.otherTenantID, OrgID: f.orgID,
		Username: "to-" + techOtherSub[:8], Name: "他店维修师", Status: "active",
	}).Error)
	require.NoError(t, f.db.Create(&models.TechnicianProfile{
		UserID: techOtherSub, TenantID: f.otherTenantID, Photo: "to.jpg", Bio: "小提琴维修",
		Experience: "[]", Status: "active",
	}).Error)
	return
}

// rsSeedOrder2229 造一张待报价维修服务单（锁定 techID）。
func rsSeedOrder2229(t *testing.T, f svcFixture, status string, customerID, code string) string {
	t.Helper()
	tech := f.techID
	rr := models.RepairRequest{
		ID: uuid.NewString(), TenantID: f.tenantID, UserID: customerID, Type: "service",
		Status: status, TechnicianID: &tech, SiteID: "",
		Description: "转派测试", Photos: "[]", ReceivePhotos: "[]", RepairCode: &code,
	}
	require.NoError(t, f.db.Omit("site_id", "user_instrument_id").Create(&rr).Error)
	return rr.ID
}

func rsReassignActor2229(f svcFixture, sub string) testutil.TestActor {
	return testutil.TestActor{TenantID: f.tenantID, OrgID: f.orgID, UserID: sub, Role: "site_admin"}
}

func TestReassignRepairService_2229_HappyPath(t *testing.T) {
	f := setupRepairServiceFixture(t)
	tech2, siteAdmin, _, _ := rsFixture2229(t, f)
	rrID := rsSeedOrder2229(t, f, models.RepairReqStatusPendingQuote, f.customerID, "R229H1")

	h := NewRepairServiceHandler()
	f.router.POST("/repair-services/:id/reassign", h.Reassign)

	code, resp := svcPost(t, f, rsReassignActor2229(f, siteAdmin), "/repair-services/"+rrID+"/reassign",
		map[string]interface{}{"technician_id": tech2, "reason": "师傅档期已满，与顾客沟通后转派"})
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)

	// 归属变更：technician_id 改为 tech2；主状态不变；site 维度清空
	var got models.RepairRequest
	require.NoError(t, f.db.Where("id = ?", rrID).First(&got).Error)
	require.NotNil(t, got.TechnicianID)
	assert.Equal(t, tech2, *got.TechnicianID)
	assert.Equal(t, models.RepairReqStatusPendingQuote, got.Status)
	assert.Equal(t, "", got.SiteID)

	// 留痕：时间线（reassigned）+ 审计（reassign_repair_service）
	var recs []models.RepairRequestRecord
	require.NoError(t, f.db.Where("repair_request_id = ? AND record_type = ?", rrID, "reassigned").Find(&recs).Error)
	require.Len(t, recs, 1)
	assert.Contains(t, recs[0].Comment, "师傅档期已满，与顾客沟通后转派")

	var audits []models.AuditLog
	require.NoError(t, f.db.Where("action = ? AND resource_id = ?", "reassign_repair_service", rrID).Find(&audits).Error)
	require.Len(t, audits, 1)

	// 通知：原师傅 + 新师傅
	var notifs []models.Notification
	require.NoError(t, f.db.Where("ref_id = ?", rrID).Find(&notifs).Error)
	assert.GreaterOrEqual(t, len(notifs), 1, "应至少通知新师傅")
}

func TestReassignRepairService_2229_Validation(t *testing.T) {
	f := setupRepairServiceFixture(t)
	tech2, siteAdmin, plainUser, techOther := rsFixture2229(t, f)
	h := NewRepairServiceHandler()
	f.router.POST("/repair-services/:id/reassign", h.Reassign)

	// 1) reason 缺失 → 40002
	rr1 := rsSeedOrder2229(t, f, models.RepairReqStatusPendingQuote, f.customerID, "R229V1")
	code, resp := svcPost(t, f, rsReassignActor2229(f, siteAdmin), "/repair-services/"+rr1+"/reassign",
		map[string]interface{}{"technician_id": tech2})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, resp["message"], "reason")

	// 2) 非网点管理员（维修师本人/普通员工）→ 403
	rr2 := rsSeedOrder2229(t, f, models.RepairReqStatusPendingQuote, f.customerID, "R229V2")
	code, _ = svcPost(t, f, testutil.TestActor{TenantID: f.tenantID, OrgID: f.orgID, UserID: f.techID, Role: "repair_technician"},
		"/repair-services/"+rr2+"/reassign", map[string]interface{}{"technician_id": tech2, "reason": "x"})
	assert.Equal(t, http.StatusForbidden, code)

	// 3) 跨商户目标 → 40003
	code, _ = svcPost(t, f, rsReassignActor2229(f, siteAdmin), "/repair-services/"+rr2+"/reassign",
		map[string]interface{}{"technician_id": techOther, "reason": "x"})
	assert.Equal(t, http.StatusBadRequest, code)

	// 4) 目标无维修师档案 → 40003
	code, _ = svcPost(t, f, rsReassignActor2229(f, siteAdmin), "/repair-services/"+rr2+"/reassign",
		map[string]interface{}{"technician_id": plainUser, "reason": "x"})
	assert.Equal(t, http.StatusBadRequest, code)

	// 5) 目标 == 当前师傅 → 40002
	code, _ = svcPost(t, f, rsReassignActor2229(f, siteAdmin), "/repair-services/"+rr2+"/reassign",
		map[string]interface{}{"technician_id": f.techID, "reason": "x"})
	assert.Equal(t, http.StatusBadRequest, code)

	// 6) 非 pending_quote（已报价）→ 409
	rr6 := rsSeedOrder2229(t, f, models.RepairReqStatusQuoted, f.customerID, "R229V6")
	code, _ = svcPost(t, f, rsReassignActor2229(f, siteAdmin), "/repair-services/"+rr6+"/reassign",
		map[string]interface{}{"technician_id": tech2, "reason": "x"})
	assert.Equal(t, http.StatusConflict, code)
}

func TestListMerchantRepairServices_2229_SiteAdminVisibilityAndUnmask(t *testing.T) {
	f := setupRepairServiceFixture(t)
	_, siteAdmin, _, _ := rsFixture2229(t, f)

	// 专用顾客（带姓名/手机，验证不脱敏）
	phone := "13800001111"
	custID := uuid.New().String()
	require.NoError(t, f.db.Create(&models.User{
		ID: custID, IAMSub: custID, TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID: "00000000-0000-0000-0000-000000000000", Username: "c-2229",
		Name: "张三", Phone: phone, Status: "active",
	}).Error)
	rrID := rsSeedOrder2229(t, f, models.RepairReqStatusPendingQuote, custID, "R229L1")

	f.router.GET("/admin/repair-services", ListMerchantRepairServices)

	// 1) 网点管理员（site_admin）→ 可见本商户服务单，顾客信息完整（不脱敏）
	req := httptest.NewRequest(http.MethodGet, "/admin/repair-services?page=1&page_size=20", nil)
	ctx := testutil.TestActor{TenantID: f.tenantID, OrgID: f.siteID, UserID: siteAdmin, Role: "site_admin"}.InjectContext(req.Context())
	ctx = database.SetTenantID(ctx, f.tenantID)
	ctx = database.SetOrgID(ctx, f.orgID)
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req.WithContext(ctx))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), rrID, "网点管理员应能看到本商户维修服务单")
	assert.Contains(t, w.Body.String(), "张三")
	assert.Contains(t, w.Body.String(), phone)

	// 2) 网点普通成员（非 site_admin）→ 仍不可见（授权范围隔离）
	req2 := httptest.NewRequest(http.MethodGet, "/admin/repair-services?page=1&page_size=20", nil)
	memberCtx := testutil.TestActor{TenantID: f.tenantID, OrgID: f.siteID, UserID: f.techID, Role: "repair_technician"}.InjectContext(req2.Context())
	memberCtx = database.SetTenantID(memberCtx, f.tenantID)
	w2 := httptest.NewRecorder()
	f.router.ServeHTTP(w2, req2.WithContext(memberCtx))
	require.Equal(t, http.StatusOK, w2.Code)
	assert.NotContains(t, w2.Body.String(), rrID, "非网点管理员不应看到")

	// 3) 商户管理员（oid=商户组织）→ 可见（#2129 既有行为回归）
	req3 := httptest.NewRequest(http.MethodGet, "/admin/repair-services?page=1&page_size=20", nil)
	adminCtx := testutil.TestActor{TenantID: f.tenantID, OrgID: f.orgID, UserID: uuid.NewString(), Role: "merchant_admin"}.InjectContext(req3.Context())
	adminCtx = database.SetTenantID(adminCtx, f.tenantID)
	adminCtx = database.SetOrgID(adminCtx, f.orgID)
	w3 := httptest.NewRecorder()
	f.router.ServeHTTP(w3, req3.WithContext(adminCtx))
	require.Equal(t, http.StatusOK, w3.Code)
	assert.Contains(t, w3.Body.String(), rrID)
}
