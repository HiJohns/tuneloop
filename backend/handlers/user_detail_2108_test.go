package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

// TestUserDetail_PersonnelTypeAndIntroLetter_2108：#2108 走查 3/4 的后端口径——
// userDetail 返回介绍信 URL（URL 化）与 personnel_type（#2064 staffExists 口径），
// 供前端「顾客资料区块对员工隐藏」与介绍信查看。
func TestUserDetail_PersonnelTypeAndIntroLetter_2108(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	customerID := uuid.New().String()
	memberID := uuid.New().String()
	techID := uuid.New().String()
	adminID := uuid.New().String()
	siteID := uuid.New().String()
	tid := uuid.New().String()

	mk := func(id string) {
		require.NoError(t, db.Create(&models.User{
			ID: id, IAMSub: id, TenantID: tid, OrgID: tid, Username: "u-" + id[:8], Name: "n-" + id[:4], Role: "USER", Status: "active",
		}).Error)
	}
	mk(customerID)
	mk(memberID)
	mk(techID)
	mk(adminID)

	introLetter := "media/intro-letters/" + customerID + "/letter.jpg"
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", customerID).
		Update("intro_letter_url", introLetter).Error)
	require.NoError(t, db.Create(&models.Site{ID: siteID, TenantID: tid, OrgID: tid, Name: "S", Type: "normal"}).Error)
	require.NoError(t, db.Create(&models.SiteMember{TenantID: tid, SiteID: siteID, UserID: memberID, Role: "STAFF", Status: "active"}).Error)
	require.NoError(t, db.Create(&models.TechnicianProfile{ID: uuid.New().String(), UserID: techID, TenantID: tid, Status: "active"}).Error)

	h := NewUserManagementHandler()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: "", OrgID: tid, UserID: adminID, Role: "ADMIN"}.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.GET("/admin/user-management/:id", h.Get)

	getDetail := func(id string) map[string]interface{} {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/user-management/"+id, nil))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp struct {
			Code int                    `json:"code"`
			Data map[string]interface{} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, 20000, resp.Code)
		return resp.Data
	}

	// 顾客：customer + 介绍信 URL 回显
	cDetail := getDetail(customerID)
	assert.Equal(t, "customer", cDetail["personnel_type"])
	assert.Contains(t, cDetail["intro_letter_url"], "intro-letters")

	// 网点成员：staff
	assert.Equal(t, "staff", getDetail(memberID)["personnel_type"])
	// 师傅：technician_profiles 口径 = staff
	assert.Equal(t, "staff", getDetail(techID)["personnel_type"])
}
