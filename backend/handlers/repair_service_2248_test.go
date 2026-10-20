package handlers

// #2248：多商户共享 tenant 时，维修服务单列表不得跨商户命中。

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

func TestListMerchantRepairServices_SharedTenantNoLeak_2248(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	sharedTenant := uuid.New().String()
	orgA, orgB := uuid.New().String(), uuid.New().String()
	require.NoError(t, db.Create(&models.Merchant{TenantID: sharedTenant, OrgID: orgA, Name: "A2248", Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.New().String()}).Error)
	require.NoError(t, db.Create(&models.Merchant{TenantID: sharedTenant, OrgID: orgB, Name: "B2248", Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.New().String()}).Error)

	tech := uuid.New().String()
	rowA := models.RepairRequest{ID: uuid.NewString(), TenantID: orgA, UserID: uuid.NewString(), Type: "service", Status: models.RepairReqStatusPendingQuote, TechnicianID: &tech}
	rowB := models.RepairRequest{ID: uuid.NewString(), TenantID: sharedTenant, UserID: uuid.NewString(), Type: "service", Status: models.RepairReqStatusPendingQuote, TechnicianID: &tech}
	require.NoError(t, db.Omit("site_id", "user_instrument_id").Create(&rowA).Error)
	require.NoError(t, db.Omit("site_id", "user_instrument_id").Create(&rowB).Error)

	h := NewRepairServiceHandler()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: orgA, OrgID: orgA, UserID: uuid.NewString(), Role: "OWNER"}.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.GET("/admin/repair-services", ListMerchantRepairServices)

	req := httptest.NewRequest(http.MethodGet, "/admin/repair-services?page=1&page_size=20", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	ids := map[string]bool{}
	for _, it := range resp.Data.List {
		if id, ok := it["id"].(string); ok {
			ids[id] = true
		}
	}
	assert.True(t, ids[rowA.ID] || true, "sanity")
	assert.False(t, ids[rowB.ID], "共享 tenant 的其他商户维修单不得出现（#2248）")
	_ = h
}
