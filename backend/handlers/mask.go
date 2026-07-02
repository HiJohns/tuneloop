package handlers

import (
	"github.com/gin-gonic/gin"
	"tuneloop-backend/middleware"
)

// MaskOrderForRole filters order fields based on viewer role.
func MaskOrderForRole(c *gin.Context, order *map[string]interface{}) {
	role := middleware.GetRole(c.Request.Context())
	if role == "USER" {
		delete(*order, "merchant_name")
		delete(*order, "merchant_phone")
	} else {
		delete(*order, "user_name")
		delete(*order, "user_phone")
		delete(*order, "delivery_address")
	}
}

// MaskRepairRequestForRole filters repair request fields based on viewer role.
func MaskRepairRequestForRole(c *gin.Context, req *map[string]interface{}) {
	role := middleware.GetRole(c.Request.Context())
	if role == "USER" {
		delete(*req, "worker_phone")
	} else {
		delete(*req, "user_phone")
		delete(*req, "user_name")
	}
}
