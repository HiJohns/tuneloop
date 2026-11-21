package handlers

// #2444 R4: 乐器创建/更新显式 0 押金 → top-level 与 pricing.deposit 双写，
// pricing-v2 读回 0（与 #2437 显式零口径同源）。

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

func setupInstrument2444(t *testing.T) (*gorm.DB, string, string, string, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	if db == nil {
		t.Fatal("test db unavailable")
	}
	testutil.SetDB(t, db)
	tenantID := uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)

	suffix := uuid.NewString()[:8]
	categoryID := uuid.NewString()
	require.NoError(t, db.Exec(`INSERT INTO categories (id, name, tenant_id) VALUES (?, ?, ?)`,
		categoryID, "钢琴-2444-"+suffix, tenantID).Error)
	levelID := uuid.NewString()
	require.NoError(t, db.Exec(`INSERT INTO instrument_levels (id, caption, code, sort_order) VALUES (?, ?, ?, 1)`,
		levelID, "入门-2444-"+suffix, "entry-2444-"+suffix).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, uuid.NewString())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/instruments", CreateInstrument)
	router.PUT("/api/instruments/:id", UpdateInstrument)
	router.GET("/api/instruments/:id/pricing-v2", GetInstrumentPricingV2)
	return db, tenantID, categoryID, levelID, router
}

func pricingV2Deposit2444(t *testing.T, router *gin.Engine, instrumentID string) float64 {
	t.Helper()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/instruments/"+instrumentID+"/pricing-v2", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Data struct {
			Deposit float64 `json:"deposit"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp.Data.Deposit
}

func legacyInstrument2444(t *testing.T, db *gorm.DB, tenantID string, topDeposit *float64, pricing string) string {
	t.Helper()
	id := uuid.NewString()
	orgID := tenantID
	inst := models.Instrument{
		ID:          id,
		TenantID:    tenantID,
		OrgID:       &orgID,
		SN:          "SN-2444-UPD-" + id[:8],
		Pricing:     pricing,
		StockStatus: "available",
		Deposit:     models.ToCentsPtr(topDeposit),
	}
	require.NoError(t, db.Create(&inst).Error)
	return id
}

// R4a: 创建显式 0 → 双写 0，pricing-v2 返回 0。
func TestCreateInstrumentExplicitZeroDeposit_2444(t *testing.T) {
	db, tenantID, categoryID, levelID, router := setupInstrument2444(t)

	body, err := json.Marshal(map[string]interface{}{
		"sn":          "SN-2444-CREATE-ZERO",
		"category_id": categoryID,
		"level_id":    levelID,
		"deposit":     0,
		"pricing":     map[string]interface{}{"daily_rent": 100, "deposit": 500},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/instruments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.NotEmpty(t, created.Data.ID)

	top, pricingDeposit := storedDeposit2444(t, db, created.Data.ID)
	assert.Equal(t, 0.0, top, "created instrument must keep explicit top-level 0")
	assert.Equal(t, 0.0, pricingDeposit, "created instrument must sync pricing.deposit=0")
	assert.Equal(t, 0.0, pricingV2Deposit2444(t, router, created.Data.ID), "pricing-v2 must read back 0")

	var stored models.Instrument
	require.NoError(t, db.First(&stored, "id = ? AND tenant_id = ?", created.Data.ID, tenantID).Error)
	require.NotNil(t, stored.Deposit)
	assert.Equal(t, models.FromYuan(0), *stored.Deposit)
}

// R4b: 更新显式 0 → 双写 0；更新显式 500 → 覆盖定价。
func TestUpdateInstrumentExplicitZeroDeposit_2444(t *testing.T) {
	db, tenantID, _, _, router := setupInstrument2444(t)
	zero := 0.0
	legacyID := legacyInstrument2444(t, db, tenantID, &zero, `{"daily_rent":100,"deposit":500}`)

	update := func(deposit float64) {
		body, err := json.Marshal(map[string]interface{}{"deposit": deposit})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPut, "/api/instruments/"+legacyID, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}

	update(0)
	top, pricingDeposit := storedDeposit2444(t, db, legacyID)
	assert.Equal(t, 0.0, top, "update explicit 0 must write top-level 0")
	assert.Equal(t, 0.0, pricingDeposit, "update explicit 0 must sync pricing.deposit=0")
	assert.Equal(t, 0.0, pricingV2Deposit2444(t, router, legacyID))

	update(500)
	top, pricingDeposit = storedDeposit2444(t, db, legacyID)
	assert.Equal(t, 500.0, top, "update explicit 500 must write top-level 500")
	assert.Equal(t, 500.0, pricingDeposit, "update explicit 500 must sync pricing.deposit=500")
	assert.Equal(t, 50000.0, pricingV2Deposit2444(t, router, legacyID), "pricing-v2 deposit is cents")
}
