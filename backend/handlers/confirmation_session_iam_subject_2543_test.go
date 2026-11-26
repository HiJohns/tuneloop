package handlers

// #2543：确认执行的主体域收敛（B1）——`executeAction` 的 `merchant_admin` 分支
// 只做本地校验，不得写 IAM。
//
// 历史缺陷：把本地 `users.id` 当 IAM 主体传给 BindUserToOrganization /
// SetUserCustomerPermissions（IAM 主体是 `users.iam_sub`）。本文件用 IAM 替身
// 计数证明「0 次 IAM 请求」，并回归 `site_staff` 分支（本地写、不调 IAM）。

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"
)

type iamSpy2543 struct {
	calls    int32
	lastPath string
	lastBody string
}

func (s *iamSpy2543) callCount() int { return int(atomic.LoadInt32(&s.calls)) }

// setupIAMSpy2543 points the IAM client at a local spy server so any IAM write
// from the confirmation path is observable (and counted).
func setupIAMSpy2543(t *testing.T) *iamSpy2543 {
	t.Helper()
	spy := &iamSpy2543{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&spy.calls, 1)
		raw, _ := io.ReadAll(r.Body)
		spy.lastPath = r.URL.Path
		spy.lastBody = string(raw)
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
	return r
}

func doConfirm2543(t *testing.T, r *gin.Engine, sessionID, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/confirmation-sessions/"+sessionID+"/confirm?token="+token, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// R1：merchant_admin 动作 → 确认成功、用户激活、本地会话 confirmed，且 0 次 IAM 调用。
func TestExecuteAction_MerchantAdmin_NoIAMWrite_2543(t *testing.T) {
	db := seedConfirmFixture2543(t)
	spy := setupIAMSpy2543(t)

	tenantID := uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)

	userID := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: "sub-2543", TenantID: tenantID, OrgID: tenantID,
		Username: "u2543-" + userID[:8], Name: "Confirm User",
		Email: userID[:8] + "@example.test", Status: "pending", Role: "merchant_admin",
	}).Error)

	merchantID := uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID: merchantID, TenantID: tenantID, OrgID: tenantID, Name: "M2543",
		AdminUID: userID, Status: "active", MerchantType: "full",
	}).Error)

	token := uuid.NewString()
	session := models.ConfirmationSession{
		ID: uuid.NewString(), TenantID: tenantID, OrgID: tenantID, UserID: userID,
		ConfirmType: "email", ConfirmTarget: "confirm2543@example.test",
		MerchantID: merchantID, ActionType: "merchant_admin",
		ActionTargetID: "00000000-0000-0000-0000-000000000000",
		Status:         "waiting",
		Token:          token, ExpiresAt: time.Now().Add(time.Hour),
	}
	require.NoError(t, db.Create(&session).Error)

	h := NewConfirmationSessionHandler()
	r := confirmRouter2543(t, tenantID, tenantID, userID, h)
	w := doConfirm2543(t, r, session.ID, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), w.Body.String())
	assert.Equal(t, 20000, resp.Code, "CONFIRM_CODE=20000")

	var stored models.ConfirmationSession
	require.NoError(t, db.Where("id = ?", session.ID).First(&stored).Error)
	assert.Equal(t, "confirmed", stored.Status, "SESSION_STATUS=confirmed")

	var storedUser models.User
	require.NoError(t, db.Where("id = ?", userID).First(&storedUser).Error)
	assert.Equal(t, "active", storedUser.Status, "USER_STATUS=active")

	require.Equal(t, 0, spy.callCount(), "IAM_CALLS=0 (no IAM write on the merchant_admin path)")
	if spy.callCount() > 0 {
		t.Logf("unexpected IAM call: path=%s body=%s", spy.lastPath, spy.lastBody)
	}

	t.Logf("CONFIRM_STATUS=%d SESSION_STATUS=confirmed USER_STATUS=active IAM_CALLS=%d", w.Code, spy.callCount())
}

// R3：site_staff 分支回归 —— 仍创建 site_members 行且不调 IAM。
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
	session := models.ConfirmationSession{
		ID: uuid.NewString(), TenantID: tenantID, OrgID: siteID, UserID: userID,
		ConfirmType: "email", ConfirmTarget: "site2543@example.test",
		MerchantID: "00000000-0000-0000-0000-000000000000",
		ActionType: "site_staff", ActionTargetID: siteID, Status: "waiting",
		Token: token, ExpiresAt: time.Now().Add(time.Hour),
	}
	require.NoError(t, db.Create(&session).Error)

	h := NewConfirmationSessionHandler()
	r := confirmRouter2543(t, tenantID, siteID, userID, h)
	w := doConfirm2543(t, r, session.ID, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var row models.SiteMember
	require.NoError(t, db.Where("site_id = ? AND user_id = ?", siteID, userID).First(&row).Error)
	assert.Equal(t, "Staff", row.Role, "SITE_MEMBER_ROLE=Staff")
	require.Equal(t, 0, spy.callCount(), "IAM_CALLS=0")

	t.Logf("SITE_MEMBER_CREATED=1 SITE_MEMBER_ROLE=%s IAM_CALLS=%d", row.Role, spy.callCount())
}

// 保持 database 包被引用（fixture 断言同一实例）。
var _ = database.GetDB
