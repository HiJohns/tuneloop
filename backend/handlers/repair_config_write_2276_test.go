package handlers

// #2276：repair 配置写路由显式隔离（按商户 org 锚）。

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

func TestRepairConfigWrite_ExplicitScope_2276(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	orgA, orgB := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{ID: uuid.NewString(), TenantID: orgA, OrgID: orgA, Name: "RC-A", Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString()}).Error)
	require.NoError(t, db.Create(&models.Merchant{ID: uuid.NewString(), TenantID: orgB, OrgID: orgB, Name: "RC-B", Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString()}).Error)

	mkRouter := func(tid, oid string, explicit bool) *gin.Engine {
		actor := testutil.TestActor{TenantID: tid, OrgID: oid, UserID: uuid.NewString(), Role: "OWNER"}
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
			c.Next()
		})
		if explicit {
			r.Use(middleware.ExplicitTenantScope())
		}
		r.PUT("/config/repair/single", SetRepairSetting)
		r.GET("/config/repair/single", GetRepairSetting)
		return r
	}
	call := func(r *gin.Engine, method, path, body string) (int, string) {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var resp struct {
			Data struct {
				Value string `json:"value"`
			} `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp.Data.Value
	}

	key := "repair_inspection_fee"
	body := `{"key":"` + key + `","value":"88.00"}`
	// A 写入（on/off 等价）
	codeA, _ := call(mkRouter(orgA, orgA, false), http.MethodPut, "/config/repair/single", body)
	require.Equal(t, http.StatusOK, codeA)
	codeA2, _ := call(mkRouter(orgA, orgA, true), http.MethodPut, "/config/repair/single", body)
	assert.Equal(t, codeA, codeA2)

	// A 读回、B 读不到 A 的值（显式模式）
	_, valA := call(mkRouter(orgA, orgA, true), http.MethodGet, "/config/repair/single?key="+key, "")
	assert.Equal(t, "88.00", valA)
	_, valB := call(mkRouter(orgB, orgB, true), http.MethodGet, "/config/repair/single?key="+key, "")
	assert.Equal(t, "", valB, "跨商户不得读到 A 的维修配置")
}
