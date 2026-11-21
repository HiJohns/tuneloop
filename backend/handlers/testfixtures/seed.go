package testfixtures

// #2344: 严格 schema（#2342 B 顺序：migrations 整链重放 → AutoMigrate 仅缺表）
// 会启用生产形状的外键；而旧「宽松 AutoMigrate」模板不建这些 FK，导致既有
// fixture 直接用「本地 users 的 tenant_id / 网点 tenant」写 site_members 时
// 触发 site_members_tenant_id_fkey。
//
// 统一出口：需要引用 tenant 的用例先调用 SeedTenantFK 建 tenants 行，
// 不要在逐用例里复制 INSERT（D2：FK 种子单一 owner）。

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"tuneloop-backend/models"
)

// SeedTenantFK 确保给定 tenant id 在 tenants 表存在（幂等）。
// 空 id 跳过：部分夹具用空串表示「无租户」，不应伪造行。
func SeedTenantFK(t *testing.T, db *gorm.DB, tenantIDs ...string) {
	t.Helper()
	for _, id := range tenantIDs {
		if id == "" {
			continue
		}
		row := models.Tenant{ID: id, Name: "test-tenant", Status: "active"}
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			t.Fatalf("#2344 seed tenants(%s): %v", id, err)
		}
	}
}

// SeedSiteFK 确保给定网点存在（严格 schema 下 site_members.site_id / orders.site_id 有 FK）。
// tenantID 可为空（sites.tenant_id 是 not null，调用方应给出实际租户）。
func SeedSiteFK(t *testing.T, db *gorm.DB, siteID, tenantID string) {
	t.Helper()
	if siteID == "" {
		return
	}
	SeedTenantFK(t, db, tenantID)
	row := models.Site{ID: siteID, TenantID: tenantID, OrgID: tenantID, Name: "test-site", Type: "normal"}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		t.Fatalf("#2344 seed sites(%s): %v", siteID, err)
	}
}

// SeedUserFK 确保给定 users 行存在（严格 schema 下 orders.user_id / site_members.user_id 有 FK）。
func SeedUserFK(t *testing.T, db *gorm.DB, userID, tenantID, orgID string) {
	t.Helper()
	if userID == "" {
		return
	}
	SeedTenantFK(t, db, tenantID)
	row := models.User{ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID, Username: "u-" + userID[:8], Status: "active"}
	q := db.Clauses(clause.OnConflict{DoNothing: true})
	if orgID == "" {
		q = q.Omit("org_id")
	}
	if err := q.Create(&row).Error; err != nil {
		t.Fatalf("#2344 seed users(%s): %v", userID, err)
	}
}

// SeedMembershipLevelFK 确保 users.membership_level_id 指向的行存在（严格 schema 有 FK）。
// #2357：旧夹具常用 `MembershipLevelID: intPtr(1)`，而 membership_levels 无该行。
func SeedMembershipLevelFK(t *testing.T, db *gorm.DB, levelID int) {
	t.Helper()
	if levelID <= 0 {
		return
	}
	row := models.MembershipLevel{ID: levelID, Name: "test-level", MinAmount: 0}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		t.Fatalf("#2357 seed membership_levels(%d): %v", levelID, err)
	}
}

// UniqueIAMSub 生成唯一的 iam_sub（严格 schema 下 users.iam_sub 有唯一索引）。
// #2357：多处夹具复用同一个 iam_sub（如 "refund_test_user"）触发 users_iam_sub_key。
func UniqueIAMSub(seed string) string {
	return "iam-" + seed + "-" + uuid.New().String()
}

// UniqueCode 生成带随机后缀的业务 code（覆盖 pricing_templates / merchants 等唯一索引）。
func UniqueCode(seed string) string {
	return seed + "-" + uuid.New().String()[:8]
}

// SeedInstrumentFK 确保给定乐器行存在（严格 schema 下 orders.instrument_id /
// instruments.current_site_id 等有 FK）。#2443 Rev.5：FK 种子单一出口。
func SeedInstrumentFK(t *testing.T, db *gorm.DB, instrumentID, tenantID, orgID string) {
	t.Helper()
	if instrumentID == "" {
		return
	}
	SeedTenantFK(t, db, tenantID)
	row := models.Instrument{
		ID:          instrumentID,
		TenantID:    tenantID,
		SN:          "SN-" + uuid.New().String()[:8],
		StockStatus: "available",
	}
	if orgID != "" {
		row.OrgID = &orgID
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		t.Fatalf("#2443 seed instruments(%s): %v", instrumentID, err)
	}
}

// SeedOrderParents 确保订单引用的 tenant / user / instrument 行存在
// （严格 schema 下 orders.user_id / orders.instrument_id 有 FK）。
// #2443 Rev.5：订单 fixture 的单一 seed 出口。
func SeedOrderParents(t *testing.T, db *gorm.DB, o *models.Order) {
	t.Helper()
	if o == nil {
		return
	}
	SeedTenantFK(t, db, o.TenantID)
	SeedUserFK(t, db, o.UserID, o.TenantID, o.OrgID)
	SeedInstrumentFK(t, db, o.InstrumentID, o.TenantID, o.OrgID)
}
