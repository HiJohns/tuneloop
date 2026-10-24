package handlers

// #2276 批次 B：乐器媒体读/删在显式模式下隔离。

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

func TestInstrumentMedia_ExplicitScope_2276(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	instA, instB := uuid.NewString(), uuid.NewString()
	batchA, batchB := uuid.NewString(), uuid.NewString()
	for _, x := range []struct{ id, org string }{{instA, orgA}, {instB, orgB}} {
		require.NoError(t, db.Create(&models.Instrument{ID: x.id, TenantID: x.org, OrgID: &x.org, SN: "MED-" + x.id[:6], StockStatus: "available"}).Error)
	}
	mk := func(id, batch, org, inst string) {
		require.NoError(t, db.Create(&models.InstrumentMedia{
			ID: id, TenantID: org, OrgID: org, InstrumentID: &inst, BatchID: batch,
			BatchType: "display", FileName: "a.jpg", FileType: "image", StorageKey: "k/" + id,
		}).Error)
	}
	mk(uuid.NewString(), batchA, orgA, instA)
	mk(uuid.NewString(), batchB, orgB, instB)

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
		r.GET("/instruments/:id/media", GetInstrumentMedia)
		r.DELETE("/instruments/:id/media/:batch_id", DeleteMediaBatch)
		return r
	}
	get := func(explicit bool, inst string) (int, int) {
		r := router(explicit)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/instruments/"+inst+"/media", nil))
		var resp struct {
			Data struct {
				Groups []interface{} `json:"groups"`
			} `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, len(resp.Data.Groups)
	}

	code, n := get(false, instA)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 1, n)
	code, n = get(false, instB)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 0, n, "跨商户媒体列表必须为空")
	code, n = get(true, instA)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 1, n, "on/off 等价（本商户）")
	code, n = get(true, instB)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 0, n, "显式模式跨商户媒体不得可见")

	del := func(explicit bool, inst, batch string) int {
		r := router(explicit)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/instruments/"+inst+"/media/"+batch, nil))
		return w.Code
	}
	assert.Equal(t, http.StatusNotFound, del(false, instB, batchB))
	assert.Equal(t, http.StatusNotFound, del(true, instB, batchB), "显式模式跨商户删除必须 404")
	assert.Equal(t, http.StatusOK, del(true, instA, batchA))
}
