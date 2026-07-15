package handlers

import (
	"log"
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

	qrURL := "/api/wechat-bind/confirm-page?token=" + token
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

// ConfirmBindPage serves the confirmation page scanned from QR code.
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

	html := `<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>微信绑定</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,sans-serif;background:#f5f5f5;display:flex;align-items:center;justify-content:center;min-height:100vh;padding:24px}
.card{background:#fff;border-radius:16px;padding:40px 24px;text-align:center;max-width:340px;width:100%%;box-shadow:0 2px 8px rgba(0,0,0,0.06)}
h2{font-size:20px;color:#333;margin-bottom:8px}.sub{font-size:14px;color:#999;margin-bottom:28px;line-height:1.6}
.btn{display:inline-block;width:80%%;max-width:260px;padding:14px 32px;border:none;border-radius:999px;font-size:16px;font-weight:700;cursor:pointer;margin:0 auto}
.btn-confirm{background:#07c160;color:#fff}.btn-confirm:disabled{background:#ccc}
.msg{margin-top:16px;font-size:13px;color:#666}
</style>
</head><body>
<div class="card">
<h2>欢迎绑定微信</h2>
<p class="sub">确认后将关联您的微信号，之后可在小程序中一键登录</p>
<button class="btn btn-confirm" onclick="confirmBind('` + token + `')">确认绑定</button>
<p class="msg" id="msg"></p>
</div>
<script>
function confirmBind(token) {
  var btn = document.querySelector('.btn');
  var msg = document.getElementById('msg');
  btn.disabled = true;
  btn.textContent = '绑定中...';
  msg.textContent = '';
  fetch('/api/wechat-bind/confirm', {
    method: 'POST',
    headers: {'Content-Type':'application/json'},
    body: JSON.stringify({token:token, wx_openid:'wx_'+token.slice(0,8)})
  })
  .then(r=>r.json())
  .then(data=>{
    if(data.code===20000){btn.textContent='绑定成功';btn.style.background='#576b95';msg.textContent='请返回管理端查看'}
    else{btn.disabled=false;btn.textContent='确认绑定';msg.textContent='绑定失败: '+data.message}
  })
  .catch(function(){btn.disabled=false;btn.textContent='确认绑定';msg.textContent='网络错误'});
}
</script>
</body></html>`
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, html)
}
