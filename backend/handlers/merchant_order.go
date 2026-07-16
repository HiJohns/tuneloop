package handlers

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// ListMerchantOrders returns orders filtered by the current user's scope.
func ListMerchantOrders(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)

	role := middleware.GetBusinessRole(ctx)
	tenantID := middleware.GetTenantID(ctx)
	orgID := middleware.GetOrgID(ctx)

	q := db.Model(&models.Order{}).Order("created_at DESC")

	// Role-based scope
	if role == "site_admin" || role == "site_member" {
		q = q.Where("orders.org_id = ?", orgID)
	} else {
		q = q.Where("orders.tenant_id = ?", tenantID)
	}

	// Optional filters
	if status := c.Query("status"); status != "" {
		q = q.Where("orders.status = ?", status)
	}
	if sn := c.Query("sn"); sn != "" {
		q = q.Joins("JOIN instruments ON instruments.id = orders.instrument_id").
			Where("instruments.sn ILIKE ?", "%"+sn+"%")
	}
	if siteID := c.Query("site_id"); siteID != "" {
		q = q.Where("orders.site_id = ?", siteID)
	}

	// Pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int64
	q.Count(&total)

	var orders []models.Order
	q.Offset(offset).Limit(pageSize).Find(&orders)

	// Build response list
	type OrderItem struct {
		ID             string  `json:"id"`
		Status         string  `json:"status"`
		InstrumentName string  `json:"instrument_name"`
		InstrumentSN   string  `json:"instrument_sn"`
		SiteName       string  `json:"site_name"`
		UserName       string  `json:"user_name"`
		StartDate      string  `json:"start_date"`
		EndDate        string  `json:"end_date"`
		DeliveredAt    string  `json:"delivered_at"`
		ShippedAt      string  `json:"shipped_at"`
		ReturnedAt     string  `json:"returned_at"`
		TotalAmount    float64 `json:"total_amount"`
		CreatedAt      string  `json:"created_at"`
	}
	list := make([]OrderItem, 0, len(orders))
	for _, o := range orders {
		startStr, endStr := "", ""
		if o.StartDate != nil { startStr = *o.StartDate }
		if o.EndDate != nil { endStr = *o.EndDate }
		item := OrderItem{
			ID:        o.ID,
			Status:    o.Status,
			StartDate: startStr,
			EndDate:   endStr,
			CreatedAt: o.CreatedAt.Format("2006-01-02 15:04"),
		}
		// Resolve user name
		var user models.User
		if o.UserID != "" {
			if err := db.Where("id = ?", o.UserID).First(&user).Error; err == nil {
				item.UserName = user.Name
				if item.UserName == "" { item.UserName = user.Username }
				if item.UserName == "" { item.UserName = user.Phone }
				log.Printf("[MerchantOrders] user %s name=%q err=%v", o.UserID, item.UserName, err)
			} else {
				log.Printf("[MerchantOrders] user %s not found: %v", o.UserID, err)
			}
			// Fallback: IAM lookup if local user has no name
			if item.UserName == "" && user.IAMSub != "" {
				iamClient := services.NewIAMClient()
				if iamUser, err := iamClient.GetUser(user.IAMSub); err == nil && iamUser != nil {
					if iamUser.Name != "" { item.UserName = iamUser.Name }
					if item.UserName == "" { item.UserName = iamUser.Username }
					if item.UserName == "" { item.UserName = iamUser.Email }
					if item.UserName == "" { item.UserName = iamUser.Phone }
				}
			}
		}
		// Timestamps
		if o.DeliveredAt != nil { item.DeliveredAt = o.DeliveredAt.Format("2006-01-02") }
		if o.ShippedAt != nil { item.ShippedAt = o.ShippedAt.Format("2006-01-02") }
		if o.ReturnedAt != nil { item.ReturnedAt = o.ReturnedAt.Format("2006-01-02") }
		// Fetch instrument name/SN
		var inst models.Instrument
		if db.First(&inst, "id = ?", o.InstrumentID).Error == nil {
			item.InstrumentName = inst.CategoryName
			item.InstrumentSN = inst.SN
		}
		list = append(list, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":  list,
			"total": total,
		},
	})
}
