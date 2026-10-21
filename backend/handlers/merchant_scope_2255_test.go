package handlers

// #2255（P3 批次1）：商户归属解析优先级守卫。
// 规则（全 Epic 共用）：org 优先 → 唯一 tenant 映射 → 共享 tenant 不解析（禁止任取）。

import (
	"context"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveMerchantOrg_Precedence_2255(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	shared := uuid.NewString()
	orgA, orgB, orgC := uuid.NewString(), uuid.NewString(), uuid.NewString()
	uniqueTenant := uuid.NewString()

	mk := func(tenant, org, name string) {
		require.NoError(t, db.Create(&models.Merchant{
			ID: uuid.NewString(), TenantID: tenant, OrgID: org, Name: name,
			Address: "x", ContactName: "c", ContactPhone: "1", Status: "active", AdminUID: uuid.NewString(),
		}).Error)
	}
	mk(shared, orgA, "A2255")
	mk(shared, orgB, "B2255")
	mk(uniqueTenant, orgC, "C2255")

	identity := database.GetDB().WithContext(database.IdentityCtx(context.Background()))

	// 1) org 命中优先（即使 tenant 是共享值）
	m, ok := resolveOperatorMerchant2247(identity, orgA, shared)
	require.True(t, ok)
	assert.Equal(t, orgA, m.OrgID)

	// 2) 唯一 tenant 映射到对应商户
	m, ok = resolveOperatorMerchant2247(identity, "", uniqueTenant)
	require.True(t, ok)
	assert.Equal(t, orgC, m.OrgID)

	// 3) 共享 tenant 歧义 → 不解析（禁止无序 First 任取商户）
	_, ok = resolveOperatorMerchant2247(identity, "", shared)
	assert.False(t, ok, "共享 tenant 必须不解析")

	// 4) 未知身份 → 不解析
	_, ok = resolveOperatorMerchant2247(identity, "", uuid.NewString())
	assert.False(t, ok)

	// 5) merchant-scoped config 解析（#2251 路径）与 #2247 口径一致
	ctxA := testutil.TestActor{TenantID: shared, OrgID: orgA, UserID: uuid.NewString(), Role: "OWNER"}.
		InjectContext(context.Background())
	m, ok = resolveOperatorMerchantConfig(ctxA)
	require.True(t, ok)
	assert.Equal(t, orgA, m.OrgID, "org 上下文应解析到本商户")

	ctxShared := testutil.TestActor{TenantID: shared, OrgID: "", UserID: uuid.NewString(), Role: "ADMIN"}.
		InjectContext(context.Background())
	_, ok = resolveOperatorMerchantConfig(ctxShared)
	assert.False(t, ok, "仅共享 tenant（无 org）时必须拒绝，不得任取商户")
}
