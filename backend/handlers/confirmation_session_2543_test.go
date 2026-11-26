package handlers

// #2543（Rev.2，Owner 2026-11-26 裁定「确认会话可以建表」+「IAM 为事实来源」）
// 的验收用例：
//
//   R1：confirmation_sessions 迁移建表（列齐全 / token 唯一 / down 可回滚），
//       且 db.go 的「无表」豁免清单不再包含 ConfirmationSession；
//   R2：merchant_admin 确认执行的 IAM 主体是 users.iam_sub（不是本地 users.id），
//       关系角色为 OWNER，customer-permissions 同样以 iam_sub 为主体；
//   R3：iam_sub 为空 → 确认显式失败（status=failed，message 含「账号尚未激活」），
//       且 0 次 IAM 请求（禁止把本地 id 发给 IAM）；
//   R4：site_staff 分支回归（只写本地 site_members，不调 IAM）。
//
// 历史缺陷：旧实现把本地 users.id 当 IAM 主体传给 BindUserToOrganization /
// SetUserCustomerPermissions；IAM 按 iam_sub 识别用户，对端 404/400。

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"
)

const confirmationSessionMigration2543 = "../database/migrations/20261126002_confirmation_sessions.%s.sql"

// ---------------------------------------------------------------- fixtures

type iamCall2543 struct {
	method string
	path   string
	body   string
}

type iamSpy2543 struct {
	mu    sync.Mutex
	calls []iamCall2543
}

func (s *iamSpy2543) record(r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, iamCall2543{method: r.Method, path: r.URL.Path, body: string(raw)})
}

func (s *iamSpy2543) snapshot() []iamCall2543 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]iamCall2543, len(s.calls))
	copy(out, s.calls)
	return out
}

func (s *iamSpy2543) count() int { return len(s.snapshot()) }

// setupIAMSpy2543 points the IAM client at a local spy so every IAM write from
// the confirmation path is observable.
func setupIAMSpy2543(t *testing.T) *iamSpy2543 {
	t.Helper()
	spy := &iamSpy2543{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Client-credentials token exchange is transport, not a business IAM write.
		if r.URL.Path == "/api/v1/auth/token" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":3600,"token_type":"Bearer"}`))
			return
		}
		// perm-version notifications are a client-side cache invalidation side
		// effect of any successful write; they carry no subject, so they are not
		// part of the subject-domain assertion.
		if strings.HasPrefix(r.URL.Path, "/api/v1/perm-version/") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":20000,"data":{}}`))
			return
		}
		spy.record(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":20000,"data":{}}`))
	}))
	t.Cleanup(srv.Close)
	prev := services.GetIAMInternalURL()
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting(prev) })
	return spy
}

func seedConfirmFixture2543(t *testing.T) *gorm.DB {
	t.Helper()
	db := testfixtures.SetupTestDB(t)
	require.NotNil(t, db, "test DB unavailable")
	testutil.SetDB(t, db)
	return db
}

func confirmRouter2543(t *testing.T, tenantID, orgID, userID string, h *ConfirmationSessionHandler) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.TestActor{TenantID: tenantID, OrgID: orgID, UserID: userID, Role: "OWNER"}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/confirmation-sessions/:id/confirm", h.Confirm)
	r.POST("/confirmation-sessions", h.Create)
	return r
}

func doConfirm2543(t *testing.T, r *gin.Engine, sessionID, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/confirmation-sessions/"+sessionID+"/confirm?token="+token, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// seedMerchantAdminSession2543 creates the user + merchant + waiting session that
// the merchant_admin confirmation path operates on.
func seedMerchantAdminSession2543(t *testing.T, db *gorm.DB, iamSub string) (tenantID, userID, sessionID, token string) {
	t.Helper()
	tenantID = uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)

	userID = uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: iamSub, TenantID: tenantID, OrgID: tenantID,
		Username: "u2543-" + userID[:8], Name: "Confirm User",
		Email: userID[:8] + "@example.test", Status: "pending", Role: "merchant_admin",
	}).Error)

	merchantID := uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: merchantID, TenantID: tenantID, OrgID: tenantID, Name: "M2543-" + userID[:8],
		AdminUID: userID, Status: "active", MerchantType: "full",
	}).Error)

	token = uuid.NewString()
	sessionID = uuid.NewString()
	require.NoError(t, db.Create(&models.ConfirmationSession{
		ID: sessionID, TenantID: tenantID, OrgID: tenantID, UserID: userID,
		ConfirmType: "email", ConfirmTarget: "confirm2543-" + userID[:8] + "@example.test",
		MerchantID: merchantID, ActionType: "merchant_admin",
		ActionTargetID: "00000000-0000-0000-0000-000000000000",
		Status:         "waiting", Token: token, ExpiresAt: time.Now().Add(time.Hour),
	}).Error)
	return tenantID, userID, sessionID, token
}

func execSQLFile2543(t *testing.T, db *gorm.DB, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "read migration %s", path)
	for _, stmt := range strings.Split(string(raw), ";") {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		require.NoError(t, db.Exec(stmt).Error, "exec migration statement from %s", path)
	}
}

func tableExists2543(t *testing.T, db *gorm.DB, table string) bool {
	t.Helper()
	var reg *string
	require.NoError(t, db.Raw("SELECT to_regclass(?)::text", table).Scan(&reg).Error)
	return reg != nil && *reg != ""
}

// ------------------------------------------------------------------- R1

// TestConfirmationSessionsMigration_2543 asserts the up/down migration pair and
// the removal of the "no table" exemption.
func TestConfirmationSessionsMigration_2543(t *testing.T) {
	db := seedConfirmFixture2543(t)

	// Start from a table-less state so the migration itself is what creates it.
	require.NoError(t, db.Exec("DROP TABLE IF EXISTS confirmation_sessions").Error)
	require.False(t, tableExists2543(t, db, "confirmation_sessions"))

	execSQLFile2543(t, db, fmt.Sprintf(confirmationSessionMigration2543, "up"))
	require.True(t, tableExists2543(t, db, "confirmation_sessions"), "up migration must create the table")
	t.Log("TABLE_EXISTS=1")

	type colRow struct {
		ColumnName string
		IsNullable string
	}
	var cols []colRow
	require.NoError(t, db.Raw(`SELECT column_name, is_nullable FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'confirmation_sessions'
		ORDER BY column_name`).Scan(&cols).Error)

	present := make(map[string]bool, len(cols))
	for _, c := range cols {
		present[c.ColumnName] = true
	}
	expected := []string{
		"id", "tenant_id", "org_id", "user_id", "confirm_type", "confirm_target",
		"merchant_id", "action_type", "action_target_id", "iam_session_id",
		"callback_url", "status", "message", "token", "expires_at",
		"confirmed_at", "created_at", "updated_at",
	}
	for _, c := range expected {
		require.True(t, present[c], "confirmation_sessions missing column %q", c)
	}
	require.GreaterOrEqual(t, len(cols), 17, "COLUMN_COUNT=%d", len(cols))
	t.Logf("COLUMN_COUNT=%d", len(cols))

	var uniqueOK bool
	require.NoError(t, db.Raw(`SELECT EXISTS (
		SELECT 1 FROM pg_indexes
		WHERE schemaname = current_schema() AND tablename = 'confirmation_sessions'
		  AND indexdef ILIKE '%UNIQUE%' AND indexdef ILIKE '%(token)%'
	)`).Scan(&uniqueOK).Error)
	require.True(t, uniqueOK, "token must have a unique index")
	t.Log("TOKEN_UNIQUE=1")

	// The model must no longer be listed as "no table ever created".
	dbGo, err := os.ReadFile("../database/db.go")
	require.NoError(t, err)
	exempt := strings.Contains(string(dbGo), "ConfirmationSession, Label")
	require.False(t, exempt, "db.go must not list ConfirmationSession in the no-table exemption")
	t.Log("EXEMPT_REMOVED=1")

	// down must roll the table back (round-trip).
	execSQLFile2543(t, db, fmt.Sprintf(confirmationSessionMigration2543, "down"))
	require.False(t, tableExists2543(t, db, "confirmation_sessions"), "down migration must drop the table")
	t.Log("DOWN_TABLE_EXISTS=0")

	// Restore for the rest of the package (the run DB is a clone, but tests are
	// order-independent and other cases need the table).
	execSQLFile2543(t, db, fmt.Sprintf(confirmationSessionMigration2543, "up"))
	require.True(t, tableExists2543(t, db, "confirmation_sessions"))
}

// ------------------------------------------------------------------- R2

// TestExecuteAction_MerchantAdmin_UsesIAMSub_2543 asserts the IAM subject is
// users.iam_sub (not the local users.id) and that the relation role is OWNER.
func TestExecuteAction_MerchantAdmin_UsesIAMSub_2543(t *testing.T) {
	db := seedConfirmFixture2543(t)
	spy := setupIAMSpy2543(t)

	iamSub := "sub-2543-" + uuid.NewString()[:8]
	tenantID, userID, sessionID, token := seedMerchantAdminSession2543(t, db, iamSub)

	h := NewConfirmationSessionHandler()
	r := confirmRouter2543(t, tenantID, tenantID, userID, h)
	w := doConfirm2543(t, r, sessionID, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var stored models.ConfirmationSession
	require.NoError(t, db.Where("id = ?", sessionID).First(&stored).Error)
	require.Equal(t, "confirmed", stored.Status)
	t.Logf("CONFIRM_STATUS=%s", stored.Status)

	calls := spy.snapshot()
	require.Len(t, calls, 2, "merchant_admin must do exactly bind + customer-permissions: %+v", calls)

	bindPath := "/api/v1/organizations/" + tenantID + "/users/" + iamSub + "/bind"
	permPath := "/api/v1/organizations/" + tenantID + "/users/" + iamSub + "/customer-permissions"
	assert.Equal(t, bindPath, calls[0].path)
	assert.Equal(t, permPath, calls[1].path)
	t.Logf("BIND_USER_ARG=%s", iamSub)
	t.Logf("PERM_USER_ARG=%s", iamSub)

	var bindBody struct {
		Action string `json:"action"`
		Role   string `json:"role"`
	}
	require.NoError(t, json.Unmarshal([]byte(calls[0].body), &bindBody))
	assert.Equal(t, "OWNER", bindBody.Role)
	t.Logf("ROLE=%s", bindBody.Role)

	localIDLeaked := false
	for _, c := range calls {
		if strings.Contains(c.path, userID) || strings.Contains(c.body, userID) {
			localIDLeaked = true
			t.Logf("LEAK: local id %s in IAM call %s %s body=%s", userID, c.method, c.path, c.body)
		}
	}
	assert.False(t, localIDLeaked, "the local users.id must never be sent to IAM")
	t.Logf("IAM_LOCAL_ID_SENT=%d", boolToInt2543(localIDLeaked))
}

// ------------------------------------------------------------------- R3

// TestExecuteAction_MerchantAdmin_EmptyIAMSub_2543 asserts an explicit failure
// with zero IAM requests when the local user has no iam_sub.
func TestExecuteAction_MerchantAdmin_EmptyIAMSub_2543(t *testing.T) {
	db := seedConfirmFixture2543(t)
	spy := setupIAMSpy2543(t)

	tenantID, userID, sessionID, token := seedMerchantAdminSession2543(t, db, "")

	h := NewConfirmationSessionHandler()
	r := confirmRouter2543(t, tenantID, tenantID, userID, h)
	w := doConfirm2543(t, r, sessionID, token)
	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), w.Body.String())
	assert.Contains(t, resp.Message, "账号尚未激活")

	var stored models.ConfirmationSession
	require.NoError(t, db.Where("id = ?", sessionID).First(&stored).Error)
	require.Equal(t, "failed", stored.Status, "the session must not stay confirmed after a failed action")
	t.Logf("CONFIRM_STATUS=%s", stored.Status)

	require.Equal(t, 0, spy.count(), "no IAM request may be sent without an iam_sub: %+v", spy.snapshot())
	t.Logf("IAM_CALLS=%d", spy.count())
	t.Logf("MESSAGE_HAS_ACTIVATE=%d", boolToInt2543(strings.Contains(stored.Message, "账号尚未激活")))
}

// ------------------------------------------------------------------- R4

// TestExecuteAction_SiteStaff_Regression_2543 keeps the local-only site branch
// behaviour (still creates site_members, still no IAM write).
func TestExecuteAction_SiteStaff_Regression_2543(t *testing.T) {
	db := seedConfirmFixture2543(t)
	spy := setupIAMSpy2543(t)

	tenantID := uuid.NewString()
	siteID := uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)
	testfixtures.SeedSiteFK(t, db, siteID, tenantID)

	userID := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: "sub-2543-site", TenantID: tenantID, OrgID: siteID,
		Username: "u2543s-" + userID[:8], Name: "Site User",
		Email: userID[:8] + "@example.test", Status: "active", Role: "STAFF",
	}).Error)

	token := uuid.NewString()
	sessionID := uuid.NewString()
	require.NoError(t, db.Create(&models.ConfirmationSession{
		ID: sessionID, TenantID: tenantID, OrgID: siteID, UserID: userID,
		ConfirmType: "email", ConfirmTarget: "site2543-" + userID[:8] + "@example.test",
		MerchantID: "00000000-0000-0000-0000-000000000000",
		ActionType: "site_staff", ActionTargetID: siteID, Status: "waiting",
		Token: token, ExpiresAt: time.Now().Add(time.Hour),
	}).Error)

	h := NewConfirmationSessionHandler()
	r := confirmRouter2543(t, tenantID, siteID, userID, h)
	w := doConfirm2543(t, r, sessionID, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var row models.SiteMember
	require.NoError(t, db.Where("site_id = ? AND user_id = ?", siteID, userID).First(&row).Error)
	assert.Equal(t, "Staff", row.Role)
	require.Equal(t, 0, spy.count(), "the site branch must stay local-only")
	t.Logf("SITE_MEMBER_CREATED=1 SITE_MEMBER_ROLE=%s IAM_CALLS=%d", row.Role, spy.count())
}

// ------------------------------------------------------------------- R5

// TestCreateConfirmationSession_IAMReady_2543 asserts Create reports whether the
// local user is already activated (iam_ready) without requiring it.
func TestCreateConfirmationSession_IAMReady_2543(t *testing.T) {
	db := seedConfirmFixture2543(t)

	tenantID := uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)

	mk := func(iamSub string) string {
		id := uuid.NewString()
		require.NoError(t, db.Create(&models.User{
			ID: id, IAMSub: iamSub, TenantID: tenantID, OrgID: tenantID,
			Username: "u2543c-" + id[:8], Name: "Create User",
			Email: id[:8] + "@example.test", Status: "active", Role: "merchant_admin",
		}).Error)
		return id
	}

	h := NewConfirmationSessionHandler()
	post := func(userID string) map[string]interface{} {
		payload := fmt.Sprintf(`{"user_id":%q,"confirm_type":"email","confirm_target":"c2543@example.test","action_type":"site_staff","action_target_id":%q}`,
			userID, "33333333-3333-3333-3333-333333333333")
		req := httptest.NewRequest(http.MethodPost, "/confirmation-sessions", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r := confirmRouter2543(t, tenantID, tenantID, userID, h)
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var body struct {
			Data map[string]interface{} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
		return body.Data
	}

	ready := post(mk("sub-2543-ready"))
	assert.Equal(t, true, ready["iam_ready"], "IAM_READY_ACTIVE=1")

	pending := post(mk(""))
	assert.Equal(t, false, pending["iam_ready"], "IAM_READY_PENDING=0")
	t.Logf("IAM_READY_ACTIVE=%v IAM_READY_PENDING=%v", ready["iam_ready"], pending["iam_ready"])
}

func boolToInt2543(b bool) int {
	if b {
		return 1
	}
	return 0
}
