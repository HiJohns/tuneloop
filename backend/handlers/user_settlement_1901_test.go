package handlers

import (
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1901: gross-basis test coupons (OREZ/ENO) only count as fully paid when
// points_basis='gross' AND the non-production opt-in flag is set.
func TestCouponCountsAsPaid(t *testing.T) {
	db := testfixtures.SetupTestDB(t)

	mkCoupon := func(code, basis string) {
		require.NoError(t, db.Create(&models.Coupon{
			ID: uuid.New().String(), Code: code, Type: "percent", Value: 10,
			Active: true, PointsBasis: basis,
		}).Error)
	}
	mkCoupon("OREZ", "gross")
	mkCoupon("ENO", "gross")
	mkCoupon("OTHER", "cash")
	mkCoupon("LEGACY", "")

	order := func(code string) models.Order {
		c := code
		return models.Order{CouponCode: &c}
	}

	assert.False(t, couponCountsAsPaid(db, order("OREZ")), "flag off → cash basis")
	t.Setenv("TEST_COUPON_GROSS_POINTS", "true")
	assert.True(t, couponCountsAsPaid(db, order("OREZ")))
	assert.True(t, couponCountsAsPaid(db, order("ENO")))
	assert.False(t, couponCountsAsPaid(db, order("OTHER")), "cash-basis coupon stays cash")
	assert.False(t, couponCountsAsPaid(db, order("LEGACY")), "missing basis is not gross")
	assert.False(t, couponCountsAsPaid(db, order("UNKNOWN")), "unknown code → false")
	assert.False(t, couponCountsAsPaid(db, models.Order{}), "no coupon → false")
}
