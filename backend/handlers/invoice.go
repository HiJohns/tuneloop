package handlers

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type InvoiceHandler struct{}

func NewInvoiceHandler() *InvoiceHandler {
	return &InvoiceHandler{}
}

// GET /user/invoices/eligible — orders completed + not yet invoice-applied for current user
func (h *InvoiceHandler) ListEligible(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "请先登录"})
		return
	}

	var orders []models.Order
	if err := db.Where("user_id = ? AND status = ? AND invoice_applied = false", userID, models.OrderStatusCompleted).
		Order("created_at DESC").
		Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "查询失败"})
		return
	}

	type eligibleOrder struct {
		OrderID       string `json:"order_id"`
		SN            string `json:"sn"`
		CreatedAt     string `json:"created_at"`
		TenantID      string `json:"tenant_id"`
		MerchantName  string `json:"merchant_name"`
		ActualCents   int64  `json:"actual_rent_cents"`
		OverdueCents  int64  `json:"overdue_cents"`
		TotalCents    int64  `json:"total_cents"`
	}
	var result []eligibleOrder

	// batch-load instruments for SN
	instrumentIDs := make([]string, 0, len(orders))
	for _, o := range orders {
		instrumentIDs = append(instrumentIDs, o.InstrumentID)
	}
	instrumentMap := map[string]string{} // id → sn
	if len(instrumentIDs) > 0 {
		var instruments []models.Instrument
		db.Where("id IN ?", instrumentIDs).Select("id, sn").Find(&instruments)
		for _, inst := range instruments {
			instrumentMap[inst.ID] = inst.SN
		}
	}

	// batch-load merchants for names
	tenantIDs := make([]string, 0)
	tenantSet := map[string]bool{}
	for _, o := range orders {
		if !tenantSet[o.TenantID] {
			tenantSet[o.TenantID] = true
			tenantIDs = append(tenantIDs, o.TenantID)
		}
	}
	merchantMap := map[string]string{} // tenant_id → name
	if len(tenantIDs) > 0 {
		var merchants []models.Merchant
		db.Where("tenant_id IN ?", tenantIDs).Select("tenant_id, name").Find(&merchants)
		for _, m := range merchants {
			merchantMap[m.TenantID] = m.Name
		}
	}

	for _, order := range orders {
		var settlement models.Settlement
		db.Where("order_id = ?", order.ID).Order("created_at DESC").First(&settlement)

		actualCents := int64(settlement.ActualRentAmount)
		overdueCents := int64(settlement.OverdueChargesTotal)

		// fallback: deriveActualRent if no settlement
		if actualCents == 0 {
			_, actualCents = deriveActualRent(db, &order, nil, nil)
		}

		createdAt := order.CreatedAt.Format(time.RFC3339)
		result = append(result, eligibleOrder{
			OrderID:      order.ID,
			SN:           instrumentMap[order.InstrumentID],
			CreatedAt:    createdAt,
			TenantID:     order.TenantID,
			MerchantName: merchantMap[order.TenantID],
			ActualCents:  actualCents,
			OverdueCents: overdueCents,
			TotalCents:   actualCents + overdueCents,
		})
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": result})
}

type invoiceSubmitGroup struct {
	TenantID string   `json:"tenant_id" binding:"required"`
	OrderIDs []string `json:"order_ids" binding:"required,min=1"`
}

type invoiceSubmitRequest struct {
	Groups []invoiceSubmitGroup `json:"groups" binding:"required,min=1"`
}

// POST /user/invoices — submit invoice applications grouped by merchant
func (h *InvoiceHandler) Submit(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "请先登录"})
		return
	}

	var req invoiceSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "参数错误: " + err.Error()})
		return
	}

	type applicationResult struct {
		ID         string `json:"id"`
		TenantID   string `json:"tenant_id"`
		TotalCents int64  `json:"total_amount"`
	}
	var results []applicationResult

	err := db.Transaction(func(tx *gorm.DB) error {
		for _, group := range req.Groups {
			if len(group.OrderIDs) == 0 {
				continue
			}

			// Verify all orders belong to current user + are completed + not yet applied
			var orders []models.Order
			if err := tx.Where("id IN ? AND user_id = ? AND status = ? AND invoice_applied = false",
				group.OrderIDs, userID, models.OrderStatusCompleted).Find(&orders).Error; err != nil {
				return fmt.Errorf("查询订单失败: %w", err)
			}
			if len(orders) != len(group.OrderIDs) {
				return fmt.Errorf("部分订单不属于您或不符合开票条件")
			}

			// Calculate total from settlements
			var totalCents int64
			for _, order := range orders {
				var settlement models.Settlement
				tx.Where("order_id = ?", order.ID).Order("created_at DESC").First(&settlement)
				actual := int64(settlement.ActualRentAmount)
				overdue := int64(settlement.OverdueChargesTotal)
				if actual == 0 {
					_, actual = deriveActualRent(tx, &order, nil, nil)
				}
				totalCents += actual + overdue
			}

			// Create application
			app := models.InvoiceApplication{
				ID:         uuid.New().String(),
				UserID:     userID,
				TenantID:   group.TenantID,
				Status:     "pending",
				TotalAmount: models.Cents(totalCents),
				OrderCount: len(orders),
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}
			if err := tx.Create(&app).Error; err != nil {
				return fmt.Errorf("创建申请失败: %w", err)
			}

			// Link orders
			for _, order := range orders {
				link := map[string]interface{}{
					"application_id": app.ID,
					"order_id":       order.ID,
				}
				if err := tx.Table("invoice_application_orders").Create(&link).Error; err != nil {
					return fmt.Errorf("关联订单失败: %w", err)
				}
			}

			// Mark orders as invoice-applied
			if err := tx.Model(&models.Order{}).Where("id IN ?", group.OrderIDs).
				Updates(map[string]interface{}{
					"invoice_applied":    true,
					"invoice_applied_at": time.Now(),
				}).Error; err != nil {
				return fmt.Errorf("更新订单状态失败: %w", err)
			}

			// Create notification to merchant admins
			// Find merchant admin user(s) — look up Merchant by tenant_id → AdminUID
			var merchant models.Merchant
			if err := tx.Where("tenant_id = ?", group.TenantID).First(&merchant).Error; err == nil && merchant.AdminUID != "" {
				notif := models.Notification{
					TenantID:   group.TenantID,
					OrgID:      group.TenantID,
					UserID:     merchant.AdminUID,
					Type:       "invoice",
					Title:      "新的发票申请",
					Content:    fmt.Sprintf("收到 %d 笔订单的发票申请，总金额 ¥%.2f", len(orders), float64(totalCents)/100),
					RefID:      app.ID,
					RefType:    "invoice",
					ActionType: "invoice_apply",
					Status:     "unread",
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				}
				actionData := fmt.Sprintf(`{"application_id":"%s"}`, app.ID)
				notif.ActionData = &actionData
				if err := tx.Create(&notif).Error; err != nil {
					log.Printf("[Invoice] Failed to create merchant notification: %v", err)
				}
			}

			results = append(results, applicationResult{
				ID:         app.ID,
				TenantID:   group.TenantID,
				TotalCents: totalCents,
			})
		}
		return nil
	})

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"applications": results}})
}

// GET /user/invoices — list all invoice applications for current user
func (h *InvoiceHandler) ListApplications(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "请先登录"})
		return
	}

	var apps []models.InvoiceApplication
	if err := db.Where("user_id = ?", userID).Order("created_at DESC").Find(&apps).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "查询失败"})
		return
	}

	// batch-load merchants
	tenantSet := map[string]bool{}
	for _, a := range apps {
		tenantSet[a.TenantID] = true
	}
	tenantIDs := make([]string, 0, len(tenantSet))
	for tid := range tenantSet {
		tenantIDs = append(tenantIDs, tid)
	}
	merchantMap := map[string]string{}
	if len(tenantIDs) > 0 {
		var merchants []models.Merchant
		db.Where("tenant_id IN ?", tenantIDs).Select("tenant_id, name").Find(&merchants)
		for _, m := range merchants {
			merchantMap[m.TenantID] = m.Name
		}
	}

	type appResponse struct {
		models.InvoiceApplication
		MerchantName string         `json:"merchant_name"`
		Orders       []orderDetail `json:"orders"`
	}

	var result []appResponse
	for _, app := range apps {
		var links []struct {
			OrderID string `gorm:"column:order_id"`
		}
		db.Table("invoice_application_orders").Where("application_id = ?", app.ID).Find(&links)

		orderIDs := make([]string, 0, len(links))
		for _, l := range links {
			orderIDs = append(orderIDs, l.OrderID)
		}

		var orders []orderDetail
		if len(orderIDs) > 0 {
			orders = h.loadOrderDetails(db, orderIDs)
		}

		ar := appResponse{
			InvoiceApplication: app,
			MerchantName:       merchantMap[app.TenantID],
			Orders:             orders,
		}
		result = append(result, ar)
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": result})
}

// GET /user/invoices/:id — single application detail
func (h *InvoiceHandler) GetApplication(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)
	appID := c.Param("id")
	if userID == "" || appID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "参数错误"})
		return
	}

	var app models.InvoiceApplication
	if err := db.Where("id = ? AND user_id = ?", appID, userID).First(&app).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "申请不存在"})
		return
	}

	// load merchant name
	var merchant models.Merchant
	db.Where("tenant_id = ?", app.TenantID).First(&merchant)

	// load linked orders
	var links []struct {
		OrderID string `gorm:"column:order_id"`
	}
	db.Table("invoice_application_orders").Where("application_id = ?", app.ID).Find(&links)
	orderIDs := make([]string, 0, len(links))
	for _, l := range links {
		orderIDs = append(orderIDs, l.OrderID)
	}
	orders := h.loadOrderDetails(db, orderIDs)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"id":            app.ID,
			"user_id":       app.UserID,
			"tenant_id":     app.TenantID,
			"status":        app.Status,
			"total_amount":  app.TotalAmount,
			"order_count":   app.OrderCount,
			"reply":         app.Reply,
			"invoice_file":  app.InvoiceFile,
			"replied_at":    app.RepliedAt,
			"created_at":    app.CreatedAt,
			"merchant_name": merchant.Name,
			"orders":        orders,
		},
	})
}

type orderDetail struct {
	OrderID      string `json:"order_id"`
	SN           string `json:"sn"`
	CreatedAt    string `json:"created_at"`
	ActualCents  int64  `json:"actual_rent_cents"`
	OverdueCents int64  `json:"overdue_cents"`
	TotalCents   int64  `json:"total_cents"`
}

func (h *InvoiceHandler) loadOrderDetails(db *gorm.DB, orderIDs []string) []orderDetail {
	var orders []models.Order
	db.Where("id IN ?", orderIDs).Find(&orders)

	// batch instruments
	instrumentIDs := make([]string, 0, len(orders))
	for _, o := range orders {
		instrumentIDs = append(instrumentIDs, o.InstrumentID)
	}
	snMap := map[string]string{}
	if len(instrumentIDs) > 0 {
		var instruments []models.Instrument
		db.Where("id IN ?", instrumentIDs).Select("id, sn").Find(&instruments)
		for _, inst := range instruments {
			snMap[inst.ID] = inst.SN
		}
	}

	var result []orderDetail
	for _, order := range orders {
		var settlement models.Settlement
		db.Where("order_id = ?", order.ID).Order("created_at DESC").First(&settlement)
		actual := int64(settlement.ActualRentAmount)
		overdue := int64(settlement.OverdueChargesTotal)
		if actual == 0 {
			_, actual = deriveActualRent(db, &order, nil, nil)
		}

		result = append(result, orderDetail{
			OrderID:      order.ID,
			SN:           snMap[order.InstrumentID],
			CreatedAt:    order.CreatedAt.Format(time.RFC3339),
			ActualCents:  actual,
			OverdueCents: overdue,
			TotalCents:   actual + overdue,
		})
	}
	return result
}