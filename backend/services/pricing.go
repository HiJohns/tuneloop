package services

import (
	"encoding/json"
)

type TierConfig struct {
	Name            string `json:"name"`
	DaysMax         int    `json:"days_max"`
	DiscountPercent int    `json:"discount_percent"`
}

type MerchantPricingConfig struct {
	TemplateID string                 `json:"template_id"`
	Config     map[string]interface{} `json:"config"`
}

type TierPrice struct {
	DaysMax   int     `json:"days_max"`
	DailyRate float64 `json:"daily_rate"`
}

type InstrumentPricing struct {
	BaseDailyRate float64     `json:"base_daily_rate"`
	Tiers         []TierPrice `json:"tiers"`
	Deposit       float64     `json:"deposit"`
	DepositMode   string      `json:"deposit_mode"`
	ShippingFee   float64     `json:"shipping_fee,omitempty"`
}

// TierDetail represents one tier's contribution to the final rent calculation.
type TierDetail struct {
	DaysMin         int     `json:"days_min"`
	DaysMax         int     `json:"days_max"`
	DiscountPercent int     `json:"discount_percent"`
	DaysInTier      int     `json:"days_in_tier"`
	EffectiveRate   float64 `json:"effective_rate"`
	Subtotal        float64 `json:"subtotal"`
}

// TieredPricingResult contains the full per-day tiered calculation result.
type TieredPricingResult struct {
	BaseDailyRate float64      `json:"base_daily_rate"`
	TotalDays     int          `json:"total_days"`
	Tiers         []TierDetail `json:"tiers"`
	TotalRent     float64      `json:"total_rent"`
}

// CalculateTieredPricing calculates rent by splitting days across discount tiers.
// Each tier specifies days_max (max day in this tier) and discount_percent.
// Example: 70 days, tiers=[{30,0},{180,5},{-1,10}] with base=100
//
//	Tier 1: 30 days × 100 = 3000
//	Tier 2: 40 days × 95 = 3800
//	Total: 6800
func CalculateTieredPricing(days int, baseDailyRate float64, tiers []TierConfig) *TieredPricingResult {
	result := &TieredPricingResult{
		BaseDailyRate: baseDailyRate,
		TotalDays:     days,
	}

	if days <= 0 || baseDailyRate <= 0 {
		return result
	}

	accumulated := 0
	prevMax := 0
	totalRent := 0.0

	for _, t := range tiers {
		daysMax := t.DaysMax
		if daysMax <= 0 {
			daysMax = days // -1 means unlimited, cap at total days
		}

		daysInTier := daysMax - prevMax
		if daysInTier <= 0 {
			prevMax = daysMax
			continue
		}

		remaining := days - accumulated
		if remaining <= 0 {
			break
		}

		if daysInTier > remaining {
			daysInTier = remaining
		}

		rate := baseDailyRate
		if t.DiscountPercent > 0 {
			rate = baseDailyRate * (1 - float64(t.DiscountPercent)/100)
		}

		subtotal := rate * float64(daysInTier)
		totalRent += subtotal

		result.Tiers = append(result.Tiers, TierDetail{
			DaysMin:         prevMax + 1,
			DaysMax:         prevMax + daysInTier,
			DiscountPercent: t.DiscountPercent,
			DaysInTier:      daysInTier,
			EffectiveRate:   rate,
			Subtotal:        subtotal,
		})

		accumulated += daysInTier
		prevMax = daysMax

		if accumulated >= days {
			break
		}
	}

	result.TotalRent = totalRent
	return result
}

// ResolvePricingConfig resolves the effective pricing config for a tenant.
// Priority: merchant-specific config → system default template.
func ResolvePricingConfig(tenantID string) ([]TierConfig, error) {
	return []TierConfig{
		{DaysMax: 30, DiscountPercent: 0},
		{DaysMax: 180, DiscountPercent: 5},
		{DaysMax: -1, DiscountPercent: 10},
	}, nil // simplified: returns system defaults; full merchant lookup deferred to sub-task
}

// CalculatePricing computes instrument pricing from base rate and merchant config
func CalculatePricing(baseDailyRate float64, totalPrice float64, configJSON string, overridesJSON string, instrumentPricingJSON ...string) *InstrumentPricing {
	var config map[string]interface{}
	json.Unmarshal([]byte(configJSON), &config)

	// Merge defaults into root level for schema-style templates
	if defaults, ok := config["defaults"].(map[string]interface{}); ok {
		for k, v := range defaults {
			if _, exists := config[k]; !exists {
				config[k] = v
			}
		}
	}

	result := &InstrumentPricing{
		BaseDailyRate: baseDailyRate,
		DepositMode:   "ratio",
	}

	// Check manual overrides first — if daily_rent is overridden, skip formula
	var overrides map[string]interface{}
	json.Unmarshal([]byte(overridesJSON), &overrides)
	if overrideVal, ok := overrides["daily_rent"].(float64); ok && overrideVal > 0 {
		result.Tiers = []TierPrice{
			{DaysMax: -1, DailyRate: overrideVal},
		}
		result.Deposit = getOverrideFloat(overrides, "deposit")
		return result
	}

	// Build tiers from config
	if tiersRaw, ok := config["tiers"].([]interface{}); ok {
		for _, tRaw := range tiersRaw {
			if t, ok := tRaw.(map[string]interface{}); ok {
				daysMax := int(getFloat(t, "days_max"))
				discount := int(getFloat(t, "discount_percent"))
				rate := baseDailyRate
				if discount > 0 {
					rate = baseDailyRate * (1 - float64(discount)/100)
				}
				result.Tiers = append(result.Tiers, TierPrice{
					DaysMax:   daysMax,
					DailyRate: rate,
				})
			}
		}
	}

	// Calculate deposit
	depositMode, _ := config["deposit_mode"].(string)
	result.DepositMode = depositMode
	switch depositMode {
	case "fixed":
		result.Deposit = getFloat(config, "deposit_fixed")
	default:
		multiplier := getFloat(config, "deposit_multiplier")
		result.Deposit = baseDailyRate * multiplier
	}

	// Check individual override fields
	if ov, ok := overrides["deposit"].(float64); ok && ov > 0 {
		result.Deposit = ov
	}
	if ov, ok := overrides["shipping_fee"].(float64); ok && ov > 0 {
		result.ShippingFee = ov
	}

	// Fallback: read shipping_fee from instrument's Pricing field
	if result.ShippingFee == 0 && len(instrumentPricingJSON) > 0 && instrumentPricingJSON[0] != "" {
		var ip map[string]interface{}
		json.Unmarshal([]byte(instrumentPricingJSON[0]), &ip)
		if fee, ok := ip["shipping_fee"].(float64); ok && fee > 0 {
			result.ShippingFee = fee
		}
	}

	return result
}

func getFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return val
		case int:
			return float64(val)
		}
	}
	return 0
}

func getOverrideFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return val
		}
	}
	return 0
}

func GetDefaultMerchantTiers() string {
	b, _ := json.Marshal([]TierConfig{
		{DaysMax: 30, DiscountPercent: 0},
		{DaysMax: 365, DiscountPercent: 20},
		{DaysMax: -1, DiscountPercent: 40},
	})
	return string(b)
}

func FormatPricingResult(p *InstrumentPricing) map[string]interface{} {
	result := map[string]interface{}{
		"base_daily_rate": p.BaseDailyRate,
		"tiers":           p.Tiers,
		"deposit":         p.Deposit,
		"deposit_mode":    p.DepositMode,
	}
	if p.ShippingFee > 0 {
		result["shipping_fee"] = p.ShippingFee
	}
	return result
}
