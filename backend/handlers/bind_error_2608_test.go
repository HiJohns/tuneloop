package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/bindmsg"
	"tuneloop-backend/middleware"
)

// #2608: 14 binding points still returned the raw decoder text, so the user
// saw the generic 40002 fallback instead of the offending field. Every point
// must now go through the `bindmsg` exit (`Message`, or `MessageScoped` when
// the same JSON name means something else in that endpoint).
//
// Each endpoint is exercised twice:
//   - malformed body (raw control character) → 请求体格式不正确，请检查后重试
//   - missing required field / wrong type      → 参数不合法：<称呼>…
//
// All 14 branches return before any DB access, so this contract test needs no
// fixture — but `go test ./handlers` still goes through `TestMain`, which
// builds the isolated PostgreSQL schema (127.0.0.1:15433). A dead DB therefore
// fails loudly instead of silently skipping.

const bind2608SyntaxBody = "{\"probe\":\"\x17\"}"

type bind2608Endpoint struct {
	key     string // token key for the summary lines (DISCOUNT/…)
	name    string
	method  string
	route   string
	path    string
	handler gin.HandlerFunc
	body    string // body exercising a required field / wrong type
	wantMsg string // exact message for the field case
}

func bind2608HasASCIILetter(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

var bind2608RawPhrases = []string{
	"invalid character",
	"invalid request",
	"cannot unmarshal",
	"Field validation",
	"json:",
	"unexpected end of JSON",
	"EOF",
}

func bind2608LeaksRawPhrase(s string) (string, bool) {
	lower := strings.ToLower(s)
	for _, phrase := range bind2608RawPhrases {
		if strings.Contains(lower, strings.ToLower(phrase)) {
			return phrase, true
		}
	}
	return "", false
}

func TestBindErrorMessages2608(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// requireMerchantAdmin (webhook_config.go) reads the business role from the
	// request context: ADMIN + tid==oid resolves to merchant_admin.
	const tenantID = "bind2608-tenant"
	const userID = "bind2608-user"

	promoInstrumentID := uuid.New().String()
	membershipLevelID := "7"
	pointsPolicyID := uuid.New().String()
	grantUserID := uuid.New().String()

	userManagement := &UserManagementHandler{}

	endpoints := []bind2608Endpoint{
		// discount.go
		{key: "DISCOUNT", name: "discount.CreateDiscountPolicy", method: http.MethodPost, route: "/merchant/discount-policies", path: "/merchant/discount-policies", handler: CreateDiscountPolicy, body: `{}`, wantMsg: "参数不合法：名称不能为空"},
		{key: "DISCOUNTCODE", name: "discount.CreateDiscountCode", method: http.MethodPost, route: "/merchant/discount-codes", path: "/merchant/discount-codes", handler: CreateDiscountCode, body: `{}`, wantMsg: "参数不合法：优惠码不能为空"},

		// gift_policies.go
		{key: "GIFTPOLICY", name: "giftpolicies.UpdateGiftPolicy", method: http.MethodPut, route: "/merchant/gift-policies", path: "/merchant/gift-policies", handler: UpdateGiftPolicy, body: `{"pay_ratio":"abc"}`, wantMsg: "参数不合法：乐币抵扣比例类型不正确"},
		{key: "POINTSETTINGS", name: "giftpolicies.UpdatePointSettings", method: http.MethodPut, route: "/admin/point-settings", path: "/admin/point-settings", handler: UpdatePointSettings, body: `{"pay_ratio_max":"abc"}`, wantMsg: "参数不合法：最高抵扣比例类型不正确"},

		// instrument_promo_override.go
		{key: "PROMO", name: "promo.UpdateInstrumentPromoOverride", method: http.MethodPut, route: "/merchant/instruments/:id/promo-overrides", path: "/merchant/instruments/" + promoInstrumentID + "/promo-overrides", handler: UpdateInstrumentPromoOverride, body: `{}`, wantMsg: "参数不合法：推广类型不能为空"},

		// membership_benefits.go — scoped label (权益项, not 乐器列表)
		{key: "BENEFITS", name: "membership.UpdateLevelBenefits", method: http.MethodPut, route: "/merchant/membership-levels/:id/benefits", path: "/merchant/membership-levels/" + membershipLevelID + "/benefits", handler: UpdateLevelBenefits, body: `{"items":"abc"}`, wantMsg: "参数不合法：权益项类型不正确"},

		// membership_level.go
		{key: "MEMBERLEVEL_CREATE", name: "membership.CreateMembershipLevel", method: http.MethodPost, route: "/merchant/membership-levels", path: "/merchant/membership-levels", handler: CreateMembershipLevel, body: `{}`, wantMsg: "参数不合法：编号不能为空"},
		{key: "MEMBERLEVEL_UPDATE", name: "membership.UpdateMembershipLevel", method: http.MethodPut, route: "/merchant/membership-levels/:id", path: "/merchant/membership-levels/" + membershipLevelID, handler: UpdateMembershipLevel, body: `{"min_amount":"abc"}`, wantMsg: "参数不合法：最低消费金额类型不正确"},

		// merchant_rebate.go
		{key: "REBATE", name: "merchant.UpdateMerchantRebateOptIn", method: http.MethodPut, route: "/merchant/rebate-opt-in", path: "/merchant/rebate-opt-in", handler: UpdateMerchantRebateOptIn, body: `{"opt_in":"abc"}`, wantMsg: "参数不合法：返点开关类型不正确"},

		// points_policy.go — scopeTypeFromPath needs the /merchant/ prefix
		{key: "POINTS", name: "points.CreatePointsPolicy", method: http.MethodPost, route: "/merchant/points-policies", path: "/merchant/points-policies", handler: CreatePointsPolicy, body: `{"valid_days":"abc"}`, wantMsg: "参数不合法：有效天数类型不正确"},
		{key: "POINTSUPDATE", name: "points.UpdatePointsPolicy", method: http.MethodPut, route: "/merchant/points-policies/:id", path: "/merchant/points-policies/" + pointsPolicyID, handler: UpdatePointsPolicy, body: `{"max_pay_ratio":"abc"}`, wantMsg: "参数不合法：最高抵扣比例类型不正确"},

		// user_management.go
		{key: "GRANTPOINTS", name: "usermgmt.GrantPoints", method: http.MethodPost, route: "/merchant/users/:id/points", path: "/merchant/users/" + grantUserID + "/points", handler: userManagement.GrantPoints, body: `{"amount":"abc"}`, wantMsg: "参数不合法：金额类型不正确"},

		// warning.go / webhook_config.go
		{key: "WARNING", name: "warning.UpdateWarningSettings", method: http.MethodPut, route: "/admin/warning-settings", path: "/admin/warning-settings", handler: UpdateWarningSettings, body: `{"cooldown_minutes":"abc"}`, wantMsg: "参数不合法：冷却间隔类型不正确"},
		{key: "WEBHOOK", name: "webhook.UpdateMerchantWebhook", method: http.MethodPut, route: "/merchant/webhook", path: "/merchant/webhook", handler: UpdateMerchantWebhook, body: `{"enabled":"abc"}`, wantMsg: "参数不合法：启用状态类型不正确"},
	}

	require.Len(t, endpoints, 14, "14 binding points are收口 by #2608")

	router := gin.New()
	withIdentity := func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, "ADMIN")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
	router.Use(withIdentity)
	for _, ep := range endpoints {
		router.Handle(ep.method, ep.route, ep.handler)
	}

	invoke := func(t *testing.T, ep bind2608Endpoint, body string) (int, int, string) {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(ep.method, ep.path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		var resp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
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
			{"syntax", bind2608SyntaxBody, bindmsg.SyntaxMessage},
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
			// #2608 D2: the business code is untouched — all 14 points are 40002.
			if code != 40002 {
				t.Errorf("%s/%s: code %d, want 40002", ep.name, variant.kind, code)
				ok = false
			}
			if bind2608HasASCIILetter(message) {
				t.Errorf("%s/%s: message contains ASCII letters: %q", ep.name, variant.kind, message)
				asciiLeak++
				ok = false
			}
			if phrase, leaked := bind2608LeaksRawPhrase(message); leaked {
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

	// Negative on scope pollution: the inventory wording must not appear on the
	// membership benefits endpoint (that is exactly what MessageScoped fixes).
	if got := captured["BENEFITS"]; strings.Contains(got, "乐器列表") {
		t.Errorf("membership.UpdateLevelBenefits leaked the inventory label: %q", got)
	}

	t.Logf("BIND2608_CASES_TOTAL=%d", total)
	t.Logf("BIND2608_CASES_FAIL=%d", caseFail)
	t.Logf("BIND2608_ASCII_LEAK=%d", asciiLeak)
	t.Logf("BIND2608_RAW_PHRASE_LEAK=%d", rawLeak)
	t.Logf("BIND2608_DISCOUNT_MSG=%s", captured["DISCOUNT"])
	t.Logf("BIND2608_DISCOUNTCODE_MSG=%s", captured["DISCOUNTCODE"])
	t.Logf("BIND2608_POINTS_MSG=%s", captured["POINTS"])
	t.Logf("BIND2608_BENEFITS_MSG=%s", captured["BENEFITS"])
	t.Logf("BIND2608_REBATE_MSG=%s", captured["REBATE"])
	t.Logf("BIND2608_SYNTAX_MSG=%s", syntaxMsg)

	if total != 28 {
		t.Errorf("BIND2608_CASES_TOTAL=%d, want 28", total)
	}
}
