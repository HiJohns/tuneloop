package handlers

// #2276：B3 遗留 instrument 写路由显式范围。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstrumentsLeftovers_ExplicitScope_2276(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantA, tenantB := uuid.NewString(), uuid.NewString()
	instID := uuid.NewString()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantA, OrgID: &tenantA, SN: "INS-2276", StockStatus: "available",
	}).Error)

	router := func(tid string) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			actor := testutil.TestActor{TenantID: tid, OrgID: tid, UserID: uuid.NewString(), Role: "ADMIN"}
			c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
			c.Next()
		}, middleware.ExplicitTenantScope())
		r.PUT("/instruments/:id/status", UpdateInstrumentStatus)
		r.PUT("/instruments/batch-pricing", BatchSetInstrumentPricing)
		r.PUT("/instruments/:id/sort", SortInstrument)
		r.GET("/instruments/:id/pricing-v2", GetInstrumentPricingV2)
		r.GET("/instruments/:id/photos/latest", GetLatestInstrumentPhotos)
		return r
	}
	put := func(tid, path string, body interface{}) (int, map[string]interface{}) {
		b, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		router(tid).ServeHTTP(w, req)
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp
	}

	// 跨租户状态更新 → 404，状态不变
	code, _ := put(tenantB, "/instruments/"+instID+"/status", map[string]interface{}{"stock_status": "maintenance"})
	assert.Equal(t, http.StatusNotFound, code)
	var after models.Instrument
	require.NoError(t, db.Where("id = ?", instID).First(&after).Error)
	assert.Equal(t, "available", after.StockStatus)

	// 本租户 → 200
	code, resp := put(tenantA, "/instruments/"+instID+"/status", map[string]interface{}{"stock_status": "maintenance"})
	require.Equal(t, http.StatusOK, code, resp)

	// 跨租户批量改价 → updated=0；本租户 → updated=1
	items := map[string]interface{}{"items": []map[string]interface{}{{"id": instID, "base_daily_rate": 88.0}}}
	code, resp = put(tenantB, "/instruments/batch-pricing", items)
	require.Equal(t, http.StatusOK, code, resp)
	assert.Equal(t, float64(0), resp["data"].(map[string]interface{})["updated"], "跨租户不得改价")
	code, resp = put(tenantA, "/instruments/batch-pricing", items)
	require.Equal(t, http.StatusOK, code, resp)
	assert.Equal(t, float64(1), resp["data"].(map[string]interface{})["updated"])
}

func TestInstrumentsLeftovers_MoreRoutes_2276(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantA, tenantB := uuid.NewString(), uuid.NewString()
	instID := uuid.NewString()
	require.NoError(t, db.Create(&models.Instrument{ID: instID, TenantID: tenantA, OrgID: &tenantA, SN: "INS-MORE-2276", StockStatus: "available"}).Error)

	router := func(tid string) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			actor := testutil.TestActor{TenantID: tid, OrgID: tid, UserID: uuid.NewString(), Role: "ADMIN"}
			c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
			c.Next()
		}, middleware.ExplicitTenantScope())
		r.PUT("/instruments/:id/sort", SortInstrument)
		r.GET("/instruments/:id/pricing-v2", GetInstrumentPricingV2)
		r.GET("/instruments/:id/photos/latest", GetLatestInstrumentPhotos)
		return r
	}
	get := func(tid, path string) int {
		w := httptest.NewRecorder()
		router(tid).ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w.Code
	}
	put := func(tid, path string, body interface{}) int {
		b, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		router(tid).ServeHTTP(w, req)
		return w.Code
	}

	assert.Equal(t, http.StatusNotFound, get(tenantB, "/instruments/"+instID+"/pricing-v2"))
	assert.Equal(t, http.StatusNotFound, get(tenantB, "/instruments/"+instID+"/photos/latest"))
	assert.Equal(t, http.StatusNotFound, put(tenantB, "/instruments/"+instID+"/sort", map[string]interface{}{"direction": "up"}))
	assert.NotEqual(t, http.StatusNotFound, get(tenantA, "/instruments/"+instID+"/pricing-v2"))
}
