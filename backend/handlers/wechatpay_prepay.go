package handlers

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services/wechatpay"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type PrepayRequest struct {
	OrderID     string  `json:"order_id"` // required for rent/repair/damage; empty allowed for points/renewal
	OrderType   string  `json:"order_type" binding:"required"` // rent | repair | damage | renewal | membership
	Amount      float64 `json:"amount" binding:"required"`
	OpenID      string  `json:"open_id,omitempty"`
	PrepaidUsed float64 `json:"prepaid_used"`
	GiftUsed    float64 `json:"gift_used"`
}

type PrepayResponse struct {
	Mock    bool            `json:"mock,omitempty"`
	Success bool            `json:"success"`
	Message string          `json:"message,omitempty"`
	Data    *PrepayData     `json:"data,omitempty"`
}

type PrepayData struct {
	OutTradeNo   string `json:"out_trade_no"`
	PrepayID     string `json:"prepay_id,omitempty"`
	CodeURL      string `json:"code_url,omitempty"`
	H5URL        string `json:"h5_url,omitempty"`
	AppID        string `json:"app_id,omitempty"`
	TimeStamp    string `json:"time_stamp,omitempty"`
	NonceStr     string `json:"nonce_str,omitempty"`
	Package      string `json:"package,omitempty"`
	SignType     string `json:"sign_type,omitempty"`
	PaySign      string `json:"pay_sign,omitempty"`
}

func PrepayOrder(c *gin.Context) {
	var req PrepayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	validTypes := map[string]bool{"rent": true, "repair": true, "damage": true, "renewal": true, "membership": true}
	if !validTypes[req.OrderType] {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid order_type, must be rent/repair/damage/renewal/membership"})
		return
	}

	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	userID := middleware.GetUserID(ctx)
	db := database.GetDB().WithContext(ctx)
	cfg := wechatpay.GetConfig()

	// For customer (USER) JWT, tenantID is empty — derive from the order
	if tenantID == "" && req.OrderID != "" {
		var order struct{ TenantID string }
		if err := db.Table("orders").Select("tenant_id").Where("id = ?", req.OrderID).Scan(&order).Error; err == nil {
			tenantID = order.TenantID
		}
	}

	outTradeNo := fmt.Sprintf("%s%s%d", req.OrderType, uuid.New().String()[:8], time.Now().Unix())

	// points/membership payments have no pre-existing order: OrderID
	// holds the local user id, resolved from the JWT (iam_sub).
	effectiveOrderID := req.OrderID
	if (req.OrderType == "points" || req.OrderType == "membership") && effectiveOrderID == "" {
		var localUser models.User
		if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err == nil {
			effectiveOrderID = localUser.ID
			// Customer JWT carries no tid; use the local cache's tenant
			// (zero UUID is valid and satisfies the not-null uuid column).
			if tenantID == "" {
				tenantID = localUser.TenantID
			}
		} else {
			effectiveOrderID = userID
		}
	}

	// Last-resort: never persist an empty string into a uuid column
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000000"
	}

	record := models.OrderPaymentRecord{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		UserID:     userID,
		OrderID:    &effectiveOrderID,
		OrderType:  req.OrderType,
		OutTradeNo: &outTradeNo,
		Amount:     req.Amount,
		Type:       "payment",
		Status:     "pending",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Store points usage on the payment record for callback consumption
	if req.PrepaidUsed > 0 || req.GiftUsed > 0 {
		raw := fmt.Sprintf(`{"prepaid_used":%.2f,"gift_used":%.2f}`, req.PrepaidUsed, req.GiftUsed)
		record.RawResponse = &raw
	}

	if cfg.MockMode {
		record.Status = "paid"
		record.Method = strPtr("mock")
		now := time.Now()
		record.UpdatedAt = now

		tx := db.Begin()
		if err := tx.Create(&record).Error; err != nil {
			tx.Rollback()
			log.Printf("[PrepayOrder] failed to save payment record: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create payment record"})
			return
		}

		if err := applySideEffects(tx, &record, now); err != nil {
			tx.Rollback()
			log.Printf("[PrepayOrder] side effects failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "payment side effects failed"})
			return
		}
		tx.Commit()

		c.JSON(http.StatusOK, gin.H{
			"code": 20000,
			"data": PrepayResponse{
				Mock:    true,
				Success: true,
				Data: &PrepayData{
					OutTradeNo: outTradeNo,
				},
			},
		})
		return
	}

	client := wechatpay.GetClient()

	switch req.OrderType {
	case "rent", "repair", "damage":
		if req.OpenID == "" {
			// Native payment (QR code) for PC
			result, err := client.CreateNativeOrder(ctx, wechatpay.NativeParams{
				OutTradeNo:  outTradeNo,
				TotalAmount: cfg.AmountToCents(req.Amount),
			Description: fmt.Sprintf("乐器租赁订单"),
			NotifyURL:   cfg.NotifyURL,
		})
		if err != nil {
			record.Status = "failed"
			fr := err.Error()
			record.FailReason = &fr
			db.Create(&record)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create payment: " + err.Error()})
			return
		}
		record.Method = strPtr("native")
		record.CodeURL = &result.CodeURL
		db.Create(&record)
		c.JSON(http.StatusOK, gin.H{
			"code": 20000,
			"data": PrepayResponse{
				Success: true,
				Data: &PrepayData{
					OutTradeNo: outTradeNo,
					CodeURL:    result.CodeURL,
				},
			},
		})
		return
		}

		// JSAPI payment (mini-program)
		result, err := client.CreateJSAPIOrder(ctx, wechatpay.JSAPIParams{
			OutTradeNo:  outTradeNo,
			OpenID:      req.OpenID,
			TotalAmount: cfg.AmountToCents(req.Amount),
			Description: fmt.Sprintf("乐器租赁订单"),
			NotifyURL:   cfg.NotifyURL,
		})
		if err != nil {
			record.Status = "failed"
			fr := err.Error()
			record.FailReason = &fr
			db.Create(&record)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create payment: " + err.Error()})
			return
		}
		record.Method = strPtr("jsapi")
		record.PrepayID = &result.PrepayID
		db.Create(&record)
		c.JSON(http.StatusOK, gin.H{
			"code": 20000,
			"data": PrepayResponse{
				Success: true,
				Data: &PrepayData{
					OutTradeNo: outTradeNo,
					PrepayID:   result.PrepayID,
					AppID:      cfg.AppID,
					TimeStamp:  result.TimeStamp,
					NonceStr:   result.NonceStr,
					Package:    result.Package,
					SignType:   result.SignType,
					PaySign:    result.Sign,
				},
			},
		})

	case "points":
		result, err := client.CreateJSAPIOrder(ctx, wechatpay.JSAPIParams{
			OutTradeNo:  outTradeNo,
			OpenID:      req.OpenID,
			TotalAmount: cfg.AmountToCents(req.Amount),
			Description: "预付点充值",
			NotifyURL:   cfg.NotifyURL,
		})
		if err != nil {
			record.Status = "failed"
			fr := err.Error()
			record.FailReason = &fr
			db.Create(&record)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create payment: " + err.Error()})
			return
		}
		record.Method = strPtr("jsapi")
		record.PrepayID = &result.PrepayID
		db.Create(&record)
		c.JSON(http.StatusOK, gin.H{
			"code": 20000,
			"data": PrepayResponse{
				Success: true,
				Data: &PrepayData{
					OutTradeNo: outTradeNo,
					PrepayID:   result.PrepayID,
					AppID:      cfg.AppID,
					TimeStamp:  result.TimeStamp,
					NonceStr:   result.NonceStr,
					Package:    result.Package,
					SignType:   result.SignType,
					PaySign:    result.Sign,
				},
			},
		})

	case "renewal":
		if req.OpenID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "renewal payment requires open_id"})
			return
		}
		result, err := client.CreateJSAPIOrder(ctx, wechatpay.JSAPIParams{
			OutTradeNo:  outTradeNo,
			OpenID:      req.OpenID,
			TotalAmount: cfg.AmountToCents(req.Amount),
			Description: "租赁续期支付",
			NotifyURL:   cfg.NotifyURL,
		})
		if err != nil {
			record.Status = "failed"
			fr := err.Error()
			record.FailReason = &fr
			db.Create(&record)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create payment: " + err.Error()})
			return
		}
		record.Method = strPtr("jsapi")
		record.PrepayID = &result.PrepayID
		db.Create(&record)
		c.JSON(http.StatusOK, gin.H{
			"code": 20000,
			"data": PrepayResponse{
				Success: true,
				Data: &PrepayData{
					OutTradeNo: outTradeNo,
					PrepayID:   result.PrepayID,
					AppID:      cfg.AppID,
					TimeStamp:  result.TimeStamp,
					NonceStr:   result.NonceStr,
					Package:    result.Package,
					SignType:   result.SignType,
					PaySign:    result.Sign,
				},
			},
		})
	}
}

func strPtr(s string) *string { return &s }

// GetPayConfig returns the WeChat Pay mock mode flag so clients can decide
// whether to show the simulate pay/refund buttons (#1498).
func GetPayConfig(c *gin.Context) {
	mockMode := false
	if cfg := wechatpay.GetConfig(); cfg != nil {
		mockMode = cfg.MockMode
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"mock_payment": mockMode,
		},
	})
}

// QueryPayment handles POST /api/pay/query
func QueryPayment(c *gin.Context) {
	var req struct {
		OutTradeNo string `json:"out_trade_no" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
		return
	}

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	var record models.OrderPaymentRecord
	if err := db.Where("out_trade_no = ?", req.OutTradeNo).First(&record).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "payment record not found"})
		return
	}

	client := wechatpay.GetClient()
	wxResult, err := client.QueryOrder(ctx, req.OutTradeNo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "query failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"out_trade_no":    req.OutTradeNo,
			"trade_state":     wxResult.TradeState,
			"transaction_id":  wxResult.TransactionID,
			"paid":            wxResult.TradeState == "SUCCESS",
		},
	})
}
