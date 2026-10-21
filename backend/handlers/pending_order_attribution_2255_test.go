package handlers

// #2255（P3 批次2，S1/S4）：待提交订单（缓存表）的 tenant 必须是**数据归属**
// （乐器所属商户 org），不得沿用操作者/用户缓存 tenant。

import (
	"bytes"
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

func TestCreatePendingOrder_TenantFromInstrument_2255(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	merchantOrg := uuid.NewString()
	sharedTenant := uuid.NewString()
	// 顾客本地缓存行：零租户（自注册形态）
	iamSub := uuid.NewString()
	userID := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: iamSub, Name: "顾客2255", Phone: "13900002255",
		TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID:    "00000000-0000-0000-0000-000000000000", Status: "active",
	}).Error)

	// 乐器归属：商户 org
	instID := uuid.NewString()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: merchantOrg, OrgID: &merchantOrg,
		SN: "SN-PEND-2255", StockStatus: "available",
	}).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(testutil.TestActor{
			TenantID: "", OrgID: "", UserID: iamSub, Role: "USER",
		}.InjectContext(c.Request.Context()))
		c.Next()
	})
	router.POST("/user/pending-orders", CreatePendingOrder)

	payload, _ := json.Marshal(map[string]interface{}{
		"payload": map[string]interface{}{
			"items":            []map[string]interface{}{{"instrument_id": instID, "rent_days": 30}},
			"delivery_address": "北京市朝阳区 XX 路 1 号",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/user/pending-orders", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var po models.PendingOrder
	require.NoError(t, db.Where("user_id = ?", userID).Order("created_at DESC").First(&po).Error)
	assert.Equal(t, merchantOrg, po.TenantID, "缓存订单必须落乐器所属商户 org（数据归属）")
	assert.NotEqual(t, sharedTenant, po.TenantID)
	assert.NotEqual(t, "00000000-0000-0000-0000-000000000000", po.TenantID)
}

func TestCreatePendingOrder_NoInstrument_FallsBackToPlatformZero_2255(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	sharedTenant := uuid.NewString()
	iamSub := uuid.NewString()
	userID := uuid.NewString()
	// 用户缓存为共享 tenant（历史脏值）：不得作为缓存订单锚
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: iamSub, Name: "顾客2255b", Phone: "13900002256",
		TenantID: sharedTenant, OrgID: sharedTenant, Status: "active",
	}).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(testutil.TestActor{
			TenantID: "", OrgID: "", UserID: iamSub, Role: "USER",
		}.InjectContext(c.Request.Context()))
		c.Next()
	})
	router.POST("/user/pending-orders", CreatePendingOrder)

	payload, _ := json.Marshal(map[string]interface{}{
		"payload": map[string]interface{}{"items": []interface{}{}},
	})
	req := httptest.NewRequest(http.MethodPost, "/user/pending-orders", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var po models.PendingOrder
	require.NoError(t, db.Where("user_id = ?", userID).Order("created_at DESC").First(&po).Error)
	assert.Equal(t, "00000000-0000-0000-0000-000000000000", po.TenantID,
		"无法解析乐器时应落平台零 UUID，不得沿用用户缓存（S1）")
}
