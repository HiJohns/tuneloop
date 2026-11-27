package bindmsg

// #2603：请求体绑定失败必须回中文可读文案（点名字段），不得外泄 Go/JSON 原文。
//
// 实测触发（预生产 2026-11-27，网点新建）：DevTools 改包后请求体里出现控制字符，
// 返回 {"code":40002,"message":"invalid parameters: invalid character '\x17' in string"}，
// PC 端因 message 无 CJK 降级成通用「信息不完整，请补充必填项」，用户不知道错在哪。

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/require"
)

// 含控制字符的 JSON（等价于预生产那次改包损坏的请求体）
const brokenJSON2603 = "{\"name\":\"复验400-1127\"\x17}"

func TestBindErrorMessage2603(t *testing.T) {
	// 1) JSON 语法错误（控制字符）
	var syntaxErr *json.SyntaxError
	err := json.Unmarshal([]byte(brokenJSON2603), &map[string]interface{}{})
	require.Error(t, err)
	if errors.As(err, &syntaxErr) {
		msg := Message(err)
		require.Equal(t, "请求体格式不正确，请检查后重试", msg)
		require.NotContains(t, msg, "invalid character")
		require.NotContains(t, msg, `\x17`)
	} else {
		t.Fatalf("期望 json.SyntaxError，实际 %T", err)
	}

	// 2) 必填字段缺失 → 点名字段
	validate := validator.New()
	type siteReq struct {
		Name string `json:"name" validate:"required"`
	}
	var req siteReq
	verr := validate.Struct(req)
	require.Error(t, verr)
	msg := Message(verr)
	require.Equal(t, "参数不合法：名称不能为空", msg)

	// 3) 类型不匹配 → 点名字段
	type intReq struct {
		Latitude float64 `json:"latitude"`
	}
	var typed intReq
	typeErr := json.Unmarshal([]byte(`{"latitude":"abc"}`), &typed)
	require.Error(t, typeErr)
	msg = Message(typeErr)
	require.Equal(t, "参数不合法：纬度类型不正确", msg)

	// 4) 未知错误 → 通用中文，绝不外泄原文
	msg = Message(errors.New("EOF"))
	require.Equal(t, "参数不合法，请检查后重试", msg)
	require.NotContains(t, msg, "EOF")

	t.Logf("BIND_MSG_SYNTAX=%s", Message(err))
	t.Logf("BIND_MSG_TYPE=%s", Message(typeErr))
}

// Go 字段名 → JSON 字段名 → 中文称呼 三跳必须打通（validator 只给 Go 字段名）。
func TestBindFieldLabelCamelCase2603(t *testing.T) {
	cases := map[string]string{
		"Name":            "名称",
		"ManagerPhone":    "管理员手机号",
		"TransitSiteID":   "中转网点",
		"ParentID":        "父级网点",
		"PostalCode":      "邮编",
		"BusinessHours":   "营业时间",
		"Latitude":        "纬度",
		"UnknownFieldXYZ": "unknown_field_xyz", // 未登记 → 回显 JSON 名，不回显 Go 名
	}
	for in, want := range cases {
		if got := bindFieldLabel(in); got != want {
			t.Errorf("bindFieldLabel(%q) = %q, want %q", in, got, want)
		}
	}
}
