package services

import (
	"testing"
)

func TestGetTierDiscount(t *testing.T) {
	tests := []struct {
		leaseTerm int
		expected  float64
	}{
		{1, 1.0},
		{30, 1.0},
		{31, 0.95},
		{180, 0.95},
		{181, 0.70},
		{365, 0.70},
		{366, 0.50},
		{1000, 0.50},
	}
	for _, tt := range tests {
		got := GetTierDiscount(tt.leaseTerm)
		if got != tt.expected {
			t.Errorf("GetTierDiscount(%d) = %f, want %f", tt.leaseTerm, got, tt.expected)
		}
	}
}

func TestFormatPricingBreakdownJSON(t *testing.T) {
	p := &PricingBreakdown{
		BaseDailyRent:       10.0,
		TierDiscountRate:    0.70,
		FinalDailyRent:      7.0,
		RentDays:            200,
		TotalAmount:         1400.0,
		AppliedPolicies: []AppliedPolicy{
			{Type: "tier_discount", PlanName: "阶梯折扣", Rate: 0.70},
		},
	}
	json := FormatPricingBreakdownJSON(p)
	if json == "" {
		t.Error("expected non-empty JSON")
	}
}
