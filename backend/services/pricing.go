package services

import (
	"encoding/json"
	"math"

	"tuneloop-backend/models"
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

// InstrumentPricingFields — authoritative parser for the instruments.pricing
// JSONB. All monetary fields there are stored in YUAN (元), unlike
// pricing_breakdown (分, #1728). Reading them directly at each call site
// caused repeated unit mix-ups (e.g. an overdue fee written as yuan into a
// Cents column, #1743). Use this everywhere a handler needs pricing JSON
// values; call ToCents() when persisting into Cents columns.
type InstrumentPricingFields struct {
	DailyRent       float64
	Deposit         float64
	ShippingFee     float64
	OverdueDailyFee float64
	Raw             map[string]interface{}
}

// ParseInstrumentPricing parses the pricing JSONB (yuan semantics). No
// fallback is applied — consumers decide their own (e.g. loadOverdueDailyRate
// falls back to baseRate×1.5, inventory RentSettings falls back to daily_rent).
func ParseInstrumentPricing(pricingJSON string) InstrumentPricingFields {
	f := InstrumentPricingFields{Raw: map[string]interface{}{}}
	if pricingJSON == "" {
		return f
	}
	if err := json.Unmarshal([]byte(pricingJSON), &f.Raw); err != nil {
		return f
	}
	f.DailyRent = getFloat(f.Raw, "daily_rent")
	f.Deposit = getFloat(f.Raw, "deposit")
	f.ShippingFee = getFloat(f.Raw, "shipping_fee")
	f.OverdueDailyFee = getFloat(f.Raw, "overdue_daily_fee")
	return f
}

// ToCents converts the yuan fields to Cents (分) for Cents-column writes.
func (f InstrumentPricingFields) ToCents() (dailyRent, deposit, shippingFee, overdueDailyFee models.Cents) {
	return models.FromYuan(f.DailyRent),
		models.FromYuan(f.Deposit),
		models.FromYuan(f.ShippingFee),
		models.FromYuan(f.OverdueDailyFee)
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

	// Check manual overrides first — if daily_rent is overridden, use it as the
	// effective base rate but still build tiers from config (tier discounts apply
	// on top of the manual rate instead of being skipped entirely).
	var overrides map[string]interface{}
	json.Unmarshal([]byte(overridesJSON), &overrides)
	effectiveBaseRate := baseDailyRate
	if overrideVal, ok := overrides["daily_rent"].(float64); ok && overrideVal > 0 {
		effectiveBaseRate = overrideVal
		result.Deposit = getOverrideFloat(overrides, "deposit")
	}
	// base_daily_rate must be the same source as the tiers so the detail page's
	// daily-rent display and the tier list stay consistent (#1487).
	result.BaseDailyRate = effectiveBaseRate

	// Build tiers from config
	if tiersRaw, ok := config["tiers"].([]interface{}); ok {
		for _, tRaw := range tiersRaw {
			if t, ok := tRaw.(map[string]interface{}); ok {
				daysMax := int(getFloat(t, "days_max"))
				discount := int(getFloat(t, "discount_percent"))
				rate := effectiveBaseRate
				if discount > 0 {
					rate = effectiveBaseRate * (1 - float64(discount)/100)
				}
				result.Tiers = append(result.Tiers, TierPrice{
					DaysMax:   daysMax,
					DailyRate: rate,
				})
			}
		}
	}

	// Calculate deposit (v3: only ratio or custom)
	depositMode, _ := config["deposit_mode"].(string)
	result.DepositMode = depositMode
	switch depositMode {
	case "custom":
		result.Deposit = 0
	case "ratio", "":
		ratio := getFloat(config, "deposit_ratio")
		if ratio == 0 {
			ratio = 1.0
		}
		if totalPrice > 0 {
			result.Deposit = totalPrice * ratio
		}
	}

	// Check individual override fields (always runs, even with daily_rent override)
	if ov, ok := overrides["deposit"].(float64); ok && ov > 0 {
		result.Deposit = ov
	}
	if ov, ok := overrides["shipping_fee"].(float64); ok && ov > 0 {
		result.ShippingFee = ov
	}

	// Fallback: read pricing fields from instrument's Pricing JSONB
	// (#1743 统一解析器：元语义)
	if len(instrumentPricingJSON) > 0 && instrumentPricingJSON[0] != "" {
		pf := ParseInstrumentPricing(instrumentPricingJSON[0])
		if result.ShippingFee == 0 && pf.ShippingFee > 0 {
			result.ShippingFee = pf.ShippingFee
		}
		if result.Deposit == 0 && pf.Deposit > 0 {
			result.Deposit = pf.Deposit
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

// FormatPricingResult renders the V2 pricing payload. #1755: all money
// fields are cents (P3 contract) — yuan inputs ×100, math.Round (NO +0.5,
// #1763 lesson). Consumers (mobile) display ÷100.
func FormatPricingResult(p *InstrumentPricing) map[string]interface{} {
	toCents := func(v float64) float64 { return math.Round(v * 100) }
	tiers := make([]map[string]interface{}, 0, len(p.Tiers))
	for _, t := range p.Tiers {
		tiers = append(tiers, map[string]interface{}{
			"days_max":   t.DaysMax,
			"daily_rate": toCents(t.DailyRate),
		})
	}
	result := map[string]interface{}{
		"base_daily_rate": toCents(p.BaseDailyRate),
		"tiers":           tiers,
		"deposit":         toCents(p.Deposit),
		"deposit_mode":    p.DepositMode,
	}
	if p.ShippingFee > 0 {
		result["shipping_fee"] = toCents(p.ShippingFee)
	}
	return result
}
