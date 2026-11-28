package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/bindmsg"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// #2639: the 400xx branches touched by this Issue must answer with a Chinese
// business reason. The clients (`resolveErrorCore` L2) forward a message only
// when it contains CJK; an English constant (or a message naming a JSON field)
// is treated as "no specific reason" and replaced by the generic
// `ERROR_CODE_MAP[40002]` fallback, so the user never sees the real cause
// ("优惠码已过期" / "推荐奖励比例超出范围" / "推广类型不正确").
//
// The test drives the real handlers (gin router + PostgreSQL fixture) for the
// 33 sites listed in the Accepted Plan plus 2 binding-shape variants (35 cases).
// Assertions are hard: HTTP 400, unchanged business code, exact Chinese message,
// no ASCII letters, no `_` (JSON field name), no raw decoder phrase.

const msg2639SyntaxBody = "{\"probe\":\"\x17\"}"

type msg2639Case struct {
	key     string // token key for the summary lines; empty = not printed
	name    string
	method  string
	route   string
	path    string
	handler gin.HandlerFunc
	code    int
	body    string
	wantMsg string
}

func msg2639HasASCIILetter(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

func msg2639HasUnderscore(s string) bool {
	return strings.Contains(s, "_")
}

var msg2639RawPhrases = []string{
	"invalid character",
	"cannot unmarshal",
	"Field validation",
	"json:",
	"unexpected end of JSON",
	"EOF",
}

func msg2639LeaksRawPhrase(s string) (string, bool) {
	lower := strings.ToLower(s)
	for _, phrase := range msg2639RawPhrases {
		if strings.Contains(lower, strings.ToLower(phrase)) {
			return phrase, true
		}
	}
	return "", false
}

func TestErrorMessages2639(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testfixtures.SetupTestDB(t)
	if db == nil {
		return // DB unavailable: GetTestDB already failed/skipped
	}

	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	userID := uuid.New().String()

	// pay_ratio_max is a configurable limit (#1945/#1939); pin it so the
	// dynamic expression in the message is deterministic.
	require.NoError(t, upsertGlobalSetting(context.Background(), services.SettingPayRatioMax, "0.8"))

	// ---- discount fixtures (#1539): one policy per rejection reason --------
	past := time.Now().Add(-48 * time.Hour)
	future := time.Now().Add(48 * time.Hour)

	activePolicy := models.DiscountPolicy{
		ID: uuid.New().String(), Name: "msg2639-active",
		RentDiscount: 0.9, DepositDiscount: 0.9, ShippingDiscount: 0.9,
		IsActive: true,
	}
	require.NoError(t, db.Create(&activePolicy).Error)

	notYetPolicy := models.DiscountPolicy{
		ID: uuid.New().String(), Name: "msg2639-future",
		RentDiscount: 0.9, DepositDiscount: 0.9, ShippingDiscount: 0.9,
		ValidFrom: &future, IsActive: true,
	}
	require.NoError(t, db.Create(&notYetPolicy).Error)

	expiredPolicy := models.DiscountPolicy{
		ID: uuid.New().String(), Name: "msg2639-expired",
		RentDiscount: 0.9, DepositDiscount: 0.9, ShippingDiscount: 0.9,
		ValidTo: &past, IsActive: true,
	}
	require.NoError(t, db.Create(&expiredPolicy).Error)

	// `discount_codes.policy_id` has a FK, so "policy not found" is exercised via
	// a policy that exists but is inactive (`Where(... AND is_active = true)`).
	// GORM skips zero-value fields that carry a `default:` tag, so `IsActive:false`
	// must be written with an explicit UPDATE.
	inactivePolicy := models.DiscountPolicy{
		ID: uuid.New().String(), Name: "msg2639-inactive",
		RentDiscount: 0.9, DepositDiscount: 0.9, ShippingDiscount: 0.9,
		IsActive: true,
	}
	require.NoError(t, db.Create(&inactivePolicy).Error)
	require.NoError(t, db.Model(&models.DiscountPolicy{}).
		Where("id = ?", inactivePolicy.ID).Update("is_active", false).Error)

	discountCodes := []models.DiscountCode{
		{Code: "MSG2639_INACTIVE", PolicyID: activePolicy.ID, IsActive: true},
		{Code: "MSG2639_USAGE", PolicyID: activePolicy.ID, MaxUses: 1, UsageCount: 1, IsActive: true},
		{Code: "MSG2639_EXPIRED", PolicyID: activePolicy.ID, ExpiresAt: &past, IsActive: true},
		{Code: "MSG2639_NOPOLICY", PolicyID: inactivePolicy.ID, IsActive: true},
		{Code: "MSG2639_FUTURE", PolicyID: notYetPolicy.ID, IsActive: true},
		{Code: "MSG2639_POLICY_EXPIRED", PolicyID: expiredPolicy.ID, IsActive: true},
	}
	for i := range discountCodes {
		discountCodes[i].ID = uuid.New().String()
		require.NoError(t, db.Create(&discountCodes[i]).Error)
	}
	// Same GORM zero-value caveat for the inactive code.
	require.NoError(t, db.Model(&models.DiscountCode{}).
		Where("code = ?", "MSG2639_INACTIVE").Update("is_active", false).Error)

	// ---- user_management.Update looks the row up before validating status ---
	managedUser := models.User{
		ID:       uuid.New().String(),
		TenantID: tenantID,
		OrgID:    orgID,
		Username: "msg2639-user",
		Name:     "msg2639 user",
		Status:   "active",
	}
	require.NoError(t, db.Create(&managedUser).Error)

	userManagementHandler := NewUserManagementHandler()
	pointsPolicyPath := "/t/points-policies"

	cases := []msg2639Case{
		// discount.go — ApplyDiscountCode (7 sites)
		{key: "BIND_CODE", name: "discount.ApplyDiscountCode/bind", method: http.MethodPost, route: "/t/discount/apply", path: "/t/discount/apply", handler: ApplyDiscountCode, code: 40002, body: `{}`, wantMsg: "参数不合法：优惠码不能为空"},
		{name: "discount.ApplyDiscountCode/inactive", method: http.MethodPost, route: "/t/discount/apply", path: "/t/discount/apply", handler: ApplyDiscountCode, code: 40002, body: `{"code":"MSG2639_INACTIVE"}`, wantMsg: "优惠码已停用"},
		{name: "discount.ApplyDiscountCode/usage-limit", method: http.MethodPost, route: "/t/discount/apply", path: "/t/discount/apply", handler: ApplyDiscountCode, code: 40002, body: `{"code":"MSG2639_USAGE"}`, wantMsg: "优惠码使用次数已达上限"},
		{key: "DISCOUNT_EXPIRED", name: "discount.ApplyDiscountCode/expired", method: http.MethodPost, route: "/t/discount/apply", path: "/t/discount/apply", handler: ApplyDiscountCode, code: 40002, body: `{"code":"MSG2639_EXPIRED"}`, wantMsg: "优惠码已过期"},
		{name: "discount.ApplyDiscountCode/policy-missing", method: http.MethodPost, route: "/t/discount/apply", path: "/t/discount/apply", handler: ApplyDiscountCode, code: 40002, body: `{"code":"MSG2639_NOPOLICY"}`, wantMsg: "折扣策略已停用或不存在"},
		{name: "discount.ApplyDiscountCode/policy-not-yet-valid", method: http.MethodPost, route: "/t/discount/apply", path: "/t/discount/apply", handler: ApplyDiscountCode, code: 40002, body: `{"code":"MSG2639_FUTURE"}`, wantMsg: "折扣策略尚未生效"},
		{name: "discount.ApplyDiscountCode/policy-expired", method: http.MethodPost, route: "/t/discount/apply", path: "/t/discount/apply", handler: ApplyDiscountCode, code: 40002, body: `{"code":"MSG2639_POLICY_EXPIRED"}`, wantMsg: "折扣策略已过期"},

		// gift_policies.go — UpdateGiftPolicy / UpdatePointSettings (7 sites)
		{name: "gift_policies.UpdateGiftPolicy/level-negative", method: http.MethodPut, route: "/t/gift-policy", path: "/t/gift-policy", handler: UpdateGiftPolicy, code: 40002, body: `{"level_id":-1}`, wantMsg: "会员等级不能为负数"},
		{name: "gift_policies.UpdateGiftPolicy/pay-ratio-range", method: http.MethodPut, route: "/t/gift-policy", path: "/t/gift-policy", handler: UpdateGiftPolicy, code: 40002, body: `{"pay_ratio":0.9}`, wantMsg: "乐币抵扣比例必须在 0~0.8 之间（赠点最多抵扣应付金额的 80%）"},
		{key: "REFERRAL_RANGE", name: "gift_policies.UpdateGiftPolicy/referral-ratio-range", method: http.MethodPut, route: "/t/gift-policy", path: "/t/gift-policy", handler: UpdateGiftPolicy, code: 40002, body: `{"referral_ratio":1.5}`, wantMsg: "推荐奖励比例必须在 0~1 之间"},
		{name: "gift_policies.UpdateGiftPolicy/referral-points-range", method: http.MethodPut, route: "/t/gift-policy", path: "/t/gift-policy", handler: UpdateGiftPolicy, code: 40002, body: `{"referral_reg_points":200000}`, wantMsg: "推荐注册赠点必须在 0~100000 之间"},
		{name: "gift_policies.UpdatePointSettings/pay-ratio-max-range", method: http.MethodPut, route: "/t/gift-policy/settings", path: "/t/gift-policy/settings", handler: UpdatePointSettings, code: 40002, body: `{"pay_ratio_max":2}`, wantMsg: "最高抵扣比例必须在 0~1 之间"},
		{name: "gift_policies.UpdatePointSettings/validity-months-range", method: http.MethodPut, route: "/t/gift-policy/settings", path: "/t/gift-policy/settings", handler: UpdatePointSettings, code: 40002, body: `{"point_batch_validity_months":0}`, wantMsg: "乐币有效期月数必须在 1~120 之间"},
		{name: "gift_policies.UpdatePointSettings/reminder-days-range", method: http.MethodPut, route: "/t/gift-policy/settings", path: "/t/gift-policy/settings", handler: UpdatePointSettings, code: 40002, body: `{"point_expiry_reminder_days":400}`, wantMsg: "到期提醒提前天数必须在 0~365 之间"},

		// instrument_promo_override.go (3 sites)
		{name: "instrument_promo_override.GetInstrumentPromoOverrides/invalid-id", method: http.MethodGet, route: "/t/instruments/:id/promo-overrides", path: "/t/instruments/not-a-uuid/promo-overrides", handler: GetInstrumentPromoOverrides, code: 40002, wantMsg: "乐器编号不正确"},
		{name: "instrument_promo_override.UpdateInstrumentPromoOverride/invalid-id", method: http.MethodPut, route: "/t/instruments/:id/promo-overrides", path: "/t/instruments/not-a-uuid/promo-overrides", handler: UpdateInstrumentPromoOverride, code: 40002, body: `{"override_type":"discount"}`, wantMsg: "乐器编号不正确"},
		{key: "OVERRIDE_TYPE", name: "instrument_promo_override.UpdateInstrumentPromoOverride/invalid-type", method: http.MethodPut, route: "/t/instruments/:id/promo-overrides", path: "/t/instruments/" + uuid.New().String() + "/promo-overrides", handler: UpdateInstrumentPromoOverride, code: 40002, body: `{"override_type":"bogus"}`, wantMsg: "推广类型必须是折扣、返利或租转售"},

		// membership_benefits.go (4 sites)
		{name: "membership_benefits.ListLevelBenefits/invalid-id", method: http.MethodGet, route: "/t/membership-levels/:id/benefits", path: "/t/membership-levels/abc/benefits", handler: ListLevelBenefits, code: 40002, wantMsg: "会员等级编号不正确"},
		{name: "membership_benefits.UpdateLevelBenefits/invalid-id", method: http.MethodPut, route: "/t/membership-levels/:id/benefits", path: "/t/membership-levels/abc/benefits", handler: UpdateLevelBenefits, code: 40002, body: `{"items":[]}`, wantMsg: "会员等级编号不正确"},
		{name: "membership_benefits.GetMembershipBenefits/missing-level", method: http.MethodGet, route: "/t/membership-benefits", path: "/t/membership-benefits", handler: GetMembershipBenefits, code: 40002, wantMsg: "请选择会员等级"},
		{name: "membership_benefits.GetMembershipBenefits/invalid-level", method: http.MethodGet, route: "/t/membership-benefits", path: "/t/membership-benefits?level_id=abc", handler: GetMembershipBenefits, code: 40002, wantMsg: "会员等级编号不正确"},

		// membership_level.go (2 sites)
		{name: "membership_level.UpdateMembershipLevel/invalid-id", method: http.MethodPut, route: "/t/membership-levels/:id", path: "/t/membership-levels/abc", handler: UpdateMembershipLevel, code: 40002, body: `{"name":"x"}`, wantMsg: "编号不正确"},
		{name: "membership_level.DeleteMembershipLevel/invalid-id", method: http.MethodDelete, route: "/t/membership-levels/:id", path: "/t/membership-levels/abc", handler: DeleteMembershipLevel, code: 40002, wantMsg: "编号不正确"},

		// points_policy.go (3 sites)
		{name: "points_policy.ListPointsPolicies/unknown-scope", method: http.MethodGet, route: pointsPolicyPath, path: pointsPolicyPath, handler: ListPointsPolicies, code: 40002, wantMsg: "请求地址不正确"},
		{name: "points_policy.CreatePointsPolicy/unknown-scope", method: http.MethodPost, route: pointsPolicyPath, path: pointsPolicyPath, handler: CreatePointsPolicy, code: 40002, body: `{}`, wantMsg: "请求地址不正确"},
		{name: "points_policy.UpdatePointsPolicy/invalid-id", method: http.MethodPut, route: pointsPolicyPath + "/:id", path: pointsPolicyPath + "/not-a-uuid", handler: UpdatePointsPolicy, code: 40002, body: `{}`, wantMsg: "编号不正确"},

		// user_management.go (4 sites)
		{name: "user_management.Update/invalid-status", method: http.MethodPut, route: "/t/admin/user-management/:id", path: "/t/admin/user-management/" + managedUser.ID, handler: userManagementHandler.Update, code: 40002, body: `{"status":"bogus"}`, wantMsg: "用户状态必须是启用或停用"},
		{name: "user_management.Update/no-editable-fields", method: http.MethodPut, route: "/t/admin/user-management/:id", path: "/t/admin/user-management/" + managedUser.ID, handler: userManagementHandler.Update, code: 40002, body: `{}`, wantMsg: "未提供可修改的内容"},
		{name: "user_management.GrantPoints/reason-required", method: http.MethodPost, route: "/t/admin/user-management/:id/points", path: "/t/admin/user-management/" + managedUser.ID + "/points", handler: userManagementHandler.GrantPoints, code: 40002, body: `{"amount":10}`, wantMsg: "请填写赠送原因"},
		{name: "user_management.GrantPoints/amount-positive", method: http.MethodPost, route: "/t/admin/user-management/:id/points", path: "/t/admin/user-management/" + managedUser.ID + "/points", handler: userManagementHandler.GrantPoints, code: 40002, body: `{"amount":0,"reason":"x"}`, wantMsg: "赠送金额必须大于 0"},

		// warning.go (3 sites)
		{key: "WARNING_REASON", name: "warning.CreateWarning/reason-required", method: http.MethodPost, route: "/t/warnings", path: "/t/warnings", handler: CreateWarning, code: 40001, body: `{}`, wantMsg: "请求参数有误，请填写预警原因"},
		{key: "BIND_STATUS", name: "warning.UpdateWarningStatus/bind-type", method: http.MethodPut, route: "/t/warnings/:id/status", path: "/t/warnings/" + uuid.New().String() + "/status", handler: UpdateWarningStatus, code: 40002, body: `{"status":123}`, wantMsg: "参数不合法：状态类型不正确"},
		{name: "warning.UpdateWarningStatus/invalid-status", method: http.MethodPut, route: "/t/warnings/:id/status", path: "/t/warnings/" + uuid.New().String() + "/status", handler: UpdateWarningStatus, code: 40003, body: `{"status":"bogus"}`, wantMsg: "预警状态取值不正确"},
	}

	require.Len(t, cases, 33, "33 sites are收口 by #2639")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	registered := map[string]bool{}
	for _, tc := range cases {
		key := tc.method + " " + tc.route
		if registered[key] {
			continue
		}
		registered[key] = true
		router.Handle(tc.method, tc.route, tc.handler)
	}

	invoke := func(t *testing.T, tc msg2639Case, body string) (int, int, string) {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(body))
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
	fieldNameLeak := 0
	codeDrift := 0
	captured := map[string]string{}

	check := func(t *testing.T, name string, status, code int, wantCode int, message, wantMsg string) {
		t.Helper()
		ok := true
		if status != http.StatusBadRequest {
			t.Errorf("%s: HTTP %d, want 400 (message=%q)", name, status, message)
			ok = false
		}
		if code != wantCode {
			t.Errorf("%s: code %d, want %d", name, code, wantCode)
			codeDrift++
			ok = false
		}
		if msg2639HasASCIILetter(message) {
			t.Errorf("%s: message contains ASCII letters: %q", name, message)
			asciiLeak++
			ok = false
		}
		if msg2639HasUnderscore(message) {
			t.Errorf("%s: message leaks a JSON field name: %q", name, message)
			fieldNameLeak++
			ok = false
		}
		if phrase, leaked := msg2639LeaksRawPhrase(message); leaked {
			t.Errorf("%s: message leaks raw phrase %q: %q", name, phrase, message)
			asciiLeak++
			ok = false
		}
		if message != wantMsg {
			t.Errorf("%s: message = %q, want %q", name, message, wantMsg)
			ok = false
		}
		if !ok {
			caseFail++
		}
	}

	for _, tc := range cases {
		total++
		status, code, message := invoke(t, tc, tc.body)
		check(t, tc.name, status, code, tc.code, message, tc.wantMsg)
		if tc.key != "" {
			captured[tc.key] = message
		}
	}

	// Two binding-shape variants: a malformed body must fall back to the
	// syntax message (never the Go decoder wording) on the same 400xx branch.
	variants := []struct {
		name    string
		method  string
		route   string
		path    string
		handler gin.HandlerFunc
		code    int
	}{
		{"discount.ApplyDiscountCode/bind-syntax", http.MethodPost, "/t/discount/apply", "/t/discount/apply", ApplyDiscountCode, 40002},
		{"warning.UpdateWarningStatus/bind-syntax", http.MethodPut, "/t/warnings/:id/status", "/t/warnings/" + uuid.New().String() + "/status", UpdateWarningStatus, 40002},
	}
	for _, variant := range variants {
		total++
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(variant.method, variant.path, bytes.NewBufferString(msg2639SyntaxBody))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		var resp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		check(t, variant.name, rec.Code, resp.Code, variant.code, resp.Message, bindmsg.SyntaxMessage)
	}

	t.Logf("MSG2639_CASES_TOTAL=%d", total)
	t.Logf("MSG2639_CASES_FAIL=%d", caseFail)
	t.Logf("MSG2639_ASCII_LEAK=%d", asciiLeak)
	t.Logf("MSG2639_FIELDNAME_LEAK=%d", fieldNameLeak)
	t.Logf("MSG2639_HTTP_CODE_DRIFT=%d", codeDrift)
	t.Logf("MSG2639_DISCOUNT_EXPIRED=%s", captured["DISCOUNT_EXPIRED"])
	t.Logf("MSG2639_REFERRAL_RANGE=%s", captured["REFERRAL_RANGE"])
	t.Logf("MSG2639_OVERRIDE_TYPE=%s", captured["OVERRIDE_TYPE"])
	t.Logf("MSG2639_BIND_CODE=%s", captured["BIND_CODE"])
	t.Logf("MSG2639_BIND_STATUS=%s", captured["BIND_STATUS"])
	t.Logf("MSG2639_WARNING_REASON=%s", captured["WARNING_REASON"])

	if total != 35 {
		t.Errorf("MSG2639_CASES_TOTAL=%d, want 35", total)
	}
}
