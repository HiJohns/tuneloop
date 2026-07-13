package services

import (
	"math"
	"testing"
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
			segs := computeTierSegments(tt.days, nil)
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
	segs := computeTierSegments(25, custom)
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
