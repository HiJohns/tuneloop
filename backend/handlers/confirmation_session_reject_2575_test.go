package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

// #2575: Reject must never rewrite a session that already ran its action
// (confirmed), already expired, or sits in a terminal non-waiting state
// (failed). The only write point is the conditional UPDATE; RowsAffected == 0
// re-reads and classifies without writing.
//
// Marker lines (`CASE_*` / `CB_*`) are consumed by scripts/verify-2575.sh r2.

const (
	reject2575Tenant = "11111111-1111-1111-1111-111111111111"
	reject2575Org    = "22222222-2222-2222-2222-222222222222"
	reject2575Site   = "33333333-3333-3333-3333-333333333333"
)

// databaseFor2575 returns the test DB handle used by the seed helpers.
func databaseFor2575(t *testing.T) *gorm.DB {
	t.Helper()
	db := database.GetDB()
	require.NotNil(t, db)
	return db
}

// seedConfirmationUser2575 inserts one pending user so executeAction (if it were
// wrongly called) would have something to act on — the tests then assert the user
// is still pending (NO_ACTION_EXECUTED).
func seedConfirmationUser2575(t *testing.T, suffix string) models.User {
	t.Helper()
	db := databaseFor2575(t)
	user := models.User{
		ID:       uuid.New().String(),
		IAMSub:   "sub-2575-" + suffix,
		TenantID: reject2575Tenant,
		OrgID:    reject2575Org,
		Name:     "Reject Guard 2575 " + suffix,
		Email:    fmt.Sprintf("reject-2575-%s@example.com", suffix),
		Status:   "pending",
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

// seedSession2575 inserts a confirmation session in the given state.
func seedSession2575(t *testing.T, userID, status, token string, expiresAt time.Time) models.ConfirmationSession {
	t.Helper()
	db := databaseFor2575(t)
	session := models.ConfirmationSession{
		ID:             uuid.New().String(),
		TenantID:       reject2575Tenant,
		OrgID:          reject2575Org,
		UserID:         userID,
		ConfirmType:    "email",
		ConfirmTarget:  "reject-2575@example.com",
		MerchantID:     "00000000-0000-0000-0000-000000000000",
		ActionType:     "site_staff",
		ActionTargetID: reject2575Site,
		Status:         status,
		Token:          token,
		ExpiresAt:      expiresAt,
	}
	require.NoError(t, db.Create(&session).Error)
	return session
}

func callReject2575(t *testing.T, sessionID, token string) (int, map[string]interface{}) {
	t.Helper()
	handler := NewConfirmationSessionHandler()

	body, err := json.Marshal(map[string]string{"token": token})
	require.NoError(t, err)
	req := httptest.NewRequest("POST", "/api/confirmation-sessions/"+sessionID+"/reject", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyTenantID, reject2575Tenant))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = []gin.Param{{Key: "id", Value: sessionID}}

	handler.Reject(c)

	var resp map[string]interface{}
	if w.Body.Len() > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
	}
	return w.Code, resp
}

func callIAMCallbackReject2575(t *testing.T, sessionID string) string {
	t.Helper()
	handler := NewConfirmationSessionHandler()

	req := httptest.NewRequest("GET", "/api/iam/confirmation-callback?session="+sessionID+"&result=reject", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.IAMConfirmationCallback(c)
	return w.Header().Get("Location")
}

func sessionStatus2575(t *testing.T, sessionID string) string {
	t.Helper()
	db := databaseFor2575(t)
	var session models.ConfirmationSession
	require.NoError(t, db.Where("id = ?", sessionID).First(&session).Error)
	return session.Status
}

// TestRejectConfirmationSession_2575 covers the five handler states plus the
// token mismatch path required by the Accepted Plan (R2).
func TestRejectConfirmationSession_2575(t *testing.T) {
	db := setupConfirmationSessionTest(t)
	defer cleanupConfirmationSessionTest(db)

	t.Run("waiting_stays_rejectable", func(t *testing.T) {
		user := seedConfirmationUser2575(t, "waiting")
		session := seedSession2575(t, user.ID, "waiting", "token-waiting-2575", time.Now().Add(time.Hour))

		status, resp := callReject2575(t, session.ID, session.Token)
		require.Equal(t, http.StatusOK, status)
		require.NotNil(t, resp)
		assert.Equal(t, float64(20000), resp["code"])

		got := sessionStatus2575(t, session.ID)
		assert.Equal(t, "rejected", got)
		t.Logf("CASE_WAITING=20000")
		t.Logf("CASE_WAITING_STATUS=%s", got)
	})

	t.Run("confirmed_is_not_rejectable", func(t *testing.T) {
		user := seedConfirmationUser2575(t, "confirmed")
		session := seedSession2575(t, user.ID, "confirmed", "token-confirmed-2575", time.Now().Add(time.Hour))

		status, resp := callReject2575(t, session.ID, session.Token)
		require.Equal(t, http.StatusConflict, status)
		require.NotNil(t, resp)
		assert.Equal(t, float64(40901), resp["code"])

		got := sessionStatus2575(t, session.ID)
		assert.Equal(t, "confirmed", got)
		t.Logf("CASE_CONFIRMED=40901")
		t.Logf("CASE_CONFIRMED_STATUS=%s", got)
	})

	t.Run("expired_is_rejected_with_40002", func(t *testing.T) {
		user := seedConfirmationUser2575(t, "expired")
		session := seedSession2575(t, user.ID, "waiting", "token-expired-2575", time.Now().Add(-time.Minute))

		status, resp := callReject2575(t, session.ID, session.Token)
		require.Equal(t, http.StatusBadRequest, status)
		require.NotNil(t, resp)
		assert.Equal(t, float64(40002), resp["code"])

		got := sessionStatus2575(t, session.ID)
		assert.Equal(t, "waiting", got)
		t.Logf("CASE_EXPIRED=40002")
		t.Logf("CASE_EXPIRED_STATUS=%s", got)
	})

	t.Run("rejected_is_idempotent", func(t *testing.T) {
		user := seedConfirmationUser2575(t, "rejected")
		session := seedSession2575(t, user.ID, "rejected", "token-rejected-2575", time.Now().Add(time.Hour))

		status, resp := callReject2575(t, session.ID, session.Token)
		require.Equal(t, http.StatusOK, status)
		require.NotNil(t, resp)
		assert.Equal(t, float64(20000), resp["code"])

		got := sessionStatus2575(t, session.ID)
		assert.Equal(t, "rejected", got)
		t.Logf("CASE_REJECTED_IDEMPOTENT=1")
	})

	t.Run("failed_is_not_rejectable", func(t *testing.T) {
		user := seedConfirmationUser2575(t, "failed")
		session := seedSession2575(t, user.ID, "failed", "token-failed-2575", time.Now().Add(time.Hour))

		status, resp := callReject2575(t, session.ID, session.Token)
		require.Equal(t, http.StatusConflict, status)
		require.NotNil(t, resp)
		assert.Equal(t, float64(40901), resp["code"])

		got := sessionStatus2575(t, session.ID)
		assert.Equal(t, "failed", got)
		t.Logf("CASE_FAILED=40901")
	})

	t.Run("wrong_token_is_404", func(t *testing.T) {
		user := seedConfirmationUser2575(t, "badtoken")
		session := seedSession2575(t, user.ID, "waiting", "token-badtoken-2575", time.Now().Add(time.Hour))

		status, _ := callReject2575(t, session.ID, "token-does-not-match")
		require.Equal(t, http.StatusNotFound, status)

		got := sessionStatus2575(t, session.ID)
		assert.Equal(t, "waiting", got)
		t.Logf("CASE_BAD_TOKEN=404")
	})

	// Guarded states must not run the action: the pending user stays pending and
	// no site member row is created.
	t.Run("no_action_executed_for_guarded_sessions", func(t *testing.T) {
		var pendingUsers int64
		require.NoError(t, db.Model(&models.User{}).
			Where("tenant_id = ? AND status = ?", reject2575Tenant, "pending").
			Count(&pendingUsers).Error)
		var members int64
		require.NoError(t, db.Model(&models.SiteMember{}).
			Where("site_id = ?", reject2575Site).
			Count(&members).Error)

		assert.Greater(t, pendingUsers, int64(0))
		assert.Zero(t, members)
		t.Logf("NO_ACTION_EXECUTED=1")
	})
}

// TestIAMCallbackReject_2575 covers the callback reject branch classification.
func TestIAMCallbackReject_2575(t *testing.T) {
	db := setupConfirmationSessionTest(t)
	defer cleanupConfirmationSessionTest(db)

	t.Run("confirmed_session_is_not_flipped", func(t *testing.T) {
		user := seedConfirmationUser2575(t, "cb-confirmed")
		session := seedSession2575(t, user.ID, "confirmed", "token-cb-confirmed-2575", time.Now().Add(time.Hour))

		location := callIAMCallbackReject2575(t, session.ID)
		assert.Contains(t, location, "status=error")
		assert.Contains(t, location, "message=already_confirmed")

		got := sessionStatus2575(t, session.ID)
		assert.Equal(t, "confirmed", got)
		t.Logf("CB_CONFIRMED=already_confirmed")
		t.Logf("CB_CONFIRMED_STATUS=%s", got)
	})

	t.Run("waiting_session_is_rejected", func(t *testing.T) {
		user := seedConfirmationUser2575(t, "cb-waiting")
		session := seedSession2575(t, user.ID, "waiting", "token-cb-waiting-2575", time.Now().Add(time.Hour))

		location := callIAMCallbackReject2575(t, session.ID)
		assert.Contains(t, location, "status=rejected")

		got := sessionStatus2575(t, session.ID)
		assert.Equal(t, "rejected", got)
		t.Logf("CB_WAITING_STATUS=%s", got)
	})

	t.Run("failed_session_is_not_rejectable", func(t *testing.T) {
		user := seedConfirmationUser2575(t, "cb-failed")
		session := seedSession2575(t, user.ID, "failed", "token-cb-failed-2575", time.Now().Add(time.Hour))

		location := callIAMCallbackReject2575(t, session.ID)
		assert.Contains(t, location, "message=not_rejectable")

		got := sessionStatus2575(t, session.ID)
		assert.Equal(t, "failed", got)
		assert.True(t, strings.Contains(location, "not_rejectable"))
		t.Logf("CB_FAILED=not_rejectable")
	})
}
