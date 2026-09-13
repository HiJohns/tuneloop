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

// #1897: the platform-staff list is scoped to the OPERATOR's own org (the
// system-admin root org) instead of the PLATFORM_ROOT_ORG_ID env variable.
func TestListPlatformStaff_UsesOperatorOrg(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID, orgID, _ := testfixtures.NewTenantIDs("1897a1b2c3d4")

	mkUser := func(name, org, role string) {
		sub := uuid.New().String()
		require.NoError(t, db.Create(&models.User{
			ID: sub, IAMSub: sub, TenantID: tenantID, OrgID: org,
			Username: name, Name: name, Role: role, Status: "active",
		}).Error)
	}
	mkUser("平台员工甲", orgID, "STAFF")
	mkUser("平台员工乙", orgID, "staff") // role 大小写不敏感
	mkUser("系统管理员", orgID, "namespace_admin")
	mkUser("根组织顾客", orgID, "")                     // 空 role 不得混入
	mkUser("其他组织员工", uuid.New().String(), "STAFF") // 跨组织不可见

	router := gin.New()
	router.GET("/admin/platform-staff", (&PlatformStaffHandler{}).List)

	actor := testutil.MakeSiteAdmin(tenantID, orgID, uuid.New().String())
	actor.Role = "NAMESPACE_ADMIN" // #1897: only system admins may manage platform staff
	req := httptest.NewRequest(http.MethodGet, "/admin/platform-staff", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				Name string `json:"name"`
			} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	assert.Len(t, resp.Data.List, 3, "staff + system admin of the operator org only")
}

// #1897: platform-staff management is restricted to system administrators.
func TestListPlatformStaff_NonSystemAdmin_Forbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID, orgID, _ := testfixtures.NewTenantIDs("1897f1e2d3c4")
	_ = db

	router := gin.New()
	router.GET("/admin/platform-staff", (&PlatformStaffHandler{}).List)

	actor := testutil.MakeSiteAdmin(tenantID, orgID, uuid.New().String())
	req := httptest.NewRequest(http.MethodGet, "/admin/platform-staff", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))
	require.Equal(t, http.StatusForbidden, w.Code)
}
