package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupTestContext(sysPerm, cusPerm int64) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/test", nil)

	ctx := c.Request.Context()
	ctx = context.WithValue(ctx, ContextKeySysPerm, sysPerm)
	ctx = context.WithValue(ctx, ContextKeyCusPerm, cusPerm)
	ctx = context.WithValue(ctx, ContextKeyGid, "test-gid")
	ctx = context.WithValue(ctx, ContextKeyCusPermExt, "")
	c.Request = c.Request.WithContext(ctx)
	return c, w
}

func TestRequireSysPerm_BitSet(t *testing.T) {
	c, w := setupTestContext(255, 0)                      // sys_perm = 255 (all bits 0-7 set)
	middleware := RequireSysPerm(SysPermTenantView)        // bit 5
	middleware(c)
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d. sys_perm=255, bit=5 should pass", w.Code)
	}
}

func TestRequireSysPerm_BitNotSet(t *testing.T) {
	c, w := setupTestContext(0, 0)                        // sys_perm = 0
	middleware := RequireSysPerm(SysPermTenantCreate)      // bit 7
	middleware(c)
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK (backward compat), got %d", w.Code)
	}
}

func TestRequireSysPerm_SpecificBitMissing(t *testing.T) {
	c, w := setupTestContext(1<<SysPermTenantView, 0)     // Only tenant_view set
	middleware := RequireSysPerm(SysPermTenantCreate)      // bit 7, NOT set
	middleware(c)
	if w.Code != http.StatusForbidden {
		t.Errorf("Expected 403, got %d. Only bit 5 set, bit 7 should fail", w.Code)
	}
}

func TestRequireSysPerm_MultipleBitsSet(t *testing.T) {
	c, w := setupTestContext(1<<SysPermTenantView|1<<SysPermTenantCreate, 0)
	middleware := RequireSysPerm(SysPermTenantCreate)
	middleware(c)
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d. bits 5 and 7 set, checking bit 7 should pass", w.Code)
	}
}

func TestRequireCusPerm_MockMode(t *testing.T) {
	c, w := setupTestContext(0, 0)
	middleware := RequireCusPerm("instrument:create")
	middleware(c)
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK (mock pass-through), got %d", w.Code)
	}
}

func TestGetSysPerm_FromContext(t *testing.T) {
	c, _ := setupTestContext(42, 0)
	if got := GetSysPerm(c.Request.Context()); got != 42 {
		t.Errorf("GetSysPerm = %d, want 42", got)
	}
}

func TestGetCusPerm_FromContext(t *testing.T) {
	c, _ := setupTestContext(0, 128)
	if got := GetCusPerm(c.Request.Context()); got != 128 {
		t.Errorf("GetCusPerm = %d, want 128", got)
	}
}

func TestGetGid_FromContext(t *testing.T) {
	c, _ := setupTestContext(0, 0)
	if got := GetGid(c.Request.Context()); got != "test-gid" {
		t.Errorf("GetGid = %q, want %q", got, "test-gid")
	}
}

func TestGetCusPermExt_DefaultEmpty(t *testing.T) {
	c, _ := setupTestContext(0, 0)
	if got := GetCusPermExt(c.Request.Context()); got != "" {
		t.Errorf("GetCusPermExt = %q, want empty string", got)
	}
}

func TestMockPermissionRegistry(t *testing.T) {
	reg := NewMockPermissionRegistry()

	bit, ok := reg.GetSysPermBit("tenant_view")
	if !ok || bit != 5 {
		t.Errorf("GetSysPermBit(tenant_view) = (%d, %v), want (5, true)", bit, ok)
	}

	bit, ok = reg.GetSysPermBit("nonexistent")
	if ok {
		t.Errorf("GetSysPermBit(nonexistent) should return false, got (%d, true)", bit)
	}

	cusBit := reg.GetCusPermBit("instrument:create")
	if cusBit != -1 {
		t.Errorf("GetCusPermBit should return -1 in mock mode, got %d", cusBit)
	}
}

func TestSysPermBitNames_Completeness(t *testing.T) {
	seen := make(map[int]bool)
	for bit, name := range SysPermBitNames {
		if bit < 0 || bit > 24 {
			t.Errorf("SysPerm bit %s has out-of-range bit %d", name, bit)
		}
		if seen[bit] {
			t.Errorf("Duplicate sys_perm bit: %d", bit)
		}
		seen[bit] = true
	}
	if len(seen) != 25 {
		t.Errorf("Expected 25 sys_perm bits (0-24), got %d", len(seen))
	}
}

func TestBackwardCompat_ZeroSysPerm(t *testing.T) {
	c, w := setupTestContext(0, 0)
	for bit := 0; bit <= 24; bit++ {
		middleware := RequireSysPerm(bit)
		middleware(c)
		// Reset writer between tests
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/test", nil)
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, ContextKeySysPerm, int64(0))
		ctx = context.WithValue(ctx, ContextKeyCusPerm, int64(0))
		c.Request = c.Request.WithContext(ctx)
	}
}
