package handlers

// #2540 D4（Owner 2026-11-26 裁定）：商户管理员审批确认必须绑定 IAM 关系角色 OWNER。
//
// 旧写法字面量 "admin" 把新建商户的管理员降为 ADMIN，而 beaconiam 的
// `POST /organizations`（建网点）要求 PermissionOwner → 商户创建者无法再建网点。
// 唯一权威映射是 `toIAMRole("merchant_admin")`（= OWNER），不得散落字面量。
//
// 断言面向 fake IAM 捕获的真实 bind payload + customer-permissions 位图。

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// R6：商户管理员确认 → bind role = OWNER，权限位取 merchant_admin 模板。
func TestMerchantAdminConfirmBindsOwner2540(t *testing.T) {
	db := database.GetDB()
	tenantID := uuid.NewString()
	orgID := uuid.NewString()
	testfixtures.SeedTenantFK(t, db, tenantID)

	userID := uuid.NewString()
	user := models.User{
		ID:       userID,
		IAMSub:   testfixtures.UniqueIAMSub("m2540"),
		TenantID: tenantID,
		OrgID:    tenantID,
		Name:     "商户管理员2540",
		Username: "m2540-" + uuid.NewString()[:8],
		Phone:    "137" + uuid.NewString()[:8],
		Email:    "m2540-" + uuid.NewString()[:8] + "@example.com",
		Status:   "active",
		Role:     "merchant_admin",
	}
	require.NoError(t, db.Create(&user).Error)
	t.Cleanup(func() { db.Where("id = ?", userID).Delete(&models.User{}) })

	merchantID := uuid.NewString()
	merchant := models.Merchant{
		ID:       merchantID,
		TenantID: tenantID,
		OrgID:    orgID,
		Name:     "商户2540",
		Code:     testfixtures.UniqueCode("m2540"),
		AdminUID: userID,
		Status:   "active",
	}
	require.NoError(t, db.Create(&merchant).Error)
	t.Cleanup(func() { db.Where("id = ?", merchantID).Delete(&models.Merchant{}) })

	installCusPermRegistry2540(t)
	f := installFakeIAM2540(t)

	session := &models.ConfirmationSession{
		ID:            uuid.NewString(),
		TenantID:      tenantID,
		OrgID:         orgID,
		UserID:        userID,
		ConfirmType:   "email",
		ConfirmTarget: user.Email,
		MerchantID:    merchantID,
		ActionType:    "merchant_admin",
		Status:        "confirmed",
	}

	handler := NewConfirmationSessionHandler()
	require.NoError(t, handler.executeAction(session), "商户管理员确认应成功")

	roles := f.bindRoles()
	require.Len(t, roles, 1, "商户管理员确认必须产生一次 bind: %v", roles)
	assert.Equal(t, services.IAMRoleOwner, roles[0], "商户管理员必须是商户组织的 OWNER（否则无法建网点）")
	fmt.Printf("BIND_ROLE=%s\n", roles[0])

	perms := f.customerPerms()
	require.Len(t, perms, 1, "商户管理员确认必须写一次 customer-permissions")
	assert.Equal(t, expectedCusPerm("merchant_admin"), perms[0], "权限位必须取 merchant_admin 模板")
	fmt.Printf("PERM_TEMPLATE=merchant_admin\n")
	fmt.Printf("CUS_PERM_MATCH=1\n")

	// 负例：不得再出现字面量 "admin"/"ADMIN" 作为商户管理员的关系角色。
	lowered := []string{}
	for _, role := range roles {
		if role != services.IAMRoleOwner {
			lowered = append(lowered, role)
		}
	}
	assert.Empty(t, lowered, "商户管理员关系角色只能是 OWNER")
	fmt.Printf("NON_OWNER_ROLE_COUNT=%d\n", len(lowered))
}
