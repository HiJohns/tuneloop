package handlers

// #2576：确认会话 Confirm / IAM 回调 accept 的 status 守卫。
//
// 缺陷形态（修前）：`Confirm` 只有「过期」与「confirmed 幂等」两道守卫，随后无条件
// 整行 `db.Save(&session)` + `executeAction` —— `rejected` / `expired` 会话会被静默改写
// 为 `confirmed` 并执行授权动作；回调 `accept` 同样会把 `rejected` 翻回 `confirmed`，
// 并对已 `confirmed` 的会话重复执行动作。
//
// 断言面向「状态未被改写 + 动作未执行（site_members 增量 0 / IAM 调用 0 次）」，而不是
// 只看 HTTP 码——只看码无法证明副作用为零（本缺陷的危害正是「返回成功后写了不该写的」）。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

const (
	tenant2576 = "2576aaaa-0000-4000-8000-000000000001"
	site2576   = "2576bbbb-0000-4000-8000-000000000002"
	org2576    = "2576cccc-0000-4000-8000-000000000003"
)

// setupConfirm2576 建隔离租户/网点并清掉本单数据，返回处理后的 DB 句柄。
func setupConfirm2576(t *testing.T) {
	t.Helper()
	db := database.GetDB()
	testfixtures.SeedTenantFK(t, db, tenant2576)
	testfixtures.SeedSiteFK(t, db, site2576, tenant2576)

	db.Where("tenant_id = ?", tenant2576).Delete(&models.ConfirmationSession{})
	db.Where("tenant_id = ?", tenant2576).Delete(&models.SiteMember{})
	db.Where("tenant_id = ?", tenant2576).Delete(&models.Merchant{})
	db.Where("tenant_id = ?", tenant2576).Delete(&models.User{})
	t.Cleanup(func() { cleanupConfirm2576() })
}

func cleanupConfirm2576() {
	db := database.GetDB()
	db.Where("tenant_id = ?", tenant2576).Delete(&models.ConfirmationSession{})
	db.Where("tenant_id = ?", tenant2576).Delete(&models.SiteMember{})
	db.Where("tenant_id = ?", tenant2576).Delete(&models.Merchant{})
	db.Where("tenant_id = ?", tenant2576).Delete(&models.User{})
}

// createUser2576 建一个本地用户；executeAction 成功时会把 pending → active。
func createUser2576(t *testing.T, status string) string {
	t.Helper()
	user := models.User{
		ID:       uuid.NewString(),
		IAMSub:   "sub-2576-" + uuid.NewString()[:8],
		TenantID: tenant2576,
		OrgID:    tenant2576,
		Name:     "确认会话用例用户",
		Email:    "confirm2576-" + uuid.NewString()[:8] + "@example.com",
		Status:   status,
	}
	require.NoError(t, database.GetDB().Create(&user).Error)
	return user.ID
}

// createSession2576 落一条确认会话；status 必须由调用方显式给出（GORM 默认 waiting
// 会让「未知状态」用例退化为 waiting，属损坏夹具）。
func createSession2576(t *testing.T, f sessionFixture2576) models.ConfirmationSession {
	t.Helper()
	if f.Status == "" {
		t.Fatalf("fixture must set status explicitly (GORM default is waiting)")
	}
	expires := f.ExpiresAt
	if expires.IsZero() {
		expires = time.Now().Add(24 * time.Hour)
	}
	merchantID := f.MerchantID
	if merchantID == "" {
		// merchant_id 是 uuid 列，空串会被 Postgres 拒绝（22P02）；用零 UUID 表示「无商户」。
		merchantID = "00000000-0000-0000-0000-000000000000"
	}
	session := models.ConfirmationSession{
		ID:             uuid.NewString(),
		TenantID:       tenant2576,
		OrgID:          org2576,
		UserID:         f.UserID,
		ConfirmType:    "email",
		ConfirmTarget:  "target2576@example.com",
		MerchantID:     merchantID,
		ActionType:     f.ActionType,
		ActionTargetID: site2576,
		// iam_session_id 必须是纯 UUID 形态：回调用 `iam_session_id = ? OR id = ?`
		// 一个参数同时匹配 varchar 列与 uuid 列，非 UUID 字面量会在 uuid 侧报 22P02。
		IAMSessionID: uuid.NewString(),
		Status:       f.Status,
		Token:        "tok-" + uuid.NewString(),
		ExpiresAt:    expires,
	}
	require.NoError(t, database.GetDB().Create(&session).Error)
	return session
}

type sessionFixture2576 struct {
	UserID     string
	Status     string
	ActionType string
	MerchantID string
	ExpiresAt  time.Time
}

// confirmWith2576 以租户上下文调用 POST /api/confirmation-sessions/:id/confirm。
func confirmWith2576(t *testing.T, sessionID, token string) *httptest.ResponseRecorder {
	t.Helper()
	handler := NewConfirmationSessionHandler()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyTenantID, tenant2576)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.POST("/api/confirmation-sessions/:id/confirm", handler.Confirm)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/confirmation-sessions/"+sessionID+"/confirm?token="+token, nil)
	r.ServeHTTP(w, req)
	return w
}

func responseCode2576(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "body=%s", w.Body.String())
	return body.Code
}

func sessionStatus2576(t *testing.T, id string) string {
	t.Helper()
	var row models.ConfirmationSession
	require.NoError(t, database.GetDB().Where("id = ?", id).First(&row).Error)
	return row.Status
}

func siteMemberCount2576(t *testing.T, userID string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, database.GetDB().Model(&models.SiteMember{}).
		Where("user_id = ? AND site_id = ?", userID, site2576).Count(&n).Error)
	return n
}

func createMerchant2576(t *testing.T) string {
	t.Helper()
	merchant := models.Merchant{
		ID:       uuid.NewString(),
		TenantID: tenant2576,
		OrgID:    org2576,
		Name:     "确认会话用例商户",
		Code:     "M2576" + uuid.NewString()[:8],
		AdminUID: uuid.NewString(),
		Status:   "active",
	}
	require.NoError(t, database.GetDB().Create(&merchant).Error)
	return merchant.ID
}

// TestConfirmSessionStatusGuard2576 覆盖 Confirm 的状态矩阵（含负例与零副作用断言）。
func TestConfirmSessionStatusGuard2576(t *testing.T) {
	setupConfirm2576(t)

	t.Run("Confirm2576_Waiting", func(t *testing.T) {
		userID := createUser2576(t, "pending")
		session := createSession2576(t, sessionFixture2576{UserID: userID, Status: "waiting", ActionType: "site_staff"})

		w := confirmWith2576(t, session.ID, session.Token)
		code := responseCode2576(t, w)
		status := sessionStatus2576(t, session.ID)
		action := siteMemberCount2576(t, userID)
		t.Logf("CASE_WAITING=%d", code)
		t.Logf("CASE_WAITING_STATUS=%s", status)
		t.Logf("CASE_WAITING_ACTION=%d", action)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, 20000, code)
		assert.Equal(t, "confirmed", status)
		assert.EqualValues(t, 1, action)
	})

	t.Run("Confirm2576_FailedRetry", func(t *testing.T) {
		userID := createUser2576(t, "pending")
		session := createSession2576(t, sessionFixture2576{UserID: userID, Status: "failed", ActionType: "site_staff"})

		w := confirmWith2576(t, session.ID, session.Token)
		code := responseCode2576(t, w)
		status := sessionStatus2576(t, session.ID)
		t.Logf("CASE_FAILED_RETRY=%d", code)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, 20000, code)
		assert.Equal(t, "confirmed", status)
	})

	t.Run("Confirm2576_ConfirmedIdempotent", func(t *testing.T) {
		userID := createUser2576(t, "pending")
		session := createSession2576(t, sessionFixture2576{UserID: userID, Status: "confirmed", ActionType: "site_staff"})

		w := confirmWith2576(t, session.ID, session.Token)
		code := responseCode2576(t, w)
		action := siteMemberCount2576(t, userID)
		t.Logf("CASE_CONFIRMED_IDEMPOTENT=%d", code)
		t.Logf("CASE_CONFIRMED_NO_DUP=%d", boolToken(action == 0))

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, 20000, code)
		assert.Equal(t, "confirmed", sessionStatus2576(t, session.ID))
		// 幂等只读：不得执行动作（无 site_members 增量）。
		assert.EqualValues(t, 0, action)
	})

	t.Run("Confirm2576_Rejected", func(t *testing.T) {
		// 用 merchant_admin 动作：若守卫失效，executeAction 会真的打 IAM（bind +
		// customer-permissions），因此 IAM 调用数 = 0 是强负例，而不仅是本地行为。
		fakeIAM := installFakeIAM2540(t)
		userID := createUser2576(t, "pending")
		merchantID := createMerchant2576(t)
		session := createSession2576(t, sessionFixture2576{
			UserID: userID, Status: "rejected", ActionType: "merchant_admin", MerchantID: merchantID,
		})

		w := confirmWith2576(t, session.ID, session.Token)
		code := responseCode2576(t, w)
		status := sessionStatus2576(t, session.ID)
		iamCalls := len(fakeIAM.snapshot())
		t.Logf("CASE_REJECTED=%d", code)
		t.Logf("CASE_REJECTED_STATUS=%s", status)
		t.Logf("CASE_REJECTED_NO_WRITE=%d", boolToken(status == "rejected"))
		t.Logf("CASE_REJECTED_IAM_CALLS=%d", iamCalls)

		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Equal(t, 40901, code)
		assert.Equal(t, "rejected", status)
		assert.Equal(t, 0, iamCalls, "rejected 会话不得触发任何 IAM 写入")
	})

	t.Run("Confirm2576_ExpiredFlag", func(t *testing.T) {
		userID := createUser2576(t, "pending")
		// status=expired（IAM 24h 超时标记）但 expires_at 仍在未来 → 只能由状态守卫拒绝。
		session := createSession2576(t, sessionFixture2576{UserID: userID, Status: "expired", ActionType: "site_staff"})

		w := confirmWith2576(t, session.ID, session.Token)
		code := responseCode2576(t, w)
		status := sessionStatus2576(t, session.ID)
		t.Logf("CASE_EXPIRED_FLAG=%d", code)
		t.Logf("CASE_EXPIRED_FLAG_STATUS=%s", status)

		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Equal(t, 40901, code)
		assert.Equal(t, "expired", status)
	})

	t.Run("Confirm2576_UnknownStatus", func(t *testing.T) {
		userID := createUser2576(t, "pending")
		session := createSession2576(t, sessionFixture2576{UserID: userID, Status: "unknown_state_2576", ActionType: "site_staff"})

		w := confirmWith2576(t, session.ID, session.Token)
		code := responseCode2576(t, w)
		t.Logf("CASE_UNKNOWN=%d", code)

		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Equal(t, 40901, code)
		assert.Equal(t, "unknown_state_2576", sessionStatus2576(t, session.ID))
	})

	t.Run("Confirm2576_PastExpiry", func(t *testing.T) {
		userID := createUser2576(t, "pending")
		session := createSession2576(t, sessionFixture2576{
			UserID: userID, Status: "waiting", ActionType: "site_staff",
			ExpiresAt: time.Now().Add(-2 * time.Hour),
		})

		w := confirmWith2576(t, session.ID, session.Token)
		code := responseCode2576(t, w)
		t.Logf("CASE_PAST=%d", code)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, 40002, code)
		assert.Equal(t, "waiting", sessionStatus2576(t, session.ID))
	})

	t.Run("Confirm2576_BadToken", func(t *testing.T) {
		userID := createUser2576(t, "pending")
		session := createSession2576(t, sessionFixture2576{UserID: userID, Status: "waiting", ActionType: "site_staff"})

		w := confirmWith2576(t, session.ID, "wrong-token-2576")
		t.Logf("CASE_BAD_TOKEN=%d", w.Code)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Equal(t, "waiting", sessionStatus2576(t, session.ID))
	})
}

// TestConfirmSessionCallback2576 覆盖 IAM 回调 accept 分支的分类（D5）。
func TestConfirmSessionCallback2576(t *testing.T) {
	setupConfirm2576(t)

	callback2576 := func(t *testing.T, session models.ConfirmationSession) *httptest.ResponseRecorder {
		t.Helper()
		handler := NewConfirmationSessionHandler()
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.GET("/api/iam/confirmation-callback", handler.IAMConfirmationCallback)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET",
			"/api/iam/confirmation-callback?session="+session.IAMSessionID+"&result=accept", nil))
		return w
	}

	t.Run("Callback2576_Waiting", func(t *testing.T) {
		userID := createUser2576(t, "pending")
		session := createSession2576(t, sessionFixture2576{UserID: userID, Status: "waiting", ActionType: "site_staff"})

		w := callback2576(t, session)
		status := sessionStatus2576(t, session.ID)
		t.Logf("CB_WAITING=%s", redirectStatus2576(w.Header().Get("Location")))
		t.Logf("CB_WAITING_STATUS=%s", status)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Contains(t, w.Header().Get("Location"), "status=success")
		assert.Equal(t, "confirmed", status)
	})

	t.Run("Callback2576_Failed", func(t *testing.T) {
		userID := createUser2576(t, "pending")
		session := createSession2576(t, sessionFixture2576{UserID: userID, Status: "failed", ActionType: "site_staff"})

		w := callback2576(t, session)
		status := sessionStatus2576(t, session.ID)
		t.Logf("CB_FAILED_STATUS=%s", status)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Contains(t, w.Header().Get("Location"), "status=success")
		assert.Equal(t, "confirmed", status)
	})

	t.Run("Callback2576_ConfirmedNoDup", func(t *testing.T) {
		fakeIAM := installFakeIAM2540(t)
		userID := createUser2576(t, "pending")
		merchantID := createMerchant2576(t)
		session := createSession2576(t, sessionFixture2576{
			UserID: userID, Status: "confirmed", ActionType: "merchant_admin", MerchantID: merchantID,
		})

		w := callback2576(t, session)
		iamCalls := len(fakeIAM.snapshot())
		t.Logf("CB_CONFIRMED=%s", redirectMessage2576(w.Header().Get("Location")))
		t.Logf("CB_CONFIRMED_STATUS=%s", sessionStatus2576(t, session.ID))
		t.Logf("CB_CONFIRMED_NO_DUP=%d", boolToken(iamCalls == 0))

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Contains(t, w.Header().Get("Location"), "already_confirmed")
		assert.Equal(t, "confirmed", sessionStatus2576(t, session.ID))
		assert.Equal(t, 0, iamCalls, "已 confirmed 的会话不得重复执行动作")
	})

	t.Run("Callback2576_Rejected", func(t *testing.T) {
		fakeIAM := installFakeIAM2540(t)
		userID := createUser2576(t, "pending")
		merchantID := createMerchant2576(t)
		session := createSession2576(t, sessionFixture2576{
			UserID: userID, Status: "rejected", ActionType: "merchant_admin", MerchantID: merchantID,
		})

		w := callback2576(t, session)
		iamCalls := len(fakeIAM.snapshot())
		t.Logf("CB_REJECTED=%s", redirectMessage2576(w.Header().Get("Location")))
		t.Logf("CB_REJECTED_STATUS=%s", sessionStatus2576(t, session.ID))
		t.Logf("CB_REJECTED_IAM_CALLS=%d", iamCalls)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Contains(t, w.Header().Get("Location"), "not_confirmable")
		assert.Equal(t, "rejected", sessionStatus2576(t, session.ID))
		assert.Equal(t, 0, iamCalls, "rejected 会话不得被 IAM 回调翻回 confirmed")
	})

	t.Run("Callback2576_ExpiredFlag", func(t *testing.T) {
		userID := createUser2576(t, "pending")
		session := createSession2576(t, sessionFixture2576{UserID: userID, Status: "expired", ActionType: "site_staff"})

		w := callback2576(t, session)
		status := sessionStatus2576(t, session.ID)
		t.Logf("CB_EXPIRED_FLAG=%s", redirectMessage2576(w.Header().Get("Location")))
		t.Logf("CB_EXPIRED_FLAG_STATUS=%s", status)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Contains(t, w.Header().Get("Location"), "not_confirmable")
		assert.Equal(t, "expired", status)
	})
}

// redirectStatus2576 从回调 Location 中取出 `status=` 的值（success/rejected/error）。
func redirectStatus2576(location string) string {
	return redirectParam2576(location, "status")
}

// redirectMessage2576 从回调 Location 中取出 `message=` 的值（already_confirmed / not_confirmable）。
func redirectMessage2576(location string) string {
	return redirectParam2576(location, "message")
}

func redirectParam2576(location, key string) string {
	idx := strings.Index(location, key+"=")
	if idx < 0 {
		return "none"
	}
	rest := location[idx+len(key)+1:]
	if amp := strings.Index(rest, "&"); amp >= 0 {
		return rest[:amp]
	}
	return rest
}

// boolToken 把布尔断言转成 0/1，便于证据脚本提取机器 token。
func boolToken(ok bool) int {
	if ok {
		return 1
	}
	return 0
}
