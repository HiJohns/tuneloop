// backfill_transit_site_orgs backfills IAM organizations for legacy transit sites (#2547).
//
// Background: transit sites used to be created with `sites.org_id = sites.tenant_id`
// (the platform tenant organization), so their members were bound to the platform
// org instead of a per-site branch organization under the namespace organization.
//
// SAFETY:
//   - dry-run by default: prints the read-only plan and performs ZERO writes
//   - `-apply` is required to create IAM organizations / update sites.org_id
//   - member re-binding is opt-in via `-phase=members` (default `orgs`)
//   - never called from a startup path (no migration, no iam_bootstrap)
//
// Usage:
//
//	go run ./tools/backfill_transit_site_orgs                 # dry-run plan
//	go run ./tools/backfill_transit_site_orgs -apply           # create orgs + update org_id
//	go run ./tools/backfill_transit_site_orgs -apply -phase=members
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"tuneloop-backend/database"
	"tuneloop-backend/services"
)

func main() {
	apply := flag.Bool("apply", false, "execute writes (default: dry-run, zero writes)")
	phase := flag.String("phase", services.TransitSiteOrgBackfillPhaseOrgs, "orgs|members (members also re-binds members to the new org)")
	asJSON := flag.Bool("json", false, "print the plan/result as JSON")
	flag.Parse()

	if *phase != services.TransitSiteOrgBackfillPhaseOrgs &&
		*phase != services.TransitSiteOrgBackfillPhaseMembers {
		log.Fatalf("invalid -phase=%q (want orgs|members)", *phase)
	}

	db, err := database.InitDB(database.LoadConfig())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	database.SetDB(db)

	iam := services.NewIAMClient()
	rows, err := services.PlanTransitSiteOrgBackfill(db, iam)
	if err != nil {
		log.Fatalf("plan failed: %v", err)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]interface{}{
			"apply": *apply,
			"phase": *phase,
			"rows":  rows,
		})
	} else {
		mode := "DRY-RUN"
		if *apply {
			mode = "APPLY"
		}
		fmt.Printf("MODE=%s PHASE=%s DRYRUN_ROWS=%d\n", mode, *phase, len(rows))
		for _, row := range rows {
			fmt.Printf("SITE=%s NAME=%q TENANT=%s ORG=%s STATUS=%s MEMBERS=%d NEEDS_BACKFILL=%t\n",
				row.SiteID, row.Name, row.TenantID, row.OrgID, row.Status, row.MemberCount, row.NeedsBackfill)
		}
	}

	// dry-run 到此为止：不产生任何 IAM/DB 写操作。
	if !*apply {
		return
	}

	res, err := services.ApplyTransitSiteOrgBackfill(db, iam, rows, services.TransitSiteOrgBackfillOptions{
		Phase:    *phase,
		Operator: "backfill",
	})
	if err != nil {
		log.Fatalf("apply failed: %v", err)
	}
	fmt.Printf("APPLY_UPDATED=%d APPLY_SKIPPED_MODERN=%d APPLY_IAM_CREATES=%d MEMBERS_REBOUND=%d MEMBERS_SKIPPED=%d\n",
		res.Updated, res.SkippedModern, res.IAMCreates, res.MembersRebound, res.MembersSkipped)
}
