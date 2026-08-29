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
	// #1778: dashboard 今日新增订单 — start_date/end_date 为 YYYY-MM-DD，
	// end_date 含当天全天（created_at < end_date + 1 day）。
	if startDate := c.Query("start_date"); startDate != "" {
		q = q.Where("orders.created_at >= ?", startDate)
	}
	if endDate := c.Query("end_date"); endDate != "" {
		q = q.Where("orders.created_at < ?", endDate+" 23:59:59.999999")
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

	log.Printf("[ListMerchantOrders] role=%s tenantID=%s page=%d total=%d orders_len=%d", role, tenantID, page, total, len(orders))

	// Build response list
	type OrderItem struct {
		ID               string  `json:"id"`
		Status           string  `json:"status"`
		InstrumentName   string  `json:"instrument_name"`
		InstrumentSN     string  `json:"instrument_sn"`
		SiteName         string  `json:"site_name"`
		UserID           string  `json:"user_id"`
		UserName         string  `json:"user_name"`
		UserVerifyStatus string  `json:"user_id_verify_status"` // #1791 T3: 买家核身聚合状态（仅状态，无敏感字段）
		StartDate        string  `json:"start_date"`
		EndDate          string  `json:"end_date"`
		DeliveredAt      string  `json:"delivered_at"`
		ShippedAt        string  `json:"shipped_at"`
		ReturnedAt       string  `json:"returned_at"`
		TotalAmount      float64 `json:"total_amount"`
		CreatedAt        string  `json:"created_at"`
	}
	list := make([]OrderItem, 0, len(orders))
	// #1776: reuse a single IAM client for all user-name lookups instead of
	// creating a new one (with a new client-credentials token) per order.
	iamClient := services.NewIAMClient()

	// #1791 T3 R2 H6: N+1 优化——一次 IN 查询所有买家 face 状态 + 一次批量查
	// 最新批次，构造 user_id → id_verify_status 映射（禁止逐行查批次表）。
	verifyStatusMap := map[string]string{}
	if len(orders) > 0 {
		userIDs := make([]string, 0, len(orders))
		for _, o := range orders {
			if o.UserID != "" {
				userIDs = append(userIDs, o.UserID)
			}
		}
		if len(userIDs) > 0 {
			// 批量加载买家用户（含 face 字段）。
			var buyers []models.User
			db.Select("id, face_verified, id_photo_front, id_photo_back, id_photo_other").
				Where("id IN ?", userIDs).Find(&buyers)
			// 批量加载最新批次（每个用户一行，submitted_at 最新）。
			rows := []struct {
				UserID string
				Status string
			}{}
			db.Raw(`SELECT DISTINCT ON (user_id) user_id, status FROM face_capture_batches
				WHERE user_id IN ?
				ORDER BY user_id, submitted_at DESC`, userIDs).Scan(&rows)
			latestStatus := map[string]string{}
			for _, r := range rows {
				latestStatus[r.UserID] = r.Status
			}
			for _, b := range buyers {
				verifyStatusMap[b.ID] = deriveIdVerifyStatusBulk(&b, latestStatus[b.ID])
			}
		}
	}
	for _, o := range orders {
		startStr, endStr := "", ""
		if o.StartDate != nil {
			startStr = *o.StartDate
		}
		if o.EndDate != nil {
			endStr = *o.EndDate
		}
		item := OrderItem{
			ID:               o.ID,
			Status:           o.Status,
			UserID:           o.UserID,
			UserVerifyStatus: verifyStatusMap[o.UserID],
			StartDate:        startStr,
			EndDate:          endStr,
			CreatedAt:        o.CreatedAt.Format("2006-01-02 15:04"),
		}
		// Resolve user name
		if o.UserID != "" {
			var name string
			if err := db.Raw("SELECT COALESCE(NULLIF(name,''), COALESCE(NULLIF(username,''), phone)) FROM users WHERE id = ? LIMIT 1", o.UserID).Scan(&name).Error; err == nil && name != "" {
				item.UserName = name
			}
			// Fallback: IAM lookup if local user has no name (use raw SQL to bypass tenant scoping)
			if item.UserName == "" {
				var iamSub string
				if err := db.Raw("SELECT iam_sub FROM users WHERE id = ? LIMIT 1", o.UserID).Scan(&iamSub).Error; err == nil && iamSub != "" {
					if iamUser, err2 := iamClient.GetUser(iamSub); err2 == nil && iamUser != nil {
						if iamUser.Name != "" {
							item.UserName = iamUser.Name
						}
						if item.UserName == "" {
							item.UserName = iamUser.Username
						}
						if item.UserName == "" {
							item.UserName = iamUser.Email
						}
						if item.UserName == "" {
							item.UserName = iamUser.Phone
						}
					}
				}
			}
		}
		// Timestamps
		if o.DeliveredAt != nil {
			item.DeliveredAt = o.DeliveredAt.Format("2006-01-02")
		}
		if o.ShippedAt != nil {
			item.ShippedAt = o.ShippedAt.Format("2006-01-02")
		}
		if o.ReturnedAt != nil {
			item.ReturnedAt = o.ReturnedAt.Format("2006-01-02")
		}
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
