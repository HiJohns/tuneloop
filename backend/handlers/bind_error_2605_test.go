package handlers

// #2605: the remaining 22 binding points (`invalid request body: ` / `Invalid
// request data: ` / `Invalid request: ` / `Invalid input: `) now funnel into
// `bindmsg.Message`. The response must name the offending field in Chinese; raw
// Go/JSON wording (`invalid character '\x17' in string`, `cannot unmarshal …`,
// `Field validation …`) must never reach the client, and the business code must
// stay exactly what it was before the fix (40000/40001 are not unified).
//
// Each endpoint is driven twice through the real gin handler:
//   - malformed body (raw control character) → 请求体格式不正确，请检查后重试
//   - missing required field / wrong type     → 参数不合法：<称呼>…
//
// Assertions are hard: HTTP 400, unchanged code, no ASCII letters, no raw
// decoder phrase, exact message equality; `CreateCategory` additionally must not
// leak the legacy `"error"` key.

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

const bind2605SyntaxBody = "{\"probe\":\"\x17\"}"

type bind2605Endpoint struct {
	idx         int // plan「22 处落点」序号
	name        string
	method      string
	route       string
	path        string
	handler     gin.HandlerFunc
	code        int    // business code, unchanged by the fix
	body        string // body exercising one required field / wrong type
	wantMsg     string
	key         string // token capture key for the summary lines
	systemAdmin bool   // needs BusinessRoleSystemAdmin (namespace admin actor)
	emptyUsers  bool   // handler requires users to be empty (setup/init)
	noErrorKey  bool   // response must not carry the legacy `error` key
}

func bind2605HasASCIILetter(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

var bind2605RawPhrases = []string{
	"invalid character",
	"invalid request",
	"invalid parameters",
	"cannot unmarshal",
	"field validation",
	"json:",
	"unexpected end of json",
	"invalid input",
	"invalid request data",
}

func bind2605LeaksRawPhrase(s string) (string, bool) {
	lower := strings.ToLower(s)
	for _, phrase := range bind2605RawPhrases {
		if strings.Contains(lower, phrase) {
			return phrase, true
		}
	}
	return "", false
}

func TestBindErrorMessages2605(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testfixtures.SetupTestDB(t)
	if db == nil {
		return // GetTestDB already failed/skipped
	}

	tenantID := uuid.NewString()
	orgID := uuid.NewString()
	userID := uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)

	// Visible staff row for UpdateUser (system admin + matching org).
	staffUserID := uuid.NewString()
	require.NoError(t, db.Create(&models.User{
		ID:       staffUserID,
		IAMSub:   uuid.NewString(),
		TenantID: tenantID,
		OrgID:    orgID,
		Name:     "绑定文案复验员工",
		Role:     "staff",
		Status:   "active",
	}).Error)

	// Guard fixtures: merchant / site / transit site / lease / deposit.
	merchantID := uuid.NewString()
	require.NoError(t, db.Create(&models.Merchant{
		ID:       merchantID,
		TenantID: tenantID,
		OrgID:    tenantID,
		Name:     "绑定文案复验商户",
		AdminUID: uuid.NewString(),
		Status:   "active",
	}).Error)

	siteID := uuid.NewString()
	require.NoError(t, db.Create(&models.Site{
		ID:       siteID,
		TenantID: tenantID,
		OrgID:    orgID,
		Name:     "绑定文案复验网点",
		Type:     "normal",
		Status:   "active",
	}).Error)

	transitSiteID := uuid.NewString()
	require.NoError(t, db.Create(&models.Site{
		ID:       transitSiteID,
		TenantID: tenantID,
		OrgID:    orgID,
		Name:     "绑定文案复验中转网点",
		Type:     "transit",
		Status:   "active",
	}).Error)

	// Lease / Deposit are not part of testfixtures.allTables (plan D11):
	// migrate and clean them locally instead of widening the shared fixture.
	require.NoError(t, db.AutoMigrate(&models.Lease{}, &models.Deposit{}))
	require.NoError(t, db.Exec("DELETE FROM deposits").Error)
	require.NoError(t, db.Exec("DELETE FROM leases").Error)

	leaseID := uuid.NewString()
	require.NoError(t, db.Create(&models.Lease{
		ID:            leaseID,
		TenantID:      tenantID,
		UserID:        staffUserID,
		InstrumentID:  uuid.NewString(),
		StartDate:     "2026-01-01",
		EndDate:       "2026-02-01",
		MonthlyRent:   models.FromYuan(1),
		DepositAmount: models.FromYuan(1),
		Status:        "active",
	}).Error)

	depositID := uuid.NewString()
	require.NoError(t, db.Create(&models.Deposit{
		ID:              depositID,
		TenantID:        tenantID,
		LeaseID:         leaseID,
		UserID:          staffUserID,
		Amount:          models.FromYuan(1),
		Type:            "rent",
		Status:          "pending",
		TransactionDate: "2026-01-01",
	}).Error)

	iamOrphanHandler := NewIAMOrphanHandler()
	userStaffHandler := &UserStaffHandler{}
	leaseHandler := NewLeaseHandler(db)
	depositHandler := NewDepositHandler(db)
	merchantHandler := NewMerchantHandler()
	merchantMemberHandler := NewMerchantMemberHandler()
	setupHandler := NewSetupHandler()
	siteMemberHandler := NewSiteMemberHandler()

	endpoints := []bind2605Endpoint{
		// B4 iam_orphan.go (1)
		{idx: 1, name: "iamorphan.Purge", method: http.MethodPost, route: "/t/iam/orphans/purge", path: "/t/iam/orphans/purge", handler: iamOrphanHandler.Purge, code: 40001, body: `{"user_ids":"x"}`, wantMsg: "参数不合法：用户列表类型不正确", systemAdmin: true},
		// B3 user_staff.go (5)
		{idx: 2, name: "userstaff.CreateUser", method: http.MethodPost, route: "/t/users", path: "/t/users", handler: userStaffHandler.CreateUser, code: 40001, body: `{"username":"staff"}`, wantMsg: "参数不合法：姓名不能为空"},
		{idx: 3, name: "userstaff.UpdateUser", method: http.MethodPut, route: "/t/users/:id", path: "/t/users/" + staffUserID, handler: userStaffHandler.UpdateUser, code: 40001, body: `{"name":123}`, wantMsg: "参数不合法：姓名类型不正确", systemAdmin: true},
		{idx: 4, name: "userstaff.UpdateCurrentUser", method: http.MethodPut, route: "/t/users/me", path: "/t/users/me", handler: userStaffHandler.UpdateCurrentUser, code: 40001, body: `{"nickname":123}`, wantMsg: "参数不合法：昵称类型不正确"},
		{idx: 5, name: "userstaff.WxBindCurrentUser", method: http.MethodPost, route: "/t/users/me/wx-bind", path: "/t/users/me/wx-bind", handler: userStaffHandler.WxBindCurrentUser, code: 40001, body: `{}`, wantMsg: "参数不合法：微信授权码不能为空", key: "WX_CODE"},
		{idx: 6, name: "userstaff.WxMergeCurrentUser", method: http.MethodPost, route: "/t/users/me/wx-merge", path: "/t/users/me/wx-merge", handler: userStaffHandler.WxMergeCurrentUser, code: 40001, body: `{"openid":"o"}`, wantMsg: "参数不合法：目标用户不能为空"},
		// B1 api.go (3) — CreateCategory also drops the legacy `error` key
		{idx: 7, name: "api.CreateCategory", method: http.MethodPost, route: "/t/categories", path: "/t/categories", handler: CreateCategory, code: 40001, body: `{"icon":"x"}`, wantMsg: "参数不合法：名称不能为空", noErrorKey: true},
		{idx: 8, name: "api.UpdateCategory", method: http.MethodPut, route: "/t/categories/:id", path: "/t/categories/" + uuid.NewString(), handler: UpdateCategory, code: 40001, body: `{"visible":"x"}`, wantMsg: "参数不合法：是否可见类型不正确"},
		{idx: 9, name: "api.UpdateCategorySort", method: http.MethodPut, route: "/t/categories/sort", path: "/t/categories/sort", handler: UpdateCategorySort, code: 40001, body: `{}`, wantMsg: "参数不合法：分类列表不能为空", key: "CATEGORY_ITEMS"},
		// B1 lease.go (4) — 40000 must stay 40000
		{idx: 10, name: "lease.CreateLease", method: http.MethodPost, route: "/t/leases", path: "/t/leases", handler: leaseHandler.CreateLease, code: 40000, body: `{"user_id":"u","start_date":"2026-01-01","end_date":"2026-02-01","monthly_rent":1,"deposit_amount":1}`, wantMsg: "参数不合法：乐器不能为空", key: "LEASE"},
		{idx: 11, name: "lease.UpdateLease", method: http.MethodPut, route: "/t/leases/:id", path: "/t/leases/" + leaseID, handler: leaseHandler.UpdateLease, code: 40000, body: `{"monthly_rent":"x"}`, wantMsg: "参数不合法：月租金类型不正确"},
		{idx: 12, name: "lease.CreateDeposit", method: http.MethodPost, route: "/t/deposits", path: "/t/deposits", handler: depositHandler.CreateDeposit, code: 40000, body: `{"lease_id":"l","user_id":"u","type":"rent","transaction_date":"2026-01-01"}`, wantMsg: "参数不合法：金额不能为空"},
		{idx: 13, name: "lease.UpdateDeposit", method: http.MethodPut, route: "/t/deposits/:id", path: "/t/deposits/" + depositID, handler: depositHandler.UpdateDeposit, code: 40000, body: `{"status":123}`, wantMsg: "参数不合法：状态类型不正确"},
		// B2 merchant.go (2)
		{idx: 14, name: "merchant.CreateMerchant", method: http.MethodPost, route: "/t/merchants", path: "/t/merchants", handler: merchantHandler.CreateMerchant, code: 40001, body: `{"phone":"1"}`, wantMsg: "参数不合法：名称不能为空"},
		{idx: 15, name: "merchant.UpdateMerchant", method: http.MethodPut, route: "/t/merchants/:id", path: "/t/merchants/" + merchantID, handler: merchantHandler.UpdateMerchant, code: 40001, body: `{"transit_phone":123}`, wantMsg: "参数不合法：中转联系电话类型不正确"},
		// B2 merchant_member.go (3)
		{idx: 16, name: "merchantmember.AddMember", method: http.MethodPost, route: "/t/merchants/:id/members", path: "/t/merchants/" + merchantID + "/members", handler: merchantMemberHandler.AddMember, code: 40001, body: `{"new_users":"x"}`, wantMsg: "参数不合法：新成员列表类型不正确"},
		{idx: 17, name: "merchantmember.UpdateMemberRole", method: http.MethodPut, route: "/t/merchants/:id/members/:uid", path: "/t/merchants/" + merchantID + "/members/" + uuid.NewString(), handler: merchantMemberHandler.UpdateMemberRole, code: 40001, body: `{}`, wantMsg: "参数不合法：角色不能为空"},
		{idx: 18, name: "merchantmember.TransferOwnership", method: http.MethodPost, route: "/t/merchants/:id/transfer-ownership", path: "/t/merchants/" + merchantID + "/transfer-ownership", handler: merchantMemberHandler.TransferOwnership, code: 40001, body: `{"user_id":123}`, wantMsg: "参数不合法：用户编号类型不正确"},
		// B2 setup.go (1) — requires users to be empty
		{idx: 19, name: "setup.InitializeSystem", method: http.MethodPost, route: "/t/setup/init", path: "/t/setup/init", handler: setupHandler.InitializeSystem, code: 40001, body: `{"email":"bad","password":"12345678"}`, wantMsg: "参数不合法：邮箱格式不正确", emptyUsers: true},
		// B2 site_member.go (2)
		{idx: 20, name: "sitemember.AddMember", method: http.MethodPost, route: "/t/sites/:id/members", path: "/t/sites/" + siteID + "/members", handler: siteMemberHandler.AddMember, code: 40001, body: `{"new_users":"x"}`, wantMsg: "参数不合法：新成员列表类型不正确"},
		{idx: 21, name: "sitemember.UpdateMemberRole", method: http.MethodPut, route: "/t/sites/:id/members/:uid", path: "/t/sites/" + siteID + "/members/" + uuid.NewString(), handler: siteMemberHandler.UpdateMemberRole, code: 40001, body: `{"roles":"x"}`, wantMsg: "参数不合法：角色列表类型不正确"},
		// B2 transit_site_admin.go (1)
		{idx: 22, name: "transit.AddTransitSiteMember", method: http.MethodPost, route: "/t/transit-sites/:id/members", path: "/t/transit-sites/" + transitSiteID + "/members", handler: AddTransitSiteMember, code: 40001, body: `{"user_ids":"x"}`, wantMsg: "参数不合法：用户列表类型不正确"},
	}

	require.Len(t, endpoints, 22, "22 binding points are closed by #2605")

	staffActor := testutil.TestActor{TenantID: tenantID, OrgID: orgID, UserID: userID, Role: "STAFF"}
	adminActor := testutil.TestActor{TenantID: tenantID, OrgID: orgID, UserID: userID, Role: "NAMESPACE_ADMIN"}

	newRouter := func(ep bind2605Endpoint) *gin.Engine {
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

	invoke := func(t *testing.T, ep bind2605Endpoint, body string) (int, int, string, string) {
		t.Helper()
		if ep.emptyUsers {
			require.NoError(t, db.Exec("DELETE FROM users").Error)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(ep.method, ep.path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		newRouter(ep).ServeHTTP(rec, req)
		var resp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp), "response must be JSON: %s", rec.Body.String())
		return rec.Code, resp.Code, resp.Message, rec.Body.String()
	}

	total := 0
	caseFail := 0
	asciiLeak := 0
	rawLeak := 0
	errorKeyLeak := 0
	syntaxMsg := ""
	captured := map[string]string{}

	for _, ep := range endpoints {
		variants := []struct {
			kind    string
			body    string
			wantMsg string
		}{
			{"syntax", bind2605SyntaxBody, bindmsg.SyntaxMessage},
			{"field", ep.body, ep.wantMsg},
		}
		for _, variant := range variants {
			total++
			status, code, message, rawBody := invoke(t, ep, variant.body)

			ok := true
			if status != http.StatusBadRequest {
				t.Errorf("#%d %s/%s: HTTP %d, want 400 (message=%q)", ep.idx, ep.name, variant.kind, status, message)
				ok = false
			}
			if code != ep.code {
				t.Errorf("#%d %s/%s: code %d, want %d (codes must not be unified)", ep.idx, ep.name, variant.kind, code, ep.code)
				ok = false
			}
			if bind2605HasASCIILetter(message) {
				t.Errorf("#%d %s/%s: message contains ASCII letters: %q", ep.idx, ep.name, variant.kind, message)
				asciiLeak++
				ok = false
			}
			if phrase, leaked := bind2605LeaksRawPhrase(message); leaked {
				t.Errorf("#%d %s/%s: message leaks raw phrase %q: %q", ep.idx, ep.name, variant.kind, phrase, message)
				rawLeak++
				ok = false
			}
			if message != variant.wantMsg {
				t.Errorf("#%d %s/%s: message = %q, want %q", ep.idx, ep.name, variant.kind, message, variant.wantMsg)
				ok = false
			}
			if ep.noErrorKey {
				var probe map[string]json.RawMessage
				if err := json.Unmarshal([]byte(rawBody), &probe); err == nil {
					if _, exists := probe["error"]; exists {
						t.Errorf("#%d %s/%s: response still carries the legacy error key: %s", ep.idx, ep.name, variant.kind, rawBody)
						errorKeyLeak++
						ok = false
					}
				}
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

	t.Logf("BIND2605_CASES_TOTAL=%d", total)
	t.Logf("BIND2605_CASES_FAIL=%d", caseFail)
	t.Logf("BIND2605_ASCII_LEAK=%d", asciiLeak)
	t.Logf("BIND2605_RAW_PHRASE_LEAK=%d", rawLeak)
	t.Logf("BIND2605_ERROR_KEY_LEAK=%d", errorKeyLeak)
	t.Logf("BIND2605_WX_CODE_MSG=%s", captured["WX_CODE"])
	t.Logf("BIND2605_CATEGORY_ITEMS_MSG=%s", captured["CATEGORY_ITEMS"])
	t.Logf("BIND2605_LEASE_MSG=%s", captured["LEASE"])
	t.Logf("BIND2605_SYNTAX_MSG=%s", syntaxMsg)

	if total != 44 {
		t.Errorf("BIND2605_CASES_TOTAL=%d, want 44", total)
	}
	require.Equal(t, 0, caseFail, "all binding cases must pass")
}
