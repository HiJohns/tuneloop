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
	OrderID   string `json:"order_id" binding:"required"`
	OrderType string `json:"order_type" binding:"required"` // rent | repair | points | damage
	Amount    float64 `json:"amount" binding:"required"`
	OpenID    string  `json:"open_id,omitempty"` // for JSAPI
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

	validTypes := map[string]bool{"rent": true, "repair": true, "points": true, "damage": true}
	if !validTypes[req.OrderType] {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid order_type, must be rent/repair/points/damage"})
		return
	}

	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	userID := middleware.GetUserID(ctx)
	db := database.GetDB().WithContext(ctx)
	cfg := wechatpay.GetConfig()

	outTradeNo := fmt.Sprintf("%s%s%d", req.OrderType, uuid.New().String()[:8], time.Now().Unix())

	record := models.OrderPaymentRecord{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		UserID:     userID,
		OrderID:    &req.OrderID,
		OrderType:  req.OrderType,
		OutTradeNo: &outTradeNo,
		Amount:     req.Amount,
		Type:       "payment",
		Status:     "pending",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if cfg.MockMode {
		record.Status = "paid"
		record.Method = strPtr("mock")
		now := time.Now()
		record.UpdatedAt = now

		if err := db.Create(&record).Error; err != nil {
			log.Printf("[PrepayOrder] failed to save payment record: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create payment record"})
			return
		}

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
				Description: fmt.Sprintf("TuneLoop %s order", req.OrderType),
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
			Description: fmt.Sprintf("TuneLoop %s order", req.OrderType),
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
			Description: "TuneLoop 预付点充值",
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
