package handlers

import (
	"net/http"
	"strconv"
	"time"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type LeaseHandler struct {
	db *gorm.DB
}

func NewLeaseHandler(db *gorm.DB) *LeaseHandler {
	return &LeaseHandler{db: db}
}

func (h *LeaseHandler) ListLeases(c *gin.Context) {
	tenantID := middleware.GetTenantID(c.Request.Context())
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "tenant_id not found"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	status := c.Query("status")
	userID := c.Query("user_id")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	offset := (page - 1) * pageSize

	query := h.db.Model(&models.Lease{}).Where("tenant_id = ?", tenantID)

	if status != "" {
		query = query.Where("status = ?", status)
	}
	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if startDate != "" {
		query = query.Where("start_date >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("end_date <= ?", endDate)
	}

	var total int64
	query.Count(&total)

	var leases []models.Lease
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&leases).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch leases: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":  leases,
			"total": total,
			"page":  page,
			"size":  pageSize,
		},
	})
}

func (h *LeaseHandler) GetLease(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	leaseID := c.Param("id")

	var lease models.Lease
	if err := h.db.Where("id = ? AND tenant_id = ?", leaseID, tenantID).First(&lease).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "Lease not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch lease: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": lease,
	})
}

type CreateLeaseRequest struct {
	UserID        string  `json:"user_id" binding:"required"`
	InstrumentID  string  `json:"instrument_id" binding:"required"`
	StartDate     string  `json:"start_date" binding:"required"`
	EndDate       string  `json:"end_date" binding:"required"`
	MonthlyRent   float64 `json:"monthly_rent" binding:"required"`
	DepositAmount float64 `json:"deposit_amount" binding:"required"`
}

func (h *LeaseHandler) CreateLease(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "tenant_id not found"})
		return
	}

	var req CreateLeaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "Invalid request: " + err.Error()})
		return
	}

	lease := models.Lease{
		TenantID:      tenantID,
		UserID:        req.UserID,
		InstrumentID:  req.InstrumentID,
		StartDate:     req.StartDate,
		EndDate:       req.EndDate,
		MonthlyRent:   req.MonthlyRent,
		DepositAmount: req.DepositAmount,
		Status:        "active",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := h.db.Create(&lease).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to create lease: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": lease,
	})
}

type UpdateLeaseRequest struct {
	StartDate     string  `json:"start_date"`
	EndDate       string  `json:"end_date"`
	MonthlyRent   float64 `json:"monthly_rent"`
	DepositAmount float64 `json:"deposit_amount"`
	Status        string  `json:"status"`
}

func (h *LeaseHandler) UpdateLease(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	leaseID := c.Param("id")

	var lease models.Lease
	if err := h.db.Where("id = ? AND tenant_id = ?", leaseID, tenantID).First(&lease).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "Lease not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch lease: " + err.Error(),
		})
		return
	}

	var req UpdateLeaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "Invalid request: " + err.Error()})
		return
	}

	updates := make(map[string]interface{})
	if req.StartDate != "" {
		updates["start_date"] = req.StartDate
	}
	if req.EndDate != "" {
		updates["end_date"] = req.EndDate
	}
	if req.MonthlyRent > 0 {
		updates["monthly_rent"] = req.MonthlyRent
	}
	if req.DepositAmount > 0 {
		updates["deposit_amount"] = req.DepositAmount
	}
	if req.Status != "" {
		updates["status"] = req.Status
	}
	updates["updated_at"] = time.Now()

	if err := h.db.Model(&lease).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to update lease: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "Lease updated successfully",
	})
}

func (h *LeaseHandler) TerminateLease(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	leaseID := c.Param("id")

	var lease models.Lease
	if err := h.db.Where("id = ? AND tenant_id = ?", leaseID, tenantID).First(&lease).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "Lease not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch lease: " + err.Error(),
		})
		return
	}

	if err := h.db.Model(&lease).Updates(map[string]interface{}{
		"status":     "terminated",
		"updated_at": time.Now(),
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to terminate lease: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "Lease terminated successfully",
	})
}

type DepositHandler struct {
	db *gorm.DB
}

func NewDepositHandler(db *gorm.DB) *DepositHandler {
	return &DepositHandler{db: db}
}

func (h *DepositHandler) ListDeposits(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "tenant_id not found"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	depositType := c.Query("type")
	status := c.Query("status")
	leaseID := c.Query("lease_id")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	offset := (page - 1) * pageSize

	query := h.db.Model(&models.Deposit{}).Where("tenant_id = ?", tenantID)

	if depositType != "" {
		query = query.Where("type = ?", depositType)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if leaseID != "" {
		query = query.Where("lease_id = ?", leaseID)
	}

	var total int64
	query.Count(&total)

	var deposits []models.Deposit
	if err := query.Order("transaction_date DESC").Offset(offset).Limit(pageSize).Find(&deposits).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch deposits: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":  deposits,
			"total": total,
			"page":  page,
			"size":  pageSize,
		},
	})
}

type CreateDepositRequest struct {
	LeaseID         string  `json:"lease_id" binding:"required"`
	UserID          string  `json:"user_id" binding:"required"`
	Amount          float64 `json:"amount" binding:"required"`
	Type            string  `json:"type" binding:"required"`
	TransactionDate string  `json:"transaction_date" binding:"required"`
	Notes           string  `json:"notes"`
}

func (h *DepositHandler) CreateDeposit(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "tenant_id not found"})
		return
	}

	var req CreateDepositRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "Invalid request: " + err.Error()})
		return
	}

	deposit := models.Deposit{
		TenantID:        tenantID,
		LeaseID:         req.LeaseID,
		UserID:          req.UserID,
		Amount:          req.Amount,
		Type:            req.Type,
		Status:          "pending",
		TransactionDate: req.TransactionDate,
		Notes:           req.Notes,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := h.db.Create(&deposit).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to create deposit: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": deposit,
	})
}

type UpdateDepositRequest struct {
	Status string `json:"status"`
	Notes  string `json:"notes"`
}

func (h *DepositHandler) UpdateDeposit(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	depositID := c.Param("id")

	var deposit models.Deposit
	if err := h.db.Where("id = ? AND tenant_id = ?", depositID, tenantID).First(&deposit).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "Deposit not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to fetch deposit: " + err.Error(),
		})
		return
	}

	var req UpdateDepositRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "Invalid request: " + err.Error()})
		return
	}

	updates := make(map[string]interface{})
	if req.Status != "" {
		updates["status"] = req.Status
	}
	if req.Notes != "" {
		updates["notes"] = req.Notes
	}
	updates["updated_at"] = time.Now()

	if err := h.db.Model(&deposit).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "Failed to update deposit: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "Deposit updated successfully",
	})
}
