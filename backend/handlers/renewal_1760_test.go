package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// setupRenewalIAMStub 提供 #2016 之后的 IAM openid 权威源桩：
// GET /api/v1/internal/users/:id/wx-openid → {"openid": ...}。
// 空 openid 表示该用户未绑定微信（仍返回 200 + 空值，调用方给出 40002）。
func setupRenewalIAMStub(t *testing.T, userID, openid string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/auth/token":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "mock-client-token", "expires_in": 3600, "token_type": "Bearer",
			})
		case strings.HasPrefix(r.URL.Path, "/api/v1/internal/users/") && strings.HasSuffix(r.URL.Path, "/wx-openid"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/internal/users/"), "/wx-openid")
			if id == userID {
				json.NewEncoder(w).Encode(map[string]string{"openid": openid})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	services.SetIAMInternalURLForTesting(srv.URL)
	t.Cleanup(func() {
		srv.Close()
		services.SetIAMInternalURLForTesting("")
	})
}

// #1760 regression tests: ConfirmRenewal must resolve the payer openid
// server-side (wx_user_bindings/users.wx_openid) when the client sends an
// empty open_id — and return a clear 40002 when the user has none, never
// a 500 from WeChat.

// TestRenewal_Confirm_OpenIDBackfilled (#1760): user has wx_openid →
// empty open_id in request succeeds and the JSAPI order is created.
func TestRenewal_Confirm_OpenIDBackfilled(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	testfixtures.SetupWechatPayMock(t)

	db := database.GetDB()
	tenantID := "00000000-0000-0000-0000-00000000d101"
	userID := "00000000-0000-0000-0000-00000000d102"
	orgID := "00000000-0000-0000-0000-00000000d103"
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "rn-openid-ok", Status: "active",
	}).Error)
	// #2016: openid authority is IAM, not users.wx_openid.
	setupRenewalIAMStub(t, userID, "openid-renewal-001")

	_, orderID := setupRenewalOrder(t, tenantID, userID, orgID, 0)
	router := renewalRouter(tenantID, userID)

	// open_id omitted entirely (frontend sends open_id: '')
	body, _ := json.Marshal(map[string]interface{}{
		"additional_days": 3,
		"open_id":         "",
	})
	req := httptest.NewRequest("POST", "/api/orders/"+orderID+"/renewal/confirm", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Success bool `json:"success"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code, w.Body.String())
	require.True(t, resp.Data.Success, "renewal confirm must succeed with server-resolved openid")
}

// TestRenewal_Confirm_NoOpenID_40002 (#1760): user without wx_openid →
// explicit 40002 message, NOT a 500.
func TestRenewal_Confirm_NoOpenID_40002(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	testfixtures.SetupWechatPayMock(t)

	db := database.GetDB()
	tenantID := "00000000-0000-0000-0000-00000000d201"
	userID := "00000000-0000-0000-0000-00000000d202"
	orgID := "00000000-0000-0000-0000-00000000d203"
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "rn-openid-none", Status: "active",
	}).Error)
	setupRenewalIAMStub(t, userID, "")

	_, orderID := setupRenewalOrder(t, tenantID, userID, orgID, 0)
	router := renewalRouter(tenantID, userID)

	body, _ := json.Marshal(map[string]interface{}{
		"additional_days": 3,
		"open_id":         "",
	})
	req := httptest.NewRequest("POST", "/api/orders/"+orderID+"/renewal/confirm", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "must be 400, not 500")
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 40002, resp.Code)
	require.Contains(t, resp.Message, "绑定微信", "clear guidance for unbound users")
}

var _ = uuid.New
