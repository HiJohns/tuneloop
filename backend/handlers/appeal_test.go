package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/testutil"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

func setupAppealTables(t *testing.T, db *gorm.DB) error {
	tables := []interface{}{
		&models.DamageReport{},
		&models.Appeal{},
	}
	for _, table := range tables {
		_ = db.Migrator().DropTable(table)
		if err := db.Migrator().CreateTable(table); err != nil {
			return err
		}
	}
	return nil
}

func TestListAppeals(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)
	if err := setupAppealTables(t, db); err != nil {
		t.Fatalf("failed to setup tables: %v", err)
	}

	tenantID := uuid.New().String()
	userID := uuid.New().String()
	damageID := uuid.New().String()
	orgID := uuid.New().String()
	appealReason := "Test appeal"
	zeroUUID := "00000000-0000-0000-0000-000000000000"
	appeal := models.Appeal{
		ID:             uuid.New().String(),
		TenantID:       tenantID,
		OrgID:          orgID,
		SiteID:         zeroUUID, // uuid 列不接受空串（22P02），显式零 UUID
		ObjectID:       zeroUUID,
		AppellantID:    zeroUUID,
		DamageReportID: &damageID,
		UserID:         &userID,
		AppealReason:   &appealReason,
		Status:         "pending",
		SubmittedAt:    time.Now(),
	}
	db.Create(&appeal)

	handler := NewAppealHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/merchant/appeals", handler.ListAppeals)

	req := httptest.NewRequest("GET", "/api/merchant/appeals", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Code int `json:"code"`
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 20000, response.Code)
	assert.Greater(t, len(response.Data.List), 0)
}

// TestAgreeDamage_CustomerNoTenant (#1724): customer JWT 无 tid（tenantID=""）时
// AgreeDamage 必须成功（不再 GetTenantID 过滤报 damage report not found），
// 且补缴/退还金额按 refund 公式（damage − refund），非押金对比。
func TestAgreeDamage_CustomerNoTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID := "00000000-0000-4000-8000-0000000000c1"
	orgID := "00000000-0000-4000-8000-0000000000c2"
	userID := "00000000-0000-4000-8000-0000000000c3"

	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "notenant", Status: "active", MembershipLevelID: intPtr(1),
	}).Error)

	// 订单：租金 3000（30 天×100），押金 500；提前归还 20 天 → actualRent 2000；
	// paidTotal 3000（租金）+500（押金）=3500。
	instrumentID := uuid.New().String()
	returnedAt := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	deliveredAt := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: instrumentID,
		StartDate:    strPtr("2026-08-01"), EndDate: strPtr("2026-08-30"),
		LeaseTerm:     30,
		Status:        models.OrderStatusPendingDamageResponse,
		DeliveredAt:   &deliveredAt,
		ReturnedAt:    &returnedAt,
		Deposit:       models.FromYuan(500),
		CashPaid:      models.FromYuan(3500), // 租金 3000 + 押金 500
		ShippingFee:   0,
		PricingBreakdown: strPtr(`{"base_daily_rent":10000,"rent_days":30,"tier_segments":[{"tier":1,"days":30,"rate":10000,"discount":1,"subtotal":300000}],"total_amount":300000}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: instrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "NO-TENANT", BaseDailyRate: models.ToCentsPtr(float64Ptr(100)), StockStatus: "rented",
	}).Error)
	outTradeNo := "dm-notenant-001"
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &order.ID, OrderType: "rent", OutTradeNo: &outTradeNo,
		Amount: models.FromYuan(3500), Type: "payment", Status: "paid", Method: strPtr("jsapi"),
	}).Error)

	damageID := uuid.New().String()
	require.NoError(t, db.Create(&models.DamageReport{
		ID: damageID, TenantID: tenantID, OrgID: orgID, LeaseID: order.ID,
		InstrumentID: instrumentID, UserID: userID,
		DamageAmount: models.ToCentsPtr(float64Ptr(300)), // 定损 300 元
		Status:       "pending",
	}).Error)

	// customer：无 tenant（tenantID=""）——#688 customer 无组织绑定
	customer := testutil.MakeCustomer("", userID)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := customer.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/user/appeals/:id/agree", (&AppealHandler{}).AgreeDamage)

	req := httptest.NewRequest("POST", "/api/user/appeals/"+damageID+"/agree", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "customer without tenant must not get 404")
	var resp struct {
		Code int `json:"code"`
		Data struct {
			PaymentRequired bool    `json:"payment_required"`
			Amount          float64 `json:"amount"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code, "body: %s", w.Body.String())

	// actualRent = 20 天 × 100 = 2000；refund = paidTotal(3500) − damage(300) − actualRent(2000) − shipping(0) = 1200
	// damage 300 < refund 1200 → 退还路径（非 payment_required）
	require.False(t, resp.Data.PaymentRequired, "damage < refund → refund path, not payment")
	rf1, rent1, paid1 := computeDamageRefund(db, order, 300.0) // damage 300
	t.Logf("first: refund=%v actualRent=%v paidTotal=%v", rf1, rent1, paid1)

	// 补缴场景：damage 1000 > refund(3500−1000−2000=500) → 补缴 1000−500=500
	// #1858: 幂等守卫上线后，已 agreed 的报告不可重复同意——第二次场景前
	// 显式把报告状态重置回 pending（现实中定损金额在同意后不可变更）。
	require.NoError(t, db.Model(&models.DamageReport{}).Where("id = ?", damageID).
		Updates(map[string]interface{}{"damage_amount": models.FromYuan(1000), "status": "pending"}).Error)
	require.NoError(t, db.Model(&models.Order{}).Where("id = ?", order.ID).
		Update("status", models.OrderStatusPendingDamageResponse).Error)

	req2 := httptest.NewRequest("POST", "/api/user/appeals/"+damageID+"/agree", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	var resp2 struct {
		Code int `json:"code"`
		Data struct {
			PaymentRequired bool    `json:"payment_required"`
			Amount          float64 `json:"amount"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp2))
	require.Equal(t, 20000, resp2.Code, "body2: %s", w2.Body.String())
	// #1854：净缺口 = damage + actualRent + shipping − paidTotal
	// damage=1000, actualRent=2000, shipping=0, paidTotal=3500 → shortfall=-500 → 不补缴
	rf2, rent2, paid2 := computeDamageRefund(db, order, 1000.0) // damage 1000
	t.Logf("second: refund=%v actualRent=%v paidTotal=%v", rf2, rent2, paid2)
	shortfall2 := 1000.0 + rent2 + 0 - paid2
	t.Logf("shortfall=%v", shortfall2)
	if shortfall2 > 0 {
		require.True(t, resp2.Data.PaymentRequired)
		require.InDelta(t, shortfall2, resp2.Data.Amount, 0.001, "payDiff = shortfall")
	} else {
		require.False(t, resp2.Data.PaymentRequired, "shortfall<=0 → no payment needed")
	}
}

// TestAgreeDamage_ShortfallPositive (#1854): 16fdfb81 形状正例——补缴净缺口
// = damage + 折后租金 + 物流 − 已付 = 1.00 + 0.36 + 0.01 − 0.72 = 0.65。
// 修复前 payDiff = damage − refund = 1.00（refund clamp 0）→ 多收 0.35。
func TestAgreeDamage_ShortfallPositive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()

	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "shortfall", Status: "active", MembershipLevelID: intPtr(1),
	}).Error)

	// 16fdfb81 形状：实租 1 天（同日归还）、优惠码两段折后租金 0.36、物流 0.01、已付 0.72
	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-SF-" + uuid.New().String()[:6], StockStatus: "rented",
	}).Error)
	delivered := time.Date(2026, 9, 8, 16, 56, 0, 0, time.UTC)
	returned := time.Date(2026, 9, 8, 20, 34, 0, 0, time.UTC) // 同日 → 实租 1 天
	days1 := 1
	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: instID,
		StartDate:    strPtr("2026-09-08"), EndDate: strPtr("2026-10-09"),
		LeaseTerm: 0, Status: models.OrderStatusPendingDamageResponse,
		DeliveredAt: &delivered, ReturnedAt: &returned,
		Deposit: 0, CashPaid: models.FromYuan(0.72), ShippingFee: models.FromYuan(0.01),
		CouponDiscount: models.FromYuan(35.64),
		PricingBreakdown: strPtr(`{"base_daily_rent":3600,"rent_days":2,"deposit":0,"total_amount":7200,
			"pricing_tiers":[{"days_max":30,"daily_rate":3600,"discount_percent":0}],
			"tier_segments":[{"tier":1,"days":2,"rate":3600,"discount":1,"subtotal":7200}]}`),
	}
	require.NoError(t, db.Create(&order).Error)
	// 已付 0.72 = 原单租金 0.36（rent）+ 续租租金 0.36（renewal, days=1）
	// ——renewal days 供段模型反推 contractDays（2−1=1），全 renewal 会导致段模型拒绝
	tnoRent := "sf-rent-" + uuid.New().String()[:8]
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &order.ID, OrderType: "rent", OutTradeNo: &tnoRent,
		Amount: models.FromYuan(0.36), Type: "payment", Status: "paid",
		Method: strPtr("jsapi"),
	}).Error)
	tnoRenew := "sf-renew-" + uuid.New().String()[:8]
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &order.ID, OrderType: "renewal", OutTradeNo: &tnoRenew,
		Amount: models.FromYuan(0.36), Type: "payment", Status: "paid",
		Method: strPtr("jsapi"), Days: &days1,
	}).Error)

	damageID := uuid.New().String()
	damageAmount := models.Cents(100) // 定损 ¥1.00
	require.NoError(t, db.Create(&models.DamageReport{
		ID: damageID, TenantID: tenantID, OrgID: orgID, LeaseID: order.ID,
		InstrumentID: instID, UserID: userID,
		DamageAmount: &damageAmount, Status: "pending",
	}).Error)

	customer := testutil.MakeCustomer("", userID)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := customer.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/user/appeals/:id/agree", (&AppealHandler{}).AgreeDamage)

	req := httptest.NewRequest("POST", "/api/user/appeals/"+damageID+"/agree", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			PaymentRequired bool    `json:"payment_required"`
			Amount          float64 `json:"amount"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code, "body: %s", w.Body.String())
	// #1854：净缺口 0.65（damage 1.00 + 折后租金 0.36 + 物流 0.01 − 已付 0.72）
	require.True(t, resp.Data.PaymentRequired, "shortfall 0.65 > 0 → payment required")
	require.InDelta(t, 0.65, resp.Data.Amount, 0.001,
		"payDiff = 净缺口 0.65（修复前 damage−refund = 1.00）")

	// 支付记录落库：65 分
	var record models.OrderPaymentRecord
	require.NoError(t, db.Where("order_id = ? AND order_type = ? AND status = ?",
		order.ID, "damage", "pending").First(&record).Error)
	require.Equal(t, models.Cents(65), record.Amount, "支付记录 = 65 分")
}

// TestLoadDamagePayment_IdCompat (#1854): 支付页 id 兼容——前端跳转传 order_id
// （存量通知 actionData 仅含 order_id），loadDamagePayment 必须按 lease_id 回退
// 查最新 report；金额口径 = 净缺口 0.65 元 = 65 分。
func TestLoadDamagePayment_IdCompat(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()

	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-IDC-" + uuid.New().String()[:6], StockStatus: "rented",
	}).Error)
	delivered := time.Date(2026, 9, 8, 16, 56, 0, 0, time.UTC)
	returned := time.Date(2026, 9, 8, 20, 34, 0, 0, time.UTC)
	days1 := 1
	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: instID,
		StartDate:    strPtr("2026-09-08"), EndDate: strPtr("2026-10-09"),
		LeaseTerm: 0, Status: models.OrderStatusPendingDamageResponse,
		DeliveredAt: &delivered, ReturnedAt: &returned,
		Deposit: 0, CashPaid: models.FromYuan(0.72), ShippingFee: models.FromYuan(0.01),
		CouponDiscount: models.FromYuan(35.64),
		PricingBreakdown: strPtr(`{"base_daily_rent":3600,"rent_days":2,"deposit":0,"total_amount":7200,
			"pricing_tiers":[{"days_max":30,"daily_rate":3600,"discount_percent":0}],
			"tier_segments":[{"tier":1,"days":2,"rate":3600,"discount":1,"subtotal":7200}]}`),
	}
	require.NoError(t, db.Create(&order).Error)
	// 同 ShortfallPositive：rent 0.36 + renewal 0.36（days=1）→ 段模型 contractDays=1
	tnoRent := "idc-rent-" + uuid.New().String()[:8]
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &order.ID, OrderType: "rent", OutTradeNo: &tnoRent,
		Amount: models.FromYuan(0.36), Type: "payment", Status: "paid",
		Method: strPtr("jsapi"),
	}).Error)
	tnoRenew := "idc-renew-" + uuid.New().String()[:8]
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &order.ID, OrderType: "renewal", OutTradeNo: &tnoRenew,
		Amount: models.FromYuan(0.36), Type: "payment", Status: "paid",
		Method: strPtr("jsapi"), Days: &days1,
	}).Error)

	damageID := uuid.New().String()
	damageAmount := models.Cents(100)
	require.NoError(t, db.Create(&models.DamageReport{
		ID: damageID, TenantID: tenantID, OrgID: orgID, LeaseID: order.ID,
		InstrumentID: instID, UserID: userID,
		DamageAmount: &damageAmount, Status: "agreed",
	}).Error)

	// Case 1: id = damage_report.id（原路径回归）
	respByID := PaymentCalculateResponse{}
	loadDamagePayment(db, damageID, &respByID)
	require.Equal(t, 65.0, respByID.Amount, "按 report.id：净缺口 65 分")
	require.Equal(t, models.Cents(65), respByID.Details["pay_amount"], "details.pay_amount = 65 分")

	// Case 2: id = order_id（lease_id 回退——修复前缺参查询恒败 → Amount=0）
	respByOrder := PaymentCalculateResponse{}
	loadDamagePayment(db, order.ID, &respByOrder)
	require.Equal(t, 65.0, respByOrder.Amount, "按 order_id 回退 lease_id：净缺口 65 分")

	// Case 3: 未知 id → 空响应（不 panic）
	respUnknown := PaymentCalculateResponse{}
	loadDamagePayment(db, uuid.New().String(), &respUnknown)
	require.Zero(t, respUnknown.Amount, "未知 id → Amount 0")
}
