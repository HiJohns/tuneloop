package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type MaintenanceHandler struct{}

func NewMaintenanceHandler() *MaintenanceHandler {
	return &MaintenanceHandler{}
}

type SubmitRepairRequest struct {
	OrderID            string   `json:"order_id" binding:"required"`
	InstrumentID       string   `json:"instrument_id" binding:"required"`
	ProblemDescription string   `json:"problem_description" binding:"required"`
	Images             []string `json:"images"`
	ServiceType        string   `json:"service_type"`
}

// SubmitRepair - Existing endpoint for merchants
func (h *MaintenanceHandler) SubmitRepair(c *gin.Context) {
	var req SubmitRepairRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())

	ticket := models.MaintenanceTicket{
		OrderID:            req.OrderID,
		InstrumentID:       req.InstrumentID,
		ProblemDescription: req.ProblemDescription,
		Images:             "[]",
		ServiceType:        req.ServiceType,
		Status:             models.TicketStatusPending,
	}

	if err := db.Create(&ticket).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to create ticket: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code": 20000,
		"data": gin.H{
			"ticket_id": ticket.ID,
			"status":    ticket.Status,
		},
	})
}

type ReportRepairRequest struct {
	InstrumentID       string   `json:"instrument_id" binding:"required"`
	ProblemDescription string   `json:"problem_description" binding:"required"`
	Images             []string `json:"images"`
	ServiceType        string   `json:"service_type"`
}

func (h *MaintenanceHandler) ReportRepair(c *gin.Context) {
	var req ReportRepairRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters: " + err.Error(),
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())

	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40101,
			"message": "user not authenticated",
		})
		return
	}

	imagesJSON := "[]"
	if len(req.Images) > 0 {
		imagesJSON = fmt.Sprintf(`["%s"]`, strings.Join(req.Images, `","`))
	}

	tenantID := c.GetString("tenant_id")
	orgID := c.GetString("org_id")

	ticket := models.MaintenanceTicket{
		TenantID:           tenantID,
		OrgID:              orgID,
		UserID:             userID,
		InstrumentID:       req.InstrumentID,
		ProblemDescription: req.ProblemDescription,
		Images:             imagesJSON,
		ServiceType:        req.ServiceType,
		Status:             models.TicketStatusPending,
	}

	if err := db.Create(&ticket).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to create ticket: " + err.Error(),
		})
		return
	}

	assignedTech, err := h.autoAssignTechnician(db, &ticket)
	if err == nil && assignedTech != "" {
		ticket.TechnicianID = assignedTech
		if updateErr := db.Model(&ticket).Update("technician_id", assignedTech).Error; updateErr != nil {
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"code": 20000,
		"data": gin.H{
			"ticket_id":     ticket.ID,
			"status":        ticket.Status,
			"technician_id": ticket.TechnicianID,
		},
	})
}

func (h *MaintenanceHandler) autoAssignTechnician(db *gorm.DB, ticket *models.MaintenanceTicket) (string, error) {
	var technicians []models.Technician
	if err := db.Where("tenant_id = ? AND site_id IS NOT NULL", ticket.TenantID).
		Limit(10).
		Find(&technicians).Error; err != nil {
		return "", err
	}

	if len(technicians) == 0 {
		return "", fmt.Errorf("no available technicians")
	}

	var assignedTech models.Technician
	lowestTicketCount := -1

	for _, tech := range technicians {
		var count int64
		if err := db.Model(&models.MaintenanceTicket{}).
			Where("technician_id = ? AND status IN (?, ?)",
				tech.ID, models.TicketStatusPending, models.TicketStatusProcessing).
			Count(&count).Error; err != nil {
			continue
		}

		if lowestTicketCount == -1 || int(count) < lowestTicketCount {
			lowestTicketCount = int(count)
			assignedTech = tech
		}
	}

	return assignedTech.ID, nil
}

func (h *MaintenanceHandler) GetMaintenanceDetail(c *gin.Context) {
	ticketID := c.Param("id")
	if ticketID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "ticket id is required",
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())

	var ticket models.MaintenanceTicket
	if err := db.First(&ticket, "id = ?", ticketID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    40400,
				"message": "ticket not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to get ticket: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": ticket,
	})
}

func (h *MaintenanceHandler) CancelMaintenance(c *gin.Context) {
	ticketID := c.Param("id")
	if ticketID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "ticket id is required",
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())

	result := db.Model(&models.MaintenanceTicket{}).Where("id = ?", ticketID).Update("status", "cancelled")
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to cancel ticket",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"status": "cancelled"},
	})
}

func (h *MaintenanceHandler) ListMerchantMaintenance(c *gin.Context) {
	status := c.Query("status")

	db := database.GetDB().WithContext(c.Request.Context())

	var tickets []models.MaintenanceTicket
	query := db.Model(&models.MaintenanceTicket{})

	if status != "" {
		query = query.Where("status = ?", status)
	}

	query.Order("created_at DESC").Find(&tickets)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":  tickets,
			"total": len(tickets),
		},
	})
}

func (h *MaintenanceHandler) AcceptMaintenance(c *gin.Context) {
	ticketID := c.Param("id")
	if ticketID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "ticket id required"})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())
	db.Model(&models.MaintenanceTicket{}).Where("id = ?", ticketID).Update("status", "processing")

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"status": "processing"},
	})
}

func (h *MaintenanceHandler) AssignTechnician(c *gin.Context) {
	ticketID := c.Param("id")

	var req struct {
		TechnicianID string `json:"technician_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters"})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())
	db.Model(&models.MaintenanceTicket{}).Where("id = ?", ticketID).Updates(map[string]interface{}{
		"technician_id": req.TechnicianID,
		"status":        "processing",
	})

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"status": "processing", "technician_id": req.TechnicianID},
	})
}

func (h *MaintenanceHandler) UpdateProgress(c *gin.Context) {
	ticketID := c.Param("id")

	var req struct {
		ProgressNotes string `json:"progress_notes"`
		Status        string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters"})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())
	updates := map[string]interface{}{"progress_notes": req.ProgressNotes}
	if req.Status != "" {
		updates["status"] = req.Status
	}
	db.Model(&models.MaintenanceTicket{}).Where("id = ?", ticketID).Updates(updates)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": updates,
	})
}

func (h *MaintenanceHandler) SendQuote(c *gin.Context) {
	ticketID := c.Param("id")

	var req struct {
		EstimatedCost float64 `json:"estimated_cost"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters"})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())
	db.Model(&models.MaintenanceTicket{}).Where("id = ?", ticketID).Update("estimated_cost", req.EstimatedCost)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"estimated_cost": req.EstimatedCost},
	})
}

type UpdateStatusRequest struct {
	Status       string   `json:"status" binding:"required,oneof=PENDING PROCESSING COMPLETED"`
	RepairReport string   `json:"repair_report"`
	RepairPhotos []string `json:"repair_photos"`
}

func (h *MaintenanceHandler) UpdateTicketStatus(c *gin.Context) {
	ticketID := c.Param("id")
	if ticketID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "ticket id required"})
		return
	}

	var req UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40101, "message": "user not authenticated"})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())

	var ticket models.MaintenanceTicket
	if err := db.First(&ticket, "id = ?", ticketID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "ticket not found"})
		return
	}

	if ticket.TechnicianID != "" && ticket.TechnicianID != userID {
		c.JSON(http.StatusForbidden, gin.H{"code": 40301, "message": "not authorized to update this ticket"})
		return
	}

	updates := map[string]interface{}{
		"status": req.Status,
	}

	if req.RepairReport != "" {
		updates["repair_report"] = req.RepairReport
	}

	if len(req.RepairPhotos) > 0 {
		photosJSON := fmt.Sprintf(`["%s"]`, strings.Join(req.RepairPhotos, `","`))
		updates["repair_photos"] = photosJSON
	}

	if req.Status == models.TicketStatusCompleted {
		now := time.Now()
		updates["completed_at"] = now

		if err := db.Model(&models.Instrument{}).Where("id = ?", ticket.InstrumentID).Update("stock_status", "available").Error; err != nil {
		}
	}

	if err := db.Model(&ticket).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update ticket"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"id":         ticket.ID,
			"status":     req.Status,
			"updated_at": time.Now(),
		},
	})
}

// ListTechnicianTickets - GET /api/technician/tickets
// List all tickets assigned to the current technician
func (h *MaintenanceHandler) ListTechnicianTickets(c *gin.Context) {
	technicianID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40101,
			"message": "technician not identified",
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())

	var tickets []models.MaintenanceTicket
	if err := db.Where("technician_id = ?", technicianID).Find(&tickets).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to fetch tickets: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": tickets,
	})
}

// AcceptTicket - PUT /api/technician/tickets/:id/accept
// Technician accepts a ticket to work on
func (h *MaintenanceHandler) AcceptTicket(c *gin.Context) {
	ticketID := c.Param("id")
	if ticketID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "ticket id required",
		})
		return
	}

	technicianID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40101,
			"message": "technician not identified",
		})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())

	var ticket models.MaintenanceTicket
	if err := db.First(&ticket, "id = ?", ticketID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "ticket not found",
		})
		return
	}

	// Update ticket status to processing and set technician
	updates := map[string]interface{}{
		"status":        models.TicketStatusProcessing,
		"technician_id": technicianID,
		"accepted_at":   time.Now(),
	}

	if err := db.Model(&ticket).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to accept ticket: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"id":      ticketID,
			"status":  models.TicketStatusProcessing,
			"message": "ticket accepted successfully",
		},
	})
}

// CompleteTicket - POST /api/technician/tickets/:id/complete
// Technician marks a ticket as completed
func (h *MaintenanceHandler) CompleteTicket(c *gin.Context) {
	ticketID := c.Param("id")
	if ticketID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "ticket id required",
		})
		return
	}

	technicianID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40101,
			"message": "technician not identified",
		})
		return
	}

	var req struct {
		Notes  string   `json:"notes"`
		Photos []string `json:"photos"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		// Notes are optional, so we can continue without them
		req.Notes = ""
	}

	db := database.GetDB().WithContext(c.Request.Context())

	var ticket models.MaintenanceTicket
	if err := db.First(&ticket, "id = ?", ticketID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "ticket not found",
		})
		return
	}

	// Verify technician is assigned to this ticket
	if ticket.TechnicianID != technicianID {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    40301,
			"message": "not authorized to complete this ticket",
		})
		return
	}

	// Update ticket as completed
	updates := map[string]interface{}{
		"status":       models.TicketStatusCompleted,
		"completed_at": time.Now(),
	}

	if req.Notes != "" {
		updates["completion_notes"] = req.Notes
	}

	if len(req.Photos) > 0 {
		photosJSON := fmt.Sprintf(`["%s"]`, strings.Join(req.Photos, `","`))
		updates["completion_photos"] = photosJSON
	}

	if err := db.Model(&ticket).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to complete ticket: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"id":           ticketID,
			"status":       models.TicketStatusCompleted,
			"completed_at": time.Now(),
			"message":      "ticket completed successfully",
		},
	})
}
