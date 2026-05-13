package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func stringPtr(s string) *string {
	return &s
}

type WarehouseHandler struct{}

func NewWarehouseHandler() *WarehouseHandler {
	return &WarehouseHandler{}
}

// GET /api/warehouse/orders - Get order list with status filter
func (h *WarehouseHandler) ListOrders(c *gin.Context) {
	ctx := c.Request.Context()

	status := c.Query("status")
	siteID := c.Query("site_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	db := database.GetDB().WithContext(ctx)

	query := db.Model(&models.Order{})

	orgID := middleware.GetOrgID(ctx)
	if orgID != "" {
		query = query.Where("org_id = ?", orgID)
	}
	if siteID != "" {
		query = query.Where("site_id = ?", siteID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	query.Count(&total)

	offset := (page - 1) * pageSize
	var orders []models.Order
	query.Offset(offset).Limit(pageSize).Find(&orders)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":     orders,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		},
	})
}

// PUT /api/warehouse/orders/:id/shipping - Update logistics info
func (h *WarehouseHandler) UpdateShipping(c *gin.Context) {
	var req struct {
		TrackingNumber string    `json:"tracking_number" binding:"required"`
		Company        string    `json:"company" binding:"required"`
		ShippedAt      time.Time `json:"shipped_at" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	orderID := c.Param("id")
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)

	// Update order logistics info
	shippedAt := req.ShippedAt
	company := req.Company
	trackingNumber := req.TrackingNumber
	if err := db.Model(&models.Order{}).Where("id = ? AND tenant_id = ?", orderID, tenantID).Updates(map[string]interface{}{
		"tracking_number": trackingNumber,
		"courier_company": company,
		"shipped_at":      shippedAt,
		"status":          "shipped",
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update shipping: " + err.Error()})
		return
	}

	// Get order to fetch org_id for history
	var order models.Order
	if err := db.Where("id = ? AND tenant_id = ?", orderID, tenantID).First(&order).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to fetch order: " + err.Error()})
		return
	}

	// 同步更新乐器状态为 shipping（对应 cases.md §0.3 状态机）
	if err := db.Model(&models.Instrument{}).Where("id = ?", order.InstrumentID).Update("stock_status", models.StockStatusShipping).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update instrument: " + err.Error()})
		return
	}

	// Record status history
	history := models.OrderStatusHistory{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		OrgID:      stringPtr(order.OrgID),
		OrderID:    orderID,
		StatusFrom: "preparing",
		StatusTo:   "shipped",
		Notes:      "物流信息已录入",
		ChangedBy:  stringPtr(userID),
		ChangedAt:  time.Now(),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := db.Create(&history).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to record status history: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"order_id": orderID,
			"status":   "shipped",
		},
	})
}

// PUT /api/warehouse/orders/:id/delivery - Confirm delivery (record lease start)
func (h *WarehouseHandler) ConfirmDelivery(c *gin.Context) {
	var req struct {
		DeliveredAt time.Time `json:"delivered_at" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	orderID := c.Param("id")
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	// 1. Look up the order to get its tenant_id (customer JWT has no tenant context)
	var order models.Order
	if err := db.First(&order, "id = ?", orderID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "order not found"})
		return
	}

	// 2. Verify the user owns this order
	userID := middleware.GetUserID(ctx)
	if order.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "not your order"})
		return
	}

	// 3. Update order status and record delivery time as lease start
	if err := db.Model(&models.Order{}).Where("id = ? AND tenant_id = ?", orderID, order.TenantID).Updates(map[string]interface{}{
		"status":       "in_lease",
		"delivered_at": req.DeliveredAt,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to confirm delivery: " + err.Error()})
		return
	}

	// 3.5 Update instrument status to rented
	if err := db.Model(&models.Instrument{}).Where("id = ?", order.InstrumentID).
		Update("stock_status", models.StockStatusRented).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update instrument status: " + err.Error()})
		return
	}

	// 4. Record status history
	history := models.OrderStatusHistory{
		ID:         uuid.New().String(),
		TenantID:   order.TenantID,
		OrderID:    orderID,
		StatusFrom: "shipped",
		StatusTo:   "in_lease",
		Notes:      "物流到达，开始租赁",
		ChangedBy:  stringPtr(userID),
		ChangedAt:  time.Now(),
	}
	if err := db.Create(&history).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to record status history: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"order_id":    orderID,
			"status":      "in_lease",
			"lease_start": req.DeliveredAt,
		},
	})
}

// PUT /api/warehouse/orders/:id/return-inspect - Return inspection
func (h *WarehouseHandler) InspectReturn(c *gin.Context) {
	var req struct {
		InstrumentSN string    `json:"instrument_sn" binding:"required"`
		ScanTime     time.Time `json:"scan_time" binding:"required"`
		Condition    string    `json:"condition" binding:"required"`
		Notes        string    `json:"notes"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	orderID := c.Param("id")
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	if req.Condition != "good" && req.Condition != "damaged" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "condition must be good or damaged"})
		return
	}

	db := database.GetDB().WithContext(ctx)

	// Get order
	var order models.Order
	if err := db.Where("id = ? AND tenant_id = ? AND status = ?", orderID, tenantID, "returning").First(&order).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "order not found or not in returning status"})
		return
	}

	// Create assessment record
	userID := middleware.GetUserID(ctx)
	assessment := models.DamageAssessment{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		OrderID:    orderID,
		UserID:     order.UserID,
		Condition:  req.Condition,
		Notes:      req.Notes,
		ScanTime:   &req.ScanTime,
		AssessedBy: stringPtr(userID),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := db.Create(&assessment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create assessment: " + err.Error()})
		return
	}

	// Update order status
	newStatus := "in_stock"
	if req.Condition == "damaged" {
		newStatus = "maintenance"
	}
	if err := db.Model(&models.Order{}).Where("id = ?", orderID).Update("status", newStatus).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update order status: " + err.Error()})
		return
	}

	// Update instrument status
	instStatus := models.StockStatusAvailable
	if req.Condition == "damaged" {
		instStatus = models.StockStatusMaintenance
	}
	if err := db.Model(&models.Instrument{}).Where("id = ?", order.InstrumentID).
		Update("stock_status", instStatus).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update instrument status: " + err.Error()})
		return
	}

	// Record status history
	history := models.OrderStatusHistory{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		OrderID:    orderID,
		StatusFrom: "returning",
		StatusTo:   newStatus,
		Notes:      "归还验收: " + req.Condition,
		ChangedBy:  stringPtr(userID),
		ChangedAt:  time.Now(),
	}
	if err := db.Create(&history).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to record status history: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"order_id":      orderID,
			"status":        newStatus,
			"condition":     req.Condition,
			"assessment_id": assessment.ID,
		},
	})
}

// PUT /api/warehouse/orders/:id/damage - Start damage assessment
func (h *WarehouseHandler) AssessDamage(c *gin.Context) {
	var req struct {
		DamageDescription string  `json:"damage_description" binding:"required"`
		DamageAmount      float64 `json:"damage_amount" binding:"required"`
		Notes             string  `json:"notes"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	orderID := c.Param("id")
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	db := database.GetDB().WithContext(ctx)

	// Get order
	var order models.Order
	if err := db.Where("id = ? AND tenant_id = ?", orderID, tenantID).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "order not found"})
		return
	}

	// Update order status to inspecting
	if err := db.Model(&models.Order{}).Where("id = ?", orderID).Update("status", "inspecting").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update order status: " + err.Error()})
		return
	}

	// Update instrument status to maintenance
	if err := db.Model(&models.Instrument{}).Where("id = ?", order.InstrumentID).
		Update("stock_status", models.StockStatusMaintenance).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update instrument status: " + err.Error()})
		return
	}

	// Record status history
	userID := middleware.GetUserID(ctx)
	history := models.OrderStatusHistory{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		OrderID:    orderID,
		StatusFrom: order.Status,
		StatusTo:   "inspecting",
		Notes:      "开始定损评估",
		ChangedBy:  stringPtr(userID),
		ChangedAt:  time.Now(),
	}
	if err := db.Create(&history).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to record status history: " + err.Error()})
		return
	}

	orgID := middleware.GetOrgID(ctx)
	notification := models.Notification{
		TenantID: tenantID,
		OrgID:    orgID,
		UserID:   order.UserID,
		Type:     "damage",
		Title:    "定损通知",
		Content:  fmt.Sprintf("您的乐器租赁订单已被定损评估，定损金额：%.2f，说明：%s", req.DamageAmount, req.DamageDescription),
		RefID:    orderID,
		RefType:  "order",
		Status:   "unread",
	}
	if err := db.Create(&notification).Error; err != nil {
		log.Printf("[AssessDamage] Failed to create notification: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"order_id":      orderID,
			"status":        "inspecting",
			"damage_amount": req.DamageAmount,
		},
	})
}
