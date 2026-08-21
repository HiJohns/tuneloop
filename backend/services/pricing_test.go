package services

import (
	"testing"
)

func TestCalculatePricing_Deposit(t *testing.T) {
	config := `{"deposit_mode":"ratio","deposit_ratio":0.3,"deposit_multiplier":7}`

	t.Run("total_price > 0 uses total_price × deposit_ratio", func(t *testing.T) {
		result := CalculatePricing(100, 50000, config, "{}")
		expected := 50000.0 * 0.3
		if result.Deposit != expected {
			t.Errorf("expected deposit %.0f, got %.0f (totalPrice×ratio)", expected, result.Deposit)
		}
	})

	t.Run("total_price = 0 yields zero deposit in ratio mode", func(t *testing.T) {
		// #1438: deposit modes reduced to ratio + custom — the old
		// baseDailyRate×deposit_multiplier formula no longer exists;
		// without a total_price there is nothing to ratio against.
		result := CalculatePricing(100, 0, config, "{}")
		if result.Deposit != 0 {
			t.Errorf("expected deposit 0 (no total_price), got %.0f", result.Deposit)
		}
	})

	t.Run("total_price > 0 with custom deposit_ratio", func(t *testing.T) {
		cfg := `{"deposit_mode":"ratio","deposit_ratio":0.5,"deposit_multiplier":7}`
		result := CalculatePricing(100, 50000, cfg, "{}")
		expected := 50000.0 * 0.5
		if result.Deposit != expected {
			t.Errorf("expected deposit %.0f, got %.0f (custom ratio)", expected, result.Deposit)
		}
	})

	t.Run("total_price = 0 custom ratio also yields zero", func(t *testing.T) {
		cfg := `{"deposit_mode":"ratio","deposit_ratio":0.1,"deposit_multiplier":3}`
		result := CalculatePricing(100, 0, cfg, "{}")
		if result.Deposit != 0 {
			t.Errorf("expected deposit 0 (no total_price), got %.0f", result.Deposit)
		}
	})

	t.Run("zero ratio defaults to full amount (#1436)", func(t *testing.T) {
		cfg := `{"deposit_mode":"ratio","deposit_ratio":0,"deposit_multiplier":0}`
		result := CalculatePricing(100, 50000, cfg, "{}")
		if result.Deposit != 50000 {
			t.Errorf("expected deposit 50000 (ratio 0 → default 1.0), got %.0f", result.Deposit)
		}
	})

	t.Run("deposit_mode = custom uses override only", func(t *testing.T) {
		// #1438: fixed/standard/free modes removed — only ratio + custom.
		cfg := `{"deposit_mode":"custom"}`
		result := CalculatePricing(100, 50000, cfg, `{"deposit":1500}`)
		if result.Deposit != 1500 {
			t.Errorf("expected deposit 1500 (custom override), got %.0f", result.Deposit)
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

func TestCalculatePricing_OverrideDailyRentKeepsTiers(t *testing.T) {
	config := `{"tiers":[{"days_max":30,"discount_percent":0},{"days_max":180,"discount_percent":10},{"days_max":-1,"discount_percent":20}],"deposit_mode":"ratio","deposit_ratio":0.3}`
	overrides := `{"daily_rent":103,"deposit":5000}`

	result := CalculatePricing(100, 50000, config, overrides)

	if len(result.Tiers) != 3 {
		t.Fatalf("expected 3 tiers, got %d", len(result.Tiers))
	}
	if result.Tiers[0].DailyRate != 103 {
		t.Errorf("expected first tier rate 103 (override daily_rent), got %.2f", result.Tiers[0].DailyRate)
	}
	if result.Tiers[1].DailyRate != 92.7 {
		t.Errorf("expected second tier rate 92.7 (103 × 0.9), got %.2f", result.Tiers[1].DailyRate)
	}
	if result.Tiers[2].DailyRate != 82.4 {
		t.Errorf("expected third tier rate 82.4 (103 × 0.8), got %.2f", result.Tiers[2].DailyRate)
	}
	if result.Deposit != 5000 {
		t.Errorf("expected deposit 5000 (override), got %.2f", result.Deposit)
	}
}

func TestCalculatePricing_NoOverrideKeepsTiers(t *testing.T) {
	config := `{"tiers":[{"days_max":30,"discount_percent":0},{"days_max":180,"discount_percent":10},{"days_max":-1,"discount_percent":20}],"deposit_mode":"ratio","deposit_ratio":0.3}`

	result := CalculatePricing(100, 50000, config, "{}")

	if len(result.Tiers) != 3 {
		t.Fatalf("expected 3 tiers, got %d", len(result.Tiers))
	}
	if result.Tiers[0].DailyRate != 100 {
		t.Errorf("expected first tier rate 100, got %.2f", result.Tiers[0].DailyRate)
	}
	if result.Tiers[1].DailyRate != 90 {
		t.Errorf("expected second tier rate 90, got %.2f", result.Tiers[1].DailyRate)
	}
	if result.Tiers[2].DailyRate != 80 {
		t.Errorf("expected third tier rate 80, got %.2f", result.Tiers[2].DailyRate)
	}
}

func TestCalculatePricing_NoTiersNoOverrideRegression(t *testing.T) {
	config := `{"deposit_mode":"ratio","deposit_ratio":0.3,"deposit_multiplier":7}`

	result := CalculatePricing(100, 50000, config, "{}")

	if len(result.Tiers) != 0 {
		t.Fatalf("expected 0 tiers (no tiers in config), got %d", len(result.Tiers))
	}
	if result.Deposit != 15000 {
		t.Errorf("expected deposit 15000, got %.2f", result.Deposit)
	}
	if result.BaseDailyRate != 100 {
		t.Errorf("expected base rate 100, got %.2f", result.BaseDailyRate)
	}
}
