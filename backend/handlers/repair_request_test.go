package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestCreateRepairRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)
	if err := setupRepairRequestTables(t, db); err != nil {
		t.Fatalf("failed to setup tables: %v", err)
	}

	tenantID := uuid.New().String()
	userID := uuid.New().String()

	handler := NewRepairRequestHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/repair-requests", handler.Create)

	t.Run("happy path — sn provided, auto-create user_instrument", func(t *testing.T) {
		body := `{"sn":"TEST-SN-001","instrument_type":"钢琴","brand":"雅马哈","model":"U1","description":"不响了","photos":["test_photo.jpg"],"video_url":"test_video.mp4","site_id":"` + uuid.New().String() + `"}`
		req := httptest.NewRequest("POST", "/api/repair-requests", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Code int                    `json:"code"`
			Data map[string]interface{} `json:"data"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 20000, resp.Code)

		repairID, ok := resp.Data["id"].(string)
		require.True(t, ok, "response should contain repair request id")
		require.NotEmpty(t, repairID)

		uiID, ok := resp.Data["user_instrument_id"].(string)
		require.True(t, ok, "response should contain user_instrument_id")
		require.NotEmpty(t, uiID, "user_instrument_id must be non-empty uuid")

		var createdUI models.UserInstrument
		err = db.Where("id = ?", uiID).First(&createdUI).Error
		require.NoError(t, err, "user_instrument record should exist")
		assert.Equal(t, "TEST-SN-001", createdUI.SN)
		assert.Equal(t, "雅马哈", createdUI.Brand)
		assert.Equal(t, "U1", createdUI.Model)
	})

	t.Run("error path — no sn and no user_instrument_id", func(t *testing.T) {
		body := `{"description":"test only","site_id":"` + uuid.New().String() + `"}`
		req := httptest.NewRequest("POST", "/api/repair-requests", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 40002, resp.Code)
		assert.Contains(t, resp.Message, "required")
	})

	// --- USER context tests (empty tenantID, no org binding) ---
	// Move User table setup here to share the same db instance with staff tests
	// This avoids test isolation issues caused by the global database.DB variable
	_ = db.Migrator().DropTable(&models.User{})
	_ = db.Migrator().CreateTable(&models.User{})
	db.Exec("ALTER TABLE users ADD COLUMN IF NOT EXISTS iam_sub VARCHAR(255) NOT NULL DEFAULT ''")

	userSub := uuid.New().String()
	localUser := models.User{
		ID:       uuid.New().String(),
		IAMSub:   userSub,
		TenantID: uuid.New().String(),
		OrgID:    uuid.New().String(),
		Status:   "active",
	}
	err = db.Create(&localUser).Error
	require.NoError(t, err)

	usrRouter := gin.New()
	usrRouter.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, "")
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, "")
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userSub)
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, "USER")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	usrRouter.GET("/api/repair-requests", handler.List)
	usrRouter.POST("/api/repair-requests/:id/pay", handler.PayRepairRequest)

	validUIID := uuid.New().String()

	t.Run("List as USER — empty tid, returns user's repair requests", func(t *testing.T) {
		repairID := uuid.New().String()
		req := models.RepairRequest{
			ID:               repairID,
			TenantID:         uuid.New().String(),
			SiteID:           uuid.New().String(),
			UserID:           userSub,
			UserInstrumentID: validUIID,
			Status:           models.RepairReqStatusPendingAssessment,
		}
		err := db.Create(&req).Error
		require.NoError(t, err)

		httpReq := httptest.NewRequest("GET", "/api/repair-requests", nil)
		w := httptest.NewRecorder()
		usrRouter.ServeHTTP(w, httpReq)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Code int `json:"code"`
			Data struct {
				List []map[string]interface{} `json:"list"`
			} `json:"data"`
		}
		err = json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 20000, resp.Code)
		assert.GreaterOrEqual(t, len(resp.Data.List), 1)
	})

	t.Run("PayRepairRequest as USER — empty tid, processes payment", func(t *testing.T) {
		quoteAmount := 500.0
		repairID := uuid.New().String()
		repairReq := models.RepairRequest{
			ID:               repairID,
			TenantID:         uuid.New().String(),
			SiteID:           uuid.New().String(),
			UserID:           userSub,
			UserInstrumentID: validUIID,
			Status:           models.RepairReqStatusPendingPay,
			QuoteAmount:      &quoteAmount,
		}
		err := db.Create(&repairReq).Error
		require.NoError(t, err)

		body := `{}`
		httpReq := httptest.NewRequest("POST", "/api/repair-requests/"+repairID+"/pay", strings.NewReader(body))
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		usrRouter.ServeHTTP(w, httpReq)

		t.Logf("pay response: %s", w.Body.String())
		assert.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		err = json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 20000, resp.Code)

		var updated models.RepairRequest
		db.Where("id = ?", repairID).First(&updated)
		assert.Equal(t, models.RepairReqStatusRepairing, updated.Status)
	})
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
	}
	for _, table := range tables {
		_ = db.Migrator().DropTable(table)
		if err := db.Migrator().CreateTable(table); err != nil {
			t.Fatalf("failed to create table: %v", err)
		}
	}

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
}
