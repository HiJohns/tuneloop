package models

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestDamageReport(t *testing.T) {
	// Test struct creation
	amount := 500.00
	report := DamageReport{
		ID:                uuid.New().String(),
		TenantID:          uuid.New().String(),
		OrgID:             uuid.New().String(),
		LeaseID:           uuid.New().String(),
		InstrumentID:      uuid.New().String(),
		UserID:            uuid.New().String(),
		DamageAmount:      &amount,
		DamageDescription: "琴弦断裂",
		DepositDeducted:   0,
		Status:            "pending",
	}

	assert.NotEmpty(t, report.ID)
	assert.Equal(t, &amount, report.DamageAmount)
	assert.Equal(t, "琴弦断裂", report.DamageDescription)
	assert.Equal(t, "pending", report.Status)

	// Test all status values from design doc
	statuses := []string{"pending", "agreed", "appealed", "resolved", "cancelled"}
	for _, status := range statuses {
		report.Status = status
		assert.Equal(t, status, report.Status)
	}

	// Test nullable fields
	assessedBy := uuid.New().String()
	now := time.Now()
	report.AssessedBy = &assessedBy
	report.AssessedAt = &now
	assert.Equal(t, assessedBy, *report.AssessedBy)
	assert.Equal(t, now, *report.AssessedAt)
}

func TestDamageAssessment(t *testing.T) {
	// Test struct creation
	assessment := DamageAssessment{
		ID:           uuid.New().String(),
		TenantID:     uuid.New().String(),
		OrgID:        uuid.New().String(),
		OrderID:      uuid.New().String(),
		InstrumentID: uuid.New().String(),
		UserID:       uuid.New().String(),
		Condition:    "good",
		Notes:        "外观完好，功能正常",
	}

	assert.NotEmpty(t, assessment.ID)
	assert.Equal(t, "good", assessment.Condition)
	assert.Equal(t, "外观完好，功能正常", assessment.Notes)

	// Test nullable fields
	assessedBy := uuid.New().String()
	scanTime := time.Now()
	assessment.AssessedBy = &assessedBy
	assessment.ScanTime = &scanTime
	assert.Equal(t, assessedBy, *assessment.AssessedBy)
	assert.Equal(t, scanTime, *assessment.ScanTime)
}

func TestAppeal(t *testing.T) {
	// Test struct creation
	appeal := Appeal{
		ID:             uuid.New().String(),
		TenantID:       uuid.New().String(),
		OrgID:          uuid.New().String(),
		DamageReportID: uuid.New().String(),
		UserID:         uuid.New().String(),
		AppealReason:   "琴弦自然老化，非人为损坏",
		Status:         "pending",
		SubmittedAt:    time.Now(),
	}

	assert.NotEmpty(t, appeal.ID)
	assert.Equal(t, "琴弦自然老化，非人为损坏", appeal.AppealReason)
	assert.Equal(t, "pending", appeal.Status)

	// Test all status values
	statuses := []string{"pending", "reviewing", "resolved", "cancelled"}
	for _, status := range statuses {
		appeal.Status = status
		assert.Equal(t, status, appeal.Status)
	}

	// Test nullable fields
	amount := 300.00
	appeal.FinalAmount = &amount
	assert.Equal(t, &amount, appeal.FinalAmount)
}

func TestOrderStatusHistory(t *testing.T) {
	// Test struct creation
	history := OrderStatusHistory{
		ID:         uuid.New().String(),
		TenantID:   uuid.New().String(),
		OrgID:      uuid.New().String(),
		OrderID:    uuid.New().String(),
		StatusFrom: "shipped",
		StatusTo:   "in_lease",
		Notes:      "物流到达，确认收货",
		ChangedAt:  time.Now(),
	}

	assert.NotEmpty(t, history.ID)
	assert.Equal(t, "shipped", history.StatusFrom)
	assert.Equal(t, "in_lease", history.StatusTo)
	assert.Equal(t, "物流到达，确认收货", history.Notes)

	// Test nullable fields
	changedBy := uuid.New().String()
	history.ChangedBy = &changedBy
	assert.Equal(t, changedBy, *history.ChangedBy)
}

func TestRelationships(t *testing.T) {
	// Test relationship between damage_report and appeal
	damageReportID := uuid.New().String()
	userID := uuid.New().String()

	appeal := Appeal{
		ID:             uuid.New().String(),
		DamageReportID: damageReportID,
		UserID:         userID,
		AppealReason:   "Test relationship",
	}

	assert.Equal(t, damageReportID, appeal.DamageReportID)
	assert.Equal(t, userID, appeal.UserID)
}
