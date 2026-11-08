package handlers

// #2276 批次 B：pickup / admin-update 显式隔离。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrdersPickupAdmin_ExplicitScope_2276(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	mkOrder := func(org string) string {
		id := uuid.NewString()
		require.NoError(t, db.Create(&models.Order{ID: id, TenantID: org, OrgID: org, UserID: uuid.NewString(),
			InstrumentID: uuid.NewString(), Level: "entry", LeaseTerm: 30, Status: "paid"}).Error)
		return id
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
		r.POST("/orders/:id/pickup", PickupOrder)
		r.PUT("/orders/:id/admin-update", AdminUpdateOrder)
		return r
	}
	call := func(explicit bool, method, path, body string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router(explicit).ServeHTTP(w, req)
		return w.Code
	}
	// admin-update：本商户 200；跨商户 404；on/off 等价
	own := mkOrder(orgA)
	cross := mkOrder(orgB)
	body := `{"status":"completed"}`
	ownOff := call(false, http.MethodPut, "/orders/"+own+"/admin-update", body)
	assert.Equal(t, http.StatusOK, ownOff)
	assert.Equal(t, http.StatusNotFound, call(false, http.MethodPut, "/orders/"+cross+"/admin-update", body))
	own2 := mkOrder(orgA)
	assert.Equal(t, ownOff, call(true, http.MethodPut, "/orders/"+own2+"/admin-update", body))
	assert.Equal(t, http.StatusNotFound, call(true, http.MethodPut, "/orders/"+mkOrder(orgB)+"/admin-update", body))

	// pickup：跨商户 404（on/off 等价）；本商户 on/off 等价
	pickCross := mkOrder(orgB)
	crossOff := call(false, http.MethodPost, "/orders/"+pickCross+"/pickup", `{"pickup_time":"`+time.Now().Format(time.RFC3339)+`"}`)
	assert.Equal(t, http.StatusNotFound, crossOff)
	assert.Equal(t, crossOff, call(true, http.MethodPost, "/orders/"+mkOrder(orgB)+"/pickup", `{"pickup_time":"`+time.Now().Format(time.RFC3339)+`"}`))
	pickOwn := mkOrder(orgA)
	assert.Equal(t, call(false, http.MethodPost, "/orders/"+pickOwn+"/pickup", `{"pickup_time":"`+time.Now().Format(time.RFC3339)+`"}`), call(true, http.MethodPost, "/orders/"+mkOrder(orgA)+"/pickup", `{"pickup_time":"`+time.Now().Format(time.RFC3339)+`"}`))
}
