package main

// #2586：乐器数据**进/出**端点的 cus_perm 守卫必须在路由层落地。
//
// 为什么用源码结构断言：handler 级测试无法发现「中间件没挂在生产入口」——
// 即使 handler 被正确调用，缺守卫的路由仍会放行无权限请求。唯一能覆盖
// 「路由表接线」的断言就是读 main.go 的注册行（#2586 Input Truth 工具盲区）。
//
// 绿样（本测试）：导出 = instrument:read；导入族 6 端点 = instrument:create；
// 乐器进/出端点无守卫计数 = 0。
// 红样（由 scripts/verify-2586.sh r3 用 git show <fix>^:backend/main.go 采集）。

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// instrumentIOPaths2586 是本单口径的「乐器数据进/出」端点（D4）。
var instrumentIOPaths2586 = map[string]string{
	"GET /instruments/export":                "instrument:read",
	"POST /instruments/import":               "instrument:create",
	"GET /instruments/import/template":       "instrument:create",
	"GET /instruments/batch-import/template": "instrument:create",
	"POST /instruments/batch-import":         "instrument:create",
	"POST /instruments/batch-import/preview": "instrument:create",
	"POST /instruments/batch-import/media":   "instrument:create",
}

func readMainSource2586(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	return string(raw)
}

// routeLine2586 找出注册指定端点的那一行（authRequired.<METHOD>("<path>", ...）。
func routeLine2586(src, method, path string) string {
	needle := "authRequired." + method + "(\"" + path + "\""
	for _, line := range strings.Split(src, "\n") {
		if strings.Contains(line, needle) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

func TestInstrumentPermRoutes2586(t *testing.T) {
	src := readMainSource2586(t)

	exportGuarded := 0
	importFamilyGuarded := 0
	unguarded := make([]string, 0)

	for key, perm := range instrumentIOPaths2586 {
		method, path, ok := strings.Cut(key, " ")
		if !ok {
			t.Fatalf("bad fixture key %q", key)
		}
		line := routeLine2586(src, method, path)
		if line == "" {
			t.Fatalf("route not found in main.go: authRequired.%s(%q) — 路由被删除或改写需同步本断言", method, path)
		}
		guarded := strings.Contains(line, `middleware.RequireCusPerm("`+perm+`")`)
		if guarded {
			if perm == "instrument:read" {
				exportGuarded++
			} else {
				importFamilyGuarded++
			}
		} else {
			unguarded = append(unguarded, method+" "+path)
		}
	}

	// 导入族必须恰好 6 个（D4 边界：导入/模板/批量导入三族 + 预览 + 媒体）。
	const importFamilyTotal = 6
	if importFamilyGuarded != importFamilyTotal {
		t.Errorf("ROUTE_IMPORT_FAMILY_GUARDED=%d（期望 %d）", importFamilyGuarded, importFamilyTotal)
	}
	if exportGuarded != 1 {
		t.Errorf("ROUTE_EXPORT_GUARDED=%d（期望 1）", exportGuarded)
	}
	if len(unguarded) != 0 {
		t.Errorf("ROUTE_INSTRUMENT_IO_UNGUARDED=%d 未挂守卫: %s", len(unguarded), strings.Join(unguarded, "; "))
	}

	// 反向守卫：不得把导出误挂成 instrument:create，或导入误挂成 instrument:read。
	exportLine := routeLine2586(src, "GET", "/instruments/export")
	if strings.Contains(exportLine, `RequireCusPerm("instrument:create")`) {
		t.Errorf("导出端点不得使用 instrument:create 守卫: %s", exportLine)
	}
	if strings.Contains(exportLine, `RequireCusPerm("instrument:update")`) {
		t.Errorf("导出端点不得使用 instrument:update 守卫: %s", exportLine)
	}
	importLine := routeLine2586(src, "POST", "/instruments/import")
	if regexp.MustCompile(`RequireCusPerm\("instrument:read"\)`).MatchString(importLine) {
		t.Errorf("导入端点不得使用 instrument:read 守卫: %s", importLine)
	}

	t.Logf("ROUTE_EXPORT_GUARDED=%d", exportGuarded)
	t.Logf("ROUTE_IMPORT_FAMILY_GUARDED=%d", importFamilyGuarded)
	t.Logf("ROUTE_INSTRUMENT_IO_UNGUARDED=%d", len(unguarded))
}
