package handlers

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

// #1802 T1: 时序验证测试 — 续费天数从独立 Days 列读取。
//
// 背景：续费天数（AdditionalDays）原存 OrderPaymentRecord.RawResponse，但
// processPaymentCallback（wechatpay_callback.go）在 applyRenewalSideEffects
// 之前把 RawResponse 覆盖为微信回调结果 → 真实回调路径续费天数丢失，
// applyRenewalSideEffects 报 "invalid renewal metadata"，订单续期失败。
//
// 本测试模拟真实回调时序：
//   1. ConfirmRenewal 创建 payment record（Days 列 + RawResponse=meta）
//   2. processPaymentCallback 覆盖 RawResponse 为回调结果（Days 列保留）
//   3. applyRenewalSideEffects 必须仍能从 Days 列读到续费天数并正确续期

func TestApplyRenewalSideEffects_DaysSurvivesCallbackOverwrite(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()

	db := database.GetDB()
	if !db.Migrator().HasTable(&models.OrderPaymentRecord{}) {
		require.NoError(t, db.Migrator().CreateTable(&models.OrderPaymentRecord{}))
	}
	tenantID := "00000000-0000-4000-8000-0000000000f1"
	userID := "00000000-0000-4000-8000-0000000000f2"
	orgID := "00000000-0000-4000-8000-0000000000f3"

	// Order: ends today+5, renewal of 10 days → new end = today+15.
	_, orderID := setupRenewalOrder(t, tenantID, userID, orgID, 5)

	additionalDays := 10
	outTradeNo := "RN-CB-" + uuid.New().String()[:12]
	meta := renewalMetadata{
		AdditionalDays: additionalDays,
		OrderID:        orderID,
		OutTradeNo:     outTradeNo,
	}
	metaJSON, _ := json.Marshal(meta)
	metaStr := string(metaJSON)

	// Step 1: ConfirmRenewal-like record — Days column populated.
	record := models.OrderPaymentRecord{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		OrgID:       &orgID,
		UserID:      userID,
		OrderID:     &orderID,
		OrderType:   "renewal",
		OutTradeNo:  &meta.OutTradeNo,
		Amount:      models.Cents(1000),
		Type:        "payment",
		Status:      "paid",
		RawResponse: &metaStr,
		Days:        &additionalDays,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, db.Create(&record).Error)

	// Step 2: processPaymentCallback overwrites RawResponse with the callback
	// result (wechatpay_callback.go:147) — Days column survives untouched.
	cbRaw, _ := json.Marshal(map[string]interface{}{
		"out_trade_no":   meta.OutTradeNo,
		"transaction_id": "wxcb-overwrite-test",
		"amount":         1000,
		"trade_state":    "SUCCESS",
	})
	cbStr := string(cbRaw)
	require.NoError(t, db.Model(&record).Update("raw_response", &cbStr).Error)

	// Reload record — Days must still be present.
	var reloaded models.OrderPaymentRecord
	require.NoError(t, db.Where("id = ?", record.ID).First(&reloaded).Error)
	require.NotNil(t, reloaded.Days, "Days column survives callback RawResponse overwrite")
	require.Equal(t, additionalDays, *reloaded.Days)

	// Step 3: applyRenewalSideEffects must extend the order end date by 10 days.
	tx := db.Begin()
	err := applyRenewalSideEffects(tx, &reloaded, time.Now())
	if err != nil {
		tx.Rollback()
		t.Fatalf("applyRenewalSideEffects failed after callback overwrite: %v", err)
	}
	tx.Commit()

	var order models.Order
	require.NoError(t, db.Where("id = ?", orderID).First(&order).Error)
	expectedEnd := time.Now().AddDate(0, 0, 5+additionalDays).Format("2006-01-02")
	require.NotNil(t, order.EndDate, "order end date set")
	require.Equal(t, expectedEnd, (*order.EndDate)[:10],
		"order end date extended by renewal days read from Days column")
	require.Equal(t, models.OrderStatusInLease, order.Status)
}

// #1802 T1: 历史数据 fallback — 旧记录无 Days 列时从 RawResponse meta 读取。
func TestApplyRenewalSideEffects_DaysFallbackToRawResponseMeta(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()

	db := database.GetDB()
	tenantID := "00000000-0000-4000-8000-0000000000f4"
	userID := "00000000-0000-4000-8000-0000000000f5"
	orgID := "00000000-0000-4000-8000-0000000000f6"

	_, orderID := setupRenewalOrder(t, tenantID, userID, orgID, 5)

	additionalDays := 7
	outTradeNo := "RN-FB-" + uuid.New().String()[:12]
	meta := renewalMetadata{
		AdditionalDays: additionalDays,
		OrderID:        orderID,
		OutTradeNo:     outTradeNo,
	}
	metaJSON, _ := json.Marshal(meta)
	metaStr := string(metaJSON)

	// Historical record: no Days column populated (nil), meta in RawResponse.
	record := models.OrderPaymentRecord{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		OrgID:       &orgID,
		UserID:      userID,
		OrderID:     &orderID,
		OrderType:   "renewal",
		OutTradeNo:  &meta.OutTradeNo,
		Amount:      models.Cents(700),
		Type:        "payment",
		Status:      "paid",
		RawResponse: &metaStr,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, db.Create(&record).Error)

	tx := db.Begin()
	err := applyRenewalSideEffects(tx, &record, time.Now())
	if err != nil {
		tx.Rollback()
		t.Fatalf("applyRenewalSideEffects failed for legacy record: %v", err)
	}
	tx.Commit()

	var order models.Order
	require.NoError(t, db.Where("id = ?", orderID).First(&order).Error)
	expectedEnd := time.Now().AddDate(0, 0, 5+additionalDays).Format("2006-01-02")
	require.NotNil(t, order.EndDate, "order end date set")
	require.Equal(t, expectedEnd, (*order.EndDate)[:10], "legacy record falls back to RawResponse meta")
}

// #1802 T1: 无 Days 且无 meta 天数 → 明确报错（不静默吞错）。
func TestApplyRenewalSideEffects_NoDaysRejected(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()

	db := database.GetDB()
	tenantID := "00000000-0000-4000-8000-0000000000f7"
	userID := "00000000-0000-4000-8000-0000000000f8"
	orgID := "00000000-0000-4000-8000-0000000000f9"

	_, orderID := setupRenewalOrder(t, tenantID, userID, orgID, 5)

	// Callback result overwrote RawResponse and no Days column value —
	// must fail loudly, not silently do nothing.
	outTradeNo := "RN-NM-" + uuid.New().String()[:12]
	cbRaw, _ := json.Marshal(map[string]interface{}{
		"out_trade_no":   outTradeNo,
		"transaction_id": "wxcb-nometa",
		"amount":         1000,
		"trade_state":    "SUCCESS",
	})
	cbStr := string(cbRaw)

	record := models.OrderPaymentRecord{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		OrgID:       &orgID,
		UserID:      userID,
		OrderID:     &orderID,
		OrderType:   "renewal",
		OutTradeNo:  &outTradeNo,
		Amount:      models.Cents(1000),
		Type:        "payment",
		Status:      "paid",
		RawResponse: &cbStr,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, db.Create(&record).Error)

	tx := db.Begin()
	err := applyRenewalSideEffects(tx, &record, time.Now())
	tx.Rollback()
	require.Error(t, err, "missing renewal days must fail loudly, not silently no-op")
	require.Contains(t, fmt.Sprintf("%v", err), "invalid renewal metadata")
}
