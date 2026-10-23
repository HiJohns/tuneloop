package middleware

// #2261 B3：按路由组显式关闭 GORM 自动租户范围。
// 仅用于已完成「显式过滤核查」的路由组；未挂载的组保持自动注入。

import (
	"github.com/gin-gonic/gin"
	"tuneloop-backend/database"
)

func ExplicitTenantScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := database.ExplicitTenantScopeOnly(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
