package handlers

// #2276 批次 B：乐器详情在显式模式下隔离。

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

func TestInstrumentDetail_ExplicitScope_2276(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	instA, instB := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Create(&models.Instrument{ID: instA, TenantID: orgA, OrgID: &orgA, SN: "DET-A", StockStatus: "available"}).Error)
	require.NoError(t, db.Create(&models.Instrument{ID: instB, TenantID: orgB, OrgID: &orgB, SN: "DET-B", StockStatus: "available"}).Error)

	actor := testutil.TestActor{TenantID: orgA, OrgID: orgA, UserID: uuid.NewString(), Role: "OWNER"}
	call := func(explicit bool, id string) int {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
			c.Next()
		})
		if explicit {
			r.Use(middleware.ExplicitTenantScope())
		}
		r.GET("/api/instruments/:id", GetInstrumentByID)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/instruments/"+id, nil))
		return w.Code
	}

	ownOff, crossOff := call(false, instA), call(false, instB)
	assert.Equal(t, http.StatusOK, ownOff)
	assert.Equal(t, http.StatusNotFound, crossOff)
	assert.Equal(t, ownOff, call(true, instA), "on/off 等价（本商户）")
	assert.Equal(t, http.StatusNotFound, call(true, instB), "显式模式跨商户 404")
}
