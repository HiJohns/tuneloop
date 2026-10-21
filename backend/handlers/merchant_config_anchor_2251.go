package handlers

// #2251：商户级配置（定价 / 维修设置 / 乐币策略 / 返点）的归属锚。
//
// 多个商户可能共享同一 tenants 值（历史数据），按操作者 tenant 存取会让共享
// tenant 的商户共用同一份配置。归属一律解析为商户 org；解析不到时由调用方
// 显式拒绝——绝不回落到 tenant（回落等于重新引入跨商户共用）。

import (
	"context"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"gorm.io/gorm"
)

// resolveOperatorMerchantConfig resolves the merchant that owns the
// merchant-scoped config for the requesting operator.
//
// Resolution order:
//  1. JWT org → merchants.org_id (merchant admins carry their merchant org)
//  2. JWT tenant → merchants.tenant_id, only when exactly one merchant owns it
//
// A shared tenant (multiple merchants) is ambiguous and intentionally does NOT
// resolve; callers must return an explicit error instead of sharing config.
func resolveOperatorMerchantConfig(ctx context.Context) (models.Merchant, bool) {
	db := merchantConfigDB(ctx)
	var m models.Merchant

	orgID := middleware.GetOrgID(ctx)
	tenantID := middleware.GetTenantID(ctx)
	if orgID != "" {
		if err := db.Where("org_id = ?", orgID).First(&m).Error; err == nil && m.OrgID != "" {
			return m, true
		}
	}
	if tenantID != "" {
		var merchants []models.Merchant
		if err := db.Where("tenant_id = ?", tenantID).Limit(2).Find(&merchants).Error; err == nil &&
			len(merchants) == 1 && merchants[0].OrgID != "" {
			return merchants[0], true
		}
	}
	return models.Merchant{}, false
}

// resolveMerchantConfigAnchor returns the merchant org used as the config key
// for the requesting operator (see resolveOperatorMerchantConfig).
func resolveMerchantConfigAnchor(ctx context.Context) (string, bool) {
	m, ok := resolveOperatorMerchantConfig(ctx)
	if !ok || m.OrgID == "" {
		return "", false
	}
	return m.OrgID, true
}

// merchantConfigDB returns a DB handle with the automatic tenant scope disabled.
// Merchant-scoped config rows are keyed by merchant org, which usually differs
// from the JWT tenant (shared tenant); leaving addTenantScope active would add
// an always-false AND clause.
func merchantConfigDB(ctx context.Context) *gorm.DB {
	return database.GetDB().WithContext(database.IdentityCtx(ctx))
}
