package handlers

// #2261 返工：Delete/写路径在显式模式下必须隔离。

import (
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

func TestOrdersInstrumentsDelete_ExplicitScope_2261(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	orgA, orgB := uuid.NewString(), uuid.NewString()
	userA, userB := uuid.NewString(), uuid.NewString()
	instA, instB := uuid.NewString(), uuid.NewString()
	orderA, orderB := uuid.NewString(), uuid.NewString()
	for _, u := range []struct{ id, org string }{{userA, orgA}, {userB, orgB}} {
		require.NoError(t, db.Create(&models.User{ID: u.id, IAMSub: u.id, TenantID: u.org, OrgID: u.org, Status: "active"}).Error)
	}
	require.NoError(t, db.Create(&models.Instrument{ID: instA, TenantID: orgA, OrgID: &orgA, SN: "DEL-A", StockStatus: "available"}).Error)
	require.NoError(t, db.Create(&models.Instrument{ID: instB, TenantID: orgB, OrgID: &orgB, SN: "DEL-B", StockStatus: "available"}).Error)
	require.NoError(t, db.Create(&models.Order{ID: orderA, TenantID: orgA, OrgID: orgA, UserID: userA, InstrumentID: instA, Level: "entry", LeaseTerm: 30, Status: "reserved"}).Error)
	require.NoError(t, db.Create(&models.Order{ID: orderB, TenantID: orgB, OrgID: orgB, UserID: userB, InstrumentID: instB, Level: "entry", LeaseTerm: 30, Status: "reserved"}).Error)

	actor := testutil.TestActor{TenantID: orgA, OrgID: orgA, UserID: uuid.NewString(), Role: "OWNER"}
	router := func(explicit bool) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
			c.Next()
		})
		if explicit {
			r.Use(middleware.ExplicitTenantScope())
		}
		r.POST("/orders/:id/cancel", CancelOrder)
		r.DELETE("/instruments/:id", DeleteInstrument)
		r.GET("/probe", func(c *gin.Context) { c.String(200, middleware.GetTenantID(c.Request.Context())) })
		return r
	}
	call := func(explicit bool, method, path string) int {
		w := httptest.NewRecorder()
		r := router(explicit)
		r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		return w.Code
	}

	// 取消：本商户 200；跨商户 404；on/off 等价
	assert.Contains(t, []int{http.StatusOK, http.StatusConflict}, call(false, http.MethodPost, "/orders/"+orderA+"/cancel"))
	assert.Equal(t, http.StatusNotFound, call(false, http.MethodPost, "/orders/"+orderB+"/cancel"))
	assert.Equal(t, call(false, http.MethodPost, "/orders/"+orderA+"/cancel"), call(true, http.MethodPost, "/orders/"+orderA+"/cancel"))
	assert.Equal(t, http.StatusNotFound, call(true, http.MethodPost, "/orders/"+orderB+"/cancel"))

	// 删除乐器：每次新建独立记录，避免前序调用影响
	mk := func(org string) string {
		id := uuid.NewString()
		require.NoError(t, db.Create(&models.Instrument{ID: id, TenantID: org, OrgID: &org, SN: "DEL-" + id[:8], StockStatus: "available"}).Error)
		return id
	}
	ownA := mk(orgA)
	assert.Equal(t, http.StatusOK, call(false, http.MethodDelete, "/instruments/"+ownA))

	crossB := mk(orgB)
	assert.Equal(t, http.StatusNotFound, call(false, http.MethodDelete, "/instruments/"+crossB))

	ownOnA := mk(orgA)
	assert.Equal(t, http.StatusOK, call(true, http.MethodDelete, "/instruments/"+ownOnA))

	crossOnB := mk(orgB)
	assert.Equal(t, http.StatusNotFound, call(true, http.MethodDelete, "/instruments/"+crossOnB))

}
