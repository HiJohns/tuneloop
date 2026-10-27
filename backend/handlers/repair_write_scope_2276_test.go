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
	siteA := uuid.NewString()
	instID, techID, rrID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Create(&models.Instrument{ID: instID, TenantID: tenantA, OrgID: &tenantA, SN: "REP-MORE-2276", RepairStatus: "repair_in_progress"}).Error)
	require.NoError(t, db.Create(&models.User{ID: techID, IAMSub: techID, TenantID: tenantA, OrgID: tenantA, Status: "active"}).Error)
	require.NoError(t, db.Create(&models.SiteMember{TenantID: tenantA, SiteID: siteA, UserID: techID, Role: "repair_technician", Status: "active"}).Error)
	require.NoError(t, db.Create(&models.RepairRequest{
		ID: rrID, TenantID: tenantA, SiteID: siteA, UserID: uuid.NewString(),
		UserInstrumentID: uuid.NewString(), Status: models.RepairReqStatusShipping,
		Type: repairServiceTypeVal, MerchantType: "full", TechnicianID: &techID,
	}).Error)

	router := func(tid string, explicit bool) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			actor := testutil.TestActor{TenantID: tid, OrgID: tid, UserID: techID, Role: "STAFF"}
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
	post := func(tid string, explicit bool, path, body string) (int, map[string]interface{}) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		router(tid, explicit).ServeHTTP(w, req)
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp
	}

	// 跨租户：on/off 均 404，状态不变
	code, _ := post(tenantB, true, "/repair-services/"+rrID+"/receive", `{"photos":["/uploads/x.jpg"]}`)
	assert.Equal(t, http.StatusNotFound, code)
	code, _ = post(tenantB, false, "/repair-services/"+rrID+"/receive", `{"photos":["/uploads/x.jpg"]}`)
	assert.Equal(t, http.StatusNotFound, code, "on/off 等价")
	var after models.RepairRequest
	require.NoError(t, db.Where("id = ?", rrID).First(&after).Error)
	assert.Equal(t, models.RepairReqStatusShipping, after.Status)

	// 本租户：收货成功 → repairing
	code, resp := post(tenantA, true, "/repair-services/"+rrID+"/receive", `{"photos":["/uploads/x.jpg"]}`)
	require.Equal(t, http.StatusOK, code, resp)
	require.NoError(t, db.Where("id = ?", rrID).First(&after).Error)
	assert.Equal(t, models.RepairReqStatusRepairing, after.Status)

	// 本租户：完成 → return_pending
	code, resp = post(tenantA, true, "/repair-requests/"+rrID+"/complete", "{}")
	require.Equal(t, http.StatusOK, code, resp)
	require.NoError(t, db.Where("id = ?", rrID).First(&after).Error)
	assert.Equal(t, models.RepairReqStatusReturnPend, after.Status)

	// 跨租户 complete on/off 404（此时本单已 return_pending；跨租户仍应 404）
	code, resp = post(tenantB, true, "/repair-requests/"+rrID+"/complete", "{}")
	assert.Equal(t, http.StatusNotFound, code, resp)
	code, resp = post(tenantB, false, "/repair-requests/"+rrID+"/complete", "{}")
	assert.Equal(t, http.StatusNotFound, code, resp, "on/off 等价")

	// AddRecord 跨租户 404 / 本租户 200
	code, _ = post(tenantB, true, "/repair/"+instID+"/records", `{"comment":"x","photos":[]}`)
	assert.Equal(t, http.StatusNotFound, code)
	code, resp = post(tenantA, true, "/repair/"+instID+"/records", `{"comment":"x","photos":[]}`)
	require.Equal(t, http.StatusOK, code, resp)
}
