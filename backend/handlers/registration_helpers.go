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
	Nickname         string                 `json:"nickname"`
	Name             string                 `json:"name"`
	Phone            string                 `json:"phone"`
	Email            string                 `json:"email"`
	Ref              string                 `json:"ref"`
	Address          map[string]interface{} `json:"address"`
	IDPhotos         map[string]string      `json:"id_photos"`
	IdPhotoOtherType string                 `json:"id_photo_other_type"` // #1807: 第三证件类型
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
	if createResp.UserID == "" {
		return "", "", fmt.Errorf("empty user_id returned from IAM CreateUser")
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

// activateReservedUser activates the init user reserved at session creation
// (#1682): UpdateUser(status → active) then WxBind via the session's
// exchange_token. Returns the IAM user id and bound openid. Any failure
// surfaces as an error (red-line #1637: never swallow external API errors).
func activateReservedUser(iamService *services.IAMService, session *models.RegistrationSession) (userID, openid string, err error) {
	userID = *session.IAMUserID
	iamClient := services.NewIAMClient()
	if err := iamClient.UpdateUser(userID, &services.UpdateUserRequest{
		Status:     "active",
		OperatorID: userID,
	}); err != nil {
		return "", "", fmt.Errorf("activate IAM user: %w", err)
	}

	if session.ExchangeToken != "" {
		bindResult, bindErr := iamService.WxBind(session.ExchangeToken, "", userID)
		if bindErr != nil {
			return "", "", fmt.Errorf("wx-bind failed for user %s: %w", userID, bindErr)
		}
		openid = bindResult.WxOpenid
	}
	return userID, openid, nil
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
		Nickname:            form.Nickname,
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

	grantRegistrationRewards(db, &newUser, form)
	return &newUser
}

// grantRegistrationRewards credits ref_code, registration gift points and
// the referral bonus on a local user (shared by the legacy create path and
// the #1688 reserved-user activation path).
func grantRegistrationRewards(db *gorm.DB, user *models.User, form *registerForm) {
	refCode := user.ID[:8]
	db.Model(user).Update("ref_code", refCode)

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
		// #1757: promo_points in cents (1 点 = 1 分).
		giftPointsCents := models.FromYuan(giftPoints)
		db.Model(user).Update("promo_points", gorm.Expr("promo_points + ?", giftPointsCents))
		db.Create(&models.PointsTransaction{
			ID:          uuid.New().String(),
			UserID:      user.ID,
			TenantID:    user.TenantID,
			Type:        "registration",
			Amount:      giftPointsCents,
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
				RefereeID:  user.ID,
				RefCode:    form.Ref,
				Status:     "registered",
			})
			if referrer.MembershipLevelID != nil {
				if ratios := services.GetGiftRatios(*referrer.MembershipLevelID); ratios != nil && ratios.ReferralRegPoints > 0 {
					regCents := models.FromYuan(ratios.ReferralRegPoints)
					db.Model(&models.User{}).Where("id = ?", referrer.ID).Updates(map[string]interface{}{
						"promo_points": gorm.Expr("promo_points + ?", regCents),
						"updated_at":   time.Now(),
					})
					db.Create(&models.PointsTransaction{
						ID:          uuid.New().String(),
						UserID:      referrer.ID,
						TenantID:    referrer.TenantID,
						Type:        "referral_reg",
						Amount:      regCents,
						Description: fmt.Sprintf("介绍新用户注册奖励 %s", user.Username),
						CreatedAt:   time.Now(),
					})
				}
			}
		}
	}
}

// activateReservedLocalUser (#1688): the local users cache was reserved with
// status=init at session creation — activate it (status→active, bind openid)
// and grant the registration rewards. Best-effort cache; IAM is authoritative.
func activateReservedLocalUser(db *gorm.DB, session *models.RegistrationSession, openid string, form *registerForm) *models.User {
	if session.LocalUserID == nil {
		return nil
	}
	var user models.User
	if err := db.Where("id = ?", *session.LocalUserID).First(&user).Error; err != nil {
		log.Printf("[register helper] reserved local user %s not found: %v", *session.LocalUserID, err)
		return nil
	}
	db.Model(&user).Updates(map[string]interface{}{
		"status":               "active",
		"wx_openid":            openid,
		"onboarding_completed": true,
		"updated_at":           time.Now(),
	})
	grantRegistrationRewards(db, &user, form)
	return &user
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
	if record.SessionID == nil || *record.SessionID == "" {
		return fmt.Errorf("registration session id missing")
	}

	var session models.RegistrationSession
	if err := tx.Where("id = ?", *record.SessionID).First(&session).Error; err != nil {
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
	var userID, openid string
	var err error
	if session.IAMUserID != nil {
		// New path (#1682): the account was reserved with status=init at
		// session creation — activate it (status → active) and bind WeChat.
		userID, openid, err = activateReservedUser(iamService, &session)
	} else {
		// Legacy path: pre-#1682 sessions have no reserved user — create now.
		password := "wx_" + uuid.NewString()[:20]
		userID, openid, err = createIAMUserWithBind(iamService, &form, password, session.ExchangeToken, "")
	}
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
	var localUser *models.User
	if session.LocalUserID != nil {
		// #1688: reserved at session creation — activate the existing cache
		// record (payment record already references this user_id).
		localUser = activateReservedLocalUser(tx, &session, openid, &form)
	} else {
		localUser = syncLocalUserAndRewards(tx, userID, "", "", openid, &form)
	}
	if localUser != nil {
		activateMembershipLevelForAmount(tx, localUser.ID, record.Amount.ToYuan())

		// Transfer ID photos from session form_data to the user record (#1787).
		// The form_data.id_photos is a map of side→storage_key populated by
		// the session-scoped upload endpoint (POST /auth/registration-sessions/:id/id-photo).
		if len(form.IDPhotos) > 0 {
			photoUpdates := map[string]interface{}{}
			for side, key := range form.IDPhotos {
				switch side {
				case "front":
					photoUpdates["id_photo_front"] = key
				case "back":
					photoUpdates["id_photo_back"] = key
				case "other":
					photoUpdates["id_photo_other"] = key
				}
			}
			if len(photoUpdates) > 0 {
				tx.Model(&models.User{}).Where("id = ?", localUser.ID).Updates(photoUpdates)
			}
		}
		// #1807: 第三证件类型一并转移。
		if form.IdPhotoOtherType != "" {
			tx.Model(&models.User{}).Where("id = ?", localUser.ID).
				Update("id_photo_other_type", form.IdPhotoOtherType)
		}
	}

	return tx.Model(&session).Updates(map[string]interface{}{
		"status":       "completed",
		"completed_at": now,
		"updated_at":   now,
		"error":        "",
	}).Error
}

// StartInitReservationCleanupScheduler periodically releases stale init
// reservations (#1682): a registration session whose reserved user stayed in
// status=init for over initReservationTTL (24h) means the user never paid —
// purge the IAM user (frees email/phone) and fail the session so the form
// can be resubmitted.
const initReservationTTL = 24 * time.Hour

func StartInitReservationCleanupScheduler(db *gorm.DB) {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			cleanupStaleInitReservations(db)
		}
	}()
	log.Println("[InitReservationScheduler] started (1h interval)")
}

func cleanupStaleInitReservations(db *gorm.DB) {
	cutoff := time.Now().Add(-initReservationTTL)

	var sessions []models.RegistrationSession
	if err := db.Where("status = ? AND created_at < ?", "pending", cutoff).Find(&sessions).Error; err != nil {
		log.Printf("[InitReservationScheduler] query failed: %v", err)
		return
	}

	iamClient := services.NewIAMClient()
	for _, s := range sessions {
		// Release the reserved IAM user (init → purge). Best-effort: if IAM
		// already removed it, the purge is a no-op error we tolerate. Legacy
		// pre-#1682 sessions have no reserved user — just fail the session.
		if s.IAMUserID != nil {
			if err := iamClient.PurgeUser(*s.IAMUserID); err != nil {
				log.Printf("[InitReservationScheduler] purge failed for user %s: %v", *s.IAMUserID, err)
				continue
			}
			log.Printf("[InitReservationScheduler] released stale init reservation: session=%s user=%s", s.ID, *s.IAMUserID)
		}
		// #1688: drop the reserved local users cache alongside the IAM user.
		if s.LocalUserID != nil {
			if err := db.Where("id = ?", *s.LocalUserID).Delete(&models.User{}).Error; err != nil {
				log.Printf("[InitReservationScheduler] local cache delete failed for %s: %v", *s.LocalUserID, err)
			}
		}
		if err := db.Model(&s).Update("status", "failed").Error; err != nil {
			log.Printf("[InitReservationScheduler] fail session %s failed: %v", s.ID, err)
		}
	}
}

// activateMembershipLevelForAmount activates the highest membership level
// whose MinAmount <= paid amount (mirrors the legacy membership callback
// branch, #1532/#1575).
func activateMembershipLevelForAmount(db *gorm.DB, localUserID string, amount float64) {
	var levels []models.MembershipLevel
	db.Order("min_amount ASC").Find(&levels)
	newLevelID := 0
	for _, l := range levels {
		if models.FromYuan(amount) >= l.MinAmount {
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
