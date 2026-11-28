package bindmsg

// #2604: bindmsg 扩面（56 条称呼 + 2 个作用域 + 复数缩写 + dive 下标归一）。
//
// 与 #2603 的关系：Message / FieldLabel 的既有签名与默认取值保持**字节不变**
// （变参 scopes 向后兼容）；本文件只锁定本单新增语义，且所有新增称呼不得含
// ASCII 字母（用户可见文案，R1 用 `[A-Za-z]` 计数 0 硬断言）。

import (
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
)

// bind2604NewLabels 是本单新增的 56 条称呼（逐字锁定，计划「字段称呼新增表」）。
var bind2604NewLabels = map[string]string{
	// 账户 / 注册（6）
	"identifier":     "登录账号",
	"nickname":       "昵称",
	"wx_code":        "微信授权码",
	"exchange_token": "微信授权凭证",
	"ref":            "推荐码",
	"agreed_terms":   "协议同意",
	// 权限 / 角色（4）
	"cus_perm_codes":        "权限项列表",
	"role_code":             "角色编码",
	"auto_generate":         "自动生成密码",
	"force_password_change": "强制修改密码",
	// 身份 / 证件（13）
	"biz_token":           "核身凭证",
	"id_card_no":          "身份证号",
	"id_card_expire":      "身份证有效期",
	"id_card_authority":   "身份证签发机关",
	"id_card_address":     "身份证地址",
	"real_name":           "真实姓名",
	"id_photo_front":      "证件正面照",
	"id_photo_back":       "证件反面照",
	"other_cert_photo":    "其它证件照",
	"id_photos":           "证件照",
	"id_photo_other_type": "其它证件类型",
	"storage_key":         "文件标识",
	"side":                "证件面",
	// 确认 / 结算（8）
	"confirm_type":       "确认方式",
	"confirm_target":     "确认目标",
	"action_type":        "操作类型",
	"action_target_id":   "操作对象",
	"receiver_type":      "收款方类型",
	"receiver_account":   "收款方账号",
	"profit_share_ratio": "分成比例",
	"is_enabled":         "启用开关",
	// 乐器 / 媒体 / 价格（12）
	"ids":             "乐器列表",
	"batch_type":      "批次类型",
	"is_display":      "展示开关",
	"object_type":     "对象类型",
	"object_id":       "对象标识",
	"files":           "文件列表",
	"file_key":        "文件标识",
	"file_type":       "文件类型",
	"template_id":     "价格模板",
	"config":          "配置项",
	"base_daily_rate": "基础日租金",
	"overrides":       "覆盖项",
	// 用户 / 地址（7）
	"membership_level_id": "会员等级",
	"recipient_name":      "收件人",
	"province":            "省份",
	"city":                "城市",
	"district":            "区县",
	"detail":              "详细地址",
	"is_default":          "默认地址",
	// 邮件（5）
	"host":    "邮件服务器",
	"port":    "端口",
	"from":    "发件邮箱",
	"use_tls": "加密传输",
	"enabled": "启用状态",
	// 通用（1）
	"user_id": "用户编号",
}

// bind2604ExistingLabels 是本单 25 端点中复用 #2603 既有登记的 15 条称呼。
var bind2604ExistingLabels = map[string]string{
	"address":     "地址",
	"code":        "优惠码",
	"company":     "物流公司",
	"email":       "邮箱",
	"id":          "编号",
	"items":       "乐器列表",
	"merchant_id": "商户",
	"name":        "名称",
	"password":    "密码",
	"phone":       "手机号",
	"postal_code": "邮编",
	"sort_order":  "排序",
	"status":      "状态",
	"title":       "标题",
	"username":    "账号",
}

func bind2604HasASCII(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

// TestBindMsgLabels2604 锁定 56 条新增称呼逐条等值 + 无 ASCII 泄漏。
func TestBindMsgLabels2604(t *testing.T) {
	if len(bind2604NewLabels) != 56 {
		t.Fatalf("bind2604NewLabels must hold 56 entries, got %d", len(bind2604NewLabels))
	}
	asciiLeak := 0
	for field, want := range bind2604NewLabels {
		got, ok := bindFieldLabels[field]
		if !ok {
			t.Errorf("bindFieldLabels missing new field %q (#2604)", field)
			continue
		}
		if got != want {
			t.Errorf("bindFieldLabels[%q] = %q, want %q", field, got, want)
		}
		if bind2604HasASCII(got) {
			t.Errorf("label for %q contains ASCII letters: %q", field, got)
			asciiLeak++
		}
	}
	t.Logf("LABEL_NEW_2604=%d", len(bind2604NewLabels))
	t.Logf("LABEL_ASCII_LEAK=%d", asciiLeak)
}

// TestBindMsgCoverage2604 断言 25 端点 71 个 JSON 名全部命中称呼表
// （56 新增 + 15 既有），未登记时会回落 JSON snake 名（ASCII）。
func TestBindMsgCoverage2604(t *testing.T) {
	missing := 0
	seen := make(map[string]bool, 71)
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
		if bind2604HasASCII(label) {
			t.Errorf("label for %q leaks ASCII: %q", field, label)
		}
	}
	for field := range bind2604NewLabels {
		check(field, "new-2604")
	}
	for field := range bind2604ExistingLabels {
		check(field, "existing-2603")
	}
	if len(seen) != 71 {
		t.Errorf("coverage union must be 71 fields, got %d", len(seen))
	}
	t.Logf("LABEL_COVERAGE_TOTAL=%d", len(seen))
	t.Logf("LABEL_COVERAGE_MISSING=%d", missing)
}

// TestBindMsgScope2604 锁定两个作用域覆盖与「默认作用域不漂移」。
func TestBindMsgScope2604(t *testing.T) {
	validate := validator.New()

	personErr := validate.Struct(personScopeFixture{})
	if personErr == nil {
		t.Fatal("person fixture unexpectedly valid")
	}
	roleErr := validate.Struct(roleScopeFixture{})
	if roleErr == nil {
		t.Fatal("role fixture unexpectedly valid")
	}
	companyErr := validate.Struct(companyFixture{})
	if companyErr == nil {
		t.Fatal("company fixture unexpectedly valid")
	}
	titleErr := validate.Struct(titleFixture{})
	if titleErr == nil {
		t.Fatal("title fixture unexpectedly valid")
	}

	personName := Message(personErr, ScopePerson)
	if personName != "参数不合法：姓名不能为空" {
		t.Errorf("ScopePerson name = %q, want 参数不合法：姓名不能为空", personName)
	}
	personCompany := Message(companyErr, ScopePerson)
	if personCompany != "参数不合法：工作单位不能为空" {
		t.Errorf("ScopePerson company = %q, want 参数不合法：工作单位不能为空", personCompany)
	}
	personTitle := Message(titleErr, ScopePerson)
	if personTitle != "参数不合法：职务不能为空" {
		t.Errorf("ScopePerson title = %q, want 参数不合法：职务不能为空", personTitle)
	}
	roleCode := Message(roleErr, ScopeRole)
	if roleCode != "参数不合法：角色编码不能为空" {
		t.Errorf("ScopeRole code = %q, want 参数不合法：角色编码不能为空", roleCode)
	}
	roleName := Message(personErr, ScopeRole)
	if roleName != "参数不合法：角色名称不能为空" {
		t.Errorf("ScopeRole name = %q, want 参数不合法：角色名称不能为空", roleName)
	}

	// 默认作用域不得漂移（#2603 既有断言）。
	defaultCode := Message(roleErr)
	if defaultCode != "参数不合法：优惠码不能为空" {
		t.Errorf("default code = %q, want 参数不合法：优惠码不能为空 (default scope drifted)", defaultCode)
	}
	defaultName := Message(personErr)
	if defaultName != "参数不合法：名称不能为空" {
		t.Errorf("default name = %q, want 参数不合法：名称不能为空 (default scope drifted)", defaultName)
	}
	// 显式传 ScopeDefault 与不传等价。
	if got := Message(personErr, ScopeDefault); got != defaultName {
		t.Errorf("explicit ScopeDefault = %q, want %q", got, defaultName)
	}

	// scope 行统一输出「称呼」本身（= 计划 G0 R1 的期望 token），完整文案由上方
	// 的 Message 断言锁定；两处口径不再用一个 token 混表（Round 1 审计失败项 3）。
	t.Logf("SCOPE_PERSON_NAME=%s", FieldLabel("name", ScopePerson))
	t.Logf("SCOPE_PERSON_COMPANY=%s", FieldLabel("company", ScopePerson))
	t.Logf("SCOPE_ROLE_CODE=%s", FieldLabel("code", ScopeRole))
	t.Logf("SCOPE_DEFAULT_CODE=%s", FieldLabel("code"))
	t.Logf("SCOPE_DEFAULT_NAME=%s", FieldLabel("name"))
}

// TestBindMsgCamelToSnake2604 锁定复数缩写（本单）与既有缩写断言（#2603 不回归）。
func TestBindMsgCamelToSnake2604(t *testing.T) {
	cases := []struct{ in, want string }{
		{"IDs", "ids"},
		{"UserIDs", "user_ids"},
		{"ID", "id"},
		{"URL", "url"},
		{"URLs", "urls"},
		{"TransitSiteID", "transit_site_id"},
		{"ManagerPhone", "manager_phone"},
		{"InstrumentSN", "instrument_sn"},
	}
	for _, tc := range cases {
		if got := camelToSnake(tc.in); got != tc.want {
			t.Errorf("camelToSnake(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	t.Logf("CAMEL_IDS=%s", camelToSnake("IDs"))
	t.Logf("CAMEL_USER_IDS=%s", camelToSnake("UserIDs"))
	t.Logf("CAMEL_TRANSIT_SITE_ID=%s", camelToSnake("TransitSiteID"))
}

// TestBindMsgDiveIndex2604 锁定 dive 下标归一：`IDs[0]` 与 `IDs` 同称呼。
func TestBindMsgDiveIndex2604(t *testing.T) {
	validate := validator.New()

	diveErr := validate.Struct(diveIDsFixture{IDs: []string{""}})
	if diveErr == nil {
		t.Fatal("dive fixture unexpectedly valid")
	}
	diveMsg := Message(diveErr)
	if diveMsg != "参数不合法：乐器列表不能为空" {
		t.Errorf("dive message = %q, want 参数不合法：乐器列表不能为空", diveMsg)
	}
	if got := FieldLabel("IDs[12]"); got != "乐器列表" {
		t.Errorf("FieldLabel(IDs[12]) = %q, want 乐器列表", got)
	}
	if got := FieldLabel("IDs[0]"); got != FieldLabel("IDs") {
		t.Errorf("FieldLabel(IDs[0]) = %q, FieldLabel(IDs) = %q", got, FieldLabel("IDs"))
	}
	// 非下标方括号不得被误剥（回落 JSON snake 名，不得 panic）。
	if got := FieldLabel("weird[name]"); got == "" {
		t.Error("FieldLabel(weird[name]) must not be empty")
	}
	if bind2604HasASCII(diveMsg) {
		t.Errorf("dive message contains ASCII letters: %q", diveMsg)
	}
	t.Logf("DIVE_INDEX_LABEL=%s", strings.TrimPrefix(FieldLabel("IDs[0]"), "参数不合法："))
}

// ---- fixtures (validator reports the struct field name) ----

type personScopeFixture struct {
	Name string `validate:"required"`
}

type roleScopeFixture struct {
	Code string `validate:"required"`
}

type companyFixture struct {
	Company string `validate:"required"`
}

type titleFixture struct {
	Title string `validate:"required"`
}

type diveIDsFixture struct {
	IDs []string `validate:"required,dive,required"`
}
