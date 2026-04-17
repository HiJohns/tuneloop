package handlers

import (
	"context"
	"net/http"
	"encoding/json"
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

func setupAppealTables(t *testing.T, db *gorm.DB) error {
	tables := []interface{}{
		&models.DamageReport{},
		&models.Appeal{},
	}
	for _, table := range tables {
		_ = db.Migrator().DropTable(table)
		if err := db.Migrator().CreateTable(table); err != nil {
			return err
		}
	}
	return nil
}

func TestListAppeals(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)
	if err := setupAppealTables(t, db); err != nil {
		t.Fatalf("failed to setup tables: %v", err)
	}

	tenantID := uuid.New().String()
	userID := uuid.New().String()
	damageID := uuid.New().String()
	orgID := uuid.New().String()
	appeal := models.Appeal{
		ID:             uuid.New().String(),
		TenantID:       tenantID,
		OrgID:          orgID,
		DamageReportID: damageID,
		UserID:         userID,
		AppealReason:   "Test appeal",
		Status:         "pending",
		SubmittedAt:    time.Now(),
	}
	db.Create(&appeal)

	handler := NewAppealHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/merchant/appeals", handler.ListAppeals)

	req := httptest.NewRequest("GET", "/api/merchant/appeals", nil)
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
