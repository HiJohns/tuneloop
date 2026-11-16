package handlers

// #2379：technician_profile.List 的商户解析必须走 merchantConfigDB（IdentityCtx）。
// R1：merchants{org_id=O, tenant_id=N} + JWT tid=O → 正确解析到商户（修复后）。
// R2：同一 fixture 用带自动 tenant scope 的句柄 → 解析失败（缺陷可复现）。

import (
	"context"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTechScope2379(t *testing.T) (db *gorm.DB, orgID string, ctx context.Context) {
	t.Helper()
	db = testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)
	orgID = uuid.New().String()
	namespaceID := uuid.New().String()
	testfixtures.SeedTenantFK(t, db, orgID, namespaceID)
	require.NoError(t, db.Create(&models.Merchant{
		ID: uuid.New().String(), TenantID: namespaceID, OrgID: orgID,
		AdminUID: uuid.New().String(), Name: "M2379-" + orgID[:8], Status: "active",
	}).Error)
	ctx = testutil.TestActor{TenantID: orgID, OrgID: orgID, UserID: uuid.New().String(), Role: "ADMIN"}.InjectContext(context.Background())
	return db, orgID, ctx
}

// R1：修复后（merchantConfigDB）解析到正确商户。
func TestTechnicianProfileMerchantScopeFixed_2379(t *testing.T) {
	_, orgID, ctx := setupTechScope2379(t)
	m, ok := resolveOperatorMerchant2247(merchantConfigDB(ctx), orgID, orgID)
	assert.True(t, ok, "merchant must resolve via IdentityCtx handle")
	assert.Equal(t, orgID, m.OrgID)
}

// R2：修复前用法（带自动 tenant scope 的句柄）→ 解析失败（红）。
func TestTechnicianProfileMerchantScopeRed_2379(t *testing.T) {
	db, orgID, ctx := setupTechScope2379(t)
	_, ok := resolveOperatorMerchant2247(db.WithContext(ctx), orgID, orgID)
	assert.False(t, ok, "auto tenant scope reproduces the defect (tenant_id filter is always false)")
}
