package main

// #2587：乐器**只读子端点**（7 个）与 legacy 照片上传（1 个）的 cus_perm 守卫必须挂在路由层。
//
// 与 #2586 同形态：handler 级测试无法发现「中间件没挂到生产入口」。
// 绿样（本测试）：7 个读端点 = instrument:read；photos/upload = instrument:media_upload；
// 乐器只读/上传端点无守卫计数 = 0。
// 红样（由 scripts/verify-2587.sh r3 用 git show <fix>^:backend/main.go 采集）。

import (
	"os"
	"strings"
	"testing"
)

// instrumentReadRoutes2587 是本单口径的端点 → 期望权限位。
var instrumentReadRoutes2587 = map[string]string{
	"GET /instruments/filter-options":     "instrument:read",
	"GET /instruments/check":              "instrument:read",
	"GET /instruments/:id/photos/latest":  "instrument:read",
	"GET /instruments/:id/media":          "instrument:read",
	"GET /instruments/:id/activity-log":   "instrument:read",
	"GET /instruments/:id/pricing":        "instrument:read",
	"GET /instruments/:id/pricing-v2":     "instrument:read",
	"POST /instruments/:id/photos/upload": "instrument:media_upload",
}

func readMainSource2587(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	return string(raw)
}

func routeLine2587(src, method, path string) string {
	needle := "authRequired." + method + "(\"" + path + "\""
	for _, line := range strings.Split(src, "\n") {
		if strings.Contains(line, needle) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

func TestInstrumentReadRoutes2587(t *testing.T) {
	src := readMainSource2587(t)

	readGuarded, uploadGuarded := 0, 0
	unguarded := make([]string, 0)

	for key, perm := range instrumentReadRoutes2587 {
		method, path, ok := strings.Cut(key, " ")
		if !ok {
			t.Fatalf("bad fixture key %q", key)
		}
		line := routeLine2587(src, method, path)
		if line == "" {
			t.Fatalf("route not found: authRequired.%s(%q)", method, path)
		}
		if strings.Contains(line, `middleware.RequireCusPerm("`+perm+`")`) {
			if perm == "instrument:read" {
				readGuarded++
			} else {
				uploadGuarded++
			}
		} else {
			unguarded = append(unguarded, method+" "+path)
		}
	}

	// D1 边界：7 读 + 1 上传（第 8 行 POST /instruments/:id/media 由既有守卫，#2586 不动）。
	const wantRead, wantUpload = 7, 1
	if readGuarded != wantRead {
		t.Errorf("ROUTE_READ_GUARDED=%d（期望 %d）", readGuarded, wantRead)
	}
	if uploadGuarded != wantUpload {
		t.Errorf("ROUTE_UPLOAD_GUARDED=%d（期望 %d）", uploadGuarded, wantUpload)
	}
	if len(unguarded) != 0 {
		t.Errorf("ROUTE_INSTRUMENT_READ_UNGUARDED=%d 未挂守卫: %s", len(unguarded), strings.Join(unguarded, "; "))
	}

	// 反向守卫：读端点不得误挂写权限位；photos/upload 不得挂 read（写端点）。
	misgranted := make([]string, 0)
	for key, perm := range instrumentReadRoutes2587 {
		method, path, _ := strings.Cut(key, " ")
		line := routeLine2587(src, method, path)
		for _, wrong := range []string{"instrument:create", "instrument:update", "instrument:delete"} {
			if strings.Contains(line, `RequireCusPerm("`+wrong+`")`) {
				misgranted = append(misgranted, method+" "+path+" → "+wrong)
			}
		}
		if perm == "instrument:media_upload" &&
			strings.Contains(line, `RequireCusPerm("instrument:read")`) {
			misgranted = append(misgranted, method+" "+path+" → instrument:read")
		}
	}
	if len(misgranted) != 0 {
		t.Errorf("误挂权限位: %s", strings.Join(misgranted, "; "))
	}

	t.Logf("ROUTE_READ_GUARDED=%d", readGuarded)
	t.Logf("ROUTE_UPLOAD_GUARDED=%d", uploadGuarded)
	t.Logf("ROUTE_INSTRUMENT_READ_UNGUARDED=%d", len(unguarded))
}
