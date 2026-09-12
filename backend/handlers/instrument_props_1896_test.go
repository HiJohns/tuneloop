package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1896: the public detail must return only ASSIGNED properties. Previously
// every property definition was injected with an empty value array, so the
// customer page rendered a row of "-" for every unfilled field.
func TestGetPublicInstrument_PropertiesOnlyAssigned(t *testing.T) {
	db, tenantID := setupPropertyResolutionTest(t)
	if db == nil {
		return
	}
	defer db.Exec("DELETE FROM instrument_properties WHERE tenant_id = ?", tenantID)
	defer db.Exec("DELETE FROM properties WHERE tenant_id = ?", tenantID)
	defer db.Exec("DELETE FROM instruments WHERE tenant_id = ?", tenantID)

	orgID := tenantID
	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "1896-PUB", StockStatus: "available",
	}).Error)

	mkProp := func(name string) {
		require.NoError(t, db.Create(&models.Property{
			ID: uuid.New().String(), TenantID: tenantID, Name: name,
			PropertyType: "text", Caption: name, ScopeType: "global", Status: "active",
		}).Error)
	}
	mkProp("制琴师") // definition only, no value
	mkProp("尺寸")  // definition with an assigned value

	require.NoError(t, db.Create(&models.InstrumentProperty{
		ID: uuid.New().String(), TenantID: tenantID,
		InstrumentID: instID, PropertyName: "尺寸", Value: "4/4",
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
			Properties map[string][]string `json:"properties"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, []string{"4/4"}, resp.Data.Properties["尺寸"])
	_, injected := resp.Data.Properties["制琴师"]
	assert.False(t, injected, "unassigned property definitions must not be injected")
	assert.Len(t, resp.Data.Properties, 1)
}
