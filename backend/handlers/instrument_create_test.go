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
	"tuneloop-backend/models"
)

func TestCreateInstrumentSavesAllFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("Test database not available, skipping:", err)
		return
	}
	database.SetDB(db)

	// Clean all tables
	db.Exec(`DELETE FROM instrument_properties`)
	db.Exec(`DELETE FROM instruments`)
	db.Exec(`DELETE FROM instrument_levels`)

	tenantID := uuid.New().String()
	now := time.Now()

	// Create test data
	siteID := uuid.New().String()
	db.Exec(`INSERT INTO sites (id, name, tenant_id, status, created_at, updated_at) 
		VALUES (?, ?, ?, 'active', ?, ?)`,
		siteID, "测试网点", tenantID, now, now)

	categoryID := uuid.New().String()
	db.Exec(`INSERT INTO categories (id, name, tenant_id, created_at) 
		VALUES (?, ?, ?, ?)`,
		categoryID, "测试分类", tenantID, now)

	propertyID := uuid.New().String()
	db.Exec(`INSERT INTO properties (id, name, tenant_id, property_type, is_required, created_at) 
		VALUES (?, ?, ?, 'text', false, ?)`,
		propertyID, "型号", tenantID, now)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "test-user")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/instruments", CreateInstrument)

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

	bodyBytes, _ := json.Marshal(requestBody)
	req := httptest.NewRequest("POST", "/api/instruments", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	var instrument struct {
		ID             string
		SN             string
		Level          string
		CategoryID     string
		SiteID         *string
		Specifications string
	}

	err = db.Raw(`SELECT id, sn, level, category_id, site_id, specifications 
		FROM instruments WHERE tenant_id = ? AND sn = ?`, tenantID, "TEST-SN-001").Scan(&instrument).Error
	require.NoError(t, err)

	assert.Equal(t, "TEST-SN-001", instrument.SN)
	assert.Equal(t, "入门", instrument.Level)
	assert.Equal(t, categoryID, instrument.CategoryID)
	require.NotNil(t, instrument.SiteID)
	assert.Equal(t, siteID, *instrument.SiteID)
	assert.NotEmpty(t, instrument.Specifications)

	// Cleanup
	db.Exec(`DELETE FROM instruments WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM properties WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM categories WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM sites WHERE tenant_id = ?`, tenantID)
}

func TestCreateInstrumentWithLevelID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("Test database not available, skipping:", err)
		return
	}
	database.SetDB(db)

	// Clean instrument_levels
	db.Exec(`DELETE FROM instrument_levels`)

	tenantID := uuid.New().String()
	now := time.Now()

	// Create test level
	levelID := uuid.New()
	db.Exec(`INSERT INTO instrument_levels (id, caption, code, sort_order) 
		VALUES (?, '专业', 'professional', 2)`, levelID)

	// Create test category
	categoryID := uuid.New().String()
	db.Exec(`INSERT INTO categories (id, name, tenant_id, created_at) 
		VALUES (?, ?, ?, ?)`,
		categoryID, "测试分类", tenantID, now)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "test-user")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/instruments", CreateInstrument)

	requestBody := map[string]interface{}{
		"sn":          "TEST-SN-LEVELID",
		"level":       "professional", // Required field
		"level_id":    levelID.String(),
		"category_id": categoryID,
		"name":        "测试乐器",
		"brand":       "雅马哈",
	}

	bodyBytes, _ := json.Marshal(requestBody)
	req := httptest.NewRequest("POST", "/api/instruments", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	var instrument struct {
		ID      string
		LevelID *uuid.UUID
		Level   string
	}

	err = db.Raw(`SELECT id, level_id, level FROM instruments 
		WHERE tenant_id = ? AND sn = ?`, tenantID, "TEST-SN-LEVELID").Scan(&instrument).Error
	require.NoError(t, err)

	require.NotNil(t, instrument.LevelID, "LevelID should be saved")
	assert.Equal(t, levelID, *instrument.LevelID)

	// Cleanup
	db.Exec(`DELETE FROM instruments WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM categories WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM instrument_levels`)
}

func TestCreateInstrumentWithProperties(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("Test database not available, skipping:", err)
		return
	}
	database.SetDB(db)

	tenantID := uuid.New().String()
	now := time.Now()

	// Create test category
	categoryID := uuid.New().String()
	db.Exec(`INSERT INTO categories (id, name, tenant_id, created_at) 
		VALUES (?, ?, ?, 1, ?, ?)`,
		categoryID, "测试分类", tenantID, now, now)

	// Create test properties
	prop1ID := uuid.New().String()
	prop2ID := uuid.New().String()
	db.Exec(`INSERT INTO properties (id, name, tenant_id, property_type, is_required, created_at) 
		VALUES (?, ?, ?, 'text', false, ?)`,
		prop1ID, "品牌", tenantID, now, now)
	db.Exec(`INSERT INTO properties (id, name, tenant_id, property_type, is_required, created_at) 
		VALUES (?, ?, ?, 'text', false, ?)`,
		prop2ID, "颜色", tenantID, now, now)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "test-user")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/instruments", CreateInstrument)

	requestBody := map[string]interface{}{
		"sn":          "TEST-SN-PROPS",
		"level":       "professional",
		"category_id": categoryID,
		"name":        "测试乐器",
		"properties": map[string]interface{}{
			"品牌": []string{"雅马哈", "卡哇伊"},
			"颜色": []string{"黑色"},
		},
	}

	bodyBytes, _ := json.Marshal(requestBody)
	req := httptest.NewRequest("POST", "/api/instruments", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	// Get created instrument
	var instrument models.Instrument
	err = db.Where("sn = ? AND tenant_id = ?", "TEST-SN-PROPS", tenantID).First(&instrument).Error
	require.NoError(t, err)

	// Verify instrument_properties were created
	var instrumentProps []models.InstrumentProperty
	err = db.Where("instrument_id = ? AND tenant_id = ?", instrument.ID, tenantID).Find(&instrumentProps).Error
	require.NoError(t, err)
	assert.Len(t, instrumentProps, 3, "Should have 3 instrument_properties")

	// Verify property_options were created with status=pending
	var propOptionCount int
	db.Raw(`SELECT COUNT(*) FROM property_options WHERE tenant_id = ? AND status = 'pending'`, tenantID).Scan(&propOptionCount)
	assert.GreaterOrEqual(t, propOptionCount, 3, "Should have created property_options with pending status")

	// Cleanup
	db.Exec(`DELETE FROM instruments WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM instrument_properties WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM property_options WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM properties WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM categories WHERE tenant_id = ?`, tenantID)
}

func TestCreateInstrumentPropertyValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("Test database not available, skipping:", err)
		return
	}
	database.SetDB(db)

	tenantID := uuid.New().String()
	now := time.Now()

	// Create test category
	categoryID := uuid.New().String()
	db.Exec(`INSERT INTO categories (id, name, tenant_id, created_at) 
		VALUES (?, ?, ?, ?)`,
		categoryID, "测试分类", tenantID, now)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
		c.Next()
	})
	router.POST("/api/instruments", CreateInstrument)

	// Test with undefined property (should fail or skip)
	requestBody := map[string]interface{}{
		"sn":          "TEST-SN-INVALID-PROP",
		"level":       "professional",
		"category_id": categoryID,
		"properties": map[string]interface{}{
			"不存在的属性": []string{"测试值"},
		},
	}

	bodyBytes, _ := json.Marshal(requestBody)
	req := httptest.NewRequest("POST", "/api/instruments", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should still create instrument but skip invalid properties
	assert.Equal(t, http.StatusCreated, w.Code)

	// Verify no instrument_properties were created for invalid property
	var instrument models.Instrument
	err = db.Where("sn = ? AND tenant_id = ?", "TEST-SN-INVALID-PROP", tenantID).First(&instrument).Error
	require.NoError(t, err)

	var instrumentProps []models.InstrumentProperty
	err = db.Where("instrument_id = ? AND tenant_id = ?", instrument.ID, tenantID).Find(&instrumentProps).Error
	require.NoError(t, err)
	assert.Len(t, instrumentProps, 0, "Should not have instrument_properties for undefined property")

	// Cleanup
	db.Exec(`DELETE FROM instruments WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM categories WHERE tenant_id = ?`, tenantID)
}

func TestCreateInstrumentBackwardCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("Test database not available, skipping:", err)
		return
	}
	database.SetDB(db)

	// Clean instrument_levels
	db.Exec(`DELETE FROM instrument_levels`)

	tenantID := uuid.New().String()
	now := time.Now()

	// Create instrument_levels entry for backward compatibility
	levelID := uuid.New()
	db.Exec(`INSERT INTO instrument_levels (id, caption, code, sort_order) 
		VALUES (?, '专业', 'professional', 2)`, levelID)

	// Create test category
	categoryID := uuid.New().String()
	db.Exec(`INSERT INTO categories (id, name, tenant_id, created_at) 
		VALUES (?, ?, ?, ?)`,
		categoryID, "测试分类", tenantID, now)

	// Create test property
	propID := uuid.New().String()
	db.Exec(`INSERT INTO properties (id, name, tenant_id, property_type, is_required, created_at) 
		VALUES (?, ?, ?, 'text', false, ?)`,
		propID, "品牌", tenantID, now, now)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
		c.Next()
	})
	router.POST("/api/instruments", CreateInstrument)

	// Test backward compatibility with level string
	requestBody := map[string]interface{}{
		"sn":          "TEST-SN-BACKWARD",
		"level":       "专业", // Using Chinese caption instead of level_id
		"category_id": categoryID,
		"properties": map[string]interface{}{
			"品牌": []string{"雅马哈"},
		},
	}

	bodyBytes, _ := json.Marshal(requestBody)
	req := httptest.NewRequest("POST", "/api/instruments", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	// Verify level was mapped to level_id
	var instrument struct {
		ID      string
		LevelID *uuid.UUID
		Level   string
	}

	err = db.Raw(`SELECT id, level_id, level FROM instruments 
		WHERE tenant_id = ? AND sn = ?`, tenantID, "TEST-SN-BACKWARD").Scan(&instrument).Error
	require.NoError(t, err)

	require.NotNil(t, instrument.LevelID, "LevelID should be auto-mapped from '专业'")
	assert.Equal(t, levelID, *instrument.LevelID)
	assert.Equal(t, "专业", instrument.Level)

	// Verify properties were processed
	var instrumentProps []models.InstrumentProperty
	err = db.Where("instrument_id = ? AND tenant_id = ?", instrument.ID, tenantID).Find(&instrumentProps).Error
	require.NoError(t, err)
	assert.Len(t, instrumentProps, 1, "Should have 1 instrument_property")

	// Cleanup
	db.Exec(`DELETE FROM instruments WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM instrument_properties WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM property_options WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM categories WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM instrument_levels`)
	db.Exec(`DELETE FROM properties WHERE tenant_id = ?`, tenantID)
}
