package handlers

// #2444 R1–R3: /inventory/rent-setting 读路径与 pricing-v2 同源；批量保存
// 不得重算 deposit，也不得把历史未配置乐器（top=0、pricing.deposit>0）固化。

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

func setupRentSetting2444(t *testing.T) (*gorm.DB, string, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	if db == nil {
		t.Fatal("test db unavailable")
	}
	testutil.SetDB(t, db)
	tenantID := uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := NewInventoryHandler()
	router.GET("/inventory/rent-setting", h.GetRentSetting)
	router.PUT("/inventory/rent-setting/batch", h.BatchUpdateRent)
	return db, tenantID, router
}

func createRentSettingInstrument2444(t *testing.T, db *gorm.DB, tenantID, sn string, topDeposit *float64, pricing string) string {
	t.Helper()
	id := uuid.NewString()
	orgID := tenantID
	inst := models.Instrument{
		ID:          id,
		TenantID:    tenantID,
		OrgID:       &orgID,
		SN:          sn,
		Pricing:     pricing,
		StockStatus: "available",
		Deposit:     models.ToCentsPtr(topDeposit),
	}
	require.NoError(t, db.Create(&inst).Error)
	return id
}

type rentSettingRow2444 struct {
	ID      string  `json:"id"`
	Deposit float64 `json:"deposit"`
}

func getRentSetting2444(t *testing.T, router *gin.Engine) map[string]rentSettingRow2444 {
	t.Helper()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/inventory/rent-setting?page=1&pageSize=50", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []rentSettingRow2444 `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	rows := map[string]rentSettingRow2444{}
	for _, row := range resp.Data.List {
		rows[row.ID] = row
	}
	return rows
}

func putRentSetting2444(t *testing.T, router *gin.Engine, items []map[string]interface{}) {
	t.Helper()
	body, err := json.Marshal(map[string]interface{}{"items": items})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/inventory/rent-setting/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// storedDeposit2444 返回 top-level（元）与 pricing.deposit（元）；top 为 nil 时用 -1 标记。
func storedDeposit2444(t *testing.T, db *gorm.DB, id string) (float64, float64) {
	t.Helper()
	var inst models.Instrument
	require.NoError(t, db.First(&inst, "id = ?", id).Error)
	top := -1.0
	if inst.Deposit != nil {
		top = inst.Deposit.ToYuan()
	}
	var pricing map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(inst.Pricing), &pricing))
	pd, _ := pricing["deposit"].(float64)
	return top, pd
}

// R1: legacy（top=0, pricing=500）→ 500；显式 0 → 0；显式 500 → 500。
func TestRentSettingDepositParity_2444(t *testing.T) {
	db, tenantID, router := setupRentSetting2444(t)

	zero := 0.0
	legacyID := createRentSettingInstrument2444(t, db, tenantID, "SN-2444-LEGACY", &zero, `{"daily_rent":100,"deposit":500}`)
	explicitZeroID := createRentSettingInstrument2444(t, db, tenantID, "SN-2444-ZERO", &zero, `{"daily_rent":100,"deposit":0}`)
	override := 500.0
	explicit500ID := createRentSettingInstrument2444(t, db, tenantID, "SN-2444-500", &override, `{"daily_rent":100,"deposit":100}`)

	rows := getRentSetting2444(t, router)
	require.Contains(t, rows, legacyID)
	require.Contains(t, rows, explicitZeroID)
	require.Contains(t, rows, explicit500ID)
	assert.Equal(t, 500.0, rows[legacyID].Deposit, "legacy top=0 + pricing.deposit=500 must fall back to 500")
	assert.Equal(t, 0.0, rows[explicitZeroID].Deposit, "explicit zero (pricing.deposit=0) must surface as 0")
	assert.Equal(t, 500.0, rows[explicit500ID].Deposit, "explicit top-level deposit must override pricing")
}

// R2: 缺省 deposit 不重算（top 与 pricing 均保持原样）。
func TestBatchUpdateRentOmittedDeposit_2444(t *testing.T) {
	db, tenantID, router := setupRentSetting2444(t)
	zero := 0.0
	legacyID := createRentSettingInstrument2444(t, db, tenantID, "SN-2444-BATCH-OMIT", &zero, `{"daily_rent":100,"deposit":500}`)

	putRentSetting2444(t, router, []map[string]interface{}{
		{"id": legacyID, "daily_rent": 120, "shipping_fee": 0, "overdue_daily_fee": 0},
	})
	top, pricingDeposit := storedDeposit2444(t, db, legacyID)
	assert.Equal(t, 0.0, top, "omitted deposit must not write top-level deposit")
	assert.Equal(t, 500.0, pricingDeposit, "omitted deposit must not recompute pricing.deposit")
	assert.Equal(t, 500.0, getRentSetting2444(t, router)[legacyID].Deposit)
}

// R3: legacy 回传列表显示值（500）→ 不固化，仍保持回落。
func TestBatchUpdateRentLegacyRoundTrip_2444(t *testing.T) {
	db, tenantID, router := setupRentSetting2444(t)
	zero := 0.0
	legacyID := createRentSettingInstrument2444(t, db, tenantID, "SN-2444-BATCH-PIN", &zero, `{"daily_rent":100,"deposit":500}`)

	putRentSetting2444(t, router, []map[string]interface{}{
		{"id": legacyID, "daily_rent": 130, "deposit": 500, "shipping_fee": 0, "overdue_daily_fee": 0},
	})
	top, pricingDeposit := storedDeposit2444(t, db, legacyID)
	assert.Equal(t, 0.0, top, "legacy round-trip must not harden top-level deposit")
	assert.Equal(t, 500.0, pricingDeposit, "legacy round-trip must keep pricing.deposit")
	assert.Equal(t, 500.0, getRentSetting2444(t, router)[legacyID].Deposit, "legacy must keep falling back after save")
}

// D4: 显式 0 → top-level 与 pricing.deposit 双写 0，读回 0。
func TestBatchUpdateRentExplicitZero_2444(t *testing.T) {
	db, tenantID, router := setupRentSetting2444(t)
	zero := 0.0
	explicitZeroID := createRentSettingInstrument2444(t, db, tenantID, "SN-2444-BATCH-ZERO", &zero, `{"daily_rent":100,"deposit":0}`)

	putRentSetting2444(t, router, []map[string]interface{}{
		{"id": explicitZeroID, "daily_rent": 130, "deposit": 0, "shipping_fee": 0, "overdue_daily_fee": 0},
	})
	top, pricingDeposit := storedDeposit2444(t, db, explicitZeroID)
	assert.Equal(t, 0.0, top)
	assert.Equal(t, 0.0, pricingDeposit)
	assert.Equal(t, 0.0, getRentSetting2444(t, router)[explicitZeroID].Deposit)
}
