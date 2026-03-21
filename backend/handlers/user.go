package handlers

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

type User struct {
	ID          string `json:"id"`
	IAMSub      string `json:"iam_sub"`
	TenantID    string `json:"tenant_id"`
	OrgID       string `json:"org_id"`
	Name        string `json:"name"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	CreditScore int    `json:"credit_score"`
	DepositMode string `json:"deposit_mode"`
	IsShadow    bool   `json:"is_shadow"`
}

type SyncUserRequest struct {
	Sub      string `json:"sub" binding:"required"`
	TenantID string `json:"tenant_id" binding:"required"`
	OrgID    string `json:"org_id" binding:"required"`
	Phone    string `json:"phone"`
}

type SyncUserResponse struct {
	UserID    string `json:"user_id"`
	IsCreated bool   `json:"is_created"`
}

func SyncUser(c *gin.Context) {
	var req SyncUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "invalid parameters",
		})
		return
	}

	// Mock user lookup/creation
	mockUsers := make(map[string]User)
	userID := "user_" + req.Sub

	user, exists := mockUsers[req.Sub]
	isCreated := false

	if !exists {
		// Create new shadow user
		user = User{
			ID:          userID,
			IAMSub:      req.Sub,
			TenantID:    req.TenantID,
			OrgID:       req.OrgID,
			Phone:       req.Phone,
			CreditScore: 600,
			DepositMode: "standard",
			IsShadow:    true,
		}
		mockUsers[req.Sub] = user
		isCreated = true
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": SyncUserResponse{
			UserID:    user.ID,
			IsCreated: isCreated,
		},
	})
}
