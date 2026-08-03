package services

import (
	"math"
	"testing"

	"tuneloop-backend/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func roundTo2(f float64) float64 {
	return math.Round(f*100) / 100
}

func TestComputeTierSegments(t *testing.T) {
	tests := []struct {
		name      string
		days      int
		expected  int
		firstDays int
		lastDisc  float64
	}{
		{"1 day", 1, 1, 1, 1.0},
		{"30 days", 30, 1, 30, 1.0},
		{"31 days", 31, 2, 30, 0.95},
		{"42 days", 42, 2, 30, 0.95},
		{"180 days", 180, 2, 30, 0.95},
		{"181 days", 181, 3, 30, 0.70},
		{"365 days", 365, 3, 30, 0.70},
		{"366 days", 366, 4, 30, 0.50},
		{"1000 days", 1000, 4, 30, 0.50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segs := ComputeTierSegments(tt.days, nil)
			if len(segs) != tt.expected {
				t.Errorf("computeTierSegments(%d) got %d segments, want %d", tt.days, len(segs), tt.expected)
				return
			}
			if segs[0].Days != tt.firstDays {
				t.Errorf("computeTierSegments(%d) first segment days = %d, want %d", tt.days, segs[0].Days, tt.firstDays)
			}
			last := segs[len(segs)-1]
			if last.Discount != tt.lastDisc {
				t.Errorf("computeTierSegments(%d) last discount = %f, want %f", tt.days, last.Discount, tt.lastDisc)
			}
		})
	}
}

func TestComputeTierSegments_CustomTiers(t *testing.T) {
	custom := []PricingTierConfig{
		{DaysMax: 10, DiscountPercent: 0},
		{DaysMax: 20, DiscountPercent: 10},
		{DaysMax: -1, DiscountPercent: 20},
	}
	segs := ComputeTierSegments(25, custom)
	if len(segs) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(segs))
	}
	if segs[0].Days != 10 || segs[0].Discount != 1.0 {
		t.Errorf("segment 1: days=%d discount=%f", segs[0].Days, segs[0].Discount)
	}
	if segs[1].Days != 10 || roundTo2(segs[1].Discount) != 0.90 {
		t.Errorf("segment 2: days=%d discount=%f", segs[1].Days, segs[1].Discount)
	}
	if segs[2].Days != 5 || roundTo2(segs[2].Discount) != 0.80 {
		t.Errorf("segment 3: days=%d discount=%f", segs[2].Days, segs[2].Discount)
	}
}

func TestFormatPricingBreakdownJSON(t *testing.T) {
	p := &PricingBreakdown{
		BaseDailyRent:  10.0,
		FinalDailyRent: 8.50,
		RentDays:       200,
		TotalAmount:    1700.0,
		TierSegments: []TierSegment{
			{Tier: 1, Days: 30, Rate: 10, Discount: 1.0, Subtotal: 300},
			{Tier: 2, Days: 150, Rate: 10, Discount: 0.95, Subtotal: 1425},
			{Tier: 3, Days: 20, Rate: 10, Discount: 0.70, Subtotal: 140},
		},
		AppliedPolicies: []AppliedPolicy{
			{Type: "tier_discount", PlanName: "阶梯折扣"},
		},
	}
	json := FormatPricingBreakdownJSON(p)
	if json == "" {
		t.Error("expected non-empty JSON")
	}
}

func TestCalculatePricingBreakdown_UsesTierDailyRates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	database.SetDB(db)

	input := RentCalcInput{
		BaseDailyRate: 100,
		LeaseTerm:     70,
		TenantID:      "test-tenant",
		PricingTiers: []PricingTierConfig{
			{DaysMax: 30, DailyRate: 103, DiscountPercent: 0},
			{DaysMax: 180, DailyRate: 92.7, DiscountPercent: 0},
			{DaysMax: -1, DailyRate: 82.4, DiscountPercent: 0},
		},
	}

	result, err := CalculatePricingBreakdown(input)
	if err != nil {
		t.Fatalf("CalculatePricingBreakdown returned error: %v", err)
	}

	if len(result.TierSegments) != 2 {
		t.Fatalf("expected 2 tier segments for 70 days, got %d", len(result.TierSegments))
	}

	seg1 := result.TierSegments[0]
	if seg1.Days != 30 || seg1.Rate != 103 {
		t.Errorf("segment 1: expected days=30 rate=103, got days=%d rate=%.2f", seg1.Days, seg1.Rate)
	}
	if roundTo2(seg1.Subtotal) != 3090 {
		t.Errorf("segment 1 subtotal: expected 3090, got %.2f", seg1.Subtotal)
	}

	seg2 := result.TierSegments[1]
	if seg2.Days != 40 || roundTo2(seg2.Rate) != 92.7 {
		t.Errorf("segment 2: expected days=40 rate=92.7, got days=%d rate=%.2f", seg2.Days, seg2.Rate)
	}
	if roundTo2(seg2.Subtotal) != 3708 {
		t.Errorf("segment 2 subtotal: expected 3708, got %.2f", seg2.Subtotal)
	}

	expectedTotal := 3090 + 3708
	if roundTo2(result.TotalAmount) != float64(expectedTotal) {
		t.Errorf("total amount: expected %.2f, got %.2f", float64(expectedTotal), result.TotalAmount)
	}
	if roundTo2(result.FinalDailyRent) != roundTo2(float64(expectedTotal)/70) {
		t.Errorf("final daily rent: expected %.2f, got %.2f", float64(expectedTotal)/70, result.FinalDailyRent)
	}
}

func TestCalculatePricingBreakdown_FallbackToBaseDailyRate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	database.SetDB(db)

	// No PricingTiers passed → segments fall back to BaseDailyRate
	input := RentCalcInput{
		BaseDailyRate: 100,
		LeaseTerm:     70,
		TenantID:      "test-tenant",
	}

	result, err := CalculatePricingBreakdown(input)
	if err != nil {
		t.Fatalf("CalculatePricingBreakdown returned error: %v", err)
	}

	if len(result.TierSegments) != 2 {
		t.Fatalf("expected 2 tier segments (default tiers), got %d", len(result.TierSegments))
	}
	// defaultTiers: 30d 0%, 180d 5%, 365d 30%, -1d 50%
	if result.TierSegments[0].Rate != 100 {
		t.Errorf("segment 1 rate: expected 100 (fallback base), got %.2f", result.TierSegments[0].Rate)
	}
	if roundTo2(result.TierSegments[1].Discount) != 0.95 {
		t.Errorf("segment 2 discount: expected 0.95, got %.2f", result.TierSegments[1].Discount)
	}
	// 30×100 + 40×100×0.95 = 3000 + 3800 = 6800
	if roundTo2(result.TotalAmount) != 6800 {
		t.Errorf("total amount: expected 6800, got %.2f", result.TotalAmount)
	}
}
