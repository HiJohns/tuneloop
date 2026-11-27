package bindmsg

// #2603：请求体绑定失败的用户可读文案（包内唯一出口，纯函数、无外部依赖）。
//
// 背景：大量 handler 用 `"invalid parameters: " + err.Error()` 直接回传 Go 绑定层原文，
// 例如预生产实测（2026-11-27，网点新建）：
//
//	{"code":40002,"message":"invalid parameters: invalid character '\x17' in string"}
//
// 问题有两层：
//  1. 对用户毫无意义（Go 标准库/sql 层措辞）；
//  2. PC 端 resolveErrorMessage 的「含 CJK 原样透传」口径（#2480）判其无具体原因，
//     于是降级成通用文案 `信息不完整，请补充必填项` —— 用户不知道是哪个字段错。
//
// 本文件提供 `Message(err)`：把 validator/encoding-json 错误翻译成中文，
// 并尽量点名字段（`required` / 类型不匹配 / JSON 语法错误）。

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

// bindFieldLabels 把常见 JSON 字段名映射为界面上的字段称呼（未登记则原样回显字段名）。
var bindFieldLabels = map[string]string{
	"name":             "名称",
	"address":          "地址",
	"phone":            "联系电话",
	"type":             "类型",
	"postal_code":      "邮编",
	"business_hours":   "营业时间",
	"parent_id":        "父级网点",
	"merchant_id":      "商户",
	"transit_site_id":  "中转网点",
	"manager_id":       "管理员",
	"manager_name":     "管理员姓名",
	"manager_username": "管理员账号",
	"manager_email":    "管理员邮箱",
	"manager_phone":    "管理员手机号",
	"latitude":         "纬度",
	"longitude":        "经度",
	"description":      "说明",
	"images":           "图片",
	"photos":           "照片",
	"reason":           "原因",
	"comment":          "评论",
	"status":           "状态",
	"role":             "角色",
	"user_id":          "用户",
	"user_ids":         "用户列表",
	"amount":           "金额",
	"quantity":         "数量",
}

// bindFieldLabel 把 validator 给出的 **Go 结构体字段名**（如 ManagerPhone / TransitSiteID）
// 还原成 JSON 字段名（manager_phone / transit_site_id）再查中文称呼。
func bindFieldLabel(field string) string {
	if label, ok := bindFieldLabels[field]; ok {
		return label
	}
	snake := camelToSnake(field)
	if label, ok := bindFieldLabels[snake]; ok {
		return label
	}
	if snake != "" {
		return snake
	}
	return field
}

// camelToSnake 把 Go 字段名转为 JSON snake_case：ManagerPhone→manager_phone、
// TransitSiteID→transit_site_id、UserIDs→user_ids、Name→name。
func camelToSnake(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		if r >= 'A' && r <= 'Z' {
			prevLower := i > 0 && (runes[i-1] >= 'a' && runes[i-1] <= 'z' || runes[i-1] >= '0' && runes[i-1] <= '9')
			nextLower := i+1 < len(runes) && runes[i+1] >= 'a' && runes[i+1] <= 'z'
			if i > 0 && (prevLower || nextLower) {
				b.WriteRune('_')
			}
			b.WriteRune(r + ('a' - 'A'))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Message 把 c.ShouldBindJSON 的错误翻译成中文（绝不含 Go/JSON 内部措辞）。
// Message 把 c.ShouldBindJSON 的错误翻译成中文（绝不含 Go/JSON 内部措辞）。
func Message(err error) string {
	if err == nil {
		return "参数不合法，请检查后重试"
	}

	// 1) 字段校验失败（binding:"required" 等）
	var verrs validator.ValidationErrors
	if errors.As(err, &verrs) && len(verrs) > 0 {
		msgs := make([]string, 0, len(verrs))
		for _, fe := range verrs {
			label := bindFieldLabel(fe.Field())
			switch fe.Tag() {
			case "required":
				msgs = append(msgs, fmt.Sprintf("%s不能为空", label))
			case "email":
				msgs = append(msgs, fmt.Sprintf("%s格式不正确", label))
			case "uuid", "uuid4":
				msgs = append(msgs, fmt.Sprintf("%s不是合法的唯一标识", label))
			case "numeric", "number", "gte", "lte", "gt", "lt":
				msgs = append(msgs, fmt.Sprintf("%s取值不合法", label))
			case "oneof":
				msgs = append(msgs, fmt.Sprintf("%s取值不在允许范围内", label))
			default:
				msgs = append(msgs, fmt.Sprintf("%s不合法", label))
			}
		}
		return "参数不合法：" + strings.Join(msgs, "；")
	}

	// 2) JSON 类型不匹配（如把数字填成字符串）
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) && typeErr.Field != "" {
		return fmt.Sprintf("参数不合法：%s类型不正确", bindFieldLabel(typeErr.Field))
	}

	// 3) JSON 语法错误（含控制字符 / 截断 / 多余的逗号）
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return "请求体格式不正确，请检查后重试"
	}

	// 4) 其余（含 EOF、未知解码错误）→ 通用中文，绝不外泄原文
	return "参数不合法，请检查后重试"
}
