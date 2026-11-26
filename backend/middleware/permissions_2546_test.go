package middleware

// #2546 R15（门控单元）：RequireSysPermOrRole 只给「命中 sys_perm 位」或「命中业务角色」
// 的调用方放行；其他业务角色仍 40303；sys_perm==0 的既有直通语义保持不变。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func withSysPermActorContext(t *testing.T, role, tid, oid string, sysPerm int64) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := c.Request.Context()
	ctx = context.WithValue(ctx, ContextKeyRole, role)
	ctx = context.WithValue(ctx, ContextKeyTenantID, tid)
	ctx = context.WithValue(ctx, ContextKeyOrgID, oid)
	if sysPerm != 0 {
		ctx = context.WithValue(ctx, ContextKeySysPerm, sysPerm)
	}
	c.Request = c.Request.WithContext(ctx)
	return c, w
}

// merchantAdminSysPerm 是 IAM merchant_admin 模板位图（bit 10-29），不含 tenant 位。
const merchantAdminSysPerm = int64(1073740800)

func TestRequireSysPermOrRole_2546(t *testing.T) {
	t.Run("sys_perm 命中即放行（既有平台管理员路径不变）", func(t *testing.T) {
		c, w := withSysPermActorContext(t, "NAMESPACE_ADMIN", "ten", "org", 1<<SysPermTenantUpdate)
		RequireSysPermOrRole(SysPermTenantUpdate, BusinessRoleMerchantAdmin)(c)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.False(t, c.IsAborted(), "MA_SYS_BIT_PASS=1")
		t.Logf("MA_SYS_BIT_PASS=1 status=%d", w.Code)
	})

	t.Run("商户管理员无 tenant 位但命中 businessRole → 放行", func(t *testing.T) {
		c, w := withSysPermActorContext(t, "OWNER", "org-1", "org-1", merchantAdminSysPerm)
		RequireSysPermOrRole(SysPermTenantUpdate, BusinessRoleMerchantAdmin)(c)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.False(t, c.IsAborted(), "MA_ROLE_PASS=1")
		t.Logf("MA_ROLE_PASS=1 status=%d", w.Code)
	})

	t.Run("其他业务角色仍 403/40303", func(t *testing.T) {
		c, w := withSysPermActorContext(t, "ADMIN", "ten", "site-org", merchantAdminSysPerm)
		RequireSysPermOrRole(SysPermTenantUpdate, BusinessRoleMerchantAdmin)(c)
		assert.Equal(t, http.StatusForbidden, w.Code, "MA_ROLE_DENY_STATUS=403")
		assert.Contains(t, w.Body.String(), "40303", "MA_ROLE_DENY_CODE=40303")
		t.Logf("MA_ROLE_DENY_STATUS=403 MA_ROLE_DENY_CODE=40303")
	})

	t.Run("sys_perm==0 保持既有直通语义", func(t *testing.T) {
		c, w := withSysPermActorContext(t, "ADMIN", "ten", "site-org", 0)
		RequireSysPermOrRole(SysPermTenantUpdate, BusinessRoleMerchantAdmin)(c)
		assert.Equal(t, http.StatusOK, w.Code, "MA_ZERO_PASSTHROUGH=1")
		assert.False(t, c.IsAborted(), "MA_ZERO_PASSTHROUGH=1")
		t.Logf("MA_ZERO_PASSTHROUGH=1 status=%d", w.Code)
	})
}
