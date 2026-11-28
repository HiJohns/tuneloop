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
	if err == nil {
		return FallbackMessage
	}

	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) && len(validationErrs) > 0 {
		fe := validationErrs[0]
		label := FieldLabel(fe.Field())
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
		return messagePrefix + FieldLabel(field) + "类型不正确"
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
	field = strings.TrimSpace(field)
	if field == "" {
		return ""
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
	// #2606：乐器新建/编辑请求体的其余字段称呼（全部不含 ASCII 字母）。
	"category_id":    "分类",
	"level_id":       "等级",
	"level":          "级别",
	"sn":             "序列号",
	"pricing":        "价格配置",
	"total_price":    "总价",
	"video":          "视频",
	"poster":         "海报",
	"specifications": "规格清单",
	"properties":     "属性值",

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
}

// camelToSnake converts a Go struct field name into its JSON snake_case name.
//
// Run-scoped acronyms are kept as one token: `RelatedCategoryID` →
// `related_category_id`, `InstrumentSN` → `instrument_sn`.
func camelToSnake(s string) string {
	if s == "" {
		return ""
	}
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
