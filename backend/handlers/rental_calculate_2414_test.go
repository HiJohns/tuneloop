package handlers

// #2414：遗留 POST /api/user/rental/calculate 押金口径对齐（#2406 规则）。
//   - instruments.deposit > 0  → 显式配置，响应押金 = 该值（分）
//   - instruments.deposit = 0  → 未配置，取定价押金（分；pricing-v2 同值）
//   - 定价押金 = 0             → 真零押金
// 入口级测试：走真实 handler + DB，断言 JSON 响应 data.deposit。

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRentalCalculate2414(t *testing.T, depositYuan float64, pricingJSON string) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)

	tenantID := uuid.New().String()
	testfixtures.SeedTenantFK(t, db, tenantID)

	// 系统默认定价模板：阶梯租金 + custom 押金（押金由 instruments.pricing 提供）。
	// idx_pricing_templates_system_default 是唯一索引 → 已存在则更新，避免 23505。
	cfg := `{"tiers":[{"days_max":30,"discount_percent":0},{"days_max":365,"discount_percent":20}],"deposit_mode":"custom"}`
	var existing models.PricingTemplate
	if err := db.Where("is_system_default = ?", true).First(&existing).Error; err == nil {
		require.NoError(t, db.Model(&models.PricingTemplate{}).
			Where("id = ?", existing.ID).
			Updates(map[string]interface{}{"config_schema": cfg, "is_active": true}).Error)
	} else {
		require.NoError(t, db.Create(&models.PricingTemplate{
			ID:              uuid.New().String(),
			Code:            "default-2414-" + uuid.New().String()[:8],
			Name:            "Default2414",
			IsSystemDefault: true,
			IsActive:        true,
			ConfigSchema:    cfg,
		}).Error)
	}

	instID := uuid.New().String()
	deposit := models.FromYuan(depositYuan)
	baseRate := models.FromYuan(100)
	require.NoError(t, db.Create(&models.Instrument{
		ID:            instID,
		TenantID:      tenantID,
		SN:            "RC2414-" + uuid.New().String()[:8],
		BaseDailyRate: &baseRate,
		Deposit:       &deposit,
		Pricing:       pricingJSON,
		StockStatus:   "available",
	}).Error)

	userID := uuid.New().String()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.TestActor{TenantID: tenantID, UserID: userID, Role: "USER"}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	h := NewUserRentalHandler()
	r.POST("/rental/calculate", h.CalculateRental)
	return r, instID
}

func postRentalCalculate2414(t *testing.T, r *gin.Engine, instID string, days int) (int, map[string]interface{}) {
	t.Helper()
	body, err := json.Marshal(map[string]interface{}{"instrument_id": instID, "days": days})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/rental/calculate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "body=%s", w.Body.String())
	return w.Code, resp
}

// R1：instruments.deposit=0（未配置）+ 定价押金 ¥0.01 → 响应 deposit=1 分（不是 0）。
func TestCalculateRentalDeposit_2406Rule_2414(t *testing.T) {
	r, instID := setupRentalCalculate2414(t, 0, `{"deposit":0.01}`)

	code, resp := postRentalCalculate2414(t, r, instID, 30)
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	data, ok := resp["data"].(map[string]interface{})
	require.True(t, ok, "data must be an object, got %T", resp["data"])
	assert.Equal(t, float64(1), data["deposit"], "pricing deposit 0.01 元 must surface as 1 cent")
	assert.NotZero(t, data["deposit"], "0 would mean the legacy endpoint still reports 零押金")
}

// R2：instruments.deposit=500 元 → 响应 deposit=50000 分（覆盖定价）。
func TestCalculateRentalDepositInstrumentOverride_2414(t *testing.T) {
	r, instID := setupRentalCalculate2414(t, 500, `{"deposit":0.01}`)

	code, resp := postRentalCalculate2414(t, r, instID, 30)
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, float64(50000), data["deposit"], "500 元 must surface as 50000 cents")
}

// R3：乐器未配置 + 定价押金 0 → 真零押金（唯一合法零押金场景）。
func TestCalculateRentalDepositTrueZero_2414(t *testing.T) {
	r, instID := setupRentalCalculate2414(t, 0, `{"deposit":0}`)

	code, resp := postRentalCalculate2414(t, r, instID, 30)
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, float64(0), data["deposit"], "zero pricing deposit stays zero")
}

// R5：响应单位一致性（D4）——金额字段一律分；两条不变量成立。
func TestCalculateRentalUnitConsistency_2414(t *testing.T) {
	r, instID := setupRentalCalculate2414(t, 0, `{"deposit":0.01}`)

	code, resp := postRentalCalculate2414(t, r, instID, 30)
	require.Equal(t, http.StatusOK, code, "resp=%v", resp)
	data := resp["data"].(map[string]interface{})

	totalRent := data["total_rent"].(float64)
	deposit := data["deposit"].(float64)
	shipping := data["shipping_fee"].(float64)
	cash := data["cash_to_pay"].(float64)
	memberDiscount := data["member_discount_amount"].(float64)

	for name, v := range map[string]float64{
		"total_rent": totalRent, "deposit": deposit, "shipping_fee": shipping,
		"cash_to_pay": cash, "member_discount_amount": memberDiscount,
	} {
		assert.Equal(t, math.Trunc(v), v, "%s must be integer cents, got %v", name, v)
	}
	assert.Equal(t, cash, totalRent+deposit+shipping, "cash_to_pay == total_rent + deposit + shipping_fee")

	tiers := data["tiers"].([]interface{})
	sumSubtotal := 0.0
	for _, it := range tiers {
		row := it.(map[string]interface{})
		sumSubtotal += row["subtotal"].(float64)
		assert.Equal(t, math.Trunc(row["subtotal"].(float64)), row["subtotal"].(float64), "tier subtotal must be cents")
	}
	assert.Equal(t, totalRent, sumSubtotal, "sum(tiers[].subtotal) == total_rent")
}
