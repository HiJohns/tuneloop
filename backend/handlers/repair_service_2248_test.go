package handlers

// #2248：多商户共享 tenant 时，维修服务单列表不得跨商户命中。

import (
	"bytes"
	"context"
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
	assert.True(t, ids[rowA.ID], "商户 A 自己的服务单必须可见")
	assert.False(t, ids[rowB.ID], "共享 tenant 的其他商户维修单不得出现（#2248）")
	_ = h
}

// TestCreateRepairService_WriteAnchoredToMerchantOrg_2248：
// 写入链必须把服务单落到商户 org（不得回落共享 tenant），且随后本商户列表可见、
// staff 操作校验通过——即“创建 → 列表可见 → 员工可操作”全链路。
func TestCreateRepairService_WriteAnchoredToMerchantOrg_2248(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	sharedTenant := uuid.New().String()
	orgA := uuid.New().String()
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.NewString(), TenantID: sharedTenant, OrgID: orgA, Name: "A2248W",
		Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
	}).Error)

	techLocal := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: techLocal, IAMSub: uuid.NewString(), Name: "维修师", Phone: "13900007711",
		TenantID: orgA, OrgID: orgA, Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.TechnicianProfile{
		ID: uuid.NewString(), UserID: techLocal, TenantID: orgA, Status: "active",
	}).Error)

	customerID := uuid.NewString()
	createRouter := gin.New()
	createRouter.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(testutil.TestActor{
			TenantID: "", OrgID: "", UserID: customerID, Role: "USER",
		}.InjectContext(c.Request.Context()))
		c.Next()
	})
	createRouter.POST("/user/repair-services", (&RepairServiceHandler{}).Create)

	payload, _ := json.Marshal(map[string]interface{}{
		"description":   "琴键异响（#2248 写入锚测试）",
		"technician_id": techLocal,
	})
	req := httptest.NewRequest(http.MethodPost, "/user/repair-services", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	createRouter.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.NotEmpty(t, created.Data.ID)

	var stored models.RepairRequest
	require.NoError(t, db.Where("id = ?", created.Data.ID).First(&stored).Error)
	assert.Equal(t, orgA, stored.TenantID, "新服务单必须落商户 org（不得回落共享 tenant）")
	assert.NotEqual(t, sharedTenant, stored.TenantID)

	// 本商户列表可见
	listRouter := gin.New()
	listRouter.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(testutil.TestActor{
			TenantID: orgA, OrgID: orgA, UserID: uuid.NewString(), Role: "OWNER",
		}.InjectContext(c.Request.Context()))
		c.Next()
	})
	listRouter.GET("/admin/repair-services", ListMerchantRepairServices)
	lreq := httptest.NewRequest(http.MethodGet, "/admin/repair-services?page=1&page_size=20", nil)
	lw := httptest.NewRecorder()
	listRouter.ServeHTTP(lw, lreq)
	require.Equal(t, http.StatusOK, lw.Code, lw.Body.String())
	var listResp struct {
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(lw.Body.Bytes(), &listResp))
	var visible bool
	for _, it := range listResp.Data.List {
		if id, ok := it["id"].(string); ok && id == stored.ID {
			visible = true
		}
	}
	assert.True(t, visible, "新建服务单必须在本商户列表可见")

	// 员工（商户 org 上下文）可操作该单
	staffCtx := testutil.TestActor{TenantID: orgA, OrgID: orgA, UserID: uuid.NewString(), Role: "ADMIN"}.InjectContext(context.Background())
	assert.True(t, repairServiceStaffAllowed(&stored, staffCtx), "本商户员工必须可操作该单")
}
