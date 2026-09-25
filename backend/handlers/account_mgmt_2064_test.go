package handlers

import (
	"encoding/json"
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

// #2064 账户管理：personnel_type 筛选（全部/顾客/员工+管理员）
func TestUserManagement2064_PersonnelTypeFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tid, org, siteID := uuid.New().String(), uuid.New().String(), uuid.New().String()
	customerID, memberID, techID, adminID := uuid.New().String(), uuid.New().String(), uuid.New().String(), uuid.New().String()

	mk := func(id, role string) {
		require.NoError(t, db.Create(&models.User{
			ID: id, IAMSub: id, TenantID: tid, OrgID: org, Username: "u-" + id[:8], Name: "n-" + id[:4], Role: role, Status: "active",
		}).Error)
	}
	mk(adminID, "admin")
	mk(customerID, "USER")
	mk(memberID, "member")
	mk(techID, "member")
	require.NoError(t, db.Create(&models.Site{ID: siteID, TenantID: tid, OrgID: tid, Name: "S", Type: "normal"}).Error)
	require.NoError(t, db.Create(&models.SiteMember{TenantID: tid, SiteID: siteID, UserID: memberID, Role: "STAFF", Status: "active"}).Error)
	require.NoError(t, db.Create(&models.TechnicianProfile{ID: uuid.New().String(), UserID: techID, TenantID: tid, Status: "active"}).Error)

	h := NewUserManagementHandler()
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			testutil.TestActor{TenantID: "", OrgID: org, UserID: adminID, Role: "ADMIN"}.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.GET("/admin/user-management", h.List)

	listIDs := func(query string) map[string]bool {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/user-management?"+query, nil))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp struct {
			Code int `json:"code"`
			Data struct {
				List []map[string]interface{} `json:"list"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		m := map[string]bool{}
		for _, row := range resp.Data.List {
			if id, ok := row["id"].(string); ok {
				m[id] = true
			}
		}
		return m
	}

	all := listIDs("page=1&pageSize=50")
	assert.True(t, all[customerID] && all[memberID] && all[techID], "全部：三类都在")

	staff := listIDs("page=1&pageSize=50&personnel_type=staff")
	assert.True(t, staff[memberID], "员工+管理员：含网点成员")
	assert.True(t, staff[techID], "员工+管理员：含师傅")
	assert.False(t, staff[customerID], "员工+管理员：不含纯顾客")

	cust := listIDs("page=1&pageSize=50&personnel_type=customer")
	assert.True(t, cust[customerID], "顾客：含纯顾客")
	assert.False(t, cust[memberID], "顾客：不含网点成员")
	assert.False(t, cust[techID], "顾客：不含师傅")
}

// #2064 固化现状：platform-staff 端点级 requireSystemAdmin（非 system_admin → 40300）
func TestPlatformStaff2064_SystemAdminGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_ = testfixtures.SetupTestDB(t)
	root := uuid.New().String()
	h := &PlatformStaffHandler{}
	r := gin.New()
	r.GET("/admin/platform-staff", func(c *gin.Context) {
		var actor testutil.TestActor
		if c.Query("actor") == "site" {
			actor = testutil.TestActor{TenantID: uuid.New().String(), OrgID: uuid.New().String(), UserID: uuid.New().String(), Role: "ADMIN"}
		} else {
			actor = testutil.TestActor{TenantID: "", OrgID: root, UserID: uuid.New().String(), Role: "ADMIN"}
		}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	}, h.List)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/platform-staff?actor=site", nil))
	assert.Equal(t, http.StatusForbidden, w.Code, "site_admin 不可访问平台员工")
	assert.Contains(t, w.Body.String(), "40300")

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/admin/platform-staff?actor=system", nil))
	assert.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
}
