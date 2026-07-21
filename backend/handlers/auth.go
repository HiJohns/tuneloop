package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AuthHandler struct {
	iamService *services.IAMService
	db         *gorm.DB
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

func NewAuthHandler(db *gorm.DB) *AuthHandler {
	iamSvc := services.NewIAMService()
	return &AuthHandler{
		iamService: iamSvc,
		db:         db,
	}
}

func (h *AuthHandler) GetOIDCAuthorizationURL(c *gin.Context) {
	externalURL := services.GetIAMExternalURL()
	iamNs := os.Getenv("IAM_NAMESPACE")
	clientID := ""
	if iamNs != "" {
		clientID = iamNs + "_web"
	}
	if clientID == "" {
		clientID = os.Getenv("IAM_PC_CLIENT_ID")
	}
	if clientID == "" {
		clientID = os.Getenv("IAM_CLIENT_ID")
	}
	redirectURI := os.Getenv("EXTERNAL_WEB_URL")
	if redirectURI == "" {
		redirectURI = "http://localhost:5554"
	}
	redirectURI += "/callback"

	culture := middleware.GetCulture(c)
	authURL := externalURL + "/oauth/authorize?client_id=" + clientID + "&redirect_uri=" + redirectURI + "&response_type=code&culture=" + culture + "&noRegister=1"

	state := generateRandomState(32)
	c.SetCookie("oauth_state", state, 300, "/", "", false, false)

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"authorization_url": authURL + "&state=" + state,
		},
	})
}

func generateRandomState(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return ""
	}
	for i := range bytes {
		bytes[i] = charset[int(bytes[i])%len(charset)]
	}
	return string(bytes)
}

func (h *AuthHandler) Callback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	var postBody struct {
		Code       string `json:"code"`
		State      string `json:"state"`
		ClientType string `json:"client_type"`
	}
	if c.Request.Method == "POST" {
		if err := c.ShouldBindJSON(&postBody); err == nil {
			if code == "" {
				code = postBody.Code
			}
			if state == "" {
				state = postBody.State
			}
		}
	}

	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "missing required parameter: code",
		})
		return
	}

	// Note: State validation is currently disabled for cross-domain cookie compatibility.
	// The oauth_state cookie set by /api/auth/oidc/authorization-url is not reliably
	// sent by browsers when redirecting from IAM back to the callback.
	// TODO: Implement server-side state storage (e.g., Redis) for production.
	_ = state // Suppress unused variable warning

	c.SetCookie("oauth_state", "", -1, "/", "", false, false)

	redirectURI := os.Getenv("EXTERNAL_WEB_URL")
	if redirectURI == "" {
		redirectURI = "http://localhost:5554"
	}
	redirectURI += "/callback"

	clientType := c.Query("client_type")
	if clientType == "" {
		clientType = postBody.ClientType
	}
	if clientType == "wx" || clientType == "wechat" || clientType == "mobile" {
		if wxURI := os.Getenv("EXTERNAL_MOBILE_URL"); wxURI != "" {
			redirectURI = wxURI + "/callback"
		}
	}

	if referer := c.GetHeader("Referer"); os.Getenv("EXTERNAL_WEB_URL") != "" && referer != "" {
		if strings.Contains(referer, "wx.") || strings.Contains(referer, "wx-") {
			if wxURI := os.Getenv("EXTERNAL_MOBILE_URL"); wxURI != "" {
				redirectURI = wxURI + "/callback"
			}
		}
	}

	// Use app-specific credentials (not namespace) for code exchange
	iamNs := os.Getenv("IAM_NAMESPACE")
	appClientID := iamNs + "_web"
	appSecret := services.GetAppSecret(appClientID)
	if clientType == "wx" || clientType == "wechat" || clientType == "mobile" {
		appClientID = iamNs + "_wechat"
		appSecret = services.GetAppSecret(appClientID)
	}

	var tokenResp *services.TokenResponse
	var err error
	// Exchange code — must use app-level client_id to match OAuth authorize request
	// ExchangeCodeWithRedirect (namespace-level) uses wrong client_id, skip it entirely
	tokenResp, err = services.ExchangeCode(appClientID, appSecret, code, redirectURI)
	if err != nil {
		log.Printf("[Auth] Token exchange failed: client=%s redirectURI=%s error=%v", appClientID, redirectURI, err)
		if strings.Contains(err.Error(), "already used") {
			c.JSON(http.StatusOK, gin.H{
				"code":    20000,
				"message": "code already used, please login again",
				"data":    gin.H{"relogin": true},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to exchange code for token: " + err.Error(),
		})
		return
	}

	// Validate token and extract claims to get tenant_id
	claims, err := h.iamService.ValidateToken(tokenResp.AccessToken)
	if err != nil {
		// Still set token but with empty tenant_id - middleware can handle this
		c.Set("tenant_id", "")
	} else {
		// Set tenant_id in context from claims
		c.Set("tenant_id", claims.TenantID)

		// Sync IAM user to local users table (same pattern as WxLogin)
		var existing models.User
		if err := h.db.Where("iam_sub = ?", claims.UserID).First(&existing).Error; err != nil {
				tenantID := claims.TenantID
			if tenantID == "" { tenantID = "00000000-0000-0000-0000-000000000000" }
			orgID := claims.OrgID
			if orgID == "" { orgID = "00000000-0000-0000-0000-000000000000" }
			newUser := models.User{
				IAMSub:   claims.UserID,
				TenantID: tenantID,
				OrgID:    orgID,
				Name:     claims.Name,
				Role:     "USER",
				Status:   "active",
			}
			if createErr := h.db.Create(&newUser).Error; createErr != nil {
				log.Printf("[Auth] Failed to create local user for iam_sub %s: %v", claims.UserID, createErr)
			}
		}
	}

	// Set access_token and refresh_token cookies
	// PC 端会话时长设置为 1 小时 (3600 秒)
	c.SetSameSite(http.SameSiteLaxMode)

	// Set cookie domain for subdomain sharing
	cookieDomain := ""
	if c.Request.Host != "" {
		if strings.Contains(c.Request.Host, "cadenzayueqi.com") {
			cookieDomain = ".cadenzayueqi.com"
		} else if strings.Contains(c.Request.Host, "linxdeep.com") {
			cookieDomain = ".linxdeep.com"
		}
	}

	if cookieDomain != "" {
		maxAge := tokenResp.ExpiresIn
		if maxAge <= 0 {
			maxAge = 900
		}
		c.SetCookie("token", tokenResp.AccessToken, maxAge, "/", cookieDomain, false, false)
		c.SetCookie("refresh_token", tokenResp.RefreshToken, 2592000, "/", cookieDomain, false, true)
	} else {
		maxAge := tokenResp.ExpiresIn
		if maxAge <= 0 {
			maxAge = 900
		}
		c.SetCookie("token", tokenResp.AccessToken, maxAge, "/", "", false, false)
		c.SetCookie("refresh_token", tokenResp.RefreshToken, 2592000, "/", "", false, true)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": tokenResp,
	})
}

func (h *AuthHandler) PostLogin(c *gin.Context) {
	var req struct {
		Identifier string `json:"identifier" binding:"required"`
		Password   string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid request: " + err.Error(),
		})
		return
	}

	tokenResp, err := h.iamService.IAMLogin(req.Identifier, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    40001,
			"message": "IAM login failed: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": tokenResp,
	})
}

func (h *AuthHandler) PostRegister(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Nickname string `json:"nickname"`
		Name     string `json:"name" binding:"required"`
		Phone    string `json:"phone" binding:"required"`
		Email    string `json:"email"`
		Password string `json:"password" binding:"required"`
		WxCode   string `json:"wx_code"`
		Ref      string `json:"ref"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid request: " + err.Error(),
		})
		return
	}

	userName := req.Username
	if userName == "" {
		userName = req.Phone
	}

	// Create user in beaconiam via IAMClient (uses client credentials internally)
	iamClient := services.NewIAMClient()
	createReq := &services.CreateUserRequest{
		Username:       userName,
		Name:           req.Name,
		Phone:          req.Phone,
		Email:          req.Email,
		Password:       req.Password,
		SkipActivation: true,
	}
	if req.Nickname != "" {
		n := req.Nickname
		createReq.Nickname = &n
	}
	_, createErr := iamClient.CreateUser(createReq)
	if createErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "IAM register failed: " + createErr.Error(),
		})
		return
	}

	// If wx_code provided, bind WeChat to this user
	if req.WxCode != "" {
		if tokenResp, wxErr := h.iamService.WxLogin(req.WxCode); wxErr == nil && tokenResp != nil && tokenResp.AccessToken != "" {
			// Sync to local users table
			claims, parseErr := h.iamService.ValidateToken(tokenResp.AccessToken)
			if parseErr == nil && claims.UserID != "" {
				var existing models.User
				if h.db.Where("iam_sub = ?", claims.UserID).First(&existing).Error != nil {
					tenantID := claims.TenantID
					if tenantID == "" { tenantID = "00000000-0000-0000-0000-000000000000" }
					orgID := claims.OrgID
					if orgID == "" { orgID = "00000000-0000-0000-0000-000000000000" }
					newUser := models.User{
						IAMSub:             claims.UserID,
						TenantID:           tenantID,
						OrgID:              orgID,
						Username:           userName,
						Name:               req.Name,
						Phone:              req.Phone,
						Email:              req.Email,
						Role:               "USER",
						Status:             "active",
						IsProfileCompleted: true,
						WxOpenid:           tokenResp.WxOpenid,
					}
					if createErr := h.db.Create(&newUser).Error; createErr != nil {
						log.Printf("[Register] Failed to create local user for iam_sub %s: %v", claims.UserID, createErr)
					} else {
						refCode := newUser.ID[:8]
						h.db.Model(&newUser).Update("ref_code", refCode)
						if req.Ref != "" && req.Ref != refCode {
							var referrer models.User
							if h.db.Where("ref_code = ?", req.Ref).First(&referrer).Error == nil {
								h.db.Create(&models.Referral{
									ReferrerID: referrer.ID,
									RefereeID:  newUser.ID,
									RefCode:    req.Ref,
									Status:     "registered",
								})
							}
						}
					}
				}
			}
			c.JSON(http.StatusOK, gin.H{
				"code": 20000,
				"data": tokenResp,
			})
			return
		}
	}

	// Login to get JWT via password grant
	tokenResp, loginErr := h.iamService.IAMLogin(userName, req.Password)
	if loginErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "IAM login after register failed: " + loginErr.Error(),
		})
		return
	}

	// Sync to local users table
	claims, parseErr := h.iamService.ValidateToken(tokenResp.AccessToken)
	if parseErr == nil && claims.UserID != "" {
		var existing models.User
		if h.db.Where("iam_sub = ?", claims.UserID).First(&existing).Error != nil {
			tenantID := claims.TenantID
			if tenantID == "" { tenantID = "00000000-0000-0000-0000-000000000000" }
			orgID := claims.OrgID
			if orgID == "" { orgID = "00000000-0000-0000-0000-000000000000" }
			newUser := models.User{
				IAMSub:             claims.UserID,
				TenantID:           tenantID,
				OrgID:              orgID,
				Username:           userName,
				Name:               req.Name,
				Phone:              req.Phone,
				Email:              req.Email,
				Role:               "USER",
				Status:             "active",
				IsProfileCompleted: true,
			}
			if createErr := h.db.Create(&newUser).Error; createErr != nil {
				log.Printf("[Register] Failed to create local user for iam_sub %s: %v", claims.UserID, createErr)
			} else {
				refCode := newUser.ID[:8]
				h.db.Model(&newUser).Update("ref_code", refCode)
				if req.Ref != "" && req.Ref != refCode {
					var referrer models.User
					if h.db.Where("ref_code = ?", req.Ref).First(&referrer).Error == nil {
						h.db.Create(&models.Referral{
							ReferrerID: referrer.ID,
							RefereeID:  newUser.ID,
							RefCode:    req.Ref,
							Status:     "registered",
						})
					}
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": tokenResp,
	})
}

func (h *AuthHandler) WxLogin(c *gin.Context) {
	var req struct {
		Code          string `json:"code" binding:"required"`
		EncryptedData string `json:"encrypted_data"`
		IV            string `json:"iv"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "missing required parameter: code",
		})
		return
	}

	// Channel 1 (weapp one-click): has encryptedData + iv → decrypt phone → find/create USER
	if req.EncryptedData != "" && req.IV != "" {
		phoneResp, err := h.iamService.WxPhone(req.EncryptedData, req.IV)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "wx-phone decrypt failed: " + err.Error(),
			})
			return
		}

		phone, _ := phoneResp["purePhoneNumber"].(string)
		if phone == "" {
			phone, _ = phoneResp["phoneNumber"].(string)
		}

		if phone == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    40002,
				"message": "unable to extract phone number",
			})
			return
		}

		// Try IAM wx-login to get/authenticate existing user
		tokenResp, wxErr := h.iamService.WxLogin(req.Code)
		isNew := false

		if wxErr != nil {
			// IAM doesn't know this user yet — create locally
			user := models.User{
				Name:               "微信用户" + phone[len(phone)-4:],
				Phone:              phone,
				Role:               "USER",
				Status:             "active",
				IsProfileCompleted: false,
			}
			if err := h.db.Create(&user).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    50000,
					"message": "failed to create user: " + err.Error(),
				})
				return
			}

			// Issue local JWT for this new user
			guestToken, tokenErr := h.iamService.CreateGuestToken()
			if tokenErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    50000,
					"message": "failed to issue token: " + tokenErr.Error(),
				})
				return
			}
			isNew = true

			c.JSON(http.StatusOK, gin.H{
				"code": 20000,
				"data": gin.H{
					"token":      guestToken.AccessToken,
					"token_type": guestToken.TokenType,
					"expires_in": guestToken.ExpiresIn,
					"user": gin.H{
						"id":                   user.ID,
						"name":                 user.Name,
						"phone":                user.Phone,
						"role":                 "USER",
						"is_profile_completed": false,
					},
					"is_new": isNew,
				},
			})
			return
		}

		if tokenResp != nil && tokenResp.AccessToken != "" {
			claims, parseErr := h.iamService.ValidateToken(tokenResp.AccessToken)
			if parseErr == nil && claims.UserID != "" {
				var existingUser models.User
				if h.db.Where("iam_sub = ?", claims.UserID).First(&existingUser).Error != nil {
					log.Printf("[WxLogin] Channel 1: iam_sub=%s not found locally, returning binding error", claims.UserID)
					c.JSON(http.StatusConflict, gin.H{
						"code":    40900,
						"message": "微信账号绑定异常，请重新绑定。如已绑定请重新登录 Web 端账号后再次尝试。",
					})
					return
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"code": 20000,
			"data": gin.H{
				"token":      tokenResp.AccessToken,
				"token_type": tokenResp.TokenType,
				"expires_in": tokenResp.ExpiresIn,
				"is_new":     isNew,
			},
		})
		return
	}

	// Channel 3 (silent guest): only code, no encryptedData → try IAM, fallback to GUEST
	tokenResp, err := h.iamService.WxLogin(req.Code)
	if err == nil && tokenResp != nil && tokenResp.AccessToken != "" {
		// IAM recognized this user — sync and return IAM token
		claims, parseErr := h.iamService.ValidateToken(tokenResp.AccessToken)
		if parseErr == nil && claims.UserID != "" {
			var existingUser models.User
			if h.db.Where("iam_sub = ?", claims.UserID).First(&existingUser).Error != nil {
				log.Printf("[WxLogin] Channel 3: iam_sub=%s not found locally, returning binding error", claims.UserID)
				c.JSON(http.StatusConflict, gin.H{
					"code":    40900,
					"message": "微信账号绑定异常，请重新绑定。如已绑定请重新登录 Web 端账号后再次尝试。",
				})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"code": 20000,
			"data": gin.H{
				"token":      tokenResp.AccessToken,
				"token_type": tokenResp.TokenType,
				"expires_in": tokenResp.ExpiresIn,
				"is_new":     false,
			},
		})
		return
	}

	// IAM doesn't know this user → create GUEST locally
	guestToken, tokenErr := h.iamService.CreateGuestToken()
	if tokenErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to issue guest token: " + tokenErr.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"token":      guestToken.AccessToken,
			"token_type": guestToken.TokenType,
			"expires_in": guestToken.ExpiresIn,
			"user": gin.H{
				"role": "GUEST",
			},
		},
	})
}

func (h *AuthHandler) WxPhone(c *gin.Context) {
	var req struct {
		EncryptedData string `json:"encrypted_data" binding:"required"`
		IV            string `json:"iv" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "missing required parameters: encrypted_data, iv",
		})
		return
	}

	resp, err := h.iamService.WxPhone(req.EncryptedData, req.IV)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "wx-phone failed: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": resp,
	})
}

func (h *AuthHandler) WxPhoneCode(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "code required"})
		return
	}

	// Read WeChat credentials from env
	appID := os.Getenv("WX_APPID")
	appSecret := os.Getenv("WX_APPSECRET")
	if appID == "" || appSecret == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "WX_APPID or WX_APPSECRET not configured"})
		return
	}

	// Get WeChat access token
	tokenResp, err := http.Get(fmt.Sprintf("https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=%s&secret=%s", appID, appSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to get WeChat token: " + err.Error()})
		return
	}
	defer tokenResp.Body.Close()
	var tokenResult struct {
		AccessToken string `json:"access_token"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenResult); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to parse WeChat token"})
		return
	}
	if tokenResult.AccessToken == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "WeChat token error: " + tokenResult.ErrMsg})
		return
	}

	// Call getuserphonenumber API
	phoneReq := map[string]string{"code": req.Code}
	phoneBody, _ := json.Marshal(phoneReq)
	phoneResp, err := http.Post(
		fmt.Sprintf("https://api.weixin.qq.com/wxa/business/getuserphonenumber?access_token=%s", tokenResult.AccessToken),
		"application/json",
		bytes.NewBuffer(phoneBody),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to call WeChat phone API: " + err.Error()})
		return
	}
	defer phoneResp.Body.Close()
	var phoneResult struct {
		ErrCode int `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		PhoneInfo struct {
			PhoneNumber     string `json:"phoneNumber"`
			PurePhoneNumber string `json:"purePhoneNumber"`
			CountryCode     string `json:"countryCode"`
		} `json:"phone_info"`
	}
	if err := json.NewDecoder(phoneResp.Body).Decode(&phoneResult); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to parse phone response"})
		return
	}
	if phoneResult.ErrCode != 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "WeChat phone error: " + phoneResult.ErrMsg})
		return
	}

	phone := phoneResult.PhoneInfo.PurePhoneNumber
	if phone == "" {
		phone = phoneResult.PhoneInfo.PhoneNumber
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{"phone": phone},
	})
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid request body",
		})
		return
	}

	tokenResp, err := h.iamService.RefreshToken(req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to refresh token",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": tokenResp,
	})
}
