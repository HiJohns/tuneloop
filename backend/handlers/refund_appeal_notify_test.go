package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

// setupRefundNotifyTables prepares the tables needed by the refund-receipt
// and appeal notification regression tests (#1602/#1603/#1604).
func setupRefundNotifyTables(t *testing.T) *gorm.DB {
	cleanup := setupMockIAMAndDB(t)
	t.Cleanup(cleanup)
	db := database.GetDB()
	for _, m := range []interface{}{
		&models.Instrument{},
		&models.Order{},
		&models.DamageAssessment{},
		&models.DamageReport{},
		&models.Appeal{},
		&models.Notification{},
		&models.OrderStatusHistory{},
		&models.Settlement{},
		&models.OrderPaymentRecord{},
		&models.OrderRefundRecord{},
		&models.LeaseSession{},
		&models.SiteMember{},
		&models.User{},
	} {
		_ = db.Migrator().DropTable(m)
		require.NoError(t, db.Migrator().CreateTable(m))
	}
	db.Exec("ALTER TABLE users ADD COLUMN IF NOT EXISTS iam_sub VARCHAR(255) NOT NULL DEFAULT ''")
	return db
}

// TestInspectReturn_Damaged_NotificationActionType verifies #1602: the
// damaged return-inspect notification must carry
// actionType='damage_accept_reject' + ref_type='damage_report' +
// ref_id=assessment.ID so MessageDetail can render accept/reject buttons.
func TestInspectReturn_Damaged_NotificationActionType(t *testing.T) {
	db := setupRefundNotifyTables(t)
	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	userID := uuid.New().String()
	staffID := uuid.New().String()
	now := time.Now()

	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "SN-DMG-" + now.Format("150405"),
		BaseDailyRate: float64Ptr(100),
		StockStatus:   "rented",
	}
	require.NoError(t, db.Create(&instrument).Error)

	start := "2026-08-01"
	end := "2026-08-30"
	order := models.Order{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		OrgID:        orgID,
		UserID:       userID,
		InstrumentID: instrument.ID,
		Status:       models.OrderStatusReturning,
		StartDate:    &start,
		EndDate:      &end,
		LeaseTerm:    30,
		Deposit:      500,
		CashPaid:     3500,
	}
	require.NoError(t, db.Create(&order).Error)

	handler := NewWarehouseHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, staffID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/warehouse/orders/:id/return-inspect", handler.InspectReturn)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"instrument_sn": instrument.SN,
		"scan_time":     now.Format(time.RFC3339),
		"condition":     "damaged",
		"notes":         "琴面刮痕",
		"damage_amount": 200.0,
	})
	req := httptest.NewRequest("PUT", "/api/warehouse/orders/"+order.ID+"/return-inspect", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			AssessmentID string `json:"assessment_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)

	var notif models.Notification
	require.NoError(t, db.Where("user_id = ? AND ref_type = ?", userID, "damage_report").
		Order("created_at desc").First(&notif).Error)
	require.Equal(t, "damage_accept_reject", notif.ActionType, "actionType drives MessageDetail accept/reject buttons")
	require.Equal(t, "damage_report", notif.RefType)
	// ref_id must point to a real damage_reports row (MessageDetail loads
	// the report by id to render accept/reject buttons) (#1607, L-04)
	var report models.DamageReport
	require.NoError(t, db.Where("id = ?", notif.RefID).First(&report).Error, "ref_id resolves to a damage report")
	require.NotNil(t, notif.ActionData)
	require.Contains(t, *notif.ActionData, `"damage_amount": 200.00`)
	require.Contains(t, *notif.ActionData, `"order_id"`)
}

// TestInspectReturn_Good_RefundReceiptNotification verifies #1603: a good
// return-inspect completes the order and the customer notification carries
// the standard receipt breakdown including the renewal fee line.
func TestInspectReturn_Good_RefundReceiptNotification(t *testing.T) {
	db := setupRefundNotifyTables(t)
	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	userID := uuid.New().String()
	staffID := uuid.New().String()
	now := time.Now()

	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "SN-GD-" + now.Format("150405"),
		BaseDailyRate: float64Ptr(100),
		StockStatus:   "rented",
	}
	require.NoError(t, db.Create(&instrument).Error)

	start := "2026-08-01"
	end := "2026-08-30"
	order := models.Order{
		ID:               uuid.New().String(),
		TenantID:         tenantID,
		OrgID:            orgID,
		UserID:           userID,
		InstrumentID:     instrument.ID,
		Status:           models.OrderStatusReturning,
		StartDate:        &start,
		EndDate:          &end,
		LeaseTerm:        30,
		Deposit:          500,
		CashPaid:         3500,
		PricingBreakdown: strPtr(`{"base_daily_rent":100,"rent_days":30,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":100}],"tier_segments":[{"tier":1,"days":30,"rate":100,"discount":1,"subtotal":3000}],"total_amount":3000}`),
	}
	require.NoError(t, db.Create(&order).Error)

	// Renewal payment record → receipt must include the renewal fee line.
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		UserID:    userID,
		OrderID:   &order.ID,
		OrderType: "renewal",
		Amount:    120,
		Type:      "payment",
		Status:    "paid",
		CreatedAt: time.Now(),
	}).Error)

	handler := NewWarehouseHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, staffID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/warehouse/orders/:id/return-inspect", handler.InspectReturn)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"instrument_sn": instrument.SN,
		"scan_time":     now.Format(time.RFC3339),
		"condition":     "good",
		"notes":         "无损",
	})
	req := httptest.NewRequest("PUT", "/api/warehouse/orders/"+order.ID+"/return-inspect", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var notif models.Notification
	require.NoError(t, db.Where("user_id = ? AND type = ?", userID, "order").
		Order("created_at desc").First(&notif).Error)
	require.Contains(t, notif.Content, "租赁结算明细")
	require.Contains(t, notif.Content, "租金：¥3000.00")
	require.Contains(t, notif.Content, "续期费用：¥120.00")
	require.Contains(t, notif.Content, "押金退还：¥500.00")
	require.Contains(t, notif.Content, "退回微信：¥500.00")
	require.Contains(t, notif.Content, "实际退款合计：¥500.00")
}

// TestBuildRefundReceipt_IncludesAllLines is a pure unit test of
// buildRefundReceipt covering every optional line (shipping/overdue/damage/
// renewal) plus the totals block (#1603).
func TestBuildRefundReceipt_IncludesAllLines(t *testing.T) {
	db := setupRefundNotifyTables(t)
	tenantID := uuid.New().String()
	userID := uuid.New().String()

	order := models.Order{
		ID:             uuid.New().String(),
		TenantID:       tenantID,
		OrgID:          uuid.New().String(),
		UserID:         userID,
		InstrumentID:   uuid.New().String(),
		Deposit:        500,
		CashPaid:       3500,
		ShippingFee:    50,
		GiftPointsUsed: 100,
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&models.OrderPaymentRecord{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		UserID:    userID,
		OrderID:   &order.ID,
		OrderType: "renewal",
		Amount:    120,
		Type:      "payment",
		Status:    "paid",
		CreatedAt: time.Now(),
	}).Error)

	s := &settlementResult{
		RentPayable:         3000,
		TotalRentPaid:       3000,
		RemainingDeposit:    500,
		DamageDeducted:      100,
		OverdueChargesTotal: 30,
		TotalRefund:         400,
		CashRefundable:      400,
		ActualDays:          30,
	}
	receipt := buildRefundReceipt(db, order, s)

	require.Contains(t, receipt, "租赁结算明细")
	require.Contains(t, receipt, "实际租期：30 天")
	require.Contains(t, receipt, "租金：¥3000.00")
	require.Contains(t, receipt, "物流费：¥50.00")
	require.Contains(t, receipt, "逾期费：¥30.00")
	require.Contains(t, receipt, "损坏赔偿：¥100.00")
	require.Contains(t, receipt, "续期费用：¥120.00")
	require.Contains(t, receipt, "应付合计：¥3530.00")
	require.Contains(t, receipt, "已收（含押金）：¥4100.00")
	require.Contains(t, receipt, "押金退还：¥500.00")
	require.Contains(t, receipt, "退回微信：¥400.00")
	require.Contains(t, receipt, "实际退款合计：¥400.00")
	require.Contains(t, receipt, "返点赠点到账")
}

// TestSubmitAppeal_StaffNotification verifies #1604: submitting an appeal
// notifies site staff (site_admin/site_member) with
// actionType='repair_request' + ref_type='appeal' + ref_id=appeal.ID.
func TestSubmitAppeal_StaffNotification(t *testing.T) {
	db := setupRefundNotifyTables(t)
	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	userID := uuid.New().String()
	siteID := uuid.New()
	staffAdmin := uuid.New().String()
	staffMember := uuid.New().String()

	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "SN-AP-" + time.Now().Format("150405"),
		BaseDailyRate: float64Ptr(100),
		SiteID:        &siteID,
		StockStatus:   "maintenance",
	}
	require.NoError(t, db.Create(&instrument).Error)

	order := models.Order{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		OrgID:        orgID,
		UserID:       userID,
		InstrumentID: instrument.ID,
		Status:       models.OrderStatusReturning,
	}
	require.NoError(t, db.Create(&order).Error)

	damageAmount := 200.0
	damageReport := models.DamageReport{
		ID:                uuid.New().String(),
		TenantID:          tenantID,
		OrgID:             orgID,
		LeaseID:           order.ID,
		InstrumentID:      instrument.ID,
		UserID:            userID,
		DamageAmount:      &damageAmount,
		DamageDescription: "琴面刮痕",
		Status:            "pending",
	}
	require.NoError(t, db.Create(&damageReport).Error)

	for _, sm := range []models.SiteMember{
		{ID: uuid.New().String(), TenantID: tenantID, SiteID: siteID.String(), UserID: staffAdmin, Role: "site_admin", Status: "active"},
		{ID: uuid.New().String(), TenantID: tenantID, SiteID: siteID.String(), UserID: staffMember, Role: "site_member", Status: "active"},
	} {
		require.NoError(t, db.Create(&sm).Error)
	}
	require.NoError(t, db.Create(&models.Site{
		ID: siteID.String(), TenantID: tenantID, OrgID: orgID, Name: "测试网点",
	}).Error)

	handler := NewAppealHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/user/appeals", handler.SubmitAppeal)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"damage_report_id": damageReport.ID,
		"appeal_reason":    "琴面刮痕为原有损伤",
	})
	req := httptest.NewRequest("POST", "/api/user/appeals", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)

	// Both staff roles must receive the "新申诉待处理" notification.
	for _, staffID := range []string{staffAdmin, staffMember} {
		var notif models.Notification
		require.NoError(t, db.Where("user_id = ? AND ref_type = ?", staffID, "appeal").
			Order("created_at desc").First(&notif).Error)
		require.Equal(t, "新申诉待处理", notif.Title)
		require.Equal(t, "repair_request", notif.ActionType, "staff notification actionType drives 查看申诉详情 button")
		require.Equal(t, "appeal", notif.RefType)
		require.Equal(t, resp.Data.ID, notif.RefID, "ref_id must point to the appeal")
		require.NotNil(t, notif.ActionData)
		require.Contains(t, *notif.ActionData, `"appeal_id"`)
	}
}

// TestResolveAppeal_Final_RefundReceiptAndStaffNotify verifies #1603+#1604:
// final resolve (confirm, amount > deposit → order completed) sends the
// customer the receipt breakdown and notifies site staff with
// actionType='order' + ref_type='order'.
func TestResolveAppeal_Final_RefundReceiptAndStaffNotify(t *testing.T) {
	db := setupRefundNotifyTables(t)
	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	userID := uuid.New().String()
	managerID := uuid.New().String()
	siteID := uuid.New()
	staffMember := uuid.New().String()

	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "SN-RS-" + time.Now().Format("150405"),
		BaseDailyRate: float64Ptr(100),
		SiteID:        &siteID,
		StockStatus:   "maintenance",
	}
	require.NoError(t, db.Create(&instrument).Error)

	start := "2026-08-01"
	end := "2026-08-30"
	order := models.Order{
		ID:               uuid.New().String(),
		TenantID:         tenantID,
		OrgID:            orgID,
		UserID:           userID,
		InstrumentID:     instrument.ID,
		Status:           models.OrderStatusDamageAppealing,
		StartDate:        &start,
		EndDate:          &end,
		LeaseTerm:        30,
		Deposit:          500,
		CashPaid:         3500,
		PricingBreakdown: strPtr(`{"base_daily_rent":100,"rent_days":30,"tiers":[{"days_max":30,"discount_percent":0,"daily_rate":100}],"tier_segments":[{"tier":1,"days":30,"rate":100,"discount":1,"subtotal":3000}],"total_amount":3000}`),
	}
	require.NoError(t, db.Create(&order).Error)

	damageAmount := 600.0 // > deposit 500 → confirm completes the order
	damageReport := models.DamageReport{
		ID:                uuid.New().String(),
		TenantID:          tenantID,
		OrgID:             orgID,
		LeaseID:           order.ID,
		InstrumentID:      instrument.ID,
		UserID:            userID,
		DamageAmount:      &damageAmount,
		DamageDescription: "琴面刮痕",
		Status:            "appealed",
	}
	require.NoError(t, db.Create(&damageReport).Error)

	appeal := models.Appeal{
		ID:             uuid.New().String(),
		TenantID:       tenantID,
		OrgID:          orgID,
		SiteID:         siteID.String(),
		Category:       "damage",
		ObjectType:     "damage_report",
		ObjectID:       damageReport.ID,
		AppellantID:    userID,
		DamageReportID: &damageReport.ID,
		UserID:         &userID,
		Status:         "pending",
		SubmittedAt:    time.Now(),
	}
	require.NoError(t, db.Create(&appeal).Error)

	require.NoError(t, db.Create(&models.SiteMember{
		ID: uuid.New().String(), TenantID: tenantID, SiteID: siteID.String(),
		UserID: staffMember, Role: "site_member", Status: "active",
	}).Error)
	require.NoError(t, db.Create(&models.Site{
		ID: siteID.String(), TenantID: tenantID, OrgID: orgID, Name: "测试网点",
	}).Error)

	handler := NewAppealHandler()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, managerID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.PUT("/api/merchant/appeals/:id/resolve", handler.ResolveAppeal)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"decision": "confirm",
		"comment":  "定损成立",
	})
	req := httptest.NewRequest("PUT", "/api/merchant/appeals/"+appeal.ID+"/resolve", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	// Customer notification carries the refund receipt.
	var customerNotif models.Notification
	require.NoError(t, db.Where("user_id = ? AND ref_type = ?", userID, "appeal").
		Order("created_at desc").First(&customerNotif).Error)
	require.Contains(t, customerNotif.Content, "租赁结算明细")
	require.Contains(t, customerNotif.Content, "实际退款")

	// Staff notification: actionType='order' → 查看退款详情 button.
	var staffNotif models.Notification
	require.NoError(t, db.Where("user_id = ? AND title = ?", staffMember, "申诉终审：退款完成").
		Order("created_at desc").First(&staffNotif).Error)
	require.Equal(t, "order", staffNotif.ActionType)
	require.Equal(t, "order", staffNotif.RefType)
	require.Equal(t, order.ID, staffNotif.RefID)
	require.NotNil(t, staffNotif.ActionData)
	require.Contains(t, *staffNotif.ActionData, `"order_id"`)
}
