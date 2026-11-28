// Package bindmsg renders request-body binding failures as user-facing Chinese
// messages.
//
// It is the single exit for `c.ShouldBindJSON` failures (#2603): handlers must
// never concatenate `err.Error()` (Go `encoding/json` / validator wording) into
// an API response. The raw text leaks internal phrasing such as
// `invalid character '\x17' in string`, which the PC/H5 clients treat as "no
// specific reason" and replace with a generic fallback — the user then cannot
// tell which field is wrong.
//
// The package is a leaf (no gin dependency) so it can be unit-tested directly.
package bindmsg

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode"

	"github.com/go-playground/validator/v10"
)

const (
	// messagePrefix opens every field-scoped message.
	messagePrefix = "参数不合法："
	// SyntaxMessage is used for malformed JSON bodies (control characters,
	// truncation, stray commas). It never names a field because the body as a
	// whole is unusable.
	SyntaxMessage = "请求体格式不正确，请检查后重试"
	// FallbackMessage is used when the error carries nothing we can name
	// (empty body, nil error, unknown decoder errors).
	FallbackMessage = "参数不合法，请检查后重试"
)

// Scope disambiguates JSON names whose user-facing wording differs by domain
// (#2604 D4). The global table keeps a single owner; a scope only overrides the
// few names that genuinely mean different things (`code` = 优惠码 in coupons but
// 角色编码 in roles; `name` = 名称 / 姓名 / 角色名称).
type Scope int

const (
	// ScopeDefault is the global wording (organization / content domain).
	ScopeDefault Scope = iota
	// ScopePerson covers person records: name→姓名, company→工作单位, title→职务.
	ScopePerson
	// ScopeRole covers role management: name→角色名称, code→角色编码.
	ScopeRole
	// ScopeWx covers WeChat login codes: code→微信授权码 (#2605 D4).
	ScopeWx
	// ScopeCategory covers category ordering payloads: items→分类列表 (#2605 D4).
	ScopeCategory
)

// scopeLabels holds the scoped overrides (5 entries, #2604 D3/D4). It is checked
// before the global table and never duplicates the global wording.
var scopeLabels = map[Scope]map[string]string{
	ScopePerson: {
		"name":    "姓名",
		"company": "工作单位",
		"title":   "职务",
	},
	ScopeRole: {
		"name": "角色名称",
		"code": "角色编码",
	},
	// #2605: the wx.login `code` and the category ordering `items` must not read
	// as 优惠码 / 乐器列表.
	ScopeWx: {
		"code": "微信授权码",
	},
	ScopeCategory: {
		"items": "分类列表",
	},
}

// Message translates a request-body binding error into a Chinese message that
// names the offending field. It never returns the original error text.
//
// Optional scopes (added by #2604) select domain-specific wording; with no
// scope the behaviour is byte-identical to #2603.
//
// Mapping (locked by #2603 Decision Lock D2):
//
//	validator `required`                        → 参数不合法：<称呼>不能为空
//	validator `email`                           → 参数不合法：<称呼>格式不正确
//	validator `uuid` / `uuid4`                  → 参数不合法：<称呼>不是合法的唯一标识
//	validator numeric tags (`gte`/`lte`/…)      → 参数不合法：<称呼>取值不合法
//	validator `oneof`                           → 参数不合法：<称呼>取值不在允许范围内
//	validator other tags                        → 参数不合法：<称呼>不合法
//	*json.UnmarshalTypeError                    → 参数不合法：<称呼>类型不正确
//	*json.SyntaxError                           → 请求体格式不正确，请检查后重试
//	anything else (io.EOF, unexpected EOF, nil) → 参数不合法，请检查后重试
func Message(err error, scopes ...Scope) string {
	if err == nil {
		return FallbackMessage
	}
	scope := joinScope(scopes)

	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) && len(validationErrs) > 0 {
		fe := validationErrs[0]
		label := FieldLabel(fe.Field(), scope)
		switch fe.Tag() {
		case "required":
			return messagePrefix + label + "不能为空"
		case "email":
			return messagePrefix + label + "格式不正确"
		case "uuid", "uuid4":
			return messagePrefix + label + "不是合法的唯一标识"
		case "numeric", "number", "gte", "lte", "gt", "lt", "min", "max":
			return messagePrefix + label + "取值不合法"
		case "oneof":
			return messagePrefix + label + "取值不在允许范围内"
		default:
			return messagePrefix + label + "不合法"
		}
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		field := strings.TrimSpace(typeErr.Field)
		if field == "" {
			return FallbackMessage
		}
		return messagePrefix + FieldLabel(field, scope) + "类型不正确"
	}

	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return SyntaxMessage
	}

	return FallbackMessage
}

// FieldLabel returns the user-facing Chinese name of a request field.
//
// `field` may be a JSON name (`instrument_sn`) or a Go struct field name
// (`InstrumentSN`); both resolve to the same label. Unregistered fields fall
// back to the JSON snake_case name so that a freshly added field still reads as
// an API name — never as a Go identifier such as `ManagerPhone`.
func FieldLabel(field string, scopes ...Scope) string {
	field = strings.TrimSpace(field)
	if field == "" {
		return ""
	}
	// validator reports dive elements as `IDs[0]`; the label belongs to the
	// slice itself (#2604 D9 — controlled exception to #2603 D4, reachable only
	// because instrument.go binds `required,dive,required`).
	if base, ok := stripIndexSuffix(field); ok {
		field = base
	}
	scope := joinScope(scopes)
	if label, ok := scopeLabels[scope][field]; ok {
		return label
	}
	if label, ok := bindFieldLabels[field]; ok {
		return label
	}
	snake := camelToSnake(field)
	if label, ok := scopeLabels[scope][snake]; ok {
		return label
	}
	if label, ok := bindFieldLabels[snake]; ok {
		return label
	}
	return snake
}

// joinScope returns the last explicitly provided scope (callers pass at most
// one). Unknown scopes fall back to the global wording.
func joinScope(scopes []Scope) Scope {
	if len(scopes) == 0 {
		return ScopeDefault
	}
	return scopes[len(scopes)-1]
}

// stripIndexSuffix turns `IDs[0]` / `IDs[12]` into `IDs`.
func stripIndexSuffix(field string) (string, bool) {
	open := strings.IndexByte(field, '[')
	if open <= 0 || !strings.HasSuffix(field, "]") {
		return field, false
	}
	if _, err := strconv.Atoi(field[open+1 : len(field)-1]); err != nil {
		return field, false
	}
	return field[:open], true
}

// bindFieldLabels is the single owner of field→称呼 mapping (#2603 D1/D3).
// Every label must be free of ASCII letters: the response is consumed by
// Chinese users and asserted by R1/R2 with `[A-Za-z]` counting 0.
var bindFieldLabels = map[string]string{
	// 组织 / 网点（site.go）
	"name":             "名称",
	"address":          "地址",
	"type":             "类型",
	"latitude":         "纬度",
	"longitude":        "经度",
	"phone":            "手机号",
	"postal_code":      "邮编",
	"business_hours":   "营业时间",
	"parent_id":        "父级网点",
	"manager_id":       "管理员",
	"manager_name":     "管理员姓名",
	"manager_username": "管理员账号",
	"manager_email":    "管理员邮箱",
	"manager_phone":    "管理员手机号",
	"transit_site_id":  "中转网点",
	"merchant_id":      "商户",

	// 账户 / 组织（iam_proxy.go）
	"email":           "邮箱",
	"username":        "账号",
	"password":        "密码",
	"role":            "角色",
	"skip_activation": "跳过激活",
	"id":              "编号",

	// 乐器（instrument.go）
	"stock_status": "库存状态",
	"quantity":     "数量",

	// 仓储（warehouse.go）
	"tracking_number":         "物流单号",
	"company":                 "物流公司",
	"shipped_at":              "发货时间",
	"shipping_fee":            "物流费",
	"delivered_at":            "收货时间",
	"instrument_sn":           "乐器序列号",
	"scan_time":               "验收时间",
	"condition":               "乐器状况",
	"notes":                   "备注",
	"damage_amount":           "定损金额",
	"damage_description":      "损坏说明",
	"overdue_fee":             "逾期费",
	"additional_shipping_fee": "追加物流费",
	"photos":                  "照片",

	// 库存（inventory.go）
	"asset_id":                "资产",
	"from_site_id":            "调出网点",
	"to_site_id":              "调入网点",
	"items":                   "乐器列表",
	"daily_rent":              "日租金",
	"deposit":                 "押金",
	"deposit_reduced_enabled": "减免押金开关",
	"deposit_reduced":         "减免后押金",
	"overdue_daily_fee":       "逾期日租金",
	"reason":                  "原因",

	// 申诉（appeal.go）
	"decision":         "处理决定",
	"adjust_amount":    "调整金额",
	"comment":          "意见",
	"damage_report_id": "定损单",
	"appeal_reason":    "申诉原因",

	// 属性（property.go）
	"property_type":       "属性类型",
	"unit":                "单位",
	"description":         "描述",
	"options":             "可选值",
	"scope_type":          "取值范围",
	"related_category_id": "关联分类",
	"related_property_id": "关联属性",
	"property_id":         "属性",
	"property_name":       "属性名称",
	"value":               "属性值",
	"new_value":           "新值",
	"source_value":        "原值",
	"target_value":        "目标值",

	// 内容（banner.go / mall_image.go）
	"image_url":  "图片地址",
	"link_url":   "链接地址",
	"title":      "标题",
	"sort_order": "排序",
	"status":     "状态",
	"bg_color":   "背景色",

	// 支付 / 营销（wechatpay_prepay.go / renewal.go / coupon_pricing.go / payment_calculate.go）
	"order_id":        "订单",
	"order_type":      "订单类型",
	"amount":          "金额",
	"open_id":         "微信身份标识",
	"gift_used":       "乐币抵扣",
	"session_id":      "支付会话",
	"coupon_code":     "优惠码",
	"deposit_tier":    "押金档位",
	"guarantor_ids":   "担保人列表",
	"out_trade_no":    "商户订单号",
	"additional_days": "续租天数",
	"code":            "优惠码",
	"amount_cents":    "金额",

	// ---- #2604 扩面：账户 / 注册 ----
	"identifier":     "登录账号",
	"nickname":       "昵称",
	"wx_code":        "微信授权码",
	"exchange_token": "微信授权凭证",
	"ref":            "推荐码",
	"agreed_terms":   "协议同意",

	// ---- #2604 扩面：权限 / 角色 ----
	"cus_perm_codes":        "权限项列表",
	"role_code":             "角色编码",
	"auto_generate":         "自动生成密码",
	"force_password_change": "强制修改密码",

	// ---- #2604 扩面：身份 / 证件 ----
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

	// ---- #2604 扩面：确认 / 结算 ----
	"confirm_type":       "确认方式",
	"confirm_target":     "确认目标",
	"action_type":        "操作类型",
	"action_target_id":   "操作对象",
	"receiver_type":      "收款方类型",
	"receiver_account":   "收款方账号",
	"profit_share_ratio": "分成比例",
	"is_enabled":         "启用开关",

	// ---- #2604 扩面：乐器 / 媒体 / 价格 ----
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

	// ---- #2604 扩面：用户 / 地址 ----
	"membership_level_id": "会员等级",
	"recipient_name":      "收件人",
	"province":            "省份",
	"city":                "城市",
	"district":            "区县",
	"detail":              "详细地址",
	"is_default":          "默认地址",

	// ---- #2604 扩面：邮件 ----
	"host":    "邮件服务器",
	"port":    "端口",
	"from":    "发件邮箱",
	"use_tls": "加密传输",
	"enabled": "启用状态",

	// ---- #2604 扩面：通用 ----
	"user_id": "用户编号",

	// ---- #2605 扩面：人员 / 微信 ----
	"position":   "职位",
	"site_id":    "网点",
	"user_type":  "用户类型",
	"bio":        "个人简介",
	"new_users":  "新成员列表",
	"roles":      "角色列表",
	"user_ids":   "用户列表",
	"openid":     "微信身份标识",
	"to_user_id": "目标用户",

	// ---- #2605 扩面：商户 ----
	"merchant_type":        "商户类型",
	"transit_address":      "中转地址",
	"transit_phone":        "中转联系电话",
	"transit_contact_name": "中转联系人",
	"admin_uid":            "管理员标识",
	"admin_name":           "管理员姓名",
	"admin_username":       "管理员账号",
	"admin_email":          "管理员邮箱",
	"admin_phone":          "管理员手机号",

	// ---- #2605 扩面：分类 ----
	"icon":    "图标",
	"visible": "是否可见",
	"sort":    "排序值",

	// ---- #2605 扩面：租约 / 押金 ----
	"instrument_id":    "乐器",
	"start_date":       "起租日期",
	"end_date":         "到期日期",
	"monthly_rent":     "月租金",
	"deposit_amount":   "押金金额",
	"lease_id":         "租约",
	"transaction_date": "交易日期",
}

// camelToSnake converts a Go struct field name into its JSON snake_case name.
//
// Run-scoped acronyms are kept as one token: `RelatedCategoryID` →
// `related_category_id`, `InstrumentSN` → `instrument_sn`.
//
// Plural acronyms are normalised first (#2604): `IDs` → `ids`, `UserIDs` →
// `user_ids` — without it the final acronym letter before the plural `s` opened
// a new word (`i_ds`), which leaked ASCII into user-visible messages.
func camelToSnake(s string) string {
	if s == "" {
		return ""
	}
	s = normalizePluralAcronym(s)
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s) + 4)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := runes[i-1]
				nextIsLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
				if unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && nextIsLower) {
					b.WriteRune('_')
				}
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// normalizePluralAcronym rewrites a trailing plural acronym so that its last
// uppercase letter does not start a new snake_case word:
//
//	IDs      → Ids      → ids
//	UserIDs  → UserIds  → user_ids
//	URLs     → urls     (all-uppercase words stay one token)
//	TransitSiteID → unchanged (no plural `s`)
func normalizePluralAcronym(s string) string {
	n := len(s)
	if n < 3 || s[n-1] != 's' || !isASCIIUpper(s[n-2]) || !isASCIIUpper(s[n-3]) {
		return s
	}
	allUpper := true
	for i := 0; i < n-1; i++ {
		if !isASCIIUpper(s[i]) {
			allUpper = false
			break
		}
	}
	if allUpper {
		return strings.ToLower(s)
	}
	return s[:n-2] + strings.ToLower(s[n-2:])
}

func isASCIIUpper(b byte) bool { return b >= 'A' && b <= 'Z' }
