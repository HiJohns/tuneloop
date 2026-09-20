package handlers

import (
	"net/http"
	"strconv"
	"time"

	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type DashboardHandler struct {
	db *gorm.DB
}

func NewDashboardHandler(db *gorm.DB) *DashboardHandler {
	return &DashboardHandler{db: db}
}

// GetDashboardStats 返回按角色分层的仪表盘统计（#2005 S1）。
//
// 分层：system_admin/platform_staff → 平台治理指标；merchant_admin → 本商户经营指标
// (tenant_id)；site_admin/site_member → 本网点经营指标 (org_id)。
// 口径：资产=instruments.stock_status；租约=lease_sessions（真实租约源，Lease 表为空属遗留）；
// 订单=orders（今日新订单；收入趋势=completed 的 cash_paid）。金额遵循 #1757 分契约输出元。
func (h *DashboardHandler) GetDashboardStats(c *gin.Context) {
	ctx := c.Request.Context()
	role := middleware.GetBusinessRole(ctx)
	tenantID := middleware.GetTenantID(ctx)
	orgID := middleware.GetOrgID(ctx)
	db := h.db.WithContext(ctx)

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	todayStr := now.Format("2006-01-02")

	// #2006 QA 返工：聚合查询失败必须返回 500，不得静默返回 0（HTTP 200）。
	// aggErr 记录首个聚合错误；countQ 在出错后不再执行后续查询。
	var aggErr error
	countQ := func(q *gorm.DB) int64 {
		var n int64
		if aggErr == nil {
			if err := q.Count(&n).Error; err != nil {
				aggErr = err
			}
		}
		return n
	}
	failAgg := func() {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to aggregate dashboard stats: " + aggErr.Error(),
		})
	}

	switch role {
	case middleware.BusinessRoleSystemAdmin, middleware.BusinessRolePlatformStaff:
		// ---- 治理支（平台级，不做 tenant/org 过滤）----
		merchantsCount := countQ(db.Model(&models.Merchant{}))
		tenantsCount := countQ(db.Model(&models.Tenant{}))
		usersCount := countQ(db.Model(&models.User{}))
		pendingFaceReview := countQ(db.Model(&models.FaceCaptureBatch{}).Where("status = ?", "pending"))
		pendingAppeals := countQ(db.Model(&models.Appeal{}).Where("status = ?", "pending"))

		var merchants []models.Merchant
		if aggErr == nil {
			if err := db.Order("created_at DESC").Limit(10).Find(&merchants).Error; err != nil {
				aggErr = err
			}
		}
		if aggErr != nil {
			failAgg()
			return
		}
		recentMerchants := make([]gin.H, 0, len(merchants))
		for _, m := range merchants {
			recentMerchants = append(recentMerchants, gin.H{
				"id":         m.ID,
				"name":       m.Name,
				"tenant_id":  m.TenantID,
				"created_at": m.CreatedAt,
			})
		}

		c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
			"role":                role,
			"merchants_count":     merchantsCount,
			"tenants_count":       tenantsCount,
			"users_count":         usersCount,
			"pending_face_review": pendingFaceReview,
			"pending_appeals":     pendingAppeals,
			"recent_merchants":    recentMerchants,
		}})
		return

	case middleware.BusinessRoleMerchantAdmin, middleware.BusinessRoleSiteAdmin, middleware.BusinessRoleSiteMember:
		// ---- 经营支 ----
		var scope func(*gorm.DB) *gorm.DB
		if role == middleware.BusinessRoleMerchantAdmin {
			if tenantID == "" {
				c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
				return
			}
			scope = func(q *gorm.DB) *gorm.DB { return q.Where("tenant_id = ?", tenantID) }
		} else {
			if orgID == "" {
				c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
				return
			}
			scope = func(q *gorm.DB) *gorm.DB { return q.Where("org_id = ?", orgID) }
		}

		countInstruments := func(stockStatus string) int64 {
			q := scope(db.Model(&models.Instrument{}))
			if stockStatus != "" {
				q = q.Where("stock_status = ?", stockStatus)
			}
			return countQ(q)
		}
		totalAssets := countInstruments("")
		rentedAssets := countInstruments("rented")
		availableAssets := countInstruments("available")
		maintenanceAssets := countInstruments("maintenance")

		// 租约：以 orders 为准（#2005 口径统一=orders，与「逾期告警」#1966 一致）
		//   生效租约 = status 'in_lease'；今日到期 = 'in_lease' 且 end_date=今天；逾期 = 'expired'
		countOrdersByStatus := func(status string, extra string, args ...interface{}) int64 {
			q := scope(db.Model(&models.Order{})).Where("status = ?", status)
			if extra != "" {
				q = q.Where(extra, args...)
			}
			return countQ(q)
		}
		activeLeases := countOrdersByStatus(models.OrderStatusInLease, "")
		expiringToday := countOrdersByStatus(models.OrderStatusInLease, "end_date = ?", todayStr)
		overdue := countOrdersByStatus(models.OrderStatusExpired, "")

		newOrdersToday := countQ(scope(db.Model(&models.Order{})).Where("created_at >= ?", todayStart))

		// 收入趋势（近 6 个月；completed 订单 cash_paid 汇总，分→元）
		type revRow struct {
			Month        string
			RevenueCents int64
		}
		var revRows []revRow
		if aggErr == nil {
			if err := scope(db.Model(&models.Order{})).
				Select("to_char(date_trunc('month', created_at), 'YYYY-MM') as month, COALESCE(SUM(cash_paid),0)::bigint as revenue_cents").
				Where("status = ?", "completed").
				Where("created_at >= ?", now.AddDate(0, -5, 0)).
				Group("to_char(date_trunc('month', created_at), 'YYYY-MM')").
				Order("1").
				Scan(&revRows).Error; err != nil {
				aggErr = err
			}
		}
		if aggErr != nil {
			failAgg()
			return
		}
		revenueTrend := make([]gin.H, 0, len(revRows))
		for _, r := range revRows {
			revenueTrend = append(revenueTrend, gin.H{
				"month":   r.Month,
				"revenue": models.Cents(r.RevenueCents).ToYuan(),
			})
		}

		c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
			"role":               role,
			"total_assets":       totalAssets,
			"rented_assets":      rentedAssets,
			"available_assets":   availableAssets,
			"maintenance_assets": maintenanceAssets,
			"active_leases":      activeLeases,
			"expiring_today":     expiringToday,
			"overdue":            overdue,
			"new_orders_today":   newOrdersToday,
			"status_distribution": []gin.H{
				{"name": "available", "value": availableAssets},
				{"name": "rented", "value": rentedAssets},
				{"name": "maintenance", "value": maintenanceAssets},
			},
			"revenue_trend": revenueTrend,
		}})
		return

	default:
		// customer / repair_technician 等无平台/商户/网点作用域身份 → 拒绝（避免空作用域全量泄漏）
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
}

func (h *DashboardHandler) GetNearTransfers(c *gin.Context) {
	tenantID := middleware.GetTenantID(c.Request.Context())
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "tenant_id not found"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	var transfers []interface{}
	// For now, return empty list as we don't have near transfer data yet
	// TODO: Implement real near transfer logic when available

	var total int64
	total = 0

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"list":  transfers,
			"total": total,
			"page":  page,
			"size":  pageSize,
		},
	})
}
