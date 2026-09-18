package handlers

import (
	"testing"
	"time"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1965：业务订单号生成（格式/当日递增/唯一冲突重试）
func TestGenerateOrderNo(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	now := time.Now()
	no1 := generateOrderNo(db, now)
	assert.Regexp(t, `^YL[0-9]{8}-[0-9]{3}$`, no1, "格式 YL<YYYYMMDD>-<NNN>")

	// 落库后 +1
	tenantID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{ID: "11111111-1111-4111-8111-111111111111", IAMSub: "11111111-1111-4111-8111-111111111111", TenantID: tenantID, OrgID: tenantID, Username: "u", Name: "n", Status: "active"}).Error)
	require.NoError(t, db.Create(&models.Instrument{ID: "22222222-2222-4222-8222-222222222222", TenantID: tenantID, SN: "SN-ORDNO", StockStatus: "available"}).Error)
	order := &models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: tenantID,
		UserID: "11111111-1111-4111-8111-111111111111", InstrumentID: "22222222-2222-4222-8222-222222222222",
		Status: "reserved", CreatedAt: now,
	}
	require.NoError(t, createOrderWithNo(db, order))
	assert.Equal(t, no1, order.OrderNo, "首单 = 序号 001")
	order2 := &models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: tenantID,
		UserID: "11111111-1111-4111-8111-111111111111", InstrumentID: "22222222-2222-4222-8222-222222222222",
		Status: "reserved", CreatedAt: now,
	}
	require.NoError(t, createOrderWithNo(db, order2))
	assert.Regexp(t, `^YL[0-9]{8}-[0-9]{3}$`, order2.OrderNo)
	assert.NotEqual(t, order.OrderNo, order2.OrderNo, "序号递增")
	assert.Equal(t, "YL"+now.Format("20060102")+"-002", order2.OrderNo)
}
