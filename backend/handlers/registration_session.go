package handlers

import (
	"encoding/json"
	"errors"
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

type resumableRegistration struct {
	SessionID   string
	IAMUserID   string
	LocalUserID string
	Amount      models.Cents
	Status      string
}

// findResumableRegistration locates an init user + unfinished session for the
// same phone/email so a retry can continue instead of returning 409 (#2307).
func (h *RegistrationSessionHandler) findResumableRegistration(phone, email string) (*resumableRegistration, error) {
	if phone == "" && email == "" {
		return nil, nil
	}
	db := h.db
	var user models.User
	q := db.Where("status = ? AND iam_sub <> ''", "init")
	if phone != "" {
		q = q.Where("phone = ?", phone)
	} else {
		q = q.Where("email = ?", email)
	}
	err := q.Order("created_at desc").First(&user).Error
	iamID := ""
	localUserID := ""
	if err == nil {
		iamID = user.IAMSub
		localUserID = user.ID
	} else if err != gorm.ErrRecordNotFound {
		return nil, err
	}
	if iamID == "" {
		// Local cache missing (manual repair / different device) → IAM fallback.
		if users, lerr := h.iamClientFor().ListUsers(); lerr == nil {
			for _, u := range users {
				if (phone != "" && u.Phone == phone) || (email != "" && u.Email == email) {
					iamID = u.ID
					break
				}
			}
		}
	}
	if iamID == "" {
		return nil, nil
	}
	var session models.RegistrationSession
	if err := db.Where("iam_user_id = ? AND status IN ?", iamID,
		[]string{"pending", "failed", "needs_rebind"}).Order("created_at desc").First(&session).Error; err != nil {
		// init user exists but no session → reuse the user, create a new session.
		return &resumableRegistration{IAMUserID: iamID, LocalUserID: localUserID, Status: "none"}, nil
	}
	return &resumableRegistration{SessionID: session.ID, IAMUserID: iamID, LocalUserID: localUserID, Amount: session.Amount, Status: session.Status}, nil
}

// CreateRegistrationSession handles POST /api/auth/registration-sessions.
// Persists the form (pending) and returns session_id + amount (base
// membership fee; coupon discounts are applied at prepay time).
func (h *RegistrationSessionHandler) CreateRegistrationSession(c *gin.Context) {
	var req struct {
		Nickname         string                 `json:"nickname" binding:"required"`
		Name             string                 `json:"name" binding:"required"`
		Phone            string                 `json:"phone" binding:"required"`
		Email            string                 `json:"email"`
		ExchangeToken    string                 `json:"exchange_token"`
		WxCode           string                 `json:"wx_code"`
		Ref              string                 `json:"ref"`
		Address          map[string]interface{} `json:"address"`
		IDPhotos         map[string]string      `json:"id_photos"`
		IdPhotoOtherType string                 `json:"id_photo_other_type"` // #1807: 第三证件类型
		AgreedTerms      bool                   `json:"agreed_terms"`        // #1923: 协议同意（必填）
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request: " + err.Error()})
		return
	}

	// #1923/#2285: 注册/支付保费前必须勾选相关协议。
	if !req.AgreedTerms {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "请先阅读并同意相关协议"})
		return
	}
	log.Printf("[Consent] registration session: terms agreed (related_agreements)")

	form := registerForm{
		Nickname:         req.Nickname,
		Name:             req.Name,
		Phone:            req.Phone,
		Email:            req.Email,
		Ref:              req.Ref,
		Address:          req.Address,
		IDPhotos:         req.IDPhotos,
		IdPhotoOtherType: req.IdPhotoOtherType,
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
	var reserveResp *services.CreateUserResponse
	var reusedIAMUser bool
	var reuseLocalUserID string
	reserveResp, reserveErr := iamClient.CreateUser(reserveReq)
	if reserveErr != nil {
		isDuplicate := errors.Is(reserveErr, services.ErrIAMUserExists) ||
			strings.Contains(reserveErr.Error(), "already exists") || strings.Contains(reserveErr.Error(), "conflict")
		if isDuplicate {
			if resume, rerr := h.findResumableRegistration(form.Phone, form.Email); rerr == nil && resume != nil {
				if resume.SessionID != "" {
					c.JSON(http.StatusConflict, gin.H{
						"code":    40901,
						"message": "检测到未完成的注册，可继续完成支付",
						"data": gin.H{
							"resume_session_id": resume.SessionID,
							"amount":            resume.Amount,
							"session_status":    resume.Status,
						},
					})
					return
				}
				// init user without a session → reuse it and create a new session.
				if resume.IAMUserID != "" {
					reusedIAMUser = true
					reuseLocalUserID = resume.LocalUserID
					reserveResp = &services.CreateUserResponse{UserID: resume.IAMUserID, Status: "init"}
				}
			}
			if reserveResp == nil {
				c.JSON(http.StatusConflict, gin.H{
					"code":    40900,
					"message": "该手机号或邮箱已注册，请直接登录",
				})
				return
			}
		}
		if reserveResp == nil {
			log.Printf("[RegistrationSession] init user reservation failed: %v", reserveErr)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "注册失败，请稍后重试"})
			return
		}
	}
	session.IAMUserID = &reserveResp.UserID

	// Reserve the local users cache alongside the IAM user (#1688): the
	// payment record's user_id must reference the real local user so the
	// customer can query their membership-fee consumption later. The cache is
	// created with status=init now and activated (status→active + gift
	// points/referral) at the payment callback. Best-effort with rollback:
	// if the local insert fails, purge the IAM reservation.
	if reuseLocalUserID != "" {
		session.LocalUserID = &reuseLocalUserID
	} else {
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
			if !reusedIAMUser {
				if purgeErr := iamClient.PurgeUser(reserveResp.UserID); purgeErr != nil {
					log.Printf("[RegistrationSession] rollback purge failed for %s: %v", reserveResp.UserID, purgeErr)
				}
			}
			log.Printf("[RegistrationSession] local user reservation failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "注册失败，请稍后重试"})
			return
		}
		session.LocalUserID = &localUser.ID
	}

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
		// Do not purge a reused init user (it existed before this request).
		if !reusedIAMUser {
			if purgeErr := iamClient.PurgeUser(*session.IAMUserID); purgeErr != nil {
				log.Printf("[RegistrationSession] failed to release reserved user %s: %v", *session.IAMUserID, purgeErr)
			}
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
			"error":  session.Error,
			// #1807: completed 后返回新注册账户的 IAM user_id——前端
			// finishMembershipFlow 直接 wx-login-select 登录新会员（多账户
			// 场景下不跳账户选择页，注册完成即切换为新账户）。
			"user_id": session.IAMUserID,
		},
	})
}

// RebindRegistrationSession handles POST /api/auth/registration-sessions/:id/rebind.
// Paid but WxBind failed (expired exchange_token) → user re-authorizes WeChat and
// the session is finalized without re-creating the account (#2308).
func (h *RegistrationSessionHandler) RebindRegistrationSession(c *gin.Context) {
	var req struct {
		WxCode string `json:"wx_code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid request: " + err.Error()})
		return
	}
	if req.WxCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "wx_code is required"})
		return
	}

	db := h.db.WithContext(c.Request.Context())
	var session models.RegistrationSession
	if err := db.Where("id = ?", c.Param("id")).First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "registration session not found"})
		return
	}
	if session.Status == "completed" {
		c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"status": "completed", "user_id": session.IAMUserID}})
		return
	}
	if session.IAMUserID == nil || *session.IAMUserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "session has no reserved user"})
		return
	}

	if session.OpenID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "session openid missing; please restart registration"})
		return
	}
	// #2308 v3: 服务端解析 wx_code 的 openid，只有与 session.OpenID 一致才允许绑定，
	// 不接受客户端提交的 openid/exchange_token 作为所有权证明。
	wxRes, wxErr := h.iamService.WxAccounts(req.WxCode)
	if wxErr != nil || wxRes == nil || wxRes.OpenID == "" {
		msg := "微信授权失败"
		if wxErr != nil {
			msg += ": " + wxErr.Error()
		}
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": msg})
		return
	}
	if wxRes.OpenID != session.OpenID {
		c.JSON(http.StatusForbidden, gin.H{"code": 40301, "message": "微信身份与注册账户不一致"})
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":         "pending",
		"error":          "",
		"exchange_token": wxRes.ExchangeToken,
		"updated_at":     now,
	}
	if err := db.Model(&session).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to update session: " + err.Error()})
		return
	}

	var record models.OrderPaymentRecord
	if err := db.Where("session_id = ? AND status = ?", session.ID, "paid").
		Order("created_at desc").First(&record).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "payment not completed for this session"})
		return
	}
	if err := applySideEffectsAndRecord(db, &record, now); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "重新授权后仍未完成: " + err.Error()})
		return
	}

	var updated models.RegistrationSession
	if err := db.Where("id = ?", session.ID).First(&updated).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to reload session"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
		"status":  updated.Status,
		"user_id": updated.IAMUserID,
	}})
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
	if side != "front" && side != "back" && side != "other" && side != "intro" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "message": "invalid side, must be front, back, other, or intro"})
		return
	}

	// #2057 裁定3: 第二证件类型自报 + student 必传介绍信（注册会话版）
	secondDocType := c.PostForm("second_doc_type")
	introLetterKey := c.PostForm("intro_letter_url")
	if side == "other" {
		if secondDocType != "" && !validSecondDocType(secondDocType) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "second_doc_type 需为 student/teacher/work/other"})
			return
		}
		if secondDocType == "student" && introLetterKey == "" {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    40902,
				"message": "学生证作为第二证件时需上传介绍信",
				"data":    gin.H{"reasons": []string{"no_intro_letter"}},
			})
			return
		}
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
	if side == "intro" {
		// 学生证介绍信：单独上传，只写 form.IntroLetterURL。
		form.IntroLetterURL = filename
	} else {
		form.IDPhotos[side] = filename
		// #2057 裁定3: 自报类型/介绍信随 form_data 落地（类型在注册完成时仍由审核端权威决定，沿用既有不变量）
		if side == "other" {
			if secondDocType != "" {
				form.IdPhotoOtherType = secondDocType
			}
			if secondDocType == "student" {
				form.IntroLetterURL = introLetterKey
			}
		}
	}

	if err := h.db.Model(&session).Update("form_data", marshalForm(form)).Error; err != nil {
		log.Printf("session form_data update failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50004, "message": "failed to save photo reference"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    20000,
		"message": "upload success",
		"data": gin.H{
			"side":     side,
			"file_key": filename,
		},
	})
}
