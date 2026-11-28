package handlers

// #2604: 25 binding points (旧写法：`"invalid request: "` 与 `err.Error()`
// 直接相加) now funnel into the shared bindmsg message builder. The response must
// name the offending field in Chinese; raw
// Go/JSON wording (`invalid character '\x17' in string`, `cannot unmarshal …`,
// `Field validation …`) must never reach the client, and the business code must
// stay exactly what it was before the fix (40000/40001/40002 are not unified).
//
// Each endpoint is driven twice through the real gin handler:
//   - malformed body (raw control character) → 请求体格式不正确，请检查后重试
//   - missing required field / wrong type     → 参数不合法：<称呼>…
//
// Assertions are hard: HTTP 400, unchanged code, no ASCII letters, no raw
// decoder phrase, exact message equality.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/bindmsg"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

const bind2604SyntaxBody = "{\"probe\":\"\x17\"}"

type bind2604Endpoint struct {
	key         string // token key for the summary lines
	name        string
	method      string
	route       string
	path        string
	handler     gin.HandlerFunc
	code        int    // business code, unchanged by the fix
	body        string // body exercising one required field / wrong type
	wantMsg     string
	systemAdmin bool // needs BusinessRoleSystemAdmin (tenant-less actor)
}

func bind2604HasASCIILetter(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

var bind2604RawPhrases = []string{
	"invalid character",
	"invalid request",
	"invalid parameters",
	"cannot unmarshal",
	"Field validation",
	"json:",
	"unexpected end of JSON",
	"EOF",
}

func bind2604LeaksRawPhrase(s string) (string, bool) {
	lower := strings.ToLower(s)
	for _, phrase := range bind2604RawPhrases {
		if strings.Contains(lower, strings.ToLower(phrase)) {
			return phrase, true
		}
	}
	return "", false
}

func TestBindErrorMessages2604(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testfixtures.SetupTestDB(t)
	if db == nil {
		return // GetTestDB already failed/skipped
	}

	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	userID := uuid.New().String()

	// Local row for the endpoints that resolve the caller before binding
	// (user_address / user_onboarding / guarantor use EnsureLocalUser).
	require.NoError(t, db.Create(&models.User{
		ID:       uuid.New().String(),
		IAMSub:   userID,
		TenantID: tenantID,
		OrgID:    orgID,
		Name:     "绑定文案复验",
		Status:   "active",
	}).Error)

	authHandler := &AuthHandler{}
	confirmationHandler := NewConfirmationSessionHandler()
	registrationHandler := NewRegistrationSessionHandler(db)
	faceVerifyHandler := NewFaceVerifyHandler(nil)
	permissionHandler := NewPermissionManageHandler(db, nil, nil)
	roleHandler := NewRoleManageHandler(db, nil, nil)
	platformStaffHandler := &PlatformStaffHandler{}
	addressHandler := NewUserAddressHandler()
	userMgmtHandler := NewUserManagementHandler()
	onboardingHandler := NewUserOnboardingHandler()
	guarantorHandler := NewGuarantorHandler(nil)

	userPathID := uuid.New().String()

	endpoints := []bind2604Endpoint{
		// auth.go (2)
		{name: "auth.PostLogin", method: http.MethodPost, route: "/t/auth/login", path: "/t/auth/login", handler: authHandler.PostLogin, code: 40002, body: `{"identifier":"13000000000"}`, wantMsg: "参数不合法：密码不能为空"},
		{key: "PERSON", name: "auth.PostRegister", method: http.MethodPost, route: "/t/auth/register", path: "/t/auth/register", handler: authHandler.PostRegister, code: 40002, body: `{"nickname":"复验","phone":"13000000000"}`, wantMsg: "参数不合法：姓名不能为空"},
		// confirmation_session.go (1)
		{name: "confirmation.Create", method: http.MethodPost, route: "/t/confirmation-sessions", path: "/t/confirmation-sessions", handler: confirmationHandler.Create, code: 40001, body: `{"user_id":"u","confirm_type":"email","action_type":"site_staff"}`, wantMsg: "参数不合法：确认目标不能为空"},
		// registration_session.go (2)
		{name: "registration.Create", method: http.MethodPost, route: "/t/auth/registration-sessions", path: "/t/auth/registration-sessions", handler: registrationHandler.CreateRegistrationSession, code: 40002, body: `{"nickname":"n","name":"张","phone":"13000000000","id_photos":"x"}`, wantMsg: "参数不合法：证件照类型不正确"},
		{name: "registration.Rebind", method: http.MethodPost, route: "/t/auth/registration-sessions/:id/rebind", path: "/t/auth/registration-sessions/" + userPathID + "/rebind", handler: registrationHandler.RebindRegistrationSession, code: 40002, body: `{"wx_code":123}`, wantMsg: "参数不合法：微信授权码类型不正确"},
		// face_verify.go (2)
		{name: "faceverify.Token", method: http.MethodPost, route: "/t/user/face-verify/token", path: "/t/user/face-verify/token", handler: faceVerifyHandler.Token, code: 40001, body: `{"name":"张三"}`, wantMsg: "参数不合法：身份证号不能为空"},
		{name: "faceverify.Result", method: http.MethodPost, route: "/t/user/face-verify/result", path: "/t/user/face-verify/result", handler: faceVerifyHandler.Result, code: 40001, body: `{}`, wantMsg: "参数不合法：核身凭证不能为空"},
		// permission_manage.go (2)
		{name: "permission.SetUserPermissions", method: http.MethodPut, route: "/t/iam/users/:id/permissions", path: "/t/iam/users/" + userPathID + "/permissions", handler: permissionHandler.SetUserPermissions, code: 40000, body: `{}`, wantMsg: "参数不合法：权限项列表不能为空"},
		{name: "permission.SetUserRole", method: http.MethodPut, route: "/t/iam/users/:id/role", path: "/t/iam/users/" + userPathID + "/role", handler: permissionHandler.SetUserRole, code: 40000, body: `{}`, wantMsg: "参数不合法：角色编码不能为空"},
		// role_manage.go (2)
		{key: "ROLE", name: "role.Create", method: http.MethodPost, route: "/t/admin/roles", path: "/t/admin/roles", handler: roleHandler.CreateRole, code: 40000, body: `{"name":"r","cus_perm_codes":["x"]}`, wantMsg: "参数不合法：角色编码不能为空"},
		{name: "role.Update", method: http.MethodPut, route: "/t/admin/roles/:id", path: "/t/admin/roles/" + userPathID, handler: roleHandler.UpdateRole, code: 40000, body: `{"name":123,"cus_perm_codes":["x"]}`, wantMsg: "参数不合法：角色名称类型不正确"},
		// platform_staff.go (1) — system-admin + own org
		{name: "platformstaff.Create", method: http.MethodPost, route: "/t/admin/platform-staff", path: "/t/admin/platform-staff", handler: platformStaffHandler.Create, code: 40002, body: `{"username":"staff"}`, wantMsg: "参数不合法：姓名不能为空", systemAdmin: true},
		// smtp_config.go (1) — system-admin
		{name: "smtp.Update", method: http.MethodPut, route: "/t/admin/smtp-config", path: "/t/admin/smtp-config", handler: UpdateSMTPConfig, code: 40002, body: `{"port":"x"}`, wantMsg: "参数不合法：端口类型不正确", systemAdmin: true},
		// user_address.go (2)
		{name: "address.Create", method: http.MethodPost, route: "/t/user/addresses", path: "/t/user/addresses", handler: addressHandler.CreateAddress, code: 40002, body: `{"province":123}`, wantMsg: "参数不合法：省份类型不正确"},
		{name: "address.Update", method: http.MethodPut, route: "/t/user/addresses/:id", path: "/t/user/addresses/" + userPathID, handler: addressHandler.UpdateAddress, code: 40002, body: `{"phone":123}`, wantMsg: "参数不合法：手机号类型不正确"},
		// user_management.go (1)
		{name: "usermgmt.Update", method: http.MethodPut, route: "/t/admin/user-management/:id", path: "/t/admin/user-management/" + userPathID, handler: userMgmtHandler.Update, code: 40002, body: `{"status":123}`, wantMsg: "参数不合法：状态类型不正确"},
		// user_management_idcard.go (1)
		{name: "usermgmt.UpdateIdCard", method: http.MethodPut, route: "/t/admin/user-management/:id/id-card", path: "/t/admin/user-management/" + userPathID + "/id-card", handler: userMgmtHandler.UpdateIdCard, code: 40002, body: `{"real_name":123}`, wantMsg: "参数不合法：真实姓名类型不正确"},
		// user_onboarding.go (1)
		{name: "onboarding.Complete", method: http.MethodPost, route: "/t/user/onboarding/complete", path: "/t/user/onboarding/complete", handler: onboardingHandler.CompleteOnboarding, code: 40002, body: `{"name":123}`, wantMsg: "参数不合法：姓名类型不正确"},
		// guarantor.go (2)
		{name: "guarantor.Create", method: http.MethodPost, route: "/t/user/guarantors", path: "/t/user/guarantors", handler: guarantorHandler.CreateGuarantor, code: 40002, body: `{"name":"张三","phone":"13000000000","id_photo_front":"a","id_photo_back":"b","other_cert_photo":"c"}`, wantMsg: "参数不合法：身份证号不能为空"},
		{name: "guarantor.OCR", method: http.MethodPost, route: "/t/user/idcard-ocr", path: "/t/user/idcard-ocr", handler: guarantorHandler.OCRIDCard, code: 40002, body: `{"storage_key":"k"}`, wantMsg: "参数不合法：证件面不能为空"},
		// instrument.go (1) — dive: `IDs[0]` must collapse to the slice label
		{key: "IDS", name: "instrument.BatchDelete", method: http.MethodDelete, route: "/t/instruments/batch", path: "/t/instruments/batch", handler: BatchDeleteInstruments, code: 40002, body: `{"ids":[""]}`, wantMsg: "参数不合法：乐器列表不能为空"},
		// instrument_media.go (1)
		{name: "instrumentmedia.Create", method: http.MethodPost, route: "/t/instruments/:id/media", path: "/t/instruments/" + userPathID + "/media", handler: CreateInstrumentMedia, code: 40002, body: `{"files":"x"}`, wantMsg: "参数不合法：文件列表类型不正确"},
		// pricing.go (2)
		{name: "pricing.UpdateMerchantConfig", method: http.MethodPut, route: "/t/merchant/pricing-config", path: "/t/merchant/pricing-config", handler: UpdateMerchantPricingConfig, code: 40001, body: `{"template_id":"t"}`, wantMsg: "参数不合法：配置项不能为空"},
		{name: "pricing.BatchSet", method: http.MethodPut, route: "/t/instruments/batch-pricing", path: "/t/instruments/batch-pricing", handler: BatchSetInstrumentPricing, code: 40001, body: `{}`, wantMsg: "参数不合法：乐器列表不能为空"},
		// billing.go (1)
		{name: "billing.UpsertSettlementConfig", method: http.MethodPut, route: "/t/admin/merchant/:id/settlement", path: "/t/admin/merchant/" + userPathID + "/settlement", handler: UpsertSettlementConfig, code: 40002, body: `{"profit_share_ratio":"x"}`, wantMsg: "参数不合法：分成比例类型不正确"},
	}

	require.Len(t, endpoints, 25, "25 binding points are收口 by #2604")

	staffActor := testutil.TestActor{TenantID: tenantID, OrgID: orgID, UserID: userID, Role: "site_member"}
	adminActor := testutil.TestActor{TenantID: "", OrgID: orgID, UserID: userID, Role: "ADMIN"}

	newRouter := func(ep bind2604Endpoint) *gin.Engine {
		actor := staffActor
		if ep.systemAdmin {
			actor = adminActor
		}
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
			c.Next()
		})
		router.Handle(ep.method, ep.route, ep.handler)
		return router
	}

	invoke := func(t *testing.T, ep bind2604Endpoint, body string) (int, int, string) {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(ep.method, ep.path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		newRouter(ep).ServeHTTP(rec, req)
		var resp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp), "response must be JSON: %s", rec.Body.String())
		return rec.Code, resp.Code, resp.Message
	}

	total := 0
	caseFail := 0
	asciiLeak := 0
	rawLeak := 0
	syntaxMsg := ""
	captured := map[string]string{}

	for _, ep := range endpoints {
		variants := []struct {
			kind    string
			body    string
			wantMsg string
		}{
			{"syntax", bind2604SyntaxBody, bindmsg.SyntaxMessage},
			{"field", ep.body, ep.wantMsg},
		}
		for _, variant := range variants {
			total++
			status, code, message := invoke(t, ep, variant.body)

			ok := true
			if status != http.StatusBadRequest {
				t.Errorf("%s/%s: HTTP %d, want 400 (message=%q)", ep.name, variant.kind, status, message)
				ok = false
			}
			if code != ep.code {
				t.Errorf("%s/%s: code %d, want %d (codes must not be unified)", ep.name, variant.kind, code, ep.code)
				ok = false
			}
			if bind2604HasASCIILetter(message) {
				t.Errorf("%s/%s: message contains ASCII letters: %q", ep.name, variant.kind, message)
				asciiLeak++
				ok = false
			}
			if phrase, leaked := bind2604LeaksRawPhrase(message); leaked {
				t.Errorf("%s/%s: message leaks raw phrase %q: %q", ep.name, variant.kind, phrase, message)
				rawLeak++
				ok = false
			}
			if message != variant.wantMsg {
				t.Errorf("%s/%s: message = %q, want %q", ep.name, variant.kind, message, variant.wantMsg)
				ok = false
			}
			if variant.kind == "syntax" && syntaxMsg == "" {
				syntaxMsg = message
			}
			if variant.kind == "field" && ep.key != "" {
				captured[ep.key] = message
			}
			if !ok {
				caseFail++
			}
		}
	}

	t.Logf("BIND2604_CASES_TOTAL=%d", total)
	t.Logf("BIND2604_CASES_FAIL=%d", caseFail)
	t.Logf("BIND2604_ASCII_LEAK=%d", asciiLeak)
	t.Logf("BIND2604_RAW_PHRASE_LEAK=%d", rawLeak)
	t.Logf("BIND2604_ROLE_CODE_MSG=%s", captured["ROLE"])
	t.Logf("BIND2604_PERSON_NAME_MSG=%s", captured["PERSON"])
	t.Logf("BIND2604_IDS_MSG=%s", captured["IDS"])
	t.Logf("BIND2604_SYNTAX_MSG=%s", syntaxMsg)

	if total != 50 {
		t.Errorf("BIND2604_CASES_TOTAL=%d, want 50", total)
	}
}
