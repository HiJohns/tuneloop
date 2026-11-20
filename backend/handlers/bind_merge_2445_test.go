package handlers

// #2445: bind-QR X→Y merge (tuneloop side).
//   - ConfirmBind needs_merge stores the IAM merge ticket and exposes it.
//   - MergeBind completes the merge, marks the token merged, and migrates
//     local staff tables from X to Y (site_members/merchant_members/tech).
//   - needs_credentials is derived from IAM username==phone (P2 hint).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

func bindMergeTestRouter() *gin.Engine {
	r := gin.New()
	h := NewWechatBindHandler()
	r.POST("/wechat-bind/confirm", h.ConfirmBind)
	r.POST("/wechat-bind/merge", h.MergeBind)
	r.GET("/users/me/wechat-bind/:token", h.PollBindToken)
	return r
}

func TestMigrateMergedAccountLocal_2445(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	if db == nil {
		return
	}
	tenantID := uuid.NewString()
	orgID := uuid.NewString()
	siteA := uuid.NewString()
	siteB := uuid.NewString()
	require.NoError(t, db.Create(&models.Tenant{ID: tenantID, Name: "T2445", Status: "active"}).Error)
	require.NoError(t, db.Create(&models.Site{ID: siteA, TenantID: tenantID, OrgID: orgID, Name: "SA2445", Type: "normal"}).Error)
	require.NoError(t, db.Create(&models.Site{ID: siteB, TenantID: tenantID, OrgID: orgID, Name: "SB2445", Type: "normal"}).Error)

	x := models.User{
		ID: uuid.NewString(), IAMSub: "iam-X-2445", TenantID: tenantID, OrgID: orgID,
		Name: "X2445", Phone: "13800000001", Status: "active",
	}
	y := models.User{
		ID: uuid.NewString(), IAMSub: "iam-Y-2445", TenantID: tenantID, OrgID: orgID,
		Name: "Y2445", Phone: "13800000002", Status: "active",
	}
	require.NoError(t, db.Create(&x).Error)
	require.NoError(t, db.Create(&y).Error)

	// Same site already has Y: dedupe + union roles.
	require.NoError(t, db.Create(&models.SiteMember{
		TenantID: tenantID, SiteID: siteA, UserID: x.ID, Role: "site_member", Roles: []string{"site_member", "repair_technician"}, Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		TenantID: tenantID, SiteID: siteA, UserID: y.ID, Role: "site_admin", Roles: []string{"site_admin"}, Status: "active",
	}).Error)
	// Site B only on X: row moves to Y.
	require.NoError(t, db.Create(&models.SiteMember{
		TenantID: tenantID, SiteID: siteB, UserID: x.ID, Role: "site_admin", Roles: []string{"site_admin"}, Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.MerchantMember{
		TenantID: tenantID, MerchantID: uuid.NewString(), UserID: x.ID, Role: "site_member", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.TechnicianProfile{
		ID: uuid.NewString(), UserID: x.ID, TenantID: tenantID, Status: "active",
	}).Error)

	require.NoError(t, migrateMergedAccountLocal(db, "iam-X-2445", "iam-Y-2445"))

	var gotX, gotY models.User
	require.NoError(t, db.Where("id = ?", x.ID).First(&gotX).Error)
	require.NoError(t, db.Where("id = ?", y.ID).First(&gotY).Error)
	assert.Equal(t, "obsolete", gotX.Status)
	assert.Equal(t, "active", gotY.Status)

	var siteARows []models.SiteMember
	require.NoError(t, db.Where("site_id = ?", siteA).Find(&siteARows).Error)
	require.Len(t, siteARows, 1, "duplicate site membership must be deduped")
	assert.Equal(t, y.ID, siteARows[0].UserID)
	assert.ElementsMatch(t, []string{"site_member", "repair_technician", "site_admin"}, siteARows[0].Roles)

	var siteBRow models.SiteMember
	require.NoError(t, db.Where("site_id = ?", siteB).First(&siteBRow).Error)
	assert.Equal(t, y.ID, siteBRow.UserID)

	var mm models.MerchantMember
	require.NoError(t, db.Where("user_id = ?", y.ID).First(&mm).Error)
	assert.Equal(t, y.ID, mm.UserID)

	var tp models.TechnicianProfile
	require.NoError(t, db.Where("user_id = ? AND tenant_id = ?", y.ID, tenantID).First(&tp).Error)
	assert.Equal(t, y.ID, tp.UserID)

	// Idempotent second run.
	require.NoError(t, migrateMergedAccountLocal(db, "iam-X-2445", "iam-Y-2445"))
}

func bindMergeMockIAM(t *testing.T, mergeTicket string, from, to string, captureBind *map[string]interface{}) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"access_token": "mock-service-token", "expires_in": 3600, "token_type": "Bearer"})
	})
	mux.HandleFunc("/api/v1/auth/wx-bind", func(w http.ResponseWriter, r *http.Request) {
		if captureBind != nil {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			*captureBind = body
		}
		writeJSON(w, map[string]interface{}{
			"needs_merge":      true,
			"openid":           "openid-2445",
			"existing_user_id": to,
			"target_user_id":   from,
			"merge_ticket":     mergeTicket,
		})
	})
	mux.HandleFunc("/api/v1/auth/wx-merge-confirm", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			MergeTicket string `json:"merge_ticket"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.MergeTicket != mergeTicket {
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, map[string]string{"error": "invalid or expired merge ticket"})
			return
		}
		writeJSON(w, map[string]interface{}{
			"merged":       true,
			"from_user_id": from,
			"to_user_id":   to,
			"openid":       "openid-2445",
		})
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"user": map[string]interface{}{
				"id": to, "username": "13900002445", "phone": "13900002445",
				"name": "Y", "status": "active",
			},
		})
	})
	return httptest.NewServer(mux)
}

func TestBindMergeFlow_2445(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	if db == nil {
		return
	}
	tenantID := uuid.NewString()
	orgID := uuid.NewString()
	siteID := uuid.NewString()
	require.NoError(t, db.Create(&models.Tenant{ID: tenantID, Name: "T2445b", Status: "active"}).Error)
	require.NoError(t, db.Create(&models.Site{ID: siteID, TenantID: tenantID, OrgID: orgID, Name: "S2445b", Type: "normal"}).Error)

	x := models.User{
		ID: uuid.NewString(), IAMSub: "iam-X-2445b", TenantID: tenantID, OrgID: orgID,
		Name: "X", Phone: "13800000011", Status: "active",
	}
	y := models.User{
		ID: uuid.NewString(), IAMSub: "iam-Y-2445b", TenantID: tenantID, OrgID: orgID,
		Name: "Y", Phone: "13800000012", Status: "active",
	}
	require.NoError(t, db.Create(&x).Error)
	require.NoError(t, db.Create(&y).Error)
	require.NoError(t, db.Create(&models.SiteMember{
		TenantID: tenantID, SiteID: siteID, UserID: x.ID, Role: "site_admin", Roles: []string{"site_admin"}, Status: "active",
	}).Error)

	ticket := uuid.NewString()
	from, to := x.IAMSub, y.IAMSub
	srv := bindMergeMockIAM(t, ticket, from, to, nil)
	defer srv.Close()
	services.SetIAMInternalURLForTesting(srv.URL)
	defer services.SetIAMInternalURLForTesting("")

	const token = "0123456789abcdee"
	bindTokensMu.Lock()
	bindTokens[token] = &bindTokenEntry{UserID: x.ID, Status: "pending", CreatedAt: time.Now()}
	bindTokensMu.Unlock()
	defer func() {
		bindTokensMu.Lock()
		delete(bindTokens, token)
		bindTokensMu.Unlock()
	}()

	router := bindMergeTestRouter()

	// 1) ConfirmBind → needs_merge + ticket captured.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/wechat-bind/confirm",
		strings.NewReader(`{"token":"`+token+`","exchange_token":"ex-2445"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusConflict, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), `"needs_merge":true`)
	assert.Contains(t, w.Body.String(), ticket)

	bindTokensMu.RLock()
	entry := bindTokens[token]
	status := entry.Status
	storedTicket := entry.MergeTicket
	bindTokensMu.RUnlock()
	assert.Equal(t, "needs_merge", status)
	assert.Equal(t, ticket, storedTicket)

	// 2) MergeBind → merged + local migration + credential hint.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/wechat-bind/merge",
		strings.NewReader(`{"token":"`+token+`","merge_ticket":"`+ticket+`"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), `"merged":true`)
	assert.Contains(t, w.Body.String(), `"needs_credentials":true`)

	var gotX models.User
	require.NoError(t, db.Where("id = ?", x.ID).First(&gotX).Error)
	assert.Equal(t, "obsolete", gotX.Status)
	var moved models.SiteMember
	require.NoError(t, db.Where("site_id = ?", siteID).First(&moved).Error)
	assert.Equal(t, y.ID, moved.UserID)

	bindTokensMu.RLock()
	assert.Equal(t, "merged", bindTokens[token].Status)
	bindTokensMu.RUnlock()

	// 3) Idempotent repeat (PC poll also sees merged).
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/wechat-bind/merge",
		strings.NewReader(`{"token":"`+token+`","merge_ticket":"`+ticket+`"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"merged":true`)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/users/me/wechat-bind/"+token, nil)
	router.ServeHTTP(w, req)
	assert.Contains(t, w.Body.String(), `"status":"merged"`)

	// sanity: DB context is the one MergeBind used
	assert.NotNil(t, database.GetDB())
	_ = context.Background()
}
