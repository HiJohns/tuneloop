package handlers

// #2276：repair 状态机写路由显式租户范围（本租户 200 / 跨租户 404）。

import (
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

	router := func(tid string) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			actor := testutil.TestActor{TenantID: tid, OrgID: tid, UserID: uuid.NewString(), Role: "ADMIN"}
			c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
			c.Next()
		}, middleware.ExplicitTenantScope())
		r.POST("/repair/:id/start", NewRepairHandler().StartRepair)
		return r
	}
	call := func(tid string) (int, map[string]interface{}) {
		w := httptest.NewRecorder()
		router(tid).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/repair/"+instID+"/start", nil))
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp
	}

	// 跨租户：404，且不得改状态
	code, _ := call(tenantB)
	assert.Equal(t, http.StatusNotFound, code, "跨租户 repair 写必须 404")
	var after models.Instrument
	require.NoError(t, db.Where("id = ?", instID).First(&after).Error)
	assert.Equal(t, "repair_pending", after.RepairStatus, "跨租户不得修改")

	// 本租户：200
	code, resp := call(tenantA)
	require.Equal(t, http.StatusOK, code, resp)
	require.NoError(t, db.Where("id = ?", instID).First(&after).Error)
	assert.Equal(t, "repair_in_progress", after.RepairStatus)
}
