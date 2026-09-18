package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services/wechatpay"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// #1942/#1950 阶段2：维修服务（type='service'）契约 + 加价语义 + 结算三支路 + 归属隔离。

type svcFixture struct {
	tenantID, orgID, siteID string
	otherTenantID           string
	otherSiteID             string
	customerSub, techID     string
	otherStaffSub           string
	router                  *gin.Engine
	db                      *gorm.DB
}

func setupRepairServiceFixture(t *testing.T) svcFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID, orgID, _ := testfixtures.NewTenantIDs("1950a1b2c3d4")
	otherTenantID, _, _ := testfixtures.NewTenantIDs("1950f9e8d7c6")
	siteID := uuid.New().String()
	otherSiteID := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{ID: siteID, TenantID: tenantID, OrgID: orgID, Name: "S"}).Error)
	require.NoError(t, db.Create(&models.Site{ID: otherSiteID, TenantID: otherTenantID, OrgID: orgID, Name: "S2"}).Error)

	customerSub := uuid.New().String()
	techID := uuid.New().String()
	otherStaffSub := uuid.New().String()
	for _, u := range []string{customerSub, techID, otherStaffSub} {
		require.NoError(t, db.Create(&models.User{
			ID: u, IAMSub: u, TenantID: tenantID, OrgID: orgID,
			Username: "u-" + u[:8], Name: "用户", Status: "active",
		}).Error)
	}
	require.NoError(t, db.Create(&models.SiteMember{
		TenantID: tenantID, SiteID: siteID, UserID: techID, Role: "repair_technician", Status: "active",
	}).Error)
	// #1974 T1：商户（师傅直属商户 → 用户寄件地址来源 = 商户地址）
	require.NoError(t, db.Create(&models.Merchant{
		TenantID: tenantID, OrgID: orgID, Name: "测试商户", Address: "北京市朝阳区 XX 路 1 号",
		ContactName: "商户联系人", ContactPhone: "13800000000", Status: "active",
		AdminUID: uuid.New().String(),
	}).Error)
	// #1974 T1：师傅档案（创建维修单需锁定 active 师傅）
	require.NoError(t, db.Create(&models.TechnicianProfile{
		UserID: techID, TenantID: tenantID, Photo: "p.jpg", Bio: "钢琴维修 12 年",
		Experience: `[{"craft":"钢琴","years":12}]`, Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		TenantID: otherTenantID, SiteID: otherSiteID, UserID: otherStaffSub, Role: "site_member", Status: "active",
	}).Error)

	h := NewRepairServiceHandler()
	r := gin.New()
	r.POST("/user/repair-services", h.Create)
	r.GET("/user/repair-services", h.ListMine)
	r.GET("/user/repair-services/:id", h.Get)
	r.POST("/user/repair-services/:id/select-technician", h.SelectTechnician)
	r.POST("/user/repair-services/:id/accept", h.AcceptQuote)
	r.POST("/user/repair-services/:id/ship", h.Ship)
	r.POST("/user/repair-services/:id/adjust/accept", h.AdjustAccept)
	r.POST("/user/repair-services/:id/adjust/decline", h.AdjustDecline)
	r.POST("/user/repair-services/:id/review", h.Review)
	r.POST("/repair-services/:id/quote", h.Quote)
	r.POST("/repair-services/:id/legs", h.AddLegFee)
	r.POST("/repair-services/:id/adjust", h.Adjust)
	r.POST("/repair-services/:id/complete", h.Complete)
	r.POST("/repair-services/:id/dispatch", h.Dispatch)
	r.GET("/repair-services", h.ListTasks)
	tp := NewTechnicianProfileHandler()
	r.GET("/common/repair-technicians", tp.PublicList)
	r.GET("/common/repair-technicians/:id", tp.PublicGet)
	r.GET("/common/repair-technicians/active-session-count", tp.ActiveSessionCount)
	r.POST("/api/pay/prepay", PrepayOrder)

	return svcFixture{tenantID: tenantID, orgID: orgID, siteID: siteID,
		otherTenantID: otherTenantID, otherSiteID: otherSiteID,
		customerSub: customerSub, techID: techID, otherStaffSub: otherStaffSub,
		router: r, db: db}
}

func svcPost(t *testing.T, f svcFixture, actor testutil.TestActor, path string, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

func svcData(t *testing.T, resp map[string]interface{}) map[string]interface{} {
	t.Helper()
	d, ok := resp["data"].(map[string]interface{})
	require.True(t, ok, "response has no data: %v", resp)
	return d
}

// svcPay 模拟微信支付回调（applySideEffects），落一条 paid 支付记录。
func svcPay(t *testing.T, f svcFixture, repairID string, amountCents int64) {
	t.Helper()
	rec := models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: f.tenantID, UserID: f.customerSub,
		OrderID: &repairID, OrderType: "repair", Amount: models.Cents(amountCents),
		Type: "payment", Status: "paid", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&rec).Error; err != nil {
			return err
		}
		return applySideEffects(tx, &rec, time.Now())
	}))
}

func TestRepairService_AmountsUnit(t *testing.T) {
	repair := models.FromYuan(200)   // 20000 分
	logistics := models.FromYuan(50) // 5000 分
	rr := &models.RepairRequest{
		Status:              models.RepairReqStatusPendingPay,
		QuoteRepairCents:    &repair,
		QuoteLogisticsCents: &logistics,
	}
	got, msg := repairServicePaymentAmount(rr)
	assert.Empty(t, msg)
	assert.Equal(t, models.Cents(25000), got, "初付 = 报价修理费 + 物流费预估")

	// 加价：new_quote 30000, incurred 5000 → 补差 = 30000-20000 = 10000（非全额）
	newQuote := models.FromYuan(300)
	incurred := models.FromYuan(50)
	rr.Status = models.RepairReqStatusAdjustPending
	rr.AdjustedQuoteCents = &newQuote
	rr.IncurredRepairCents = &incurred
	got, msg = repairServicePaymentAmount(rr)
	assert.Empty(t, msg)
	assert.Equal(t, models.Cents(10000), got, "补差 = 新总价 − 原报价修理费")

	// 结算基准：已加价 → 新总价；拒绝 → incurred；无加价 → 原报价
	rr.Status = models.RepairReqStatusRepairing
	rr.QuoteStatus = "accepted"
	assert.Equal(t, models.Cents(30000), repairServiceRepairOnly(rr))
	rr.QuoteStatus = "declined"
	assert.Equal(t, models.Cents(5000), repairServiceRepairOnly(rr))
}

func TestRepairService_HappyPathSettlementRefund(t *testing.T) {
	f := setupRepairServiceFixture(t)
	customer := testutil.MakeCustomer("", f.customerSub)
	staff := testutil.MakeSiteMember(f.tenantID, f.siteID, f.techID)

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "琴颈修复", "technician_id": f.techID})
	id := svcData(t, resp)["id"].(string)
	require.NotEmpty(t, id)

	_, resp = svcPost(t, f, customer, "/user/repair-services/"+id+"/select-technician", gin.H{"technician_id": f.techID})
	require.Equal(t, float64(20000), resp["code"])
	var stored models.RepairRequest
	require.NoError(t, f.db.First(&stored, "id = ?", id).Error)
	assert.Equal(t, f.siteID, stored.SiteID)
	assert.Equal(t, f.tenantID, stored.TenantID)

	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/quote", gin.H{
		"quote_repair_cents": 20000, "quote_logistics_cents": 5000,
	})
	require.Equal(t, float64(20000), resp["code"])

	_, resp = svcPost(t, f, customer, "/user/repair-services/"+id+"/accept", nil)
	require.Equal(t, float64(20000), resp["code"])
	assert.Equal(t, float64(25000), svcData(t, resp)["payable_cents"])

	svcPay(t, f, id, 25000)
	require.NoError(t, f.db.First(&stored, "id = ?", id).Error)
	assert.Equal(t, models.RepairReqStatusPaid, stored.Status)

	_, resp = svcPost(t, f, customer, "/user/repair-services/"+id+"/ship", gin.H{"tracking_number": "SF123"})
	require.Equal(t, float64(20000), resp["code"])

	// 契约：完工端点为 /complete
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/complete", nil)
	require.Equal(t, float64(20000), resp["code"])

	// 分段物流费契约字段：logistics_fee_cents
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/legs", gin.H{"leg": 2, "logistics_fee_cents": 1000})
	require.Equal(t, float64(20000), resp["code"])

	// 结算：实付 25000，实际 = 20000 修理 + (1000 + 3000 末段) → 退 1000
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/dispatch", gin.H{
		"tracking_number": "SF999", "logistics_fee_cents": 3000,
	})
	require.Equal(t, float64(20000), resp["code"])
	sd := svcData(t, resp)
	assert.Equal(t, float64(24000), sd["actual_cents"])
	assert.Equal(t, float64(25000), sd["prepaid_cents"])
	assert.Equal(t, float64(1000), sd["refund_cents"])

	require.NoError(t, f.db.First(&stored, "id = ?", id).Error)
	assert.Equal(t, models.RepairReqStatusClosed, stored.Status)
	assert.NotNil(t, stored.ClosedAt)
}

// TestRepairService_AdjustAcceptPaysDifference：用户继续 → 只付差价；结算以新总价为基准。
func TestRepairService_AdjustAcceptPaysDifference(t *testing.T) {
	f := setupRepairServiceFixture(t)
	customer := testutil.MakeCustomer("", f.customerSub)
	staff := testutil.MakeSiteMember(f.tenantID, f.siteID, f.techID)

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "加价-继续", "technician_id": f.techID})
	id := svcData(t, resp)["id"].(string)
	svcPost(t, f, customer, "/user/repair-services/"+id+"/select-technician", gin.H{"technician_id": f.techID})
	svcPost(t, f, staff, "/repair-services/"+id+"/quote", gin.H{"quote_repair_cents": 20000, "quote_logistics_cents": 5000})
	svcPost(t, f, customer, "/user/repair-services/"+id+"/accept", nil)
	svcPay(t, f, id, 25000)
	svcPost(t, f, customer, "/user/repair-services/"+id+"/ship", gin.H{"tracking_number": "SF1"})

	// 师傅加价：新总价 30000、到此为止 5000（双字段契约）
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/adjust", gin.H{
		"new_quote_cents": 30000, "incurred_cents": 5000,
	})
	require.Equal(t, float64(20000), resp["code"])
	assert.Equal(t, float64(10000), svcData(t, resp)["payable_cents"], "应付为差价 10000")

	// 用户继续 → 补差 10000
	_, resp = svcPost(t, f, customer, "/user/repair-services/"+id+"/adjust/accept", nil)
	require.Equal(t, float64(20000), resp["code"])
	assert.Equal(t, float64(10000), svcData(t, resp)["payable_cents"])

	svcPay(t, f, id, 10000)
	var stored models.RepairRequest
	require.NoError(t, f.db.First(&stored, "id = ?", id).Error)
	assert.Equal(t, models.RepairReqStatusRepairing, stored.Status)
	require.NotNil(t, stored.AdjustedQuoteCents)
	assert.Equal(t, models.Cents(30000), *stored.AdjustedQuoteCents, "加价后新总价保留，不被 incurred 覆盖")

	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/complete", nil)
	require.Equal(t, float64(20000), resp["code"])
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/dispatch", gin.H{
		"tracking_number": "SF2", "logistics_fee_cents": 4000,
	})
	require.Equal(t, float64(20000), resp["code"])
	sd := svcData(t, resp)
	assert.Equal(t, float64(34000), sd["actual_cents"], "30000 新总价 + 4000 物流")
	assert.Equal(t, float64(35000), sd["prepaid_cents"], "25000 初付 + 10000 补差")
	assert.Equal(t, float64(1000), sd["refund_cents"])
}

// TestRepairService_AdjustDeclineSettlement：用户拒绝 → 结算基准=incurred，按用户例退 160。
func TestRepairService_AdjustDeclineSettlement(t *testing.T) {
	f := setupRepairServiceFixture(t)
	customer := testutil.MakeCustomer("", f.customerSub)
	staff := testutil.MakeSiteMember(f.tenantID, f.siteID, f.techID)

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "加价-拒绝", "technician_id": f.techID})
	id := svcData(t, resp)["id"].(string)
	svcPost(t, f, customer, "/user/repair-services/"+id+"/select-technician", gin.H{"technician_id": f.techID})
	svcPost(t, f, staff, "/repair-services/"+id+"/quote", gin.H{"quote_repair_cents": 20000, "quote_logistics_cents": 5000})
	svcPost(t, f, customer, "/user/repair-services/"+id+"/accept", nil)
	svcPay(t, f, id, 25000)
	svcPost(t, f, customer, "/user/repair-services/"+id+"/ship", gin.H{"tracking_number": "SF1"})
	svcPost(t, f, staff, "/repair-services/"+id+"/adjust", gin.H{"new_quote_cents": 30000, "incurred_cents": 5000})

	_, resp = svcPost(t, f, customer, "/user/repair-services/"+id+"/adjust/decline", nil)
	require.Equal(t, float64(20000), resp["code"])
	var stored models.RepairRequest
	require.NoError(t, f.db.First(&stored, "id = ?", id).Error)
	assert.Equal(t, models.RepairReqStatusDoneRepair, stored.Status)

	// 用户例：已付 250，到此为止修理 50，实际物流 40 → 退 160
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/dispatch", gin.H{
		"tracking_number": "SF3", "logistics_fee_cents": 4000,
	})
	require.Equal(t, float64(20000), resp["code"])
	sd := svcData(t, resp)
	assert.Equal(t, float64(9000), sd["actual_cents"], "incurred 5000 + 物流 4000")
	assert.Equal(t, float64(25000), sd["prepaid_cents"])
	assert.Equal(t, float64(16000), sd["refund_cents"], "退 160 元（用户例）")
}

// TestRepairService_SettlementShortfall：实际 > 已付 → 生成 pending 补缴记录。
func TestRepairService_SettlementShortfall(t *testing.T) {
	f := setupRepairServiceFixture(t)
	customer := testutil.MakeCustomer("", f.customerSub)
	staff := testutil.MakeSiteMember(f.tenantID, f.siteID, f.techID)

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "补缴", "technician_id": f.techID})
	id := svcData(t, resp)["id"].(string)
	svcPost(t, f, customer, "/user/repair-services/"+id+"/select-technician", gin.H{"technician_id": f.techID})
	svcPost(t, f, staff, "/repair-services/"+id+"/quote", gin.H{"quote_repair_cents": 1000, "quote_logistics_cents": 0})
	svcPost(t, f, customer, "/user/repair-services/"+id+"/accept", nil)
	svcPay(t, f, id, 1000)

	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/complete", nil)
	require.Equal(t, float64(20000), resp["code"])
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/dispatch", gin.H{
		"tracking_number": "SF4", "logistics_fee_cents": 12000,
	})
	require.Equal(t, float64(20000), resp["code"])
	sd := svcData(t, resp)
	assert.Equal(t, float64(13000), sd["actual_cents"])
	assert.Equal(t, float64(1000), sd["prepaid_cents"])
	assert.Equal(t, float64(12000), sd["shortfall_cents"])

	var rec models.OrderPaymentRecord
	require.NoError(t, f.db.Where("order_id = ? AND order_type = ? AND status = ?", id, "repair", "pending").
		First(&rec).Error)
	assert.Equal(t, models.Cents(12000), rec.Amount)
}

func TestRepairService_StatusGuardsAndTenantIsolation(t *testing.T) {
	f := setupRepairServiceFixture(t)
	customer := testutil.MakeCustomer("", f.customerSub)
	staff := testutil.MakeSiteMember(f.tenantID, f.siteID, f.techID)

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "守卫", "technician_id": f.techID})
	id := svcData(t, resp)["id"].(string)

	// #1974 T1：创建即锁定师傅（tenant 已回填）→ 本租户员工可操作；
	// 但未支付/未寄出即完工 → 409（状态守卫）
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/complete", nil)
	assert.Equal(t, float64(40900), resp["code"])
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/quote", gin.H{"quote_repair_cents": 10000, "quote_logistics_cents": 0})
	require.Equal(t, float64(20000), resp["code"], "创建即锁定 → 本租户员工可报价")
	// 重复报价 → 409
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/quote", gin.H{"quote_repair_cents": 10000, "quote_logistics_cents": 0})
	assert.Equal(t, float64(40900), resp["code"])

	// F1：他租户员工操作本单 → 403（跨租户隔离）
	otherStaff := testutil.TestActor{TenantID: f.otherTenantID, OrgID: f.otherSiteID, UserID: f.otherStaffSub, Role: "site_member"}
	status, resp := svcPost(t, f, otherStaff, "/repair-services/"+id+"/complete", nil)
	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, float64(40300), resp["code"])

	// 未结算即评价 → 409
	_, resp = svcPost(t, f, customer, "/user/repair-services/"+id+"/review", gin.H{"rating": 5})
	assert.Equal(t, float64(40900), resp["code"])

	// 非本人响应加价 → 403
	_, resp = svcPost(t, f, customer, "/user/repair-services/"+id+"/adjust/decline", nil)
	assert.Equal(t, float64(40900), resp["code"], "无 pending 加价 → 409")
}

// TestRepairService_TaskListScopes（RS-API-2）：员工上下文 scope 过滤 + 跨租户隔离。
func TestRepairService_TaskListScopes(t *testing.T) {
	f := setupRepairServiceFixture(t)
	customer := testutil.MakeCustomer("", f.customerSub)
	staff := testutil.MakeSiteMember(f.tenantID, f.siteID, f.techID)
	otherStaff := testutil.TestActor{TenantID: f.otherTenantID, OrgID: f.otherSiteID, UserID: f.otherStaffSub, Role: "site_member"}

	mkSvc := func(desc string) string {
		_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": desc, "technician_id": f.techID})
		return svcData(t, resp)["id"].(string)
	}
	idA := mkSvc("A-本网点")
	svcPost(t, f, customer, "/user/repair-services/"+idA+"/select-technician", gin.H{"technician_id": f.techID})
	idB := mkSvc("B-未选师")

	getList := func(actor testutil.TestActor, query string) (int, []models.RepairRequest) {
		req := httptest.NewRequest(http.MethodGet, "/repair-services"+query, nil)
		w := httptest.NewRecorder()
		f.router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))
		var resp struct {
			Code int `json:"code"`
			Data struct {
				List []models.RepairRequest `json:"list"`
			} `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp.Data.List
	}

	// scope=site（JWT oid=site）→ 只见本网点已选师单
	code, list := getList(staff, "?scope=site")
	require.Equal(t, http.StatusOK, code)
	ids := map[string]bool{}
	for _, rr := range list {
		ids[rr.ID] = true
	}
	assert.True(t, ids[idA], "本网点单在列")
	// #1974 T1：服务单去 site 维度 → 本租户的「无 site 单」对网点账号可见
	assert.True(t, ids[idB], "无 site 单（本租户）对网点账号可见")

	// scope=mine（师傅本人）→ 指派给我的
	code, list = getList(staff, "?scope=mine&status=pending_quote")
	require.Equal(t, http.StatusOK, code)
	// #1974 T1：创建即锁定师傅 → 两单均指派给该师傅（pending_quote）
	require.Len(t, list, 2)
	for _, rr := range list {
		assert.Equal(t, models.RepairReqStatusPendingQuote, rr.Status)
		assert.Equal(t, f.techID, *rr.TechnicianID)
	}

	// 跨租户员工 scope=site → 空（JWT oid=otherSite，#688）
	code, list = getList(otherStaff, "?scope=site")
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, list, 0)

	// 非员工角色 → 403
	code, _ = getList(customer, "?scope=site")
	assert.Equal(t, http.StatusForbidden, code)
}

// TestRepairService_DetailSite（RS-API-3）：详情含寄件地址；跨租户员工 403。
func TestRepairService_DetailSite(t *testing.T) {
	f := setupRepairServiceFixture(t)
	customer := testutil.MakeCustomer("", f.customerSub)
	staff := testutil.MakeSiteMember(f.tenantID, f.siteID, f.techID)
	otherStaff := testutil.TestActor{TenantID: f.otherTenantID, OrgID: f.otherSiteID, UserID: f.otherStaffSub, Role: "site_member"}

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "详情site", "technician_id": f.techID})
	id := svcData(t, resp)["id"].(string)
	svcPost(t, f, customer, "/user/repair-services/"+id+"/select-technician", gin.H{"technician_id": f.techID})

	getDetail := func(actor testutil.TestActor) (int, map[string]interface{}) {
		req := httptest.NewRequest(http.MethodGet, "/user/repair-services/"+id, nil)
		w := httptest.NewRecorder()
		f.router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))
		var out map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}

	// 顾客上下文（tid/oid 空）→ 可读 + site 寄件地址
	code, out := getDetail(customer)
	require.Equal(t, http.StatusOK, code)
	data := svcData(t, out)
	site, ok := data["site"].(map[string]interface{})
	require.True(t, ok, "详情应含 site 对象")
	assert.Equal(t, f.siteID, site["id"])
	assert.NotNil(t, site["address"])

	// 本网点员工 → 可读
	code, _ = getDetail(staff)
	assert.Equal(t, http.StatusOK, code)

	// #1974 T1：未选师（无 site）单 → 返回【商户】寄件地址（新设计来源）
	_, resp2 := svcPost(t, f, customer, "/user/repair-services", gin.H{
		"description": "无site单", "technician_id": f.techID,
	})
	id2 := svcData(t, resp2)["id"].(string)
	req2 := httptest.NewRequest(http.MethodGet, "/user/repair-services/"+id2, nil)
	w2 := httptest.NewRecorder()
	f.router.ServeHTTP(w2, req2.WithContext(customer.InjectContext(req2.Context())))
	require.Equal(t, http.StatusOK, w2.Code)
	var out2 map[string]interface{}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &out2))
	d2 := svcData(t, out2)
	merchant, hasMerchant := d2["merchant"].(map[string]interface{})
	require.True(t, hasMerchant, "无 site 单应返回 merchant 地址块")
	assert.Equal(t, "测试商户", merchant["name"])
	assert.Equal(t, "北京市朝阳区 XX 路 1 号", merchant["address"])

	// 跨租户员工 → 403（#688）
	code, _ = getDetail(otherStaff)
	assert.Equal(t, http.StatusForbidden, code)
}

// TestRepairService_Timeline（RS-API-4）：全流程各迁移点均写时间线且按序返回。
func TestRepairService_Timeline(t *testing.T) {
	f := setupRepairServiceFixture(t)
	customer := testutil.MakeCustomer("", f.customerSub)
	staff := testutil.MakeSiteMember(f.tenantID, f.siteID, f.techID)

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "时间线", "technician_id": f.techID})
	id := svcData(t, resp)["id"].(string)
	svcPost(t, f, customer, "/user/repair-services/"+id+"/select-technician", gin.H{"technician_id": f.techID})
	svcPost(t, f, staff, "/repair-services/"+id+"/quote", gin.H{"quote_repair_cents": 20000, "quote_logistics_cents": 5000})
	svcPost(t, f, customer, "/user/repair-services/"+id+"/accept", nil)
	svcPay(t, f, id, 25000)
	svcPost(t, f, customer, "/user/repair-services/"+id+"/ship", gin.H{"tracking_number": "SF-TL"})
	svcPost(t, f, staff, "/repair-services/"+id+"/adjust", gin.H{"new_quote_cents": 30000, "incurred_cents": 5000})
	svcPost(t, f, customer, "/user/repair-services/"+id+"/adjust/accept", nil)
	svcPay(t, f, id, 10000)
	svcPost(t, f, staff, "/repair-services/"+id+"/legs", gin.H{"leg": 1, "logistics_fee_cents": 1000})
	svcPost(t, f, staff, "/repair-services/"+id+"/complete", nil)
	svcPost(t, f, staff, "/repair-services/"+id+"/dispatch", gin.H{"tracking_number": "SF-TL2", "logistics_fee_cents": 2000})
	svcPost(t, f, customer, "/user/repair-services/"+id+"/review", gin.H{"rating": 5, "message": "好评"})

	req := httptest.NewRequest(http.MethodGet, "/user/repair-services/"+id, nil)
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req.WithContext(customer.InjectContext(req.Context())))
	require.Equal(t, http.StatusOK, w.Code)
	var out struct {
		Data struct {
			Timeline []struct {
				RecordType string `json:"record_type"`
			} `json:"timeline"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	wantOrder := []string{"created", "technician_selected", "quoted", "quote_accepted", "paid",
		"shipped", "adjust_requested", "adjust_accepted", "adjust_paid", "leg_fee",
		"repair_completed", "settled", "reviewed"}
	got := make([]string, 0, len(out.Data.Timeline))
	for _, t2 := range out.Data.Timeline {
		got = append(got, t2.RecordType)
	}
	require.Len(t, got, len(wantOrder), "时间线条目数：got=%v", got)
	for i, want := range wantOrder {
		require.Equal(t, want, got[i], "时间线第 %d 项", i+1)
	}
}

// TestRepairService_PaymentsSummary（RS-API-5）：已付/待补缴/退款汇总正确。
func TestRepairService_PaymentsSummary(t *testing.T) {
	f := setupRepairServiceFixture(t)
	customer := testutil.MakeCustomer("", f.customerSub)
	staff := testutil.MakeSiteMember(f.tenantID, f.siteID, f.techID)

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "汇总", "technician_id": f.techID})
	id := svcData(t, resp)["id"].(string)
	svcPost(t, f, customer, "/user/repair-services/"+id+"/select-technician", gin.H{"technician_id": f.techID})
	svcPost(t, f, staff, "/repair-services/"+id+"/quote", gin.H{"quote_repair_cents": 1000, "quote_logistics_cents": 0})
	svcPost(t, f, customer, "/user/repair-services/"+id+"/accept", nil)
	svcPay(t, f, id, 1000)
	svcPost(t, f, staff, "/repair-services/"+id+"/complete", nil)
	// 实际物流 12000 > 已付 1000 → 补缴 11000
	svcPost(t, f, staff, "/repair-services/"+id+"/dispatch", gin.H{"tracking_number": "SF-PS", "logistics_fee_cents": 12000})

	req := httptest.NewRequest(http.MethodGet, "/user/repair-services/"+id, nil)
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req.WithContext(customer.InjectContext(req.Context())))
	var out struct {
		Data struct {
			Payments struct {
				MadeCents             int64 `json:"made_cents"`
				PendingShortfallCents int64 `json:"pending_shortfall_cents"`
				RefundCents           int64 `json:"refund_cents"`
				Records               []struct {
					Kind   string `json:"kind"`
					Status string `json:"status"`
					Amount int64  `json:"amount_cents"`
				} `json:"records"`
			} `json:"payments"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	p := out.Data.Payments
	assert.Equal(t, int64(1000), p.MadeCents)
	assert.Equal(t, int64(12000), p.PendingShortfallCents, "补缴 = actual 13000 − prepaid 1000")
	assert.Equal(t, int64(0), p.RefundCents)
	foundPending := false
	for _, r := range p.Records {
		if r.Kind == "payment" && r.Status == "pending" {
			foundPending = true
		}
	}
	assert.True(t, foundPending, "明细含 pending 补缴记录")
}

// TestRepairService_ListMineStatusFilter（RS-API-6）：用户列表状态过滤。
func TestRepairService_ListMineStatusFilter(t *testing.T) {
	f := setupRepairServiceFixture(t)
	customer := testutil.MakeCustomer("", f.customerSub)
	staff := testutil.MakeSiteMember(f.tenantID, f.siteID, f.techID)

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "过滤A", "technician_id": f.techID})
	idA := svcData(t, resp)["id"].(string)
	svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "过滤B", "technician_id": f.techID})
	svcPost(t, f, customer, "/user/repair-services/"+idA+"/select-technician", gin.H{"technician_id": f.techID})
	svcPost(t, f, staff, "/repair-services/"+idA+"/quote", gin.H{"quote_repair_cents": 1000, "quote_logistics_cents": 0})

	req := httptest.NewRequest(http.MethodGet, "/user/repair-services?status=pending_payment", nil)
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req.WithContext(customer.InjectContext(req.Context())))
	var out struct {
		Data struct {
			List  []models.RepairRequest `json:"list"`
			Total int                    `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Equal(t, 1, out.Data.Total, "只返回 pending_payment 状态的单")
	assert.Equal(t, idA, out.Data.List[0].ID)
}

// TestRepairService_ShortfallPrepay（RS-API-7）：补缴支付——金额取服务端记录额，
// 回调幂等关闭补缴记录（M-08 解除），订单保持 closed。
func TestRepairService_ShortfallPrepay(t *testing.T) {
	f := setupRepairServiceFixture(t)
	customer := testutil.MakeCustomer("", f.customerSub)
	staff := testutil.MakeSiteMember(f.tenantID, f.siteID, f.techID)

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "补缴支付", "technician_id": f.techID})
	id := svcData(t, resp)["id"].(string)
	svcPost(t, f, customer, "/user/repair-services/"+id+"/select-technician", gin.H{"technician_id": f.techID})
	svcPost(t, f, staff, "/repair-services/"+id+"/quote", gin.H{"quote_repair_cents": 1000, "quote_logistics_cents": 0})
	svcPost(t, f, customer, "/user/repair-services/"+id+"/accept", nil)
	svcPay(t, f, id, 1000)
	svcPost(t, f, staff, "/repair-services/"+id+"/complete", nil)
	svcPost(t, f, staff, "/repair-services/"+id+"/dispatch", gin.H{"tracking_number": "SF-SP", "logistics_fee_cents": 12000})

	// prepay（stub 客户端）：客户端传错误金额 0.01 → 服务端应按补缴记录额 11000 收单
	wechatpay.ResetGlobalForTesting()
	wechatpay.SetClientForTesting(stubJSAPIClient{}, &wechatpay.Config{
		AppID: "wx_test", NotifyURL: "http://localhost/notify", RefundNotifyURL: "http://localhost/notify",
	})
	t.Cleanup(func() {
		wechatpay.ResetGlobalForTesting()
		testfixtures.SetupWechatPayMock(t)
	})
	body, _ := json.Marshal(map[string]interface{}{"order_type": "repair", "order_id": id, "amount": 0.01})
	req := httptest.NewRequest(http.MethodPost, "/api/pay/prepay", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req.WithContext(customer.InjectContext(req.Context())))
	require.Equal(t, http.StatusOK, w.Code, "响应：%s", w.Body.String())
	var pre struct {
		Data struct {
			Success bool `json:"success"`
			Data    struct {
				OutTradeNo string `json:"out_trade_no"`
			} `json:"data"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &pre))
	require.True(t, pre.Data.Success, "prepay 应成功：%s", w.Body.String())
	require.NotEmpty(t, pre.Data.Data.OutTradeNo)

	// 服务端收单金额 = 补缴额 12000 分（客户端 0.01 元被忽略）
	var newRec models.OrderPaymentRecord
	require.NoError(t, f.db.Where("out_trade_no = ?", pre.Data.Data.OutTradeNo).First(&newRec).Error)
	assert.Equal(t, models.Cents(12000), newRec.Amount, "收单金额=补缴记录额")

	// 模拟回调：记录置 paid + applySideEffects → 补缴记录关闭、订单保持 closed
	require.NoError(t, f.db.Model(&newRec).Update("status", "paid").Error)
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error {
		return applySideEffects(tx, &newRec, time.Now())
	}))
	var closedShortfall models.OrderPaymentRecord
	require.NoError(t, f.db.Where("order_id = ? AND order_type = ? AND method = ? AND status = ?",
		id, "repair", "shortfall", "closed").First(&closedShortfall).Error, "补缴记录应被幂等关闭")
	assert.Equal(t, models.Cents(12000), closedShortfall.Amount)

	// M-08 解除：不再存在 order_type='repair' status='pending' 的记录
	var pendingCount int64
	require.NoError(t, f.db.Model(&models.OrderPaymentRecord{}).
		Where("order_id = ? AND order_type = ? AND status = ?", id, "repair", "pending").
		Count(&pendingCount).Error)
	assert.Equal(t, int64(0), pendingCount, "M-08 阻塞解除")

	// 订单保持 closed（不复活为 paid）
	var stored models.RepairRequest
	require.NoError(t, f.db.First(&stored, "id = ?", id).Error)
	assert.Equal(t, models.RepairReqStatusClosed, stored.Status)

	// 时间线含补缴支付
	var tlCount int64
	require.NoError(t, f.db.Model(&models.RepairRequestRecord{}).
		Where("repair_request_id = ? AND record_type = ?", id, "shortfall_paid").Count(&tlCount).Error)
	assert.Equal(t, int64(1), tlCount)
}
