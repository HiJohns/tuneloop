package handlers

import (
	"testing"
	"time"

	"tuneloop-backend/models"

	"github.com/stretchr/testify/assert"
)

// #1926: before delivery the actual rent figures are 0 even when the pricing
// breakdown carries initial/planned actual_* snapshots.
func TestDeriveActualRent_PreDeliveryZero(t *testing.T) {
	pb := map[string]interface{}{
		"actual_rent_days":   float64(30),
		"actual_rent_amount": float64(300000),
	}
	order := &models.Order{} // DeliveredAt nil → paid / shipped states

	days, cents := deriveActualRent(nil, order, nil, pb)
	assert.Equal(t, 0, days, "pre-delivery days must be 0")
	assert.Equal(t, int64(0), cents, "pre-delivery actual rent must be 0")
}

// Settlement remains authoritative (e.g. returned orders with legacy nil
// delivered_at) and post-delivery figures are unchanged.
func TestDeriveActualRent_SettlementAndPostDelivery(t *testing.T) {
	settlement := map[string]interface{}{
		"actual_rent_days":   5,
		"actual_rent_amount": models.Cents(50000),
	}
	days, cents := deriveActualRent(nil, &models.Order{}, settlement, nil)
	assert.Equal(t, 5, days)
	assert.Equal(t, int64(50000), cents)

	delivered := time.Now().Add(-72 * time.Hour)
	returned := time.Now()
	pb := map[string]interface{}{
		"actual_rent_days":   float64(3),
		"actual_rent_amount": float64(30000),
	}
	days, cents = deriveActualRent(nil, &models.Order{DeliveredAt: &delivered, ReturnedAt: &returned}, nil, pb)
	assert.Equal(t, 3, days)
	assert.Equal(t, int64(30000), cents)
}
