package handlers

import (
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
)

// #1734: base_daily_rent 单位归一测试——存量元残留 → ×100；新订单分 → 原样；
// 乐器现价回退判定；无乐器时不放大错误。

func TestResolveBaseDailyRentCents(t *testing.T) {
	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Skip("test database not available")
		return
	}
	database.SetDB(db)

	// 无乐器（order nil / instrument 不存在）→ 原样返回
	if got := resolveBaseDailyRentCents(db, nil, 28); got != 28 {
		t.Errorf("nil order: got %v want 28", got)
	}
	// 分语义直接返回
	if got := resolveBaseDailyRentCents(db, nil, 3600); got != 3600 {
		t.Errorf("cents value: got %v want 3600", got)
	}
	if got := resolveBaseDailyRentCents(db, nil, 0); got != 0 {
		t.Errorf("zero: got %v want 0", got)
	}

	// 有乐器：存量元残留（bdr=28 元 vs 乐器现价 3600 分）→ ×100 = 2800
	tenantID := "00000000-0000-4000-8000-0000000000a1"
	orgID := "00000000-0000-4000-8000-0000000000a2"
	inst := models.Instrument{
		ID: "00000000-0000-4000-8000-0000000000a3", TenantID: tenantID, OrgID: &orgID,
		BaseDailyRate: models.ToCentsPtr(float64Ptr(36)), // 3600 分 = 36 元
		StockStatus:   "available",
	}
	if err := db.Create(&inst).Error; err != nil {
		t.Fatalf("seed instrument: %v", err)
	}
	defer db.Where("id = ?", inst.ID).Delete(&models.Instrument{})

	order := &models.Order{ID: "00000000-0000-4000-8000-0000000000a4", InstrumentID: inst.ID, TenantID: tenantID}
	if got := resolveBaseDailyRentCents(db, order, 28); got != 2800 {
		t.Errorf("legacy yuan residue: got %v want 2800", got)
	}
	// 分语义 + 乐器存在 → 原样
	if got := resolveBaseDailyRentCents(db, order, 3600); got != 3600 {
		t.Errorf("cents with instrument: got %v want 3600", got)
	}
	// 高价乐器元残留（bdr=205 元 vs 乐器现价 20500 分）→ ×100 = 20500
	// （固定 <100 阈值会漏判——#1734 修正为最近邻判定）
	inst3 := models.Instrument{
		ID: "00000000-0000-4000-8000-0000000000a7", TenantID: tenantID, OrgID: &orgID,
		BaseDailyRate: models.ToCentsPtr(float64Ptr(205)), // 20500 分 = 205 元
		StockStatus:   "available",
	}
	if err := db.Create(&inst3).Error; err != nil {
		t.Fatalf("seed instrument3: %v", err)
	}
	defer db.Where("id = ?", inst3.ID).Delete(&models.Instrument{})
	order3 := &models.Order{ID: "00000000-0000-4000-8000-0000000000a8", InstrumentID: inst3.ID, TenantID: tenantID}
	if got := resolveBaseDailyRentCents(db, order3, 205); got != 20500 {
		t.Errorf("high-price yuan residue: got %v want 20500", got)
	}
	// 高价乐器分语义 → 原样
	if got := resolveBaseDailyRentCents(db, order3, 20500); got != 20500 {
		t.Errorf("high-price cents: got %v want 20500", got)
	}
	// 无法判定（乐器现价 ≤ bdr×100，如 bdr=50 分=¥0.5 且乐器现价 50 分）→ 按分
	inst2 := models.Instrument{
		ID: "00000000-0000-4000-8000-0000000000a5", TenantID: tenantID, OrgID: &orgID,
		BaseDailyRate: models.ToCentsPtr(float64Ptr(0.5)), // 50 分
		StockStatus:   "available",
	}
	if err := db.Create(&inst2).Error; err != nil {
		t.Fatalf("seed instrument2: %v", err)
	}
	defer db.Where("id = ?", inst2.ID).Delete(&models.Instrument{})
	order2 := &models.Order{ID: "00000000-0000-4000-8000-0000000000a6", InstrumentID: inst2.ID, TenantID: tenantID}
	if got := resolveBaseDailyRentCents(db, order2, 50); got != 50 {
		t.Errorf("indeterminate: got %v want 50", got)
	}
}

