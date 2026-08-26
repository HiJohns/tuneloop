package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

func setupInvoiceTestData(t *testing.T, tenantID, userID, orgID string) (string, string) {
	t.Helper()
	db := database.GetDB()

	instrument := models.Instrument{
		TenantID:    tenantID,
		OrgID:       &orgID,
		SN:          "INV-" + uuid.New().String()[:8],
		StockStatus: "available",
	}
	require.NoError(t, db.Create(&instrument).Error)

	order1 := models.Order{
		TenantID:     tenantID,
		OrgID:        orgID,
		UserID:       userID,
		InstrumentID: instrument.ID,
		LeaseTerm:    30,
		MonthlyRent:  500000,
		Status:       models.OrderStatusCompleted,
	}
	require.NoError(t, db.Create(&order1).Error)

	settlement := models.Settlement{
		OrderID:             order1.ID,
		ActualRentDays:      25,
		ActualRentAmount:    416700,
		OverdueChargesTotal: 0,
	}
	require.NoError(t, db.Create(&settlement).Error)

	order2 := models.Order{
		TenantID:     tenantID,
		OrgID:        orgID,
		UserID:       userID,
		InstrumentID: instrument.ID,
		LeaseTerm:    30,
		MonthlyRent:  300000,
		Status:       models.OrderStatusInLease,
	}
	require.NoError(t, db.Create(&order2).Error)

	return order1.ID, order2.ID
}

func invoiceRouter(tenantID, userID, orgID string) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := NewInvoiceHandler()
	mh := NewMerchantInvoiceHandler()
	router.GET("/api/user/invoices/eligible", h.ListEligible)
	router.POST("/api/user/invoices", h.Submit)
	router.GET("/api/user/invoices", h.ListApplications)
	router.GET("/api/user/invoices/:id", h.GetApplication)
	router.GET("/api/merchant/invoices", mh.ListApplications)
	router.GET("/api/merchant/invoices/:id", mh.GetApplication)
	router.POST("/api/merchant/invoices/:id/reply", mh.Reply)
	return router
}

func TestInvoice_EligibleList(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000101"
	userID := "00000000-0000-0000-0000-000000000102"
	orgID := "00000000-0000-0000-0000-000000000103"

	db := database.GetDB()
	merchant := models.Merchant{
		ID:       uuid.New().String(),
		TenantID: tenantID,
		OrgID:    tenantID,
		Name:     "测试商户",
		AdminUID: uuid.New().String(),
		Status:   "active",
	}
	require.NoError(t, db.Create(&merchant).Error)

	completedOrderID, _ := setupInvoiceTestData(t, tenantID, userID, orgID)

	router := invoiceRouter(tenantID, userID, orgID)

	req := httptest.NewRequest("GET", "/api/user/invoices/eligible", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data []struct {
			OrderID      string `json:"order_id"`
			ActualCents  int64  `json:"actual_rent_cents"`
			OverdueCents int64  `json:"overdue_cents"`
			TotalCents   int64  `json:"total_cents"`
			MerchantName string `json:"merchant_name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Len(t, resp.Data, 1, "only completed order should be eligible")
	require.Equal(t, completedOrderID, resp.Data[0].OrderID)
	require.Equal(t, int64(416700), resp.Data[0].TotalCents)
}

func TestInvoice_Submit(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000201"
	userID := "00000000-0000-0000-0000-000000000202"
	orgID := "00000000-0000-0000-0000-000000000203"

	db := database.GetDB()
	merchant := models.Merchant{
		ID:       uuid.New().String(),
		TenantID: tenantID,
		OrgID:    tenantID,
		Name:     "测试商户",
		AdminUID: uuid.New().String(),
		Status:   "active",
	}
	require.NoError(t, db.Create(&merchant).Error)

	completedOrderID, _ := setupInvoiceTestData(t, tenantID, userID, orgID)

	router := invoiceRouter(tenantID, userID, orgID)

	body, _ := json.Marshal(map[string]interface{}{
		"groups": []map[string]interface{}{
			{"tenant_id": tenantID, "order_ids": []string{completedOrderID}},
		},
	})
	req := httptest.NewRequest("POST", "/api/user/invoices", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Applications []struct {
				ID       string `json:"id"`
				TenantID string `json:"tenant_id"`
			} `json:"applications"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Len(t, resp.Data.Applications, 1)
	require.Equal(t, tenantID, resp.Data.Applications[0].TenantID)

	// Verify order marked as invoice-applied
	var order models.Order
	require.NoError(t, db.Where("id = ?", completedOrderID).First(&order).Error)
	require.True(t, order.InvoiceApplied)
	require.NotNil(t, order.InvoiceAppliedAt)

	// Verify notification created
	var notifCount int64
	db.Model(&models.Notification{}).Where("type = ? AND ref_id = ?", "invoice", resp.Data.Applications[0].ID).Count(&notifCount)
	require.Equal(t, int64(1), notifCount)
}

func TestInvoice_SubmitValidation(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000301"
	userID := "00000000-0000-0000-0000-000000000302"
	orgID := "00000000-0000-0000-0000-000000000303"

	db := database.GetDB()
	merchant := models.Merchant{
		ID:       uuid.New().String(),
		TenantID: tenantID,
		OrgID:    tenantID,
		Name:     "测试商户",
		AdminUID: uuid.New().String(),
		Status:   "active",
	}
	require.NoError(t, db.Create(&merchant).Error)

	_, nonCompletedOrderID := setupInvoiceTestData(t, tenantID, userID, orgID)

	router := invoiceRouter(tenantID, userID, orgID)

	// non-completed order → should fail
	body, _ := json.Marshal(map[string]interface{}{
		"groups": []map[string]interface{}{
			{"tenant_id": tenantID, "order_ids": []string{nonCompletedOrderID}},
		},
	})
	req := httptest.NewRequest("POST", "/api/user/invoices", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, 400, w.Code)
}

func TestInvoice_MerchantReply(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000401"
	userID := "00000000-0000-0000-0000-000000000402"
	orgID := "00000000-0000-0000-0000-000000000403"
	merchantAdminID := "00000000-0000-0000-0000-000000000404"

	db := database.GetDB()
	merchant := models.Merchant{
		ID:       uuid.New().String(),
		TenantID: tenantID,
		OrgID:    tenantID,
		Name:     "测试商户",
		AdminUID: merchantAdminID,
		Status:   "active",
	}
	require.NoError(t, db.Create(&merchant).Error)

	completedOrderID, _ := setupInvoiceTestData(t, tenantID, userID, orgID)

	app := models.InvoiceApplication{
		ID:          uuid.New().String(),
		UserID:      userID,
		TenantID:    tenantID,
		Status:      "pending",
		TotalAmount: 416700,
		OrderCount:  1,
	}
	require.NoError(t, db.Create(&app).Error)

	link := map[string]interface{}{
		"application_id": app.ID,
		"order_id":       completedOrderID,
	}
	require.NoError(t, db.Table("invoice_application_orders").Create(&link).Error)

	require.NoError(t, db.Model(&models.Order{}).Where("id = ?", completedOrderID).
		Updates(map[string]interface{}{"invoice_applied": true}).Error)

	// Router uses merchant admin's identity
	router := invoiceRouter(tenantID, merchantAdminID, orgID)

	body, _ := json.Marshal(map[string]interface{}{
		"reply":        "发票已开具",
		"invoice_file": "https://example.com/invoice.pdf",
	})
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/merchant/invoices/%s/reply", app.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Status      string  `json:"status"`
			Reply       string  `json:"reply"`
			InvoiceFile string  `json:"invoice_file"`
			RepliedAt   *string `json:"replied_at"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.Equal(t, "replied", resp.Data.Status)
	require.Equal(t, "发票已开具", resp.Data.Reply)
	require.Equal(t, "https://example.com/invoice.pdf", resp.Data.InvoiceFile)
	require.NotNil(t, resp.Data.RepliedAt)

	// Verify notification to customer
	var notifCount int64
	db.Model(&models.Notification{}).
		Where("user_id = ? AND type = ? AND action_type = ?", userID, "invoice", "invoice_reply").
		Count(&notifCount)
	require.Equal(t, int64(1), notifCount)
}

func TestInvoice_MerchantReply_CrossTenant(t *testing.T) {
	tenantID := "00000000-0000-0000-0000-000000000501"
	userID := "00000000-0000-0000-0000-000000000502"
	orgID := "00000000-0000-0000-0000-000000000503"
	otherTenantID := "00000000-0000-0000-0000-000000000504"

	db := database.GetDB()
	merchant := models.Merchant{
		ID:       uuid.New().String(),
		TenantID: tenantID,
		OrgID:    tenantID,
		Name:     "测试商户",
		AdminUID: uuid.New().String(),
		Status:   "active",
	}
	require.NoError(t, db.Create(&merchant).Error)

	completedOrderID, _ := setupInvoiceTestData(t, tenantID, userID, orgID)

	app := models.InvoiceApplication{
		ID:          uuid.New().String(),
		UserID:      userID,
		TenantID:    tenantID,
		Status:      "pending",
		TotalAmount: 416700,
		OrderCount:  1,
	}
	require.NoError(t, db.Create(&app).Error)

	link := map[string]interface{}{
		"application_id": app.ID,
		"order_id":       completedOrderID,
	}
	require.NoError(t, db.Table("invoice_application_orders").Create(&link).Error)

	// Router uses a DIFFERENT tenant identity
	router := invoiceRouter(otherTenantID, uuid.New().String(), otherTenantID)

	body, _ := json.Marshal(map[string]interface{}{
		"reply":        "发票已开具",
		"invoice_file": "https://example.com/invoice.pdf",
	})
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/merchant/invoices/%s/reply", app.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, 404, w.Code, "cross-tenant reply should return 404")
}
