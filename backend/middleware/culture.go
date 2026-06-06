package middleware

import (
	"github.com/gin-gonic/gin"
)

func CultureMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		culture := c.GetHeader("Accept-Language")
		if culture == "" || (culture != "zh" && culture != "en") {
			culture = "zh"
		}
		c.Set("culture", culture)
		c.Next()
	}
}

func GetCulture(c *gin.Context) string {
	if v, ok := c.Get("culture"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return "zh"
}
