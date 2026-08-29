package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"tuneloop-backend/middleware"
)

// #1795 T6 tests: 平台员工角色识别（PLATFORM_ROOT_ORG_ID 精确匹配）。

func ctxWithTenantOrg(t *testing.T, tenantID, orgID, role string) context.Context {
	t.Helper()
	ctx := context.Background()
	ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
	ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID)
	ctx = context.WithValue(ctx, middleware.ContextKeyRole, role)
	return ctx
}

// TestGetBusinessRole_PlatformStaff (#1795 T6): oid 精确匹配 PLATFORM_ROOT_ORG_ID
// → platform_staff（非 merchant_admin）。
func TestGetBusinessRole_PlatformStaff(t *testing.T) {
	// 模拟 PLATFORM_ROOT_ORG_ID 配置。
	rootOrgID := "11111111-1111-4111-8111-111111111111"
	middleware.SetPlatformRootOrgIDForTesting(rootOrgID)
	defer middleware.SetPlatformRootOrgIDForTesting("")

	// 平台员工：oid = 根组织，tid != oid（商户根组织形态相同不被误判）。
	ctx := ctxWithTenantOrg(t, "22222222-2222-4222-8222-222222222222", rootOrgID, "STAFF")
	require.Equal(t, middleware.BusinessRolePlatformStaff, middleware.GetBusinessRole(ctx), "oid matches root org → platform_staff")
}

// TestGetBusinessRole_MerchantAdmin_NotPlatformStaff (#1795 T6): 商户根组织
// （tid == oid）即使 oid 形似根组织也不误判——oid 不等于 PLATFORM_ROOT_ORG_ID。
func TestGetBusinessRole_MerchantAdmin_NotPlatformStaff(t *testing.T) {
	middleware.SetPlatformRootOrgIDForTesting("11111111-1111-4111-8111-111111111111")
	defer middleware.SetPlatformRootOrgIDForTesting("")

	// 商户管理员：tid == oid，但 oid 不是根组织 → merchant_admin。
	merchantOrg := "33333333-3333-4333-8333-333333333333"
	ctx := ctxWithTenantOrg(t, merchantOrg, merchantOrg, "ADMIN")
	require.Equal(t, middleware.BusinessRoleMerchantAdmin, middleware.GetBusinessRole(ctx), "tid==oid and not root org → merchant_admin")
}

// TestGetBusinessRole_PlatformStaff_NotConfigured (#1795 T6): 未配置
// PLATFORM_ROOT_ORG_ID → 平台员工分支不触发（默认行为不变）。
func TestGetBusinessRole_PlatformStaff_NotConfigured(t *testing.T) {
	middleware.SetPlatformRootOrgIDForTesting("")
	// oid 任意值，tid==oid → merchant_admin（默认逻辑）。
	orgID := "44444444-4444-4444-8444-444444444444"
	ctx := ctxWithTenantOrg(t, orgID, orgID, "ADMIN")
	require.Equal(t, middleware.BusinessRoleMerchantAdmin, middleware.GetBusinessRole(ctx))
}

// TestGetBusinessRole_PlatformStaff_VisibleOrgs (#1795 T6 R2 C2): 平台员工
// GetVisibleOrgIDs 返回 nil（全用户可见，无 org 过滤）。
func TestGetBusinessRole_PlatformStaff_VisibleOrgs(t *testing.T) {
	middleware.SetPlatformRootOrgIDForTesting("11111111-1111-4111-8111-111111111111")
	defer middleware.SetPlatformRootOrgIDForTesting("")

	ctx := ctxWithTenantOrg(t, "22222222-2222-4222-8222-222222222222", "11111111-1111-4111-8111-111111111111", "STAFF")
	ids, err := middleware.GetVisibleOrgIDs(ctx)
	require.NoError(t, err)
	require.Nil(t, ids, "platform staff sees all users (no org filter)")
}
