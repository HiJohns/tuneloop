package handlers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// TestOverdueScheduler_TransitionToExpiredOnly verifies #1492: the overdue
// scheduler only transitions in_lease → expired and no longer creates
// overdue_charges (daily auto-deduction removed).
func TestOverdueScheduler_TransitionToExpiredOnly(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()

	// Ensure needed tables exist for this test.
	require.NoError(t, db.AutoMigrate(&models.Order{}, &models.Instrument{}, &models.OverdueCharge{}, &models.OrderStatusHistory{}, &models.OrderLog{}))

	tenantID := "00000000-0000-0000-0000-0000000000c1"
	orgID := "00000000-0000-0000-0000-0000000000c2"
	userID := "00000000-0000-0000-0000-0000000000c3"

	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "OD-" + time.Now().Format("150405"),
		BaseDailyRate: float64Ptr(10),
		StockStatus:   "rented",
	}
	require.NoError(t, db.Create(&instrument).Error)

	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	startDate := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
	order := models.Order{
		TenantID:     tenantID,
		OrgID:        orgID,
		UserID:       userID,
		InstrumentID: instrument.ID,
		StartDate:    &startDate,
		EndDate:      &yesterday,
		LeaseTerm:    5,
		Status:       models.OrderStatusInLease,
	}
	require.NoError(t, db.Create(&order).Error)

	// Run the scheduler's overdue processing.
	s := services.NewOverdueDeductionScheduler()
	s.SetDBForTest(db)
	s.ProcessOverdueForTest()

	// Order must transition to expired.
	var updated models.Order
	require.NoError(t, db.First(&updated, "id = ?", order.ID).Error)
	require.Equal(t, models.OrderStatusExpired, updated.Status, "in_lease order past end_date must become expired")

	// No overdue_charges may be created (daily deduction removed).
	var chargeCount int64
	db.Model(&models.OverdueCharge{}).Where("order_id = ?", order.ID).Count(&chargeCount)
	require.Zero(t, chargeCount, "no overdue_charges may be created after deduction removal")
}

// TestOverdueScheduler_NoTransitionWhenNotDue verifies an order still in its
// lease term is not touched.
func TestOverdueScheduler_NoTransitionWhenNotDue(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()

	require.NoError(t, db.AutoMigrate(&models.Order{}, &models.Instrument{}, &models.OverdueCharge{}, &models.OrderStatusHistory{}, &models.OrderLog{}))

	tenantID := "00000000-0000-0000-0000-0000000000d1"
	orgID := "00000000-0000-0000-0000-0000000000d2"
	userID := "00000000-0000-0000-0000-0000000000d3"

	instrument := models.Instrument{
		TenantID:      tenantID,
		OrgID:         &orgID,
		SN:            "OD2-" + time.Now().Format("150405"),
		BaseDailyRate: float64Ptr(10),
		StockStatus:   "rented",
	}
	require.NoError(t, db.Create(&instrument).Error)

	future := time.Now().AddDate(0, 0, 5).Format("2006-01-02")
	startDate := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
	order := models.Order{
		TenantID:     tenantID,
		OrgID:        orgID,
		UserID:       userID,
		InstrumentID: instrument.ID,
		StartDate:    &startDate,
		EndDate:      &future,
		LeaseTerm:    10,
		Status:       models.OrderStatusInLease,
	}
	require.NoError(t, db.Create(&order).Error)

	s := services.NewOverdueDeductionScheduler()
	s.SetDBForTest(db)
	s.ProcessOverdueForTest()

	var updated models.Order
	require.NoError(t, db.First(&updated, "id = ?", order.ID).Error)
	require.Equal(t, models.OrderStatusInLease, updated.Status, "order not past end_date must stay in_lease")
}
