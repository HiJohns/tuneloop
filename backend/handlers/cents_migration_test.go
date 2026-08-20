package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"tuneloop-backend/database"
	"tuneloop-backend/handlers/testfixtures"
)

// TestCentsMigration_20260820001 verifies the P2 cents migration end-to-end
// (#1727): DECIMAL(yuan) columns are converted to BIGINT(cents) and JSONB
// compound fields (pricing_breakdown / raw_response / action_data) are
// converted to cents with an idempotent marker — all fail-fast.
func TestCentsMigration_20260820001(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	// 模拟迁移前状态：测试基建按新模型（bigint）建表——把相关列改回 numeric(10,2)，
	// 使 INSERT 的元值不被截断，从而验证 up.sql 的 ×100 转换逻辑。
	for _, stmt := range []string{
		`ALTER TABLE orders ALTER COLUMN monthly_rent TYPE numeric(10,2)`,
		`ALTER TABLE orders ALTER COLUMN deposit TYPE numeric(10,2)`,
		`ALTER TABLE orders ALTER COLUMN shipping_fee TYPE numeric(10,2)`,
		`ALTER TABLE orders ALTER COLUMN cash_paid TYPE numeric(10,2)`,
		`ALTER TABLE order_payment_records ALTER COLUMN amount TYPE numeric(10,2)`,
	} {
		require.NoError(t, db.Exec(stmt).Error)
	}

	// --- 1. 造元数据（orders 关键列 + JSONB）---
	require.NoError(t, db.Exec(`INSERT INTO orders (id, tenant_id, org_id, user_id, instrument_id, level, lease_term,
		monthly_rent, deposit, shipping_fee, cash_paid, status, pricing_breakdown, created_at, updated_at) VALUES (
		'11111111-1111-4111-8111-111111111111',
		'00000000-0000-0000-0000-000000000000',
		'00000000-0000-0000-0000-000000000000',
		'00000000-0000-0000-0000-000000000001',
		'00000000-0000-0000-0000-000000000002',
		'standard', 1, 3000.00, 500.00, 12.50, 3512.50, 'reserved',
		'{"total_amount":3512.50,"base_daily_rent":100.00,"deposit":500.00,"shipping_fee":12.50}', NOW(), NOW())`).Error)

	require.NoError(t, db.Exec(`INSERT INTO order_payment_records (id, tenant_id, user_id, order_type, out_trade_no, amount, type, status, raw_response, created_at, updated_at) VALUES (
		'22222222-2222-4222-8222-222222222223',
		'00000000-0000-0000-0000-000000000000',
		'00000000-0000-0000-0000-000000000001',
		'rent', 'test-out-001', 3512.50, 'payment', 'pending',
		'{"gift_used":0.50}', NOW(), NOW())`).Error)

	db.Exec(`DELETE FROM notifications WHERE user_id='00000000-0000-0000-0000-000000000001'`)
	require.NoError(t, db.Exec(`INSERT INTO notifications (id, tenant_id, org_id, user_id, type, title, content, action_data, status, created_at, updated_at) VALUES (
		'33333333-3333-4333-8333-333333333334',
		'00000000-0000-0000-0000-000000000000',
		'00000000-0000-0000-0000-000000000000',
		'00000000-0000-0000-0000-000000000001',
		'payment', 'test', 'test',
		'{"payment_required":true,"amount":100.00}', 'unread', NOW(), NOW())`).Error)

	// --- 2. 执行 20260820001 up.sql（列转换）---
	sqlPath := filepath.Join("..", "database", "migrations", "20260820001_cents_money_columns.up.sql")
	sqlBytes, err := os.ReadFile(sqlPath)
	require.NoError(t, err)
	for _, stmt := range strings.Split(string(sqlBytes), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		err := db.Exec(stmt).Error
		if err != nil && strings.Contains(err.Error(), "does not exist") {
			continue // 测试基建未建的表跳过（部署库必然存在）
		}
		require.NoError(t, err, "statement: %s", stmt)
	}

	// --- 3. 校验金额列已为分（bigint 值）---
	var cols struct {
		MonthlyRent int64
		Deposit     int64
		ShippingFee int64
		CashPaid    int64
	}
	require.NoError(t, db.Raw(`SELECT monthly_rent, deposit, shipping_fee, cash_paid FROM orders WHERE id='11111111-1111-4111-8111-111111111111'`).Scan(&cols).Error)
	require.Equal(t, int64(300000), cols.MonthlyRent, "3000.00 yuan → 300000 cents")
	require.Equal(t, int64(50000), cols.Deposit, "500.00 → 50000")
	require.Equal(t, int64(1250), cols.ShippingFee, "12.50 → 1250")
	require.Equal(t, int64(351250), cols.CashPaid, "3512.50 → 351250")

	// --- 4. JSONB 迁移（dry-run → 执行 → 幂等）---
	count, err := MigrateJSONBCents(true)
	require.NoError(t, err)
	require.Equal(t, 3, count, "dry-run counts 3 records (orders.pricing_breakdown, payment raw_response, notification action_data)")

	count, err = MigrateJSONBCents(false)
	require.NoError(t, err)
	require.Equal(t, 3, count, "executed 3 records")

	count, err = MigrateJSONBCents(false)
	require.NoError(t, err)
	require.Equal(t, 0, count, "idempotent — rerun converts nothing")

	// 回滚验证：分 → 元 + 移除标记
	count, err = MigrateJSONBCentsWithMode(true, true)
	require.NoError(t, err)
	require.Equal(t, 3, count, "reverse dry-run counts 3")
	count, err = MigrateJSONBCentsWithMode(false, true)
	require.NoError(t, err)
	require.Equal(t, 3, count, "reverse executed 3")
	count, err = MigrateJSONBCentsWithMode(false, true)
	require.NoError(t, err)
	require.Equal(t, 0, count, "reverse idempotent")
	// 正向可再次执行（回滚后未标记 → 重新转换）
	count, err = MigrateJSONBCents(false)
	require.NoError(t, err)
	require.Equal(t, 3, count, "re-migrate after reverse")

	// --- 5. 校验 JSONB 值已为分 + 标记 ---
	var pbJSON string
	require.NoError(t, db.Raw(`SELECT pricing_breakdown FROM orders WHERE id='11111111-1111-4111-8111-111111111111'`).Scan(&pbJSON).Error)
	var pb map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(pbJSON), &pb))
	require.Equal(t, float64(351250), pb["total_amount"], "total_amount 3512.50 元 → 351250 分")
	require.Equal(t, float64(1250), pb["shipping_fee"])
	require.Equal(t, true, pb["_cents_migrated"])

	var rawJSON string
	require.NoError(t, db.Raw(`SELECT raw_response FROM order_payment_records WHERE id='22222222-2222-4222-8222-222222222223'`).Scan(&rawJSON).Error)
	var rawMap map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(rawJSON), &rawMap))
	require.Equal(t, float64(50), rawMap["gift_used"], "gift_used 0.50 元 → 50 分")

	// --- 6. 启动校验（金额列 bigint + JSONB 已迁移）---
	require.NoError(t, database.ValidateMoneyColumnsForTest(db), "money columns must be bigint after migration")
}
