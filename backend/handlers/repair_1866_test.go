package handlers

import (
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
)

// T1: AgreeDamage on instrument with repair_pending → stock_status stays maintenance,
// repair_status remains repair_pending (repair chain must close the loop).
// Path requirement: damage(100) ≤ refund(3500−100−3200=200) → deposit_refunding
// branch is actually taken, so the repair_status guard block IS executed.
func TestAgreeDamage_StockGuard_WithRepairPending(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID, orgID, userID := testfixtures.NewTenantIDs("a1b2c3d4e5f6")
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "t1user", Status: "active", MembershipLevelID: intPtr(1),
	}).Error)

	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "T1-REPAIR-PENDING", StockStatus: "maintenance",
		RepairStatus: "repair_pending",
	}).Error)

	returnedAt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	deliveredAt := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: instID,
		StartDate:    strPtr("2026-08-01"), EndDate: strPtr("2026-08-30"),
		LeaseTerm:        30,
		Status:           models.OrderStatusPendingDamageResponse,
		DeliveredAt:      &deliveredAt,
		ReturnedAt:       &returnedAt,
		Deposit:          models.FromYuan(500),
		CashPaid:         models.FromYuan(3500),
		ShippingFee:      0,
		PricingBreakdown: strPtr(`{"base_daily_rent":10000,"rent_days":30,"total_amount":300000}`),
	}
	require.NoError(t, db.Create(&order).Error)
	outTradeNo := "t1-pay-001"
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &order.ID, OrderType: "rent", OutTradeNo: &outTradeNo,
		Amount: models.FromYuan(3500), Type: "payment", Status: "paid", Method: strPtr("jsapi"),
	}).Error)

	damageID := uuid.New().String()
	require.NoError(t, db.Create(&models.DamageReport{
		ID: damageID, TenantID: tenantID, OrgID: orgID, LeaseID: order.ID,
		InstrumentID: instID, UserID: userID,
		// damage 100 ≤ refund 200 → deposit_refunding (guard path must execute).
		DamageAmount: models.ToCentsPtr(float64Ptr(100)),
		Status:       "pending",
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

	require.Equal(t, http.StatusOK, w.Code, "AgreeDamage should succeed")
	var resp struct {
		Code int `json:"code"`
		Data struct {
			OrderStatus string `json:"order_status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code, "body: %s", w.Body.String())
	// Path proof: deposit_refunding means the guard block was entered (non-trivial test).
	assert.Equal(t, "deposit_refunding", resp.Data.OrderStatus,
		"T1: must take deposit_refunding path for the repair guard to execute")

	// Verify instrument stock_status is NOT changed to available (repair_pending guard)
	var inst models.Instrument
	require.NoError(t, db.Where("id = ?", instID).First(&inst).Error)
	assert.Equal(t, "maintenance", inst.StockStatus,
		"T1: stock_status must stay maintenance when repair_status is repair_pending")
	assert.Equal(t, "repair_pending", inst.RepairStatus,
		"T1: repair_status must remain repair_pending (repair chain closes the loop)")
}

// T2: AgreeDamage on instrument without repair_status → stock_status becomes available.
func TestAgreeDamage_StockGuard_NoRepairStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID, orgID, userID := testfixtures.NewTenantIDs("b2c3d4e5f6a1")
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "t2user", Status: "active", MembershipLevelID: intPtr(1),
	}).Error)

	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "T2-NO-REPAIR", StockStatus: "rented",
		// RepairStatus intentionally empty — no repair挂载
	}).Error)

	returnedAt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	deliveredAt := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: instID,
		StartDate:    strPtr("2026-08-01"), EndDate: strPtr("2026-08-30"),
		LeaseTerm:        30,
		Status:           models.OrderStatusPendingDamageResponse,
		DeliveredAt:      &deliveredAt,
		ReturnedAt:       &returnedAt,
		Deposit:          models.FromYuan(500),
		CashPaid:         models.FromYuan(3500),
		ShippingFee:      0,
		PricingBreakdown: strPtr(`{"base_daily_rent":10000,"rent_days":30,"total_amount":300000}`),
	}
	require.NoError(t, db.Create(&order).Error)
	outTradeNo := "t2-pay-001"
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &order.ID, OrderType: "rent", OutTradeNo: &outTradeNo,
		Amount: models.FromYuan(3500), Type: "payment", Status: "paid", Method: strPtr("jsapi"),
	}).Error)

	// damage=100 ≤ refund(=3500−100−3000=400) → deposit refund path → stock_status update
	damageID := uuid.New().String()
	require.NoError(t, db.Create(&models.DamageReport{
		ID: damageID, TenantID: tenantID, OrgID: orgID, LeaseID: order.ID,
		InstrumentID: instID, UserID: userID,
		DamageAmount: models.ToCentsPtr(float64Ptr(100)),
		Status:       "pending",
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

	require.Equal(t, http.StatusOK, w.Code, "AgreeDamage should succeed")
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code, "body: %s", w.Body.String())

	// Verify instrument stock_status IS changed to available (no repair_status guard)
	var inst models.Instrument
	require.NoError(t, db.Where("id = ?", instID).First(&inst).Error)
	assert.Equal(t, "available", inst.StockStatus,
		"T2: stock_status must become available when no repair_status is set")
}

// T3: ListRecords returns damage object when damage_report exists, null when not.
func TestListRecords_DamageObject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID, orgID, _ := testfixtures.NewTenantIDs("c3d4e5f6a1b2")
	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "T3-LIST-RECORDS", StockStatus: "available",
	}).Error)

	// Case A: No damage report → damage=null
	routerA := gin.New()
	routerA.Use(func(c *gin.Context) {
		c.Next()
	})
	routerA.GET("/api/repair/:id/records", (&RepairHandler{}).ListRecords)

	reqA := httptest.NewRequest("GET", "/api/repair/"+instID+"/records", nil)
	wA := httptest.NewRecorder()
	routerA.ServeHTTP(wA, reqA)

	require.Equal(t, http.StatusOK, wA.Code)
	var respA struct {
		Code int `json:"code"`
		Data struct {
			Damage interface{} `json:"damage"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wA.Body.Bytes(), &respA))
	assert.Equal(t, 20000, respA.Code)
	assert.Nil(t, respA.Data.Damage, "T3a: damage should be null when no damage_report exists")

	// Case B: Damage report exists → damage object with fields
	damageID := uuid.New().String()
	leaseID := uuid.New().String()
	require.NoError(t, db.Create(&models.DamageReport{
		ID: damageID, TenantID: tenantID, OrgID: orgID,
		LeaseID: leaseID, InstrumentID: instID,
		UserID:            uuid.New().String(),
		DamageAmount:      models.ToCentsPtr(float64Ptr(500)),
		DamageDescription: "琴盒损坏",
		Notes:             "外观有划痕",
		Status:            "pending",
	}).Error)

	reqB := httptest.NewRequest("GET", "/api/repair/"+instID+"/records", nil)
	wB := httptest.NewRecorder()
	routerA.ServeHTTP(wB, reqB)

	require.Equal(t, http.StatusOK, wB.Code)
	var respB struct {
		Code int `json:"code"`
		Data struct {
			Damage *struct {
				ID                string  `json:"id"`
				LeaseID           string  `json:"lease_id"`
				DamageDescription string  `json:"damage_description"`
				Notes             string  `json:"notes"`
				DamageAmount      float64 `json:"damage_amount"`
				Status            string  `json:"status"`
			} `json:"damage"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wB.Body.Bytes(), &respB))
	require.NotNil(t, respB.Data.Damage, "T3b: damage object should exist")
	assert.Equal(t, leaseID, respB.Data.Damage.LeaseID, "T3b: lease_id must match created value")
	assert.Equal(t, "琴盒损坏", respB.Data.Damage.DamageDescription)
	assert.Equal(t, "外观有划痕", respB.Data.Damage.Notes)
	assert.Equal(t, "pending", respB.Data.Damage.Status)
	assert.InDelta(t, 50000.0, respB.Data.Damage.DamageAmount, 0.01,
		"T3b: damage_amount in cents (500 yuan × 100 = 50000)")
}

// T4: ListPendingRepairs excludes other tenants' repair_pending instruments.
func TestListPendingRepairs_TenantFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantA, orgA, _ := testfixtures.NewTenantIDs("d4e5f6a1b2c3")
	tenantB, orgB, _ := testfixtures.NewTenantIDs("e5f6a1b2c3d4")

	// Tenant A: one repair_pending instrument
	instA := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instA, TenantID: tenantA, OrgID: &orgA,
		SN: "T4-TENANT-A", StockStatus: "maintenance", RepairStatus: "repair_pending",
	}).Error)

	// Tenant B: one repair_pending instrument (should NOT appear in A's results)
	instB := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instB, TenantID: tenantB, OrgID: &orgB,
		SN: "T4-TENANT-B", StockStatus: "maintenance", RepairStatus: "repair_pending",
	}).Error)

	// Tenant A: one available instrument (should NOT appear — not repair_pending)
	require.NoError(t, db.Create(&models.Instrument{
		ID: uuid.New().String(), TenantID: tenantA, OrgID: &orgA,
		SN: "T4-TENANT-A-AVAIL", StockStatus: "available",
	}).Error)

	// Request as tenant A staff
	staff := testutil.MakeSiteMember(tenantA, orgA, uuid.New().String())
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := staff.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/repair/pending", (&RepairHandler{}).ListPendingRepairs)

	req := httptest.NewRequest("GET", "/api/repair/pending", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				ID       string `json:"id"`
				TenantID string `json:"tenant_id"`
				SN       string `json:"sn"`
			} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code, "body: %s", w.Body.String())

	// Should only contain tenant A's repair_pending instrument
	assert.Len(t, resp.Data.List, 1, "T4: should return exactly 1 instrument for tenant A")
	assert.Equal(t, instA, resp.Data.List[0].ID, "T4: must be tenant A's instrument")
	assert.Equal(t, tenantA, resp.Data.List[0].TenantID, "T4: tenant_id must match")
}
