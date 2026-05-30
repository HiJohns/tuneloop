package services

import (
	"encoding/json"
)

type TierConfig struct {
	Name            string  `json:"name"`
	DaysMax         int     `json:"days_max"`
	DiscountPercent int     `json:"discount_percent"`
}

type MerchantPricingConfig struct {
	TemplateID    string                 `json:"template_id"`
	Config        map[string]interface{} `json:"config"`
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

// CalculatePricing computes instrument pricing from base rate and merchant config
func CalculatePricing(baseDailyRate float64, configJSON string, overridesJSON string) *InstrumentPricing {
	var config map[string]interface{}
	json.Unmarshal([]byte(configJSON), &config)

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
		ratio := getFloat(config, "deposit_ratio")
		if ratio <= 0 {
			ratio = 2.0
		}
		result.Deposit = baseDailyRate * ratio
	}

	// Check individual override fields
	if ov, ok := overrides["deposit"].(float64); ok && ov > 0 {
		result.Deposit = ov
	}
	if ov, ok := overrides["shipping_fee"].(float64); ok && ov > 0 {
		result.ShippingFee = ov
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
