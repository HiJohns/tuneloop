package handlers

// #2261 返工：/orders/by-trade-no 在显式模式下必须按租户隔离。

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

func TestOrdersByTradeNo_ExplicitScopeIsolation_2261(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	orgA, orgB := uuid.NewString(), uuid.NewString()
	userA, userB := uuid.NewString(), uuid.NewString()
	sessA, sessB := uuid.NewString(), uuid.NewString()
	instA, instB := uuid.NewString(), uuid.NewString()
	ta, tb := "tradeA2261", "tradeB2261"

	require.NoError(t, db.Create(&models.User{ID: userA, IAMSub: userA, TenantID: orgA, OrgID: orgA, Status: "active"}).Error)
	require.NoError(t, db.Create(&models.User{ID: userB, IAMSub: userB, TenantID: orgB, OrgID: orgB, Status: "active"}).Error)
	require.NoError(t, db.Create(&models.Instrument{ID: instA, TenantID: orgA, OrgID: &orgA, StockStatus: "available"}).Error)
	require.NoError(t, db.Create(&models.Instrument{ID: instB, TenantID: orgB, OrgID: &orgB, StockStatus: "available"}).Error)
	require.NoError(t, db.Create(&models.PaymentSession{ID: sessA, TenantID: orgA, UserID: userA, Amount: models.FromYuan(1), OutTradeNo: &ta}).Error)
	require.NoError(t, db.Create(&models.PaymentSession{ID: sessB, TenantID: orgB, UserID: userB, Amount: models.FromYuan(1), OutTradeNo: &tb}).Error)

	actor := testutil.TestActor{TenantID: orgA, OrgID: orgA, UserID: uuid.NewString(), Role: "OWNER"}
	run := func(explicit bool, trade string) int {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
			c.Next()
		})
		if explicit {
			r.Use(middleware.ExplicitTenantScope())
		}
		r.GET("/orders/by-trade-no/:out_trade_no", GetOrdersByOutTradeNo)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/orders/by-trade-no/"+trade, nil))
		return w.Code
	}

	ownOff, crossOff := run(false, ta), run(false, tb)
	ownOn, crossOn := run(true, ta), run(true, tb)
	assert.Equal(t, http.StatusOK, ownOff)
	assert.Equal(t, http.StatusNotFound, crossOff)
	assert.Equal(t, ownOff, ownOn, "on/off 等价（本商户）")
	assert.Equal(t, crossOff, crossOn, "on/off 等价（跨商户）")
	assert.Equal(t, http.StatusNotFound, crossOn, "显式模式下跨商户会话必须 404")
}
