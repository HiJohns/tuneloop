package handlers

// #2276：repair 状态机写路由显式租户范围（本租户 200 / 跨租户 404）。

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

func TestRepairWrite_ExplicitScope_2276(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantA, tenantB := uuid.NewString(), uuid.NewString()
	instID := uuid.NewString()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantA, OrgID: &tenantA, SN: "REP-2276", RepairStatus: "repair_pending",
	}).Error)

	router := func(tid string, explicit bool) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			actor := testutil.TestActor{TenantID: tid, OrgID: tid, UserID: uuid.NewString(), Role: "ADMIN"}
			c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
			c.Next()
		})
		if explicit {
			r.Use(middleware.ExplicitTenantScope())
		}
		r.POST("/repair/:id/start", NewRepairHandler().StartRepair)
		r.POST("/repair/:id/records", NewRepairHandler().AddRecord)
		r.POST("/repair-requests/:id/complete", NewRepairRequestHandler().CompleteRepairRequest)
		r.POST("/repair-services/:id/receive", NewRepairServiceHandler().Receive)
		return r
	}
	call := func(tid string, explicit bool, path, body string) (int, map[string]interface{}) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		router(tid, explicit).ServeHTTP(w, req)
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp
	}

	// 跨租户：404，且不得改状态
	code, _ := call(tenantB, true, "/repair/"+instID+"/start", "")
	assert.Equal(t, http.StatusNotFound, code, "跨租户 repair 写必须 404")
	var after models.Instrument
	require.NoError(t, db.Where("id = ?", instID).First(&after).Error)
	assert.Equal(t, "repair_pending", after.RepairStatus, "跨租户不得修改")

	// 本租户：200
	code, resp := call(tenantA, true, "/repair/"+instID+"/start", "")
	require.Equal(t, http.StatusOK, code, resp)
	require.NoError(t, db.Where("id = ?", instID).First(&after).Error)
	assert.Equal(t, "repair_in_progress", after.RepairStatus)
}

func TestRepairMoreWrites_ExplicitScope_2276(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantA, tenantB := uuid.NewString(), uuid.NewString()
	instID := uuid.NewString()
	require.NoError(t, db.Create(&models.Instrument{ID: instID, TenantID: tenantA, OrgID: &tenantA, SN: "REP-MORE-2276", RepairStatus: "repair_in_progress"}).Error)
	rrID := uuid.NewString()
	require.NoError(t, db.Create(&models.RepairRequest{
		ID: rrID, TenantID: tenantA, SiteID: uuid.NewString(), UserID: uuid.NewString(),
		UserInstrumentID: uuid.NewString(), Status: models.RepairReqStatusPendingShip,
		Type: "repair_service", MerchantType: "full",
	}).Error)

	router := func(tid string, explicit bool) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			actor := testutil.TestActor{TenantID: tid, OrgID: tid, UserID: uuid.NewString(), Role: "ADMIN"}
			c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
			c.Next()
		})
		if explicit {
			r.Use(middleware.ExplicitTenantScope())
		}
		r.POST("/repair/:id/records", NewRepairHandler().AddRecord)
		r.POST("/repair-requests/:id/complete", NewRepairRequestHandler().CompleteRepairRequest)
		r.POST("/repair-services/:id/receive", NewRepairServiceHandler().Receive)
		return r
	}
	call := func(tid string, explicit bool) int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/repair/"+instID+"/records", bytes.NewBufferString(`{"comment":"x","photos":[]}`))
		req.Header.Set("Content-Type", "application/json")
		router(tid, explicit).ServeHTTP(w, req)
		return w.Code
	}
	assert.Equal(t, http.StatusNotFound, call(tenantB, true))
	assert.Equal(t, http.StatusNotFound, call(tenantB, false), "on/off 等价")
	assert.Equal(t, http.StatusOK, call(tenantA, true))

	post := func(tid string, explicit bool, path, body string) int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		router(tid, explicit).ServeHTTP(w, req)
		return w.Code
	}
	// repair-request / repair-service 代表路由：跨租户 on/off 均 404
	assert.Equal(t, http.StatusNotFound, post(tenantB, true, "/repair-requests/"+rrID+"/complete", "{}"))
	assert.Equal(t, http.StatusNotFound, post(tenantB, false, "/repair-requests/"+rrID+"/complete", "{}"), "on/off 等价")
	assert.Equal(t, http.StatusNotFound, post(tenantB, true, "/repair-services/"+rrID+"/receive", `{"photos":["/uploads/x.jpg"]}`))
	assert.Equal(t, http.StatusNotFound, post(tenantB, false, "/repair-services/"+rrID+"/receive", `{"photos":["/uploads/x.jpg"]}`), "on/off 等价")
}
