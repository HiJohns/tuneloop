package handlers

import (
	"context"
	"encoding/json"
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
}
