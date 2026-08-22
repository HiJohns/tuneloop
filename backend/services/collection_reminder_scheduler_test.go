package services

import (
	"testing"
	"time"

	"encoding/json"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

// #1749 L-04D：补缴超时催缴 Ticker——扫描（24h/状态/已提醒排除）、
// 通知（商户+网点管理员）、幂等（不重复报警）、不改订单状态。

// setupReminderTestDB 建 #1749 所需表（services 层独立测试基建，防 import cycle）。
func setupReminderTestDB(t *testing.T) {
	t.Helper()
	config := database.LoadConfig()
	db, err := database.InitDB(config)
	if err != nil {
		t.Skip("test database not available")
	}
	database.SetDB(db)

	for _, m := range []interface{}{
		&models.User{}, &models.Instrument{}, &models.Order{},
		&models.OrderPaymentRecord{}, &models.Notification{}, &models.SiteMember{},
		&models.Site{}, &models.OrderLog{},
	} {
		_ = db.Migrator().DropTable(m)
		require.NoError(t, db.Migrator().CreateTable(m))
	}
	db.Exec("ALTER TABLE users ADD COLUMN IF NOT EXISTS iam_sub VARCHAR(255) NOT NULL DEFAULT ''")
	db.Exec("ALTER TABLE order_payment_records ADD COLUMN IF NOT EXISTS reminded_at TIMESTAMP")
}

// TestReminderScan_FiltersOverduePending: 仅扫描 pending + 超 24h + 未提醒。
func TestReminderScan_FiltersOverduePending(t *testing.T) {
	setupReminderTestDB(t)
	db := database.GetDB()

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "reminder-1", Status: "active", Name: "催缴顾客", Phone: "13800138000",
	}).Error)

	orderID := uuid.New().String()
	require.NoError(t, db.Create(&models.Order{
		ID: orderID, TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: uuid.New().String(), Status: models.OrderStatusReturning,
	}).Error)

	mkRecord := func(hoursAgo int, status string, reminded bool) models.OrderPaymentRecord {
		rec := models.OrderPaymentRecord{
			ID:        uuid.New().String(),
			TenantID:  tenantID,
			OrgID:     &orgID,
			UserID:    userID,
			OrderID:   &orderID,
			OrderType: "payment_shortfall",
			Amount:    models.FromYuan(150),
			Type:      "payment",
			Status:    status,
			CreatedAt: time.Now().Add(-time.Duration(hoursAgo) * time.Hour),
		}
		if reminded {
			now := time.Now()
			rec.RemindedAt = &now
		}
		require.NoError(t, db.Create(&rec).Error)
		return rec
	}

	// 25h pending 未提醒 → 应被扫描
	target := mkRecord(25, "pending", false)
	// 23h pending → 未超阈值 → 跳过
	mkRecord(23, "pending", false)
	// 25h pending 已提醒 → 跳过（幂等）
	mkRecord(25, "pending", true)
	// 25h paid → 状态过滤 → 跳过
	mkRecord(25, "paid", false)

	s := &CollectionReminderScheduler{db: db}
	require.NoError(t, s.process())

	// 只有 target 被标记 reminded
	var rec models.OrderPaymentRecord
	require.NoError(t, db.Where("id = ?", target.ID).First(&rec).Error)
	require.NotNil(t, rec.RemindedAt, "超时未提醒记录必须被标记")

	var remindedCount int64
	db.Model(&models.OrderPaymentRecord{}).
		Where("order_type = ? AND reminded_at IS NOT NULL", "payment_shortfall").Count(&remindedCount)
	// 2 = 预置的已提醒记录 + process 标记的 target（幂等：target 未被重复标记）
	require.Equal(t, int64(2), remindedCount, "target 被标记，预置已提醒未被重复处理")
}

// TestReminderNotification_TargetsAdmins: 催缴通知发给商户管理员 + 网点管理员。
func TestReminderNotification_TargetsAdmins(t *testing.T) {
	setupReminderTestDB(t)
	db := database.GetDB()

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	siteID := uuid.New().String()
	siteUUID := uuid.MustParse(siteID) // instrument.SiteID 与 site_members.site_id 必须同值

	// 顾客
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "reminder-cust", Status: "active", Name: "王顾客", Phone: "13900139000",
	}).Error)
	// 商户管理员（OWNER）
	adminID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: adminID, IAMSub: adminID, TenantID: tenantID, OrgID: orgID,
		Username: "reminder-admin", Status: "active", Role: "OWNER",
	}).Error)
	// 网点管理员
	staffID := uuid.New().String()
	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.New().String(), TenantID: tenantID, SiteID: siteID, UserID: staffID,
		Role: "site_admin", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.Site{
		ID: siteID, TenantID: tenantID, OrgID: orgID, Name: "测试网点",
	}).Error)

	orderID := uuid.New().String()
	instrumentID := uuid.New().String()
	require.NoError(t, db.Create(&models.Order{
		ID: orderID, TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: instrumentID, Status: models.OrderStatusReturning,
	}).Error)
	require.NoError(t, db.Create(&models.Instrument{
		ID: instrumentID, TenantID: tenantID, OrgID: &orgID,
		SN: "SN-REMINDER", SiteID: &siteUUID, StockStatus: "available",
	}).Error)
	outTradeNo := "shortfall_abcdef"
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &orderID, OrderType: "payment_shortfall",
		Amount: models.FromYuan(150), OutTradeNo: &outTradeNo,
		Type: "payment", Status: "pending",
		CreatedAt: time.Now().Add(-30 * time.Hour),
	}).Error)

	s := &CollectionReminderScheduler{db: db}
	require.NoError(t, s.process())

	// 商户管理员收到
	var adminNotif models.Notification
	require.NoError(t, db.Where("user_id = ? AND action_type = ?", adminID, "collect_reminder").First(&adminNotif).Error)
	require.Equal(t, "补缴超时催缴", adminNotif.Title)
	require.Contains(t, adminNotif.Content, "王顾客", "content 含顾客姓名")
	require.Contains(t, adminNotif.Content, "13900139000", "content 含顾客手机")
	require.NotNil(t, adminNotif.ActionData)
	var ad struct {
		CustomerName    string `json:"customer_name"`
		CustomerPhone   string `json:"customer_phone"`
		ShortfallAmount int64  `json:"shortfall_amount"`
		OrderID         string `json:"order_id"`
		OutTradeNo      string `json:"out_trade_no"`
	}
	require.NoError(t, json.Unmarshal([]byte(*adminNotif.ActionData), &ad))
	require.Equal(t, "王顾客", ad.CustomerName)
	require.Equal(t, "13900139000", ad.CustomerPhone)
	require.Equal(t, int64(15000), ad.ShortfallAmount, "分单位")
	require.Equal(t, orderID, ad.OrderID)
	require.Equal(t, "shortfall_abcdef", ad.OutTradeNo)

	// 网点管理员收到
	var staffNotif models.Notification
	require.NoError(t, db.Where("user_id = ? AND action_type = ?", staffID, "collect_reminder").First(&staffNotif).Error)

	// 订单日志留痕
	var logCount int64
	db.Model(&models.OrderLog{}).Where("order_id = ? AND event = ?", orderID, "补缴超时催缴").Count(&logCount)
	require.Equal(t, int64(1), logCount, "订单日志留痕")

	// L-04D 关键规则：催缴不改订单状态（保持 returning）
	var after models.Order
	require.NoError(t, db.Where("id = ?", orderID).First(&after).Error)
	require.Equal(t, models.OrderStatusReturning, after.Status)
}

// TestReminderIdempotent: 再次 process 不重复报警（reminded_at 已标记）。
func TestReminderIdempotent(t *testing.T) {
	setupReminderTestDB(t)
	db := database.GetDB()

	tenantID := uuid.New().String()
	orgID := tenantID
	userID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: userID, IAMSub: userID, TenantID: tenantID, OrgID: orgID,
		Username: "reminder-idem", Status: "active",
	}).Error)

	orderID := uuid.New().String()
	require.NoError(t, db.Create(&models.Order{
		ID: orderID, TenantID: tenantID, OrgID: orgID, UserID: userID,
		InstrumentID: uuid.New().String(), Status: models.OrderStatusReturning,
	}).Error)
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: &orgID, UserID: userID,
		OrderID: &orderID, OrderType: "payment_shortfall",
		Amount: models.FromYuan(150), Type: "payment", Status: "pending",
		CreatedAt: time.Now().Add(-30 * time.Hour),
	}).Error)

	s := &CollectionReminderScheduler{db: db}
	require.NoError(t, s.process())
	require.NoError(t, s.process()) // 第二次：全部已提醒 → 无新通知

	var notifCount int64
	db.Model(&models.Notification{}).Where("action_type = ?", "collect_reminder").Count(&notifCount)
	require.Equal(t, int64(0), notifCount, "无管理员时通知数 0（本测试未建管理员）")

	var remindedCount int64
	db.Model(&models.OrderPaymentRecord{}).Where("reminded_at IS NOT NULL").Count(&remindedCount)
	require.Equal(t, int64(1), remindedCount, "仅提醒一次")
}
