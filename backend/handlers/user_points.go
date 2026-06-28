package handlers

import (
	"net/http"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

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

	var localUser models.User
	if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found"})
		return
	}

	oldPrepaid := localUser.PrepaidPoints
	oldPromo := localUser.PromoPoints
	newPrepaid := oldPrepaid + req.Amount
	now := time.Now()

	tx := db.Begin()

	if err := tx.Model(&localUser).Updates(map[string]interface{}{
		"prepaid_points":  newPrepaid,
		"total_spending":  localUser.TotalSpending + req.Amount,
		"updated_at": now,
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update points"})
		return
	}

	transaction := models.PointsTransaction{
		ID:                  uuid.New().String(),
		UserID:              localUser.ID,
		TenantID:            localUser.TenantID,
		Type:                "prepaid_purchase",
		Amount:              req.Amount,
		BalanceAfterPrepaid: newPrepaid,
		BalanceAfterPromo:   oldPromo,
		Description:         "预购点数",
		CreatedAt:           now,
	}

	if err := tx.Create(&transaction).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to record transaction"})
		return
	}

	tx.Commit()

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"prepaid_points": newPrepaid,
			"promo_points":   oldPromo,
			"transaction_id": transaction.ID,
		},
	})
}


