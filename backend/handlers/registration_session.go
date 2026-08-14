package handlers

import (
	"encoding/json"
	"log"
	"net/http"

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
}

func NewRegistrationSessionHandler(db *gorm.DB) *RegistrationSessionHandler {
	return &RegistrationSessionHandler{
		db:         db,
		iamService: services.NewIAMService(),
	}
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
		IDPhotos      []string               `json:"id_photos"`
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
		Amount:        getMembershipFee(h.db),
		Status:        "pending",
	}

	// Resolve openid when a WeChat code is provided (used by
	// GetMyRegistrationSession to resume a pending session).
	if req.WxCode != "" || req.ExchangeToken != "" {
		if openid, err := h.resolveOpenid(req.WxCode); err == nil {
			session.OpenID = openid
		} else {
			log.Printf("[RegistrationSession] openid resolution skipped: %v", err)
		}
	}

	if err := h.db.Create(&session).Error; err != nil {
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

// resolveOpenid exchanges a WeChat login code for the openid via IAM
// wx-accounts (does not consume the code's exchange_token — only the
// subsequent wx-bind/wx-login exchange does).
func (h *RegistrationSessionHandler) resolveOpenid(code string) (string, error) {
	if code == "" {
		return "", nil
	}
	res, err := h.iamService.WxAccounts(code)
	if err != nil {
		return "", err
	}
	return res.OpenID, nil
}

func marshalForm(form registerForm) string {
	b, _ := json.Marshal(form)
	return string(b)
}