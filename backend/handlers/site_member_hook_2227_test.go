package handlers

import (
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2227: SiteMember.BeforeCreate 兜底——未显式设置 Roles 时从主 Role 派生，
// 避免 GORM 将 nil 切片序列化为 SQL NULL（roles 列为 NOT NULL）。
// 测试库由模型 tag 生成（含 not null 约束），故 NULL 写入会直接报错。
func TestSiteMemberBeforeCreate_FillsRoles2227(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	tenantID, orgID, _ := testfixtures.NewTenantIDs("2227abcdef01")
	siteID := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{ID: siteID, TenantID: tenantID, OrgID: orgID, Name: "S2227"}).Error)

	// A) 只给主 Role → Roles 自动派生为 [Role]
	userA := uuid.New().String()
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.New().String(), TenantID: tenantID, SiteID: siteID, UserID: userA, Role: "site_member",
	}).Error)
	var a models.SiteMember
	require.NoError(t, db.First(&a, "user_id = ?", userA).Error)
	assert.Equal(t, []string{"site_member"}, a.Roles, "BeforeCreate 应从主 Role 派生 Roles")

	// B) Role 也为空 → Roles 落为 []（非 nil），不违反 NOT NULL
	userB := uuid.New().String()
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.New().String(), TenantID: tenantID, SiteID: siteID, UserID: userB,
	}).Error)
	var b models.SiteMember
	require.NoError(t, db.First(&b, "user_id = ?", userB).Error)
	assert.Empty(t, b.Roles, "Role 为空时 Roles 应为空集（非 NULL）")
}
