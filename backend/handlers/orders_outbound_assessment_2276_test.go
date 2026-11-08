package handlers

// #2276 批次 B：outbound / assessment 显式隔离。

import (
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

func TestOutboundAssessment_ExplicitScope_2276(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	mk := func(org string) string {
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
		h := NewOutboundHandler(db)
		a := NewAssessmentHandler(db)
		r.GET("/orders/:id/outbound-photos", h.GetOutboundPhotos)
		r.POST("/orders/:id/outbound-confirm", h.ConfirmOutbound)
		r.GET("/orders/:id/assessment", a.GetAssessmentData)
		return r
	}
	call := func(explicit bool, method, path, body string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router(explicit).ServeHTTP(w, req)
		return w.Code
	}

	own := mk(orgA)
	cross := mk(orgB)
	assert.Equal(t, http.StatusOK, call(false, http.MethodGet, "/orders/"+own+"/outbound-photos", ""))
	assert.Equal(t, http.StatusNotFound, call(false, http.MethodGet, "/orders/"+cross+"/outbound-photos", ""))
	assert.Equal(t, http.StatusOK, call(true, http.MethodGet, "/orders/"+mk(orgA)+"/outbound-photos", ""))
	assert.Equal(t, http.StatusNotFound, call(true, http.MethodGet, "/orders/"+mk(orgB)+"/outbound-photos", ""))

	assert.Equal(t, http.StatusOK, call(false, http.MethodGet, "/orders/"+own+"/assessment", ""))
	assert.Equal(t, http.StatusNotFound, call(false, http.MethodGet, "/orders/"+cross+"/assessment", ""))
	assert.Equal(t, http.StatusNotFound, call(true, http.MethodGet, "/orders/"+mk(orgB)+"/assessment", ""))

	assert.Equal(t, http.StatusNotFound, call(false, http.MethodPost, "/orders/"+cross+"/outbound-confirm", `{"confirmed":true}`))
	assert.Equal(t, http.StatusNotFound, call(true, http.MethodPost, "/orders/"+mk(orgB)+"/outbound-confirm", `{"confirmed":true}`))
	ownOff := call(false, http.MethodPost, "/orders/"+mk(orgA)+"/outbound-confirm", `{"confirmed":true}`)
	assert.Equal(t, ownOff, call(true, http.MethodPost, "/orders/"+mk(orgA)+"/outbound-confirm", `{"confirmed":true}`))
}
