package services

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

// #1947 Sub-D：乐币批次化记账——生命周期/FIFO/退回/过期/提醒/存量迁移。

func setupPointBatchTestDB(t *testing.T) {
	t.Helper()
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
	}
	database.SetDB(db)
	for _, m := range []interface{}{
		&models.User{}, &models.PointBatch{}, &models.PointBatchConsumption{},
		&models.Notification{}, &models.PointsTransaction{},
	} {
		_ = db.Migrator().DropTable(m)
		require.NoError(t, db.Migrator().CreateTable(m))
	}
	db.Exec("ALTER TABLE users ADD COLUMN IF NOT EXISTS iam_sub VARCHAR(255) NOT NULL DEFAULT ''")
}

func mkPointBatchUser(t *testing.T, promo models.Cents) (*gorm.DB, string, string) {
	t.Helper()
	db := database.GetDB()
	tenantID := uuid.New().String()
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: tenantID,
		Username: "pb-" + userID[:8], Status: "active", Name: "批次用户", PromoPoints: promo,
	}).Error)
	return db, userID, tenantID
}

func mkBatch(t *testing.T, userID string, amount models.Cents, exp *time.Time, sat *time.Time) *models.PointBatch {
	t.Helper()
	now := time.Now()
	if sat != nil {
		now = *sat
	}
	b := &models.PointBatch{
		ID: uuid.New().String(), UserID: userID, SourceType: PointBatchSourceSignup,
		AmountCents: amount, RemainingCents: amount, AcquiredAt: now, ExpiresAt: exp,
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, database.GetDB().Create(b).Error)
	return b
}

func TestPointBatch_ConsumeFIFO(t *testing.T) {
	setupPointBatchTestDB(t)
	db, userID, _ := mkPointBatchUser(t, 0)

	now := time.Now()
	soon := now.Add(time.Hour)
	far := now.Add(365 * 24 * time.Hour)
	bSoon := mkBatch(t, userID, 300, &soon, nil)
	bFar := mkBatch(t, userID, 500, &far, nil)
	bNever := mkBatch(t, userID, 400, nil, nil)

	txn := uuid.New().String()
	consumed, err := ConsumePointsFIFO(db, userID, 700, txn)
	require.NoError(t, err)
	require.Equal(t, models.Cents(700), consumed)

	var rSoon, rFar, rNever models.PointBatch
	require.NoError(t, db.Where("id = ?", bSoon.ID).First(&rSoon).Error)
	require.NoError(t, db.Where("id = ?", bFar.ID).First(&rFar).Error)
	require.NoError(t, db.Where("id = ?", bNever.ID).First(&rNever).Error)
	require.Equal(t, models.Cents(0), rSoon.RemainingCents, "先扣临期批次")
	require.Equal(t, models.Cents(100), rFar.RemainingCents, "跨批次扣远期")
	require.Equal(t, models.Cents(400), rNever.RemainingCents, "NULL 到期视为最晚")

	var consCount int64
	db.Model(&models.PointBatchConsumption{}).Where("transaction_id = ?", txn).Count(&consCount)
	require.Equal(t, int64(2), consCount, "跨 2 批次留痕")
}

func TestPointBatch_RestoreByTransaction(t *testing.T) {
	setupPointBatchTestDB(t)
	db, userID, _ := mkPointBatchUser(t, 0)

	now := time.Now()
	soon := now.Add(time.Hour)
	far := now.Add(365 * 24 * time.Hour)
	bSoon := mkBatch(t, userID, 300, &soon, nil)
	bFar := mkBatch(t, userID, 500, &far, nil)

	txn := uuid.New().String()
	consumed, err := ConsumePointsFIFO(db, userID, 700, txn)
	require.NoError(t, err)
	require.Equal(t, models.Cents(700), consumed)

	restored, err := RestorePointsByTransaction(db, userID, txn, 700)
	require.NoError(t, err)
	require.Equal(t, models.Cents(700), restored)

	var rSoon, rFar models.PointBatch
	require.NoError(t, db.Where("id = ?", bSoon.ID).First(&rSoon).Error)
	require.NoError(t, db.Where("id = ?", bFar.ID).First(&rFar).Error)
	require.Equal(t, models.Cents(300), rSoon.RemainingCents, "恢复原批次（到期日不变）")
	require.Equal(t, models.Cents(500), rFar.RemainingCents)
	require.NotNil(t, rSoon.ExpiresAt)

	// 幂等：重复恢复不产生额外额度
	again, err := RestorePointsByTransaction(db, userID, txn, 700)
	require.NoError(t, err)
	require.Equal(t, models.Cents(0), again)
}
func TestPointBatch_ExpireDueBatches(t *testing.T) {
	setupPointBatchTestDB(t)
	db, userID, tenantID := mkPointBatchUser(t, 150)

	past := time.Now().Add(-time.Hour)
	expired := mkBatch(t, userID, 150, &past, nil)

	n, total, err := ExpireDueBatches(db)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, models.Cents(150), total)

	var b models.PointBatch
	require.NoError(t, db.Where("id = ?", expired.ID).First(&b).Error)
	require.Equal(t, models.Cents(0), b.RemainingCents)
	require.Equal(t, models.Cents(150), b.ExpiredCents)
	require.NotNil(t, b.ExpiredAt)

	var u models.User
	require.NoError(t, db.Where("id = ?", userID).First(&u).Error)
	require.Equal(t, models.Cents(0), u.PromoPoints, "快照同步扣减")

	var expiredTx int64
	db.Model(&models.PointsTransaction{}).
		Where("user_id = ? AND tenant_id = ? AND type = ?", userID, tenantID, "expired").Count(&expiredTx)
	require.Equal(t, int64(1), expiredTx, "过期留痕交易")

	// B 裁定：按用户合并一条过期通知
	var expiredNotif int64
	db.Model(&models.Notification{}).
		Where("user_id = ? AND type = ?", userID, "points_expired").Count(&expiredNotif)
	require.Equal(t, int64(1), expiredNotif, "合并过期通知")

	snapshot, batchSum, err := ReconcilePoints(db, userID)
	require.NoError(t, err)
	require.Equal(t, snapshot, batchSum, "快照 == SUM(未过期 remaining)")
}

func TestPointBatch_RemindExpiringBatches(t *testing.T) {
	setupPointBatchTestDB(t)
	db, userID, _ := mkPointBatchUser(t, 200)

	soon := time.Now().Add(10 * 24 * time.Hour) // 10 天内到期（< 30 天阈值）
	mkBatch(t, userID, 200, &soon, nil)

	n, err := RemindExpiringBatches(db)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	// 幂等：同一批次不重复提醒
	n2, err := RemindExpiringBatches(db)
	require.NoError(t, err)
	require.Equal(t, 0, n2)
}

func TestCreatePointsBatch_Validity(t *testing.T) {
	setupPointBatchTestDB(t)
	db, userID, _ := mkPointBatchUser(t, 0)

	normal, err := CreatePointsBatch(db, userID, PointBatchSourceSignup, "registration", 9900)
	require.NoError(t, err)
	require.NotNil(t, normal)
	require.NotNil(t, normal.ExpiresAt, "普通批次有到期日")
	require.Equal(t, 1, normal.ExpiresAt.Day(), "归一化到次月首日")
	require.Equal(t, 0, normal.ExpiresAt.Hour(), "零点")
	require.Equal(t, 0, normal.ExpiresAt.In(pointBatchLocation).Hour())

	mig, err := CreatePointsBatch(db, userID, PointBatchSourceMigration, "legacy", 5000)
	require.NoError(t, err)
	require.NotNil(t, mig.ExpiresAt, "迁移批次同样归一化（D 裁定 now+2y）")
	require.Equal(t, 1, mig.ExpiresAt.Day())

	// 非正数不建批次
	none, err := CreatePointsBatch(db, userID, PointBatchSourceSignup, "", 0)
	require.NoError(t, err)
	require.Nil(t, none)
}

// TestNormalizeBatchExpiry_NextMonthFirst：2026-09-10 获取、2 年期 → 2028-10-01 00:00（北京）。
func TestNormalizeBatchExpiry_NextMonthFirst(t *testing.T) {
	acquired := time.Date(2026, 9, 10, 15, 30, 0, 0, pointBatchLocation)
	got := normalizeBatchExpiry(acquired)
	want := time.Date(2028, 10, 1, 0, 0, 0, 0, pointBatchLocation)
	require.True(t, got.Equal(want), "got %s want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
}

// TestPointBatch_MigrationSQL 直接执行迁移 up 文件，验证存量余额 → migration 批次（幂等）。
func TestPointBatch_MigrationSQL(t *testing.T) {
	setupPointBatchTestDB(t)
	db, userID, _ := mkPointBatchUser(t, 250)

	sqlBytes, err := os.ReadFile("../database/migrations/20260917010_point_batches.up.sql")
	require.NoError(t, err)

	run := func() {
		for _, stmt := range strings.Split(string(sqlBytes), ";") {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			require.NoError(t, db.Exec(stmt).Error)
		}
	}
	run()
	run() // 幂等

	var count int64
	db.Model(&models.PointBatch{}).
		Where("user_id = ? AND source_type = ?", userID, PointBatchSourceMigration).Count(&count)
	require.Equal(t, int64(1), count, "每余额用户一条 migration 批次（幂等）")

	var b models.PointBatch
	require.NoError(t, db.Where("user_id = ? AND source_type = ?", userID, PointBatchSourceMigration).First(&b).Error)
	require.Equal(t, models.Cents(250), b.AmountCents)
	require.Equal(t, models.Cents(250), b.RemainingCents)
	require.NotNil(t, b.ExpiresAt, "D 裁定：迁移批次 now+2y（归一化，不再 NULL）")
	require.Equal(t, 1, b.ExpiresAt.In(pointBatchLocation).Day(), "归一化到次月首日")

	// down 迁移：移除两表
	downBytes, err := os.ReadFile("../database/migrations/20260917010_point_batches.down.sql")
	require.NoError(t, err)
	for _, stmt := range strings.Split(string(downBytes), ";") {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		require.NoError(t, db.Exec(stmt).Error)
	}
	var remaining int64
	db.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('point_batches','point_batch_consumptions')`).Scan(&remaining)
	require.Equal(t, int64(0), remaining, "down 迁移移除批次两表")
}

func TestPointBatchScheduler_Run(t *testing.T) {
	setupPointBatchTestDB(t)
	db, userID, _ := mkPointBatchUser(t, 150)

	past := time.Now().Add(-time.Hour)
	mkBatch(t, userID, 150, &past, nil)
	soon := time.Now().Add(5 * 24 * time.Hour)
	mkBatch(t, userID, 100, &soon, nil)

	s := NewPointBatchScheduler()
	s.SetDBForTest(db)
	s.RunForTest()

	var expired models.PointBatch
	require.NoError(t, db.Where("user_id = ? AND expired_cents > 0", userID).First(&expired).Error)
	var notifCount int64
	db.Model(&models.Notification{}).
		Where("user_id = ? AND type = ?", userID, "points_expiring").Count(&notifCount)
	require.Equal(t, int64(1), notifCount)
}
