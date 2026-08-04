package services

import (
	"testing"

	"github.com/stretchr/testify/require"
	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

func setupMembershipTestDB(t *testing.T) func() {
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
	}
	database.SetDB(db)

	_ = db.Migrator().DropTable(&models.MembershipLevel{})
	_ = db.Migrator().DropTable(&models.Settlement{})
	_ = db.Migrator().DropTable(&models.OrderRefundRecord{})
	_ = db.Migrator().DropTable(&models.OrderPaymentRecord{})
	_ = db.Migrator().DropTable(&models.Order{})
	_ = db.Migrator().DropTable(&models.Instrument{})
	_ = db.Migrator().DropTable(&models.User{})
	require.NoError(t, db.Migrator().CreateTable(&models.User{}))
	db.Exec("ALTER TABLE users ADD COLUMN IF NOT EXISTS iam_sub VARCHAR(255) NOT NULL DEFAULT ''")
	require.NoError(t, db.Migrator().CreateTable(&models.Instrument{}))
	require.NoError(t, db.Migrator().CreateTable(&models.Order{}))
	require.NoError(t, db.Migrator().CreateTable(&models.OrderPaymentRecord{}))
	require.NoError(t, db.Migrator().CreateTable(&models.OrderRefundRecord{}))
	require.NoError(t, db.Migrator().CreateTable(&models.Settlement{}))
	require.NoError(t, db.Migrator().CreateTable(&models.MembershipLevel{}))

	return func() {}
}

func TestCheckAndUpgradeLevel_Aggregation(t *testing.T) {
	cleanup := setupMembershipTestDB(t)
	defer cleanup()
	db := database.GetDB()

	// Levels: 初级 0, 中级 5000, 高级 10000
	require.NoError(t, db.Create(&models.MembershipLevel{ID: 1, Name: "初级", MinAmount: 0}).Error)
	require.NoError(t, db.Create(&models.MembershipLevel{ID: 2, Name: "中级", MinAmount: 5000}).Error)
	require.NoError(t, db.Create(&models.MembershipLevel{ID: 3, Name: "高级", MinAmount: 10000}).Error)

	userID := "00000000-0000-0000-0000-0000000000d1"
	tenantID := "00000000-0000-0000-0000-0000000000d2"
	orgID := "00000000-0000-0000-0000-0000000000d3"
	level1 := 1
	user := models.User{
		ID:                userID,
		IAMSub:            userID,
		TenantID:          tenantID,
		OrgID:             orgID,
		MembershipLevelID: &level1,
	}
	require.NoError(t, db.Create(&user).Error)

	t.Run("4000_spending_no_upgrade", func(t *testing.T) {
		// prepaid purchase 1000 + repair payment 3000 = 4000 < 5000
		require.NoError(t, db.Create(&models.OrderPaymentRecord{
			ID: "00000000-0000-0000-0000-0000000000e1", UserID: userID, TenantID: tenantID,
			OrderType: "points", Type: "payment", Status: "paid", Amount: 1000,
		}).Error)
		require.NoError(t, db.Create(&models.OrderPaymentRecord{
			ID: "00000000-0000-0000-0000-0000000000e2", UserID: userID, TenantID: tenantID,
			OrderType: "repair", Type: "payment", Status: "paid", Amount: 3000,
		}).Error)

		require.NoError(t, CheckAndUpgradeLevel(userID, db))
		var after models.User
		require.NoError(t, db.First(&after, "id = ?", userID).Error)
		require.Equal(t, 1, *after.MembershipLevelID, "4000 < 5000, stays 初级")
	})

	t.Run("5000_spending_upgrade_to_intermediate", func(t *testing.T) {
		// rent settlement 1000 → total 5000 → upgrade to 中级
		instrument := models.Instrument{ID: "00000000-0000-0000-0000-0000000000f1", TenantID: tenantID, SN: "MBR-1"}
		require.NoError(t, db.Create(&instrument).Error)
		order := models.Order{
			ID: "00000000-0000-0000-0000-0000000000f2", UserID: userID, TenantID: tenantID, OrgID: orgID,
			InstrumentID: instrument.ID, Status: "completed",
		}
		require.NoError(t, db.Create(&order).Error)
		require.NoError(t, db.Create(&models.Settlement{
			ID: "00000000-0000-0000-0000-0000000000f3", OrderID: order.ID, ActualRentAmount: 1000,
		}).Error)

		require.NoError(t, CheckAndUpgradeLevel(userID, db))
		var after models.User
		require.NoError(t, db.First(&after, "id = ?", userID).Error)
		require.Equal(t, 2, *after.MembershipLevelID, "5000 >= 5000, upgrades to 中级")
	})

	t.Run("refund_does_not_downgrade", func(t *testing.T) {
		// refund 200 → 4800 < 5000 but level stays 中级 (only-up)
		payment := models.OrderPaymentRecord{
			ID: "00000000-0000-0000-0000-0000000000e3", UserID: userID, TenantID: tenantID, OrgID: &orgID,
			OrderType: "repair", Type: "payment", Status: "paid", Amount: 200,
		}
		require.NoError(t, db.Create(&payment).Error)
		require.NoError(t, db.Create(&models.OrderRefundRecord{
			ID: "00000000-0000-0000-0000-0000000000e4", TenantID: tenantID, PaymentRecordID: &payment.ID,
			Status: "refunded", Amount: 200,
		}).Error)

		require.NoError(t, CheckAndUpgradeLevel(userID, db))
		var after models.User
		require.NoError(t, db.First(&after, "id = ?", userID).Error)
		require.Equal(t, 2, *after.MembershipLevelID, "refund reduces spend but level only upgrades")
	})
}
