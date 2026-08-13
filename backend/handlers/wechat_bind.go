package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
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

// GenBindToken generates a binding token and a mini-program QR code (PC side).
// The QR is a WeChat mini-program code: scanning it opens the Bind page in the
// mini-program where wx.login() can obtain the real openid (a browser page
// cannot). Token is 16 hex chars so scene "bind_<token>" fits the 32-char
// wxacode scene limit.
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

	randBytes := make([]byte, 8)
	if _, err := rand.Read(randBytes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to generate bind token"})
		return
	}
	token := hex.EncodeToString(randBytes)

	// Generate bind token
	bindTokensMu.Lock()
	bindTokens[token] = &bindTokenEntry{
		UserID:    user.ID,
		Status:    "pending",
		CreatedAt: time.Now(),
	}
	bindTokensMu.Unlock()

	// Generate the mini-program code (wxacode.getunlimit). A failure here MUST
	// surface to the caller — a fake openid binding is worse than no binding.
	accessToken, tokenErr := services.GetWxAccessToken()
	if tokenErr != nil {
		log.Printf("[GenBindToken] wx access token failed: %v", tokenErr)
		bindTokensMu.Lock()
		delete(bindTokens, token)
		bindTokensMu.Unlock()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to generate binding QR code"})
		return
	}
	imgData, codeErr := services.GetWxacodeUnlimited(accessToken, "bind_"+token, "pages-weapp/bind/index", services.GetWxEnvVersion())
	if codeErr != nil {
		log.Printf("[GenBindToken] wxacode failed: %v", codeErr)
		bindTokensMu.Lock()
		delete(bindTokens, token)
		bindTokensMu.Unlock()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to generate binding QR code"})
		return
	}
	wxacodeBase64 := base64.StdEncoding.EncodeToString(imgData)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"token":          token,
			"wxacode_base64": wxacodeBase64,
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
		log.Printf("[ConfirmBind] body parse error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "token and wx_openid required"})
		return
	}

	log.Printf("[ConfirmBind] req.Token=%q req.WxOpenid=%q", req.Token, req.WxOpenid)

	bindTokensMu.Lock()
	entry, exists := bindTokens[req.Token]
	if !exists || entry.Status != "pending" {
		bindTokensMu.Unlock()
		log.Printf("[ConfirmBind] token not found or used: exists=%v status=%q", exists, func() string { if exists { return entry.Status } ; return "n/a" }())
		c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "message": "invalid or expired token"})
		return
	}
	entry.WxOpenid = req.WxOpenid
	userID := entry.UserID
	bindTokensMu.Unlock()

	// Lookup user's iam_sub for IAM API call
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	var user models.User
	if err := db.Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40004, "message": "user not found"})
		return
	}

	// Update IAM user's wx_openid (IAM expects iam_sub as user identifier)
	iamClient := services.NewIAMClient()
	updErr := iamClient.UpdateUser(user.IAMSub, &services.UpdateUserRequest{
		WxOpenid: &req.WxOpenid,
	})
	if updErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to bind: " + updErr.Error()})
		return
	}

	// Update local user
	db.Model(&models.User{}).Where("id = ?", userID).Update("wx_openid", req.WxOpenid)

	// Mark token as bound (triggers PC polling success)
	bindTokensMu.Lock()
	entry.Status = "bound"
	bindTokensMu.Unlock()

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
	if err := iamClient.UpdateUser(user.IAMSub, &services.UpdateUserRequest{WxOpenid: &empty}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "unbind failed: " + err.Error()})
		return
	}

	db.Model(&user).Update("wx_openid", "")
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "unbind success"})
}

// ConfirmBindPage serves a static guide page for old QR codes. Real binding
// happens inside the mini-program Bind page: a browser page cannot call
// wx.login(), so it can never obtain the real openid (see #1640 — a fake
// openid was submitted here before).
func (h *WechatBindHandler) ConfirmBindPage(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.String(http.StatusBadRequest, "invalid token")
		return
	}

	bindTokensMu.RLock()
	entry, exists := bindTokens[token]
	bindTokensMu.RUnlock()

	if !exists || entry.Status != "pending" {
		c.String(http.StatusGone, "二维码已过期或已使用")
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, `<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>微信绑定</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,sans-serif;background:#f5f5f5;display:flex;align-items:center;justify-content:center;min-height:100vh;padding:24px}
.card{background:#fff;border-radius:16px;padding:40px 24px;text-align:center;max-width:340px;width:100%;box-shadow:0 2px 8px rgba(0,0,0,0.06)}
h2{font-size:20px;color:#333;margin-bottom:12px}.sub{font-size:14px;color:#999;margin-bottom:16px;line-height:1.8}
</style>
</head><body>
<div class="card">
<h2>请在微信小程序中完成绑定</h2>
<p class="sub">请返回 PC 端重新点击「绑定微信」，使用微信「扫一扫」扫描新生成的小程序码，将在小程序内完成绑定。</p>
</div>
</body></html>`)
}
