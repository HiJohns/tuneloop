package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

func inspectReturnRouter(t *testing.T, tenantID, orgID, staffID string) (*gin.Engine, func(t *testing.T, tenantID, orgID, userID string) (models.Order, *gorm.DB)) {
	t.Helper()
	handler := NewWarehouseHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, staffID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/warehouse/orders/:id/return-inspect", handler.InspectReturn)
	return router, func(t *testing.T, tenantID, orgID, userID string) (models.Order, *gorm.DB) {
		t.Helper()
		db := testfixtures.SetupTestDB(t)
		now := time.Now()
		instrument := models.Instrument{
			TenantID:      tenantID,
			OrgID:         &orgID,
			SN:            "SN-1801-" + now.Format("150405"),
			BaseDailyRate: models.ToCentsPtr(float64Ptr(100)),
			StockStatus:   "rented",
		}
		require.NoError(t, db.Create(&instrument).Error)
		start := "2026-08-01"
		end := "2026-08-30"
		order := models.Order{
			ID:           uuid.New().String(),
			TenantID:     tenantID,
			OrgID:        orgID,
			UserID:       userID,
			InstrumentID: instrument.ID,
			Status:       models.OrderStatusReturning,
			StartDate:    &start,
			EndDate:      &end,
			LeaseTerm:    30,
			Deposit:      models.FromYuan(500),
			CashPaid:     models.FromYuan(500),
		}
		require.NoError(t, db.Create(&order).Error)
		return order, db
	}
}

// TestInspectReturn_AdditionalShippingFeeSaved (#1801):
// 员工填写追加物流费 + 手填逾期费 → 落库 damage_reports（Cents），
// 且员工手填逾期费覆盖自动计算值。
func TestInspectReturn_AdditionalShippingFeeSaved(t *testing.T) {
	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	userID := uuid.New().String()
	staffID := uuid.New().String()

	router, mkOrder := inspectReturnRouter(t, tenantID, orgID, staffID)
	order, db := mkOrder(t, tenantID, orgID, userID)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"instrument_sn":           "SN-1801-" + time.Now().Format("150405"),
		"scan_time":               time.Now().Format(time.RFC3339),
		"notes":                   "验收通过",
		"damage_amount":           0,
		"overdue_fee":             30,
		"additional_shipping_fee": 50,
		"photos":                  []string{"/uploads/media/inspect-test.jpg"},
	})
	req := httptest.NewRequest("PUT", "/api/warehouse/orders/"+order.ID+"/return-inspect", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var report models.DamageReport
	require.NoError(t, db.Where("lease_id = ?", order.ID).Order("created_at desc").First(&report).Error)
	require.Equal(t, models.Cents(5000), report.AdditionalShippingFee, "追加物流费 50 元 = 5000 分")
	require.Equal(t, models.Cents(3000), report.OverdueFee, "员工手填逾期费 30 元 = 3000 分（覆盖自动值）")
	require.Equal(t, "good", report.Condition, "damage_amount=0 → good")
}

// TestInspectReturn_ConditionConflictDamageWins (#1801 M1):
// 显式 condition='good' 与 damage_amount>0 冲突 → 以 damage_amount 为准 →
// damaged → 订单 pending_damage_response（不得静默完成）。
func TestInspectReturn_ConditionConflictDamageWins(t *testing.T) {
	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	userID := uuid.New().String()
	staffID := uuid.New().String()

	router, mkOrder := inspectReturnRouter(t, tenantID, orgID, staffID)
	order, db := mkOrder(t, tenantID, orgID, userID)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"instrument_sn": "SN-1801-" + time.Now().Format("150405"),
		"scan_time":     time.Now().Format(time.RFC3339),
		"condition":     "good", // 旧前端显式传 good，但 damage_amount > 0
		"notes":         "琴面刮痕",
		"damage_amount": 200,
		"photos":        []string{"/uploads/media/inspect-test.jpg"},
	})
	req := httptest.NewRequest("PUT", "/api/warehouse/orders/"+order.ID+"/return-inspect", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, models.OrderStatusPendingDamageResponse, resp.Data.Status,
		"damage_amount>0 必须走 pending_damage_response（静默完成 = 静默扣款）")

	var after models.Order
	require.NoError(t, db.Where("id = ?", order.ID).First(&after).Error)
	require.Equal(t, models.OrderStatusPendingDamageResponse, after.Status)
}

// TestComputeSettlement_IncludesAdditionalShippingFee (#1801):
// damage_reports.additional_shipping_fee 合入 shipping 费用项（应付合计）。
func TestComputeSettlement_IncludesAdditionalShippingFee(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()

	start := "2026-08-01"
	end := "2026-09-05"
	order := models.Order{
		ID:               uuid.New().String(),
		TenantID:         tenantID,
		OrgID:            orgID,
		UserID:           userID,
		InstrumentID:     uuid.New().String(),
		StartDate:        &start,
		EndDate:          &end,
		LeaseTerm:        35,
		Status:           models.OrderStatusCompleted,
		ReturnedAt:       timePtr1743(time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)),
		Deposit:          models.FromYuan(3000),
		ShippingFee:      models.FromYuan(20),
		CashPaid:         models.FromYuan(3400),
		PricingBreakdown: strPtr(`{"base_daily_rent":1000,"rent_days":35,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":1000},{"days_max":35,"discount_percent":20,"daily_rate":1000}],"tier_segments":[{"tier":1,"days":30,"rate":1000,"discount":1,"subtotal":30000},{"tier":2,"days":5,"rate":1000,"discount":0.8,"subtotal":4000}],"total_amount":34000}`),
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: order.InstrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-SETTLE-ADD-SHIP", StockStatus: "available",
	}).Error)

	// 最新 damage_report 含追加物流费 50 元
	reportID := uuid.New().String()
	require.NoError(t, db.Create(&models.DamageReport{
		ID:                    reportID,
		TenantID:              tenantID,
		OrgID:                 orgID,
		LeaseID:               order.ID,
		InstrumentID:          order.InstrumentID,
		UserID:                userID,
		Condition:             "good",
		Status:                "completed",
		AdditionalShippingFee: models.FromYuan(50),
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}).Error)

	result := computeSettlement(order, db)
	// shipping 合计 = order.ShippingFee(20) + additional(50) = 70 元 = 7000 分
	require.Equal(t, int64(7000), result.Breakdown["deposit_deducted_shipping"],
		"追加物流费必须合入 shipping 费用项")
}
