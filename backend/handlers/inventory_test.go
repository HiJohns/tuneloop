package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

func TestGetInventoryRentSetting(t *testing.T) {
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
	siteID := uuid.New()

	// Create test site
	db.Exec(`INSERT INTO sites (id, name, tenant_id, status, created_at, updated_at) 
		VALUES (?, '测试网点', ?, 'active', ?, ?)`,
		siteID, tenantID, now, now)

	// Create test instrument with pricing
	instrumentID := uuid.New()
	pricingJSON := `[{"name":"standard","daily_rent":100.00,"monthly_rent":2500.00,"deposit":5000.00,"stock":5}]`
	db.Exec(`INSERT INTO instruments (id, sn, category_name, level_name, site_id, pricing, tenant_id, org_id, stock_status, name, level, created_at, updated_at) 
		VALUES (?, 'SN123456', '钢琴', '专业级', ?, ?, ?, ?, 'available', '测试钢琴', 'professional', ?, ?)`,
		instrumentID, siteID, pricingJSON, tenantID, tenantID, now, now)

	// Setup route
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	inventoryHandler := NewInventoryHandler()
	router.GET("/inventory/rent-setting", inventoryHandler.GetRentSetting)

	// Call API
	req := httptest.NewRequest("GET", "/inventory/rent-setting?page=1&pageSize=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Verify
	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Code int `json:"code"`
		Data struct {
			List     []map[string]interface{} `json:"list"`
			Total    int64                    `json:"total"`
			Page     int                      `json:"page"`
			PageSize int                      `json:"pageSize"`
		} `json:"data"`
	}

	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 20000, response.Code)
	assert.Greater(t, len(response.Data.List), 0)

	// Verify first item has correct structure
	firstItem := response.Data.List[0]
	assert.NotEmpty(t, firstItem["id"])
	assert.Equal(t, "SN123456", firstItem["sn"])
	assert.Equal(t, "钢琴", firstItem["category_name"])
	assert.Equal(t, "专业级", firstItem["level_name"])
	assert.Equal(t, 100.0, firstItem["daily_rent"])

	// Cleanup
	db.Exec(`DELETE FROM instruments WHERE tenant_id = ?`, tenantID)
	db.Exec(`DELETE FROM sites WHERE tenant_id = ?`, tenantID)
}

func TestBatchUpdateRent(t *testing.T) {
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

	// Create test instrument with initial pricing
	instrumentID := uuid.New()
	pricingJSON := `[{"name":"standard","daily_rent":100.00}]`
	db.Exec(`INSERT INTO instruments (id, sn, category_name, level_name, pricing, tenant_id, org_id, stock_status, name, level, created_at, updated_at) 
		VALUES (?, 'SN123456', '钢琴', '专业级', ?, ?, ?, 'available', '测试钢琴', 'professional', ?, ?)`,
		instrumentID, pricingJSON, tenantID, tenantID, now, now)

	// Setup route
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	inventoryHandler := NewInventoryHandler()
	router.PUT("/inventory/rent-setting/batch", inventoryHandler.BatchUpdateRent)

	// Call API with new rent price
	body := map[string]interface{}{
		"items": []map[string]interface{}{
			{
				"id":         instrumentID.String(),
				"daily_rent": 150.00,
			},
		},
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/inventory/rent-setting/batch", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Code int `json:"code"`
		Data struct {
			Updated int `json:"updated"`
		} `json:"data"`
	}

	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 20000, response.Code)
	assert.Equal(t, 1, response.Data.Updated)

	// Verify database update
	var instrument models.Instrument
	err = db.First(&instrument, "id = ?", instrumentID).Error
	require.NoError(t, err)

	var pricing []map[string]interface{}
	err = json.Unmarshal([]byte(instrument.Pricing), &pricing)
	require.NoError(t, err)
	assert.Equal(t, 150.0, pricing[0]["daily_rent"])

	// Cleanup
	db.Exec(`DELETE FROM instruments WHERE tenant_id = ?`, tenantID)
}
