package handlers

// #2315 / #2519：批量设价的显式字段语义。
//
// #2315 原用例锁定「`overrides.overdue_daily_fee` 生效」，但 #2519 Rev.5 明确**废弃**
// `overrides` 入参（D11）：批量设价只认 base_daily_rate / overdue_daily_fee / deposit
// 三个显式字段，其余（含 legacy overrides）→ failed[] reason=no_fields。
//
// 因此本文件保留 #2315 的业务意图（逾期日租金仍可设置），断言从 legacy overrides 改为
// 显式字段，并新增「legacy overrides 被拒绝」的负例（R13）。
//
// Evidence rows: R5（日租金）、R6（逾期）、R7（押金含显式 0）、R8（三字段 + 部分失败）、
// R13（legacy overrides 拒绝）。金额口径：入参/`pricing` JSONB 为元，列为分；
// pricing-v2 返回分（日租/押金断言用分，逾期按 plan 语义断言元）。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type pricingResp2519 struct {
	Code int `json:"code"`
	Data struct {
		Updated int `json:"updated"`
		Failed  []struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		} `json:"failed"`
	} `json:"data"`
}

func pricingRouter2519(tenantID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.TestActor{TenantID: tenantID, OrgID: tenantID, UserID: uuid.NewString(), Role: "ADMIN"}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	}, middleware.ExplicitTenantScope())
	r.PUT("/instruments/batch-pricing", BatchSetInstrumentPricing)
	r.GET("/instruments/:id/pricing-v2", GetInstrumentPricingV2)
	return r
}

func seedPricedInstrument2519(t *testing.T, db *gorm.DB, tenantID, sn string, pricing string, overrides string) string {
	t.Helper()
	rate := models.FromYuan(100)
	id := uuid.NewString()
	org := tenantID
	require.NoError(t, db.Create(&models.Instrument{
		ID: id, TenantID: tenantID, OrgID: &org, SN: sn, StockStatus: "available",
		BaseDailyRate: &rate, Pricing: pricing, PricingOverrides: overrides,
	}).Error)
	return id
}

func putBatchPricing2519(t *testing.T, r *gin.Engine, items []map[string]interface{}) pricingResp2519 {
	t.Helper()
	raw, err := json.Marshal(map[string]interface{}{"items": items})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/instruments/batch-pricing", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out pricingResp2519
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return out
}

// pricingV2_2519 读 pricing-v2（分域字段：base_daily_rate/deposit 为分，overdue 为元）。
func pricingV2_2519(t *testing.T, r *gin.Engine, id string) map[string]float64 {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/instruments/"+id+"/pricing-v2", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	out := map[string]float64{}
	for _, key := range []string{"base_daily_rate", "deposit", "overdue_daily_fee"} {
		if v, ok := resp.Data[key].(float64); ok {
			out[key] = v
		}
	}
	return out
}

func overrideKeys2519(t *testing.T, db *gorm.DB, id string) map[string]interface{} {
	t.Helper()
	var row models.Instrument
	require.NoError(t, db.Where("id = ?", id).First(&row).Error)
	out := map[string]interface{}{}
	if row.PricingOverrides != "" {
		_ = json.Unmarshal([]byte(row.PricingOverrides), &out)
	}
	return out
}

func pricingJSON2519(t *testing.T, db *gorm.DB, id string) map[string]interface{} {
	t.Helper()
	var row models.Instrument
	require.NoError(t, db.Where("id = ?", id).First(&row).Error)
	out := map[string]interface{}{}
	if row.Pricing != "" {
		_ = json.Unmarshal([]byte(row.Pricing), &out)
	}
	return out
}

// R5：显式 base_daily_rate → 列 5000 分 + pricing.daily_rent 50 元 + override 同名键被删。
func TestBatchSetInstrumentPricing_DailyRate_2519(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID := uuid.NewString()
	id := seedPricedInstrument2519(t, db, tenantID, "PRC-2519-DAY",
		`{"daily_rent":100,"overdue_daily_fee":5}`, `{"daily_rent":999,"shipping_fee":20}`)
	r := pricingRouter2519(tenantID)

	resp := putBatchPricing2519(t, r, []map[string]interface{}{{"id": id, "base_daily_rate": 50.0}})
	require.EqualValues(t, 20000, resp.Code)
	require.Equal(t, 1, resp.Data.Updated)

	var row models.Instrument
	require.NoError(t, db.Where("id = ?", id).First(&row).Error)
	pricing := pricingJSON2519(t, db, id)
	overrides := overrideKeys2519(t, db, id)
	v2 := pricingV2_2519(t, r, id)

	_, stillHasDailyRent := overrides["daily_rent"]
	_, keptOtherOverride := overrides["shipping_fee"]
	fmt.Printf("R5 STATUS=%d UPDATED=%d BASE_DAILY_RATE=%d PRICING_DAILY_RENT=%.0f OVERRIDE_DAILY_RENT_REMOVED=%d PRICING_V2_RATE=%.0f OTHER_OVERRIDE_KEPT=%d\n",
		resp.Code, resp.Data.Updated, *row.BaseDailyRate, pricing["daily_rent"],
		flag2519(!stillHasDailyRent), v2["base_daily_rate"], flag2519(keptOtherOverride))
	require.Equal(t, models.FromYuan(50), *row.BaseDailyRate)
	require.EqualValues(t, 50, pricing["daily_rent"])
	require.False(t, stillHasDailyRent, "overrides.daily_rent must be removed")
	require.True(t, keptOtherOverride, "unrelated override keys must be preserved")
	require.EqualValues(t, 5000, v2["base_daily_rate"])
}

// R6：显式 overdue_daily_fee → pricing 元值 + override 同名键被删 + pricing-v2 逾期生效。
func TestBatchSetInstrumentPricing_ExplicitOverdue_2519(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID := uuid.NewString()
	id := seedPricedInstrument2519(t, db, tenantID, "PRC-2519-OVD",
		`{"daily_rent":100}`, `{"overdue_daily_fee":999,"deposit":3}`)
	r := pricingRouter2519(tenantID)

	resp := putBatchPricing2519(t, r, []map[string]interface{}{{"id": id, "overdue_daily_fee": 120.0}})
	require.EqualValues(t, 20000, resp.Code)
	require.Equal(t, 1, resp.Data.Updated)

	pricing := pricingJSON2519(t, db, id)
	overrides := overrideKeys2519(t, db, id)
	v2 := pricingV2_2519(t, r, id)
	_, hasOverdueOverride := overrides["overdue_daily_fee"]
	_, keptDepositOverride := overrides["deposit"]

	fmt.Printf("R6 STATUS=%d UPDATED=%d PRICING_OVERDUE=%.0f OVERRIDE_OVERDUE_REMOVED=%d PRICING_V2_OVERDUE=%.0f PRICING_V2_OVERDUE_CENTS=%.0f OTHER_OVERRIDE_KEPT=%d\n",
		resp.Code, resp.Data.Updated, pricing["overdue_daily_fee"], flag2519(!hasOverdueOverride),
		v2["overdue_daily_fee"]/100, v2["overdue_daily_fee"], flag2519(keptDepositOverride))
	require.EqualValues(t, 120, pricing["overdue_daily_fee"])
	require.False(t, hasOverdueOverride, "overrides.overdue_daily_fee must be removed")
	require.True(t, keptDepositOverride, "unrelated override keys must be preserved")
	require.EqualValues(t, 12000, v2["overdue_daily_fee"], "pricing-v2 returns cents")
}

// R7：押金列 + JSONB 双写；deposit=0 是显式零押金（档位 zero）。
func TestBatchSetInstrumentPricing_Deposit_2519(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID := uuid.NewString()
	paidID := seedPricedInstrument2519(t, db, tenantID, "PRC-2519-DEP", `{"daily_rent":100}`, `{"deposit":3}`)
	zeroID := seedPricedInstrument2519(t, db, tenantID, "PRC-2519-DEP0", `{"daily_rent":100,"deposit":50}`, `{}`)
	r := pricingRouter2519(tenantID)

	resp := putBatchPricing2519(t, r, []map[string]interface{}{
		{"id": paidID, "deposit": 100.0},
		{"id": zeroID, "deposit": 0.0},
	})
	require.EqualValues(t, 20000, resp.Code)
	require.Equal(t, 2, resp.Data.Updated)

	var paid, zero models.Instrument
	require.NoError(t, db.Where("id = ?", paidID).First(&paid).Error)
	require.NoError(t, db.Where("id = ?", zeroID).First(&zero).Error)
	paidPricing := pricingJSON2519(t, db, paidID)
	paidOverrides := overrideKeys2519(t, db, paidID)
	zeroPricing := pricingJSON2519(t, db, zeroID)
	_, hasDepositOverride := paidOverrides["deposit"]

	prev := services.InstrumentPricing{Deposit: 9999}
	tier, _, _, _ := resolveDepositTier(&zero, &prev, "")

	fmt.Printf("R7 STATUS=%d UPDATED=%d DEPOSIT_COL=%d PRICING_DEPOSIT=%.0f OVERRIDE_DEPOSIT_REMOVED=%d ZERO_CASE_COL=%d ZERO_CASE_PRICING=%.0f ZERO_CASE_TIER=%s\n",
		resp.Code, resp.Data.Updated, *paid.Deposit, paidPricing["deposit"], flag2519(!hasDepositOverride),
		*zero.Deposit, zeroPricing["deposit"], tier)
	require.Equal(t, models.FromYuan(100), *paid.Deposit)
	require.EqualValues(t, 100, paidPricing["deposit"])
	require.False(t, hasDepositOverride, "overrides.deposit must be removed")
	require.Equal(t, models.Cents(0), *zero.Deposit)
	require.EqualValues(t, 0, zeroPricing["deposit"])
	require.Equal(t, models.DepositTierZero, tier)
}

// R8：一条三字段全开 + 一条不存在 id → 三项同时生效，失败项显式上报。
func TestBatchSetInstrumentPricing_MultiFieldPartialFail_2519(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID := uuid.NewString()
	id := seedPricedInstrument2519(t, db, tenantID, "PRC-2519-MULTI", `{"daily_rent":100}`, `{"daily_rent":7,"overdue_daily_fee":7,"deposit":7}`)
	r := pricingRouter2519(tenantID)
	missingID := uuid.NewString()

	resp := putBatchPricing2519(t, r, []map[string]interface{}{
		{"id": id, "base_daily_rate": 50.0, "overdue_daily_fee": 120.0, "deposit": 100.0},
		{"id": missingID, "base_daily_rate": 60.0},
	})
	require.EqualValues(t, 20000, resp.Code)

	var row models.Instrument
	require.NoError(t, db.Where("id = ?", id).First(&row).Error)
	pricing := pricingJSON2519(t, db, id)
	overrides := overrideKeys2519(t, db, id)
	failedPresent := len(resp.Data.Failed) == 1 && resp.Data.Failed[0].ID == missingID

	fmt.Printf("R8 STATUS=%d UPDATED=%d FAILED=%d BASE=%d OVERDUE=%.0f DEPOSIT=%d OVERRIDES_CLEARED=%d\n",
		resp.Code, resp.Data.Updated, len(resp.Data.Failed), *row.BaseDailyRate,
		pricing["overdue_daily_fee"], *row.Deposit, flag2519(len(overrides) == 0))
	require.Equal(t, 1, resp.Data.Updated)
	require.Equal(t, 1, len(resp.Data.Failed))
	require.True(t, failedPresent)
	require.Equal(t, models.FromYuan(50), *row.BaseDailyRate)
	require.EqualValues(t, 120, pricing["overdue_daily_fee"])
	require.Equal(t, models.FromYuan(100), *row.Deposit)
	require.Empty(t, overrides, "all three overrides must be cleared")
}

// R13：legacy `overrides` 入参已废弃 —— 只传 overrides → no_fields，定价不变。
func TestBatchSetInstrumentPricing_LegacyOverridesRejected_2519(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID := uuid.NewString()
	id := seedPricedInstrument2519(t, db, tenantID, "PRC-2519-LEGACY", `{"daily_rent":100}`, `{}`)
	r := pricingRouter2519(tenantID)

	resp := putBatchPricing2519(t, r, []map[string]interface{}{
		{"id": id, "overrides": map[string]interface{}{"overdue_daily_fee": 15}},
	})
	require.EqualValues(t, 20000, resp.Code)

	overrides := overrideKeys2519(t, db, id)
	v2 := pricingV2_2519(t, r, id)
	reason := ""
	if len(resp.Data.Failed) == 1 {
		reason = resp.Data.Failed[0].Reason
	}

	fmt.Printf("R13 STATUS=%d UPDATED=%d FAILED=%d FAILED_REASON=%s OVERRIDES_STORED=%d PRICING_V2_OVERDUE=%.0f\n",
		resp.Code, resp.Data.Updated, len(resp.Data.Failed), reason, len(overrides), v2["overdue_daily_fee"])
	assert.Equal(t, 0, resp.Data.Updated)
	require.Equal(t, 1, len(resp.Data.Failed))
	assert.Equal(t, "no_fields", reason)
	assert.Empty(t, overrides, "legacy overrides must not be stored")
	assert.EqualValues(t, 0, v2["overdue_daily_fee"])
}
