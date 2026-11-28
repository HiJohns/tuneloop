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

// Message translates a request-body binding error into a Chinese message that
// names the offending field. It never returns the original error text.
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
func Message(err error) string {
	return MessageScoped("", err)
}

// MessageScoped renders a binding failure with endpoint-scoped field labels.
//
// Scoped labels only override fields whose JSON name carries a different
// meaning in the given endpoint (#2608): `items` is 乐器列表 for
// `inventory.BatchUpdateRent` but 权益项 for
// `membership_benefits.UpdateLevelBenefits`. The global table stays the single
// owner; a scope only supplies overrides. An unknown/empty scope falls back to
// the global table, so `Message(err)` keeps the exact #2603 semantics.
func MessageScoped(scope string, err error) string {
	if err == nil {
		return FallbackMessage
	}
	label := func(field string) string { return FieldLabelScoped(scope, field) }

	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) && len(validationErrs) > 0 {
		fe := validationErrs[0]
		fieldLabel := label(fe.Field())
		switch fe.Tag() {
		case "required":
			return messagePrefix + fieldLabel + "不能为空"
		case "email":
			return messagePrefix + fieldLabel + "格式不正确"
		case "uuid", "uuid4":
			return messagePrefix + fieldLabel + "不是合法的唯一标识"
		case "numeric", "number", "gte", "lte", "gt", "lt", "min", "max":
			return messagePrefix + fieldLabel + "取值不合法"
		case "oneof":
			return messagePrefix + fieldLabel + "取值不在允许范围内"
		default:
			return messagePrefix + fieldLabel + "不合法"
		}
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		field := strings.TrimSpace(typeErr.Field)
		if field == "" {
			return FallbackMessage
		}
		return messagePrefix + label(field) + "类型不正确"
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
func FieldLabel(field string) string {
	return FieldLabelScoped("", field)
}

// FieldLabelScoped is FieldLabel with endpoint-scoped overrides applied first.
// Scoped labels win only for fields listed in bindFieldLabelsScoped[scope];
// everything else resolves through the global table (#2608 D6).
func FieldLabelScoped(scope, field string) string {
	field = strings.TrimSpace(field)
	if field == "" {
		return ""
	}
	if scoped, ok := bindFieldLabelsScoped[scope]; ok {
		if label, ok := scoped[field]; ok && label != "" {
			return label
		}
	}
	if label, ok := bindFieldLabels[field]; ok {
		return label
	}
	snake := camelToSnake(field)
	if label, ok := bindFieldLabels[snake]; ok {
		return label
	}
	return snake
}

// bindFieldLabelsScoped is the single owner of endpoint-scoped 称呼 overrides
// (#2608 D6). A field lands here only when the same JSON name means different
// things in different endpoints; the global table must never be bent to one
// endpoint. Rule of Three: one real conflict, one scope.
var bindFieldLabelsScoped = map[string]map[string]string{
	"membership_benefits": {
		"items": "权益项",
	},
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

	// 折扣（discount.go，#2608）
	"rent_discount":     "租金折扣",
	"deposit_discount":  "押金折扣",
	"shipping_discount": "物流费折扣",
	"max_amount":        "最高优惠金额",
	"valid_from":        "生效时间",
	"valid_to":          "失效时间",
	"policy_id":         "折扣策略",
	"max_uses":          "最大使用次数",
	"expires_at":        "过期时间",

	// 礼品 / 积分（gift_policies.go、points_policy.go，#2608）
	"level_id":                    "会员等级",
	"pay_ratio":                   "乐币抵扣比例",
	"referral_ratio":              "推荐奖励比例",
	"referral_reg_points":         "推荐注册赠点",
	"is_active":                   "启用状态",
	"pay_ratio_max":               "最高抵扣比例",
	"point_batch_validity_months": "乐币有效期月数",
	"point_expiry_reminder_days":  "到期提醒提前天数",
	"scope_id":                    "作用范围",
	"max_pay_ratio":               "最高抵扣比例",
	"valid_days":                  "有效天数",

	// 推广 / 会员（instrument_promo_override.go、membership_level.go，#2608）
	"override_type": "推广类型",
	"enabled":       "启用状态",
	"content":       "推广内容",
	"min_amount":    "最低消费金额",

	// 返点 / 用户（merchant_rebate.go、user_management.go，#2608）
	"opt_in": "返点开关",

	// 通知（warning.go、webhook_config.go，#2608）
	"emails":           "收件邮箱",
	"cooldown_minutes": "冷却间隔",
	"webhook_enabled":  "企业微信通知开关",
	"webhook_url":      "企业微信通知地址",
	"url":              "通知地址",

	// 缩写复数（#2608 §3）
	"ids": "编号列表",
}

// camelToSnake converts a Go struct field name into its JSON snake_case name.
//
// Run-scoped acronyms are kept as one token: `RelatedCategoryID` →
// `related_category_id`, `InstrumentSN` → `instrument_sn`.
//
// A trailing lowercase `s` whose predecessor is uppercase is a plural suffix,
// not a new token (#2608 §3): `IDs` → `ids`, `GuarantorIDs` → `guarantor_ids`,
// `URLs` → `urls`. Fields whose final `s` follows a lowercase letter
// (`Status`, `Address`, `Notes`) take the normal path and are unaffected.
func camelToSnake(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	last := len(runes) - 1
	var b strings.Builder
	b.Grow(len(s) + 4)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := runes[i-1]
				nextIsLower := i+1 <= last && unicode.IsLower(runes[i+1])
				// `…IDs`: the trailing `s` pluralises the acronym instead of
				// starting a new token, so no underscore before `D`.
				pluralSuffix := i == last-1 && runes[last] == 's'
				if (unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && nextIsLower)) && !pluralSuffix {
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
