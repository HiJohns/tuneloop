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
	"gorm.io/gorm"
)

type MerchantInvoiceHandler struct{}

func NewMerchantInvoiceHandler() *MerchantInvoiceHandler {
	return &MerchantInvoiceHandler{}
}

// GET /merchant/invoices — list applications for current merchant
func (h *MerchantInvoiceHandler) ListApplications(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)
	if tenantID == "" {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "无商户权限"})
		return
	}

	statusFilter := c.Query("status")
	query := db.Where("tenant_id = ?", tenantID)
	if statusFilter != "" {
		query = query.Where("status = ?", statusFilter)
	}

	var apps []models.InvoiceApplication
	if err := query.Order("created_at DESC").Find(&apps).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "查询失败"})
		return
	}

	// Load customer names
	userIDs := make([]string, 0)
	userSet := map[string]bool{}
	for _, a := range apps {
		if !userSet[a.UserID] {
			userSet[a.UserID] = true
			userIDs = append(userIDs, a.UserID)
		}
	}
	userNameMap := map[string]string{}
	if len(userIDs) > 0 {
		var users []models.User
		db.Where("id IN ?", userIDs).Select("id, name").Find(&users)
		for _, u := range users {
			userNameMap[u.ID] = u.Name
		}
	}

	type appResp struct {
		models.InvoiceApplication
		CustomerName string         `json:"customer_name"`
		Orders       []orderDetail  `json:"orders"`
	}

	var result []appResp
	for _, app := range apps {
		var links []struct {
			OrderID string `gorm:"column:order_id"`
		}
		db.Table("invoice_application_orders").Where("application_id = ?", app.ID).Find(&links)

		orderIDs := make([]string, 0, len(links))
		for _, l := range links {
			orderIDs = append(orderIDs, l.OrderID)
		}

		orders := h.loadOrderDetails(db, orderIDs)
		ar := appResp{
			InvoiceApplication: app,
			CustomerName:       userNameMap[app.UserID],
			Orders:             orders,
		}
		result = append(result, ar)
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": result})
}

// POST /merchant/invoices/:id/reply — reply with invoice file
func (h *MerchantInvoiceHandler) Reply(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)
	appID := c.Param("id")
	if tenantID == "" || appID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "参数错误"})
		return
	}

	var req struct {
		Reply       string `json:"reply"`
		InvoiceFile string `json:"invoice_file"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "参数错误: " + err.Error()})
		return
	}
	if req.InvoiceFile == "" && req.Reply == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "回复内容和发票文件不能同时为空"})
		return
	}

	// Verify application belongs to this tenant
	var app models.InvoiceApplication
	if err := db.Where("id = ? AND tenant_id = ?", appID, tenantID).First(&app).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "申请不存在"})
		return
	}

	if app.Status == "replied" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "该申请已回复"})
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":      "replied",
		"replied_at":  now,
		"updated_at":  now,
	}
	if req.Reply != "" {
		updates["reply"] = req.Reply
	}
	if req.InvoiceFile != "" {
		updates["invoice_file"] = req.InvoiceFile
	}

	if err := db.Model(&models.InvoiceApplication{}).Where("id = ?", appID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "回复失败"})
		return
	}

	// Notify customer
	actionData := fmt.Sprintf(`{"application_id":"%s"}`, appID)
	notif := models.Notification{
		TenantID:   tenantID,
		OrgID:      tenantID,
		UserID:     app.UserID,
		Type:       "invoice",
		Title:      "发票申请已回复",
		Content:    "您的发票申请已处理，请查看发票附件。",
		RefID:      appID,
		RefType:    "invoice",
		ActionType: "invoice_reply",
		ActionData: &actionData,
		Status:     "unread",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := db.Create(&notif).Error; err != nil {
		// log but don't fail the reply
		log.Printf("[MerchantInvoice] Failed to create customer notification: %v", err)
	}

	// Return updated detail
	var merchant models.Merchant
	db.Where("tenant_id = ?", tenantID).First(&merchant)

	var links []struct {
		OrderID string `gorm:"column:order_id"`
	}
	db.Table("invoice_application_orders").Where("application_id = ?", appID).Find(&links)
	orderIDs := make([]string, 0, len(links))
	for _, l := range links {
		orderIDs = append(orderIDs, l.OrderID)
	}
	orders := h.loadOrderDetails(db, orderIDs)

	// reload app
	db.Where("id = ?", appID).First(&app)

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "回复成功",
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

// GET /merchant/invoices/:id — single application detail
func (h *MerchantInvoiceHandler) GetApplication(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tenantID := middleware.GetTenantID(ctx)
	appID := c.Param("id")
	if tenantID == "" || appID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "参数错误"})
		return
	}

	var app models.InvoiceApplication
	if err := db.Where("id = ? AND tenant_id = ?", appID, tenantID).First(&app).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "申请不存在"})
		return
	}

	var merchant models.Merchant
	db.Where("tenant_id = ?", tenantID).First(&merchant)

	var links []struct {
		OrderID string `gorm:"column:order_id"`
	}
	db.Table("invoice_application_orders").Where("application_id = ?", app.ID).Find(&links)
	orderIDs := make([]string, 0, len(links))
	for _, l := range links {
		orderIDs = append(orderIDs, l.OrderID)
	}
	orders := h.loadOrderDetails(db, orderIDs)

	// customer name
	var user models.User
	db.Where("id = ?", app.UserID).Select("id, name").First(&user)

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
			"customer_name": user.Name,
			"orders":        orders,
		},
	})
}

func (h *MerchantInvoiceHandler) loadOrderDetails(db *gorm.DB, orderIDs []string) []orderDetail {
	var orders []models.Order
	db.Where("id IN ?", orderIDs).Find(&orders)

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
