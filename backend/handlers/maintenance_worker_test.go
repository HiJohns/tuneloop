package handlers

import (
	"bytes"
	"context"
	"encoding/json"
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

func setupTestTables(t *testing.T, db *gorm.DB) error {
	tables := []interface{}{&models.MaintenanceWorker{}}
	for _, table := range tables {
		_ = db.Migrator().DropTable(table)
		if err := db.Migrator().CreateTable(table); err != nil {
			return err
		}
	}
	return nil
}

func TestCreateMaintenanceWorker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)
	if err := setupTestTables(t, db); err != nil {
		t.Fatalf("failed to setup tables: %v", err)
	}

	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	handler := NewMaintenanceWorkerHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/maintenance/workers", handler.CreateWorker)

	body := map[string]interface{}{"name": "张三", "phone": "13800000000"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/maintenance/workers", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 20000, response.Code)
	assert.Equal(t, "张三", response.Data["name"])
	db.Exec("DELETE FROM maintenance_workers WHERE tenant_id = ?", tenantID)
}

func TestListMaintenanceWorkers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)
	if err := setupTestTables(t, db); err != nil {
		t.Fatalf("failed to setup tables: %v", err)
	}

	tenantID := uuid.New().String()
	worker := models.MaintenanceWorker{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: uuid.New().String(),
		Name: "李四", Phone: "13900000000", Status: "active",
	}
	db.Create(&worker)

	handler := NewMaintenanceWorkerHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/maintenance/workers", handler.ListWorkers)

	req := httptest.NewRequest("GET", "/api/maintenance/workers", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Code int `json:"code"`
		Data struct{ List []map[string]interface{} `json:"list"` } `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 20000, response.Code)
	assert.Greater(t, len(response.Data.List), 0)
	db.Exec("DELETE FROM maintenance_workers WHERE tenant_id = ?", tenantID)
}

func TestGetMaintenanceWorker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)
	if err := setupTestTables(t, db); err != nil {
		t.Fatalf("failed to setup tables: %v", err)
	}

	tenantID := uuid.New().String()
	workerID := uuid.New().String()
	worker := models.MaintenanceWorker{
		ID: workerID, TenantID: tenantID, OrgID: uuid.New().String(),
		Name: "王五", Phone: "13700000000", Status: "active",
	}
	db.Create(&worker)

	handler := NewMaintenanceWorkerHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/maintenance/workers/:id", handler.GetWorker)

	req := httptest.NewRequest("GET", "/api/maintenance/workers/"+workerID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 20000, response.Code)
	assert.Equal(t, workerID, response.Data["id"])
	db.Exec("DELETE FROM maintenance_workers WHERE tenant_id = ?", tenantID)
}

func TestDeleteMaintenanceWorker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)
	if err := setupTestTables(t, db); err != nil {
		t.Fatalf("failed to setup tables: %v", err)
	}

	tenantID := uuid.New().String()
	workerID := uuid.New().String()
	worker := models.MaintenanceWorker{
		ID: workerID, TenantID: tenantID, OrgID: uuid.New().String(),
		Name: "赵六", Phone: "13600000000", Status: "active",
	}
	db.Create(&worker)

	handler := NewMaintenanceWorkerHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.DELETE("/api/maintenance/workers/:id", handler.DeleteWorker)

	req := httptest.NewRequest("DELETE", "/api/maintenance/workers/"+workerID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 20000, response.Code)
	assert.Equal(t, workerID, response.Data["id"])
	db.Exec("DELETE FROM maintenance_workers WHERE tenant_id = ?", tenantID)
}
