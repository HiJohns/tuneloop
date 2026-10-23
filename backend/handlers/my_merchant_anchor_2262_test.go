package handlers

// #2262 B4：GetMyMerchant 商户身份锚 = 商户 org（禁止 OR 回退共享 tenant）。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMyMerchant_SharedTenantOrgAnchor_2262(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	shared := uuid.NewString()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	// 先插入 B（父实现 OR 查询会命中它）
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: shared, OrgID: orgB, Name: "B-2262",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)
	merchantA := uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: merchantA, TenantID: shared, OrgID: orgA, Name: "A-2262",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)

	call := func(t *testing.T, tid, oid string) (int, map[string]interface{}) {
		t.Helper()
		actor := testutil.TestActor{TenantID: tid, OrgID: oid, UserID: uuid.NewString(), Role: "OWNER"}
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
			c.Next()
		})
		r.GET("/api/admin/my-merchant", GetMyMerchant)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/admin/my-merchant", nil))
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp
	}

	code, resp := call(t, shared, orgA)
	require.Equal(t, http.StatusOK, code, "%v", resp)
	data, _ := resp["data"].(map[string]interface{})
	assert.Equal(t, merchantA, data["id"], "必须返回操作者 org 对应商户（不得 OR 到共享 tenant 的其他商户）")

	code, _ = call(t, shared, uuid.NewString())
	assert.Equal(t, http.StatusNotFound, code, "无法解析商户时必须 404（不猜测）")
}
