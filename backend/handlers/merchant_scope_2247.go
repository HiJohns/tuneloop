package handlers

// #2247：操作者商户范围解析与平台管理员判定。
// 归属锚 = 商户 org；tenant 仅在该 tenant 未被多商户共享时作为兼容值。

import (
	"log"

	"gorm.io/gorm"

	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// resolveOperatorMerchant2247 由操作者 JWT org（优先）或 tenant 解析商户。
//
// 返工（#2247 审计观察项）：tenant 回退必须**确定性**——
// 仅当 tenant 唯一归属一个商户时才解析；共享 tenant（多商户）不解析，
// 禁止无序 First 绑定到「任意一个商户」（平台管理员会因此得到随机商户范围）。
func resolveOperatorMerchant2247(db *gorm.DB, orgID, tenantID string) (models.Merchant, bool) {
	var m models.Merchant
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
	return m, false
}

// isPlatformOrg2247：IAM 组织为命名空间主组织（is_primary）→ 平台管理员范围。
func isPlatformOrg2247(iam *services.IAMClient, orgID string) bool {
	if orgID == "" || iam == nil {
		return false
	}
	org, err := iam.GetOrganization(orgID)
	if err != nil || org == nil {
		log.Printf("[#2247] GetOrganization(%s) failed: %v", orgID, err)
		return false
	}
	return org.IsPrimary
}

// merchantScopeIDs2247 返回商户范围的 tenant_id 列表：
// 始终包含商户 org；仅当该 tenant 不被多商户共享时才追加 tenant（#2248 防跨商户）。
func merchantScopeIDs2247(db *gorm.DB, m models.Merchant) []string {
	ids := []string{m.OrgID}
	if m.TenantID != "" && m.TenantID != m.OrgID {
		var cnt int64
		db.Model(&models.Merchant{}).Where("tenant_id = ?", m.TenantID).Count(&cnt)
		if cnt <= 1 {
			ids = append(ids, m.TenantID)
		}
	}
	return uniqueStrings(ids)
}
