package handlers

const maxPageSize = 100

// clampPageSize 收敛请求的分页大小到 [def, max]：
// raw < 1 取默认值 def；raw > max 截断为 max；否则返回原值。
func clampPageSize(raw, def, max int) int {
	if raw < 1 {
		return def
	}
	if raw > max {
		return max
	}
	return raw
}
