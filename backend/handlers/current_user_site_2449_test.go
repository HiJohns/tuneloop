package handlers

// #2449: /users/me current site follows JWT org context (not first site row).

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
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"
)

func TestGetCurrentUser_SiteByJWTOrg_2449(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	if db == nil {
		return
	}
	iamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/auth/token" {
			_, _ = w.Write([]byte(`{"access_token":"t2449","expires_in":3600}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer iamSrv.Close()
	services.SetIAMInternalURLForTesting(iamSrv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	tenantID := uuid.NewString()
	userID := uuid.NewString()
	user := models.User{ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: tenantID, Username: "multi-site", Status: "active", IsShadow: false}
	require.NoError(t, db.Create(&user).Error)

	siteA, siteB := uuid.NewString(), uuid.NewString()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Create(&models.Site{ID: siteA, TenantID: tenantID, OrgID: orgA, Name: "SiteA", Status: "active", Type: "normal"}).Error)
	require.NoError(t, db.Create(&models.Site{ID: siteB, TenantID: tenantID, OrgID: orgB, Name: "SiteB", Status: "active", Type: "normal"}).Error)
	require.NoError(t, db.Create(&models.SiteMember{TenantID: tenantID, SiteID: siteA, UserID: userID, Role: "site_member", Status: "active"}).Error)
	require.NoError(t, db.Create(&models.SiteMember{TenantID: tenantID, SiteID: siteB, UserID: userID, Role: "site_member", Status: "active"}).Error)

	h := &UserStaffHandler{}
	call := func(orgID string) map[string]interface{} {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			actor := testutil.TestActor{UserID: userID, TenantID: tenantID, OrgID: orgID, Role: "STAFF"}
			c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
			c.Next()
		})
		r.GET("/users/me", h.GetCurrentUser)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp struct {
			Data map[string]interface{} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp.Data
	}

	assert.Equal(t, siteA, call(orgA)["site_id"])
	assert.Equal(t, "SiteA", call(orgA)["site_name"])
	assert.Equal(t, siteB, call(orgB)["site_id"])
	assert.Nil(t, call("")["site_id"], "customer context (oid empty) must not fall back")
	assert.Nil(t, call(uuid.NewString())["site_id"], "multi-site unmatched oid must not guess first site")

}

// R2 (#2449): staff list keeps compatibility fields and exposes all active site names.
// 独立用例，避免与 R1（/users/me）复用同一证据。
func TestListStaff_SiteNames_2449(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	if db == nil {
		return
	}
	tenantID := uuid.NewString()
	userID := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: tenantID,
		Username: "multi-site-list", Status: "active", IsShadow: false,
	}).Error)

	siteA, siteB := uuid.NewString(), uuid.NewString()
	orgA, orgB := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Create(&models.Site{ID: siteA, TenantID: tenantID, OrgID: orgA, Name: "SiteA", Status: "active", Type: "normal"}).Error)
	require.NoError(t, db.Create(&models.Site{ID: siteB, TenantID: tenantID, OrgID: orgB, Name: "SiteB", Status: "active", Type: "normal"}).Error)
	require.NoError(t, db.Create(&models.SiteMember{TenantID: tenantID, SiteID: siteA, UserID: userID, Role: "site_member", Status: "active"}).Error)
	require.NoError(t, db.Create(&models.SiteMember{TenantID: tenantID, SiteID: siteB, UserID: userID, Role: "site_member", Status: "active"}).Error)

	h := &UserStaffHandler{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		actor := testutil.MakeSiteAdmin(tenantID, tenantID, userID)
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.GET("/staff", h.ListStaff)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/staff?page=1&page_size=20", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var listResp struct {
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	require.NotEmpty(t, listResp.Data.List)
	var row map[string]interface{}
	for _, item := range listResp.Data.List {
		if item["id"] == userID {
			row = item
			break
		}
	}
	require.NotNil(t, row)
	require.NotNil(t, row["site_id"], "compat site_id must be present")
	require.NotNil(t, row["site_name"], "compat site_name must be present")
	require.ElementsMatch(t, []interface{}{"SiteA", "SiteB"}, row["site_names"], "site_names must list all active sites")
}
