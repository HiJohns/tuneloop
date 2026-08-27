package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RegistrationSessionHandler serves the two-phase registration endpoints
// (#1663/#1664). All endpoints are unauthenticated: the user has no account
// yet, identity is carried by the WeChat code / exchange_token.
type RegistrationSessionHandler struct {
	db         *gorm.DB
	iamService *services.IAMService
	// iamClient is injectable for tests (#1688); nil → NewIAMClient().
	iamClient *services.IAMClient
}

func NewRegistrationSessionHandler(db *gorm.DB) *RegistrationSessionHandler {
	return &RegistrationSessionHandler{
		db:         db,
		iamService: services.NewIAMService(),
	}
}

// setIAMClient overrides the IAM client used for init-user reservation
// (tests only, #1688).
func (h *RegistrationSessionHandler) setIAMClient(c *services.IAMClient) {
	h.iamClient = c
}

// iamClientFor returns the injectable client or a fresh default one.
func (h *RegistrationSessionHandler) iamClientFor() *services.IAMClient {
	if h.iamClient != nil {
		return h.iamClient
	}
	return services.NewIAMClient()
}

// CreateRegistrationSession handles POST /api/auth/registration-sessions.
// Persists the form (pending) and returns session_id + amount (base
// membership fee; coupon discounts are applied at prepay time).
func (h *RegistrationSessionHandler) CreateRegistrationSession(c *gin.Context) {
	var req struct {
		Nickname      string                 `json:"nickname" binding:"required"`
		Name          string                 `json:"name" binding:"required"`
		Phone         string                 `json:"phone" binding:"required"`
		Email         string                 `json:"email"`
		ExchangeToken string                 `json:"exchange_token"`
		WxCode        string                 `json:"wx_code"`
		Ref           string                 `json:"ref"`
		Address       map[string]interface{} `json:"address"`
		IDPhotos      map[string]string      `json:"id_photos"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request: " + err.Error()})
		return
	}

	form := registerForm{
		Nickname: req.Nickname,
		Name:     req.Name,
		Phone:    req.Phone,
		Email:    req.Email,
		Ref:      req.Ref,
		Address:  req.Address,
		IDPhotos: req.IDPhotos,
	}

	session := models.RegistrationSession{
		ID:            uuid.New().String(),
		ExchangeToken: req.ExchangeToken,
		FormData:      marshalForm(form),
		Amount:        models.FromYuan(getMembershipFee(h.db)),
		Status:        "pending",
	}

	// Reserve the account atomically (#1682): IAM CreateUser with status=init
	// claims email/phone/username (handler uniqueness check + init partial
	// unique indexes) — a conflicting registration is rejected here, before
	// any payment happens, instead of failing at the payment callback.
	// init users cannot log in (IAM requires status=active).
	iamClient := h.iamClientFor()
	reserveReq := &services.CreateUserRequest{
		Username: form.Phone,
		Name:     form.Name,
		Phone:    form.Phone,
		Email:    form.Email,
		Password: "wx_" + uuid.NewString()[:20],
		Status:   "init",
	}
	if form.Nickname != "" {
		n := form.Nickname
		reserveReq.Nickname = &n
	}
	reserveResp, reserveErr := iamClient.CreateUser(reserveReq)
	if reserveErr != nil {
		if strings.Contains(reserveErr.Error(), "already exists") || strings.Contains(reserveErr.Error(), "conflict") {
			c.JSON(http.StatusConflict, gin.H{
				"code":    40900,
				"message": "该手机号或邮箱已注册，请直接登录",
			})
			return
		}
		log.Printf("[RegistrationSession] init user reservation failed: %v", reserveErr)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "注册失败，请稍后重试"})
		return
	}
	session.IAMUserID = &reserveResp.UserID

	// Reserve the local users cache alongside the IAM user (#1688): the
	// payment record's user_id must reference the real local user so the
	// customer can query their membership-fee consumption later. The cache is
	// created with status=init now and activated (status→active + gift
	// points/referral) at the payment callback. Best-effort with rollback:
	// if the local insert fails, purge the IAM reservation.
	localUser := models.User{
		IAMSub:             reserveResp.UserID,
		TenantID:           "00000000-0000-0000-0000-000000000000",
		OrgID:              "00000000-0000-0000-0000-000000000000",
		Username:           form.Phone,
		Name:               form.Name,
		Nickname:           form.Nickname,
		Phone:              form.Phone,
		Email:              form.Email,
		Role:               "USER",
		Status:             "init",
		IsProfileCompleted: true,
	}
	if err := h.db.Create(&localUser).Error; err != nil {
		if purgeErr := iamClient.PurgeUser(reserveResp.UserID); purgeErr != nil {
			log.Printf("[RegistrationSession] rollback purge failed for %s: %v", reserveResp.UserID, purgeErr)
		}
		log.Printf("[RegistrationSession] local user reservation failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "注册失败，请稍后重试"})
		return
	}
	session.LocalUserID = &localUser.ID

	// Resolve openid when a WeChat code is provided (used by
	// GetMyRegistrationSession to resume a pending session). wx-accounts mints
	// a fresh single-use exchange_token on every call (#1682): the token the
	// frontend captured earlier can expire before the payment callback runs,
	// so the freshest token replaces the session one.
	if req.WxCode != "" || req.ExchangeToken != "" {
		if res, err := h.resolveOpenidResult(req.WxCode); err == nil && res != nil {
			session.OpenID = res.OpenID
			if res.ExchangeToken != "" {
				session.ExchangeToken = res.ExchangeToken
			}
		} else {
			log.Printf("[RegistrationSession] openid resolution skipped: %v", err)
		}
	}

	if err := h.db.Create(&session).Error; err != nil {
		// Release the reserved user so the email/phone is not stuck (#1682).
		if purgeErr := iamClient.PurgeUser(*session.IAMUserID); purgeErr != nil {
			log.Printf("[RegistrationSession] failed to release reserved user %s: %v", *session.IAMUserID, purgeErr)
		}
		log.Printf("[RegistrationSession] create failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create registration session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"session_id": session.ID,
			"amount":     session.Amount,
		},
	})
}

// GetMyRegistrationSession handles GET /api/auth/registration-sessions/me.
// Resumes the pending session of the caller: ?code= (weapp wx.login code →
// openid lookup) or ?session_id= (H5/local cache). 404 when no session.
func (h *RegistrationSessionHandler) GetMyRegistrationSession(c *gin.Context) {
	code := c.Query("code")
	sessionID := c.Query("session_id")

	db := h.db.WithContext(c.Request.Context())
	var session models.RegistrationSession
	if sessionID != "" {
		if err := db.Where("id = ?", sessionID).First(&session).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "registration session not found"})
			return
		}
	} else if code != "" {
		openid, err := h.resolveOpenid(code)
		if err != nil {
			log.Printf("[RegistrationSession] openid resolution failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to resolve wechat identity"})
			return
		}
		if err := db.Where("openid = ? AND status = ?", openid, "pending").
			Order("created_at desc").First(&session).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "no pending registration session"})
			return
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "code or session_id is required"})
		return
	}

	var form map[string]interface{}
	if err := json.Unmarshal([]byte(session.FormData), &form); err != nil {
		form = map[string]interface{}{}
	}

	// Legacy pre-#1682 sessions carry no reserved init user; continuing them
	// would collide with the new reservation mechanism at the payment
	// callback. Treat them as expired — the frontend clears the resume state
	// and the user resubmits the form (creating a fresh reserved session).
	if session.IAMUserID == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "registration session expired, please resubmit"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"session_id": session.ID,
			"status":     session.Status,
			"form_data":  form,
			"amount":     session.Amount,
		},
	})
}

// GetRegistrationSessionStatus handles GET
// /api/auth/registration-sessions/:id/status. Polled by the frontend after
// payment to detect pending → completed (account creation done).
func (h *RegistrationSessionHandler) GetRegistrationSessionStatus(c *gin.Context) {
	db := h.db.WithContext(c.Request.Context())
	var session models.RegistrationSession
	if err := db.Where("id = ?", c.Param("id")).First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "registration session not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"status": session.Status,
		},
	})
}

// resolveOpenidResult exchanges a WeChat login code for the openid via IAM
// wx-accounts (does not consume the code's exchange_token — only the
// subsequent wx-bind/wx-login exchange does). Returns the full wx-accounts
// result so callers can also refresh the session exchange_token (#1682).
func (h *RegistrationSessionHandler) resolveOpenidResult(code string) (*services.WxAccountsResult, error) {
	if code == "" {
		return nil, nil
	}
	res, err := h.iamService.WxAccounts(code)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// resolveOpenid keeps the narrow openid-only contract for callers that only
// need the WeChat identity (GetMyRegistrationSession resume lookup).
func (h *RegistrationSessionHandler) resolveOpenid(code string) (string, error) {
	res, err := h.resolveOpenidResult(code)
	if err != nil || res == nil {
		return "", err
	}
	return res.OpenID, nil
}

func marshalForm(form registerForm) string {
	b, _ := json.Marshal(form)
	return string(b)
}

// UploadSessionIDPhoto handles POST /api/auth/registration-sessions/:id/id-photo.
// This is a public (unauthenticated) endpoint — the user has no account yet.
// It stores the uploaded file and updates the session's form_data.id_photos map.
func (h *RegistrationSessionHandler) UploadSessionIDPhoto(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "message": "session id required"})
		return
	}

	var session models.RegistrationSession
	if err := h.db.Where("id = ? AND status = ?", sessionID, "pending").First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "registration session not found or not pending"})
		return
	}

	side := c.PostForm("side")
	if side != "front" && side != "back" && side != "other" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "message": "invalid side, must be front, back, or other"})
		return
	}

	c.Request.ParseMultipartForm(10 << 20)
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "no file uploaded"})
		return
	}

	mimeType := file.Header.Get("Content-Type")
	if mimeType != "image/jpeg" && mimeType != "image/png" && mimeType != "image/webp" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "only JPEG, PNG, WebP allowed"})
		return
	}

	if file.Size > 5*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "file too large, max 5MB"})
		return
	}

	ext := filepath.Ext(file.Filename)
	filename := fmt.Sprintf("id_photos/pending_%s_%s_%d%s", sessionID, side, time.Now().UnixNano(), ext)

	reader, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50002, "message": "failed to open file"})
		return
	}
	defer reader.Close()

	storage := services.NewMediaStorage()
	if err := storage.Upload(c.Request.Context(), filename, reader, mimeType); err != nil {
		log.Printf("session id photo upload failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50003, "message": "failed to save file"})
		return
	}

	// Update form_data.id_photos map: side → storage key.
	var form registerForm
	if session.FormData != "" {
		_ = json.Unmarshal([]byte(session.FormData), &form)
	}
	if form.IDPhotos == nil {
		form.IDPhotos = map[string]string{}
	}
	form.IDPhotos[side] = filename

	if err := h.db.Model(&session).Update("form_data", marshalForm(form)).Error; err != nil {
		log.Printf("session form_data update failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50004, "message": "failed to save photo reference"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "upload success",
		"data": gin.H{
			"side": side,
		},
	})
}
