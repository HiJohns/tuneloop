package services

import (
	"fmt"

	"gorm.io/gorm"

	"tuneloop-backend/models"
)

// #2457: sites.name 是 IAM organizations.name 的本地镜像（Decision Lock D1）。
// 存量漂移只能「看」，不能在代码里静默改名（D6）：本文件的纯函数只负责比对，
// 由只读 CLI（`--site-name-drift`）与单测共用，避免出现第二套比对逻辑。

// SiteNameRef 是本地网点名快照（sites 表）。
type SiteNameRef struct {
	SiteID string
	OrgID  string
	Name   string
}

// OrgNameRef 是 IAM 组织名快照（organizations）。
type OrgNameRef struct {
	OrgID string
	Name  string
}

// DriftRow 是一行「本地与 IAM 不一致」的差异记录。
type DriftRow struct {
	SiteID    string
	OrgID     string
	LocalName string
	IAMName   string
}

// ComputeSiteNameDrift 按 org_id 对齐本地网点与 IAM 组织，输出名称不一致的行。
// 规则：
//   - 本地 org_id 为空 → 不参与比对（历史网点无 IAM 组织绑定）；
//   - IAM 侧缺少该组织 → 不产出差异行（由组织的删除/重建语义处理，不在本单）；
//   - 名称完全一致（区分大小写）→ 不产出；
//   - 输出按输入顺序稳定，便于报告 diff 与断言。
func ComputeSiteNameDrift(local []SiteNameRef, iam []OrgNameRef) []DriftRow {
	iamByOrgID := make(map[string]string, len(iam))
	for _, org := range iam {
		if org.OrgID == "" {
			continue
		}
		iamByOrgID[org.OrgID] = org.Name
	}

	rows := make([]DriftRow, 0, len(local))
	for _, site := range local {
		if site.OrgID == "" {
			continue
		}
		iamName, ok := iamByOrgID[site.OrgID]
		if !ok {
			continue
		}
		if iamName == site.Name {
			continue
		}
		rows = append(rows, DriftRow{
			SiteID:    site.SiteID,
			OrgID:     site.OrgID,
			LocalName: site.Name,
			IAMName:   iamName,
		})
	}
	return rows
}

// RunSiteNameDriftReport 打印「本地网点名 vs IAM 组织名」漂移清单（只读）。
// 无差异时输出 `DRIFT: 0`；有差异时每行 `DRIFT: site=<id> org=<id> local=<name> iam=<name>`，
// 末行 `DRIFT: <count>`。不含任何写路径（Decision Lock D6）。
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

	orgs, err := NewIAMClient().ListOrganizations()
	if err != nil {
		return fmt.Errorf("site name drift report: list IAM organizations: %w", err)
	}

	local := make([]SiteNameRef, 0, len(sites))
	for _, site := range sites {
		local = append(local, SiteNameRef{SiteID: site.ID, OrgID: site.OrgID, Name: site.Name})
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
	return nil
}
