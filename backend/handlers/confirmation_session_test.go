package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

// setupConfirmationSessionTest creates a test database and seeds initial data
func setupConfirmationSessionTest(t *testing.T) *gorm.DB {
	db := database.GetDB()

	// Clean up any existing test data
	db.Where("tenant_id = ?", "test-tenant-id").Delete(&models.ConfirmationSession{})
	db.Where("tenant_id = ?", "test-tenant-id").Delete(&models.User{})
	db.Where("tenant_id = ?", "test-tenant-id").Delete(&models.SiteMember{})

	return db
}

// cleanupConfirmationSessionTest removes test data
func cleanupConfirmationSessionTest(db *gorm.DB) {
	db.Where("tenant_id = ?", "test-tenant-id").Delete(&models.ConfirmationSession{})
	db.Where("tenant_id = ?", "test-tenant-id").Delete(&models.User{})
	db.Where("tenant_id = ?", "test-tenant-id").Delete(&models.SiteMember{})
}

func TestConfirmationSessionHandler_Create(t *testing.T) {
	db := setupConfirmationSessionTest(t)
	defer cleanupConfirmationSessionTest(db)

	handler := NewConfirmationSessionHandler()

	t.Run("CreateConfirmationSession_Success_Email", func(t *testing.T) {
		// Create test user
		testUser := models.User{
			ID:       uuid.New().String(),
			IAMSub:   "test-sub-1",
			TenantID: "test-tenant-id",
			OrgID:    "test-org-id",
			Name:     "Test User",
			Email:    "test@example.com",
			Phone:    "13800000000",
			Status:   "pending",
		}
		err := db.Create(&testUser).Error
		assert.NoError(t, err)

		// Prepare request
		reqBody := map[string]interface{}{
			"user_id":          testUser.ID,
			"confirm_type":     "email",
			"confirm_target":   testUser.Email,
			"merchant_id":      "test-merchant-id",
			"action_type":      "site_manager",
			"action_target_id": "test-site-id",
		}

		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/confirmation-sessions", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Set("tenant_id", "test-tenant-id")
		c.Set("org_id", "test-org-id")

		// Execute handler
		handler.Create(c)

		// Assert response
		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, 20100, int(response["code"].(float64)))

		data := response["data"].(map[string]interface{})
		assert.NotEmpty(t, data["id"])
		assert.Equal(t, testUser.ID, data["user_id"])
		assert.Equal(t, "waiting", data["status"])
		assert.NotEmpty(t, data["token"])

		// Verify database record
		var session models.ConfirmationSession
		err = db.Where("id = ?", data["id"]).First(&session).Error
		assert.NoError(t, err)
		assert.Equal(t, "email", session.ConfirmType)
		assert.Equal(t, "test@example.com", session.ConfirmTarget)
		assert.Equal(t, "waiting", session.Status)
		assert.NotEmpty(t, session.Token)
		assert.WithinDuration(t, time.Now().Add(24*time.Hour), session.ExpiresAt, 5*time.Second)
	})

	t.Run("CreateConfirmationSession_UserNotFound", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"user_id":          "non-existent-user",
			"confirm_type":     "email",
			"confirm_target":   "test@example.com",
			"action_type":      "merchant_admin",
			"action_target_id": "test-merchant-id",
		}

		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/confirmation-sessions", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Set("tenant_id", "test-tenant-id")

		handler.Create(c)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, 40400, int(response["code"].(float64)))
	})

	t.Run("CreateConfirmationSession_InvalidConfirmType", func(t *testing.T) {
		// Create test user
		testUser := models.User{
			ID:       uuid.New().String(),
			IAMSub:   "test-sub-2",
			TenantID: "test-tenant-id",
			OrgID:    "test-org-id",
			Name:     "Test User 2",
			Email:    "test2@example.com",
			Status:   "active",
		}
		err := db.Create(&testUser).Error
		assert.NoError(t, err)

		reqBody := map[string]interface{}{
			"user_id":        testUser.ID,
			"confirm_type":   "invalid_type",
			"confirm_target": "test@example.com",
			"action_type":    "site_manager",
		}

		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/confirmation-sessions", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("tenant_id", "test-tenant-id")

		handler.Create(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestConfirmationSessionHandler_Get(t *testing.T) {
	db := setupConfirmationSessionTest(t)
	defer cleanupConfirmationSessionTest(db)

	handler := NewConfirmationSessionHandler()

	// Create test session
	testSession := models.ConfirmationSession{
		ID:             uuid.New().String(),
		TenantID:       "test-tenant-id",
		OrgID:          "test-org-id",
		UserID:         "test-user-id",
		ConfirmType:    "email",
		ConfirmTarget:  "test@example.com",
		ActionType:     "site_manager",
		ActionTargetID: "test-site-id",
		Status:         "waiting",
		Token:          "test-token-12345",
		ExpiresAt:      time.Now().Add(24 * time.Hour),
	}
	err := db.Create(&testSession).Error
	assert.NoError(t, err)

	t.Run("GetConfirmationSession_Success", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/confirmation-sessions/"+testSession.ID, nil)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Set("tenant_id", "test-tenant-id")
		c.Params = []gin.Param{{Key: "id", Value: testSession.ID}}

		handler.Get(c)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, 20000, int(response["code"].(float64)))

		data := response["data"].(map[string]interface{})
		assert.Equal(t, testSession.ID, data["id"])
		assert.Equal(t, "waiting", data["status"])
	})

	t.Run("GetConfirmationSession_NotFound", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/confirmation-sessions/non-existent-id", nil)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Set("tenant_id", "test-tenant-id")
		c.Params = []gin.Param{{Key: "id", Value: "non-existent-id"}}

		handler.Get(c)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestConfirmationSessionHandler_Confirm(t *testing.T) {
	db := setupConfirmationSessionTest(t)
	defer cleanupConfirmationSessionTest(db)

	handler := NewConfirmationSessionHandler()

	t.Run("ConfirmConfirmationSession_Success", func(t *testing.T) {
		// Create test user
		testUser := models.User{
			ID:       uuid.New().String(),
			IAMSub:   "test-sub-confirm",
			TenantID: "test-tenant-id",
			OrgID:    "test-org-id",
			Name:     "Test User Confirm",
			Email:    "confirm@example.com",
			Status:   "pending", // Start with pending status
		}
		err := db.Create(&testUser).Error
		assert.NoError(t, err)

		// Create test session
		testSession := models.ConfirmationSession{
			ID:             uuid.New().String(),
			TenantID:       "test-tenant-id",
			OrgID:          "test-org-id",
			UserID:         testUser.ID,
			ConfirmType:    "email",
			ConfirmTarget:  testUser.Email,
			ActionType:     "site_staff",
			ActionTargetID: "test-site-id",
			Status:         "waiting",
			Token:          "confirm-token-12345",
			ExpiresAt:      time.Now().Add(24 * time.Hour),
		}
		err = db.Create(&testSession).Error
		assert.NoError(t, err)

		// Confirm the session
		req := httptest.NewRequest("POST", "/api/confirmation-sessions/"+testSession.ID+"/confirm?token="+testSession.Token, nil)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Set("tenant_id", "test-tenant-id")
		c.Params = []gin.Param{{Key: "id", Value: testSession.ID}}

		handler.Confirm(c)

		// Assert response
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, 20000, int(response["code"].(float64)))

		// Verify session was updated
		var updatedSession models.ConfirmationSession
		err = db.Where("id = ?", testSession.ID).First(&updatedSession).Error
		assert.NoError(t, err)
		assert.Equal(t, "confirmed", updatedSession.Status)
		assert.NotNil(t, updatedSession.ConfirmedAt)

		// Verify user status was updated to active
		var updatedUser models.User
		err = db.Where("id = ?", testUser.ID).First(&updatedUser).Error
		assert.NoError(t, err)
		assert.Equal(t, "active", updatedUser.Status)

		// Verify site member was created
		var siteMember models.SiteMember
		err = db.Where("user_id = ? AND site_id = ?", testUser.ID, "test-site-id").First(&siteMember).Error
		assert.NoError(t, err)
		assert.Equal(t, "Staff", siteMember.Role)
	})

	t.Run("ConfirmConfirmationSession_InvalidToken", func(t *testing.T) {
		// Create test session
		testSession := models.ConfirmationSession{
			ID:            uuid.New().String(),
			TenantID:      "test-tenant-id",
			UserID:        "test-user-id",
			ConfirmType:   "email",
			ConfirmTarget: "test@example.com",
			ActionType:    "merchant_admin",
			Status:        "waiting",
			Token:         "valid-token",
			ExpiresAt:     time.Now().Add(24 * time.Hour),
		}
		err := db.Create(&testSession).Error
		assert.NoError(t, err)

		// Try to confirm with invalid token
		req := httptest.NewRequest("POST", "/api/confirmation-sessions/"+testSession.ID+"/confirm?token=invalid-token", nil)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Set("tenant_id", "test-tenant-id")
		c.Params = []gin.Param{{Key: "id", Value: testSession.ID}}

		handler.Confirm(c)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("ConfirmConfirmationSession_Expired", func(t *testing.T) {
		// Create expired session
		expiredSession := models.ConfirmationSession{
			ID:            uuid.New().String(),
			TenantID:      "test-tenant-id",
			UserID:        "test-user-id",
			ConfirmType:   "email",
			ConfirmTarget: "test@example.com",
			ActionType:    "site_manager",
			Status:        "waiting",
			Token:         "expired-token",
			ExpiresAt:     time.Now().Add(-1 * time.Hour), // Already expired
		}
		err := db.Create(&expiredSession).Error
		assert.NoError(t, err)

		// Try to confirm expired session
		req := httptest.NewRequest("POST", "/api/confirmation-sessions/"+expiredSession.ID+"/confirm?token="+expiredSession.Token, nil)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		c.Set("tenant_id", "test-tenant-id")
		c.Params = []gin.Param{{Key: "id", Value: expiredSession.ID}}

		handler.Confirm(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, 40002, int(response["code"].(float64)))
	})
}
