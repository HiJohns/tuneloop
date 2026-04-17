package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

func setupMaintenanceSessionTables(t *testing.T, db *gorm.DB) error {
	tables := []interface{}{
		&models.MaintenanceWorker{},
		&models.MaintenanceSession{},
		&models.MaintenanceSessionRecord{},
	}
	for _, table := range tables {
		_ = db.Migrator().DropTable(table)
		if err := db.Migrator().CreateTable(table); err != nil {
			return err
		}
	}
	return nil
}

func TestUpdateMaintenanceSessionStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)
	if err := setupMaintenanceSessionTables(t, db); err != nil {
		t.Fatalf("failed to setup tables: %v", err)
	}

	tenantID := uuid.New().String()
	sessionID := uuid.New().String()
	session := models.MaintenanceSession{
		ID: sessionID, TenantID: tenantID, OrgID: uuid.New().String(),
		MaintenanceTicketID: uuid.New().String(), Status: "pending",
	}
	db.Create(&session)

	handler := NewMaintenanceSessionHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/maintenance/sessions/:id/status", handler.UpdateStatus)

	body := map[string]interface{}{"status": "assigned", "comment": "分配技师"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/maintenance/sessions/"+sessionID+"/status", bytes.NewReader(bodyBytes))
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
	assert.Equal(t, "assigned", response.Data["status"])
	db.Exec("DELETE FROM maintenance_sessions WHERE tenant_id = ?", tenantID)
}

func TestStartMaintenanceWork(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)
	if err := setupMaintenanceSessionTables(t, db); err != nil {
		t.Fatalf("failed to setup tables: %v", err)
	}

	tenantID := uuid.New().String()
	sessionID := uuid.New().String()
	session := models.MaintenanceSession{
		ID: sessionID, TenantID: tenantID, OrgID: uuid.New().String(),
		MaintenanceTicketID: uuid.New().String(), Status: "assigned",
	}
	db.Create(&session)

	handler := NewMaintenanceSessionHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/maintenance/sessions/:id/start-work", handler.StartWork)

	body := map[string]interface{}{"instrument_sn": "SN123456", "scan_time": time.Now()}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/maintenance/sessions/"+sessionID+"/start-work", bytes.NewReader(bodyBytes))
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
	assert.Equal(t, "in_progress", response.Data["status"])
	db.Exec("DELETE FROM maintenance_sessions WHERE tenant_id = ?", tenantID)
}

func TestSubmitMaintenanceRecord(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)
	if err := setupMaintenanceSessionTables(t, db); err != nil {
		t.Fatalf("failed to setup tables: %v", err)
	}

	tenantID := uuid.New().String()
	sessionID := uuid.New().String()
	session := models.MaintenanceSession{
		ID: sessionID, TenantID: tenantID, OrgID: uuid.New().String(),
		MaintenanceTicketID: uuid.New().String(), Status: "in_progress",
	}
	db.Create(&session)

	handler := NewMaintenanceSessionHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/maintenance/sessions/:id/records", handler.SubmitRecord)

	body := map[string]interface{}{"type": "comment", "content": "更换琴弦"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/maintenance/sessions/"+sessionID+"/records", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var response struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 20000, response.Code)
	assert.Equal(t, "comment", response.Data["record_type"])
	db.Exec("DELETE FROM maintenance_sessions WHERE tenant_id = ?", tenantID)
	db.Exec("DELETE FROM maintenance_session_records WHERE tenant_id = ?", tenantID)
}

func TestInspectMaintenanceSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)
	if err := setupMaintenanceSessionTables(t, db); err != nil {
		t.Fatalf("failed to setup tables: %v", err)
	}

	tenantID := uuid.New().String()
	sessionID := uuid.New().String()
	session := models.MaintenanceSession{
		ID: sessionID, TenantID: tenantID, OrgID: uuid.New().String(),
		MaintenanceTicketID: uuid.New().String(), Status: "completed",
	}
	db.Create(&session)

	handler := NewMaintenanceSessionHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/maintenance/sessions/:id/inspect", handler.Inspect)

	body := map[string]interface{}{"result": "passed", "comment": "验收通过"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/maintenance/sessions/"+sessionID+"/inspect", bytes.NewReader(bodyBytes))
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
	assert.Equal(t, "passed", response.Data["result"])
	db.Exec("DELETE FROM maintenance_sessions WHERE tenant_id = ?", tenantID)
}
