package bindmsg

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
)

// Fixtures: one struct per validator tag so the reported Field() is the field
// under test (validator.Var would leave Field() empty).
type requiredFixture struct {
	Name string `validate:"required"`
}

type emailFixture struct {
	Email string `validate:"email"`
}

type uuidFixture struct {
	ID string `validate:"uuid"`
}

type rangeFixture struct {
	Quantity int `validate:"gte=1"`
}

type oneofFixture struct {
	Status string `validate:"oneof=available rented"`
}

type otherTagFixture struct {
	Name string `validate:"alpha"`
}

// bind2603CoveredFields is the union of every request-body field on the 26
// handler binding points收敛 in #2603 (plus the two site.go points from the
// seed). It is derived from the actual struct tags in backend/handlers:
//
//	site.go (16+8), property.go (5 structs), warehouse.go (4),
//	wechatpay_prepay.go (2), renewal.go (2), mall_image.go (2),
//	inventory.go (2), iam_proxy.go (2), banner.go (2), appeal.go (2),
//	payment_calculate.go (1), instrument.go (1), coupon_pricing.go (1)
var bind2603CoveredFields = []string{
	// site.go
	"name", "address", "type", "latitude", "longitude", "phone", "postal_code",
	"business_hours", "parent_id", "manager_id", "manager_name",
	"manager_username", "manager_email", "manager_phone", "transit_site_id",
	"merchant_id",
	// property.go
	"property_type", "unit", "description", "options", "scope_type",
	"related_category_id", "related_property_id", "property_id", "value",
	"property_name", "new_value", "source_value", "target_value",
	// warehouse.go
	"tracking_number", "company", "shipped_at", "shipping_fee", "delivered_at",
	"photos", "instrument_sn", "scan_time", "condition", "notes",
	"damage_amount", "overdue_fee", "additional_shipping_fee",
	"damage_description",
	// wechatpay_prepay.go
	"order_id", "order_type", "amount", "open_id", "gift_used", "session_id",
	"coupon_code", "deposit_tier", "guarantor_ids", "out_trade_no",
	// renewal.go
	"additional_days",
	// mall_image.go / banner.go
	"image_url", "link_url", "title", "sort_order", "status", "bg_color",
	// inventory.go
	"asset_id", "from_site_id", "to_site_id", "reason", "id", "daily_rent",
	"deposit", "deposit_reduced_enabled", "deposit_reduced", "overdue_daily_fee",
	"items",
	// iam_proxy.go
	"email", "username", "password", "role", "skip_activation",
	// appeal.go
	"decision", "adjust_amount", "comment", "damage_report_id", "appeal_reason",
	// instrument.go / coupon_pricing.go
	"stock_status", "code", "amount_cents",
}

func isASCIILetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func hasASCIILetter(s string) bool {
	for _, r := range s {
		if isASCIILetter(r) {
			return true
		}
	}
	return false
}

// TestBindErrorMessage2603 locks the nine message templates of #2603 D2.
func TestBindErrorMessage2603(t *testing.T) {
	validate := validator.New()

	requiredErr := validate.Struct(requiredFixture{})
	if requiredErr == nil {
		t.Fatal("required fixture unexpectedly valid")
	}
	emailErr := validate.Struct(emailFixture{Email: "not-an-email"})
	if emailErr == nil {
		t.Fatal("email fixture unexpectedly valid")
	}
	uuidErr := validate.Struct(uuidFixture{ID: "not-a-uuid"})
	if uuidErr == nil {
		t.Fatal("uuid fixture unexpectedly valid")
	}
	rangeErr := validate.Struct(rangeFixture{Quantity: 0})
	if rangeErr == nil {
		t.Fatal("range fixture unexpectedly valid")
	}
	oneofErr := validate.Struct(oneofFixture{Status: "bogus"})
	if oneofErr == nil {
		t.Fatal("oneof fixture unexpectedly valid")
	}
	otherErr := validate.Struct(otherTagFixture{Name: "张三"})
	if otherErr == nil {
		t.Fatal("alpha fixture unexpectedly valid")
	}

	// Real decoder errors, not synthetic values.
	var target struct {
		Latitude float64 `json:"latitude"`
	}
	typeErr := json.Unmarshal([]byte(`{"latitude":"abc"}`), &target)
	if typeErr == nil {
		t.Fatal("type fixture unexpectedly valid")
	}
	var syntaxTarget struct {
		Name string `json:"name"`
	}
	syntaxErr := json.Unmarshal([]byte("{\"name\":\"\x17\"}"), &syntaxTarget)
	if syntaxErr == nil {
		t.Fatal("syntax fixture unexpectedly valid")
	}

	cases := []struct {
		name string
		err  error
		want string
	}{
		{"required", requiredErr, "参数不合法：名称不能为空"},
		{"email", emailErr, "参数不合法：邮箱格式不正确"},
		{"uuid", uuidErr, "参数不合法：编号不是合法的唯一标识"},
		{"range", rangeErr, "参数不合法：数量取值不合法"},
		{"oneof", oneofErr, "参数不合法：状态取值不在允许范围内"},
		{"other", otherErr, "参数不合法：名称不合法"},
		{"type", typeErr, "参数不合法：纬度类型不正确"},
		{"syntax", syntaxErr, "请求体格式不正确，请检查后重试"},
		{"eof", io.EOF, "参数不合法，请检查后重试"},
		{"nil", nil, "参数不合法，请检查后重试"},
	}

	for _, tc := range cases {
		got := Message(tc.err)
		if got != tc.want {
			t.Errorf("Message(%s) = %q, want %q", tc.name, got, tc.want)
		}
	}

	// Token lines consumed by scripts/verify-2603.sh r1.
	t.Logf("BMSG_REQUIRED=%s", Message(requiredErr))
	t.Logf("BMSG_EMAIL=%s", Message(emailErr))
	t.Logf("BMSG_UUID=%s", Message(uuidErr))
	t.Logf("BMSG_RANGE=%s", Message(rangeErr))
	t.Logf("BMSG_ONEOF=%s", Message(oneofErr))
	t.Logf("BMSG_OTHER=%s", Message(otherErr))
	t.Logf("BMSG_TYPE=%s", Message(typeErr))
	t.Logf("BMSG_SYNTAX=%s", Message(syntaxErr))
	t.Logf("BMSG_FALLBACK=%s", Message(io.EOF))

	// Negative: no template may leak Go/JSON phrasing or ASCII letters.
	forbidden := []string{"invalid", "cannot unmarshal", "Field validation", "json:", "EOF"}
	for _, tc := range cases {
		got := Message(tc.err)
		if hasASCIILetter(got) {
			t.Errorf("Message(%s) contains ASCII letters: %q", tc.name, got)
		}
		for _, word := range forbidden {
			if word != "" && strings.Contains(strings.ToLower(got), strings.ToLower(word)) {
				t.Errorf("Message(%s) leaks raw phrase %q: %q", tc.name, word, got)
			}
		}
	}
}

// TestBindFieldLabelsCoverage2603 asserts the 称呼 table covers every field of
// the 26 endpoints and that no label contains ASCII letters (#2603 D3).
func TestBindFieldLabelsCoverage2603(t *testing.T) {
	missing := 0
	asciiLeak := 0
	seen := make(map[string]bool, len(bind2603CoveredFields))
	for _, field := range bind2603CoveredFields {
		if seen[field] {
			t.Errorf("coverage list contains duplicate field %q", field)
		}
		seen[field] = true
		label, ok := bindFieldLabels[field]
		if !ok {
			t.Errorf("bindFieldLabels missing field %q", field)
			missing++
			continue
		}
		if label == "" {
			t.Errorf("bindFieldLabels[%q] is empty", field)
			missing++
			continue
		}
		if hasASCIILetter(label) {
			t.Errorf("bindFieldLabels[%q] = %q contains ASCII letters", field, label)
			asciiLeak++
		}
	}

	// Every label in the table must be ASCII-free, even ones outside the
	// 26-endpoint union (they are reachable through the same handler paths).
	for field, label := range bindFieldLabels {
		if hasASCIILetter(label) {
			t.Errorf("label for %q leaks ASCII: %q", field, label)
			asciiLeak++
		}
	}

	// Negative: an unregistered field falls back to the JSON snake_case name,
	// never to a Go identifier.
	if got := FieldLabel("ManagerPhone"); got != "管理员手机号" {
		t.Errorf("FieldLabel(ManagerPhone) = %q, want 管理员手机号", got)
	}
	if got := FieldLabel("unknown_field_xyz"); got != "unknown_field_xyz" {
		t.Errorf("FieldLabel(unknown_field_xyz) = %q, want the JSON name", got)
	}
	if got := FieldLabel(""); got != "" {
		t.Errorf("FieldLabel(\"\") = %q, want empty", got)
	}

	t.Logf("LABEL_TABLE_SIZE=%d", len(bindFieldLabels))
	t.Logf("LABEL_COVERAGE_FIELDS=%d", len(bind2603CoveredFields))
	t.Logf("LABEL_COVERAGE_MISSING=%d", missing)
	t.Logf("LABEL_ASCII_LEAK=%d", asciiLeak)
}
