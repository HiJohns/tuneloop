package handlers

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2565 R2：语义守卫用例。
//
// #2526 把「`users.role` 为空或 `user` 且无任何成员关系」的行定义为**顾客**，
// 员工 by-id 动作（详情 / 保存 / 激活）对这类行一律 404（`40400`），且必须在
// 触达 IAM 之前就拒绝（不泄漏存在性、不产生 IAM 副作用）。
//
// 本用例把该语义钉死：若未来有人为了「修红」而放宽 personnelUserVisible
// （例如放行本地 pending 空角色行），本用例会立刻变红。
func TestActivateUser2565_EmptyRoleNoMembershipIs404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	operatorID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: operatorID, IAMSub: operatorID, TenantID: tenantID, OrgID: orgID,
		Username: "guard2565op", Role: "site_admin", Status: "active",
	}).Error)
	t.Setenv("IAM_SECRET", testIAMSecret)

	// 纯顾客形态：role 空（isCustomerUserRole 判真）、无 site_members / merchant_members、
	// 仍是本地 pending（尚未激活 → iam_sub 为空）。
	pendingID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: pendingID, TenantID: tenantID, OrgID: orgID,
		Name: "空角色待激活", Phone: "13900002565", Email: "guard2565@example.com",
		Status: "pending",
	}).Error)

	var iamCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&iamCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"guard test must never reach IAM"}`))
	}))
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })

	h := &UserStaffHandler{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.MakeSiteAdmin(tenantID, orgID, operatorID).InjectContext(c.Request.Context()))
		c.Next()
	})
	r.POST("/users/:id/activate", h.ActivateUser)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users/"+pendingID+"/activate", nil)
	r.ServeHTTP(w, req)

	calls := atomic.LoadInt32(&iamCalls)
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"code":40400`, "invisible rows must map to 40400 (no existence leak)")
	assert.EqualValues(t, 0, calls, "visibility gate must reject before any IAM call")

	t.Logf("GUARD_STATUS=%d", w.Code)
	t.Logf("GUARD_CODE=40400")
	t.Logf("GUARD_IAM_CALLS=%d", calls)
}
