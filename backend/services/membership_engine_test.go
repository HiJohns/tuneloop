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
	_ = db.Migrator().DropTable(&models.Appeal{})
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
	require.NoError(t, db.Migrator().CreateTable(&models.Appeal{}))

	return func() {}
}

func TestCheckAndUpgradeLevel_Aggregation(t *testing.T) {
	cleanup := setupMembershipTestDB(t)
	defer cleanup()
	db := database.GetDB()

	// Levels: 乐手 0, 首席 500000 分, 演奏家 1000000 分（#1939 Sub-C 对齐手册）
	require.NoError(t, db.Create(&models.MembershipLevel{ID: 1, Name: "乐手", MinAmount: 0}).Error)
	require.NoError(t, db.Create(&models.MembershipLevel{ID: 2, Name: "首席", MinAmount: 5000}).Error)
	require.NoError(t, db.Create(&models.MembershipLevel{ID: 3, Name: "演奏家", MinAmount: 10000}).Error)

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
		require.Equal(t, 1, *after.MembershipLevelID, "4000 < 500000, stays 乐手")
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
			ID: "00000000-0000-0000-0000-0000000000f3", OrderID: order.ID, ActualRentAmount: models.Cents(1000),
		}).Error)

		require.NoError(t, CheckAndUpgradeLevel(userID, db))
		var after models.User
		require.NoError(t, db.First(&after, "id = ?", userID).Error)
		require.Equal(t, 2, *after.MembershipLevelID, "5000 >= 5000, upgrades to 首席")
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

// #1946（#1939 Sub-C）：晋升违约校验（M-08 四项，任一命中阻止晋升）。
func TestCheckAndUpgradeLevel_DefaultsBlock(t *testing.T) {
	cleanup := setupMembershipTestDB(t)
	defer cleanup()
	db := database.GetDB()

	require.NoError(t, db.Create(&models.MembershipLevel{ID: 1, Name: "乐手", MinAmount: 0}).Error)
	require.NoError(t, db.Create(&models.MembershipLevel{ID: 2, Name: "首席", MinAmount: 5000}).Error)

	userID := "00000000-0000-0000-0000-0000000001a1"
	tenantID := "00000000-0000-0000-0000-0000000001a2"
	orgID := "00000000-0000-0000-0000-0000000001a3"
	level1 := 1
	user := models.User{ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID, MembershipLevelID: &level1}
	require.NoError(t, db.Create(&user).Error)

	instrument := models.Instrument{ID: "00000000-0000-0000-0000-0000000001b1", TenantID: tenantID, SN: "MBR-DEF"}
	require.NoError(t, db.Create(&instrument).Error)
	mkOrder := func(id, status string) *models.Order {
		o := &models.Order{
			ID: id, UserID: userID, TenantID: tenantID, OrgID: orgID,
			InstrumentID: instrument.ID, Status: status,
		}
		require.NoError(t, db.Create(o).Error)
		return o
	}

	// 消费达标（6 个月租结算 6000 分 ≥ 首席 5000 分）——若无违约应晋升
	require.NoError(t, db.Create(&models.Settlement{
		ID: "00000000-0000-0000-0000-0000000001c1", OrderID: "00000000-0000-0000-0000-0000000001c0",
		ActualRentAmount: models.Cents(6000),
	}).Error)
	mkOrder("00000000-0000-0000-0000-0000000001c0", "completed")

	assertBlocked := func(name string) {
		t.Helper()
		require.NoError(t, CheckAndUpgradeLevel(userID, db))
		var after models.User
		require.NoError(t, db.First(&after, "id = ?", userID).Error)
		require.Equal(t, 1, *after.MembershipLevelID, name+" 应阻止晋升")
	}

	// 违约 1：逾期未归还
	mkOrder("00000000-0000-0000-0000-0000000001d1", "expired")
	assertBlocked("orders.status=expired")
	require.NoError(t, db.Model(&models.Order{}).Where("id = ?", "00000000-0000-0000-0000-0000000001d1").Update("status", "completed").Error)

	// 违约 2：补缴未支付（repair 类 pending，即维修服务 dispatch 补缴）
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: "00000000-0000-0000-0000-0000000001d2", UserID: userID, TenantID: tenantID,
		OrderType: "repair", Type: "payment", Status: "pending", Amount: 3000,
	}).Error)
	assertBlocked("payment pending repair")
	// 结清后解除
	require.NoError(t, db.Model(&models.OrderPaymentRecord{}).Where("id = ?", "00000000-0000-0000-0000-0000000001d2").Update("status", "paid").Error)
	require.NoError(t, CheckAndUpgradeLevel(userID, db))
	var mid models.User
	require.NoError(t, db.First(&mid, "id = ?", userID).Error)
	require.Equal(t, 2, *mid.MembershipLevelID, "违约解除后可晋升")

	// 回退等级以便后续用例验证（只升不降——改用新用户验证剩余违约项）
	userID2 := "00000000-0000-0000-0000-0000000001e1"
	user2 := models.User{ID: userID2, IAMSub: userID2, TenantID: tenantID, OrgID: orgID, MembershipLevelID: &level1}
	require.NoError(t, db.Create(&user2).Error)
	// user2 自己的已完成订单 + 租金结算 6000 分（≥ 首席 5000 分）
	require.NoError(t, db.Create(&models.Order{
		ID: "00000000-0000-0000-0000-0000000001e3", UserID: userID2, TenantID: tenantID, OrgID: orgID,
		InstrumentID: instrument.ID, Status: "completed",
	}).Error)
	require.NoError(t, db.Create(&models.Settlement{
		ID: "00000000-0000-0000-0000-0000000001e2", OrderID: "00000000-0000-0000-0000-0000000001e3",
		ActualRentAmount: models.Cents(6000),
	}).Error)

	// 违约 3：损坏流程未结（user2 自己的订单）
	require.NoError(t, db.Create(&models.Order{
		ID: "00000000-0000-0000-0000-0000000001d3", UserID: userID2, TenantID: tenantID, OrgID: orgID,
		InstrumentID: instrument.ID, Status: "pending_damage_response",
	}).Error)
	require.NoError(t, CheckAndUpgradeLevel(userID2, db))
	var after3 models.User
	require.NoError(t, db.First(&after3, "id = ?", userID2).Error)
	require.Equal(t, 1, *after3.MembershipLevelID, "pending_damage_response 应阻止晋升")
	require.NoError(t, db.Model(&models.Order{}).Where("id = ?", "00000000-0000-0000-0000-0000000001d3").Update("status", "completed").Error)

	// 违约 4：申诉未决（appellant_id = IAM sub）
	require.NoError(t, db.Create(&models.Appeal{
		ID: "00000000-0000-0000-0000-0000000001d4", TenantID: tenantID, OrgID: orgID, SiteID: orgID,
		AppellantID: userID2, Category: "damage", ObjectType: "order",
		ObjectID: "00000000-0000-0000-0000-0000000001c0", Status: "pending",
	}).Error)
	require.NoError(t, CheckAndUpgradeLevel(userID2, db))
	var after4 models.User
	require.NoError(t, db.First(&after4, "id = ?", userID2).Error)
	require.Equal(t, 1, *after4.MembershipLevelID, "appeals pending 应阻止晋升")
	// 申诉结决后解除
	require.NoError(t, db.Model(&models.Appeal{}).Where("id = ?", "00000000-0000-0000-0000-0000000001d4").Update("status", "resolved").Error)
	require.NoError(t, CheckAndUpgradeLevel(userID2, db))
	var after5 models.User
	require.NoError(t, db.First(&after5, "id = ?", userID2).Error)
	require.Equal(t, 2, *after5.MembershipLevelID, "违约解除后晋升至首席")
}
