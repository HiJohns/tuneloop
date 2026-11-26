package services

// #2552 P2：中转网点组织归属（`sites.org_id`）的存量回填工具。
//
// 背景（#2547 / beaconiam#509）：中转网点是平台直属直营分支，应当持有**自己的**
// `branch` 子组织（`sites.org_id`），而 `sites.tenant_id` 仍是平台租户——两者不同域。
// 历史数据里存在 `org_id` 为空或 `org_id = tenant_id`（把租户当组织）的站点。
//
// 安全约束（Decision Lock D4）：
//   - 默认 **dry-run**：`PlanTransitSiteOrgBackfill` 只读；`Apply...` 仅在 `Apply=true` 时写；
//   - 写入用**条件更新**（`org_id IS NULL OR org_id = tenant_id`）保证幂等；
//   - `--phase=members` 才改 IAM 关系（重绑成员），默认不动；
//   - **不挂任何启动路径**（main / migration / bootstrap 均不调用），只能由 CLI 显式触发。
//
// 本文件只被 `backend/tools/backfill_transit_site_orgs` 调用。

import (
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"tuneloop-backend/models"
)

// TransitSiteBackfillPhase 限定回填阶段。
const (
	TransitBackfillPhaseOrgs    = "orgs"
	TransitBackfillPhaseMembers = "members"
)

// TransitSiteBackfillRow 一行中转网点的回填计划（只读快照）。
type TransitSiteBackfillRow struct {
	SiteID        string `json:"site_id"`
	Name          string `json:"name"`
	TenantID      string `json:"tenant_id"`
	OrgID         string `json:"org_id"`
	Status        string `json:"status"`
	MemberCount   int    `json:"member_count"`
	NeedsBackfill bool   `json:"needs_backfill"`
}

// TransitSiteBackfillOptions 控制 apply 行为。
type TransitSiteBackfillOptions struct {
	// Apply 为 false 时只读（dry-run），不建组织、不写库、不重绑成员。
	Apply bool
	// Phase 取值 "orgs"（默认）或 "members"。
	Phase string
	// OperatorID 记录在 IAM 请求里，便于审计（可为空）。
	OperatorID string
}

// TransitBackfillResult 是 apply 的机器可读计数（CLI 直接输出）。
type TransitBackfillResult struct {
	Created       int `json:"created"`
	Updated       int `json:"updated"`
	Skipped       int `json:"skipped"`
	MembersRebind int `json:"members_rebind"`
	// IAMCreates 统计实际发起的建组织调用数（dry-run 必须为 0）。
	IAMCreates int `json:"iam_creates"`
}

// transitOrgNeedsBackfill 报告 `sites.org_id` 是否需要回填：
// 空（NULL/”）或等于 `tenant_id`（把租户当组织）都算需要。
func transitOrgNeedsBackfill(orgID, tenantID string) bool {
	org := strings.TrimSpace(orgID)
	if org == "" {
		return true
	}
	return org == strings.TrimSpace(tenantID)
}

// PlanTransitSiteOrgBackfill 列出所有中转网点及其回填判定（只读，不产生写入）。
func PlanTransitSiteOrgBackfill(db *gorm.DB) ([]TransitSiteBackfillRow, error) {
	if db == nil {
		return nil, fmt.Errorf("PlanTransitSiteOrgBackfill: nil db")
	}
	var sites []models.Site
	if err := db.Where("type = ? AND deleted_at IS NULL", "transit").
		Order("created_at ASC, id ASC").Find(&sites).Error; err != nil {
		return nil, fmt.Errorf("PlanTransitSiteOrgBackfill: query transit sites: %w", err)
	}

	rows := make([]TransitSiteBackfillRow, 0, len(sites))
	for _, site := range sites {
		var members int64
		if err := db.Table("site_members").
			Where("site_id = ? AND status = 'active'", site.ID).
			Count(&members).Error; err != nil {
			return nil, fmt.Errorf("PlanTransitSiteOrgBackfill: count members site=%s: %w", site.ID, err)
		}
		rows = append(rows, TransitSiteBackfillRow{
			SiteID:        site.ID,
			Name:          site.Name,
			TenantID:      site.TenantID,
			OrgID:         site.OrgID,
			Status:        site.Status,
			MemberCount:   int(members),
			NeedsBackfill: transitOrgNeedsBackfill(site.OrgID, site.TenantID),
		})
	}
	return rows, nil
}

// ApplyTransitSiteOrgBackfill 执行回填（`opts.Apply=false` 时是纯 dry-run）。
//
// 每个需要回填的站点：解析命名空间主组织 → 建 `type=branch` 子组织（`Name=site.Name`，
// 无 owner）→ 条件更新 `sites.org_id`。条件更新未命中说明并发下已被修好，此时
// best-effort 删除刚建的组织（不留孤儿）并计入 skipped。
func ApplyTransitSiteOrgBackfill(db *gorm.DB, iam *IAMClient, opts TransitSiteBackfillOptions) (TransitBackfillResult, error) {
	result := TransitBackfillResult{}
	plan, err := PlanTransitSiteOrgBackfill(db)
	if err != nil {
		return result, err
	}
	if !opts.Apply {
		// dry-run：不解析组织、不建组织、不写库。
		return result, nil
	}
	if iam == nil {
		return result, fmt.Errorf("ApplyTransitSiteOrgBackfill: nil IAM client")
	}

	primaryOrgID := ""
	for _, row := range plan {
		if !row.NeedsBackfill {
			result.Skipped++
			continue
		}
		if primaryOrgID == "" {
			resolved, err := ResolveNamespacePrimaryOrg(iam)
			if err != nil {
				return result, fmt.Errorf("resolve namespace primary org: %w", err)
			}
			primaryOrgID = resolved
		}

		created, err := iam.CreateOrganization(&CreateOrganizationRequest{
			Name:       row.Name,
			Type:       "branch",
			ParentID:   primaryOrgID,
			OperatorID: opts.OperatorID,
			AdminInfo:  nil, // 中转网点没有 owner
		})
		if err != nil {
			return result, fmt.Errorf("create branch org for transit site %s (%s): %w", row.SiteID, row.Name, err)
		}
		result.IAMCreates++

		// 条件更新：只在「仍需要回填」时写入，保证幂等。
		// 注意 `sites.org_id` 是 uuid 列：NULL 表示缺失，不能与 '' 比较（PG 22P02）。
		update := db.Model(&models.Site{}).
			Where("id = ? AND (org_id IS NULL OR org_id = tenant_id)", row.SiteID).
			Updates(map[string]interface{}{"org_id": created.OrgID, "updated_at": time.Now()})
		if update.Error != nil {
			// #2552 L1（Round 1 审计）：条件更新**本身失败**时也必须补偿已建组织。
			// 旧实现只在 `RowsAffected == 0`（并发下已被回填）时删除；DB 报错路径
			// 直接返回 → IAM 里留下一个没有任何站点引用的孤儿组织。
			if delErr := iam.DeleteOrganization(created.OrgID); delErr != nil {
				log.Printf("[TransitBackfill] WARN: orphan org cleanup failed org=%s site=%s: %v",
					created.OrgID, row.SiteID, delErr)
			} else {
				log.Printf("[TransitBackfill] compensated orphan org=%s after update failure site=%s",
					created.OrgID, row.SiteID)
			}
			return result, fmt.Errorf("update sites.org_id for %s: %w", row.SiteID, update.Error)
		}
		if update.RowsAffected == 0 {
			// 并发下已被回填：撤销刚建的组织，避免孤儿。
			if delErr := iam.DeleteOrganization(created.OrgID); delErr != nil {
				log.Printf("[TransitBackfill] WARN: orphan org cleanup failed org=%s site=%s: %v",
					created.OrgID, row.SiteID, delErr)
			}
			result.Skipped++
			continue
		}
		result.Created++
		result.Updated++
	}

	// #2552 P2：`members` 阶段必须在**组织阶段完成后**单独再跑一遍——
	// 组织刚被回填的站点此时已可用，但它们在上面的循环里走的是「回填」分支，
	// 若把重绑嵌在循环内，第二次单独 `--phase=members` 会因「已回填 ⇒ skip」
	// 而永远不重绑成员。这里重新取一次计划，对所有 org 可用的站点执行。
	if opts.Phase == TransitBackfillPhaseMembers {
		refreshed, err := PlanTransitSiteOrgBackfill(db)
		if err != nil {
			return result, err
		}
		for _, row := range refreshed {
			if row.NeedsBackfill {
				// 组织仍不可用（并发/回填失败）：跳过而不是把成员绑到错误组织。
				log.Printf("[TransitBackfill] WARN: skip member rebind site=%s: org still needs backfill (org=%q tenant=%q)",
					row.SiteID, row.OrgID, row.TenantID)
				continue
			}
			rebound, err := RebindTransitSiteMembers(db, iam, row, opts)
			if err != nil {
				return result, err
			}
			result.MembersRebind += rebound
		}
	}
	return result, nil
}

// RebindTransitSiteMembers 把某个中转网点的 active 成员重绑到该站点自己的组织。
//
// 只在 `--phase=members` 时调用。IAM 绑定失败必须显式返回错误（不得静默部分成功）；
// 解绑旧组织失败只记录（绑定已成功，解绑是收敛动作）。
func RebindTransitSiteMembers(db *gorm.DB, iam *IAMClient, row TransitSiteBackfillRow, opts TransitSiteBackfillOptions) (int, error) {
	if iam == nil {
		return 0, fmt.Errorf("RebindTransitSiteMembers: nil IAM client")
	}

	var site models.Site
	if err := db.Where("id = ?", row.SiteID).First(&site).Error; err != nil {
		return 0, fmt.Errorf("RebindTransitSiteMembers: load site %s: %w", row.SiteID, err)
	}
	targetOrg := strings.TrimSpace(site.OrgID)
	if targetOrg == "" || transitOrgNeedsBackfill(site.OrgID, site.TenantID) {
		return 0, fmt.Errorf("RebindTransitSiteMembers: site %s has no usable org_id (org=%q tenant=%q)",
			row.SiteID, site.OrgID, site.TenantID)
	}

	var members []models.SiteMember
	if err := db.Where("site_id = ? AND status = 'active'", row.SiteID).
		Find(&members).Error; err != nil {
		return 0, fmt.Errorf("RebindTransitSiteMembers: list members site=%s: %w", row.SiteID, err)
	}

	rebound := 0
	for _, member := range members {
		var user models.User
		if err := db.Where("id = ?", member.UserID).First(&user).Error; err != nil {
			return rebound, fmt.Errorf("RebindTransitSiteMembers: load user %s: %w", member.UserID, err)
		}
		sub := strings.TrimSpace(user.IAMSub)
		if sub == "" {
			return rebound, fmt.Errorf("RebindTransitSiteMembers: user %s has no iam_sub, cannot rebind", member.UserID)
		}

		if err := iam.BindUserToOrganization(sub, targetOrg, transitBackfillIAMRole(member.Role), opts.OperatorID); err != nil {
			return rebound, fmt.Errorf("RebindTransitSiteMembers: bind user %s to org %s: %w", member.UserID, targetOrg, err)
		}
		rebound++

		oldOrg := strings.TrimSpace(user.OrgID)
		if oldOrg != "" && oldOrg != targetOrg {
			if err := iam.UnbindUserFromOrganization(sub, oldOrg, opts.OperatorID); err != nil {
				log.Printf("[TransitBackfill] WARN: unbind old org failed user=%s old=%s: %v", member.UserID, oldOrg, err)
			}
		}
	}
	return rebound, nil
}

// transitBackfillIAMRole 把中转网点角色映射为 IAM 组织角色。
// 与 handlers.toIAMRole 同口径（services 不能依赖 handlers 包）。
func transitBackfillIAMRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "site_admin":
		return "ADMIN"
	case "site_member":
		return "STAFF"
	default:
		return strings.TrimSpace(role)
	}
}
