package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
)

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
		Username: "rn-openid-ok", WxOpenid: "mock-openid-123", Status: "active",
	}).Error)

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
