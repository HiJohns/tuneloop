package services

// #2547: 中转网点（platform-direct transit site）在建站时创建命名空间组织下的
// branch 子组织；存量站点由本文件的只读清单 + 显式 apply 工具收敛。
//
// 设计边界（Accepted Plan D6）：本服务**不在任何启动路径调用**（不挂 migration、
// 不挂 iam_bootstrap）。dry-run 是默认行为，`--apply` 才写入；成员关系默认不动，
// 只有 `--phase=members` 才重新绑定。

import (
	"fmt"
	"log"
	"strings"

	"gorm.io/gorm"

	"tuneloop-backend/models"
)

const (
	// TransitSiteOrgBackfillPhaseOrgs 只建组织 + 回填 sites.org_id（默认）。
	TransitSiteOrgBackfillPhaseOrgs = "orgs"
	// TransitSiteOrgBackfillPhaseMembers 额外把成员绑定迁到新组织。
	TransitSiteOrgBackfillPhaseMembers = "members"

	transitSiteTypeForBackfill = "transit"
	iAMOrganizationTypeBranch  = "branch"
)

// TransitSiteOrgBackfillRow 是只读清单的一行（CLI dry-run 直接打印）。
type TransitSiteOrgBackfillRow struct {
	SiteID        string `json:"site_id"`
	Name          string `json:"name"`
	TenantID      string `json:"tenant_id"`
	OrgID         string `json:"org_id"`
	Status        string `json:"status"`
	MemberCount   int64  `json:"member_count"`
	NeedsBackfill bool   `json:"needs_backfill"`
}

// TransitSiteOrgBackfillOptions 控制 apply 行为；Phase 为空等价于 orgs。
type TransitSiteOrgBackfillOptions struct {
	Phase    string
	Operator string
}

// TransitSiteOrgBackfillResult 汇总一次 apply 的写入量（幂等：二次 apply 全 0）。
type TransitSiteOrgBackfillResult struct {
	Updated        int `json:"updated"`
	SkippedModern  int `json:"skipped_modern"`
	IAMCreates     int `json:"iam_creates"`
	MembersRebound int `json:"members_rebound"`
	MembersSkipped int `json:"members_skipped"`
}

// ResolveNamespacePrimaryOrg 返回命名空间的主组织 ID（branch 子组织的 parent_id）。
// 恰好一个 is_primary=true 才是唯一解；0 个或多个 → 显式失败（不猜、不回落租户）。
func ResolveNamespacePrimaryOrg(iam *IAMClient) (string, error) {
	if iam == nil {
		return "", fmt.Errorf("ResolveNamespacePrimaryOrg: nil IAM client")
	}
	orgs, err := iam.ListOrganizations()
	if err != nil {
		return "", fmt.Errorf("ResolveNamespacePrimaryOrg: list organizations: %w", err)
	}
	primary := ""
	count := 0
	for _, org := range orgs {
		if !org.IsPrimary {
			continue
		}
		count++
		primary = org.ID
	}
	if count != 1 {
		return "", fmt.Errorf("ResolveNamespacePrimaryOrg: expected exactly 1 primary organization, got %d", count)
	}
	if strings.TrimSpace(primary) == "" {
		return "", fmt.Errorf("ResolveNamespacePrimaryOrg: primary organization has empty id")
	}
	return primary, nil
}

// isTransitSiteOrgModern 判定站点是否已有自己的组织（org_id 非空且不等于平台租户）。
func isTransitSiteOrgModern(site models.Site) bool {
	orgID := strings.TrimSpace(site.OrgID)
	return orgID != "" && orgID != site.TenantID
}

// PlanTransitSiteOrgBackfill 列出需要回填的存量中转网点（只读，0 写入）。
// 判据：type='transit' 且 (org_id 为空 或 org_id = tenant_id)（仍在用平台租户组织）。
// iam 仅用于前置校验「命名空间主组织唯一可解析」，不产生任何写调用。
func PlanTransitSiteOrgBackfill(db *gorm.DB, iam *IAMClient) ([]TransitSiteOrgBackfillRow, error) {
	if db == nil {
		return nil, fmt.Errorf("PlanTransitSiteOrgBackfill: nil db")
	}
	if _, err := ResolveNamespacePrimaryOrg(iam); err != nil {
		return nil, err
	}

	var sites []models.Site
	if err := db.Where("type = ?", transitSiteTypeForBackfill).
		Order("created_at ASC").Find(&sites).Error; err != nil {
		return nil, fmt.Errorf("PlanTransitSiteOrgBackfill: query transit sites: %w", err)
	}

	rows := make([]TransitSiteOrgBackfillRow, 0, len(sites))
	for _, site := range sites {
		if isTransitSiteOrgModern(site) {
			continue
		}
		var memberCount int64
		if err := db.Model(&models.SiteMember{}).Where("site_id = ?", site.ID).
			Count(&memberCount).Error; err != nil {
			return nil, fmt.Errorf("PlanTransitSiteOrgBackfill: count members for site %s: %w", site.ID, err)
		}
		rows = append(rows, TransitSiteOrgBackfillRow{
			SiteID:        site.ID,
			Name:          site.Name,
			TenantID:      site.TenantID,
			OrgID:         site.OrgID,
			Status:        site.Status,
			MemberCount:   memberCount,
			NeedsBackfill: true,
		})
	}
	return rows, nil
}

// ApplyTransitSiteOrgBackfill 为清单中的存量站点创建 branch 子组织并回填 sites.org_id。
//
// 幂等：apply 前重新读取库内站点状态，已是 modern（org_id 非空且 ≠ tenant_id）的
// 站点一律跳过（计 SkippedModern）；rows 为空时不写任何东西。
func ApplyTransitSiteOrgBackfill(db *gorm.DB, iam *IAMClient, rows []TransitSiteOrgBackfillRow, opts TransitSiteOrgBackfillOptions) (TransitSiteOrgBackfillResult, error) {
	var res TransitSiteOrgBackfillResult
	if db == nil {
		return res, fmt.Errorf("ApplyTransitSiteOrgBackfill: nil db")
	}
	if iam == nil {
		return res, fmt.Errorf("ApplyTransitSiteOrgBackfill: nil IAM client")
	}
	if len(rows) == 0 {
		return res, nil
	}

	parentID, err := ResolveNamespacePrimaryOrg(iam)
	if err != nil {
		return res, err
	}

	selected := make(map[string]bool, len(rows))
	for _, row := range rows {
		if row.SiteID != "" {
			selected[row.SiteID] = true
		}
	}

	var sites []models.Site
	if err := db.Where("type = ?", transitSiteTypeForBackfill).
		Order("created_at ASC").Find(&sites).Error; err != nil {
		return res, fmt.Errorf("ApplyTransitSiteOrgBackfill: query transit sites: %w", err)
	}

	operator := strings.TrimSpace(opts.Operator)
	if operator == "" {
		operator = "backfill"
	}
	phase := strings.TrimSpace(opts.Phase)
	if phase == "" {
		phase = TransitSiteOrgBackfillPhaseOrgs
	}

	for _, site := range sites {
		if isTransitSiteOrgModern(site) {
			res.SkippedModern++
			continue
		}
		if !selected[site.ID] {
			continue
		}

		// 回填以 client credentials 执行（无用户会话）；分支建组织不传 admin_info。
		orgResp, err := iam.CreateOrganization(&CreateOrganizationRequest{
			Name:         site.Name,
			ParentID:     parentID,
			Type:         iAMOrganizationTypeBranch,
			Address:      site.Address,
			ContactPhone: site.Phone,
			OperatorID:   operator,
			AdminInfo:    nil, // 分支组织无 owner
		})
		if err != nil {
			return res, fmt.Errorf("ApplyTransitSiteOrgBackfill: create org for site %s (%s): %w", site.ID, site.Name, err)
		}
		res.IAMCreates++

		if err := db.Model(&models.Site{}).Where("id = ?", site.ID).
			Updates(map[string]interface{}{"org_id": orgResp.OrgID}).Error; err != nil {
			// 补偿：本地回填失败时删除刚建的组织，避免留下无站点引用的孤儿组织。
			if delErr := iam.DeleteOrganization(orgResp.OrgID); delErr != nil {
				log.Printf("[TransitSiteOrgBackfill] rollback DeleteOrganization failed for org %s: %v", orgResp.OrgID, delErr)
			}
			return res, fmt.Errorf("ApplyTransitSiteOrgBackfill: update site %s org_id: %w", site.ID, err)
		}
		res.Updated++

		if phase == TransitSiteOrgBackfillPhaseMembers {
			rebound, skipped, err := rebindTransitSiteMembers(db, iam, site.ID, orgResp.OrgID, operator)
			res.MembersRebound += rebound
			res.MembersSkipped += skipped
			if err != nil {
				return res, err
			}
		}
	}
	return res, nil
}

// backfillIAMRoleForBusinessRole 把 `site_members.role`（业务角色模板码）映射为
// IAM 组织关系角色。无法识别 → STAFF（不得把业务码当 IAM 角色写入）。
//
// 说明：#2540 会把映射下沉为 `services.ToIAMRole` + `NormalizeIAMRelationRole`；
// 本单（#2547）先保留局部实现，待 #2540 合并后改为调用共享函数（避免同名冲突）。
func backfillIAMRoleForBusinessRole(role string) string {
	switch strings.TrimSpace(role) {
	case "merchant_admin":
		return "OWNER"
	case "site_admin":
		return "ADMIN"
	case "repair_technician":
		return "repair_technician"
	case "site_member", "merchant_staff", "staff":
		return "STAFF"
	default:
		return "STAFF"
	}
}

// rebindTransitSiteMembers 把站点成员的组织关系迁到新的 branch 子组织。
// 本地 site_members 行不改（site_id 不变），只改 IAM 关系角色绑定。
func rebindTransitSiteMembers(db *gorm.DB, iam *IAMClient, siteID, orgID, operator string) (int, int, error) {
	var members []models.SiteMember
	if err := db.Where("site_id = ?", siteID).Find(&members).Error; err != nil {
		return 0, 0, fmt.Errorf("rebindTransitSiteMembers: query members for site %s: %w", siteID, err)
	}
	rebound := 0
	skipped := 0
	for _, member := range members {
		var user models.User
		if err := db.Where("id = ?", member.UserID).First(&user).Error; err != nil {
			skipped++
			log.Printf("[TransitSiteOrgBackfill] skip member %s: user lookup failed: %v", member.UserID, err)
			continue
		}
		iamSub := strings.TrimSpace(user.IAMSub)
		if iamSub == "" {
			skipped++
			log.Printf("[TransitSiteOrgBackfill] skip member %s: empty iam_sub (user %s)", member.UserID, user.ID)
			continue
		}
		role := backfillIAMRoleForBusinessRole(member.Role)
		if err := iam.BindUserToOrganization(iamSub, orgID, role, operator); err != nil {
			return rebound, skipped, fmt.Errorf("rebindTransitSiteMembers: bind user %s to org %s: %w", member.UserID, orgID, err)
		}
		rebound++
	}
	return rebound, skipped, nil
}
