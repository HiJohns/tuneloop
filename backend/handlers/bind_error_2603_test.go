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
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

// #2603: every handler that binds a JSON request body must answer a binding
// failure with a Chinese, field-naming message. Raw Go/JSON wording
// (`invalid character '\x17' in string`, `invalid parameters: …`) must never
// reach the response body — the PC/H5 clients treat non-CJK messages as "no
// specific reason" and replace them with a generic fallback.
//
// The test drives the real handlers (gin router + PostgreSQL fixture) for the
// 26 binding points收口 by #2603. Each endpoint is exercised twice:
//   - malformed body (raw control character) → 请求体格式不正确，请检查后重试
//   - missing required field / wrong type      → 参数不合法：<称呼>…
//
// Assertions are hard: HTTP 400, unchanged business code, no ASCII letters in
// the message, and no raw decoder phrase.

const bind2603SyntaxBody = "{\"probe\":\"\x17\"}"

type bind2603Endpoint struct {
	key      string // token key for the summary lines (SITE/PROPERTY/…)
	name     string
	method   string
	route    string
	path     string
	handler  gin.HandlerFunc
	code     int    // business code, unchanged by the fix
	body     string // body exercising a required field / wrong type
	wantMsg  string // exact message for the field case
	captureM bool
}

func bind2603HasASCIILetter(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

var bind2603RawPhrases = []string{
	"invalid character",
	"invalid parameters",
	"cannot unmarshal",
	"Field validation",
	"json:",
	"unexpected end of JSON",
	"EOF",
}

func bind2603LeaksRawPhrase(s string) (string, bool) {
	lower := strings.ToLower(s)
	for _, phrase := range bind2603RawPhrases {
		if strings.Contains(lower, strings.ToLower(phrase)) {
			return phrase, true
		}
	}
	return "", false
}

func TestBindErrorMessages2603(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testfixtures.SetupTestDB(t)
	if db == nil {
		return // DB unavailable: GetTestDB already failed/skipped
	}

	tenantID := uuid.New().String()
	userID := uuid.New().String()

	// The two update endpoints look the row up before binding, so seed them.
	banner := models.Banner{
		TenantID: tenantID,
		ImageURL: "https://example.test/bind2603-banner.png",
		Status:   "active",
	}
	require.NoError(t, db.Create(&banner).Error)

	mallImage := models.MallImage{
		ImageURL: "https://example.test/bind2603-mall.png",
		Status:   "active",
	}
	require.NoError(t, db.Create(&mallImage).Error)

	propertyHandler := NewPropertyHandler()
	warehouseHandler := NewWarehouseHandler()
	mallImageHandler := NewMallImageHandler()
	inventoryHandler := NewInventoryHandler()
	iamProxyHandler := NewIAMProxyHandler()
	bannerHandler := NewBannerHandler()
	appealHandler := NewAppealHandler()

	endpoints := []bind2603Endpoint{
		// property.go
		{key: "PROPERTY", name: "property.CreateProperty", method: http.MethodPost, route: "/t/property", path: "/t/property", handler: propertyHandler.CreateProperty, code: 40002, body: `{"name":"x"}`, wantMsg: "参数不合法：属性类型不能为空", captureM: true},
		{name: "property.UpdateProperty", method: http.MethodPut, route: "/t/property/:id", path: "/t/property/" + uuid.New().String(), handler: propertyHandler.UpdateProperty, code: 40002, body: `{"property_type":"x"}`, wantMsg: "参数不合法：名称不能为空"},
		{name: "property.CreatePropertyOption", method: http.MethodPost, route: "/t/property/option", path: "/t/property/option", handler: propertyHandler.CreatePropertyOption, code: 40002, body: `{}`, wantMsg: "参数不合法：属性不能为空"},
		{name: "property.ConfirmPropertyValue", method: http.MethodPut, route: "/t/property/confirm", path: "/t/property/confirm", handler: propertyHandler.ConfirmPropertyValue, code: 40002, body: `{"property_id":"` + uuid.New().String() + `"}`, wantMsg: "参数不合法：属性值不能为空"},
		{name: "property.MergePropertyValues", method: http.MethodPut, route: "/t/property/merge", path: "/t/property/merge", handler: propertyHandler.MergePropertyValues, code: 40002, body: `{"property_id":"` + uuid.New().String() + `"}`, wantMsg: "参数不合法：原值不能为空"},

		// warehouse.go
		{key: "WAREHOUSE", name: "warehouse.UpdateShipping", method: http.MethodPut, route: "/t/warehouse/orders/:id/shipping", path: "/t/warehouse/orders/" + uuid.New().String() + "/shipping", handler: warehouseHandler.UpdateShipping, code: 40002, body: `{"company":"x","shipped_at":"2026-01-01T00:00:00Z"}`, wantMsg: "参数不合法：物流单号不能为空", captureM: true},
		{name: "warehouse.ConfirmDelivery", method: http.MethodPut, route: "/t/warehouse/orders/:id/delivery", path: "/t/warehouse/orders/" + uuid.New().String() + "/delivery", handler: warehouseHandler.ConfirmDelivery, code: 40002, body: `{"photos":["p"]}`, wantMsg: "参数不合法：收货时间不能为空"},
		{name: "warehouse.InspectReturn", method: http.MethodPut, route: "/t/warehouse/orders/:id/return-inspect", path: "/t/warehouse/orders/" + uuid.New().String() + "/return-inspect", handler: warehouseHandler.InspectReturn, code: 40002, body: `{"scan_time":"2026-01-01T00:00:00Z","photos":["p"]}`, wantMsg: "参数不合法：乐器序列号不能为空"},
		{name: "warehouse.AssessDamage", method: http.MethodPut, route: "/t/warehouse/orders/:id/damage", path: "/t/warehouse/orders/" + uuid.New().String() + "/damage", handler: warehouseHandler.AssessDamage, code: 40002, body: `{"damage_amount":1}`, wantMsg: "参数不合法：损坏说明不能为空"},

		// wechatpay_prepay.go
		{name: "prepay.PrepayOrder", method: http.MethodPost, route: "/t/pay/prepay", path: "/t/pay/prepay", handler: PrepayOrder, code: 40002, body: `{}`, wantMsg: "参数不合法：订单类型不能为空"},
		{name: "prepay.QueryPayment", method: http.MethodPost, route: "/t/pay/query", path: "/t/pay/query", handler: QueryPayment, code: 40002, body: `{}`, wantMsg: "参数不合法：商户订单号不能为空"},

		// renewal.go
		{name: "renewal.CalculateRenewal", method: http.MethodPost, route: "/t/orders/:id/renewal/calculate", path: "/t/orders/" + uuid.New().String() + "/renewal/calculate", handler: CalculateRenewal, code: 40002, body: `{}`, wantMsg: "参数不合法：续租天数不能为空"},
		{name: "renewal.ConfirmRenewal", method: http.MethodPost, route: "/t/orders/:id/renewal/confirm", path: "/t/orders/" + uuid.New().String() + "/renewal/confirm", handler: ConfirmRenewal, code: 40002, body: `{}`, wantMsg: "参数不合法：续租天数不能为空"},

		// mall_image.go
		{name: "mallimage.CreateMallImage", method: http.MethodPost, route: "/t/admin/mall-images", path: "/t/admin/mall-images", handler: mallImageHandler.CreateMallImage, code: 40002, body: `{}`, wantMsg: "参数不合法：图片地址不能为空"},
		{name: "mallimage.UpdateMallImage", method: http.MethodPut, route: "/t/admin/mall-images/:id", path: "/t/admin/mall-images/" + mallImage.ID, handler: mallImageHandler.UpdateMallImage, code: 40002, body: `{"sort_order":"abc"}`, wantMsg: "参数不合法：排序类型不正确"},

		// inventory.go
		{name: "inventory.TransferInventory", method: http.MethodPost, route: "/t/merchant/inventory/transfer", path: "/t/merchant/inventory/transfer", handler: inventoryHandler.TransferInventory, code: 40002, body: `{}`, wantMsg: "参数不合法：资产不能为空"},
		{name: "inventory.BatchUpdateRent", method: http.MethodPut, route: "/t/inventory/rent-setting/batch", path: "/t/inventory/rent-setting/batch", handler: inventoryHandler.BatchUpdateRent, code: 40002, body: `{}`, wantMsg: "参数不合法：乐器列表不能为空"},

		// iam_proxy.go
		{name: "iam.CreateUser", method: http.MethodPost, route: "/t/iam/users", path: "/t/iam/users", handler: iamProxyHandler.CreateUser, code: 40002, body: `{}`, wantMsg: "参数不合法：名称不能为空"},
		{name: "iam.UpdateIAMUser", method: http.MethodPut, route: "/t/iam/users/:id", path: "/t/iam/users/" + uuid.New().String(), handler: iamProxyHandler.UpdateIAMUser, code: 40002, body: `{"name":123}`, wantMsg: "参数不合法：名称类型不正确"},

		// banner.go
		{name: "banner.CreateBanner", method: http.MethodPost, route: "/t/admin/banners", path: "/t/admin/banners", handler: bannerHandler.CreateBanner, code: 40002, body: `{}`, wantMsg: "参数不合法：图片地址不能为空"},
		{name: "banner.UpdateBanner", method: http.MethodPut, route: "/t/admin/banners/:id", path: "/t/admin/banners/" + banner.ID, handler: bannerHandler.UpdateBanner, code: 40002, body: `{"sort_order":"abc"}`, wantMsg: "参数不合法：排序类型不正确"},

		// appeal.go
		{name: "appeal.ResolveAppeal", method: http.MethodPut, route: "/t/merchant/appeals/:id/resolve", path: "/t/merchant/appeals/" + uuid.New().String() + "/resolve", handler: appealHandler.ResolveAppeal, code: 40002, body: `{"comment":"x"}`, wantMsg: "参数不合法：处理决定不能为空"},
		{name: "appeal.SubmitAppeal", method: http.MethodPost, route: "/t/user/appeals", path: "/t/user/appeals", handler: appealHandler.SubmitAppeal, code: 40002, body: `{}`, wantMsg: "参数不合法：定损单不能为空"},

		// payment_calculate.go
		{name: "payment.CalculatePayment", method: http.MethodPost, route: "/t/pay/calculate", path: "/t/pay/calculate", handler: CalculatePayment, code: 40002, body: `{}`, wantMsg: "参数不合法：类型不能为空"},

		// instrument.go (this branch answers 40001, not 40002 — must not change)
		{name: "instrument.UpdateInstrumentStatus", method: http.MethodPut, route: "/t/instruments/:id/status", path: "/t/instruments/" + uuid.New().String() + "/status", handler: UpdateInstrumentStatus, code: 40001, body: `{}`, wantMsg: "参数不合法：库存状态不能为空"},

		// coupon_pricing.go
		{key: "COUPON", name: "coupon.ValidateCoupon", method: http.MethodPost, route: "/t/coupons/validate", path: "/t/coupons/validate", handler: ValidateCoupon, code: 40002, body: `{}`, wantMsg: "参数不合法：优惠码不能为空", captureM: true},
	}

	require.Len(t, endpoints, 26, "26 binding points are收口 by #2603")

	router := gin.New()
	withIdentity := func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
	router.Use(withIdentity)
	for _, ep := range endpoints {
		router.Handle(ep.method, ep.route, ep.handler)
	}

	invoke := func(t *testing.T, ep bind2603Endpoint, body string) (int, int, string) {
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
			{"syntax", bind2603SyntaxBody, bindmsg.SyntaxMessage},
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
				t.Errorf("%s/%s: code %d, want %d", ep.name, variant.kind, code, ep.code)
				ok = false
			}
			if bind2603HasASCIILetter(message) {
				t.Errorf("%s/%s: message contains ASCII letters: %q", ep.name, variant.kind, message)
				asciiLeak++
				ok = false
			}
			if phrase, leaked := bind2603LeaksRawPhrase(message); leaked {
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

	// Seed sanity: the two site.go points were already converged before this
	// batch, so they are not part of the 52 contract cases — but the Issue's
	// original symptom is here, and R2 pins its exact wording.
	siteHandler := NewSiteHandler()
	siteRouter := gin.New()
	siteRouter.Use(withIdentity)
	siteRouter.POST("/t/merchant/sites", siteHandler.CreateSite)
	siteRec := httptest.NewRecorder()
	siteReq := httptest.NewRequest(http.MethodPost, "/t/merchant/sites", bytes.NewBufferString(`{}`))
	siteReq.Header.Set("Content-Type", "application/json")
	siteRouter.ServeHTTP(siteRec, siteReq)
	var siteResp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(siteRec.Body.Bytes(), &siteResp)
	if siteResp.Message != "参数不合法：名称不能为空" {
		t.Errorf("site.CreateSite: message = %q, want 参数不合法：名称不能为空", siteResp.Message)
	}
	captured["SITE"] = siteResp.Message

	t.Logf("BIND2603_CASES_TOTAL=%d", total)
	t.Logf("BIND2603_CASES_FAIL=%d", caseFail)
	t.Logf("BIND2603_ASCII_LEAK=%d", asciiLeak)
	t.Logf("BIND2603_RAW_PHRASE_LEAK=%d", rawLeak)
	t.Logf("BIND2603_SITE_MSG=%s", captured["SITE"])
	t.Logf("BIND2603_PROPERTY_MSG=%s", captured["PROPERTY"])
	t.Logf("BIND2603_WAREHOUSE_MSG=%s", captured["WAREHOUSE"])
	t.Logf("BIND2603_COUPON_MSG=%s", captured["COUPON"])
	t.Logf("BIND2603_SYNTAX_MSG=%s", syntaxMsg)

	if total != 52 {
		t.Errorf("BIND2603_CASES_TOTAL=%d, want 52", total)
	}
}
