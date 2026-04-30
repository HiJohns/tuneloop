package middleware

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

type IAMClaims struct {
	jwt.RegisteredClaims
	Tid   string   `json:"tid"`
	Oid   string   `json:"oid"`
	Nid   string   `json:"nid"`
	Role  string   `json:"role"`
	Own   bool     `json:"own"`
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
}

type ContextKey string

const (
	ContextKeyTenantID        ContextKey = "tenant_id"
	ContextKeyOrgID           ContextKey = "org_id"
	ContextKeyNamespaceID     ContextKey = "namespace_id"
	ContextKeyUserID          ContextKey = "user_id"
	ContextKeyRole            ContextKey = "role"
	ContextKeyIsOwner         ContextKey = "is_owner"
	ContextKeyFunctionalRoles ContextKey = "functional_roles"
)

const (
	BusinessRoleSystemAdmin   = "system_admin"
	BusinessRoleMerchantAdmin = "merchant_admin"
	BusinessRoleSiteAdmin     = "site_admin"
	BusinessRoleSiteMember    = "site_member"
)

var validIssuers = []string{
	"beacon-iam",
	"http://opencode.linxdeep.com:5552",
	"http://localhost:5552",
	"https://iam.cadenzayueqi.com",
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
		path := c.Request.URL.Path
		log.Printf("[IAM DEBUG] Request: %s %s", c.Request.Method, path)

		if isPublicRoute(path) {
			log.Printf("[IAM DEBUG] Public route, skipping auth")
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")
		log.Printf("[IAM DEBUG] Authorization header: %s", authHeader)

		// 如果 Authorization header 为空，尝试从 Cookie 读取
		if authHeader == "" {
			if token, err := c.Cookie("token"); err == nil && token != "" {
				authHeader = "Bearer " + token
				log.Printf("[IAM DEBUG] Using token from Cookie, length: %d", len(token))
			} else {
				log.Printf("[IAM DEBUG] No token in cookie, err: %v", err)
			}
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			log.Printf("[IAM DEBUG] Missing or invalid Authorization header, returning 401")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    40100,
				"message": "missing or invalid authorization header",
			})
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		log.Printf("[IAM DEBUG] Token string length: %d, first 20 chars: %s...", len(tokenString), tokenString[:min(20, len(tokenString))])

		claims, err := iamService.ValidateToken(tokenString)
		if err != nil {
			log.Printf("[IAM DEBUG] Token validation failed: %v", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    40101,
				"message": "invalid token: " + err.Error(),
			})
			return
		}

		log.Printf("[IAM DEBUG] Token validated, claims: sub=%s, tid=%s, oid=%s, role=%s, iss=%s", claims.Subject, claims.TenantID, claims.OrgID, claims.Role, claims.Issuer)

		issuerValid := false
		for _, issuer := range validIssuers {
			if claims.Issuer == issuer {
				issuerValid = true
				break
			}
		}
		// Skip issuer validation if issuer is empty (for IAM compatibility)
		if claims.Issuer == "" {
			issuerValid = true
		}
		if !issuerValid {
			log.Printf("[IAM DEBUG] Invalid issuer: %s, valid issuers: %v", claims.Issuer, validIssuers)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    40102,
				"message": "invalid token issuer",
			})
			return
		}

		// Use tid (tenant_id) from token; fall back to oid (org_id) if tid is empty
		tenantID := claims.TenantID
		if tenantID == "" {
			tenantID = claims.OrgID
		}
		ctx := database.SetTenantID(c.Request.Context(), tenantID)
		ctx = context.WithValue(ctx, ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, ContextKeyOrgID, tenantID)
		ctx = context.WithValue(ctx, ContextKeyNamespaceID, claims.NamespaceID)
		ctx = context.WithValue(ctx, ContextKeyUserID, claims.Subject)
		ctx = context.WithValue(ctx, ContextKeyRole, claims.Role)
		ctx = context.WithValue(ctx, ContextKeyIsOwner, claims.IsOwner)
		ctx = context.WithValue(ctx, ContextKeyFunctionalRoles, claims.Roles)
		c.Request = c.Request.WithContext(ctx)

		// Sliding expiration: Check if token is about to expire
		if claims.ExpiresAt != nil {
			timeUntilExpiry := time.Until(claims.ExpiresAt.Time)
			// If token expires in less than 10 minutes, set header to indicate soon expiration
			if timeUntilExpiry < 10*time.Minute {
				log.Printf("[IAM] Token for user %s expires in %v", claims.Subject, timeUntilExpiry)
				c.Header("X-Token-Expires-Soon", "true")
				c.Header("X-Token-Expires-At", claims.ExpiresAt.Time.Format(time.RFC3339))
			}
		}

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

func RequireSiteManager() gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole := GetRole(c.Request.Context())
		if userRole != "OWNER" && userRole != "ADMIN" && userRole != "SITE_MANAGER" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    40302,
				"message": "site manager privileges required",
			})
			return
		}
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

func GetNamespaceID(ctx context.Context) string {
	if nid, ok := ctx.Value(ContextKeyNamespaceID).(string); ok {
		return nid
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

func GetFunctionalRoles(ctx context.Context) []string {
	if roles, ok := ctx.Value(ContextKeyFunctionalRoles).([]string); ok {
		return roles
	}
	return []string{}
}

func GetBusinessRole(ctx context.Context) string {
	role := GetRole(ctx)
	orgID := GetOrgID(ctx)
	isOwner := IsOwner(ctx)

	if role == "" && orgID == "" {
		return BusinessRoleSiteMember
	}

	if isOwner {
		db := database.GetDB().WithContext(ctx)
		var org struct {
			ParentID *string `gorm:"column:parent_id"`
		}
		err := db.Table("merchants").Where("id = ?", orgID).First(&org).Error
		if err != nil || org.ParentID == nil {
			return BusinessRoleMerchantAdmin
		}
		return BusinessRoleSiteAdmin
	}

	if role == "ADMIN" {
		return BusinessRoleSiteAdmin
	}

	if role == "STAFF" || role == "WORKER" {
		return BusinessRoleSiteMember
	}

	return BusinessRoleSiteMember
}

func GetVisibleOrgIDs(ctx context.Context) ([]string, error) {
	businessRole := GetBusinessRole(ctx)
	orgID := GetOrgID(ctx)

	if orgID == "" {
		return nil, nil
	}

	switch businessRole {
	case BusinessRoleSystemAdmin:
		return nil, nil
	case BusinessRoleSiteMember:
		return []string{orgID}, nil
	default:
		return getOrgDescendants(ctx, orgID)
	}
}

func getOrgDescendants(ctx context.Context, orgID string) ([]string, error) {
	db := database.GetDB().WithContext(ctx)

	type orgResult struct {
		ID string
	}

	var results []orgResult

	err := db.Table("merchants").
		Select("id").
		Where("parent_id = ?", orgID).
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	orgIDs := []string{orgID}
	for _, r := range results {
		childIDs, err := getOrgDescendants(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		orgIDs = append(orgIDs, childIDs...)
	}

	return orgIDs, nil
}

func ApplyOrgScope(db *gorm.DB, ctx context.Context) (*gorm.DB, error) {
	orgIDs, err := GetVisibleOrgIDs(ctx)
	if err != nil {
		return nil, err
	}
	if orgIDs == nil {
		return db, nil
	}
	return db.Where("org_id IN ?", orgIDs), nil
}
