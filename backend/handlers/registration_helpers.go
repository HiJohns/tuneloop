package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// registerForm mirrors the registration form persisted in
// registration_sessions.form_data. Kept parallel to PostRegister's request
// struct — the two-phase flow (#1663) runs side-by-side with the legacy
// register endpoint (which is untouched).
type registerForm struct {
	Nickname string                 `json:"nickname"`
	Name     string                 `json:"name"`
	Phone    string                 `json:"phone"`
	Email    string                 `json:"email"`
	Ref      string                 `json:"ref"`
	Address  map[string]interface{} `json:"address"`
	IDPhotos []string               `json:"id_photos"`
}

// createIAMUserWithBind creates the IAM user and binds the WeChat identity
// (exchange_token preferred, wx_code fallback — mirroring PostRegister
// #1640/#1644). Returns the IAM user id and bound openid. Any WxBind failure
// surfaces as an error (red-line #1637: never swallow external API errors).
func createIAMUserWithBind(iamService *services.IAMService, form *registerForm, password, exchangeToken, wxCode string) (userID, openid string, err error) {
	userName := form.Phone
	iamClient := services.NewIAMClient()
	createReq := &services.CreateUserRequest{
		Username:       userName,
		Name:           form.Name,
		Phone:          form.Phone,
		Email:          form.Email,
		Password:       password,
		SkipActivation: true,
	}
	if form.Nickname != "" {
		n := form.Nickname
		createReq.Nickname = &n
	}
	createResp, createErr := iamClient.CreateUser(createReq)
	if createErr != nil {
		return "", "", fmt.Errorf("create IAM user: %w", createErr)
	}

	if exchangeToken != "" || wxCode != "" {
		bindResult, bindErr := iamService.WxBind(exchangeToken, wxCode, createResp.UserID)
		if bindErr != nil {
			// #1637 red-line fix: never swallow the bind failure. Unlike
			// PostRegister there is no purge here — the membership fee has
			// already been paid, the session is marked failed for manual
			// handling instead.
			return "", "", fmt.Errorf("wx-bind failed for user %s: %w", createResp.UserID, bindErr)
		}
		openid = bindResult.WxOpenid
	}
	return createResp.UserID, openid, nil
}

// syncLocalUserAndRewards mirrors PostRegister's local users cache sync
// (iam_sub, ref_code, registration gift points, referral bonus). Best-effort
// for the local cache; IAM remains the source of truth.
func syncLocalUserAndRewards(db *gorm.DB, iamUserID, tenantID, orgID, openid string, form *registerForm) *models.User {
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000000"
	}
	if orgID == "" {
		orgID = "00000000-0000-0000-0000-000000000000"
	}
	newUser := models.User{
		IAMSub:              iamUserID,
		TenantID:            tenantID,
		OrgID:               orgID,
		Username:            form.Phone,
		Name:                form.Name,
		Phone:               form.Phone,
		Email:               form.Email,
		Role:                "USER",
		Status:              "active",
		IsProfileCompleted:  true,
		OnboardingCompleted: true,
		WxOpenid:            openid,
	}
	if err := db.Create(&newUser).Error; err != nil {
		log.Printf("[register helper] failed to create local user for iam_sub %s: %v", iamUserID, err)
		return nil
	}

	refCode := newUser.ID[:8]
	db.Model(&newUser).Update("ref_code", refCode)

	// Membership registration gift points (#1533): default 99, configurable
	// via system_settings membership_gift_points.
	giftPoints := 99.0
	var giftSetting models.SystemSetting
	if err := db.Where("setting_key = ?", "membership_gift_points").First(&giftSetting).Error; err == nil {
		if v, perr := strconv.ParseFloat(giftSetting.SettingValue, 64); perr == nil {
			giftPoints = v
		}
	}
	if giftPoints > 0 {
		db.Model(&newUser).Update("promo_points", gorm.Expr("promo_points + ?", giftPoints))
		db.Create(&models.PointsTransaction{
			ID:          uuid.New().String(),
			UserID:      newUser.ID,
			TenantID:    newUser.TenantID,
			Type:        "registration",
			Amount:      giftPoints,
			Description: "会员注册赠点",
			CreatedAt:   time.Now(),
		})
	}

	// Referral (#1496/#1534): only when the ref param was provided.
	if form.Ref != "" && form.Ref != refCode {
		var referrer models.User
		if db.Where("ref_code = ?", form.Ref).First(&referrer).Error == nil {
			db.Create(&models.Referral{
				ReferrerID: referrer.ID,
				RefereeID:  newUser.ID,
				RefCode:    form.Ref,
				Status:     "registered",
			})
			if referrer.MembershipLevelID != nil {
				if ratios := services.GetGiftRatios(*referrer.MembershipLevelID); ratios != nil && ratios.ReferralRegPoints > 0 {
					db.Model(&models.User{}).Where("id = ?", referrer.ID).Updates(map[string]interface{}{
						"promo_points": gorm.Expr("promo_points + ?", ratios.ReferralRegPoints),
						"updated_at":   time.Now(),
					})
					db.Create(&models.PointsTransaction{
						ID:          uuid.New().String(),
						UserID:      referrer.ID,
						TenantID:    referrer.TenantID,
						Type:        "referral_reg",
						Amount:      ratios.ReferralRegPoints,
						Description: fmt.Sprintf("介绍新用户注册奖励 %s", newUser.Username),
						CreatedAt:   time.Now(),
					})
				}
			}
		}
	}
	return &newUser
}

// getMembershipFee returns the membership registration fee from
// system_settings (default 99).
func getMembershipFee(db *gorm.DB) float64 {
	fee := 99.0
	var feeSetting models.SystemSetting
	if err := db.Where("setting_key = ?", "membership_fee").First(&feeSetting).Error; err == nil {
		if v, perr := strconv.ParseFloat(feeSetting.SettingValue, 64); perr == nil {
			fee = v
		}
	}
	return fee
}

// completeRegistrationFromSession runs the payment-callback side of the
// two-phase registration (#1663): the membership fee has been paid, so the
// account is created server-side from the session's form_data. Idempotent —
// a completed session is skipped (repeat callbacks only create once).
func completeRegistrationFromSession(tx *gorm.DB, record *models.OrderPaymentRecord, now time.Time) error {
	var raw struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal([]byte(*record.RawResponse), &raw); err != nil || raw.SessionID == "" {
		return fmt.Errorf("registration session id missing in raw_response")
	}

	var session models.RegistrationSession
	if err := tx.Where("id = ?", raw.SessionID).First(&session).Error; err != nil {
		return fmt.Errorf("registration session not found: %w", err)
	}
	switch session.Status {
	case "completed":
		return nil // idempotent: repeat callback must not create a second account
	case "failed":
		return fmt.Errorf("registration session already failed: %s", session.Error)
	}

	var form registerForm
	if err := json.Unmarshal([]byte(session.FormData), &form); err != nil {
		return fmt.Errorf("invalid session form_data: %w", err)
	}

	// The wx.login code is long expired by callback time; the single-use
	// exchange_token minted at session creation is still valid.
	iamService := services.NewIAMService()
	password := "wx_" + uuid.NewString()[:20]
	userID, openid, err := createIAMUserWithBind(iamService, &form, password, session.ExchangeToken, "")
	if err != nil {
		// Paid but account creation failed → keep a traceable failed record
		// for manual handling (red-line #1637: error surfaced).
		tx.Model(&session).Updates(map[string]interface{}{
			"status":     "failed",
			"error":      err.Error(),
			"updated_at": now,
		})
		return err
	}

	// Local users cache + registration gift points + referral bonus
	// (best-effort local cache; IAM is the source of truth).
	localUser := syncLocalUserAndRewards(tx, userID, "", "", openid, &form)
	if localUser != nil {
		activateMembershipLevelForAmount(tx, localUser.ID, record.Amount)
	}

	return tx.Model(&session).Updates(map[string]interface{}{
		"status":       "completed",
		"completed_at": now,
		"updated_at":   now,
		"error":        "",
	}).Error
}

// activateMembershipLevelForAmount activates the highest membership level
// whose MinAmount <= paid amount (mirrors the legacy membership callback
// branch, #1532/#1575).
func activateMembershipLevelForAmount(db *gorm.DB, localUserID string, amount float64) {
	var levels []models.MembershipLevel
	db.Order("min_amount ASC").Find(&levels)
	newLevelID := 0
	for _, l := range levels {
		if amount >= l.MinAmount {
			newLevelID = l.ID
		}
	}
	if newLevelID > 0 {
		if err := db.Model(&models.User{}).Where("id = ?", localUserID).
			Update("membership_level_id", newLevelID).Error; err != nil {
			log.Printf("[completeRegistration] membership level activate failed: %v", err)
		}
	}
}


