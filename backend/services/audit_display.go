package services

import "strings"

// #1969 审计日志展示层（人性化）：动作/资源中文词典 + UA 摘要。
// 动作采用「动词 + 资源」组合式（覆盖中间件路由映射表全矩阵），
// 新增动作/资源时同步 middleware/audit_logger.go 的路由映射表。

var auditVerbLabels = map[string]string{
	"ACCEPT": "受理", "ACTIVATE": "激活", "AGREE": "同意", "APPROVE": "审核通过", "ASSIGN": "指派",
	"BATCH_CREATE": "批量创建", "BATCH_IMPORT": "批量导入", "BATCH_PRICING": "批量定价", "BATCH_UPDATE": "批量更新",
	"CANCEL": "取消", "CHANGE_PASSWORD": "修改密码", "COMPLETE": "完成", "CONFIRM": "确认", "CREATE": "创建",
	"DAMAGE": "定损", "DELETE": "删除", "DELIVERY": "交付", "IMPORT": "导入", "INIT": "初始化", "INSPECT": "验收",
	"INVITE": "邀请", "LAST_MILE": "末段发运", "LOST": "丢失登记", "MARK_READ": "标记已读", "MERGE": "归并",
	"PAY": "支付", "PICKUP": "取件", "PREVIEW": "预览", "QUOTE": "报价", "READY": "备货", "RECEIVE": "收货",
	"RECORD": "记录", "RECOVER": "恢复", "REJECT": "拒绝", "REPORT": "上报", "RESEND_EMAIL": "重发邮件",
	"RESET_PASSWORD": "重置密码", "RESOLVE": "处理", "RETURN": "归还", "RETURN_INSPECT": "归还验收",
	"SCRAP": "报废", "SET_DEFAULT": "设为默认", "SHIP": "发货", "SHIPPING": "发货", "SORT": "排序",
	"START": "开始", "SUBMIT": "提交", "SYNC": "同步", "TERMINATE": "终止", "TRANSFER": "转移",
	"TRANSFER_OWNERSHIP": "转移归属", "UPDATE": "更新", "UPDATE_STATUS": "更新状态",
}

var auditResourceLabels = map[string]string{
	"account": "账号", "address": "地址", "appeal": "申诉", "assessment": "定损", "banner": "轮播图",
	"category": "分类", "confirmation": "确认单", "deposit": "押金", "forwarding_session": "转发会话",
	"iam_user": "IAM 用户", "instrument": "乐器", "instrument_media": "乐器媒体", "inventory": "库存",
	"label": "标签", "lease": "租约", "maintenance_ticket": "维修工单", "maintenance_worker": "维修师傅",
	"merchant": "商户", "notification": "通知", "order": "订单", "organization": "组织", "outbound": "出库",
	"pricing_config": "定价配置", "property": "属性", "property_option": "属性选项", "rent_setting": "租金设置",
	"role": "角色", "role_permission": "角色权限", "setting": "设置", "site": "网点", "site_member": "网点成员",
	"system": "系统", "user": "用户", "user_order": "用户订单", "user_permission": "用户权限",
	"user_rental": "用户租赁", "user_role": "用户角色", "user_self": "本人",
}

// AuditActionLabel 动作中文：动词标签 + 资源标签（组合式）；未知动作回退原文（不 panic）
func AuditActionLabel(action, resourceType string) string {
	verb, ok := auditVerbLabels[action]
	if !ok {
		return action
	}
	if res, ok2 := auditResourceLabels[resourceType]; ok2 {
		return verb + res
	}
	return verb
}

// AuditResourceLabel 资源类型中文；未知回退原文
func AuditResourceLabel(resourceType string) string {
	if l, ok := auditResourceLabels[resourceType]; ok {
		return l
	}
	return resourceType
}

// ParseUserAgent 解析 UA → 「平台 · 系统 · 浏览器」摘要；解析失败回退原文
func ParseUserAgent(ua string) string {
	if strings.TrimSpace(ua) == "" {
		return ""
	}
	parts := make([]string, 0, 3)
	switch {
	case strings.Contains(ua, "MiniProgramEnv") || (strings.Contains(ua, "MicroMessenger") && strings.Contains(ua, "miniProgram")):
		parts = append(parts, "微信小程序")
	case strings.Contains(ua, "MicroMessenger"):
		parts = append(parts, "微信")
	}
	switch {
	case strings.Contains(ua, "Android"):
		parts = append(parts, "Android")
	case strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad") || strings.Contains(ua, "iOS"):
		parts = append(parts, "iOS")
	case strings.Contains(ua, "Windows"):
		parts = append(parts, "Windows")
	case strings.Contains(ua, "Macintosh") || strings.Contains(ua, "Mac OS X"):
		parts = append(parts, "macOS")
	}
	switch {
	case strings.Contains(ua, "Edg"):
		parts = append(parts, "Edge")
	case strings.Contains(ua, "Firefox"):
		parts = append(parts, "Firefox")
	case strings.Contains(ua, "Chrome") || strings.Contains(ua, "XWEB"):
		parts = append(parts, "Chrome")
	case strings.Contains(ua, "Safari"):
		parts = append(parts, "Safari")
	}
	if len(parts) == 0 {
		return ua
	}
	return strings.Join(parts, " · ")
}
