package handlers

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
)

// #1945 审计返工：迁移 20260918003（id 基裂变预设）up/down 幂等 + 预设覆盖。
// 场景复现原缺陷：级别名称非手册名（管理员改名）时 1/2/3 级 referral_ratio 为 0。
func TestGiftPolicyReferralPresetMigration(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	// 非手册名称（复现 name 匹配未命中）+ 三条 referral_ratio=0 的级别行。
	require.NoError(t, db.Create(&models.MembershipLevel{ID: 1, Name: "乐手会员"}).Error)
	require.NoError(t, db.Create(&models.MembershipLevel{ID: 2, Name: "首席乐手"}).Error)
	require.NoError(t, db.Create(&models.MembershipLevel{ID: 3, Name: "演奏家会员"}).Error)
	require.NoError(t, db.Create(&models.GiftPolicy{LevelID: 0, PayRatio: 0.3, ReferralRatio: 0.02, ReferralRegPoints: 10, IsActive: true}).Error)
	for _, id := range []int{1, 2, 3} {
		require.NoError(t, db.Create(&models.GiftPolicy{LevelID: id, ReferralRatio: 0, ReferralRegPoints: 10, IsActive: true}).Error)
	}

	execSQLFile := func(path string) {
		b, err := os.ReadFile(path)
		require.NoError(t, err)
		for _, stmt := range strings.Split(string(b), ";") {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			require.NoError(t, db.Exec(stmt).Error)
		}
	}

	upPath := "../database/migrations/20260918003_gift_policies_referral_presets.up.sql"
	execSQLFile(upPath)
	execSQLFile(upPath) // 幂等

	var rows []models.GiftPolicy
	require.NoError(t, db.Where("level_id IN ?", []int{1, 2, 3}).Order("level_id ASC").Find(&rows).Error)
	require.Len(t, rows, 3)
	require.Equal(t, 0.02, rows[0].ReferralRatio, "level 1 → 乐手 2%")
	require.Equal(t, 0.05, rows[1].ReferralRatio, "level 2 → 首席 5%")
	require.Equal(t, 0.08, rows[2].ReferralRatio, "level 3 → 演奏家 8%")

	// level-0 兜底不受影响。
	var def models.GiftPolicy
	require.NoError(t, db.Where("level_id = ?", 0).First(&def).Error)
	require.Equal(t, 0.02, def.ReferralRatio, "level-0 兜底保持 0.02")

	// down：恢复修复前状态。
	execSQLFile("../database/migrations/20260918003_gift_policies_referral_presets.down.sql")
	var after models.GiftPolicy
	require.NoError(t, db.Where("level_id = ?", 1).First(&after).Error)
	require.Equal(t, 0.0, after.ReferralRatio, "down 回滚至 0")
}
