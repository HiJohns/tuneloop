package handlers

import (
	"net/http"
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
		Status:             "pending",
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
