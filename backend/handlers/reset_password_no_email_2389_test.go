package handlers

// #2389：无邮箱账号重设密码必须显式 400，而不是 skipped + 200「假成功」。
//
// #2527 Rev.2：可见性判定（#2526 的 personnelUserVisible）在「无邮箱」之前执行，
// 因此本用例的夹具必须是**可见员工**（site_member + site_members），否则纯顾客形态
// 会先落进 skipped_invisible，永远到不了 40002。断言本身不变，并补一条负例：
// 同批里的纯顾客 → 20000 + skipped_invisible=1，不得返回 40002。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetPasswordNoEmail_2389(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)

	tenantID := uuid.New().String()
	testfixtures.SeedTenantFK(t, db, tenantID)
	orgID := uuid.New().String()
	siteID := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{ID: siteID, TenantID: tenantID, OrgID: orgID, Name: "网点2389", Status: "active"}).Error)

	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: uuid.New().String(), TenantID: tenantID, OrgID: orgID,
		Username: "noemail2389", Name: "无邮箱用户", Email: "", Status: "active",
		Role: "site_member",
	}).Error)
	// #2527 Rev.2：员工形态（membership）才在 #2526 的可见集合内。
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.New().String(), TenantID: tenantID, SiteID: siteID, UserID: userID,
		Role: "site_member", Roles: []string{"site_member"}, Status: "active",
	}).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: tenantID, OrgID: tenantID, UserID: uuid.New().String(), Role: "ADMIN"}.InjectContext(c.Request.Context()))
		c.Next()
	})
	router.POST("/users/reset-password", (&UserStaffHandler{}).ResetPassword)

	body, _ := json.Marshal(map[string]interface{}{"user_ids": []string{userID}})
	req := httptest.NewRequest(http.MethodPost, "/users/reset-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.EqualValues(t, 40002, resp["code"])
	assert.Contains(t, resp["message"], "未配置邮箱")
}

// #2527 R8 负例：同批里的纯顾客（role=""、无 membership）必须显式 skipped_invisible，
// 且不得把整批判成「未配置邮箱」。
func TestResetPasswordInvisibleCustomerSkipped_2527(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)

	tenantID := uuid.New().String()
	testfixtures.SeedTenantFK(t, db, tenantID)
	orgID := uuid.New().String()
	siteID := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{ID: siteID, TenantID: tenantID, OrgID: orgID, Name: "网点2527R8", Status: "active"}).Error)

	visibleID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: visibleID, IAMSub: visibleID, TenantID: tenantID, OrgID: orgID,
		Username: "visible2527", Name: "可见成员", Email: "visible2527@example.com",
		Role: "site_member", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.New().String(), TenantID: tenantID, SiteID: siteID, UserID: visibleID,
		Role: "site_member", Roles: []string{"site_member"}, Status: "active",
	}).Error)

	customerID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: customerID, IAMSub: customerID, TenantID: tenantID, OrgID: tenantID,
		Username: "customer2527", Name: "纯顾客", Email: "customer2527@example.com",
		Role: "", Status: "active",
	}).Error)

	fake := installFakeIAM2527(t)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: tenantID, OrgID: tenantID, UserID: uuid.New().String(), Role: "ADMIN"}.InjectContext(c.Request.Context()))
		c.Next()
	})
	router.POST("/users/reset-password", (&UserStaffHandler{}).ResetPassword)

	body, _ := json.Marshal(map[string]interface{}{"user_ids": []string{visibleID, customerID}})
	req := httptest.NewRequest(http.MethodPost, "/users/reset-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-user-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Sent             int      `json:"sent"`
			SkippedInvisible []string `json:"skipped_invisible"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.EqualValues(t, 20000, resp.Code)
	require.Equal(t, 1, resp.Data.Sent)

	skippedCustomer := len(resp.Data.SkippedInvisible) == 1 && resp.Data.SkippedInvisible[0] == customerID
	subs := fake.resetSubIDs()
	fmt.Printf("INVISIBLE_CUSTOMER_SKIPPED=%d SKIPPED_INVISIBLE=%d IAM_IDS=%d\n",
		flag2527(skippedCustomer), len(resp.Data.SkippedInvisible), len(subs))
	require.True(t, skippedCustomer, "skipped_invisible=%v", resp.Data.SkippedInvisible)
	require.Equal(t, []string{visibleID}, subs)
}
