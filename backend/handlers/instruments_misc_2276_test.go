package handlers

// #2276 批次 B：乐器 activity-log / scrap 在显式模式下隔离。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestInstrumentMisc_ExplicitScope_2276(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	instA, instB := uuid.NewString(), uuid.NewString()
	for _, x := range []struct{ id, org string }{{instA, orgA}, {instB, orgB}} {
		require.NoError(t, db.Create(&models.Instrument{ID: x.id, TenantID: x.org, OrgID: &x.org, SN: "MISC-" + x.id[:6], StockStatus: "available"}).Error)
	}
	for _, x := range []struct{ inst, org string }{{instA, orgA}, {instB, orgB}} {
		oid := uuid.NewString()
		require.NoError(t, db.Create(&models.Order{ID: oid, TenantID: x.org, OrgID: x.org, UserID: uuid.NewString(),
			InstrumentID: x.inst, Level: "entry", LeaseTerm: 30, Status: "paid"}).Error)
		require.NoError(t, db.Create(&models.OrderStatusHistory{ID: uuid.NewString(), TenantID: x.org, OrderID: oid, StatusFrom: "reserved", StatusTo: "paid"}).Error)
	}

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
		r.GET("/instruments/:id/activity-log", GetInstrumentActivityLog)
		r.POST("/instruments/:id/scrap", ScrapInstrument)
		return r
	}
	logCount := func(explicit bool, id string) int {
		w := httptest.NewRecorder()
		router(explicit).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/instruments/"+id+"/activity-log", nil))
		var resp struct {
			Data struct {
				Sessions []interface{} `json:"sessions"`
			} `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return len(resp.Data.Sessions)
	}
	assert.GreaterOrEqual(t, logCount(false, instA), 1)
	assert.Equal(t, 0, logCount(false, instB))
	assert.GreaterOrEqual(t, logCount(true, instA), 1, "on/off 等价（本商户）")
	assert.Equal(t, 0, logCount(true, instB), "显式模式跨商户日志不得可见")

	scrap := func(explicit bool, id string) int {
		req := httptest.NewRequest(http.MethodPost, "/instruments/"+id+"/scrap", strings.NewReader(`{"reason":"test"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router(explicit).ServeHTTP(w, req)
		return w.Code
	}
	assert.Equal(t, http.StatusNotFound, scrap(false, instB))
	assert.Equal(t, http.StatusNotFound, scrap(true, instB), "显式模式跨商户报废必须 404")
}
