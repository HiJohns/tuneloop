package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// #2320: 生产端 H5 JS 错误上报。只做可观测性，不产生任何业务写操作。
const (
	clientErrorZeroUUID     = "00000000-0000-0000-0000-000000000000"
	clientErrorMaxMessage   = 1024
	clientErrorMaxStack     = 8192
	clientErrorMaxRoute     = 512
	clientErrorMaxVersion   = 64
	clientErrorMaxUserAgent = 500
	clientErrorMaxBody      = 16 * 1024
	clientErrorWindow       = time.Minute
)

var (
	clientErrorBearerRe = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/\-]+=*`)
	clientErrorFieldRe  = regexp.MustCompile(`(?i)((?:token|access_token|refresh_token|phone|id_card|id_card_no|password)\s*[=:]\s*)[^\s,;]+`)
	clientErrorPhoneRe  = regexp.MustCompile(`\b1[3-9]\d{9}\b`)
	clientErrorIDRe     = regexp.MustCompile(`\b\d{17}[\dXx]\b`)
)

type clientErrorRequest struct {
	Message   string `json:"message"`
	Stack     string `json:"stack"`
	Route     string `json:"route"`
	Version   string `json:"version"`
	UserAgent string `json:"user_agent"`
}

var clientErrorLimiter = struct {
	mu   sync.Mutex
	seen map[string]time.Time
}{seen: make(map[string]time.Time)}

// ClientError POST /api/client-error
func ClientError(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, clientErrorMaxBody)

	var req clientErrorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "invalid client error payload"})
		return
	}

	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "message is required"})
		return
	}
	if len(req.Message) > clientErrorMaxMessage ||
		len(req.Stack) > clientErrorMaxStack ||
		len(req.Route) > clientErrorMaxRoute ||
		len(req.Version) > clientErrorMaxVersion ||
		len(req.UserAgent) > clientErrorMaxUserAgent {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "client error payload too large"})
		return
	}

	message := redactClientError(req.Message)
	stack := redactClientError(req.Stack)
	route := req.Route
	version := req.Version
	ua := truncateString(req.UserAgent, clientErrorMaxUserAgent)
	ip := c.ClientIP()

	if !clientErrorAllowed(ip, message) {
		c.Status(http.StatusNoContent)
		return
	}

	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	if tenantID == "" {
		tenantID = clientErrorZeroUUID
	}
	userID := middleware.GetUserID(ctx)
	if userID == "" {
		userID = clientErrorZeroUUID
	}
	orgID := middleware.GetOrgID(ctx)

	detailsBytes, err := json.Marshal(map[string]string{
		"route":   route,
		"version": version,
		"message": message,
		"stack":   stack,
	})
	if err != nil {
		log.Printf("[client-error] details marshal failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to record client error"})
		return
	}
	details := string(detailsBytes)

	record := models.AuditLog{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		UserID:       userID,
		Action:       "client_error",
		ResourceType: "frontend",
		ResourceID:   truncateString(route, 100),
		StatusCode:   http.StatusInternalServerError,
		Status:       "error",
		Details:      &details,
		IPAddress:    ip,
		UserAgent:    ua,
		CreatedAt:    time.Now(),
	}
	if orgID != "" {
		record.OrgID = &orgID
	}

	if err := database.GetDB().WithContext(ctx).Create(&record).Error; err != nil {
		log.Printf("[client-error] persist failed route=%s version=%s message=%s: %v", route, version, message, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to record client error"})
		return
	}

	log.Printf("[client-error] route=%s version=%s message=%s", route, version, message)
	c.Status(http.StatusNoContent)
}

func redactClientError(s string) string {
	if s == "" {
		return s
	}
	s = clientErrorBearerRe.ReplaceAllString(s, "${1}[redacted]")
	s = clientErrorFieldRe.ReplaceAllString(s, "${1}[redacted]")
	s = clientErrorPhoneRe.ReplaceAllString(s, "[redacted-phone]")
	s = clientErrorIDRe.ReplaceAllString(s, "[redacted-id]")
	return s
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func clientErrorAllowed(ip, message string) bool {
	sum := sha256.Sum256([]byte(ip + "|" + message))
	key := hex.EncodeToString(sum[:])
	now := time.Now()

	clientErrorLimiter.mu.Lock()
	defer clientErrorLimiter.mu.Unlock()

	if last, ok := clientErrorLimiter.seen[key]; ok && now.Sub(last) < clientErrorWindow {
		return false
	}
	clientErrorLimiter.seen[key] = now

	// Opportunistic cleanup: keep the in-process map bounded.
	if len(clientErrorLimiter.seen) > 4096 {
		for k, ts := range clientErrorLimiter.seen {
			if now.Sub(ts) >= clientErrorWindow {
				delete(clientErrorLimiter.seen, k)
			}
		}
	}
	return true
}
