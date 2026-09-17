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

// #1942 阶段2：维修服务（type='service'）全链路 + 状态守卫 + 结算多退少补。

type svcFixture struct {
	tenantID, orgID, siteID string
	customerSub, techID     string
	router                  *gin.Engine
	db                      *gorm.DB
}

func setupRepairServiceFixture(t *testing.T) svcFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID, orgID, _ := testfixtures.NewTenantIDs("1950a1b2c3d4")
	siteID := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{ID: siteID, TenantID: tenantID, OrgID: orgID, Name: "S"}).Error)

	customerSub := uuid.New().String()
	techID := uuid.New().String()
	for _, u := range []string{customerSub, techID} {
		require.NoError(t, db.Create(&models.User{
			ID: u, IAMSub: u, TenantID: tenantID, OrgID: orgID,
			Username: "u-" + u[:8], Name: "用户", Status: "active",
		}).Error)
	}
	require.NoError(t, db.Create(&models.SiteMember{
		TenantID: tenantID, SiteID: siteID, UserID: techID, Role: "repair_technician", Status: "active",
	}).Error)

	h := NewRepairServiceHandler()
	r := gin.New()
	r.POST("/user/repair-services", h.Create)
	r.GET("/user/repair-services/:id", h.Get)
	r.POST("/user/repair-services/:id/select-technician", h.SelectTechnician)
	r.POST("/user/repair-services/:id/accept", h.AcceptQuote)
	r.POST("/user/repair-services/:id/ship", h.Ship)
	r.POST("/user/repair-services/:id/adjust/respond", h.RespondAdjust)
	r.POST("/user/repair-services/:id/review", h.Review)
	r.POST("/repair-services/:id/quote", h.Quote)
	r.POST("/repair-services/:id/adjust", h.Adjust)
	r.POST("/repair-services/:id/done-repair", h.DoneRepair)
	r.POST("/repair-services/:id/dispatch", h.Dispatch)

	return svcFixture{tenantID: tenantID, orgID: orgID, siteID: siteID,
		customerSub: customerSub, techID: techID, router: r, db: db}
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
	assert.Equal(t, models.Cents(25000), got)

	incurred := models.FromYuan(300)
	rr.Status = models.RepairReqStatusAdjustPending
	rr.IncurredRepairCents = &incurred
	got, msg = repairServicePaymentAmount(rr)
	assert.Empty(t, msg)
	assert.Equal(t, models.Cents(30000), got)

	// 加价已支付后，结算修理费以 adjusted 为准
	adjusted := models.FromYuan(300)
	rr.Status = models.RepairReqStatusDoneRepair
	rr.AdjustedQuoteCents = &adjusted
	assert.Equal(t, models.Cents(30000), repairServiceRepairOnly(rr))
}

func TestRepairService_HappyPathSettlementRefund(t *testing.T) {
	f := setupRepairServiceFixture(t)
	customer := testutil.MakeCustomer("", f.customerSub)
	staff := testutil.MakeSiteMember(f.tenantID, f.siteID, f.techID)

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "琴颈修复"})
	d := svcData(t, resp)
	id, _ := d["id"].(string)
	require.NotEmpty(t, id)
	assert.NotEmpty(t, d["repair_code"])

	// 选维修师 → 回填网点/租户
	_, resp = svcPost(t, f, customer, "/user/repair-services/"+id+"/select-technician", gin.H{"technician_id": f.techID})
	require.Equal(t, float64(20000), resp["code"])
	var stored models.RepairRequest
	require.NoError(t, f.db.First(&stored, "id = ?", id).Error)
	assert.Equal(t, f.siteID, stored.SiteID)
	assert.Equal(t, f.tenantID, stored.TenantID)

	// 报价 200 + 50 物流预估
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/quote", gin.H{
		"quote_repair_cents": 20000, "quote_logistics_cents": 5000,
	})
	require.Equal(t, float64(20000), resp["code"])

	// 用户接受报价 → 应付 25000
	_, resp = svcPost(t, f, customer, "/user/repair-services/"+id+"/accept", nil)
	require.Equal(t, float64(20000), resp["code"])
	assert.Equal(t, float64(25000), svcData(t, resp)["payable_cents"])

	// 模拟支付回调（applySideEffects）→ paid
	rec := models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: f.tenantID, UserID: f.customerSub,
		OrderID: &id, OrderType: "repair", Amount: 25000, Type: "payment", Status: "paid",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&rec).Error; err != nil {
			return err
		}
		return applySideEffects(tx, &rec, time.Now())
	}))
	require.NoError(t, f.db.First(&stored, "id = ?", id).Error)
	assert.Equal(t, models.RepairReqStatusPaid, stored.Status)

	// 用户寄出
	_, resp = svcPost(t, f, customer, "/user/repair-services/"+id+"/ship", gin.H{"tracking_number": "SF123"})
	require.Equal(t, float64(20000), resp["code"])

	// 师傅完工
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/done-repair", nil)
	require.Equal(t, float64(20000), resp["code"])

	// 员工发回 + 结算：实付 25000，实际 = 20000 修理 + 3000 末段物流 → 退 2000
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/dispatch", gin.H{
		"tracking_number": "SF999", "logistics_fee_cents": 3000,
	})
	require.Equal(t, float64(20000), resp["code"])
	sd := svcData(t, resp)
	assert.Equal(t, float64(23000), sd["actual_cents"])
	assert.Equal(t, float64(25000), sd["prepaid_cents"])
	assert.Equal(t, float64(2000), sd["refund_cents"])

	require.NoError(t, f.db.First(&stored, "id = ?", id).Error)
	assert.Equal(t, models.RepairReqStatusClosed, stored.Status)
	assert.NotNil(t, stored.ClosedAt)
}

func TestRepairService_AdjustmentFlow(t *testing.T) {
	f := setupRepairServiceFixture(t)
	customer := testutil.MakeCustomer("", f.customerSub)
	staff := testutil.MakeSiteMember(f.tenantID, f.siteID, f.techID)

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "加价流程"})
	id := svcData(t, resp)["id"].(string)
	svcPost(t, f, customer, "/user/repair-services/"+id+"/select-technician", gin.H{"technician_id": f.techID})
	svcPost(t, f, staff, "/repair-services/"+id+"/quote", gin.H{"quote_repair_cents": 10000, "quote_logistics_cents": 0})
	svcPost(t, f, customer, "/user/repair-services/"+id+"/accept", nil)

	// 支付完成后进入 repairing（师傅维修中）
	var stored models.RepairRequest
	require.NoError(t, f.db.Model(&stored).Where("id = ?", id).Update("status", models.RepairReqStatusRepairing).Error)

	// 师傅加价 300
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/adjust", gin.H{"incurred_repair_cents": 30000})
	require.Equal(t, float64(20000), resp["code"])
	require.NoError(t, f.db.First(&stored, "id = ?", id).Error)
	assert.Equal(t, models.RepairReqStatusAdjustPending, stored.Status)

	// 用户接受加价 → 支付差价 → 回调置 repairing 并落定 adjusted
	rec := models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: f.tenantID, UserID: f.customerSub,
		OrderID: &id, OrderType: "repair", Amount: 20000, Type: "payment", Status: "paid",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&rec).Error; err != nil {
			return err
		}
		return applySideEffects(tx, &rec, time.Now())
	}))
	require.NoError(t, f.db.First(&stored, "id = ?", id).Error)
	assert.Equal(t, models.RepairReqStatusRepairing, stored.Status)
	require.NotNil(t, stored.AdjustedQuoteCents)
	assert.Equal(t, models.Cents(30000), *stored.AdjustedQuoteCents)
}

func TestRepairService_StatusGuards(t *testing.T) {
	f := setupRepairServiceFixture(t)
	customer := testutil.MakeCustomer("", f.customerSub)
	staff := testutil.MakeSiteMember(f.tenantID, f.siteID, f.techID)

	_, resp := svcPost(t, f, customer, "/user/repair-services", gin.H{"description": "守卫"})
	id := svcData(t, resp)["id"].(string)

	// 未选师/未报价即完工 → 409
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/done-repair", nil)
	assert.Equal(t, float64(40900), resp["code"])

	// pending_quote 状态重复报价前先正常报价，随后再次报价 → 409
	svcPost(t, f, customer, "/user/repair-services/"+id+"/select-technician", gin.H{"technician_id": f.techID})
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/quote", gin.H{"quote_repair_cents": 10000, "quote_logistics_cents": 0})
	require.Equal(t, float64(20000), resp["code"])
	_, resp = svcPost(t, f, staff, "/repair-services/"+id+"/quote", gin.H{"quote_repair_cents": 10000, "quote_logistics_cents": 0})
	assert.Equal(t, float64(40900), resp["code"])

	// 未结算即评价 → 409
	_, resp = svcPost(t, f, customer, "/user/repair-services/"+id+"/review", gin.H{"rating": 5})
	assert.Equal(t, float64(40900), resp["code"])

	// 非本人访问详情 → 403
	other := testutil.MakeCustomer("", uuid.New().String())
	status, _ := svcPost(t, f, other, "/user/repair-services/"+id+"/accept", nil)
	assert.Equal(t, http.StatusForbidden, status)
}
