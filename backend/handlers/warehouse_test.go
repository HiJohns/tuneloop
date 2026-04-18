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

func setupWarehouseTables(t *testing.T, db *gorm.DB) error {
	tables := []interface{}{
		&models.Order{},
		&models.OrderStatusHistory{},
		&models.DamageAssessment{},
	}
	for _, table := range tables {
		_ = db.Migrator().DropTable(table)
		if err := db.Migrator().CreateTable(table); err != nil {
			return err
		}
	}
	return nil
}

func TestListWarehouseOrders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)
	if err := setupWarehouseTables(t, db); err != nil {
		t.Fatalf("failed to setup tables: %v", err)
	}

	tenantID := uuid.New().String()
	order := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, UserID: uuid.New().String(),
		InstrumentID: uuid.New().String(), OrgID: uuid.New().String(), Status: "shipped",
	}
	db.Create(&order)

	handler := NewWarehouseHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/warehouse/orders", handler.ListOrders)

	req := httptest.NewRequest("GET", "/api/warehouse/orders", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Code int `json:"code"`
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 20000, response.Code)
	assert.Greater(t, len(response.Data.List), 0)
}

func TestUpdateShipping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)
	if err := setupWarehouseTables(t, db); err != nil {
		t.Fatalf("failed to setup tables: %v", err)
	}

	tenantID := uuid.New().String()
	orderID := uuid.New().String()
	order := models.Order{
		ID: orderID, TenantID: tenantID, UserID: uuid.New().String(), OrgID: uuid.New().String(),
		Status: "preparing", InstrumentID: uuid.New().String(),
	}
	db.Create(&order)

	handler := NewWarehouseHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, uuid.New().String())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/warehouse/orders/:id/shipping", handler.UpdateShipping)

	reqBody := map[string]interface{}{
		"tracking_number": "SF123456",
		"company":         "顺丰",
		"shipped_at":      time.Now(),
	}
	jsonBody, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/warehouse/orders/"+orderID+"/shipping", bytes.NewBuffer(jsonBody))
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
	db.Exec("DELETE FROM orders WHERE tenant_id = ?", tenantID)
	db.Exec("DELETE FROM order_status_history WHERE tenant_id = ?", tenantID)
}
