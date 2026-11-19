package services

// #2361（#2268）：SettlementExecutor 验收测试。
// R1 并发唯一 / R2 幂等重试 / R3 部分成功 / R4 冲正版本 / R5 净额边界。
// 使用真实 PG（SELECT … FOR UPDATE 与 partial unique index 在 mock 下不可信）。

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"tuneloop-backend/models"
)

type fakeRunner2361 struct {
	mu        sync.Mutex
	calls     map[string]int
	failTimes map[string]int
	failWith  map[string]error
}

func newFakeRunner2361() *fakeRunner2361 {
	return &fakeRunner2361{
		calls:     map[string]int{},
		failTimes: map[string]int{},
		failWith:  map[string]error{},
	}
}

func (f *fakeRunner2361) Execute(ctx context.Context, req SettlementActionRequest) (string, error) {
	f.mu.Lock()
	f.calls[req.ActionType]++
	remaining := f.failTimes[req.ActionType]
	if remaining > 0 {
		f.failTimes[req.ActionType] = remaining - 1
		err := f.failWith[req.ActionType]
		f.mu.Unlock()
		if err == nil {
			err = errors.New("channel returned 500")
		}
		return "", err
	}
	f.mu.Unlock()
	if req.IdempotencyKey == "" {
		return "", errors.New("missing idempotency key")
	}
	return "txn-" + req.ActionType, nil
}

func (f *fakeRunner2361) callCount(action string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[action]
}

func executor2361Env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func executor2361DSN(schema, dbName string) string {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable default_query_exec_mode=simple_protocol",
		executor2361Env("POSTGRES_HOST", "localhost"),
		executor2361Env("POSTGRES_PORT", "5432"),
		executor2361Env("POSTGRES_USER", "tuneloop"),
		os.Getenv("POSTGRES_PASSWORD"),
		dbName,
	)
	if schema != "" {
		dsn += " search_path=" + schema
	}
	return dsn
}

// openBaseScratch2361 在独立数据库 + 独立 schema 中打开句柄，绝不触碰开发库
// （#2360 测试同口径：TUNELOOP_TEST_DB，默认 tuneloop_test，必要时创建）。
func openBaseScratch2361(t *testing.T) *gorm.DB {
	t.Helper()
	dbName := executor2361Env("TUNELOOP_TEST_DB", "tuneloop_test")
	admin, err := gorm.Open(gormPostgres.Open(executor2361DSN("", "postgres")), &gorm.Config{})
	if err != nil {
		t.Skipf("test database not available: %v", err)
	}
	var exists int64
	require.NoError(t, admin.Raw(`SELECT COUNT(*) FROM pg_database WHERE datname = ?`, dbName).Scan(&exists).Error)
	if exists == 0 {
		require.NoError(t, admin.Exec(`CREATE DATABASE "`+strings.ReplaceAll(dbName, `"`, `""`)+`"`).Error,
			"create test database %s", dbName)
	}
	schema := "exec2361_" + strings.ReplaceAll(uuid.New().String()[:8], "-", "")
	base, err := gorm.Open(gormPostgres.Open(executor2361DSN("", dbName)), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, base.Exec("CREATE SCHEMA IF NOT EXISTS "+schema).Error)

	db, err := gorm.Open(gormPostgres.Open(executor2361DSN(schema, dbName)), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = base.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE").Error
		if sqlDB, err := base.DB(); err == nil {
			_ = sqlDB.Close()
		}
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func openExecutorScratch2361(t *testing.T) *gorm.DB {
	t.Helper()
	db := openBaseScratch2361(t)
	require.NoError(t, db.AutoMigrate(
		&models.SettlementCalculation{}, &models.SettlementActionLedger{},
		&models.Instrument{}, &models.Order{},
	))
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_settlement_active_order
		ON settlement_calculations (order_id)
		WHERE status IN ('confirmed','processing','succeeded','partially_succeeded','failed_retryable')`).Error)
	return db
}

func setupExecutor2361(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	db := openExecutorScratch2361(t)

	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	userID := uuid.New().String()
	instID := uuid.New().String()
	require.NoError(t, db.Create(&models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "EXEC-" + uuid.New().String()[:8], StockStatus: "rented",
	}).Error)
	order := models.Order{
		TenantID: tenantID, OrgID: orgID, UserID: userID, InstrumentID: instID,
		Status: models.OrderStatusReturned,
	}
	require.NoError(t, db.Create(&order).Error)
	return db, order.ID
}

func execInput2361(orderID string, version int64) SettlementInput {
	return SettlementInput{
		SettlementID:                 "exec-" + orderID,
		OrderID:                      orderID,
		SettlementVersion:            version,
		RatioNum:                     3,
		RatioDen:                     10,
		RatioSource:                  RatioSourceSnapshot,
		DailyRateCents:               10000,
		ActualDays:                   28,
		GiftPointsUsedStoredCents:    100000,
		CashPaidCents:                200000,
		RentCashPaidNetCents:         150000,
		DepositCashCollectedNetCents: 50000,
		DepositHeldCents:             50000,
	}
}

func TestSettlementExecutorConcurrency_2361(t *testing.T) {
	db, orderID := setupExecutor2361(t)
	runner := newFakeRunner2361()
	exec := NewSettlementExecutor(db, runner)

	const n = 10
	var wg sync.WaitGroup
	results := make([]*SettlementExecutionResult, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = exec.Execute(context.Background(), SettlementExecutionRequest{
				OrderID: orderID, Input: execInput2361(orderID, 1),
			})
		}(i)
	}
	wg.Wait()

	var settlementID string
	for i := 0; i < n; i++ {
		require.NoError(t, errs[i], "caller %d", i)
		require.NotNil(t, results[i])
		require.Equal(t, SettlementStatusSucceeded, results[i].Status, "caller %d", i)
		if settlementID == "" {
			settlementID = results[i].SettlementID
		}
		require.Equal(t, settlementID, results[i].SettlementID, "all callers must see the same settlement")
	}

	var count int64
	require.NoError(t, db.Model(&models.SettlementCalculation{}).Where("order_id = ?", orderID).Count(&count).Error)
	require.Equal(t, int64(1), count, "only one settlement row for the order")
	require.Equal(t, 1, runner.callCount(ActionDepositRefund))
	require.Equal(t, 1, runner.callCount(ActionFeeCharge))
	require.Equal(t, 1, runner.callCount(ActionPointsRestore))
}

func TestSettlementExecutorIdempotency_2361(t *testing.T) {
	db, orderID := setupExecutor2361(t)
	runner := newFakeRunner2361()
	exec := NewSettlementExecutor(db, runner)
	exec.Hooks.AfterAction = func(models.SettlementActionLedger) error {
		// 模拟：首个子动作（押金退款）已成功落库，进程在继续前中断。
		return errors.New("simulated crash after first action")
	}

	_, err := exec.Execute(context.Background(), SettlementExecutionRequest{
		OrderID: orderID, Input: execInput2361(orderID, 1),
	})
	require.Error(t, err, "first run must abort after the injected crash")

	var row models.SettlementCalculation
	require.NoError(t, db.Where("order_id = ?", orderID).First(&row).Error)
	require.Equal(t, SettlementStatusProcessing, row.Status)
	var firstKey string
	require.NoError(t, db.Model(&models.SettlementActionLedger{}).
		Where("settlement_id = ? AND action_type = ?", row.ID, ActionDepositRefund).
		Pluck("idempotency_key", &firstKey).Error)
	require.NotEmpty(t, firstKey)

	exec.Hooks.AfterAction = nil
	result, err := exec.Execute(context.Background(), SettlementExecutionRequest{
		OrderID: orderID, Input: execInput2361(orderID, 1),
	})
	require.NoError(t, err)
	require.Equal(t, SettlementStatusSucceeded, result.Status)
	require.Equal(t, 1, runner.callCount(ActionDepositRefund), "refund must not be replayed")
	require.Equal(t, 1, runner.callCount(ActionFeeCharge))
	require.Equal(t, 1, runner.callCount(ActionPointsRestore))

	var keys []string
	require.NoError(t, db.Model(&models.SettlementActionLedger{}).
		Where("settlement_id = ? AND action_type = ?", row.ID, ActionDepositRefund).
		Pluck("idempotency_key", &keys).Error)
	require.Equal(t, []string{firstKey}, keys, "idempotency key must stay stable across retries")
}

func TestSettlementExecutorPartial_2361(t *testing.T) {
	db, orderID := setupExecutor2361(t)
	runner := newFakeRunner2361()
	runner.failTimes[ActionPointsRestore] = 1
	exec := NewSettlementExecutor(db, runner)

	result, err := exec.Execute(context.Background(), SettlementExecutionRequest{
		OrderID: orderID, Input: execInput2361(orderID, 1),
	})
	require.NoError(t, err)
	require.Equal(t, SettlementStatusPartiallySucceeded, result.Status, "cash ok + points failed → partially_succeeded")
	require.Equal(t, 1, runner.callCount(ActionDepositRefund))
	require.Equal(t, 1, runner.callCount(ActionFeeCharge))
	require.Equal(t, 1, runner.callCount(ActionPointsRestore))

	// 重试：现金动作不得重放，只重试失败的点数恢复。
	result, err = exec.Execute(context.Background(), SettlementExecutionRequest{
		OrderID: orderID, Input: execInput2361(orderID, 1),
	})
	require.NoError(t, err)
	require.Equal(t, SettlementStatusSucceeded, result.Status)
	require.Equal(t, 1, runner.callCount(ActionDepositRefund), "cash action must not be replayed")
	require.Equal(t, 1, runner.callCount(ActionFeeCharge), "cash action must not be replayed")
	require.Equal(t, 2, runner.callCount(ActionPointsRestore), "only the failed action is retried")
}

func TestSettlementExecutorReversal_2361(t *testing.T) {
	db, orderID := setupExecutor2361(t)
	runner := newFakeRunner2361()
	exec := NewSettlementExecutor(db, runner)

	result, err := exec.Execute(context.Background(), SettlementExecutionRequest{
		OrderID: orderID, Input: execInput2361(orderID, 1),
	})
	require.NoError(t, err)
	require.Equal(t, SettlementStatusSucceeded, result.Status)
	oldID := result.SettlementID

	var before models.SettlementCalculation
	require.NoError(t, db.Where("id = ?", oldID).First(&before).Error)

	superseded, err := exec.Supersede(context.Background(), orderID, execInput2361(orderID, 2))
	require.NoError(t, err)
	require.Equal(t, SettlementStatusCalculated, superseded.Status)
	require.Equal(t, int64(2), superseded.Version)
	require.Equal(t, oldID, superseded.Supersedes)

	var after models.SettlementCalculation
	require.NoError(t, db.Where("id = ?", oldID).First(&after).Error)
	require.Equal(t, SettlementStatusSuperseded, after.Status)
	require.Equal(t, before.RentCashPaidNetCents, after.RentCashPaidNetCents, "old amounts must not change")
	require.Equal(t, before.DepositHeldCents, after.DepositHeldCents, "old amounts must not change")
	require.Equal(t, before.DepositCashCollectedNetCents, after.DepositCashCollectedNetCents, "old amounts must not change")

	var fresh models.SettlementCalculation
	require.NoError(t, db.Where("id = ?", superseded.SettlementID).First(&fresh).Error)
	require.Equal(t, SettlementStatusCalculated, fresh.Status)
	require.NotNil(t, fresh.Supersedes)
	require.Equal(t, oldID, *fresh.Supersedes)

	var active int64
	require.NoError(t, db.Model(&models.SettlementCalculation{}).
		Where("order_id = ? AND status IN ?", orderID, ActiveSettlementStatuses).Count(&active).Error)
	require.Zero(t, active, "superseded + calculated must not occupy the active unique slot")
}

func TestSettlementExecutorNetting_2361(t *testing.T) {
	db, orderID := setupExecutor2361(t)
	runner := newFakeRunner2361()
	exec := NewSettlementExecutor(db, runner)

	// 渠道/协议不允许净额 → 分笔：押金退款 + 费用扣收（≥2 个现金动作）。
	result, err := exec.Execute(context.Background(), SettlementExecutionRequest{
		OrderID: orderID, Input: execInput2361(orderID, 1), AllowNetting: false,
	})
	require.NoError(t, err)
	require.Equal(t, SettlementStatusSucceeded, result.Status)

	var ledgers []models.SettlementActionLedger
	require.NoError(t, db.Where("settlement_id = ?", result.SettlementID).Find(&ledgers).Error)
	var cashActions int
	var sum int64
	for _, l := range ledgers {
		switch l.ActionType {
		case ActionRentRefund, ActionDepositRefund, ActionNetRefund:
			cashActions++
			sum += int64(l.ExecutedAmountCents)
		case ActionFeeCharge, ActionNetCharge:
			cashActions++
			sum -= int64(l.ExecutedAmountCents)
		}
	}
	require.GreaterOrEqual(t, cashActions, 2, "non-netting path must split cash actions")
	require.Equal(t, int64(4000), result.NetCashCents)
	require.Equal(t, result.NetCashCents, sum, "ledger cash sum must equal net_cash")

	// 净额被允许 → 单个 net_refund（+ 点数动作），合计仍等于 net_cash。
	res, err := NewSettlementCalculator().Calculate(execInput2361(orderID, 3))
	require.NoError(t, err)
	plans, err := PlanSettlementActions(res, true)
	require.NoError(t, err)
	var netActions int
	for _, p := range plans {
		if p.Type == ActionNetRefund || p.Type == ActionNetCharge {
			netActions++
			require.Equal(t, res.NetCashCents, p.AmountCents)
		}
	}
	require.Equal(t, 1, netActions, "netting allowed → one net cash action")
}

// R6：迁移 up/down 幂等 + supersedes 列/索引真实存在（独立 schema）。
func TestSettlementSupersedesMigration_2361(t *testing.T) {
	db := openBaseScratch2361(t)
	require.NoError(t, db.Exec(`CREATE TABLE settlement_calculations (
		id UUID PRIMARY KEY, order_id UUID NOT NULL)`).Error)

	up, err := os.ReadFile(filepath.Join("..", "database", "migrations", "20261117001_settlement_supersedes_2361.up.sql"))
	require.NoError(t, err)
	require.NoError(t, db.Exec(string(up)).Error)

	columnCount := func() int64 {
		var n int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = 'settlement_calculations'
			  AND column_name = 'supersedes'`).Scan(&n).Error)
		return n
	}
	indexCount := func() int64 {
		var n int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM pg_indexes
			WHERE schemaname = current_schema() AND indexname = 'idx_settlement_supersedes'`).Scan(&n).Error)
		return n
	}
	require.Equal(t, int64(1), columnCount(), "up must add supersedes")
	require.Equal(t, int64(1), indexCount(), "up must add supersedes index")

	// up 幂等：重复执行不报错。
	require.NoError(t, db.Exec(string(up)).Error)

	down, err := os.ReadFile(filepath.Join("..", "database", "migrations", "20261117001_settlement_supersedes_2361.down.sql"))
	require.NoError(t, err)
	require.NoError(t, db.Exec(string(down)).Error)
	require.Equal(t, int64(0), columnCount(), "down must drop supersedes")
	require.Equal(t, int64(0), indexCount(), "down must drop supersedes index")
}
