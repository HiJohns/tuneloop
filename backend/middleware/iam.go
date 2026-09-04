package middleware

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

type IAMClaims struct {
	jwt.RegisteredClaims
	Tid        string   `json:"tid"`
	Oid        string   `json:"oid"`
	Gid        string   `json:"gid"`
	Nid        string   `json:"nid"`
	Role       string   `json:"role"`
	Own        bool     `json:"own"`
	Name       string   `json:"name"`
	Roles      []string `json:"roles"`
	SysPerm    int64    `json:"sys_perm"`
	CusPerm    int64    `json:"cus_perm"`
	CusPermExt string   `json:"cus_perm_ext,omitempty"`
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
	ContextKeyGid             ContextKey = "gid"
	ContextKeySysPerm         ContextKey = "sys_perm"
	ContextKeyCusPerm         ContextKey = "cus_perm"
	ContextKeyCusPermExt      ContextKey = "cus_perm_ext"
	ContextKeyIAMClient       ContextKey = "iam_client"
	ContextKeyName            ContextKey = "name"
)

const (
	BusinessRoleSystemAdmin   = "system_admin"
	BusinessRoleMerchantAdmin = "merchant_admin"
	BusinessRoleSiteAdmin     = "site_admin"
	BusinessRoleSiteMember    = "site_member"
	BusinessRoleCustomer      = "customer"
	BusinessRolePlatformStaff = "platform_staff" // #1795 T6: 平台员工（PLATFORM_ROOT_ORG_ID 根组织成员）
)

// platformRootOrgID 缓存 PLATFORM_ROOT_ORG_ID 环境变量（#1795 T6）。
// 未配置时为空 → 平台员工分支不触发（默认行为不变）。
var platformRootOrgID = ""

func init() {
	platformRootOrgID = os.Getenv("PLATFORM_ROOT_ORG_ID")
}

// PlatformRootOrgID 返回 PLATFORM_ROOT_ORG_ID 配置值（#1795 T6，供 handlers 使用）。
func PlatformRootOrgID() string {
	return platformRootOrgID
}

// SetPlatformRootOrgIDForTesting 测试辅助：覆盖平台根组织 ID（#1795 T6）。
func SetPlatformRootOrgIDForTesting(v string) {
	platformRootOrgID = v
}

var validIssuers = []string{
	"beacon-iam",
	"http://opencode.linxdeep.com:5552",
	"http://localhost:5552",
	"https://iam.cadenzayueqi.com",
	"https://preiam.cadenzayueqi.com",
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

// enforceSubjectValidity (#1735): after signature validation passes, verify
// with beaconiam (authoritative source) that the subject still exists, is
// active, and that the token was not issued before a token_version bump.
// Responses use dedicated semantic codes so frontends can clear stale tokens:
//   - 40105 account_not_found — user deleted in IAM
//   - 40106 account_inactive  — user deactivated in IAM
//   - 40107 token_revoked     — token issued before a token_version bump
//
// Transient IAM failures fail OPEN (log + allow) to avoid mass logouts on
// beaconiam hiccups; the next request retries the check.
// Returns true when the request may proceed.
func enforceSubjectValidity(c *gin.Context, iamClient *services.IAMClient, claims *services.JWTClaims) bool {
	// GUEST tokens are issued locally for anonymous visitors — there is no
	// backing IAM user to validate.
	if iamClient == nil || claims == nil || claims.UserID == "" || claims.Role == "GUEST" {
		return true
	}

	tokenVersion, status, err := iamClient.GetUserAuthState(claims.UserID)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			log.Printf("[IAM] Subject rejected: user %s not found in IAM (account deleted)", claims.UserID)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    40105,
				"message": "account_not_found",
			})
			return false
		}
		log.Printf("[IAM WARNING] Subject check unavailable for user %s, failing open: %v", claims.UserID, err)
		return true
	}

	if status != "" && status != "active" {
		log.Printf("[IAM] Subject rejected: user %s has status %q", claims.UserID, status)
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"code":    40106,
			"message": "account_inactive",
		})
		return false
	}

	// Compare at SECOND granularity: JWT iat (NumericDate) has second
	// precision while token_version is stored in milliseconds. Millisecond
	// comparison would falsely reject tokens legitimately issued in the same
	// second as a bump (truncated iat looks older). Tokens issued within the
	// bump second stay valid — the exposure window is <1s.
	iatSec := int64(0)
	if claims.IssuedAt != nil {
		iatSec = claims.IssuedAt.Time.Unix()
	}
	if tokenVersion > 0 && iatSec < tokenVersion/1000 {
		log.Printf("[IAM] Subject rejected: token for user %s predates token_version bump (iat=%d s < tv=%d ms)", claims.UserID, iatSec, tokenVersion)
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"code":    40107,
			"message": "token_revoked",
		})
		return false
	}

	return true
}

func IAMInterceptor(iamService *services.IAMService, iamClient *services.IAMClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		if isPublicRoute(path) {
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")

		if authHeader == "" {
			if token, err := c.Cookie("token"); err == nil && token != "" {
				authHeader = "Bearer " + token
			}
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    40100,
				"message": "missing or invalid authorization header",
			})
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		claims, err := iamService.ValidateToken(tokenString)
		if err != nil {
			log.Printf("[IAM] Token validation failed: %v", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    40101,
				"message": "invalid token: " + err.Error(),
			})
			return
		}

		log.Printf("[IAM DEBUG] Token validated, claims: sub=%s, tid=%s, oid=%s, role=%s, iss=%s", claims.UserID, claims.TenantID, claims.OrgID, claims.Role, claims.Issuer)

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

		// #1735: subject validity check against beaconiam (deleted/inactive
		// accounts and pre-bump tokens get explicit 40105/40106/40107).
		if !enforceSubjectValidity(c, iamClient, claims) {
			return
		}

		// Use tid (tenant_id) from token; fall back to oid (org_id) if tid is empty
		tenantID := claims.TenantID
		if tenantID == "" {
			tenantID = claims.OrgID
		}
		orgID := claims.OrgID
		if orgID == "" {
			orgID = tenantID
		}

		if tenantID == "" {
			log.Printf("[IAM] Token rejected: user %s has no organization binding (tid=%q, oid=%q, iss=%q)",
				claims.UserID, claims.TenantID, claims.OrgID, claims.Issuer)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    40104,
				"message": "no organization binding in token, please contact your system administrator",
			})
			return
		}

		// Resolve top-level tenant: if this org has a parent, trace up to root
		if iamClient != nil {
			org, err := iamClient.GetOrganization(tenantID)
			if err == nil && org.ParentID != nil {
				currentID := *org.ParentID
				for {
					parent, err := iamClient.GetOrganization(currentID)
					if err != nil || parent.ParentID == nil {
						if err == nil {
							tenantID = currentID
						}
						break
					}
					currentID = *parent.ParentID
				}
			}
		}

		ctx := database.SetTenantID(c.Request.Context(), tenantID)
		ctx = context.WithValue(ctx, ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, ContextKeyOrgID, orgID)
		ctx = database.SetOrgID(ctx, orgID)
		ctx = context.WithValue(ctx, ContextKeyNamespaceID, claims.NamespaceID)
		ctx = context.WithValue(ctx, ContextKeyUserID, claims.UserID)
		ctx = context.WithValue(ctx, ContextKeyRole, claims.Role)
		ctx = context.WithValue(ctx, ContextKeyIsOwner, claims.IsOwner)
		ctx = context.WithValue(ctx, ContextKeyFunctionalRoles, claims.Roles)
		ctx = context.WithValue(ctx, ContextKeyGid, claims.Gid)
		ctx = context.WithValue(ctx, ContextKeySysPerm, claims.SysPerm)
		ctx = context.WithValue(ctx, ContextKeyCusPerm, claims.CusPerm)
		ctx = context.WithValue(ctx, ContextKeyCusPermExt, claims.CusPermExt)
		ctx = context.WithValue(ctx, ContextKeyIAMClient, iamClient)
		ctx = context.WithValue(ctx, ContextKeyName, claims.Name)
		c.Request = c.Request.WithContext(ctx)

		// Sliding expiration: Check if token is about to expire
		if claims.ExpiresAt != nil {
			timeUntilExpiry := time.Until(claims.ExpiresAt.Time)
			// If token expires in less than 10 minutes, set header to indicate soon expiration
			if timeUntilExpiry < 10*time.Minute {
				log.Printf("[IAM] Token for user %s expires in %v", claims.UserID, timeUntilExpiry)
				c.Header("X-Token-Expires-Soon", "true")
				c.Header("X-Token-Expires-At", claims.ExpiresAt.Time.Format(time.RFC3339))
			}
		}

		c.Next()
	}
}

func OptionalIAMInterceptor(iamService *services.IAMService, iamClient *services.IAMClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")

		if authHeader == "" {
			if token, err := c.Cookie("token"); err == nil && token != "" {
				authHeader = "Bearer " + token
			}
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			// Anonymous pass-through (#1681): two-phase registration prepay
			// runs without any token (no account exists yet). No claims are
			// injected — handlers read empty userID and must tolerate it.
			// Invalid/expired tokens still get rejected below.
			c.Next()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		claims, err := iamService.ValidateToken(tokenString)
		if err != nil {
			log.Printf("[IAM] Token validation failed: %v", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    40101,
				"message": "invalid token: " + err.Error(),
			})
			return
		}

		log.Printf("[IAM DEBUG] OptionalIAMInterceptor: sub=%s, tid=%s, oid=%s, role=%s, iss=%s", claims.UserID, claims.TenantID, claims.OrgID, claims.Role, claims.Issuer)

		issuerValid := len(validIssuers) == 0
		for _, issuer := range validIssuers {
			if claims.Issuer == issuer {
				issuerValid = true
				break
			}
		}
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

		// #1735: token valid but subject dead (deleted/deactivated/pre-bump)
		// must surface as an explicit 401 — NOT anonymous pass-through — so
		// the frontend learns it must clear the stale token and re-login.
		if !enforceSubjectValidity(c, iamClient, claims) {
			return
		}

		tenantID := claims.TenantID
		if tenantID == "" {
			tenantID = claims.OrgID
		}
		orgID := claims.OrgID
		if orgID == "" {
			orgID = tenantID
		}

		ctx := database.SetTenantID(c.Request.Context(), tenantID)
		ctx = context.WithValue(ctx, ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, ContextKeyOrgID, orgID)
		ctx = database.SetOrgID(ctx, orgID)
		ctx = context.WithValue(ctx, ContextKeyNamespaceID, claims.NamespaceID)
		ctx = context.WithValue(ctx, ContextKeyUserID, claims.UserID)
		ctx = context.WithValue(ctx, ContextKeyRole, claims.Role)
		ctx = context.WithValue(ctx, ContextKeyIsOwner, claims.IsOwner)
		ctx = context.WithValue(ctx, ContextKeyFunctionalRoles, claims.Roles)
		ctx = context.WithValue(ctx, ContextKeyGid, claims.Gid)
		ctx = context.WithValue(ctx, ContextKeySysPerm, claims.SysPerm)
		ctx = context.WithValue(ctx, ContextKeyCusPerm, claims.CusPerm)
		ctx = context.WithValue(ctx, ContextKeyCusPermExt, claims.CusPermExt)
		ctx = context.WithValue(ctx, ContextKeyIAMClient, iamClient)
		ctx = context.WithValue(ctx, ContextKeyName, claims.Name)
		c.Request = c.Request.WithContext(ctx)

		if claims.ExpiresAt != nil {
			timeUntilExpiry := time.Until(claims.ExpiresAt.Time)
			if timeUntilExpiry < 10*time.Minute {
				log.Printf("[IAM] Token for user %s expires in %v", claims.UserID, timeUntilExpiry)
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
		if userRole != "OWNER" && userRole != "ADMIN" && userRole != "SITE_MANAGER" && userRole != "STAFF" {
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

func GetGid(ctx context.Context) string {
	if gid, ok := ctx.Value(ContextKeyGid).(string); ok {
		return gid
	}
	return ""
}

func GetSysPerm(ctx context.Context) int64 {
	if perm, ok := ctx.Value(ContextKeySysPerm).(int64); ok {
		return perm
	}
	return 0
}

func GetCusPerm(ctx context.Context) int64 {
	if perm, ok := ctx.Value(ContextKeyCusPerm).(int64); ok {
		return perm
	}
	return 0
}

func GetCusPermExt(ctx context.Context) string {
	if ext, ok := ctx.Value(ContextKeyCusPermExt).(string); ok {
		return ext
	}
	return ""
}

func GetName(ctx context.Context) string {
	if name, ok := ctx.Value(ContextKeyName).(string); ok {
		return name
	}
	return ""
}

// EnsureLocalUser ensures a local users record exists for the JWT-authenticated user.
// Returns the local user ID. Creates a shadow user record if none exists.
func EnsureLocalUser(ctx context.Context, db *gorm.DB) (string, error) {
	iamSub := GetUserID(ctx)
	if iamSub == "" {
		return "", fmt.Errorf("no user ID in context")
	}
	var user models.User
	if err := db.Where("iam_sub = ?", iamSub).First(&user).Error; err == nil {
		// Disabled users are blocked from all authenticated access (#1545).
		if user.Status == "disabled" {
			return "", fmt.Errorf("user is disabled")
		}
		return user.ID, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("failed to query user: %w", err)
	}

	tenantID := GetTenantID(ctx)
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000000"
	}
	orgID := GetOrgID(ctx)
	if orgID == "" {
		orgID = "00000000-0000-0000-0000-000000000000"
	}
	user = models.User{
		IAMSub:   iamSub,
		TenantID: tenantID,
		OrgID:    orgID,
		Name:     GetName(ctx),
		Status:   "active",
		IsShadow: true,
	}
	if err := db.Create(&user).Error; err != nil {
		if err2 := db.Where("iam_sub = ?", iamSub).First(&user).Error; err2 == nil {
			return user.ID, nil
		}
		return "", fmt.Errorf("failed to create user: %w", err)
	}
	return user.ID, nil
}

// LocalUserID resolves the local users.id for the JWT-authenticated user by
// reverse lookup on iam_sub (#1742). Read-only: unlike EnsureLocalUser it
// never creates a shadow record — notification/query read paths must not
// write. Returns ("", nil) when no local user exists.
func LocalUserID(ctx context.Context, db *gorm.DB) (string, error) {
	iamSub := GetUserID(ctx)
	if iamSub == "" {
		// Anonymous on an optional-auth route: no local user by definition —
		// return empty (not an error) so read paths (notifications/
		// unread-count/appeals) answer 200 with empty results instead of 500
		// (observed: registration-flow & expired-token clients poll
		// unread-count anonymously).
		return "", nil
	}
	var user models.User
	if err := db.Where("iam_sub = ?", iamSub).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", fmt.Errorf("failed to query user: %w", err)
	}
	return user.ID, nil
}

func GetBusinessRole(ctx context.Context) string {
	role := GetRole(ctx)
	tid := GetTenantID(ctx)
	oid := GetOrgID(ctx)

	// Empty role or USER role should return customer, not staff member
	if role == "" || role == "USER" {
		return BusinessRoleCustomer
	}

	// Namespace admin or platform-level user (no tenant)
	funRoles := GetFunctionalRoles(ctx)
	if role == "NAMESPACE_ADMIN" || tid == "" || contains(funRoles, "namespace_admin") {
		return BusinessRoleSystemAdmin
	}

	// #1795 T6: 平台员工——oid 精确匹配 PLATFORM_ROOT_ORG_ID（env 配置）。
	// 禁止用 tid==oid 推断（商户根组织形态相同会被误判，R2 约束）；
	// 未配置 PLATFORM_ROOT_ORG_ID 时该分支不触发。
	if platformRootOrgID != "" && oid == platformRootOrgID {
		return BusinessRolePlatformStaff
	}

	// Merchant admin: logged into the merchant root org
	if tid == oid {
		return BusinessRoleMerchantAdmin
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
	case BusinessRolePlatformStaff: // #1795 T6 R2 C2: 平台员工全用户可见（无 org 过滤）
		return nil, nil
	case BusinessRoleSiteMember:
		return []string{orgID}, nil
	default:
		return getOrgDescendants(ctx, orgID)
	}
}

func getOrgDescendants(ctx context.Context, orgID string) ([]string, error) {
	iamClient, _ := ctx.Value(ContextKeyIAMClient).(*services.IAMClient)
	if iamClient == nil {
		return []string{orgID}, nil
	}

	orgs, err := iamClient.ListOrganizations()
	if err != nil {
		return []string{orgID}, nil
	}

	// Build parent→children map
	children := map[string][]string{}
	for _, org := range orgs {
		if org.ParentID != nil {
			children[*org.ParentID] = append(children[*org.ParentID], org.ID)
		}
	}

	// Recursively collect descendants
	result := []string{orgID}
	queue := []string{orgID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, childID := range children[current] {
			result = append(result, childID)
			queue = append(queue, childID)
		}
	}

	return result, nil
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

func contains(items []string, target string) bool {
	for _, s := range items {
		if s == target {
			return true
		}
	}
	return false
}

// RequirePasswordNotForceChange checks if the user needs to change password before accessing any route.
// Uses local users.force_password_change flag (方案 A).
// Exempts /api/user/change-password itself.
func RequirePasswordNotForceChange() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/api/user/reset-password" || c.Request.URL.Path == "/api/user/change-password" {
			c.Next()
			return
		}
		userID := GetUserID(c.Request.Context())
		if userID == "" {
			c.Next()
			return
		}
		db := database.GetDB().WithContext(c.Request.Context())
		var count int64
		if err := db.Table("users").Where("(iam_sub = ? OR id = ?) AND force_password_change = ? AND deleted_at IS NULL", userID, userID, true).Count(&count).Error; err == nil && count > 0 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    40302,
				"message": "请先修改密码后再使用系统功能",
			})
			return
		}
		c.Next()
	}
}

// IsDebugMode returns true if DEBUG_MODE env is enabled.
func IsDebugMode() bool {
	return os.Getenv("DEBUG_MODE") == "true"
}

// RequireDebugMode checks that DEBUG_MODE is enabled before processing.
func RequireDebugMode() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !IsDebugMode() {
			c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "debug mode not enabled"})
			c.Abort()
			return
		}
		c.Next()
	}
}
