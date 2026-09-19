package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// seedPointsBatch creates an activity batch with the given cents for a user.
// #1983 stage 2: balance = SUM(unexpired point_batches.remaining_cents), so
// tests seed points via a batch instead of the removed users.promo_points.
func seedPointsBatch(t *testing.T, db *gorm.DB, userID string, cents models.Cents) {
	t.Helper()
	_, err := services.CreatePointsBatch(db, userID, services.PointBatchSourceActivity, "test_seed", cents)
	require.NoError(t, err)
}

// pointsBalance returns the batch-SUM balance for a user (#1983).
func pointsBalance(t *testing.T, db *gorm.DB, userID string) models.Cents {
	t.Helper()
	b, err := services.GetUserPointsBalance(db, userID)
	require.NoError(t, err)
	return b
}
