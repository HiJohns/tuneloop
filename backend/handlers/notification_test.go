package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

// #1742: notifications and damage-report queries key on the LOCAL users.id,
// while the JWT sub is the IAM-side id. These tests lock the mapping:
// write side resolves iam_sub → users.id, read side must see the rows.
func setupNotificationTest(t *testing.T) *gin.Engine {
	cleanup := setupMockIAMAndDB(t)
	t.Cleanup(cleanup)
	db := database.GetDB()
	for _, m := range []interface{}{
		&models.Notification{},
		&models.User{},
	} {
		_ = db.Migrator().DropTable(m)
		require.NoError(t, db.Migrator().CreateTable(m))
	}
	db.Exec("ALTER TABLE users ADD COLUMN IF NOT EXISTS iam_sub VARCHAR(255) NOT NULL DEFAULT ''")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, notifyTenantUUID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, notifyJWTSub)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/user/notifications", GetNotifications)
	router.GET("/api/user/notifications/unread-count", GetUnreadCount)
	router.POST("/api/user/notifications/:id/read", MarkNotificationRead)
	router.POST("/api/user/notifications/read-all", MarkAllNotificationsRead)
	return router
}

// createNotifyUser inserts a local user whose iam_sub equals the JWT sub
// used by the router above; returns the local id to key business rows on.
func createNotifyUser(t *testing.T, iamSub string) string {
	t.Helper()
	db := database.GetDB()
	localID := uuid.New().String()
	require.NoError(t, db.Exec(`INSERT INTO users (id, iam_sub, tenant_id, org_id, name, email, phone, credit_score, is_shadow, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 600, false, NOW(), NOW())`,
		localID, iamSub, notifyTenantUUID, notifyTenantUUID, "Notify User", "notify@example.com", "13700137000").Error)
	return localID
}

var (
	notifyJWTSub     = uuid.New().String()
	notifyTenantUUID = uuid.New().String()
)

func TestGetNotifications_MapsJWTSubToLocalID(t *testing.T) {
	router := setupNotificationTest(t)
	db := database.GetDB()

	localID := createNotifyUser(t, notifyJWTSub)
	require.NoError(t, db.Create(&models.Notification{
		TenantID: notifyTenantUUID, OrgID: notifyTenantUUID, UserID: localID, Type: "order",
		RefID: uuid.New().String(), Title: "n1", Status: "unread",
	}).Error)
	require.NoError(t, db.Create(&models.Notification{
		TenantID: notifyTenantUUID, OrgID: notifyTenantUUID, UserID: uuid.New().String(), Type: "order",
		RefID: uuid.New().String(), Title: "other-user", Status: "unread",
	}).Error)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/user/notifications", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				Title string `json:"title"`
			} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Len(t, resp.Data.List, 1, "only the local-user's notification is visible")
	require.Equal(t, "n1", resp.Data.List[0].Title)
}

func TestGetNotifications_NoLocalUser_ReturnsEmptyListNotError(t *testing.T) {
	router := setupNotificationTest(t)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/user/notifications", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Contains(t, w.Body.String(), `"list":[]`, "list must serialize as [], never null")
}

func TestGetUnreadCount_MapsJWTSubToLocalID(t *testing.T) {
	router := setupNotificationTest(t)
	db := database.GetDB()

	localID := createNotifyUser(t, notifyJWTSub)
	for _, status := range []string{"unread", "unread", "read"} {
		require.NoError(t, db.Create(&models.Notification{
			TenantID: notifyTenantUUID, OrgID: notifyTenantUUID, UserID: localID, Type: "order",
			Title: "c-" + status, RefID: uuid.New().String(), Status: status,
		}).Error)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/user/notifications/unread-count", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Count int64 `json:"count"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, int64(2), resp.Data.Count)
}

func TestMarkAllNotificationsRead_NoLocalUser_IdempotentSuccess(t *testing.T) {
	router := setupNotificationTest(t)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/api/user/notifications/read-all", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"code":20000`)
}

func TestMarkNotificationRead_ScopedToLocalUser(t *testing.T) {
	router := setupNotificationTest(t)
	db := database.GetDB()

	localID := createNotifyUser(t, notifyJWTSub)
	notif := models.Notification{
		TenantID: notifyTenantUUID, OrgID: notifyTenantUUID, UserID: localID, Type: "order",
		RefID: uuid.New().String(), Title: "mine", Status: "unread",
	}
	require.NoError(t, db.Create(&notif).Error)
	other := models.Notification{
		TenantID: notifyTenantUUID, OrgID: notifyTenantUUID, UserID: uuid.New().String(), Type: "order",
		RefID: uuid.New().String(), Title: "theirs", Status: "unread",
	}
	require.NoError(t, db.Create(&other).Error)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/api/user/notifications/"+notif.ID+"/read", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var after models.Notification
	require.NoError(t, db.Where("id = ?", notif.ID).First(&after).Error)
	require.Equal(t, "read", after.Status)

	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, httptest.NewRequest("POST", "/api/user/notifications/"+other.ID+"/read", nil))
	require.Equal(t, http.StatusOK, w2.Code, "marking another user's notification still returns success")

	var otherAfter models.Notification
	require.NoError(t, db.Where("id = ?", other.ID).First(&otherAfter).Error)
	require.Equal(t, "unread", otherAfter.Status, "must NOT mark another user's notification read")
}

func TestLocalUserID_ReadOnlyNoShadowCreation(t *testing.T) {
	setupMockIAMAndDB(t)
	db := database.GetDB()

	iamSub := "sub-no-local-row"

	ctx := context.WithValue(context.Background(), middleware.ContextKeyUserID, iamSub)

	got, err := middleware.LocalUserID(ctx, db)
	require.NoError(t, err, "missing local user is a normal condition, not an error")
	require.Empty(t, got)

	var count int64
	db.Model(&models.User{}).Where("iam_sub = ?", iamSub).Count(&count)
	require.Zero(t, count, "LocalUserID must never create a shadow user row")
}
