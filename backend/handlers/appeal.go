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

type AppealHandler struct{}

func NewAppealHandler() *AppealHandler {
	return &AppealHandler{}
}

// GET /api/merchant/appeals - Get appeal list
func (h *AppealHandler) ListAppeals(c *gin.Context) {
	ctx := c.Request.Context()

	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	db := database.GetDB().WithContext(ctx)

	query := db.Model(&models.Appeal{})
	orgID := middleware.GetOrgID(ctx)
	if orgID != "" {
		query = query.Where("org_id = ?", orgID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	query.Count(&total)

	offset := (page - 1) * pageSize
	var appeals []models.Appeal
	query.Offset(offset).Limit(pageSize).Order("submitted_at DESC").Find(&appeals)

	// Batch load related data
	type appealItem struct {
		models.Appeal
		DamageReport *models.DamageReport `json:"damage_report,omitempty"`
		Order        *models.Order        `json:"order,omitempty"`
		InstrumentSN string               `json:"instrument_sn"`
		CategoryName string               `json:"category_name"`
		UserName     string               `json:"user_name"`
	}

	var items []appealItem
	var drIDs []string
	var orderIDs []string
	for _, a := range appeals {
		drIDs = append(drIDs, a.DamageReportID)
	}

	// Load all damage reports
	var damageReports []models.DamageReport
	if len(drIDs) > 0 {
		db.Where("id IN ?", drIDs).Find(&damageReports)
	}
	drMap := make(map[string]models.DamageReport)
	for _, dr := range damageReports {
		drMap[dr.ID] = dr
		orderIDs = append(orderIDs, dr.LeaseID)
	}

	// Load all orders
	var orders []models.Order
	if len(orderIDs) > 0 {
		db.Where("id IN ?", orderIDs).Find(&orders)
	}
	orderMap := make(map[string]models.Order)
	for _, o := range orders {
		orderMap[o.ID] = o
	}

	// Load all instruments and users
	var instrIDs []string
	var userIDs []string
	for _, o := range orders {
		instrIDs = append(instrIDs, o.InstrumentID)
		userIDs = append(userIDs, o.UserID)
	}

	var instruments []models.Instrument
	if len(instrIDs) > 0 {
		db.Where("id IN ?", instrIDs).Find(&instruments)
	}
	instrMap := make(map[string]models.Instrument)
	for _, inst := range instruments {
		instrMap[inst.ID] = inst
	}

	var users []models.User
	if len(userIDs) > 0 {
		db.Where("id IN ?", userIDs).Find(&users)
	}
	userMap := make(map[string]models.User)
	for _, u := range users {
		userMap[u.ID] = u
	}

	for _, a := range appeals {
		item := appealItem{Appeal: a}
		if dr, ok := drMap[a.DamageReportID]; ok {
			item.DamageReport = &dr
			if o, ok := orderMap[dr.LeaseID]; ok {
				item.Order = &o
				if inst, ok := instrMap[o.InstrumentID]; ok {
					item.InstrumentSN = inst.SN
					item.CategoryName = inst.CategoryName
				}
				if u, ok := userMap[o.UserID]; ok {
					item.UserName = u.Name
				}
			}
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":     items,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		},
	})
}

// GET /api/merchant/appeals/:id - Get appeal details
func (h *AppealHandler) GetAppeal(c *gin.Context) {
	appealID := c.Param("id")
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	db := database.GetDB().WithContext(ctx)

	var appeal models.Appeal
	if err := db.Where("id = ? AND tenant_id = ?", appealID, tenantID).First(&appeal).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "appeal not found"})
		return
	}

	var damageReport models.DamageReport
	if err := db.Where("id = ?", appeal.DamageReportID).First(&damageReport).Error; err != nil {
		damageReport = models.DamageReport{}
	}

	// Get order info
	type detailData struct {
		Appeal        models.Appeal        `json:"appeal"`
		DamageReport  models.DamageReport  `json:"damage_report"`
		Order         *models.Order        `json:"order,omitempty"`
		InstrumentSN  string               `json:"instrument_sn"`
		CategoryName  string               `json:"category_name"`
		UserName      string               `json:"user_name"`
	}

	var data detailData
	data.Appeal = appeal
	data.DamageReport = damageReport

	var order models.Order
	if err := db.Where("id = ? AND tenant_id = ?", damageReport.LeaseID, tenantID).First(&order).Error; err == nil {
		data.Order = &order

		var instrument models.Instrument
		if err := db.Where("id = ?", order.InstrumentID).First(&instrument).Error; err == nil {
			data.InstrumentSN = instrument.SN
			data.CategoryName = instrument.CategoryName
		}

		var user models.User
		if err := db.Where("id = ?", order.UserID).First(&user).Error; err == nil {
			data.UserName = user.Name
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": data,
	})
}

// PUT /api/merchant/appeals/:id/resolve - Resolve appeal (arbitration)
func (h *AppealHandler) ResolveAppeal(c *gin.Context) {
	var req struct {
		Decision     string  `json:"decision" binding:"required"` // no_damage, adjust, confirm
		AdjustAmount float64 `json:"adjust_amount"`
		Comment      string  `json:"comment" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	if req.Decision != "no_damage" && req.Decision != "adjust" && req.Decision != "confirm" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "decision must be no_damage, adjust, or confirm"})
		return
	}

	appealID := c.Param("id")
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)

	db := database.GetDB().WithContext(ctx)

	// Get appeal with damage report
	var appeal models.Appeal
	if err := db.Where("id = ? AND tenant_id = ?", appealID, tenantID).First(&appeal).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "appeal not found"})
		return
	}

	// Get associated damage report
	var damageReport models.DamageReport
	if err := db.Where("id = ?", appeal.DamageReportID).First(&damageReport).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "damage report not found"})
		return
	}

	// Get associated order (DamageReport.LeaseID stores orderID)
	var order models.Order
	if err := db.Where("id = ? AND tenant_id = ?", damageReport.LeaseID, tenantID).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "order not found"})
		return
	}

	now := time.Now()
	userID := middleware.GetUserID(ctx)

	// Calculate final amount and next state
	var finalAmount float64
	var nextOrderStatus string
	var notifType, notifTitle, notifContent, notifActionType string
	var notifActionData string

	switch req.Decision {
	case "no_damage":
		finalAmount = 0
		nextOrderStatus = models.OrderStatusCompleted
		damageReport.Status = "cancelled"
		damageReport.DepositDeducted = 0
		notifType = "appeal"
		notifActionType = "info"
		notifTitle = "申诉结果：无损坏"
		notifContent = "经理判定无损坏，订单将关闭"

	case "adjust":
		if req.AdjustAmount <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "adjust_amount required for adjust decision"})
			return
		}
		finalAmount = req.AdjustAmount
		damageReport.DepositDeducted = req.AdjustAmount

		if req.AdjustAmount < order.Deposit {
			nextOrderStatus = models.OrderStatusDepositRefunding
			notifType = "refund"
			notifActionType = "info"
			notifTitle = "申诉结果：金额调整"
			notifContent = fmt.Sprintf("调整后金额 ¥%.2f，押金 ¥%.2f，将退还差额 ¥%.2f", req.AdjustAmount, order.Deposit, order.Deposit-req.AdjustAmount)
		} else if req.AdjustAmount == order.Deposit {
			nextOrderStatus = models.OrderStatusCompleted
			notifType = "appeal"
			notifActionType = "info"
			notifTitle = "申诉结果"
			notifContent = fmt.Sprintf("调整后金额等于押金，订单关闭")
		} else {
			nextOrderStatus = models.OrderStatusCompleted
			notifType = "payment"
			notifActionType = "payment"
			notifTitle = "申诉结果：需支付"
			notifContent = fmt.Sprintf("调整后金额 ¥%.2f，押金 ¥%.2f，需支付差额 ¥%.2f", req.AdjustAmount, order.Deposit, req.AdjustAmount-order.Deposit)
		}
		damageReport.Status = "resolved"

	case "confirm":
		if damageReport.DamageAmount == nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "damage report has no amount"})
			return
		}
		finalAmount = *damageReport.DamageAmount
		damageReport.DepositDeducted = finalAmount

		if finalAmount < order.Deposit {
			nextOrderStatus = models.OrderStatusDepositRefunding
			notifType = "refund"
			notifActionType = "info"
			notifTitle = "申诉结果：金额确认"
			notifContent = fmt.Sprintf("确认定损金额 ¥%.2f，押金 ¥%.2f，将退还差额 ¥%.2f", finalAmount, order.Deposit, order.Deposit-finalAmount)
		} else if finalAmount == order.Deposit {
			nextOrderStatus = models.OrderStatusCompleted
			notifType = "appeal"
			notifActionType = "info"
			notifTitle = "申诉结果"
			notifContent = fmt.Sprintf("确认金额等于押金，订单关闭")
		} else {
			nextOrderStatus = models.OrderStatusCompleted
			notifType = "payment"
			notifActionType = "payment"
			notifTitle = "申诉结果：需支付"
			notifContent = fmt.Sprintf("确认金额 ¥%.2f，押金 ¥%.2f，需支付差额 ¥%.2f", finalAmount, order.Deposit, finalAmount-order.Deposit)
		}
		damageReport.Status = "resolved"
	}

	// Update appeal
	appeal.Status = "resolved"
	appeal.Resolution = req.Decision
	appeal.FinalAmount = &finalAmount
	appeal.ManagerComment = req.Comment
	appeal.ResolvedAt = &now

	// Save appeal
	if err := db.Save(&appeal).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to resolve appeal: " + err.Error()})
		return
	}

	// Save damage report
	if err := db.Save(&damageReport).Error; err != nil {
		log.Printf("[ResolveAppeal] Failed to update damage report: %v", err)
	}

	// Update order status
	if err := db.Model(&models.Order{}).Where("id = ? AND tenant_id = ?", order.ID, tenantID).Update("status", nextOrderStatus).Error; err != nil {
		log.Printf("[ResolveAppeal] Failed to update order status: %v", err)
	}

	// Update instrument if order completed
	if nextOrderStatus == models.OrderStatusCompleted {
		if err := db.Model(&models.Instrument{}).Where("id = ?", order.InstrumentID).Update("stock_status", models.StockStatusAvailable).Error; err != nil {
			log.Printf("[ResolveAppeal] Failed to update instrument status: %v", err)
		}
	}

	// Record status history
	history := models.OrderStatusHistory{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		OrderID:    order.ID,
		StatusFrom: order.Status,
		StatusTo:   nextOrderStatus,
		Notes:      fmt.Sprintf("申诉解决: %s - %s", req.Decision, req.Comment),
		ChangedBy:  stringPtr(userID),
		ChangedAt:  now,
	}
	if err := db.Create(&history).Error; err != nil {
		log.Printf("[ResolveAppeal] Failed to record status history: %v", err)
	}

	// Create notification
	notifActionData = fmt.Sprintf(`{"final_amount":%.2f,"deposit":%.2f,"order_id":"%s"}`, finalAmount, order.Deposit, order.ID)
	notification := models.Notification{
		TenantID:   tenantID,
		OrgID:      middleware.GetOrgID(ctx),
		UserID:     order.UserID,
		Type:       notifType,
		Title:      notifTitle,
		Content:    notifContent,
		RefID:      appeal.ID,
		RefType:    "appeal",
		ActionType: notifActionType,
		ActionData: notifActionData,
		Status:     "unread",
	}
	if err := db.Create(&notification).Error; err != nil {
		log.Printf("[ResolveAppeal] Failed to create notification: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data":    appeal,
	})
}

// POST /api/user/appeals - Submit appeal
func (h *AppealHandler) SubmitAppeal(c *gin.Context) {
	var req struct {
		DamageReportID string `json:"damage_report_id" binding:"required"`
		AppealReason   string `json:"appeal_reason" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	userID := middleware.GetUserID(ctx)

	db := database.GetDB().WithContext(ctx)

	// Verify damage report exists and belongs to user
	var damageReport models.DamageReport
	if err := db.Where("id = ? AND tenant_id = ? AND user_id = ?", req.DamageReportID, tenantID, userID).First(&damageReport).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "damage report not found"})
		return
	}

	// Update damage report status
	damageReport.Status = "appealed"
	if err := db.Save(&damageReport).Error; err != nil {
		log.Printf("[SubmitAppeal] Failed to update damage report: %v", err)
	}

	// Update order status to damage_appealing
	if err := db.Model(&models.Order{}).Where("id = ? AND tenant_id = ?", damageReport.LeaseID, tenantID).
		Update("status", models.OrderStatusDamageAppealing).Error; err != nil {
		log.Printf("[SubmitAppeal] Failed to update order status: %v", err)
	}

	// Record order status history
	now := time.Now()
	history := models.OrderStatusHistory{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		OrderID:    damageReport.LeaseID,
		StatusFrom: models.OrderStatusReturning,
		StatusTo:   models.OrderStatusDamageAppealing,
		Notes:      "用户申诉",
		ChangedBy:  stringPtr(userID),
		ChangedAt:  now,
	}
	if err := db.Create(&history).Error; err != nil {
		log.Printf("[SubmitAppeal] Failed to record status history: %v", err)
	}

	// Create appeal
	appeal := models.Appeal{
		ID:             uuid.New().String(),
		TenantID:       tenantID,
		OrgID:          middleware.GetOrgID(ctx),
		DamageReportID: req.DamageReportID,
		UserID:         userID,
		AppealReason:   req.AppealReason,
		Status:         "pending",
		SubmittedAt:    now,
	}

	if err := db.Create(&appeal).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create appeal: " + err.Error()})
		return
	}

	// Create notification
	notification := models.Notification{
		TenantID:   tenantID,
		OrgID:      middleware.GetOrgID(ctx),
		UserID:     userID,
		Type:       "appeal",
		Title:      "申诉已提交",
		Content:    fmt.Sprintf("您的申诉已提交，等待处理。申诉原因：%s", req.AppealReason),
		RefID:      appeal.ID,
		RefType:    "appeal",
		ActionType: "info",
		Status:     "unread",
	}
	if err := db.Create(&notification).Error; err != nil {
		log.Printf("[SubmitAppeal] Failed to create notification: %v", err)
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":    20000,
		"message": "success",
		"data":    appeal,
	})
}

// POST /api/user/appeals/:damage_id/agree - Agree to damage assessment
func (h *AppealHandler) AgreeDamage(c *gin.Context) {
	damageID := c.Param("id")
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	userID := middleware.GetUserID(ctx)

	db := database.GetDB().WithContext(ctx)

	// Verify damage report exists and belongs to user
	var damageReport models.DamageReport
	if err := db.Where("id = ? AND tenant_id = ? AND user_id = ?", damageID, tenantID, userID).First(&damageReport).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "damage report not found"})
		return
	}

	// Get associated order (DamageReport.LeaseID stores orderID)
	var order models.Order
	if err := db.Where("id = ? AND tenant_id = ?", damageReport.LeaseID, tenantID).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "order not found"})
		return
	}

	// Compare damage amount vs deposit
	damageAmount := float64(0)
	if damageReport.DamageAmount != nil {
		damageAmount = *damageReport.DamageAmount
	}
	now := time.Now()

	damageReport.Status = "agreed"

	var nextOrderStatus string
	var notifType, notifTitle, notifContent, notifActionType string

	if damageAmount < order.Deposit {
		// Deposit refund: damage < deposit
		nextOrderStatus = models.OrderStatusDepositRefunding
		damageReport.DepositDeducted = damageAmount
		notifType = "refund"
		notifActionType = "info"
		notifTitle = "押金退还通知"
		notifContent = fmt.Sprintf("定损金额 ¥%.2f，押金 ¥%.2f，将退还差额 ¥%.2f", damageAmount, order.Deposit, order.Deposit-damageAmount)
	} else {
		// Payment needed: damage >= deposit
		nextOrderStatus = models.OrderStatusCompleted
		damageReport.DepositDeducted = order.Deposit
		notifType = "payment"
		notifActionType = "info"
		notifTitle = "定损付款通知"
		notifContent = fmt.Sprintf("定损金额 ¥%.2f，押金 ¥%.2f，需支付差额 ¥%.2f", damageAmount, order.Deposit, damageAmount-order.Deposit)
	}

	// Update damage report
	if err := db.Save(&damageReport).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update damage report: " + err.Error()})
		return
	}

	// Update order status
	if err := db.Model(&models.Order{}).Where("id = ? AND tenant_id = ?", order.ID, tenantID).Update("status", nextOrderStatus).Error; err != nil {
		log.Printf("[AgreeDamage] Failed to update order status: %v", err)
	}

	// Update instrument if order completed
	if nextOrderStatus == models.OrderStatusCompleted {
		if err := db.Model(&models.Instrument{}).Where("id = ?", order.InstrumentID).Update("stock_status", models.StockStatusAvailable).Error; err != nil {
			log.Printf("[AgreeDamage] Failed to update instrument status: %v", err)
		}
	}

	// Record order status history
	history := models.OrderStatusHistory{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		OrderID:    order.ID,
		StatusFrom: order.Status,
		StatusTo:   nextOrderStatus,
		Notes:      "用户同意定损",
		ChangedBy:  stringPtr(userID),
		ChangedAt:  now,
	}
	if err := db.Create(&history).Error; err != nil {
		log.Printf("[AgreeDamage] Failed to record status history: %v", err)
	}

	// Create notification
	notification := models.Notification{
		TenantID:   tenantID,
		OrgID:      middleware.GetOrgID(ctx),
		UserID:     userID,
		Type:       notifType,
		Title:      notifTitle,
		Content:    notifContent,
		RefID:      order.ID,
		RefType:    "order",
		ActionType: notifActionType,
		ActionData: fmt.Sprintf(`{"damage_amount":%.2f,"deposit":%.2f,"order_id":"%s"}`, damageAmount, order.Deposit, order.ID),
		Status:     "unread",
	}
	if err := db.Create(&notification).Error; err != nil {
		log.Printf("[AgreeDamage] Failed to create notification: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "success",
		"data": gin.H{
			"damage_report":   damageReport,
			"order_status":    nextOrderStatus,
			"deposit_deducted": damageReport.DepositDeducted,
		},
	})
}

func float64Ptr(f float64) *float64 {
	return &f
}
