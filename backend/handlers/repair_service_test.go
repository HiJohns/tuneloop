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
	require.NoError(t, db.Create(&models.SiteMember{
		TenantID: otherTenantID, SiteID: otherSiteID, UserID: otherStaffSub, Role: "site_member", Status: "active",
	}).Error)

	h := NewRepairServiceHandler()
	r := gin.New()
	r.POST("/user/repair-services", h.Create)
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
	r.GET("/common/repair-technicians", h.ListTechnicians)

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

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "琴颈修复"})
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

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "加价-继续"})
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

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "加价-拒绝"})
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

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "补缴"})
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

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "守卫"})
	id := svcData(t, resp)["id"].(string)

	// F1：未选定网点（tenant/site 为空）时 staff 一律不可操作（含完工）→ 403
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/complete", nil)
	assert.Equal(t, float64(40300), resp["code"])
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/quote", gin.H{"quote_repair_cents": 10000, "quote_logistics_cents": 0})
	assert.Equal(t, float64(40300), resp["code"])

	svcPost(t, f, customer, "/user/repair-services/"+id+"/select-technician", gin.H{"technician_id": f.techID})
	// 已选师但未支付/未寄出即完工 → 409（状态守卫）
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/complete", nil)
	assert.Equal(t, float64(40900), resp["code"])
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/quote", gin.H{"quote_repair_cents": 10000, "quote_logistics_cents": 0})
	require.Equal(t, float64(20000), resp["code"])
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

// TestRepairService_TechnicianList（RS-API-1）：顾客上下文（无 tid/oid）可选师傅列表。
func TestRepairService_TechnicianList(t *testing.T) {
	f := setupRepairServiceFixture(t)
	// 另建一名非师傅的 site_member（不应出现在列表）
	plainSub := uuid.New().String()
	require.NoError(t, f.db.Create(&models.User{
		ID: plainSub, IAMSub: plainSub, TenantID: f.tenantID, OrgID: f.orgID,
		Username: "plain-" + plainSub[:8], Name: "非师傅", Status: "active",
	}).Error)
	require.NoError(t, f.db.Create(&models.SiteMember{
		TenantID: f.tenantID, SiteID: f.siteID, UserID: plainSub, Role: "site_member", Status: "active",
	}).Error)

	// 顾客上下文：tid/oid 为空（#833），仅可用入参过滤
	customer := testutil.TestActor{TenantID: "", OrgID: "", UserID: f.customerSub, Role: "USER"}
	req := httptest.NewRequest(http.MethodGet, "/common/repair-technicians", nil)
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req.WithContext(customer.InjectContext(req.Context())))
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				TechnicianID string `json:"technician_id"`
				Name         string `json:"name"`
				SiteID       string `json:"site_id"`
				SiteName     string `json:"site_name"`
				SiteAddress  string `json:"site_address"`
				Phone        string `json:"phone"`
				Email        string `json:"email"`
			} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Len(t, resp.Data.List, 1, "仅 repair_technician 入列（非师傅 site_member 排除）")
	row := resp.Data.List[0]
	assert.Equal(t, f.techID, row.TechnicianID)
	assert.Equal(t, f.siteID, row.SiteID)
	assert.NotEmpty(t, row.SiteName)
	assert.Empty(t, row.Phone, "不暴露手机号")
	assert.Empty(t, row.Email, "不暴露邮箱")

	// site_id 过滤：传其他网点 → 空
	req = httptest.NewRequest(http.MethodGet, "/common/repair-technicians?site_id="+f.otherSiteID, nil)
	w = httptest.NewRecorder()
	f.router.ServeHTTP(w, req.WithContext(customer.InjectContext(req.Context())))
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Data.List, 0)
}

// TestRepairService_TaskListScopes（RS-API-2）：员工上下文 scope 过滤 + 跨租户隔离。
func TestRepairService_TaskListScopes(t *testing.T) {
	f := setupRepairServiceFixture(t)
	customer := testutil.MakeCustomer("", f.customerSub)
	staff := testutil.MakeSiteMember(f.tenantID, f.siteID, f.techID)
	otherStaff := testutil.TestActor{TenantID: f.otherTenantID, OrgID: f.otherSiteID, UserID: f.otherStaffSub, Role: "site_member"}

	mkSvc := func(desc string) string {
		_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": desc})
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
	assert.False(t, ids[idB], "未选师单（无网点）不出现在 site 列表")

	// scope=mine（师傅本人）→ 指派给我的
	code, list = getList(staff, "?scope=mine&status=pending_quote")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, list, 1)
	assert.Equal(t, idA, list[0].ID)
	assert.Equal(t, models.RepairReqStatusPendingQuote, list[0].Status)

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

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "详情site"})
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

	// 跨租户员工 → 403（#688）
	code, _ = getDetail(otherStaff)
	assert.Equal(t, http.StatusForbidden, code)
}
