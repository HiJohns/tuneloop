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

// #1884: /site-members/me must expose per-site memberships (site_id + role) so
// the mobile UI can gate panels by the request's site — not just by role name.

func TestGetMyRoles_IncludesSites(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	router := gin.New()
	router.GET("/site-members/me", GetMyRoles)

	tenantID, orgID, _ := testfixtures.NewTenantIDs("1884a1b2c3d4")
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "u-1884", Name: "员工", Status: "active",
	}).Error)

	siteA := uuid.New().String()
	siteB := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{ID: siteA, TenantID: tenantID, OrgID: orgID, Name: "网点A"}).Error)
	require.NoError(t, db.Create(&models.Site{ID: siteB, TenantID: tenantID, OrgID: orgID, Name: "网点B"}).Error)
	require.NoError(t, db.Create(&models.SiteMember{TenantID: tenantID, SiteID: siteA, UserID: userID, Role: "site_member", Status: "active"}).Error)
	require.NoError(t, db.Create(&models.SiteMember{TenantID: tenantID, SiteID: siteB, UserID: userID, Role: "repair_technician", Status: "active"}).Error)

	actor := testutil.MakeSiteMember(tenantID, orgID, userID)
	req := httptest.NewRequest(http.MethodGet, "/site-members/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req.WithContext(actor.InjectContext(req.Context())))
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Roles []string `json:"roles"`
			Sites []struct {
				SiteID string `json:"site_id"`
				Role   string `json:"role"`
			} `json:"sites"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)

	assert.ElementsMatch(t, []string{"site_member", "repair_technician"}, resp.Data.Roles)
	require.Len(t, resp.Data.Sites, 2)
	siteMap := map[string]string{}
	for _, s := range resp.Data.Sites {
		siteMap[s.SiteID] = s.Role
	}
	assert.Equal(t, "site_member", siteMap[siteA])
	assert.Equal(t, "repair_technician", siteMap[siteB])
}
