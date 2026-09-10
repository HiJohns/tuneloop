package handlers

import (
	"bytes"
	"encoding/json"
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
