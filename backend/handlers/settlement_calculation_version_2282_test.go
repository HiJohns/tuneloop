package handlers

// #2282 P4b：结算计算幂等键 (order_id, trigger, ledger_version) 递增。

import (
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

	recordSettlementCalculation(db, &order, "confirm", 30, []byte(`{"a":1}`))
	recordSettlementCalculation(db, &order, "confirm", 30, []byte(`{"a":2}`))

	var rows []models.SettlementCalculation
	require.NoError(t, db.Where("order_id = ? AND trigger = ?", orderID, "confirm").Order("ledger_version").Find(&rows).Error)
	require.Len(t, rows, 2)
	assert.EqualValues(t, 1, rows[0].LedgerVersion)
	assert.EqualValues(t, 2, rows[1].LedgerVersion)
}
