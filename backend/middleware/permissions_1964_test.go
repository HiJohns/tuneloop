package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// stubCusPermRegistry 让 cus_perm 检查在单测中生效（mock registry 返回 -1 直通）。
type stubCusPermRegistry struct{ bit int }

func (s stubCusPermRegistry) GetSysPermBit(string) (int, bool)  { return 0, false }
func (s stubCusPermRegistry) GetCusPermBit(string) int          { return s.bit }
func (s stubCusPermRegistry) GetCusPermMapping() map[string]int { return map[string]int{} }

// #1964: RequireCusPerm/RequireAnyCusPerm 必须放行系统管理员/平台员工，
// 否则其 cus_perm 位图不含商户/员工权限位 → 误拒 40305（preweb 仪表盘/库存 500）。
func withRoleContext(t *testing.T, role, tid, oid string, cusPerm int64) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/test", nil)
	ctx := c.Request.Context()
	ctx = context.WithValue(ctx, ContextKeyRole, role)
	ctx = context.WithValue(ctx, ContextKeyTenantID, tid)
	ctx = context.WithValue(ctx, ContextKeyOrgID, oid)
	ctx = context.WithValue(ctx, ContextKeyCusPerm, cusPerm)
	c.Request = c.Request.WithContext(ctx)
	return c, w
}

func TestRequireCusPerm_1964_SystemAdminAndPlatformStaffBypass(t *testing.T) {
	prev := PermissionRegistry
	defer func() { PermissionRegistry = prev }()
	PermissionRegistry = stubCusPermRegistry{bit: 3}
	const unsetCusPerm = int64(1 << 0) // 不含 bit 3

	t.Run("system_admin bypasses cus_perm gate", func(t *testing.T) {
		c, w := withRoleContext(t, "NAMESPACE_ADMIN", "t1", "o1", unsetCusPerm)
		RequireCusPerm("instrument:read")(c)
		if w.Code != http.StatusOK {
			t.Fatalf("system_admin must pass, got %d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("platform_staff bypasses cus_perm gate", func(t *testing.T) {
		SetPlatformRootOrgIDForTesting("platform-root")
		defer SetPlatformRootOrgIDForTesting("")
		c, w := withRoleContext(t, "STAFF", "t1", "platform-root", unsetCusPerm)
		RequireCusPerm("instrument:read")(c)
		if w.Code != http.StatusOK {
			t.Fatalf("platform_staff must pass, got %d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("site_member without bit rejected 40305", func(t *testing.T) {
		c, w := withRoleContext(t, "STAFF", "t1", "o2", unsetCusPerm)
		RequireCusPerm("instrument:read")(c)
		if w.Code != http.StatusForbidden {
			t.Fatalf("site_member without bit must be rejected, got %d", w.Code)
		}
	})

	t.Run("site_member with bit passes", func(t *testing.T) {
		c, w := withRoleContext(t, "STAFF", "t1", "o2", int64(1<<3))
		RequireCusPerm("instrument:read")(c)
		if w.Code != http.StatusOK {
			t.Fatalf("site_member with bit must pass, got %d", w.Code)
		}
	})

	t.Run("RequireAnyCusPerm also bypasses for system_admin", func(t *testing.T) {
		c, w := withRoleContext(t, "NAMESPACE_ADMIN", "t1", "o1", unsetCusPerm)
		RequireAnyCusPerm("instrument:read", "instrument:write")(c)
		if w.Code != http.StatusOK {
			t.Fatalf("system_admin must pass RequireAnyCusPerm, got %d", w.Code)
		}
	})

	t.Run("RequireAnyCusPerm rejects site_member without any bit", func(t *testing.T) {
		PermissionRegistry = stubCusPermRegistry{bit: 3}
		c, w := withRoleContext(t, "STAFF", "t1", "o2", unsetCusPerm)
		RequireAnyCusPerm("instrument:read", "instrument:write")(c)
		if w.Code != http.StatusForbidden {
			t.Fatalf("site_member without any bit must be rejected, got %d", w.Code)
		}
	})
}
