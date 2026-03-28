package middleware

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"tuneloop-backend/database"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type IAMClaims struct {
	jwt.RegisteredClaims
	Tid  string `json:"tid"`
	Oid  string `json:"oid"`
	Role string `json:"role"`
	Own  bool   `json:"own"`
	Name string `json:"name"`
}

type ContextKey string

const (
	ContextKeyTenantID ContextKey = "tenant_id"
	ContextKeyOrgID    ContextKey = "org_id"
	ContextKeyUserID   ContextKey = "user_id"
	ContextKeyRole     ContextKey = "role"
	ContextKeyIsOwner  ContextKey = "is_owner"
)

var validIssuers = []string{
	"beacon-iam",
	"http://opencode.linxdeep.com:5552",
	"http://localhost:5552",
}

var publicRoutes = []string{
	"/health",
	"/api/health",
	"/api/auth/callback",
	"/api/auth/refresh",
	"/api/auth/login",
}

func isPublicRoute(path string) bool {
	for _, route := range publicRoutes {
		if strings.HasPrefix(path, route) {
			return true
		}
	}
	return false
}

func IAMInterceptor(iamService *services.IAMService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isPublicRoute(c.Request.URL.Path) {
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")

		// 如果 Authorization header 为空，尝试从 Cookie 读取
		if authHeader == "" {
			if token, err := c.Cookie("token"); err == nil && token != "" {
				authHeader = "Bearer " + token
				fmt.Println("[Auth] Using token from Cookie")
			}
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			// 🔍 调试：记录收到的原始 header
			log.Printf("40100 Debug: Received Header [%s], Cookie [%s]", authHeader, c.GetHeader("Cookie"))

			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    40100,
				"message": "missing or invalid authorization header",
			})
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		claims, err := iamService.ValidateToken(tokenString)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    40101,
				"message": "invalid token: " + err.Error(),
			})
			return
		}

		issuerValid := false
		for _, issuer := range validIssuers {
			if claims.Issuer == issuer {
				issuerValid = true
				break
			}
		}
		if !issuerValid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    40102,
				"message": "invalid token issuer",
			})
			return
		}

		ctx := database.SetTenantID(c.Request.Context(), claims.TenantID)
		ctx = context.WithValue(ctx, ContextKeyTenantID, claims.TenantID)
		ctx = context.WithValue(ctx, ContextKeyOrgID, claims.TenantID)
		ctx = context.WithValue(ctx, ContextKeyUserID, claims.Subject)
		ctx = context.WithValue(ctx, ContextKeyRole, claims.Role)
		ctx = context.WithValue(ctx, ContextKeyIsOwner, claims.IsOwner)
		c.Request = c.Request.WithContext(ctx)
		log.Printf("[IAM DEBUG] User=%s, Role=%s, IsOwner=%v", claims.Subject, claims.Role, claims.IsOwner)

		c.Next()
	}
}

func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if isPublicRoute(c.Request.URL.Path) {
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    40100,
				"message": "authentication required",
			})
			return
		}
		c.Next()
	}
}

func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole := GetRole(c.Request.Context())
		for _, role := range roles {
			if userRole == role {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"code":    40300,
			"message": "insufficient permissions",
		})
	}
}

func RequireOwner() gin.HandlerFunc {
	return func(c *gin.Context) {
		fmt.Printf("[RBAC] RequireOwner called - Path: %s\n", c.Request.URL.Path)
		userRole := GetRole(c.Request.Context())
		log.Printf("[RBAC DEBUG] RequireOwner called - Path: %s, Role: '%s'", c.Request.URL.Path, userRole)
		if userRole != "OWNER" && userRole != "ADMIN" {
			log.Printf("[RBAC] Denied - UserRole: '%s', Path: %s", userRole, c.Request.URL.Path)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    40301,
				"message": "owner privileges required",
			})
			return
		}
		log.Printf("[RBAC] Allowed - UserRole: '%s', Path: %s", userRole, c.Request.URL.Path)
		c.Next()
	}
}

func GetTenantID(ctx context.Context) string {
	if tid, ok := ctx.Value(ContextKeyTenantID).(string); ok {
		return tid
	}
	return ""
}

func GetOrgID(ctx context.Context) string {
	if oid, ok := ctx.Value(ContextKeyOrgID).(string); ok {
		return oid
	}
	return ""
}

func GetUserID(ctx context.Context) string {
	if uid, ok := ctx.Value(ContextKeyUserID).(string); ok {
		return uid
	}
	return ""
}

func GetRole(ctx context.Context) string {
	if role, ok := ctx.Value(ContextKeyRole).(string); ok {
		return role
	}
	return ""
}

func IsOwner(ctx context.Context) bool {
	if own, ok := ctx.Value(ContextKeyIsOwner).(bool); ok {
		return own
	}
	return false
}
