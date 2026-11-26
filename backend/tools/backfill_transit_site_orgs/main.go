// backfill_transit_site_orgs 回填中转网点的组织归属（#2552 P2）。
//
// 历史数据里存在 `type='transit'` 的站点其 `sites.org_id` 为空或等于 `tenant_id`
// （把平台租户当组织）。中转网点应当持有自己的 `branch` 子组织（beaconiam#509），
// `sites.tenant_id` 仍为平台租户——两列不同域（Decision Lock D2）。
//
// 安全语义（Decision Lock D4）：
//   - **默认 dry-run**：只打印计划行与 `writes=0 iam_creates=0`；
//   - `-apply` 才写（建子组织 + 条件更新 `sites.org_id`，条件更新保证幂等）；
//   - `-phase=members` 才重绑成员 IAM 关系（默认 `orgs`，不碰 IAM 成员关系）；
//   - 本工具未挂在任何启动路径上，只能显式执行。
//
// 用法：
//
//	cd backend
//	go run ./tools/backfill_transit_site_orgs                 # dry-run（默认）
//	go run ./tools/backfill_transit_site_orgs -apply         # 回填组织
//	go run ./tools/backfill_transit_site_orgs -apply -phase=members
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"tuneloop-backend/database"
	"tuneloop-backend/services"
)

func main() {
	var (
		apply    = flag.Bool("apply", false, "执行写入（默认 dry-run，只读）")
		phase    = flag.String("phase", services.TransitBackfillPhaseOrgs, "阶段：orgs|members（members 才会重绑成员 IAM 关系）")
		operator = flag.String("operator-id", "", "记录在 IAM 请求中的操作者 ID（用于审计）")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: go run ./tools/backfill_transit_site_orgs [-apply] [-phase=orgs|members]\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	phaseValue := strings.TrimSpace(*phase)
	if phaseValue != services.TransitBackfillPhaseOrgs && phaseValue != services.TransitBackfillPhaseMembers {
		log.Fatalf("invalid -phase=%q (want orgs|members)", *phase)
	}

	db, err := database.InitDB(database.LoadConfig())
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	database.SetDB(db)

	plan, err := services.PlanTransitSiteOrgBackfill(db)
	if err != nil {
		log.Fatalf("plan failed: %v", err)
	}

	fmt.Printf("PHASE=%s\n", phaseValue)
	fmt.Printf("APPLY=%t\n", *apply)
	fmt.Printf("PLAN_ROWS=%d\n", len(plan))
	need := 0
	for _, row := range plan {
		if row.NeedsBackfill {
			need++
		}
		fmt.Printf("PLAN_ROW site=%s name=%q tenant=%s org=%q status=%s members=%d needs_backfill=%t\n",
			row.SiteID, row.Name, row.TenantID, row.OrgID, row.Status, row.MemberCount, row.NeedsBackfill)
	}
	fmt.Printf("PLAN_NEEDS_BACKFILL=%d\n", need)

	if !*apply {
		// dry-run 必须在输出上自证 0 写入（不能只靠代码阅读）。
		// #2552 L2（Round 1 审计）：计数由服务返回值派生，而不是写死常量——
		// 一旦 `ApplyTransitSiteOrgBackfill` 在 Apply=false 时产生写入，这里会直接暴露。
		dry, err := services.ApplyTransitSiteOrgBackfill(db, nil, services.TransitSiteBackfillOptions{
			Apply:      false,
			Phase:      phaseValue,
			OperatorID: *operator,
		})
		if err != nil {
			log.Fatalf("dry-run plan failed: %v", err)
		}
		fmt.Printf("DRYRUN_WRITES=%d\n", dry.Created+dry.Updated)
		fmt.Printf("DRYRUN_IAM_CREATES=%d\n", dry.IAMCreates)
		fmt.Printf("APPLY_CREATED=%d\n", dry.Created)
		fmt.Printf("APPLY_UPDATED=%d\n", dry.Updated)
		fmt.Printf("APPLY_SKIPPED=%d\n", dry.Skipped)
		fmt.Printf("MEMBERS_REBIND=%d\n", dry.MembersRebind)
		fmt.Printf("DRYRUN_UNCHANGED_SITES=%d\n", len(plan)-need)
		return
	}

	iam := services.NewIAMClient()
	result, err := services.ApplyTransitSiteOrgBackfill(db, iam, services.TransitSiteBackfillOptions{
		Apply:      true,
		Phase:      phaseValue,
		OperatorID: *operator,
	})
	if err != nil {
		log.Fatalf("apply failed: %v", err)
	}

	fmt.Printf("APPLY_CREATED=%d\n", result.Created)
	fmt.Printf("APPLY_UPDATED=%d\n", result.Updated)
	fmt.Printf("APPLY_SKIPPED=%d\n", result.Skipped)
	fmt.Printf("APPLY_IAM_CREATES=%d\n", result.IAMCreates)
	fmt.Printf("MEMBERS_REBIND=%d\n", result.MembersRebind)
}
