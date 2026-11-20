package services

import "strings"

// #2428：冲突检测的掩码/匹配纯函数（前端只展示后端值，禁止自行实现规则）。
func MaskPhone(phone string) string {
	if phone == "" {
		return ""
	}
	if len(phone) < 9 {
		return strings.Repeat("*", len(phone))
	}
	return phone[:3] + "****" + phone[len(phone)-4:]
}

func MaskEmail(email string) string {
	if email == "" {
		return ""
	}
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return strings.Repeat("*", len(email))
	}
	local, domain := email[:at], email[at:]
	if len(local) <= 2 {
		return "**" + domain
	}
	return local[:2] + "****" + domain
}

func MaskUsername(name string) string {
	if name == "" {
		return ""
	}
	if len(name) <= 4 {
		return strings.Repeat("*", len(name))
	}
	return name[:2] + "****" + name[len(name)-1:]
}

// MatchIdentifier 返回本次查询命中的字段名（仅字段名，不含值）。
func MatchIdentifier(phone, email, username, qPhone, qEmail, qUsername string) []string {
	fields := make([]string, 0, 3)
	if qPhone != "" && phone == qPhone {
		fields = append(fields, "phone")
	}
	if qEmail != "" && email == qEmail {
		fields = append(fields, "email")
	}
	if qUsername != "" && username == qUsername {
		fields = append(fields, "username")
	}
	return fields
}
