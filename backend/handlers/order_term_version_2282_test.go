package handlers

// #2282 P4a：订单创建写入 version=1 的不可变期限快照。

import (
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrderTermVersion_InitialSnapshot_2282(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	pb := `{"base_daily_rent":100,"rent_days":30,"final_daily_rent":100,"total_amount":3000,"tier_segments":[{"days":30,"rate":100,"discount":1,"subtotal":3000}]}`
	order := models.Order{
		ID: uuid.NewString(), TenantID: uuid.NewString(), OrgID: uuid.NewString(),
		UserID: uuid.NewString(), InstrumentID: uuid.NewString(), Level: "entry",
		LeaseTerm: 30, Status: "reserved", PricingBreakdown: &pb,
	}
	require.NoError(t, db.Create(&order).Error)
	insertInitialTermVersion(db, &order)

	var v models.OrderTermVersion
	require.NoError(t, db.Where("order_id = ? AND version = 1", order.ID).First(&v).Error)
	assert.Equal(t, 30, v.Days)
	assert.Equal(t, models.FromYuan(3000), v.TotalAmount, "总额必须是分")
	assert.Contains(t, v.TierSegments, "\"days\"")
}
