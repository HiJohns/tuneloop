package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

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
)

var validIssuers = []string{
	"beacon-iam",
	"http://opencode.linxdeep.com:5552",
	"http://localhost:5552",
}

func IAMInterceptor(publicKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    40100,
				"message": "missing or invalid authorization header",
			})
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		token, err := jwt.ParseWithClaims(tokenString, &IAMClaims{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(publicKey), nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    40101,
				"message": "invalid token: " + err.Error(),
			})
			return
		}

		claims, ok := token.Claims.(*IAMClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    40101,
				"message": "invalid token claims",
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
				"message": "invalid token issuer: " + claims.Issuer,
			})
			return
		}

		ctx := context.WithValue(c.Request.Context(), ContextKeyTenantID, claims.Tid)
		ctx = context.WithValue(ctx, ContextKeyOrgID, claims.Oid)
		ctx = context.WithValue(ctx, ContextKeyUserID, claims.Subject)
		ctx = context.WithValue(ctx, ContextKeyRole, claims.Role)
		c.Request = c.Request.WithContext(ctx)

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
