package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

func setupRepairRequestTables(t *testing.T, db *gorm.DB) error {
	tables := []interface{}{
		&models.RepairRequest{},
		&models.UserInstrument{},
	}
	for _, table := range tables {
		_ = db.Migrator().DropTable(table)
		if err := db.Migrator().CreateTable(table); err != nil {
			return err
		}
	}
	return nil
}

func TestGetRepairRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)

	tables := []interface{}{
		&models.RepairRequest{},
		&models.UserInstrument{},
		&models.Site{},
		&models.Tenant{},
		&models.User{},
	}
	for _, table := range tables {
		_ = db.Migrator().DropTable(table)
		if err := db.Migrator().CreateTable(table); err != nil {
			t.Fatalf("failed to create table: %v", err)
		}
	}
	db.Exec("ALTER TABLE users ADD COLUMN IF NOT EXISTS iam_sub VARCHAR(255) NOT NULL DEFAULT ''")

	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	userID := uuid.New().String()

	site := models.Site{
		ID:       uuid.New().String(),
		TenantID: tenantID,
		OrgID:    orgID,
		Name:     "测试网点",
	}
	require.NoError(t, db.Create(&site).Error)

	tenant := models.Tenant{
		ID:   tenantID,
		Name: "测试商户",
	}
	require.NoError(t, db.Create(&tenant).Error)

	localUser := models.User{
		ID:       uuid.New().String(),
		IAMSub:   userID,
		TenantID: tenantID,
		OrgID:    orgID,
		Name:     "张三",
		Status:   "active",
	}
	require.NoError(t, db.Create(&localUser).Error)

	ui := models.UserInstrument{
		ID:             uuid.New().String(),
		UserID:         userID,
		SN:             "SN-TEST-001",
		InstrumentType: "钢琴",
		Brand:          "雅马哈",
		Model:          "U1",
	}
	require.NoError(t, db.Create(&ui).Error)

	req := models.RepairRequest{
		ID:               uuid.New().String(),
		TenantID:         tenantID,
		SiteID:           site.ID,
		UserID:           userID,
		UserInstrumentID: ui.ID,
		Status:           models.RepairReqStatusPendingAssessment,
		Description:      "不响了",
	}
	require.NoError(t, db.Create(&req).Error)

	handler := NewRepairRequestHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/repair-requests/:id", handler.Get)

	httpReq := httptest.NewRequest("GET", "/api/repair-requests/"+req.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 20000, resp.Code)

	assert.Equal(t, req.ID, resp.Data["id"])
	assert.Equal(t, "SN-TEST-001", resp.Data["instrument_sn"])
	assert.Equal(t, "钢琴", resp.Data["instrument_type"])
	assert.Equal(t, "雅马哈", resp.Data["brand"])
	assert.Equal(t, "U1", resp.Data["model"])
	assert.Equal(t, "测试网点", resp.Data["site_name"])
	assert.Equal(t, "测试商户", resp.Data["merchant_name"])
	assert.Equal(t, "张三", resp.Data["reporter_name"])
	assert.Equal(t, "pending_assessment", resp.Data["status"])
	assert.Equal(t, "不响了", resp.Data["description"])
}

func TestListRepairRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)

	tables := []interface{}{
		&models.RepairRequest{},
		&models.UserInstrument{},
		&models.Site{},
		&models.Tenant{},
		&models.User{},
	}
	for _, table := range tables {
		_ = db.Migrator().DropTable(table)
		if err := db.Migrator().CreateTable(table); err != nil {
			t.Fatalf("failed to create table: %v", err)
		}
	}
	// #2056: iam_sub 带 -:migration 标签，CreateTable 不建列，手动补（镜像 setupMockIAMAndDB）
	db.Exec("ALTER TABLE users ADD COLUMN IF NOT EXISTS iam_sub VARCHAR(255) NOT NULL DEFAULT ''")

	tenantID := uuid.New().String()
	orgID := uuid.New().String()

	site := models.Site{
		ID:       uuid.New().String(),
		TenantID: tenantID,
		OrgID:    orgID,
		Name:     "测试网点",
	}
	require.NoError(t, db.Create(&site).Error)

	tenant := models.Tenant{
		ID:   tenantID,
		Name: "测试商户",
	}
	require.NoError(t, db.Create(&tenant).Error)

	ui := models.UserInstrument{
		ID:             uuid.New().String(),
		UserID:         "test-customer-id",
		SN:             "SN-TEST-002",
		InstrumentType: "小提琴",
		Brand:          "星河",
		Model:          "V-1",
	}
	require.NoError(t, db.Create(&ui).Error)

	for i := 0; i < 2; i++ {
		r := models.RepairRequest{
			ID:               uuid.New().String(),
			TenantID:         tenantID,
			SiteID:           site.ID,
			UserID:           "test-customer-id",
			UserInstrumentID: ui.ID,
			Status:           models.RepairReqStatusPendingAssessment,
			Description:      fmt.Sprintf("问题 %d", i+1),
		}
		require.NoError(t, db.Create(&r).Error)
	}

	handler := NewRepairRequestHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		// #2056: 未注入 role/user_id 会走员工分支（resolveOperatorSiteMemberships→空集）恒失败
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, "USER")
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "test-customer-id")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/repair-requests", handler.List)

	httpReq := httptest.NewRequest("GET", "/api/repair-requests", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 20000, resp.Code)

	listRaw, ok := resp.Data["list"].([]interface{})
	require.True(t, ok, "response should contain list")
	assert.GreaterOrEqual(t, len(listRaw), 2)

	for _, itemRaw := range listRaw {
		item := itemRaw.(map[string]interface{})
		assert.Equal(t, "SN-TEST-002", item["instrument_sn"])
		assert.Equal(t, "小提琴", item["instrument_type"])
		assert.Equal(t, "星河", item["brand"])
		assert.Equal(t, "V-1", item["model"])
		assert.Equal(t, "测试网点", item["site_name"])
		assert.Equal(t, "测试商户", item["merchant_name"])
	}

	// #2056 负向断言：员工分支按网点隔离——无站点归属的员工必须看到空集（不得回退全量）。
	// 注：USER 分支是**身份域**（仅按 user_id 过滤，跨租户仍属本人），故隔离性须在员工分支验证。
	other := gin.New()
	other.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, uuid.New().String())
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, "STAFF")
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, uuid.New().String())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	other.GET("/api/repair-requests", handler.List)
	w2 := httptest.NewRecorder()
	other.ServeHTTP(w2, httptest.NewRequest("GET", "/api/repair-requests", nil))
	assert.Equal(t, http.StatusOK, w2.Code)
	var resp2 struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp2))
	assert.Equal(t, 20000, resp2.Code)
	if l2, ok2 := resp2.Data["list"].([]interface{}); ok2 {
		assert.Len(t, l2, 0, "无站点归属的员工不得看到任何报修")
	}
}
