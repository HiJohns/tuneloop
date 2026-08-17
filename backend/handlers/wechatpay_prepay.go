package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services/wechatpay"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type PrepayRequest struct {
	OrderID    string  `json:"order_id"`                      // required for rent/repair/damage; empty allowed for renewal/membership
	OrderType  string  `json:"order_type" binding:"required"` // rent | repair | damage | renewal | membership
	Amount     float64 `json:"amount"`                        // membership: client base fee ignored — server re-prices from session+coupon (#1682)
	OpenID     string  `json:"open_id,omitempty"`
	GiftUsed   float64 `json:"gift_used"`
	SessionID  string  `json:"session_id,omitempty"`  // membership: two-phase registration session (#1663)
	CouponCode string  `json:"coupon_code,omitempty"` // membership: discount code (OREZ/ENO)
}

type PrepayResponse struct {
	Mock    bool        `json:"mock,omitempty"`
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    *PrepayData `json:"data,omitempty"`
}

type PrepayData struct {
	OutTradeNo string `json:"out_trade_no"`
	PrepayID   string `json:"prepay_id,omitempty"`
	CodeURL    string `json:"code_url,omitempty"`
	H5URL      string `json:"h5_url,omitempty"`
	AppID      string `json:"app_id,omitempty"`
	TimeStamp  string `json:"time_stamp,omitempty"`
	NonceStr   string `json:"nonce_str,omitempty"`
	Package    string `json:"package,omitempty"`
	SignType   string `json:"sign_type,omitempty"`
	PaySign    string `json:"pay_sign,omitempty"`
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

	// membership payments have no pre-existing order: OrderID
	// holds the local user id, resolved from the JWT (iam_sub).
	// Two-phase session flow (#1663): the user does NOT exist locally yet
	// (no-orphan principle) → OrderID stays nil (uuid column must be null).
	effectiveOrderID := req.OrderID
	if req.OrderType == "membership" && req.SessionID == "" && effectiveOrderID == "" {
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
	// Session flow (#1663): the payer has no account yet (no-orphan
	// principle) — JWT user_id may be empty; store the zero UUID.
	if req.OrderType == "membership" && req.SessionID != "" && userID == "" {
		userID = "00000000-0000-0000-0000-000000000000"
	}

	record := models.OrderPaymentRecord{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		UserID:     userID,
		OrderType:  req.OrderType,
		OutTradeNo: &outTradeNo,
		Amount:     req.Amount,
		Type:       "payment",
		Status:     "pending",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if effectiveOrderID != "" {
		record.OrderID = &effectiveOrderID
	}

	// Store gift points usage on the payment record for callback consumption
	if req.GiftUsed > 0 {
		raw := fmt.Sprintf(`{"gift_used":%.2f}`, req.GiftUsed)
		record.RawResponse = &raw
	}

	// Two-phase registration session flow (#1663): the membership fee is
	// priced server-side from the session + optional coupon — the client's
	// amount is ignored (计费以后端为准).
	sessionFlow := req.OrderType == "membership" && req.SessionID != ""
	sessionAmount := req.Amount

	// Anonymous guard (#1682 regression): every flow except the two-phase
	// membership session requires a logged-in user. An empty user_id would
	// violate the uuid column and (with the previously unchecked Create) the
	// payment record silently never persisted — the WeChat callback then can
	// never find the payment and orders stay stuck after a real charge.
	if !sessionFlow && userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "请先登录"})
		return
	}

	var session models.RegistrationSession
	if sessionFlow {
		if err := db.Where("id = ? AND status = ?", req.SessionID, "pending").First(&session).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "registration session not found or not pending"})
			return
		}
		// #1688: the payment record's user_id must reference the reserved
		// local user (consumption records queryable after activation), not
		// the zero UUID.
		if session.LocalUserID != nil {
			record.UserID = *session.LocalUserID
		}
		amount := session.Amount
		couponCode := ""
		if req.CouponCode != "" {
			var coupon models.Coupon
			if err := db.Where("code = ? AND active = ?", strings.ToUpper(req.CouponCode), true).First(&coupon).Error; err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid coupon code"})
				return
			}
			switch coupon.Type {
			case "waive":
				amount = 0
			case "percent":
				amount = math.Round(amount*coupon.Value/100*100) / 100
			default:
				c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "unsupported coupon type"})
				return
			}
			couponCode = coupon.Code
		}
		sessionAmount = amount
		record.Amount = amount // server-priced; never trust the client amount
		// Dedicated column: the session link must survive the payment
		// callback, which overwrites RawResponse with the callback result
		// (#1664 audit: real-callback session_id was lost in RawResponse).
		record.SessionID = &req.SessionID
		sessionRaw := map[string]interface{}{"session_id": req.SessionID, "original_amount": session.Amount}
		if couponCode != "" {
			sessionRaw["coupon_code"] = couponCode
		}
		if rawJSON, err := json.Marshal(sessionRaw); err == nil {
			if record.RawResponse != nil {
				var existing map[string]interface{}
				if json.Unmarshal([]byte(*record.RawResponse), &existing) == nil {
					for k, v := range sessionRaw {
						existing[k] = v
					}
					if merged, err := json.Marshal(existing); err == nil {
						mergedStr := string(merged)
						record.RawResponse = &mergedStr
					}
				}
			} else {
				rawStr := string(rawJSON)
				record.RawResponse = &rawStr
			}
		}
	}

	// Zero-amount guard (#1682): the membership session flow re-prices
	// server-side (coupon OREZ → 0 waive handled below); every other flow
	// must carry a positive amount — a 0-amount real WeChat order is invalid.
	if !sessionFlow && req.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "amount must be positive"})
		return
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

	// OREZ full waiver (amount = 0): no WeChat payment is created; the
	// record is booked as paid and the callback-equivalent side effects run
	// (server-side account creation) — same path as the mock mode above.
	if sessionFlow && sessionAmount == 0 {
		record.Status = "paid"
		record.Method = strPtr("waived")
		now := time.Now()
		record.UpdatedAt = now

		tx := db.Begin()
		if err := tx.Create(&record).Error; err != nil {
			tx.Rollback()
			log.Printf("[PrepayOrder] failed to save waiver payment record: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create payment record"})
			return
		}
		if err := applySideEffects(tx, &record, now); err != nil {
			tx.Rollback()
			log.Printf("[PrepayOrder] waiver side effects failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "payment side effects failed"})
			return
		}
		tx.Commit()

		c.JSON(http.StatusOK, gin.H{
			"code": 20000,
			"data": PrepayResponse{
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
				if err := db.Create(&record).Error; err != nil {
					log.Printf("[PrepayOrder] failed to save payment record: %v", err)
					c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to save payment record"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create payment: " + err.Error()})
				return
			}
			record.Method = strPtr("native")
			record.CodeURL = &result.CodeURL
			if err := db.Create(&record).Error; err != nil {
				log.Printf("[PrepayOrder] failed to save payment record: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to save payment record"})
				return
			}
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
			if err := db.Create(&record).Error; err != nil {
				log.Printf("[PrepayOrder] failed to save payment record: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to save payment record"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create payment: " + err.Error()})
			return
		}
		record.Method = strPtr("jsapi")
		record.PrepayID = &result.PrepayID
		if err := db.Create(&record).Error; err != nil {
			log.Printf("[PrepayOrder] failed to save payment record: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to save payment record"})
			return
		}
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
			if err := db.Create(&record).Error; err != nil {
				log.Printf("[PrepayOrder] failed to save payment record: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to save payment record"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create payment: " + err.Error()})
			return
		}
		record.Method = strPtr("jsapi")
		record.PrepayID = &result.PrepayID
		if err := db.Create(&record).Error; err != nil {
			log.Printf("[PrepayOrder] failed to save payment record: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to save payment record"})
			return
		}
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

	case "membership":
		// Membership registration fee — JSAPI payment (mini-program).
		// Missing branch previously returned an empty 200 (no switch arm
		// matched), leaving the client with a silent no-op on "发起支付".
		// openid 优先取两阶段 session 中已解析的微信身份（#1678）：weapp
		// 创建 session 时已通过 wx code/exchange_token 解析并存库，前端
		// 无需再调用 /api/wechat/openid。
		openid := req.OpenID
		if sessionFlow && openid == "" {
			openid = session.OpenID
		}
		if openid == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "membership payment requires open_id"})
			return
		}
		amount := req.Amount
		if sessionFlow {
			amount = sessionAmount // server-priced (session + coupon)
		}
		result, err := client.CreateJSAPIOrder(ctx, wechatpay.JSAPIParams{
			OutTradeNo:  outTradeNo,
			OpenID:      openid,
			TotalAmount: cfg.AmountToCents(amount),
			Description: "会员入会费",
			NotifyURL:   cfg.NotifyURL,
		})
		if err != nil {
			record.Status = "failed"
			fr := err.Error()
			record.FailReason = &fr
			if err := db.Create(&record).Error; err != nil {
				log.Printf("[PrepayOrder] failed to save payment record: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to save payment record"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create payment: " + err.Error()})
			return
		}
		record.Method = strPtr("jsapi")
		record.PrepayID = &result.PrepayID
		if err := db.Create(&record).Error; err != nil {
			log.Printf("[PrepayOrder] failed to save payment record: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to save payment record"})
			return
		}
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
			"out_trade_no":   req.OutTradeNo,
			"trade_state":    wxResult.TradeState,
			"transaction_id": wxResult.TransactionID,
			"paid":           wxResult.TradeState == "SUCCESS",
		},
	})
}
