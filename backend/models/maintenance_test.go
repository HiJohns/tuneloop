package models

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestMaintenanceWorker(t *testing.T) {
	// Test struct creation
	worker := MaintenanceWorker{
		ID:       uuid.New().String(),
		TenantID: uuid.New().String(),
		OrgID:    uuid.New().String(),
		Name:     "张三",
		Phone:    "13800000000",
		Status:   "active",
	}

	assert.NotEmpty(t, worker.ID)
	assert.Equal(t, "张三", worker.Name)
	assert.Equal(t, "active", worker.Status)

	// Test with deleted_at
	now := time.Now()
	worker.DeletedAt = &now
	assert.NotNil(t, worker.DeletedAt)
}

func TestMaintenanceSession(t *testing.T) {
	// Test struct creation
	session := MaintenanceSession{
		ID:                  uuid.New().String(),
		TenantID:            uuid.New().String(),
		OrgID:               uuid.New().String(),
		MaintenanceTicketID: uuid.New().String(),
		Status:              "pending",
	}

	assert.NotEmpty(t, session.ID)
	assert.Equal(t, "pending", session.Status)

	// Test status values
	statuses := []string{"pending", "assigned", "in_progress", "completed", "passed", "failed"}
	for _, status := range statuses {
		session.Status = status
		assert.Equal(t, status, session.Status)
	}
}

func TestMaintenanceSessionRecord(t *testing.T) {
	// Test struct creation
	record := MaintenanceSessionRecord{
		ID:         uuid.New().String(),
		TenantID:   uuid.New().String(),
		SessionID:  uuid.New().String(),
		RecordType: "comment",
		Content:    "维修记录测试",
	}

	assert.NotEmpty(t, record.ID)
	assert.Equal(t, "comment", record.RecordType)
	assert.Equal(t, "维修记录测试", record.Content)
}

// Test relationships
func TestMaintenanceRelationships(t *testing.T) {
	workerID := uuid.New().String()
	sessionID := uuid.New().String()

	worker := MaintenanceWorker{
		ID:   workerID,
		Name: "李四",
	}

	session := MaintenanceSession{
		ID:       sessionID,
		WorkerID: &workerID,
		Status:   "assigned",
	}

	assert.Equal(t, workerID, *session.WorkerID)
	assert.Equal(t, "assigned", session.Status)
	assert.Equal(t, "李四", worker.Name)
}
