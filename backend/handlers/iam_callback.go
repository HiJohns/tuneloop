package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type CallbackEvent struct {
	Action    string      `json:"action"`
	Timestamp string      `json:"timestamp"`
	Payload   interface{} `json:"payload"`
}

var (
	idempotentCache = sync.Map{}
	idempotentTTL   = 5 * time.Minute
)

func HandleIAMCallback(c *gin.Context) {
	// Parse body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "failed to read body"})
		return
	}

	// Verify HMAC signature
	secret := os.Getenv("IAM_CALLBACK_SECRET")
	if secret == "" {
		log.Println("[IAMCallback] IAM_CALLBACK_SECRET not configured")
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "callback not configured"})
		return
	}
	signature := c.GetHeader("X-IAM-Signature")
	eventID := c.GetHeader("X-IAM-Event-ID")
	if signature == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "missing signature"})
		return
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "invalid signature"})
		return
	}

	// Idempotency check
	if eventID != "" {
		if _, loaded := idempotentCache.LoadOrStore(eventID, time.Now()); loaded {
			c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "already processed"})
			return
		}
		time.AfterFunc(idempotentTTL, func() { idempotentCache.Delete(eventID) })
	}

	// Parse event
	var event CallbackEvent
	if err := json.Unmarshal(body, &event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "invalid event body"})
		return
	}

	payloadBytes, _ := json.Marshal(event.Payload)
	var payload map[string]interface{}
	json.Unmarshal(payloadBytes, &payload)

	switch event.Action {
	case "user.registered":
		handleUserRegistered(payload, c)
		return
	case "user.org_bound":
		handleUserOrgBound(payload, c)
		return
	case "user.activated":
		// #2228 P1：IAM 邮件邀请激活（/confirm → status=active）——完成本侧邀请绑定
		handleUserActivated(payload, c)
		return
	default:
		log.Printf("[IAMCallback] unknown action: %s (ignored)", event.Action)
		c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "unknown action, ignored"})
	}
}

// handleUserActivated（#2228 P1）：受邀人经 IAM 邮件链接激活后，完成 tuneloop 侧：
//  1. 本地用户补建（预建未同步时的兜底）；
//  2. 该用户所有 pending 定向邀请的绑定（bindInvitee：组织关系 + 职能角色模板 #2032），
//     并标记邀请 accepted（激活即接受——邮件链路本人操作）。
//
// 失败返回 500 让 IAM 重试（回调带事件幂等；bindInvitee/标记均幂等）。
func handleUserActivated(payload map[string]interface{}, c *gin.Context) {
	userID, _ := payload["user_id"].(string) // IAM sub
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "user_id is required"})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())
	var u models.User
	err := db.Where("iam_sub = ?", userID).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// #2228：tenant_id/org_id 为 uuid NOT NULL——顾客形态用零 UUID（#2078/#2090）
		newUser := models.User{
			IAMSub: userID, Role: "USER", Status: "active",
			TenantID: "00000000-0000-0000-0000-000000000000",
			OrgID:    "00000000-0000-0000-0000-000000000000",
		}
		if email, ok := payload["email"].(string); ok {
			newUser.Email = email
		}
		if name, ok := payload["name"].(string); ok {
			newUser.Name = name
		}
		if phone, ok := payload["phone"].(string); ok {
			newUser.Phone = phone
		}
		if e := db.Create(&newUser).Error; e != nil {
			log.Printf("[IAMCallback] failed to create local user for activated iam_sub=%s: %v", userID, e)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create user"})
			return
		}
		log.Printf("[IAMCallback] created local user for activated iam_sub=%s", userID)
		u = newUser
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to load user"})
		return
	}

	// 匹配 pending 定向邀请（口径同 /user/invitations/pending）
	idents := []string{}
	for _, v := range []string{u.Phone, u.Email} {
		if strings.TrimSpace(v) != "" {
			idents = append(idents, v)
		}
	}
	q := db.Where("status = ? AND expires_at > ?", "pending", time.Now())
	if len(idents) > 0 {
		q = q.Where("invitee_user_id = ? OR (invitee_user_id IS NULL AND invitee_identifier IN ?)", u.ID, idents)
	} else {
		q = q.Where("invitee_user_id = ?", u.ID)
	}
	var invites []models.StaffInvite
	if err := q.Find(&invites).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "查询失败"})
		return
	}

	bound := 0
	for i := range invites {
		inv := &invites[i]
		if err := bindInvitee(db, inv, &u, inviteBindOpts{}); err != nil {
			log.Printf("[IAMCallback] bind invite %s for user %s failed: %v", inv.ID, u.ID, err)
			// 返回 500 触发 IAM 重试（全链路幂等）；错误如实返回，不静默
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "bind failed: " + err.Error()})
			return
		}
		now := time.Now()
		db.Model(&models.StaffInvite{}).Where("id = ?", inv.ID).Updates(map[string]interface{}{
			"status":          "accepted",
			"accepted_by":     u.ID,
			"accepted_at":     now,
			"invitee_user_id": u.ID,
		})
		bound++
	}

	log.Printf("[IAMCallback] user.activated processed: iam_sub=%s local=%s bound_invites=%d", userID, u.ID, bound)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"user_id": u.ID, "bound_invites": bound}})
}

func handleUserRegistered(payload map[string]interface{}, c *gin.Context) {
	userID, _ := payload["user_id"].(string)
	email, _ := payload["email"].(string)
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "user_id is required"})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())
	var existing models.User
	if err := db.Where("iam_sub = ?", userID).First(&existing).Error; err == nil {
		c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "user already exists"})
		return
	}

	newUser := models.User{
		IAMSub: userID,
		Email:  email,
		Role:   "USER",
		Status: "active",
		// 同族修复（#2228）：tenant_id/org_id 为 uuid NOT NULL，零值 '' 会 22P02
		TenantID: "00000000-0000-0000-0000-000000000000",
		OrgID:    "00000000-0000-0000-0000-000000000000",
	}
	if name, ok := payload["name"].(string); ok {
		newUser.Name = name
	}
	if phone, ok := payload["phone"].(string); ok {
		newUser.Phone = phone
	}

	if err := db.Create(&newUser).Error; err != nil {
		log.Printf("[IAMCallback] failed to create user %s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to create user"})
		return
	}

	log.Printf("[IAMCallback] created local user for iam_sub=%s email=%s", userID, email)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "user created"})
}

func handleUserOrgBound(payload map[string]interface{}, c *gin.Context) {
	userID, _ := payload["user_id"].(string)
	orgID, _ := payload["org_id"].(string)
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "message": "user_id is required"})
		return
	}

	db := database.GetDB().WithContext(c.Request.Context())
	var existing models.User
	if err := db.Where("iam_sub = ?", userID).First(&existing).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "user not found locally"})
		return
	}

	updates := map[string]interface{}{}
	if orgID != "" {
		updates["org_id"] = orgID
	}
	if role, ok := payload["role"].(string); ok {
		updates["role"] = role
	}
	if len(updates) > 0 {
		db.Model(&existing).Updates(updates)
	}

	log.Printf("[IAMCallback] updated org for iam_sub=%s org_id=%s", userID, orgID)
	c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "org binding updated"})
}
