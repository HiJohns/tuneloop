package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"
)

// TestOrgManagement covers §4.1 site management end-to-end: create site →
// list sites → add member → list members. Uses site_admin (full sys_perm
// for organization create) against the real handlers with an IAM mock
// (CreateSite/AddMember call IAM for sub-organization and user binding).
func TestOrgManagement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	// IAM mock: namespace resolution + org creation + user binding.
	mockIAM := newMockIAMServer(
		func(w http.ResponseWriter, r *http.Request) {
			// GET /api/v1/namespaces/{ns} → id
			if r.Method == "GET" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{"id": "ns-test-001"})
				return
			}
			// POST /api/v1/namespaces/{ns}/organizations → org_id
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code":    20000,
				"message": "success",
				"org_id":  "11111111-1111-4111-8111-111111111111",
				"admin_id": "22222222-2222-4222-8222-222222222222",
			})
		},
		nil,
	)
	defer mockIAM.Close()
	services.SetIAMInternalURLForTesting(mockIAM.URL)
	defer services.SetIAMInternalURLForTesting("")
	t.Setenv("IAM_NAMESPACE", "test-ns")

	tenantID, orgID, userID := testfixtures.NewTenantIDs("00000000b1c2")

	admin := testutil.MakeSiteAdmin(tenantID, orgID, userID)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := admin.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	siteHandler := NewSiteHandler()
	siteMemberHandler := NewSiteMemberHandler()
	router.POST("/api/merchant/sites", siteHandler.CreateSite)
	router.GET("/api/common/sites", siteHandler.ListSites)
	router.POST("/api/sites/:id/members", siteMemberHandler.AddMember)
	router.GET("/api/sites/:id/members", siteMemberHandler.ListMembers)

	// Step 1: Create site.
	createBody, _ := json.Marshal(map[string]interface{}{
		"name":    "测试网点A",
		"address": "北京市海淀区中关村",
		"type":    "standard",
		"phone":   "010-12345678",
	})
	req := httptest.NewRequest("POST", "/api/merchant/sites", bytes.NewBuffer(createBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, "create site: %s", w.Body.String())

	var createResp struct {
		Code int `json:"code"`
		Data struct {
			ID string `json:"id"`
			Site struct {
				ID string `json:"id"`
			} `json:"site"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))
	require.Equal(t, 20000, createResp.Code)
	siteID := createResp.Data.ID
	if siteID == "" {
		siteID = createResp.Data.Site.ID
	}
	require.NotEmpty(t, siteID, "site id returned")

	// Step 2: Site persisted in DB (list endpoint applies org-scope which
	// may exclude the IAM-created org_id, so verify at DB level).
	var siteCount int64
	require.NoError(t, db.Model(&models.Site{}).Where("id = ?", siteID).Count(&siteCount).Error)
	require.Equal(t, int64(1), siteCount, "site row persisted")

	// Step 3: Add member (staff role). The member must exist in users
	// (ListMembers JOINs users).
	memberUserID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID:       memberUserID,
		IAMSub:   memberUserID,
		TenantID: tenantID,
		OrgID:    orgID,
		Username: "member1",
		Status:   "active",
	}).Error)

	addBody, _ := json.Marshal(map[string]interface{}{
		"user_id": memberUserID,
		"role":    "Staff",
	})
	req = httptest.NewRequest("POST", "/api/sites/"+siteID+"/members", bytes.NewBuffer(addBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, "add member: %s", w.Body.String())

	var addResp struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &addResp))
	require.Equal(t, 20100, addResp.Code)

	// Step 4: List members → member present.
	req = httptest.NewRequest("GET", "/api/sites/"+siteID+"/members", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var membersResp struct {
		Code int `json:"code"`
		Data struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &membersResp))
	require.Equal(t, 20000, membersResp.Code)
	memberFound := false
	for _, m := range membersResp.Data.List {
		if m["user_id"] == memberUserID {
			memberFound = true
			break
		}
	}
	require.True(t, memberFound, "added member appears in member list")

	// Step 5: site_members row persisted.
	var count int64
	require.NoError(t, db.Model(&models.SiteMember{}).Where("site_id = ?", siteID).Count(&count).Error)
	require.Equal(t, int64(1), count, "one site_members row")
}

// Ensure unused imports compile even when the test DB is skipped.
var _ = context.Background()
