package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// #1969：动作/资源词典覆盖（中间件路由映射表全矩阵）+ UA 摘要
func TestAuditDisplay_Dictionaries(t *testing.T) {
	cases := []struct{ action, resource, want string }{
		{"MARK_READ", "notification", "标记已读通知"},
		{"CREATE", "order", "创建订单"},
		{"UPDATE", "instrument", "更新乐器"},
		{"SHIPPING", "order", "发货订单"},
		{"APPROVE", "label", "审核通过标签"},
		{"UNKNOWN_ACT", "order", "UNKNOWN_ACT"}, // 未知动作回退原文
	}
	for _, c := range cases {
		assert.Equal(t, c.want, AuditActionLabel(c.action, c.resource), c.action)
	}
	assert.Equal(t, "订单", AuditResourceLabel("order"))
	assert.Equal(t, "zzz", AuditResourceLabel("zzz"), "未知资源回退原文")
}

func TestAuditDisplay_UserAgent(t *testing.T) {
	weapp := "Mozilla/5.0 (Linux; Android 12; HBN-AL80) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/150.0 Mobile Safari/537.36 XWEB/1500135 MMWEBSDK/20260801 MicroMessenger/8.0.78 Weixin MiniProgramEnv/android"
	assert.Contains(t, ParseUserAgent(weapp), "微信小程序")
	assert.Contains(t, ParseUserAgent(weapp), "Android")

	desktop := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"
	ua := ParseUserAgent(desktop)
	assert.Contains(t, ua, "Windows")
	assert.Contains(t, ua, "Chrome")

	assert.Equal(t, "raw-agent", ParseUserAgent("raw-agent"), "无法解析回退原文")
	assert.Equal(t, "", ParseUserAgent(""))
}
