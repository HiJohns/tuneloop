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

type UserPointsHandler struct{}

func NewUserPointsHandler() *UserPointsHandler {
	return &UserPointsHandler{}
}

func (h *UserPointsHandler) GetBalance(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)
	db := database.GetDB().WithContext(ctx)

	var localUser models.User
	if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"prepaid_points": localUser.PrepaidPoints,
			"promo_points":   localUser.PromoPoints,
		},
	})
}

func (h *UserPointsHandler) ListTransactions(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)
	db := database.GetDB().WithContext(ctx)

	var localUser models.User
	if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}

	txType := c.Query("type")
	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parseInt(c.DefaultQuery("page_size", "20"), 20)

	query := db.Where("user_id = ?", localUser.ID)
	if txType != "" {
		query = query.Where("type = ?", txType)
	}

	var total int64
	query.Model(&models.PointsTransaction{}).Count(&total)

	var transactions []models.PointsTransaction
	if err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&transactions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query transactions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":  transactions,
			"total": total,
			"page":  page,
			"page_size": pageSize,
		},
	})
}

func (h *UserPointsHandler) PurchasePoints(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)
	tenantID := middleware.GetTenantID(ctx)
	db := database.GetDB().WithContext(ctx)

	var req struct {
		Amount float64 `json:"amount"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "invalid request"})
		return
	}

	if req.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "amount must be positive"})
		return
	}

	cfg := wechatpay.GetConfig()
	outTradeNo := fmt.Sprintf("pts_%s_%d", userID[:8], time.Now().Unix())

	record := models.OrderPaymentRecord{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		UserID:     userID,
		OrderID:    &userID,
		OrderType:  "points",
		OutTradeNo: &outTradeNo,
		Amount:     req.Amount,
		Type:       "payment",
		Status:     "pending",
		Method:     strPtr("jsapi"),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if cfg.MockMode {
		record.Status = "paid"
		if err := db.Create(&record).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create payment record"})
			return
		}

		var localUser models.User
		if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
			return
		}
		newPrepaid := localUser.PrepaidPoints + req.Amount
		if err := db.Model(&localUser).Updates(map[string]interface{}{
			"prepaid_points": newPrepaid,
			"updated_at":     time.Now(),
		}).Error; err != nil {
			log.Printf("[PurchasePoints] failed to add points: %v", err)
		}

		c.JSON(http.StatusOK, gin.H{
			"code": 20000,
			"data": gin.H{
				"mock":        true,
				"prepaid_points": newPrepaid,
			},
		})
		return
	}

	if err := db.Create(&record).Error; err != nil {
		log.Printf("[PurchasePoints] failed to create payment record: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create payment record"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"payment_required": true,
			"amount":           req.Amount,
			"out_trade_no":     outTradeNo,
		},
	})
}


