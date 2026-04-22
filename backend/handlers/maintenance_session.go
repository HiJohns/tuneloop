package handlers

import (
	"fmt"
	"net/http"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type MaintenanceSessionHandler struct{}

func NewMaintenanceSessionHandler() *MaintenanceSessionHandler {
	return &MaintenanceSessionHandler{}
}

// PUT /api/maintenance/sessions/:id/status - Update session status
func (h *MaintenanceSessionHandler) UpdateStatus(c *gin.Context) {
	var req struct {
		Status  string   `json:"status" binding:"required"`
		Comment string   `json:"comment"`
		Photos  []string `json:"photos"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	sessionID := c.Param("id")
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	db := database.GetDB().WithContext(ctx)

	// Verify session exists
	var session models.MaintenanceSession
	if err := db.Where("id = ? AND tenant_id = ?", sessionID, tenantID).First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "session not found"})
		return
	}

	// Validate status transition
	validStatuses := []string{"pending", "assigned", "in_progress", "completed", "passed", "failed"}
	isValid := false
	for _, s := range validStatuses {
		if req.Status == s {
			isValid = true
			break
		}
	}
	if !isValid {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid status value"})
		return
	}

	// Update session
	session.Status = req.Status
	if req.Status == "in_progress" && session.StartTime == nil {
		now := time.Now()
		session.StartTime = &now
	}
	if req.Status == "completed" && session.EndTime == nil {
		now := time.Now()
		session.EndTime = &now
	}
	if req.Status == "in_progress" {
		session.ProgressNotes = req.Comment
	}
	if req.Status == "completed" {
		session.CompletionNotes = req.Comment
	}
	session.UpdatedAt = time.Now()

	if err := db.Save(&session).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update session: " + err.Error()})
		return
	}

	// Create session record for comment/photos
	if req.Comment != "" || len(req.Photos) > 0 {
		record := models.MaintenanceSessionRecord{
			ID:         uuid.New().String(),
			TenantID:   tenantID,
			SessionID:  sessionID,
			RecordType: "comment",
			Content:    req.Comment,
		}
		db.Create(&record)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data":    session,
	})
}

// POST /api/maintenance/sessions/:id/start-work - Scan QR code to start work
func (h *MaintenanceSessionHandler) StartWork(c *gin.Context) {
	var req struct {
		InstrumentSN string    `json:"instrument_sn" binding:"required"`
		ScanTime     time.Time `json:"scan_time" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	sessionID := c.Param("id")
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	db := database.GetDB().WithContext(ctx)

	// Verify session exists and is assigned
	var session models.MaintenanceSession
	if err := db.Where("id = ? AND tenant_id = ? AND status = ?", sessionID, tenantID, "assigned").First(&session).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "session not found or not in assigned status"})
		return
	}

	// Update session status to in_progress
	now := time.Now()
	session.Status = "in_progress"
	session.StartTime = &now
	session.UpdatedAt = time.Now()

	if err := db.Save(&session).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update session: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"session_id":    sessionID,
			"status":        "in_progress",
			"instrument_sn": req.InstrumentSN,
			"start_time":    now,
		},
	})
}

// POST /api/maintenance/sessions/:id/records - Submit maintenance record
func (h *MaintenanceSessionHandler) SubmitRecord(c *gin.Context) {
	var req struct {
		Type    string   `json:"type" binding:"required"`
		Content string   `json:"content" binding:"required"`
		Photos  []string `json:"photos"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	sessionID := c.Param("id")
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	db := database.GetDB().WithContext(ctx)

	// Verify session exists and is in_progress
	var session models.MaintenanceSession
	if err := db.Where("id = ? AND tenant_id = ? AND status = ?", sessionID, tenantID, "in_progress").First(&session).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "session not found or not in progress"})
		return
	}

	// Create session record
	record := models.MaintenanceSessionRecord{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		SessionID:  sessionID,
		RecordType: req.Type,
		Content:    req.Content,
	}

	if err := db.Create(&record).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create record: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":    20000,
		"message": "success",
		"data":    record,
	})
}

// PUT /api/maintenance/sessions/:id/inspect - Inspection processing
func (h *MaintenanceSessionHandler) Inspect(c *gin.Context) {
	var req struct {
		Result  string   `json:"result" binding:"required"` // passed or failed
		Comment string   `json:"comment"`
		Photos  []string `json:"photos"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	if req.Result != "passed" && req.Result != "failed" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "result must be 'passed' or 'failed'"})
		return
	}

	sessionID := c.Param("id")
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	db := database.GetDB().WithContext(ctx)

	// Verify session exists and is completed
	var session models.MaintenanceSession
	if err := db.Where("id = ? AND tenant_id = ? AND status = ?", sessionID, tenantID, "completed").First(&session).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "session not found or not completed"})
		return
	}

	// Update session with inspection result
	session.Status = req.Result
	session.InspectionComment = req.Comment
	session.UpdatedAt = time.Now()

	if err := db.Save(&session).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update session: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"session_id": sessionID,
			"result":     req.Result,
			"comment":    req.Comment,
		},
	})
}

// GET /api/maintenance/sessions - List maintenance sessions
func (h *MaintenanceSessionHandler) ListSessions(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	// Pagination parameters
	page := 1
	pageSize := 20
	if p, err := parseIntParam(c.Query("page"), 1); err == nil && p > 0 {
		page = p
	}
	if ps, err := parseIntParam(c.Query("page_size"), 20); err == nil && ps > 0 {
		pageSize = ps
	}
	offset := (page - 1) * pageSize

	db := database.GetDB().WithContext(ctx)

	// Build query
	query := db.Where("tenant_id = ?", tenantID)

	// Optional filters
	if ticketID := c.Query("ticket_id"); ticketID != "" {
		query = query.Where("maintenance_ticket_id = ?", ticketID)
	}
	if workerID := c.Query("worker_id"); workerID != "" {
		query = query.Where("worker_id = ?", workerID)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	var sessions []models.MaintenanceSession
	var total int64

	// Count total
	if err := query.Model(&models.MaintenanceSession{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to count sessions: " + err.Error()})
		return
	}

	// Query sessions with pagination
	if err := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&sessions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query sessions: " + err.Error()})
		return
	}

	// Prepare result
	result := make([]gin.H, len(sessions))
	for i, session := range sessions {
		item := gin.H{
			"id":                     session.ID,
			"maintenance_ticket_id":  session.MaintenanceTicketID,
			"worker_id":              session.WorkerID,
			"status":                 session.Status,
			"start_time":             session.StartTime,
			"end_time":               session.EndTime,
			"progress_notes":         session.ProgressNotes,
			"completion_notes":       session.CompletionNotes,
			"inspection_result":      session.InspectionResult,
			"inspection_comment":     session.InspectionComment,
			"created_at":             session.CreatedAt,
			"updated_at":             session.UpdatedAt,
		}

		result[i] = item
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"list":  result,
			"total": total,
			"page":  page,
			"page_size": pageSize,
		},
	})
}

// GET /api/maintenance/sessions/:id - Get a single maintenance session
func (h *MaintenanceSessionHandler) GetSession(c *gin.Context) {
	sessionID := c.Param("id")
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "session ID is required"})
		return
	}

	db := database.GetDB().WithContext(ctx)

	var session models.MaintenanceSession
	if err := db.Where("id = ? AND tenant_id = ?", sessionID, tenantID).First(&session).Error; err != nil {
		if err.Error() == "record not found" {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "session not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query session: " + err.Error()})
		}
		return
	}

	// Load related records
	var records []models.MaintenanceSessionRecord
	if err := db.Where("session_id = ?", sessionID).Order("created_at ASC").Find(&records).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query records: " + err.Error()})
		return
	}

	recordData := make([]gin.H, len(records))
	for i, record := range records {
		recordData[i] = gin.H{
			"id":         record.ID,
			"record_type": record.RecordType,
			"content":    record.Content,
			"created_at": record.CreatedAt,
		}
	}

	result := gin.H{
		"id":                    session.ID,
		"maintenance_ticket_id": session.MaintenanceTicketID,
		"worker_id":            session.WorkerID,
		"status":               session.Status,
		"start_time":           session.StartTime,
		"end_time":             session.EndTime,
		"progress_notes":       session.ProgressNotes,
		"completion_notes":     session.CompletionNotes,
		"inspection_result":    session.InspectionResult,
		"inspection_comment":   session.InspectionComment,
		"created_at":           session.CreatedAt,
		"updated_at":           session.UpdatedAt,
		"records":              recordData,
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data":    result,
	})
}

// parseIntParam safely parses string to int with default value
func parseIntParam(s string, defaultVal int) (int, error) {
	if s == "" {
		return defaultVal, nil
	}
	var val int
	_, err := fmt.Sscanf(s, "%d", &val)
	return val, err
}
