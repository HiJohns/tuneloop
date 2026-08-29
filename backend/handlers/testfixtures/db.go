package testfixtures

import (
	"testing"

	"gorm.io/gorm"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/stretchr/testify/require"
)

// All business tables used by integration tests. Kept in one place so
// every flow test gets the same isolated schema (Phase 1a, #1583).
// Order matters: referenced tables (level/category/site) must be created
// before referencing tables (instrument) so GORM can build FKs.
var allTables = []interface{}{
	&models.InstrumentLevel{},
	&models.Category{},
	&models.Site{},
	&models.SiteMember{},
	&models.Instrument{},
	&models.Order{},
	&models.LeaseSession{},
	&models.Notification{},
	&models.UserAddress{},
	&models.Merchant{},
	&models.ElectronicContract{},
	&models.OrderLog{},
	&models.OrderStatusHistory{},
	&models.AuditLog{},

	&models.Settlement{},
	&models.OrderRefundRecord{},
	&models.OrderPaymentRecord{},
	&models.PointsTransaction{},
	&models.User{},
	&models.FaceCaptureBatch{}, // #1789 T1: 核身批次表
	&models.MediaAsset{},       // #1790 T2: 媒体资产注册表（face_capture GC 豁免）
	&models.DamageReport{},
	&models.MembershipGiftRatio{},
	&models.GiftPolicy{},
	&models.PricingTemplate{},
	&models.MerchantPricingConfig{},
	&models.PointsPolicy{},
	&models.MerchantSettlementConfig{},
	&models.SystemSetting{},
	&models.PromoPlan{},
	&models.Referral{},
	&models.MembershipLevel{},
	&models.DiscountPolicy{},
	&models.DiscountCode{},
	&models.DiscountCodeUsage{},
	&models.ConfirmationSession{},
}

// SetupTestDB connects to the test database, drops and recreates all
// business tables, and registers the global DB. Returns the *gorm.DB.
// Skips the test when the test DB is unavailable.
func SetupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
		return nil
	}
	database.SetDB(db)

	// #1721: DropTable 按序删除会被外键阻止（残留导致 CreateTable 报 already
	// exists）——用 CASCADE 原生清理全部业务表。
	var tables []string
	db.Raw(`SELECT tablename FROM pg_tables WHERE schemaname = 'public'`).Scan(&tables)
	for _, tb := range tables {
		if err := db.Exec(`DROP TABLE IF EXISTS "` + tb + `" CASCADE`).Error; err != nil {
			t.Logf("warning: drop table %s: %v", tb, err)
		}
	}
	for _, table := range allTables {
		require.NoError(t, db.Migrator().CreateTable(table))
	}
	// iam_sub has -:migration tag and is excluded from CreateTable.
	if !db.Migrator().HasColumn(&models.User{}, "iam_sub") {
		require.NoError(t, db.Exec(`ALTER TABLE users ADD COLUMN iam_sub varchar(255)`).Error)
	}
	return db
}

// NewTenantIDs returns a unique tenant/org/user triple per call so
// parallel tests never collide on the shared test DB. seed must be a
// 12-char hex string (e.g. "a1b2c3d4e5f6"); the trailing digit selects
// tenant/org/user variants.
func NewTenantIDs(seed string) (tenantID, orgID, userID string) {
	base := "00000000-0000-4000-8000-" + seed
	return base[:35] + "1", base[:35] + "2", base[:35] + "3"
}
