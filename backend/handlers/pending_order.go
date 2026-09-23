package handlers

import (
	"encoding/json"
	"net/http"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

// pendingOrderMax 每用户最多缓存条数（超出替换最旧）（#2041）
const pendingOrderMax = 5

func currentLocalUser(c *gin.Context) (models.User, bool) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	iamSub := middleware.GetUserID(ctx)
	if iamSub == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "请先登录"})
		return models.User{}, false
	}
	var u models.User
	if err := db.Where("iam_sub = ?", iamSub).First(&u).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "用户不存在"})
		return models.User{}, false
	}
	return u, true
}

// CreatePendingOrder POST /api/user/pending-orders —— 缓存下单表单（#2041）
func CreatePendingOrder(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	u, ok := currentLocalUser(c)
	if !ok {
		return
	}

	var req struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Payload) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "缺少 payload"})
		return
	}

	// 上限保护：超出则替换最旧
	var count int64
	db.Model(&models.PendingOrder{}).Where("user_id = ? AND status = ?", u.ID, "pending").Count(&count)
	if count >= pendingOrderMax {
		var oldest models.PendingOrder
		if err := db.Where("user_id = ? AND status = ?", u.ID, "pending").
			Order("created_at ASC").First(&oldest).Error; err == nil {
			db.Delete(&oldest)
		}
	}

	po := models.PendingOrder{
		TenantID: u.TenantID,
		UserID:   u.ID,
		Payload:  string(req.Payload),
		Status:   "pending",
	}
	if err := db.Create(&po).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "缓存订单失败"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": 20100, "data": gin.H{"id": po.ID}})
}

// ListPendingOrders GET /api/user/pending-orders —— 本人待提交订单（#2041）
func ListPendingOrders(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	u, ok := currentLocalUser(c)
	if !ok {
		return
	}
	var rows []models.PendingOrder
	if err := db.Where("user_id = ? AND status = ?", u.ID, "pending").
		Order("created_at DESC").Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "查询失败"})
		return
	}
	list := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		list = append(list, gin.H{
			"id":         r.ID,
			"payload":    json.RawMessage(r.Payload),
			"created_at": r.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": list}})
}

// DeletePendingOrder DELETE /api/user/pending-orders/:id —— 放弃缓存（#2041）
func DeletePendingOrder(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	u, ok := currentLocalUser(c)
	if !ok {
		return
	}
	id := c.Param("id")
	res := db.Model(&models.PendingOrder{}).
		Where("id = ? AND user_id = ? AND status = ?", id, u.ID, "pending").
		Update("status", "abandoned")
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "操作失败"})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "待提交订单不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000})
}
