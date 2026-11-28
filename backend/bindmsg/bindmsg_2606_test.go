package bindmsg

import (
	"regexp"
	"testing"
)

// #2606 R1 口径：新增 10 条乐器字段称呼逐字锁定，且称呼不含 ASCII 字母。
func TestBindMsgLabels2606(t *testing.T) {
	want := map[string]string{
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
	}
	ascii := regexp.MustCompile(`[A-Za-z]`)
	for field, label := range want {
		got, ok := bindFieldLabels[field]
		if !ok {
			t.Fatalf("label for %q not registered", field)
		}
		if got != label {
			t.Fatalf("label for %q = %q, want %q", field, got, label)
		}
		if ascii.MatchString(got) {
			t.Fatalf("label for %q contains ASCII letters: %q", field, got)
		}
	}

	// 覆盖断言：本单 10 条 + 基座已登记的 5 条（status/deposit/deposit_reduced_enabled/
	// deposit_reduced/description）必须全部命中；`site_id`/`base_daily_rate`/`images`
	// 的称呼由未合入的 #2604（base_daily_rate）及其余批次补齐，本单不重复登记
	// （否则合并时 map 重复键 → 编译失败）。
	base := []string{"status", "deposit", "deposit_reduced_enabled", "deposit_reduced", "description"}
	missing := 0
	for field := range want {
		if _, ok := bindFieldLabels[field]; !ok {
			missing++
		}
	}
	for _, field := range base {
		if _, ok := bindFieldLabels[field]; !ok {
			missing++
		}
	}
	if missing != 0 {
		t.Fatalf("label coverage missing = %d, want 0", missing)
	}

	// 全部已登记称呼都不得含 ASCII 字母（响应面向中文用户）。
	for field, label := range bindFieldLabels {
		if ascii.MatchString(label) {
			t.Fatalf("registered label for %q contains ASCII letters: %q", field, label)
		}
	}
}
