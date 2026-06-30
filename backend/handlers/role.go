package handlers

import (
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

// GetMyRoles returns the current user's site_members roles.
func GetMyRoles(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	userID := middleware.GetUserID(ctx)

	var localUser models.User
	if err := db.Where("iam_sub = ?", userID).First(&localUser).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"roles": []string{}}})
		return
	}

	var roles []string
	db.Table("site_members").Where("user_id = ?", localUser.ID).Pluck("role", &roles)

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"roles": uniqueStrings(roles)}})
}

func uniqueStrings(s []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}
