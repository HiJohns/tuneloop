package database

// #2360（#2268 子任务）：结算数据模型与支付分配账本的迁移验收。
//
// R1 单迁移 up → down → up 幂等；R2 订单级唯一最终结算（5 种 active 状态）；
// R4 新表/列登记 + 启动校验（validateModelColumns / defaultMigrationArtifacts）。
//
// 所有用例在独立 schema 中执行（不改动共享测试库的 public schema），
// 通过 DSN 的 search_path + simple_protocol 让整份迁移文件一次执行
// （多语句需要 simple protocol）。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"tuneloop-backend/models"
)

const settlement2360Migration = "20261116001_settlement_ledger_2360"

// settlement2360BaseDDL 是迁移前 settlement_calculations 的等价形态
// （20260822001 + 20260824007 result TEXT + 20261025002 ledger_version）。
const settlement2360BaseDDL = `
CREATE TABLE IF NOT EXISTS settlement_calculations (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id       UUID NOT NULL,
    tenant_id      UUID,
    trigger        VARCHAR(20) NOT NULL DEFAULT 'preview',
    input_snapshot JSONB,
    result         TEXT,
    actual_days    INTEGER NOT NULL DEFAULT 0,
    ledger_version BIGINT NOT NULL DEFAULT 0,
    created_at     TIMESTAMP NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_sc_order_id ON settlement_calculations (order_id);
`

func settlement2360Env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func settlement2360DSN(schema, dbName string) string {
	if dbName == "" {
		dbName = settlement2360Env("TUNELOOP_TEST_DB", settlement2360Env("TUNELOOP_DB", "tuneloop"))
	}
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable default_query_exec_mode=simple_protocol",
		settlement2360Env("POSTGRES_HOST", "localhost"),
		settlement2360Env("POSTGRES_PORT", "5432"),
		settlement2360Env("POSTGRES_USER", "tuneloop"),
		os.Getenv("POSTGRES_PASSWORD"),
		dbName,
	)
	if schema != "" {
		dsn += " search_path=" + schema
	}
	return dsn
}

// ensureDatabase2360 连接维护库 postgres，必要时创建测试库（与 handlers TestMain 同口径）。
func ensureDatabase2360(t *testing.T) {
	t.Helper()
	dbName := settlement2360Env("TUNELOOP_TEST_DB", settlement2360Env("TUNELOOP_DB", "tuneloop"))
	admin, err := gorm.Open(gormPostgres.Open(settlement2360DSN("", "postgres")), &gorm.Config{})
	if err != nil {
		t.Skipf("test database not available: %v", err)
	}
	var exists int64
	require.NoError(t, admin.Raw(`SELECT COUNT(*) FROM pg_database WHERE datname = ?`, dbName).Scan(&exists).Error)
	if exists == 0 {
		require.NoError(t, admin.Exec(`CREATE DATABASE "`+strings.ReplaceAll(dbName, `"`, `""`)+`"`).Error,
			"create test database %s", dbName)
	}
}

// openScratch2360 建立独立 schema（不存在则创建，存在则先清空），返回指向它的句柄。
func openScratch2360(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	schema := "mig2360_" + strings.ReplaceAll(uuid.New().String()[:8], "-", "")
	ensureDatabase2360(t)

	admin, err := gorm.Open(gormPostgres.Open(settlement2360DSN("", "")), &gorm.Config{})
	if err != nil {
		t.Skipf("test database not available: %v", err)
	}
	require.NoError(t, admin.Exec("CREATE SCHEMA IF NOT EXISTS "+schema).Error)

	db, err := gorm.Open(gormPostgres.Open(settlement2360DSN(schema, "")), &gorm.Config{})
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = admin.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE").Error
	})
	return db, schema
}

func readMigration2360(t *testing.T, suffix string) string {
	t.Helper()
	candidates := []string{
		filepath.Join("migrations", settlement2360Migration+suffix),
		filepath.Join("..", "database", "migrations", settlement2360Migration+suffix),
	}
	if dir := strings.TrimSpace(os.Getenv("MIGRATION_PATH")); dir != "" {
		candidates = append([]string{filepath.Join(dir, settlement2360Migration+suffix)}, candidates...)
	}
	for _, p := range candidates {
		if b, err := os.ReadFile(p); err == nil {
			return string(b)
		}
	}
	t.Fatalf("migration file not found: %s", settlement2360Migration+suffix)
	return ""
}

// 说明：information_schema/pg_indexes 是跨 schema 的全局视图，必须带 schema 过滤，
// 否则同库里其它测试（public schema 的 AutoMigrate）会污染断言（#2360 实测）。

func columnExists2360(t *testing.T, db *gorm.DB, schema, table, column string) bool {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM information_schema.columns
        WHERE table_schema = ? AND table_name = ? AND column_name = ?`, schema, table, column).Scan(&n).Error)
	return n > 0
}

func tableExists2360(t *testing.T, db *gorm.DB, schema, table string) bool {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM information_schema.tables
        WHERE table_schema = ? AND table_name = ?`, schema, table).Scan(&n).Error)
	return n > 0
}

func indexExists2360(t *testing.T, db *gorm.DB, schema, name string) bool {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM pg_indexes WHERE schemaname = ? AND indexname = ?`, schema, name).Scan(&n).Error)
	return n > 0
}

// R1：空库（仅迁移前基线表）→ up → down → up，验证幂等与结构增删。
func TestMigrationSettlementModel_2360(t *testing.T) {
	db, schema := openScratch2360(t)
	require.NoError(t, db.Exec(settlement2360BaseDDL).Error)

	up := readMigration2360(t, ".up.sql")
	down := readMigration2360(t, ".down.sql")

	// 第一次 up
	require.NoError(t, db.Exec(up).Error, "first up must succeed")
	for _, col := range []string{
		"settlement_version", "status", "algorithm_version", "ratio_num", "ratio_den", "ratio_source",
		"calculated_at", "accounts_json", "rent_cash_paid_net_cents", "deposit_cash_collected_net_cents",
		"deposit_held_cents", "overdue_penalty_cents", "damage_fee_cents", "logistics_fee_cents",
		"damage_report_id", "logistics_ref", "pricing_policy_snapshot", "pricing_policy_version", "legacy_marker",
	} {
		assert.True(t, columnExists2360(t, db, schema, "settlement_calculations", col), "column %s must exist after up", col)
	}
	assert.True(t, tableExists2360(t, db, schema, "order_payment_allocations"))
	assert.True(t, tableExists2360(t, db, schema, "settlement_action_ledger"))
	assert.True(t, indexExists2360(t, db, schema, "idx_settlement_active_order"))
	assert.True(t, indexExists2360(t, db, schema, "idx_opa_source"))
	assert.True(t, indexExists2360(t, db, schema, "idx_sal_idempotency"))

	// down：结构消失
	require.NoError(t, db.Exec(down).Error, "down must succeed")
	assert.False(t, tableExists2360(t, db, schema, "order_payment_allocations"), "down drops allocation table")
	assert.False(t, tableExists2360(t, db, schema, "settlement_action_ledger"), "down drops action ledger")
	assert.False(t, columnExists2360(t, db, schema, "settlement_calculations", "settlement_version"))
	assert.False(t, columnExists2360(t, db, schema, "settlement_calculations", "legacy_marker"))
	assert.False(t, indexExists2360(t, db, schema, "idx_settlement_active_order"))

	// 第二次 up：幂等恢复
	require.NoError(t, db.Exec(up).Error, "second up must succeed (idempotent)")
	assert.True(t, columnExists2360(t, db, schema, "settlement_calculations", "settlement_version"))
	assert.True(t, tableExists2360(t, db, schema, "order_payment_allocations"))
	assert.True(t, indexExists2360(t, db, schema, "idx_settlement_active_order"))
}

// R2：订单级唯一最终结算——5 种 active 状态均阻止第二版本；failed 历史行可插。
func TestSettlementUniqueActive_2360(t *testing.T) {
	db, _ := openScratch2360(t)
	require.NoError(t, db.Exec(settlement2360BaseDDL).Error)
	require.NoError(t, db.Exec(readMigration2360(t, ".up.sql")).Error)

	orderID := uuid.New().String()
	require.NoError(t, db.Exec(`INSERT INTO settlement_calculations
        (order_id, trigger, ledger_version, settlement_version, status, calculated_at)
        VALUES (?, 'confirm', 1, 1, 'confirmed', now())`, orderID).Error)

	active := []string{"confirmed", "processing", "succeeded", "partially_succeeded", "failed_retryable"}
	rejected := 0
	for i, status := range active {
		err := db.Exec(`INSERT INTO settlement_calculations
            (order_id, trigger, ledger_version, settlement_version, status, calculated_at)
            VALUES (?, 'confirm', ?, ?, ?, now())`, orderID, i+2, i+2, status).Error
		if err == nil {
			t.Fatalf("status %s must be rejected by idx_settlement_active_order", status)
		}
		assert.Contains(t, strings.ToLower(err.Error()), "unique", "status %s error must be a unique violation, got: %v", status, err)
		rejected++
	}
	assert.Equal(t, 5, rejected, "all five active statuses must be blocked")

	// failed（非 active）/ calculated（非 active）允许插入
	require.NoError(t, db.Exec(`INSERT INTO settlement_calculations
        (order_id, trigger, ledger_version, settlement_version, status)
        VALUES (?, 'confirm', 10, 10, 'failed')`, orderID).Error)
	require.NoError(t, db.Exec(`INSERT INTO settlement_calculations
        (order_id, trigger, ledger_version, settlement_version, status)
        VALUES (?, 'preview', 11, 11, 'calculated')`, orderID).Error)

	// 不同订单不受影响
	otherID := uuid.New().String()
	require.NoError(t, db.Exec(`INSERT INTO settlement_calculations
        (order_id, trigger, ledger_version, settlement_version, status)
        VALUES (?, 'confirm', 1, 1, 'confirmed')`, otherID).Error)
}

// R4：新表/列登记 + 启动 schema 校验（validateModelColumns / 工件断言）。
func TestSchemaValidationSettlement_2360(t *testing.T) {
	db, schema := openScratch2360(t)
	require.NoError(t, db.Exec(settlement2360BaseDDL).Error)
	require.NoError(t, db.Exec(readMigration2360(t, ".up.sql")).Error)

	// 先做 schema 作用域断言（information_schema 是跨 schema 全局视图，
	// 不能只靠 validateModelColumns 判断本 schema 的迁移是否生效）。
	for _, col := range []string{
		"settlement_version", "status", "algorithm_version", "ratio_num", "ratio_den", "ratio_source",
		"calculated_at", "accounts_json", "rent_cash_paid_net_cents", "deposit_cash_collected_net_cents",
		"deposit_held_cents", "overdue_penalty_cents", "damage_fee_cents", "logistics_fee_cents",
		"damage_report_id", "logistics_ref", "pricing_policy_snapshot", "pricing_policy_version", "legacy_marker",
	} {
		assert.True(t, columnExists2360(t, db, schema, "settlement_calculations", col), "migration must add %s", col)
	}
	assert.True(t, tableExists2360(t, db, schema, "order_payment_allocations"))
	assert.True(t, tableExists2360(t, db, schema, "settlement_action_ledger"))

	// 启动校验会对这三个模型逐列核对
	require.NoError(t, validateModelColumns(db, &models.SettlementCalculation{}), "settlement_calculations columns")
	require.NoError(t, validateModelColumns(db, &models.OrderPaymentAllocation{}), "order_payment_allocations table/columns")
	require.NoError(t, validateModelColumns(db, &models.SettlementActionLedger{}), "settlement_action_ledger table/columns")

	// 工件断言：仅本迁移登记项（scratch schema 只重放了 20261116001，
	// 不能连带断言更早迁移的列，否则会误报 orders.order_no 等历史工件）。
	var arts []migrationArtifact
	for _, a := range defaultMigrationArtifacts {
		if a.Version == 20261116001 {
			arts = append(arts, a)
		}
	}
	require.NotEmpty(t, arts, "20261116001 artifacts must be registered")
	require.NoError(t, verifyArtifacts(db, 20261116001, arts), "20261116001 artifacts must match schema")
	// 负例：登记不存在的列必须被识别（防止断言失效却静默通过）
	bad := append(append([]migrationArtifact{}, arts...), migrationArtifact{Version: 20261116001, Table: "settlement_calculations", Column: "settlement_missing_col_2360", Desc: "synthetic"})
	require.Error(t, verifyArtifacts(db, 20261116001, bad), "synthetic missing column must fail verification")

	// 启动清单必须包含两个新模型（否则启动校验不会覆盖）
	found := map[string]bool{}
	for _, m := range ModelsToValidateForTest() {
		switch m.(type) {
		case *models.OrderPaymentAllocation:
			found["allocation"] = true
		case *models.SettlementActionLedger:
			found["action"] = true
		}
	}
	assert.True(t, found["allocation"], "OrderPaymentAllocation must be in modelsToValidate")
	assert.True(t, found["action"], "SettlementActionLedger must be in modelsToValidate")

	// 历史回填：confirm 行 → confirmed（仅最新一条），preview 保持 calculated。
	orderID := uuid.New().String()
	require.NoError(t, db.Exec(`INSERT INTO settlement_calculations (order_id, trigger, ledger_version) VALUES (?, 'confirm', 1)`, orderID).Error)
	require.NoError(t, db.Exec(`INSERT INTO settlement_calculations (order_id, trigger, ledger_version) VALUES (?, 'confirm', 2)`, orderID).Error)
	require.NoError(t, db.Exec(`INSERT INTO settlement_calculations (order_id, trigger, ledger_version) VALUES (?, 'preview', 3)`, orderID).Error)

	// down + up 触发回填逻辑（up 可重复执行）
	require.NoError(t, db.Exec(readMigration2360(t, ".down.sql")).Error)
	require.NoError(t, db.Exec(readMigration2360(t, ".up.sql")).Error)

	// 回填后同一订单只能有一条 confirmed，不违反 partial unique index
	var confirmed int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM settlement_calculations WHERE order_id = ? AND status = 'confirmed'`, orderID).Scan(&confirmed).Error)
	assert.EqualValues(t, 1, confirmed, "only the latest confirm row becomes confirmed")
}
