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
// #2255（P3 批次1）：与 #2247 的 resolveOperatorMerchant2247 共用同一实现，
// 消除两套并行解析（org 优先 → 唯一 tenant → 共享 tenant 不解析）。
// merchantConfigDB 清除自动 tenant scope，确保 org 分支不被 addTenantScope 叠加。
func resolveOperatorMerchantConfig(ctx context.Context) (models.Merchant, bool) {
	return resolveOperatorMerchant2247(merchantConfigDB(ctx), middleware.GetOrgID(ctx), middleware.GetTenantID(ctx))
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
