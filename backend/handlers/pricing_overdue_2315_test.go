package handlers

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

func TestBatchSetInstrumentPricing_OverdueOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID, instID := uuid.NewString(), uuid.NewString()
	orgID := tenantID
	rate := models.FromYuan(100)
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID, SN: "OVD-2315",
		StockStatus: "available", BaseDailyRate: &rate,
	}).Error)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.TestActor{TenantID: tenantID, OrgID: orgID, UserID: uuid.NewString(), Role: "ADMIN"}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	}, middleware.ExplicitTenantScope())
	r.PUT("/instruments/batch-pricing", BatchSetInstrumentPricing)
	r.GET("/instruments/:id/pricing-v2", GetInstrumentPricingV2)

	body, _ := json.Marshal(map[string]interface{}{
		"items": []map[string]interface{}{{
			"id":        instID,
			"overrides": map[string]interface{}{"overdue_daily_fee": 15},
		}},
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/instruments/batch-pricing", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/instruments/"+instID+"/pricing-v2", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Data struct {
			Overdue float64 `json:"overdue_daily_fee"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(1500), resp.Data.Overdue)
}
