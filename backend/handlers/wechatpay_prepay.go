package handlers

import (
	"encoding/json"
	"fmt"
	"log"
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
	CouponCode string  `json:"coupon_code,omitempty"` // 通用优惠码（OREZ waive 全免 / ENO percent 1%，#1719）
}

type PrepayResponse struct {
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

	validTypes := map[string]bool{"rent": true, "repair": true, "damage": true, "renewal": true, "membership": true, "payment_shortfall": true}
	if !validTypes[req.OrderType] {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid order_type, must be rent/repair/damage/renewal/membership/payment_shortfall"})
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
		Amount:     models.FromYuan(req.Amount),
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
		raw := fmt.Sprintf(`{"gift_used":%d}`, int64(models.FromYuan(req.GiftUsed)))
		record.RawResponse = &raw
	}

	// Two-phase registration session flow (#1663): the membership fee is
	// priced server-side from the session + optional coupon — the client's
	// amount is ignored (计费以后端为准).
	sessionFlow := req.OrderType == "membership" && req.SessionID != ""

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
		// Dedicated column: the session link must survive the payment
		// callback, which overwrites RawResponse with the callback result
		// (#1664 audit: real-callback session_id was lost in RawResponse).
		record.SessionID = &req.SessionID
	}

	// 服务端定价（#1719 优惠码通用化）：金额基础值 = session 价（membership
	// 两阶段）或客户端金额（其余 order type）。优惠码对所有支付类型通用：
	// waive（OREZ）→ 0；percent（ENO）→ 按 value 比例。客户端金额不可信。
	// #1744 业务修正：优惠码适用【整单所有费用】（租金+押金+物流+逾期+定损），
	// 整单统一折扣——OREZ 全免（实付 0）、ENO 整单 × 比例。
	baseAmount := models.FromYuan(req.Amount)
	if sessionFlow {
		baseAmount = session.Amount
	}
	couponApplied := ""
	if req.CouponCode != "" {
		var coupon models.Coupon
		if err := db.Where("code = ? AND active = ?", strings.ToUpper(req.CouponCode), true).First(&coupon).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid coupon code"})
			return
		}
		switch coupon.Type {
		case "waive":
			baseAmount = 0
		case "percent":
			// Integer math (audit #1726 R7): coupon.Value is permille (‰),
			// amount in cents — 分 × ‰ / 1000, no float round-trip.
			baseAmount = baseAmount * models.Cents(coupon.Value) / 1000
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "unsupported coupon type"})
			return
		}
		couponApplied = coupon.Code
	}
	record.Amount = baseAmount // server-priced; never trust the client amount

	// #1744: 优惠码使用订单快照 — prepay 服务端重算后回写订单，
	// 优惠事实（码 + 折扣金额分）落库可审计/对账重现。
	// 仅非 session 流程（有 order_id）；幂等（重复 prepay 覆盖写同值）；
	// 失败仅 log 不阻断支付主流程（事后对账可发现）。
	// 折扣 = 整单原价 − 整单折后实付（含押金）。
	if couponApplied != "" && effectiveOrderID != "" {
		originalAmount := models.FromYuan(req.Amount) // 优惠前（服务端语义，元→分）
		discount := originalAmount - baseAmount       // OREZ 全免 → 原价；ENO → 原价−折后
		if discount < 0 {
			discount = 0
		}
		if err := db.Model(&models.Order{}).Where("id = ?", effectiveOrderID).
			Updates(map[string]interface{}{"coupon_code": couponApplied, "coupon_discount": int64(discount)}).Error; err != nil {
			log.Printf("[PrepayOrder] failed to write coupon snapshot for order %s: %v", effectiveOrderID, err)
		}
	}

	if sessionFlow {
		sessionRaw := map[string]interface{}{"session_id": req.SessionID, "original_amount": int64(session.Amount)}
		if couponApplied != "" {
			sessionRaw["coupon_code"] = couponApplied
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

	// 零金额校验（#1682 调整）：无优惠码时金额必须为正；waive 优惠码后为 0
	// 合法（走下方 waive 记账分支）。percent 优惠码后为 0 的极端场景同样允许。
	if baseAmount < 0 || (baseAmount == 0 && couponApplied == "") {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "amount must be positive"})
		return
	}

	// Full waiver (amount = 0): no WeChat payment is created; the record is
	// booked as paid and the callback-equivalent side effects run
	// (server-side account creation) — #1719 通用化：任意 order type 优惠后
	// 为 0 均走此路径（原仅 sessionFlow）。
	if baseAmount == 0 {
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
	case "rent", "repair", "damage", "payment_shortfall":
		// #1684: backfill openid from the local users cache (wx_openid bound
		// at registration) — the weapp Payment.jsx no longer sends open_id
		// (#1678), so mini-program rent payments must resolve it server-side.
		// PC keeps the Native (QR) fallback when no openid exists.
		if req.OpenID == "" {
			if userID != "" {
				var localUser models.User
				// middleware.GetUserID returns the IAM sub; the local cache
				// links it via iam_sub, not id (prod incident 2026-08-18).
				if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err == nil && localUser.WxOpenid != "" {
					req.OpenID = localUser.WxOpenid
				}
			}
		}
		if req.OpenID == "" {
			// Native payment (QR code) for PC
			result, err := client.CreateNativeOrder(ctx, wechatpay.NativeParams{
				OutTradeNo:  outTradeNo,
				TotalAmount: int64(record.Amount),
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
			TotalAmount: int64(record.Amount),
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
		// #1719: backfill openid from the local users cache — same as the
		// rent/repair/damage branch (the weapp Payment.jsx no longer sends
		// open_id per #1678), otherwise renewal payment fails with 400.
		if req.OpenID == "" {
			if userID != "" {
				var localUser models.User
				if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err == nil && localUser.WxOpenid != "" {
					req.OpenID = localUser.WxOpenid
				}
			}
		}
		if req.OpenID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "renewal payment requires open_id"})
			return
		}
		result, err := client.CreateJSAPIOrder(ctx, wechatpay.JSAPIParams{
			OutTradeNo:  outTradeNo,
			OpenID:      req.OpenID,
			TotalAmount: int64(record.Amount),
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
		// 金额已由通用优惠码逻辑服务端重算（record.Amount，#1719）：sessionFlow
		// 用 session 价 + 优惠码，非 session 用客户端金额 + 优惠码。
		amount := record.Amount
		result, err := client.CreateJSAPIOrder(ctx, wechatpay.JSAPIParams{
			OutTradeNo:  outTradeNo,
			OpenID:      openid,
			TotalAmount: int64(amount),
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
