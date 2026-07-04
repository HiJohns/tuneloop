package services

import (
	"testing"
)

func TestCalculatePricing_Deposit(t *testing.T) {
	config := `{"deposit_mode":"ratio","deposit_ratio":0.3}`

	t.Run("total_price > 0 uses total_price × ratio", func(t *testing.T) {
		result := CalculatePricing(100, 50000, config, "{}")
		expected := 50000.0 * 0.3
		if result.Deposit != expected {
			t.Errorf("expected deposit %.0f, got %.0f (totalPrice×ratio)", expected, result.Deposit)
		}
	})

	t.Run("total_price = 0 with deposit_ratio=0 falls back to baseDailyRate × 2.0", func(t *testing.T) {
		cfg := `{"deposit_mode":"ratio","deposit_ratio":0}`
		result := CalculatePricing(100, 0, cfg, "{}")
		expected := 100.0 * 2.0
		if result.Deposit != expected {
			t.Errorf("expected deposit %.0f, got %.0f (legacy fallback 2.0)", expected, result.Deposit)
		}
	})

	t.Run("total_price = 0 uses config deposit_ratio when set (>0)", func(t *testing.T) {
		cfg := `{"deposit_mode":"ratio","deposit_ratio":1.5}`
		result := CalculatePricing(100, 0, cfg, "{}")
		expected := 100.0 * 1.5
		if result.Deposit != expected {
			t.Errorf("expected deposit %.0f, got %.0f (config ratio for legacy)", expected, result.Deposit)
		}
	})

	t.Run("total_price = 0 with no deposit_ratio falls back to baseDailyRate × 2.0", func(t *testing.T) {
		cfg := `{"deposit_mode":"ratio"}`
		result := CalculatePricing(100, 0, cfg, "{}")
		expected := 100.0 * 2.0
		if result.Deposit != expected {
			t.Errorf("expected deposit %.0f, got %.0f (legacy fallback 2.0)", expected, result.Deposit)
		}
	})

	t.Run("total_price = 0 uses config ratio when provided (>0)", func(t *testing.T) {
		cfg := `{"deposit_mode":"ratio","deposit_ratio":1.5}`
		result := CalculatePricing(100, 0, cfg, "{}")
		expected := 100.0 * 1.5
		if result.Deposit != expected {
			t.Errorf("expected deposit %.0f, got %.0f (config ratio for legacy)", expected, result.Deposit)
		}
	})

	t.Run("deposit_mode = fixed uses fixed value", func(t *testing.T) {
		cfg := `{"deposit_mode":"fixed","deposit_fixed":2000}`
		result := CalculatePricing(100, 50000, cfg, "{}")
		if result.Deposit != 2000 {
			t.Errorf("expected deposit 2000, got %.0f (fixed mode)", result.Deposit)
		}
	})

	t.Run("total_price > 0 with custom ratio from config", func(t *testing.T) {
		cfg := `{"deposit_mode":"ratio","deposit_ratio":0.5}`
		result := CalculatePricing(100, 50000, cfg, "{}")
		expected := 50000.0 * 0.5
		if result.Deposit != expected {
			t.Errorf("expected deposit %.0f, got %.0f (custom ratio)", expected, result.Deposit)
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
