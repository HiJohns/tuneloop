package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strings"
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
	UserID   string
	WxOpenid string
	Status   string // pending / bound / needs_merge / merged / expired
	// #2445: merge ticket minted by IAM wx-bind (single-use, 5 min) and the
	// idempotent result of a successful X→Y merge.
	MergeTicket       string
	MergedToUserID    string
	NeedsCredentials  bool
	SuggestedUsername string
	CreatedAt         time.Time
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
// #2410：绑定只能走 IAM /api/v1/auth/wx-bind（#2228 一微信一账户）。
// exchange_token 是唯一权威来源；客户端传来的 wx_openid 忽略且不转发。
func (h *WechatBindHandler) ConfirmBind(c *gin.Context) {
	var req struct {
		Token         string `json:"token" binding:"required"`
		ExchangeToken string `json:"exchange_token" binding:"required"`
		WxOpenid      string `json:"wx_openid"` // 兼容字段，不作为绑定权威
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[ConfirmBind] body parse error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "token and exchange_token required"})
		return
	}

	log.Printf("[ConfirmBind] req.Token=%q exchange_token=%s", req.Token, func() string {
		if req.ExchangeToken == "" {
			return "empty"
		}
		return "present"
	}())

	bindTokensMu.Lock()
	entry, exists := bindTokens[req.Token]
	if !exists || entry.Status != "pending" {
		bindTokensMu.Unlock()
		log.Printf("[ConfirmBind] token not found or used: exists=%v status=%q", exists, func() string {
			if exists {
				return entry.Status
			}
			return "n/a"
		}())
		c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "message": "invalid or expired token"})
		return
	}
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

	// #2410 D1/D2：只提交 exchange_token，不发送 wx_openid（防止绕过 needs_merge）。
	_, bindErr := services.NewIAMService().WxBind(req.ExchangeToken, "", user.IAMSub)

	var mergeErr *services.WxNeedsMergeError
	if errors.As(bindErr, &mergeErr) {
		// #2410 D3：needs_merge 不落绑定；token 置 needs_merge 供 PC 轮询提示。
		bindTokensMu.Lock()
		if e, ok := bindTokens[req.Token]; ok {
			e.Status = "needs_merge"
			e.MergeTicket = mergeErr.MergeTicket
		}
		bindTokensMu.Unlock()

		c.JSON(http.StatusConflict, gin.H{
			"code":    40900,
			"message": "该微信已绑定其他账户，需要合并",
			"data": gin.H{
				"needs_merge":      true,
				"existing_user_id": mergeErr.ExistingUserID,
				"target_user_id":   mergeErr.TargetUserID,
				"openid":           mergeErr.OpenID,
				"merge_ticket":     mergeErr.MergeTicket,
			},
		})
		return
	}
	if bindErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to bind: " + bindErr.Error()})
		return
	}

	// #2016 S3: 本地 users.wx_openid 缓存列已废弃（IAM 绑定表为唯一来源）

	// Mark token as bound (triggers PC polling success)
	bindTokensMu.Lock()
	entry.Status = "bound"
	bindTokensMu.Unlock()

	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "binding success"})
}

// MergeBind completes a bind-QR needs_merge conflict (#2445).
//
// The QR token identifies the admin-provisioned account X; the IAM merge
// ticket proves the caller controls X's bind token and is bound to
// (openid, X, Y). Merge direction is always X→Y: X becomes obsolete, the
// employee's personal account Y keeps its password and receives X's org
// relations (switch identity appears after re-login).
func (h *WechatBindHandler) MergeBind(c *gin.Context) {
	var req struct {
		Token       string `json:"token" binding:"required"`
		MergeTicket string `json:"merge_ticket" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "token and merge_ticket required"})
		return
	}

	bindTokensMu.Lock()
	entry, exists := bindTokens[req.Token]
	if !exists {
		bindTokensMu.Unlock()
		c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "message": "invalid or expired token"})
		return
	}
	if entry.Status == "merged" {
		data := gin.H{
			"merged":             true,
			"to_user_id":         entry.MergedToUserID,
			"needs_credentials":  entry.NeedsCredentials,
			"suggested_username": entry.SuggestedUsername,
		}
		bindTokensMu.Unlock()
		c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "already merged", "data": data})
		return
	}
	if entry.Status != "needs_merge" {
		status := entry.Status
		bindTokensMu.Unlock()
		c.JSON(http.StatusBadRequest, gin.H{"code": 40902, "message": "token is not waiting for merge (status=" + status + ")"})
		return
	}
	if entry.MergeTicket == "" || entry.MergeTicket != req.MergeTicket {
		bindTokensMu.Unlock()
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid or expired merge ticket"})
		return
	}
	userID := entry.UserID
	bindTokensMu.Unlock()

	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	var user models.User
	if err := db.Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40004, "message": "user not found"})
		return
	}

	result, mergeErr := services.NewIAMService().WxMergeByTicket(req.MergeTicket)
	if mergeErr != nil {
		log.Printf("[MergeBind] IAM wx-merge-confirm failed for token=%s: %v", req.Token, mergeErr)
		c.JSON(http.StatusConflict, gin.H{"code": 40900, "message": "账户合并失败: " + mergeErr.Error()})
		return
	}
	if result.FromUserID != user.IAMSub {
		// Ticket/token mismatch: never let a ticket for another account mutate
		// this bind token's local data.
		log.Printf("[MergeBind] ticket from_user_id %s != token user iam_sub %s", result.FromUserID, user.IAMSub)
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "merge ticket does not match binding token"})
		return
	}

	// Local staff tables are keyed by local user id (site_members etc.); sync
	// them synchronously, with the IAM user.account_merged callback as retry
	// safety net. A local failure must be surfaced (never silently swallowed).
	localSyncErr := migrateMergedAccountLocal(db, result.FromUserID, result.ToUserID)
	if localSyncErr != nil {
		log.Printf("[MergeBind] local account sync failed (from=%s to=%s): %v", result.FromUserID, result.ToUserID, localSyncErr)
	}

	needsCred, suggested := needsCredentialsForIAMUser(result.ToUserID)

	bindTokensMu.Lock()
	if e, ok := bindTokens[req.Token]; ok {
		e.Status = "merged"
		e.MergedToUserID = result.ToUserID
		e.NeedsCredentials = needsCred
		e.SuggestedUsername = suggested
	}
	bindTokensMu.Unlock()

	data := gin.H{
		"merged":             true,
		"from_user_id":       result.FromUserID,
		"to_user_id":         result.ToUserID,
		"needs_credentials":  needsCred,
		"suggested_username": suggested,
	}
	if localSyncErr != nil {
		data["local_sync"] = false
		data["warning"] = "员工身份本地数据同步失败，系统将自动重试；如员工身份未生效请联系管理员"
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "merged", "data": data})
}

// needsCredentialsForIAMUser reports whether the account has never had a
// user-chosen username/password (WeChat self-registration sets username=phone
// and a random password). Best-effort: failures are logged and treated as
// "no prompt" so a merge success is never blocked by this enrichment.
func needsCredentialsForIAMUser(iamUserID string) (bool, string) {
	u, err := services.NewIAMClient().GetUser(iamUserID)
	if err != nil {
		log.Printf("[MergeBind] GetUser(%s) for credential hint failed: %v", iamUserID, err)
		return false, ""
	}
	phone := strings.TrimSpace(u.Phone)
	username := strings.TrimSpace(u.Username)
	suggested := phone
	if suggested == "" {
		suggested = username
	}
	return username == "" || (phone != "" && username == phone), suggested
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
	// #2016 S3: 本地 users.wx_openid 缓存列已废弃（IAM 绑定表为唯一来源）
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
