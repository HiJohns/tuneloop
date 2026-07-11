package handlers

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// bindTokenStore holds binding tokens in memory with 5min TTL.
type bindTokenEntry struct {
	UserID    string
	WxOpenid  string
	Status    string // pending / bound / expired
	CreatedAt time.Time
}

var (
	bindTokensMu sync.RWMutex
	bindTokens   = map[string]*bindTokenEntry{}
)

func init() {
	// Periodic cleanup every 5 minutes
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			bindTokensMu.Lock()
			for k, v := range bindTokens {
				if time.Since(v.CreatedAt) > 5*time.Minute {
					v.Status = "expired"
					delete(bindTokens, k)
				}
			}
			bindTokensMu.Unlock()
		}
	}()
}

type WechatBindHandler struct{}

func NewWechatBindHandler() *WechatBindHandler {
	return &WechatBindHandler{}
}

// GenBindToken generates a binding token for the current user (PC side).
func (h *WechatBindHandler) GenBindToken(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)

	// Lookup user
	db := database.GetDB().WithContext(ctx)
	var user models.User
	if err := db.Where("iam_sub = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40004, "message": "user not found"})
		return
	}

	// Generate bind token
	token := uuid.New().String()
	bindTokensMu.Lock()
	bindTokens[token] = &bindTokenEntry{
		UserID:    user.ID,
		Status:    "pending",
		CreatedAt: time.Now(),
	}
	bindTokensMu.Unlock()

	qrURL := "/pages-weapp/bind/index?token=" + token
	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"token":      token,
			"qrcode_url": qrURL,
		},
	})
}

// PollBindToken returns the binding status (PC side polling).
func (h *WechatBindHandler) PollBindToken(c *gin.Context) {
	token := c.Param("token")

	bindTokensMu.RLock()
	entry, exists := bindTokens[token]
	bindTokensMu.RUnlock()

	if !exists || time.Since(entry.CreatedAt) > 5*time.Minute {
		c.JSON(http.StatusOK, gin.H{
			"code": 20000,
			"data": gin.H{"status": "expired"},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"status": entry.Status},
	})
}

// ConfirmBind completes the binding (WeChat MP side).
func (h *WechatBindHandler) ConfirmBind(c *gin.Context) {
	var req struct {
		Token   string `json:"token" binding:"required"`
		WxOpenid string `json:"wx_openid" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "token and wx_openid required"})
		return
	}

	bindTokensMu.Lock()
	entry, exists := bindTokens[req.Token]
	if !exists || entry.Status != "pending" {
		bindTokensMu.Unlock()
		c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "message": "invalid or expired token"})
		return
	}
	entry.Status = "bound"
	entry.WxOpenid = req.WxOpenid
	userID := entry.UserID
	bindTokensMu.Unlock()

	// Update IAM user's wx_openid
	iamClient := services.NewIAMClient()
	updErr := iamClient.UpdateUser(userID, &services.UpdateUserRequest{
		WxOpenid: &req.WxOpenid,
	})
	if updErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to bind: " + updErr.Error()})
		return
	}

	// Update local user
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	db.Model(&models.User{}).Where("id = ?", userID).Update("wx_openid", req.WxOpenid)

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "binding success"})
}

// Unbind clears the wx_openid for the current user (PC side).
func (h *WechatBindHandler) Unbind(c *gin.Context) {
	ctx := c.Request.Context()
	userID := middleware.GetUserID(ctx)

	db := database.GetDB().WithContext(ctx)
	var user models.User
	if err := db.Where("iam_sub = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40004, "message": "user not found"})
		return
	}

	empty := ""
	iamClient := services.NewIAMClient()
	if err := iamClient.UpdateUser(user.ID, &services.UpdateUserRequest{WxOpenid: &empty}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "unbind failed: " + err.Error()})
		return
	}

	db.Model(&user).Update("wx_openid", "")
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "unbind success"})
}
