package handlers

// #2276 批次 B：乐器更新/批量删除在显式模式下隔离。

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

func TestInstrumentsWrite_ExplicitScope_2276(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()

	mk := func(org, sn string) string {
		id := uuid.NewString()
		require.NoError(t, db.Create(&models.Instrument{ID: id, TenantID: org, OrgID: &org, SN: sn, StockStatus: "available"}).Error)
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
		r.PUT("/instruments/:id", UpdateInstrument)
		r.DELETE("/instruments/batch", BatchDeleteInstruments)
		return r
	}
	do := func(explicit bool, method, path, body string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router(explicit).ServeHTTP(w, req)
		return w.Code
	}

	// 更新：本商户 200，跨商户 404，on/off 等价
	ownA := mk(orgA, "UPD-A")
	crossB := mk(orgB, "UPD-B")
	assert.Equal(t, http.StatusOK, do(false, http.MethodPut, "/instruments/"+ownA, `{"name":"renamed"}`))
	assert.Equal(t, http.StatusNotFound, do(false, http.MethodPut, "/instruments/"+crossB, `{"name":"x"}`))
	assert.Equal(t, http.StatusOK, do(true, http.MethodPut, "/instruments/"+mk(orgA, "UPD-A2"), `{"name":"renamed2"}`))
	assert.Equal(t, http.StatusNotFound, do(true, http.MethodPut, "/instruments/"+mk(orgB, "UPD-B2"), `{"name":"x"}`))

	// 批量删除：跨商户不删；本商户删除成功
	batchB := mk(orgB, "BATCH-B")
	code := do(true, http.MethodDelete, "/instruments/batch", `{"ids":["`+batchB+`"]}`)
	assert.Contains(t, []int{http.StatusOK, http.StatusForbidden, http.StatusNotFound}, code)
	var cnt int64
	require.NoError(t, db.Model(&models.Instrument{}).Where("id = ?", batchB).Count(&cnt).Error)
	assert.EqualValues(t, 1, cnt, "跨商户乐器不得被删除")
}
