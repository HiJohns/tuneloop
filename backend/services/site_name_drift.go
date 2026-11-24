package services

import (
	"fmt"

	"gorm.io/gorm"

	"tuneloop-backend/models"
)

// #2457 / #2459: sites.name 与 merchants.name 都是 IAM organizations.name 的本地镜像
// （Decision Lock D1）。存量漂移只能「看」，不能在代码里静默改名：
// 本文件的纯函数只负责比对，由只读 CLI（`--site-name-drift`）与单测共用，
// 避免出现第二套比对逻辑。

// SiteNameRef 是本地网点名快照（sites 表）。
type SiteNameRef struct {
	SiteID string
	OrgID  string
	Name   string
}

// MerchantNameRef 是本地商户名快照（merchants 表）。
type MerchantNameRef struct {
	MerchantID string
	OrgID      string
	Name       string
}

// OrgNameRef 是 IAM 组织名快照（organizations）。
type OrgNameRef struct {
	OrgID string
	Name  string
}

// DriftRow 是一行「本地与 IAM 不一致」的差异记录。
// LocalID 是本地行 id：网点为 sites.id，商户为 merchants.id。
type DriftRow struct {
	SiteID    string
	OrgID     string
	LocalName string
	IAMName   string
}

// localNameRef 是 ComputeSiteNameDrift / ComputeMerchantNameDrift 共享的比对输入。
// 两个镜像表字段不同（site_id / merchant_id），比对核心只认「本地 id + org_id + 名称」。
type localNameRef struct {
	LocalID string
	OrgID   string
	Name    string
}

// ComputeSiteNameDrift 按 org_id 对齐本地网点与 IAM 组织，输出名称不一致的行。
func ComputeSiteNameDrift(local []SiteNameRef, iam []OrgNameRef) []DriftRow {
	refs := make([]localNameRef, 0, len(local))
	for _, site := range local {
		refs = append(refs, localNameRef{LocalID: site.SiteID, OrgID: site.OrgID, Name: site.Name})
	}
	return computeNameDrift(refs, iam)
}

// ComputeMerchantNameDrift 按 org_id 对齐本地商户与 IAM 组织，输出名称不一致的行。
// 与网点漂移共用同一比较核心（不复制第二套比对逻辑，#2459 变更点 2）。
func ComputeMerchantNameDrift(local []MerchantNameRef, iam []OrgNameRef) []DriftRow {
	refs := make([]localNameRef, 0, len(local))
	for _, merchant := range local {
		refs = append(refs, localNameRef{LocalID: merchant.MerchantID, OrgID: merchant.OrgID, Name: merchant.Name})
	}
	return computeNameDrift(refs, iam)
}

// computeNameDrift 是本地镜像 ↔ IAM 组织名比对核心。规则：
//   - 本地 org_id 为空 → 不参与比对（历史数据无 IAM 组织绑定）；
//   - IAM 侧缺少该组织 → 不产出差异行（组织删除/重建语义不在本模块）；
//   - 名称完全一致（区分大小写）→ 不产出；
//   - 输出按输入顺序稳定，便于报告 diff 与断言。
func computeNameDrift(local []localNameRef, iam []OrgNameRef) []DriftRow {
	iamByOrgID := make(map[string]string, len(iam))
	for _, org := range iam {
		if org.OrgID == "" {
			continue
		}
		iamByOrgID[org.OrgID] = org.Name
	}

	rows := make([]DriftRow, 0, len(local))
	for _, entry := range local {
		if entry.OrgID == "" {
			continue
		}
		iamName, ok := iamByOrgID[entry.OrgID]
		if !ok {
			continue
		}
		if iamName == entry.Name {
			continue
		}
		rows = append(rows, DriftRow{
			SiteID:    entry.LocalID,
			OrgID:     entry.OrgID,
			LocalName: entry.Name,
			IAMName:   iamName,
		})
	}
	return rows
}

// RunSiteNameDriftReport 打印「本地网点/商户名 vs IAM 组织名」漂移清单（只读）。
// 网点段：无差异时输出 `DRIFT: 0`；有差异时每行
// `DRIFT: site=<id> org=<id> local=<name> iam=<name>`，末行 `DRIFT: <count>`。
// 商户段（#2459）：`MERCHANT_DRIFT: merchant=<id> org=<id> local=<name> iam=<name>`，
// 末行 `MERCHANT_DRIFT: <count>`。不含任何写路径（Decision Lock D6）。
func RunSiteNameDriftReport(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("site name drift report: database is nil")
	}

	var sites []models.Site
	if err := db.Select("id", "org_id", "name").
		Where("deleted_at IS NULL AND org_id IS NOT NULL AND org_id::text <> ''").
		Find(&sites).Error; err != nil {
		return fmt.Errorf("site name drift report: load sites: %w", err)
	}

	var merchants []models.Merchant
	if err := db.Select("id", "org_id", "name").
		Where("org_id IS NOT NULL AND org_id::text <> ''").
		Find(&merchants).Error; err != nil {
		return fmt.Errorf("site name drift report: load merchants: %w", err)
	}

	orgs, err := NewIAMClient().ListOrganizations()
	if err != nil {
		return fmt.Errorf("site name drift report: list IAM organizations: %w", err)
	}

	local := make([]SiteNameRef, 0, len(sites))
	for _, site := range sites {
		local = append(local, SiteNameRef{SiteID: site.ID, OrgID: site.OrgID, Name: site.Name})
	}
	localMerchants := make([]MerchantNameRef, 0, len(merchants))
	for _, merchant := range merchants {
		localMerchants = append(localMerchants, MerchantNameRef{MerchantID: merchant.ID, OrgID: merchant.OrgID, Name: merchant.Name})
	}
	iam := make([]OrgNameRef, 0, len(orgs))
	for _, org := range orgs {
		iam = append(iam, OrgNameRef{OrgID: org.ID, Name: org.Name})
	}

	rows := ComputeSiteNameDrift(local, iam)
	for _, row := range rows {
		fmt.Printf("DRIFT: site=%s org=%s local=%q iam=%q\n", row.SiteID, row.OrgID, row.LocalName, row.IAMName)
	}
	fmt.Printf("DRIFT: %d\n", len(rows))

	merchantRows := ComputeMerchantNameDrift(localMerchants, iam)
	for _, row := range merchantRows {
		fmt.Printf("MERCHANT_DRIFT: merchant=%s org=%s local=%q iam=%q\n", row.SiteID, row.OrgID, row.LocalName, row.IAMName)
	}
	fmt.Printf("MERCHANT_DRIFT: %d\n", len(merchantRows))
	return nil
}
