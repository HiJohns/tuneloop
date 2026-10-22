package handlers

// #2261（B3 域3）：membership 域（technician_profiles）显式范围就绪守卫。
// 关闭自动租户范围后，列表仍必须按操作者商户隔离。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTechnicianProfilesDomain_ExplicitScopeIsolation_2261(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	shared := uuid.NewString()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: shared, OrgID: orgA, Name: "M-A",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: shared, OrgID: orgB, Name: "M-B",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)

	// #2247 写入锚：profile.tenant_id = 商户 org
	require.NoError(t, db.Create(&models.TechnicianProfile{UserID: uuid.NewString(), TenantID: orgA, Status: "active"}).Error)
	require.NoError(t, db.Create(&models.TechnicianProfile{UserID: uuid.NewString(), TenantID: orgB, Status: "active"}).Error)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.TestActor{TenantID: shared, OrgID: orgA, UserID: uuid.NewString(), Role: "OWNER"}
		ctx := database.ExplicitTenantScopeOnly(actor.InjectContext(c.Request.Context()))
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.GET("/api/technician-profiles", NewTechnicianProfileHandler().List)

	// 显式模式：中间件已通过 database.ExplicitTenantScopeOnly 关闭自动租户范围

	req := httptest.NewRequest(http.MethodGet, "/api/technician-profiles", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Data struct {
			List []struct {
				TenantID string `json:"tenant_id"`
			} `json:"list"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.EqualValues(t, 1, resp.Data.Total, "跨商户技师必须不可见")
	for _, p := range resp.Data.List {
		assert.Equal(t, orgA, p.TenantID)
	}
}
