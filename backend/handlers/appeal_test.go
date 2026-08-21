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
	require.NoError(t, db.Model(&models.DamageReport{}).Where("id = ?", damageID).
		Update("damage_amount", models.FromYuan(1000)).Error)
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
	// refund 不变 1200；补缴 = 2000 − 1200 = 800
	require.True(t, resp2.Data.PaymentRequired)
	rf2, rent2, paid2 := computeDamageRefund(db, order, 2000.0) // damage 2000
	t.Logf("second: refund=%v actualRent=%v paidTotal=%v", rf2, rent2, paid2)
	require.InDelta(t, 500.0, resp2.Data.Amount, 0.001, "payDiff = damage − refund = 1000 − 500")
}
