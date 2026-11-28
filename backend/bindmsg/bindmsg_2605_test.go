package bindmsg

// #2605: 27 条称呼 + 2 个作用域（ScopeWx / ScopeCategory）。
//
// 与 #2603 / #2604 的关系：默认作用域取值**不得漂移**（`code`=优惠码、
// `items`=乐器列表），作用域只在显式传入时覆盖；新增称呼不得含 ASCII 字母
// （用户可见文案，R1 用 `[A-Za-z]` 计数 0 硬断言）。

import (
	"testing"

	"github.com/go-playground/validator/v10"
)

// bind2605NewLabels 是本单新增的 27 条称呼（逐字锁定，计划「字段称呼新增表」）。
var bind2605NewLabels = map[string]string{
	// 人员 / 微信（8）
	"position":   "职位",
	"site_id":    "网点",
	"user_type":  "用户类型",
	"bio":        "个人简介",
	"new_users":  "新成员列表",
	"roles":      "角色列表",
	"openid":     "微信身份标识",
	"to_user_id": "目标用户",
	// 商户（9）
	"merchant_type":        "商户类型",
	"transit_address":      "中转地址",
	"transit_phone":        "中转联系电话",
	"transit_contact_name": "中转联系人",
	"admin_uid":            "管理员标识",
	"admin_name":           "管理员姓名",
	"admin_username":       "管理员账号",
	"admin_email":          "管理员邮箱",
	"admin_phone":          "管理员手机号",
	// 分类（3）
	"icon":    "图标",
	"visible": "是否可见",
	"sort":    "排序值",
	// 租约 / 押金（7）
	"instrument_id":    "乐器",
	"start_date":       "起租日期",
	"end_date":         "到期日期",
	"monthly_rent":     "月租金",
	"deposit_amount":   "押金金额",
	"lease_id":         "租约",
	"transaction_date": "交易日期",
}

// bind2605ExistingLabels 是本单 22 端点中复用既有登记的 26 条称呼
// （#2603 基线 + 本单依赖的 #2604 扩面）。
var bind2605ExistingLabels = map[string]string{
	"address":               "地址",
	"amount":                "金额",
	"auto_generate":         "自动生成密码",
	"code":                  "优惠码",
	"email":                 "邮箱",
	"force_password_change": "强制修改密码",
	"id":                    "编号",
	"id_card_no":            "身份证号",
	"id_photo_back":         "证件反面照",
	"id_photo_front":        "证件正面照",
	"id_photo_other_type":   "其它证件类型",
	"items":                 "乐器列表",
	"name":                  "名称",
	"nickname":              "昵称",
	"notes":                 "备注",
	"parent_id":             "父级网点",
	"password":              "密码",
	"phone":                 "手机号",
	"real_name":             "真实姓名",
	"role":                  "角色",
	"skip_activation":       "跳过激活",
	"status":                "状态",
	"type":                  "类型",
	"user_id":               "用户编号",
	"user_ids":              "用户列表",
	"username":              "账号",
}

func bind2605HasASCII(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

// TestBindMsgLabels2605 锁定 27 条新增称呼逐条等值 + 无 ASCII 泄漏。
func TestBindMsgLabels2605(t *testing.T) {
	if len(bind2605NewLabels) != 27 {
		t.Fatalf("bind2605NewLabels must hold 27 entries, got %d", len(bind2605NewLabels))
	}
	asciiLeak := 0
	for field, want := range bind2605NewLabels {
		got, ok := bindFieldLabels[field]
		if !ok {
			t.Errorf("bindFieldLabels missing new field %q (#2605)", field)
			continue
		}
		if got != want {
			t.Errorf("bindFieldLabels[%q] = %q, want %q", field, got, want)
		}
		if bind2605HasASCII(got) {
			t.Errorf("label for %q contains ASCII letters: %q", field, got)
			asciiLeak++
		}
	}
	// 全表断言：任何已登记称呼都不得含 ASCII 字母。
	for field, label := range bindFieldLabels {
		if bind2605HasASCII(label) {
			t.Errorf("registered label for %q contains ASCII letters: %q", field, label)
			asciiLeak++
		}
	}
	t.Logf("LABEL_NEW_2605=%d", len(bind2605NewLabels))
	t.Logf("LABEL_ASCII_LEAK=%d", asciiLeak)
}

// TestBindMsgCoverage2605 断言 22 端点 53 个 JSON 名全部命中称呼表
// （27 新增 + 26 既有），未登记时会回落 JSON snake 名（ASCII）。
func TestBindMsgCoverage2605(t *testing.T) {
	missing := 0
	seen := make(map[string]bool, 53)
	check := func(field, domain string) {
		if seen[field] {
			t.Errorf("coverage list contains duplicate field %q (%s)", field, domain)
			return
		}
		seen[field] = true
		label, ok := bindFieldLabels[field]
		if !ok || label == "" {
			t.Errorf("coverage missing %q (%s)", field, domain)
			missing++
			return
		}
		if bind2605HasASCII(label) {
			t.Errorf("label for %q leaks ASCII: %q", field, label)
		}
	}
	for field := range bind2605NewLabels {
		check(field, "new-2605")
	}
	for field := range bind2605ExistingLabels {
		check(field, "existing")
	}
	if len(seen) != 53 {
		t.Errorf("coverage union must be 53 fields, got %d", len(seen))
	}
	t.Logf("LABEL_COVERAGE_TOTAL=%d", len(seen))
	t.Logf("LABEL_COVERAGE_MISSING=%d", missing)
}

// TestBindMsgScope2605 锁定两个新作用域覆盖与「默认作用域不漂移」。
func TestBindMsgScope2605(t *testing.T) {
	validate := validator.New()

	codeErr := validate.Struct(scopeCodeFixture{})
	if codeErr == nil {
		t.Fatal("code fixture unexpectedly valid")
	}
	itemsErr := validate.Struct(scopeItemsFixture{})
	if itemsErr == nil {
		t.Fatal("items fixture unexpectedly valid")
	}

	wxCode := Message(codeErr, ScopeWx)
	if wxCode != "参数不合法：微信授权码不能为空" {
		t.Errorf("ScopeWx code = %q, want 参数不合法：微信授权码不能为空", wxCode)
	}
	categoryItems := Message(itemsErr, ScopeCategory)
	if categoryItems != "参数不合法：分类列表不能为空" {
		t.Errorf("ScopeCategory items = %q, want 参数不合法：分类列表不能为空", categoryItems)
	}

	// 默认作用域不得漂移（#2603 / #2604 既有断言）。
	defaultCode := Message(codeErr)
	if defaultCode != "参数不合法：优惠码不能为空" {
		t.Errorf("default code = %q, want 参数不合法：优惠码不能为空 (default scope drifted)", defaultCode)
	}
	defaultItems := Message(itemsErr)
	if defaultItems != "参数不合法：乐器列表不能为空" {
		t.Errorf("default items = %q, want 参数不合法：乐器列表不能为空 (default scope drifted)", defaultItems)
	}
	if got := Message(codeErr, ScopeDefault); got != defaultCode {
		t.Errorf("explicit ScopeDefault code = %q, want %q", got, defaultCode)
	}
	// 作用域不串味：ScopeRole 仍按角色语义，不受 ScopeWx 影响。
	roleCode := Message(codeErr, ScopeRole)
	if roleCode != "参数不合法：角色编码不能为空" {
		t.Errorf("ScopeRole code = %q, want 参数不合法：角色编码不能为空", roleCode)
	}

	t.Logf("SCOPE_WX_CODE=%s", FieldLabel("code", ScopeWx))
	t.Logf("SCOPE_CATEGORY_ITEMS=%s", FieldLabel("items", ScopeCategory))
	t.Logf("SCOPE_DEFAULT_CODE=%s", FieldLabel("code"))
	t.Logf("SCOPE_DEFAULT_ITEMS=%s", FieldLabel("items"))
}

// ---- fixtures (validator reports the struct field name) ----

type scopeCodeFixture struct {
	Code string `validate:"required"`
}

type scopeItemsFixture struct {
	Items []string `validate:"required"`
}
