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

	mkUser := func(name, org string) {
		sub := uuid.New().String()
		require.NoError(t, db.Create(&models.User{
			ID: sub, IAMSub: sub, TenantID: tenantID, OrgID: org,
			Username: name, Name: name, Status: "active",
		}).Error)
	}
	mkUser("平台员工甲", orgID)
	mkUser("平台员工乙", orgID)
	mkUser("其他组织成员", uuid.New().String())

	router := gin.New()
	router.GET("/admin/platform-staff", (&PlatformStaffHandler{}).List)

	actor := testutil.MakeSiteAdmin(tenantID, orgID, uuid.New().String())
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
	assert.Len(t, resp.Data.List, 2, "only members of the operator's org are listed")
}
