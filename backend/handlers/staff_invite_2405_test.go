package handlers

// #2405：邀请接受按 kind 落本地行（merchant_staff/repair_technician）且幂等。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
	"tuneloop-backend/testutil"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupInvite2405(t *testing.T) (db *gorm.DB, tenantID, orgID string, merchant models.Merchant, invites []models.StaffInvite) {
	t.Helper()
	db = testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	tenantID = uuid.New().String()
	orgID = uuid.New().String()
	testfixtures.SeedTenantFK(t, db, tenantID, orgID)
	merchant = models.Merchant{ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, AdminUID: uuid.New().String(), Name: "M2405-" + orgID[:8], Status: "active"}
	require.NoError(t, db.Create(&merchant).Error)

	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"access_token": "t2405", "expires_in": 7200})
	})
	mux.HandleFunc("/api/v1/organizations/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"code": 20000})
	})
	mux.HandleFunc("/api/v1/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []interface{}{})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() { services.SetIAMInternalURLForTesting("") })
	t.Setenv("IAM_SECRET", testIAMSecret)
	return db, tenantID, orgID, merchant, nil
}

func inviteUser2405(t *testing.T, db *gorm.DB, tenantID, orgID string) *models.User {
	t.Helper()
	u := &models.User{ID: uuid.New().String(), IAMSub: uuid.New().String(), TenantID: orgID, OrgID: orgID, Username: "u2405_" + uuid.New().String()[:8], Name: "受邀", Status: "active"}
	require.NoError(t, db.Create(u).Error)
	return u
}

func TestAcceptInviteMerchantStaff_2405(t *testing.T) {
	db, tenantID, orgID, merchant, _ := setupInvite2405(t)
	u := inviteUser2405(t, db, tenantID, orgID)
	invite := models.StaffInvite{ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, Kind: "merchant_staff", Role: "merchant_staff", Code: "c2405a"}
	require.NoError(t, db.Create(&invite).Error)

	require.NoError(t, bindInvitee(db, &invite, u, inviteBindOpts{}))
	var mm models.MerchantMember
	require.NoError(t, db.Where("user_id = ?", u.ID).First(&mm).Error)
	assert.Equal(t, merchant.ID, mm.MerchantID)
	assert.Equal(t, "merchant_staff", mm.Role)
	assert.Equal(t, "active", mm.Status)
}

func TestAcceptInviteTechnician_2405(t *testing.T) {
	db, tenantID, orgID, merchant, _ := setupInvite2405(t)
	u := inviteUser2405(t, db, tenantID, orgID)
	invite := models.StaffInvite{ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, Kind: "repair_technician", Role: "repair_technician", Code: "c2405b"}
	require.NoError(t, db.Create(&invite).Error)

	require.NoError(t, bindInvitee(db, &invite, u, inviteBindOpts{}))
	var mm models.MerchantMember
	require.NoError(t, db.Where("user_id = ?", u.ID).First(&mm).Error)
	assert.Equal(t, merchant.ID, mm.MerchantID)
	assert.Equal(t, "site_member", mm.Role, "与创建路径一致")
	var tp models.TechnicianProfile
	require.NoError(t, db.Where("user_id = ?", u.ID).First(&tp).Error)
	assert.Equal(t, "active", tp.Status)
}

func TestAcceptInviteIdempotent_2405(t *testing.T) {
	db, tenantID, orgID, _, _ := setupInvite2405(t)
	u := inviteUser2405(t, db, tenantID, orgID)
	invite := models.StaffInvite{ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, Kind: "repair_technician", Role: "repair_technician", Code: "c2405c"}
	require.NoError(t, db.Create(&invite).Error)

	require.NoError(t, bindInvitee(db, &invite, u, inviteBindOpts{}))
	require.NoError(t, bindInvitee(db, &invite, u, inviteBindOpts{}))

	var mmCount, tpCount int64
	require.NoError(t, db.Model(&models.MerchantMember{}).Where("user_id = ?", u.ID).Count(&mmCount).Error)
	require.NoError(t, db.Model(&models.TechnicianProfile{}).Where("user_id = ?", u.ID).Count(&tpCount).Error)
	assert.EqualValues(t, 1, mmCount)
	assert.EqualValues(t, 1, tpCount)
}

func TestAcceptInviteSiteRegression_2405(t *testing.T) {
	db, tenantID, orgID, _, _ := setupInvite2405(t)
	site := models.Site{ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, Name: "S2405"}
	require.NoError(t, db.Create(&site).Error)
	u := inviteUser2405(t, db, tenantID, orgID)
	siteID := site.ID
	invite := models.StaffInvite{ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID, SiteID: &siteID, Kind: "site_member", Role: "site_member", Code: "c2405d"}
	require.NoError(t, db.Create(&invite).Error)

	require.NoError(t, bindInvitee(db, &invite, u, inviteBindOpts{}))
	var sm models.SiteMember
	require.NoError(t, db.Where("user_id = ? AND site_id = ?", u.ID, site.ID).First(&sm).Error)
	assert.Equal(t, "site_member", sm.Role)
}
