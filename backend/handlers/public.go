package handlers

import (
	"net/http"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

// ListPublicMerchants returns full-control merchants and a has_controlled flag.
// No auth required — used by customer-facing pages (create repair, etc.).
func ListPublicMerchants(c *gin.Context) {
	db := database.GetDB()

	var merchants []models.Merchant
	db.Where("merchant_type = ? AND status = ?", models.MerchantTypeFull, "active").
		Select("id, name, address, phone").
		Find(&merchants)

	var controlledCount int64
	db.Model(&models.Merchant{}).Where("merchant_type = ? AND status = ?", models.MerchantTypeControlled, "active").
		Count(&controlledCount)

	type merchantItem struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Address string `json:"address"`
		Phone   string `json:"phone"`
	}
	list := make([]merchantItem, 0, len(merchants))
	for _, m := range merchants {
		list = append(list, merchantItem{
			ID:      m.ID,
			Name:    m.Name,
			Address: m.Address,
			Phone:   m.Phone,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"merchants":      list,
			"has_controlled": controlledCount > 0,
		},
	})
}
