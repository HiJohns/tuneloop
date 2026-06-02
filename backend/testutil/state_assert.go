package testutil

import (
	"testing"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/stretchr/testify/assert"
)

func AssertState(t *testing.T, orderID, expectedStatus string) {
	t.Helper()
	db := database.GetDB()
	var order models.Order
	err := db.Where("id = ?", orderID).First(&order).Error
	if assert.NoError(t, err) {
		assert.Equal(t, expectedStatus, order.Status)
	}
}

func AssertStateHistoryContains(t *testing.T, orderID, statusFrom, statusTo string) bool {
	t.Helper()
	db := database.GetDB()
	var count int64
	db.Model(&models.OrderStatusHistory{}).
		Where("order_id = ? AND status_from = ? AND status_to = ?", orderID, statusFrom, statusTo).
		Count(&count)
	return count > 0
}
