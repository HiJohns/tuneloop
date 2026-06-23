package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
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
	default:
		log.Printf("[IAMCallback] unknown action: %s (ignored)", event.Action)
		c.JSON(http.StatusOK, gin.H{"code": 20000, "message": "unknown action, ignored"})
	}
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
