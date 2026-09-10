package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(b bool) *bool { return &b }

// T1: Upsert rent_to_own — create new row with content + enabled.
func TestPromoOverride_Upsert_RentToOwn_Create(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID, orgID, userID := testfixtures.NewTenantIDs("a1b2c3d4e5f6")
	admin := testutil.MakeSiteAdmin(tenantID, orgID, userID)

	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "T1-PROMO", StockStatus: "available",
	}).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := admin.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/instruments/:id/promo-overrides", UpdateInstrumentPromoOverride)

	body := `{"override_type":"rent_to_own","enabled":true,"content":"自定义文案"}`
	req := httptest.NewRequest("PUT", "/api/instruments/"+instID+"/promo-overrides",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	var resp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)

	// Verify row created
	var row models.InstrumentPromoOverride
	require.NoError(t, db.Where("instrument_id = ? AND override_type = ?", instID, "rent_to_own").First(&row).Error)
	assert.True(t, row.Enabled != nil && *row.Enabled)
	assert.Equal(t, "自定义文案", row.Content)
}

// T2: Upsert rent_to_own — update existing row to enabled=false.
func TestPromoOverride_Upsert_RentToOwn_Disable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID, orgID, userID := testfixtures.NewTenantIDs("b2c3d4e5f6a1")
	admin := testutil.MakeSiteAdmin(tenantID, orgID, userID)

	instID := uuid.New().String()
	// Pre-create with enabled=true
	require.NoError(t, db.Create(&models.InstrumentPromoOverride{
		TenantID: tenantID, InstrumentID: instID, OverrideType: "rent_to_own",
		Enabled: boolPtr(true), Content: "原始文案",
	}).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := admin.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/instruments/:id/promo-overrides", UpdateInstrumentPromoOverride)

	// Send enabled=false — must NOT return 400
	body := `{"override_type":"rent_to_own","enabled":false}`
	req := httptest.NewRequest("PUT", "/api/instruments/"+instID+"/promo-overrides",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "enabled:false must not return 400, body: %s", w.Body.String())

	// Verify row updated
	var row models.InstrumentPromoOverride
	require.NoError(t, db.Where("instrument_id = ? AND override_type = ?", instID, "rent_to_own").First(&row).Error)
	assert.False(t, row.Enabled != nil && *row.Enabled, "enabled must be false after update")
}

// T3: GetPublicInstrumentByID returns rent_to_own config when configured.
func TestGetPublicInstrument_RentToOwn_Configured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID, orgID, _ := testfixtures.NewTenantIDs("c3d4e5f6a1b2")
	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "T3-PUBLIC", StockStatus: "available",
	}).Error)

	// Configure rent_to_own as disabled with custom content
	require.NoError(t, db.Create(&models.InstrumentPromoOverride{
		TenantID: tenantID, InstrumentID: instID, OverrideType: "rent_to_own",
		Enabled: boolPtr(false), Content: "此乐器不出售",
	}).Error)

	router := gin.New()
	router.GET("/api/public/instruments/:id", GetPublicInstrumentByID)

	req := httptest.NewRequest("GET", "/api/public/instruments/"+instID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			RentToOwn *struct {
				Enabled bool   `json:"enabled"`
				Content string `json:"content"`
			} `json:"rent_to_own"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Data.RentToOwn, "rent_to_own must be present")
	assert.False(t, resp.Data.RentToOwn.Enabled, "must reflect configured enabled=false")
	assert.Equal(t, "此乐器不出售", resp.Data.RentToOwn.Content)
}

// T4: GetPublicInstrumentByID returns rent_to_own defaults when unconfigured.
func TestGetPublicInstrument_RentToOwn_Default(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID, orgID, _ := testfixtures.NewTenantIDs("d4e5f6a1b2c3")
	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "T4-DEFAULT", StockStatus: "available",
	}).Error)

	router := gin.New()
	router.GET("/api/public/instruments/:id", GetPublicInstrumentByID)

	req := httptest.NewRequest("GET", "/api/public/instruments/"+instID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			RentToOwn *struct {
				Enabled bool   `json:"enabled"`
				Content string `json:"content"`
			} `json:"rent_to_own"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Data.RentToOwn, "rent_to_own must always be present")
	assert.True(t, resp.Data.RentToOwn.Enabled, "default enabled must be true")
	assert.Empty(t, resp.Data.RentToOwn.Content, "default content must be empty")
}

// T5: Whitelist rejects invalid override_type.
func TestPromoOverride_Whitelist_RejectsInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID, orgID, userID := testfixtures.NewTenantIDs("e5f6a1b2c3d4")
	admin := testutil.MakeSiteAdmin(tenantID, orgID, userID)

	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "T5-WL", StockStatus: "available",
	}).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := admin.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/instruments/:id/promo-overrides", UpdateInstrumentPromoOverride)

	body := `{"override_type":"invalid_type","enabled":true}`
	req := httptest.NewRequest("PUT", "/api/instruments/"+instID+"/promo-overrides",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "invalid type must return 400")
}

// T6: Update with content omitted must preserve the stored custom copy.
// (#1863 audit: PC switch toggle sends enabled-only; content must not be wiped.)
func TestPromoOverride_ContentPreservedWhenOmitted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID, orgID, userID := testfixtures.NewTenantIDs("f6a1b2c3d4e5")
	admin := testutil.MakeSiteAdmin(tenantID, orgID, userID)

	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.InstrumentPromoOverride{
		TenantID: tenantID, InstrumentID: instID, OverrideType: "rent_to_own",
		Enabled: boolPtr(true), Content: "自定义文案",
	}).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := admin.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/instruments/:id/promo-overrides", UpdateInstrumentPromoOverride)

	// Case A: enabled-only update (PC switch toggle) → content preserved.
	body := `{"override_type":"rent_to_own","enabled":false}`
	req := httptest.NewRequest("PUT", "/api/instruments/"+instID+"/promo-overrides",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var row models.InstrumentPromoOverride
	require.NoError(t, db.Where("instrument_id = ? AND override_type = ?", instID, "rent_to_own").First(&row).Error)
	assert.False(t, row.Enabled != nil && *row.Enabled, "enabled must be updated to false")
	assert.Equal(t, "自定义文案", row.Content, "content must be preserved when omitted")

	// Case B: explicit empty string resets to the default copy.
	body2 := `{"override_type":"rent_to_own","content":""}`
	req2 := httptest.NewRequest("PUT", "/api/instruments/"+instID+"/promo-overrides",
		bytes.NewBufferString(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code, "body: %s", w2.Body.String())

	var row2 models.InstrumentPromoOverride
	require.NoError(t, db.Where("instrument_id = ? AND override_type = ?", instID, "rent_to_own").First(&row2).Error)
	assert.Empty(t, row2.Content, "explicit empty string must reset content")
}

// T7: migration 20260910002 widens the override_type CHECK to allow rent_to_own
// and is idempotent (up/down), covering the production-schema fidelity gap.
func TestPromoOverride_CheckConstraintMigration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID, orgID, _ := testfixtures.NewTenantIDs("a6b6c6d6e6f6")
	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "T7-MIG", StockStatus: "available",
	}).Error)

	// Simulate the production constraint created by migration 077 (pre-#1863).
	require.NoError(t, db.Exec(`ALTER TABLE instrument_promo_overrides DROP CONSTRAINT IF EXISTS instrument_promo_overrides_override_type_check`).Error)
	require.NoError(t, db.Exec(`ALTER TABLE instrument_promo_overrides ADD CONSTRAINT instrument_promo_overrides_override_type_check CHECK (override_type IN ('discount','rebate'))`).Error)

	insert := func(typ string) error {
		return db.Exec(`INSERT INTO instrument_promo_overrides (id, tenant_id, instrument_id, override_type, enabled, content) VALUES (gen_random_uuid(), ?, ?, ?, true, '')`, tenantID, instID, typ).Error
	}
	// Pre-migration: rent_to_own is rejected by the check constraint (23514).
	require.Error(t, insert("rent_to_own"), "pre-migration constraint must reject rent_to_own")

	upSQL, err := os.ReadFile("../database/migrations/20260910002_widen_promo_override_type_check.up.sql")
	require.NoError(t, err)
	require.NoError(t, db.Exec(string(upSQL)).Error, "up migration must apply")
	require.NoError(t, insert("rent_to_own"), "post-migration insert must succeed")

	// Idempotent re-run of up.
	require.NoError(t, db.Exec(string(upSQL)).Error, "up migration must be idempotent")

	downSQL, err := os.ReadFile("../database/migrations/20260910002_widen_promo_override_type_check.down.sql")
	require.NoError(t, err)
	require.NoError(t, db.Exec(string(downSQL)).Error, "down migration must apply")

	var count int64
	require.NoError(t, db.Model(&models.InstrumentPromoOverride{}).Where("override_type = ?", "rent_to_own").Count(&count).Error)
	assert.Zero(t, count, "down migration must remove rent_to_own rows")
	require.Error(t, insert("rent_to_own"), "down migration must restore the 2-value constraint")
}
