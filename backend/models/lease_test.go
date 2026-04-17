package models

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestLeaseSession(t *testing.T) {
	// Test struct creation
	session := LeaseSession{
		ID:           uuid.New().String(),
		TenantID:     uuid.New().String(),
		OrgID:        uuid.New().String(),
		OrderID:      uuid.New().String(),
		UserID:       uuid.New().String(),
		InstrumentID: uuid.New().String(),
		Status:       "active",
		ReturnMethod: "logistics",
	}

	assert.NotEmpty(t, session.ID)
	assert.Equal(t, "active", session.Status)

	// Test all status values
	statuses := []string{"active", "expiring_soon", "overdue", "return_requested", "returning", "completed", "cancelled"}
	for _, status := range statuses {
		session.Status = status
		assert.Equal(t, status, session.Status)
	}

	// Test date fields
	startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	session.StartDate = startDate
	session.EndDate = endDate
	assert.Equal(t, startDate, session.StartDate)
	assert.Equal(t, endDate, session.EndDate)

	// Test actual_end_date (can be nil)
	actualEndDate := time.Date(2024, 1, 14, 0, 0, 0, 0, time.UTC)
	session.ActualEndDate = &actualEndDate
	assert.Equal(t, actualEndDate, *session.ActualEndDate)
}

func TestElectronicContract(t *testing.T) {
	// Test struct creation
	contract := ElectronicContract{
		ID:             uuid.New().String(),
		TenantID:       uuid.New().String(),
		OrgID:          uuid.New().String(),
		OrderID:        uuid.New().String(),
		UserID:         uuid.New().String(),
		InstrumentID:   uuid.New().String(),
		ContractURL:    "https://cdn.example.com/contract.pdf",
		ContractNumber: "CONT-2024-001",
		Status:         "active",
	}

	assert.NotEmpty(t, contract.ID)
	assert.Equal(t, "https://cdn.example.com/contract.pdf", contract.ContractURL)
	assert.Equal(t, "CONT-2024-001", contract.ContractNumber)
	assert.Equal(t, "active", contract.Status)
}
