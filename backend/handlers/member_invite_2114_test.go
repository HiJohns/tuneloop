package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2114 邀请与成员管理：申请 / 审批 / 徽标 / 直接邀请。

func setupMemberInvite2114(t *testing.T) (*gin.Engine, string, string, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tid, org, _ := testfixtures.NewTenantIDs("2114ab0c000a")

	merchantID := uuid.New().String()
	require.NoError(t, db.Create(&models.Merchant{
		ID: merchantID, TenantID: tid, OrgID: org, AdminUID: uuid.New().String(), Name: "卡丹萨", Status: "active",
	}).Error)

	siteOrg := uuid.New().String()
	siteID := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{
		ID: siteID, TenantID: tid, OrgID: siteOrg, Name: "西铁营店", Type: "normal", Status: "active",
	}).Error)

	adminID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: adminID, IAMSub: adminID, TenantID: tid, OrgID: org, Role: "OWNER", Name: "商户管理员", Status: "active",
	}).Error)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: tid, OrgID: org, UserID: adminID, Role: "OWNER"}.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/api/sites/:id/members/apply", ApplySiteMembership)
	r.GET("/api/admin/invites", ListMerchantInvites)
	r.GET("/api/admin/invites/pending-count", MerchantInvitesPendingCount)
	r.POST("/api/admin/invites/:id/approve", ApproveInvite)
	r.POST("/api/admin/invites/:id/reject", RejectInvite)
	r.POST("/api/admin/merchants/:id/invites", InviteToMerchant)
	return r, tid, siteID, merchantID
}

func doJSON(t *testing.T, r *gin.Engine, method, path, body string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

func TestMemberInvite2114_ApplyCreatesPendingApprovalAndNotifies(t *testing.T) {
	r, tid, siteID, _ := setupMemberInvite2114(t)
	db := database.GetDB()

	code, resp := doJSON(t, r, http.MethodPost, "/api/sites/"+siteID+"/members/apply",
		`{"identifier":"p2@example.com","role":"site_member","note":"老员工"}`)
	require.Equal(t, http.StatusCreated, code, resp)
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "pending_approval", data["status"])

	var inv models.StaffInvite
	require.NoError(t, db.Where("tenant_id = ? AND site_id = ?", tid, siteID).First(&inv).Error)
	assert.Equal(t, "pending_approval", inv.Status)
	assert.Equal(t, "site_member", inv.Kind)
	assert.Equal(t, "p2@example.com", inv.InviteeIdentifier)
	assert.NotNil(t, inv.RequestedBy)

	var notifCount int64
	db.Model(&models.Notification{}).Where("tenant_id = ? AND type = ?", tid, "invite_application").Count(&notifCount)
	assert.GreaterOrEqual(t, notifCount, int64(1), "商户管理员应收到邀请申请通知")
}

func TestMemberInvite2114_PendingCountApproveAndReject(t *testing.T) {
	r, tid, siteID, _ := setupMemberInvite2114(t)
	db := database.GetDB()

	// 两笔申请
	for _, idf := range []string{"a@example.com", "b@example.com"} {
		code, resp := doJSON(t, r, http.MethodPost, "/api/sites/"+siteID+"/members/apply",
			`{"identifier":"`+idf+`","role":"site_member"}`)
		require.Equal(t, http.StatusCreated, code, resp)
	}

	code, resp := doJSON(t, r, http.MethodGet, "/api/admin/invites/pending-count", "")
	require.Equal(t, http.StatusOK, code, resp)
	assert.Equal(t, float64(2), resp["data"].(map[string]interface{})["count"])

	var invs []models.StaffInvite
	require.NoError(t, db.Where("tenant_id = ?", tid).Order("created_at ASC").Find(&invs).Error)
	require.Len(t, invs, 2)

	// 同意 → pending
	code, resp = doJSON(t, r, http.MethodPost, "/api/admin/invites/"+invs[0].ID+"/approve", `{}`)
	require.Equal(t, http.StatusOK, code, resp)
	var after models.StaffInvite
	require.NoError(t, db.Where("id = ?", invs[0].ID).First(&after).Error)
	assert.Equal(t, "pending", after.Status)
	require.NotNil(t, after.ApprovedBy)

	// 拒绝 → rejected
	code, resp = doJSON(t, r, http.MethodPost, "/api/admin/invites/"+invs[1].ID+"/reject", `{"reason":"不是本商户员工"}`)
	require.Equal(t, http.StatusOK, code, resp)
	var rejected models.StaffInvite
	require.NoError(t, db.Where("id = ?", invs[1].ID).First(&rejected).Error)
	assert.Equal(t, "rejected", rejected.Status)
	assert.Equal(t, "不是本商户员工", rejected.RejectReason)

	code, resp = doJSON(t, r, http.MethodGet, "/api/admin/invites/pending-count", "")
	require.Equal(t, http.StatusOK, code, resp)
	assert.Equal(t, float64(0), resp["data"].(map[string]interface{})["count"])
}

func TestMemberInvite2114_DirectMerchantInvite(t *testing.T) {
	r, tid, _, merchantID := setupMemberInvite2114(t)
	db := database.GetDB()

	code, resp := doJSON(t, r, http.MethodPost, "/api/admin/merchants/"+merchantID+"/invites",
		`{"identifier":"p4@example.com","kind":"merchant_staff"}`)
	require.Equal(t, http.StatusCreated, code, resp)
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "pending", data["status"])
	assert.Equal(t, "merchant_staff", data["kind"])

	var inv models.StaffInvite
	require.NoError(t, db.Where("tenant_id = ? AND kind = ?", tid, "merchant_staff").First(&inv).Error)
	assert.Nil(t, inv.SiteID, "商户级邀请 site_id 应为 NULL")
}
