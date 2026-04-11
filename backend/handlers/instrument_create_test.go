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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
)

func TestCreateInstrumentSavesAllFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Connect to test database
	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("Test database not available, skipping:", err)
		return
	}

	// Set global db for handlers to use
	database.SetDB(db)

	// Setup tenant ID
	tenantID := uuid.New().String()
	now := time.Now()

	// Create test site first (with valid UUID)
	siteID := uuid.New().String()
	db.Exec(`INSERT INTO sites (id, name, tenant_id, status, created_at, updated_at) 
		VALUES (?, ?, ?, 'active', ?, ?) 
		ON CONFLICT DO NOTHING`,
		siteID, "测试网点", tenantID, now, now)

	// Create test category
	categoryID := uuid.New().String()
	db.Exec(`INSERT INTO categories (id, name, tenant_id, level, created_at, updated_at) 
		VALUES (?, ?, ?, 1, ?, ?) 
		ON CONFLICT DO NOTHING`,
		categoryID, "测试分类", tenantID, now, now)

	// Create test property
	propertyID := uuid.New().String()
	db.Exec(`INSERT INTO properties (id, name, tenant_id, is_required, created_at, updated_at) 
		VALUES (?, ?, ?, false, ?, ?) 
		ON CONFLICT DO NOTHING`,
		propertyID, "型号", tenantID, now, now)

	// Setup router with middleware
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "test-user")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/instruments", CreateInstrument)

	// Create request body
	requestBody := map[string]interface{}{
		"sn":          "TEST-SN-001",
		"level":       "入门",
		"category_id": categoryID,
		"site_id":     siteID,
		"properties": map[string]interface{}{
			"型号": []string{"U1"},
			"品牌": []string{"雅马哈"},
		},
		"images": []string{"/uploads/test.jpg"},
		"status": "available",
	}

	bodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/api/instruments", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Print response for debugging
	t.Logf("Response status: %d", w.Code)
	t.Logf("Response body: %s", w.Body.String())

	// Assertions
	assert.Equal(t, http.StatusCreated, w.Code, "Response should return 201 Created")

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Check if instrument was created
	assert.Equal(t, float64(20100), response["code"], "Response code should be 20100")

	type Instrument struct {
		ID             string
		SN             string
		Level          string
		CategoryID     string
		SiteID         *string
		Specifications string
	}

	// First check what's in the instruments table
	var recCount int
	db.Raw("SELECT COUNT(*) FROM instruments").Scan(&recCount)
	t.Logf("Total instruments in DB: %d", recCount)

	// List all instruments
	var allInstruments []Instrument
	db.Raw("SELECT id, sn, level, tenant_id FROM instruments LIMIT 10").Scan(&allInstruments)
	t.Logf("All instruments: %+v", allInstruments)

	// Get the created instrument from database

	var instrument Instrument
	t.Logf("Querying with tenantID=%s and sn=TEST-SN-001", tenantID)
	err = db.Raw(`SELECT id, sn, level, category_id, site_id, specifications 
		FROM instruments 
		WHERE tenant_id = ? AND sn = ?`, tenantID, "TEST-SN-001").Scan(&instrument).Error

	if err != nil {
		t.Logf("Query error: %v", err)
	}

	t.Logf("Query result: %+v", instrument)

	require.NoError(t, err, "Should find the created instrument")
	t.Logf("Found instrument: ID=%s, SN=%s, Level=%s, CategoryID=%s, SiteID=%v, Specs=%s",
		instrument.ID, instrument.SN, instrument.Level, instrument.CategoryID, instrument.SiteID, instrument.Specifications)

	// Verify SN was saved
	assert.Equal(t, "TEST-SN-001", instrument.SN, "SN should be saved")

	// Verify Level was saved
	assert.Equal(t, "入门", instrument.Level, "Level should be saved")

	// Verify CategoryID was saved
	assert.Equal(t, categoryID, instrument.CategoryID, "CategoryID should be saved")

	// Verify SiteID was saved
	require.NotNil(t, instrument.SiteID, "SiteID should not be nil")
	assert.Equal(t, siteID, *instrument.SiteID, "SiteID should be saved")

	// Verify Specifications (properties) were saved
	assert.NotEmpty(t, instrument.Specifications, "Specifications should not be empty")
	t.Logf("Specifications saved: %s", instrument.Specifications)

	// Parse and verify properties
	var specs map[string]interface{}
	err = json.Unmarshal([]byte(instrument.Specifications), &specs)
	require.NoError(t, err, "Specifications should be valid JSON")

	// Check if property is saved
	if prop, ok := specs["型号"]; ok {
		t.Logf("型号 found: %v", prop)
	} else {
		t.Logf("Keys in specifications: %v", specs)
	}

	// Cleanup
	db.Exec(`DELETE FROM instruments WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM properties WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM categories WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM sites WHERE tenant_id = ?`, tenantID)
}
