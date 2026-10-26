package handlers

// #2282 P4b：结算计算幂等键 (order_id, trigger, ledger_version)。

import (
	"sync"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettlementCalculation_LedgerVersion_2282(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	org, user, orderID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Create(&models.User{ID: user, IAMSub: user, TenantID: org, OrgID: org, Status: "active"}).Error)
	order := models.Order{ID: orderID, TenantID: org, OrgID: org, UserID: user,
		InstrumentID: uuid.NewString(), Level: "entry", LeaseTerm: 30, Status: "completed"}
	require.NoError(t, db.Create(&order).Error)

	// 无账本（version=0）：重复触发只保留 1 行，并返回既有行。
	r1, err := recordSettlementCalculation(db, &order, "confirm", 30, []byte(`{"a":1}`))
	require.NoError(t, err)
	r2, err := recordSettlementCalculation(db, &order, "confirm", 30, []byte(`{"a":2}`))
	require.NoError(t, err)
	assert.Equal(t, r1.ID, r2.ID, "重复 confirm 必须返回既有 calculation")

	var rows []models.SettlementCalculation
	require.NoError(t, db.Where("order_id = ? AND trigger = ?", orderID, "confirm").Order("ledger_version").Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.EqualValues(t, 0, rows[0].LedgerVersion)

	// 新增支付流水 → version 变化 → 产生新版本（旧版本不可变）。
	pr := models.OrderPaymentRecord{ID: uuid.NewString(), TenantID: org, UserID: user, OrderID: &orderID,
		OrderType: "rent", Amount: models.FromYuan(100), Type: "payment", Status: "paid"}
	require.NoError(t, db.Create(&pr).Error)
	r3, err := recordSettlementCalculation(db, &order, "confirm", 30, []byte(`{"a":3}`))
	require.NoError(t, err)
	assert.NotEqual(t, r1.ID, r3.ID)
	assert.EqualValues(t, 1, r3.LedgerVersion)
	require.NoError(t, db.Where("order_id = ? AND trigger = ?", orderID, "confirm").Order("ledger_version").Find(&rows).Error)
	require.Len(t, rows, 2)
	assert.EqualValues(t, 0, rows[0].LedgerVersion)
	assert.EqualValues(t, 1, rows[1].LedgerVersion)
	assert.JSONEq(t, `{"a":1}`, *rows[0].Result, "旧版本内容不得被覆盖")
}

func TestSettlementCalculation_ConcurrentIdempotent_2282(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	org, user, orderID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Create(&models.User{ID: user, IAMSub: user, TenantID: org, OrgID: org, Status: "active"}).Error)
	order := models.Order{ID: orderID, TenantID: org, OrgID: org, UserID: user,
		InstrumentID: uuid.NewString(), Level: "entry", LeaseTerm: 30, Status: "completed"}
	require.NoError(t, db.Create(&order).Error)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = recordSettlementCalculation(db, &order, "confirm", 30, []byte(`{"a":1}`))
		}()
	}
	wg.Wait()

	var count int64
	require.NoError(t, db.Model(&models.SettlementCalculation{}).
		Where("order_id = ? AND trigger = ?", orderID, "confirm").Count(&count).Error)
	assert.EqualValues(t, 1, count, "并发重复触发仍只允许 1 行（唯一键兜底）")
}
