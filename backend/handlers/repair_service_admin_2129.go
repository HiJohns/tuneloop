package handlers

// #2129：PC 后台「维修服务单」列表——**授权范围 = 商户组织**（维修单关联商户组织；
// 网点账号 oid=网点组织 → 查不到商户 → 天然不可见，不依赖 cus_perm）。
// cus_perm `repair:read` 作为操作谓词由路由中间件叠加。
// #2229：**本商户网点管理员**（site_admin 成员的站点归属该商户）亦可见——转派询价要求
// 的前置可见性（受控商户脱敏口径不适用：当前维修师信息对用户可见、用户信息对商户亦可见）。

import (
	"net/http"
	"strconv"
	"strings"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
)

// ListMerchantRepairServices GET /api/admin/repair-services（#2129 / #2229）
func ListMerchantRepairServices(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	oid := middleware.GetOrgID(ctx)
	tid := middleware.GetTenantID(ctx)

	// 授权范围（#2129/#2229）：先按商户组织（oid）解析；解析不到且调用者为**本商户网点
	// 管理员**时，按站点归属解析其商户。**不得**盲目回退按 tid 匹配——否则任意同租户
	// 员工都能看到全商户（破坏 #2129 授权范围隔离）。
	var merchant models.Merchant
	found := false
	if oid != "" {
		if err := db.WithContext(database.IdentityCtx(ctx)).
			Where("org_id = ?", oid).First(&merchant).Error; err == nil {
			found = true
		}
	}
	if !found && tid != "" {
		adminTenants := siteAdminMerchantTenants(db, middleware.GetUserID(ctx))
		if len(adminTenants) > 0 {
			var m models.Merchant
			if err := db.WithContext(database.IdentityCtx(ctx)).
				Where("tenant_id = ? OR org_id = ?", tid, tid).First(&m).Error; err == nil {
				if adminTenants[normalizeRepairTenant(db, m.TenantID)] ||
					adminTenants[normalizeRepairTenant(db, m.OrgID)] {
					merchant = m
					found = true
				}
			}
		}
	}
	if !found {
		c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": []interface{}{}, "total": 0}})
		return
	}
	// 服务单 tenant_id 可能存商户 tenant 或 org id（#2125 形态）→ 双值范围
	scopeIDs := uniqueStrings([]string{merchant.TenantID, merchant.OrgID})

	// #2237：显式 scope 已限定为本商户 tenant/org 双值，必须用 IdentityCtx 关闭
	// GORM 自动租户范围——否则当 JWT tid=商户**组织**时会叠加 tenant_id=<org>，
	// 与显式 IN (tenant, org) 求交 → 列表恒空（生产实测：服务单存于商户 tenant）。
	q := db.WithContext(database.IdentityCtx(ctx)).Model(&models.RepairRequest{}).
		Where("type = ?", repairServiceTypeVal).
		Where("tenant_id IN ?", scopeIDs)
	if s := c.Query("status"); s != "" {
		q = q.Where("status IN ?", strings.Split(s, ","))
	}
	if sp := c.Query("start"); sp != "" {
		if t, ok := parseTimeParam(sp); ok {
			q = q.Where("updated_at >= ?", t)
		}
	}
	if ep := c.Query("end"); ep != "" {
		if t, ok := parseTimeParam(ep); ok {
			q = q.Where("updated_at < ?", t)
		}
	}

	var total int64
	q.Count(&total)

	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	ps, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	ps = clampPageSize(ps, 20, maxPageSize)

	var list []models.RepairRequest
	if err := q.Order("updated_at DESC").Offset((page - 1) * ps).Limit(ps).Find(&list).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "查询失败"})
		return
	}

	// 相关人：顾客（脱敏）+ 维修师名
	userIDs := make([]string, 0, len(list)*2)
	for _, rr := range list {
		if rr.UserID != "" {
			userIDs = append(userIDs, rr.UserID)
		}
		if rr.TechnicianID != nil && *rr.TechnicianID != "" {
			userIDs = append(userIDs, *rr.TechnicianID)
		}
	}
	names := resolveUserNamesByAnyID(db, userIDs)
	phones := map[string]string{}
	if len(userIDs) > 0 {
		var us []models.User
		if err := db.WithContext(database.IdentityCtx(ctx)).
			Where("id IN ? OR iam_sub IN ?", userIDs, userIDs).
			Select("id, iam_sub, phone").Find(&us).Error; err == nil {
			for _, u := range us {
				phones[u.ID] = u.Phone
				if u.IAMSub != "" {
					phones[u.IAMSub] = u.Phone
				}
			}
		}
	}

	// 费用汇总（批量，避免 N+1）
	orderIDs := make([]string, 0, len(list))
	for _, rr := range list {
		orderIDs = append(orderIDs, rr.ID)
	}
	paidByOrder := map[string]int64{}
	shortfallByOrder := map[string]int64{}
	if len(orderIDs) > 0 {
		type agg struct {
			OrderID string
			Sum     int64
		}
		var paid []agg
		db.Model(&models.OrderPaymentRecord{}).
			Select("order_id, COALESCE(SUM(amount),0) as sum").
			Where("order_id IN ? AND order_type = ? AND type = ? AND status = ?", orderIDs, "repair", "payment", "paid").
			Group("order_id").Scan(&paid)
		for _, a := range paid {
			paidByOrder[a.OrderID] = a.Sum
		}
		var short []agg
		db.Model(&models.OrderPaymentRecord{}).
			Select("order_id, COALESCE(SUM(amount),0) as sum").
			Where("order_id IN ? AND order_type = ? AND type = ? AND status = ? AND method = ?", orderIDs, "repair", "payment", "pending", "shortfall").
			Group("order_id").Scan(&short)
		for _, a := range short {
			shortfallByOrder[a.OrderID] = a.Sum
		}
	}

	rows := make([]gin.H, 0, len(list))
	for _, rr := range list {
		techName := ""
		if rr.TechnicianID != nil {
			techName = names[*rr.TechnicianID]
		}
		quoteTotal := int64(0)
		if rr.QuoteRepairCents != nil {
			quoteTotal += int64(*rr.QuoteRepairCents)
		}
		quoteTotal += int64(repairServiceMaterialCents(&rr))
		if rr.QuoteLogisticsCents != nil {
			quoteTotal += int64(*rr.QuoteLogisticsCents)
		}
		rows = append(rows, gin.H{
			"id":                rr.ID,
			"repair_code":       rr.RepairCode,
			"status":            rr.Status,
			"customer_name":     names[rr.UserID],
			"customer_phone":    phones[rr.UserID],
			"technician_name":   techName,
			"quote_total_cents": quoteTotal,
			"paid_cents":        paidByOrder[rr.ID],
			"shortfall_cents":   shortfallByOrder[rr.ID],
			"created_at":        rr.CreatedAt,
			"updated_at":        rr.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
		"list": rows, "total": total, "page": page, "page_size": ps,
	}})
}
