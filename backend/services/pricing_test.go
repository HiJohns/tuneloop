package services

import (
	"testing"
)

func TestCalculatePricing_Deposit(t *testing.T) {
	config := `{"deposit_mode":"ratio","deposit_multiplier":7}`

	t.Run("uses baseDailyRate × deposit_multiplier", func(t *testing.T) {
		result := CalculatePricing(100, 0, config, "{}")
		expected := 100.0 * 7
		if result.Deposit != expected {
			t.Errorf("expected deposit %.0f, got %.0f (baseDailyRate×multiplier)", expected, result.Deposit)
		}
	})

	t.Run("zero multiplier yields zero deposit", func(t *testing.T) {
		cfg := `{"deposit_mode":"ratio","deposit_multiplier":0}`
		result := CalculatePricing(100, 0, cfg, "{}")
		if result.Deposit != 0 {
			t.Errorf("expected deposit 0 (zero multiplier), got %.0f", result.Deposit)
		}
	})

	t.Run("custom multiplier from config", func(t *testing.T) {
		cfg := `{"deposit_mode":"ratio","deposit_multiplier":1.5}`
		result := CalculatePricing(100, 0, cfg, "{}")
		expected := 100.0 * 1.5
		if result.Deposit != expected {
			t.Errorf("expected deposit %.0f, got %.0f (custom multiplier)", expected, result.Deposit)
		}
	})

	t.Run("missing multiplier yields zero deposit", func(t *testing.T) {
		cfg := `{"deposit_mode":"ratio"}`
		result := CalculatePricing(100, 0, cfg, "{}")
		if result.Deposit != 0 {
			t.Errorf("expected deposit 0 (no multiplier), got %.0f", result.Deposit)
		}
	})

	t.Run("deposit_mode = fixed uses fixed value", func(t *testing.T) {
		cfg := `{"deposit_mode":"fixed","deposit_fixed":2000}`
		result := CalculatePricing(100, 50000, cfg, "{}")
		if result.Deposit != 2000 {
			t.Errorf("expected deposit 2000, got %.0f (fixed mode)", result.Deposit)
		}
	})

	t.Run("override deposit takes precedence", func(t *testing.T) {
		overrides := `{"deposit":9999}`
		result := CalculatePricing(100, 50000, config, overrides)
		if result.Deposit != 9999 {
			t.Errorf("expected deposit 9999 (override), got %.0f", result.Deposit)
		}
	})
}

func TestCalculateTieredPricing_70Days(t *testing.T) {
	tiers := []TierConfig{
		{DaysMax: 30, DiscountPercent: 0},
		{DaysMax: 180, DiscountPercent: 5},
		{DaysMax: -1, DiscountPercent: 10},
	}
	result := CalculateTieredPricing(70, 100, tiers)

	if len(result.Tiers) != 2 {
		t.Errorf("expected 2 tiers, got %d", len(result.Tiers))
	}

	expectedTotal := 100.0*30 + 95.0*40 // = 6800
	if result.TotalRent != expectedTotal {
		t.Errorf("expected total rent %.2f, got %.2f", expectedTotal, result.TotalRent)
	}

	if result.Tiers[0].Subtotal != 3000 {
		t.Errorf("expected tier1 subtotal 3000, got %.2f", result.Tiers[0].Subtotal)
	}

	if result.Tiers[1].Subtotal != 3800 {
		t.Errorf("expected tier2 subtotal 3800, got %.2f", result.Tiers[1].Subtotal)
	}

	if result.BaseDailyRate != 100 {
		t.Errorf("expected base rate 100, got %.2f", result.BaseDailyRate)
	}
}

func TestCalculateTieredPricing_SingleTier(t *testing.T) {
	tiers := []TierConfig{
		{DaysMax: -1, DiscountPercent: 0},
	}
	result := CalculateTieredPricing(30, 200, tiers)

	if len(result.Tiers) != 1 {
		t.Errorf("expected 1 tier, got %d", len(result.Tiers))
	}

	if result.TotalRent != 6000 {
		t.Errorf("expected total rent 6000, got %.2f", result.TotalRent)
	}

	if result.Tiers[0].DaysInTier != 30 {
		t.Errorf("expected 30 days in tier, got %d", result.Tiers[0].DaysInTier)
	}
}

func TestCalculateTieredPricing_ZeroDays(t *testing.T) {
	result := CalculateTieredPricing(0, 100, []TierConfig{{DaysMax: 30, DiscountPercent: 0}})
	if result.TotalRent != 0 {
		t.Errorf("expected 0 rent for 0 days, got %.2f", result.TotalRent)
	}
}
