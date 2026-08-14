package handlers

import (
	"log"
	"net/http"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
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

	// Lazy re-check membership level (fixes stale levels after aggregation bug)
	if err := services.CheckAndUpgradeLevel(localUser.ID, nil); err != nil {
		log.Printf("[GetBalance] membership level check failed: %v", err)
	}

	// Max gift points ratio for display (default 0.3, from PointsPolicy).
	maxPayRatio := 0.3
	policies, err := queryApplicablePointsPolicies(db, localUser.TenantID, "")
	if err == nil && len(policies) > 0 {
		maxPayRatio = policies[0].MaxPayRatio
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"prepaid_points": localUser.PrepaidPoints,
			"promo_points":   localUser.PromoPoints,
			"max_pay_ratio":  maxPayRatio,
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
			"list":      transactions,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}
