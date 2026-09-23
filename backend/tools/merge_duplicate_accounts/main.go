// merge_duplicate_accounts merges legacy multi-account rows that share one
// WeChat openid into a single kept account (#2029 S3, refined by #2034).
//
// It consolidates, per group:
//   - IAM  : wx_user_bindings, user_org_relations (transfer org relations and
//     merge functional_roles onto the kept user), optional customer role
//   - local: site_members (transfer + merge multi-roles), business rows
//     (opt-in per table via plan.repoint), removed users marked deleted
//
// SAFETY: dry-run by default. Pass -apply to execute. Business-data re-pointing
// is opt-in per table so nothing is moved implicitly.
//
// Usage:
//
//	IAM_DSN=... TUNELOOP_DSN=... go run ./tools/merge_duplicate_accounts -plan plan.json
//	... -apply
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
)

type Repoint struct {
	Table  string `json:"table"`
	Column string `json:"column"`
}

type Group struct {
	OpenID string   `json:"openid"`
	Keep   string   `json:"keep"`
	Remove []string `json:"remove"`
}

type Plan struct {
	Groups            []Group   `json:"groups"`
	Repoint           []Repoint `json:"repoint,omitempty"`
	TransferRelations bool      `json:"transfer_relations,omitempty"`
	GrantCustomer     bool      `json:"grant_customer,omitempty"`
}

func main() {
	planPath := flag.String("plan", "", "path to merge plan JSON")
	apply := flag.Bool("apply", false, "execute (default: dry-run)")
	flag.Parse()
	if *planPath == "" {
		log.Fatal("missing -plan")
	}
	b, err := os.ReadFile(*planPath)
	if err != nil {
		log.Fatalf("read plan: %v", err)
	}
	var plan Plan
	if err := json.Unmarshal(b, &plan); err != nil {
		log.Fatalf("parse plan: %v", err)
	}
	if len(plan.Groups) == 0 {
		log.Fatal("plan has no groups")
	}

	iam, err := sql.Open("postgres", os.Getenv("IAM_DSN"))
	if err != nil {
		log.Fatalf("open IAM: %v", err)
	}
	defer iam.Close()
	tl, err := sql.Open("postgres", os.Getenv("TUNELOOP_DSN"))
	if err != nil {
		log.Fatalf("open tuneloop: %v", err)
	}
	defer tl.Close()

	mode := "DRY-RUN"
	if *apply {
		mode = "APPLY"
	}
	fmt.Printf("=== merge_duplicate_accounts [%s] transfer_relations=%v grant_customer=%v ===\n",
		mode, plan.TransferRelations, plan.GrantCustomer)

	for _, g := range plan.Groups {
		if err := processGroup(iam, tl, g, plan, *apply); err != nil {
			log.Fatalf("[group openid=%s] %v", g.OpenID, err)
		}
	}
	fmt.Println("done.")
}

type relRow struct {
	OrgID           string
	Role            string
	FunctionalRoles []string
}

func processGroup(iam, tl *sql.DB, g Group, plan Plan, apply bool) error {
	fmt.Printf("\n-- openid=%s keep=%s remove=%v\n", g.OpenID, g.Keep, g.Remove)

	// --- IAM relations inventory ---
	keepRels, err := loadRels(iam, g.Keep)
	if err != nil {
		return err
	}
	removedRels := map[string][]relRow{}
	for _, uid := range g.Remove {
		rs, err := loadRels(iam, uid)
		if err != nil {
			return err
		}
		removedRels[uid] = rs
		fmt.Printf("   IAM relations of %s: %d\n", uid, len(rs))
	}
	fmt.Printf("   IAM relations of keep: %d\n", len(keepRels))

	// --- local site_members inventory ---
	keepSites, err := loadSiteMembers(tl, g.Keep)
	if err != nil {
		return err
	}
	removedSites := map[string][]siteRow{}
	for _, uid := range g.Remove {
		ss, err := loadSiteMembers(tl, uid)
		if err != nil {
			return err
		}
		removedSites[uid] = ss
		fmt.Printf("   local site_members of %s: %d\n", uid, len(ss))
	}
	fmt.Printf("   local site_members of keep: %d\n", len(keepSites))

	// --- business rows to repoint ---
	for _, uid := range g.Remove {
		for _, rp := range plan.Repoint {
			var n int64
			q := fmt.Sprintf(`SELECT count(*) FROM %s WHERE %s = $1`, rp.Table, rp.Column)
			if err := tl.QueryRow(q, uid).Scan(&n); err != nil {
				fmt.Printf("   [skip] %s.%s: %v\n", rp.Table, rp.Column, err)
				continue
			}
			fmt.Printf("   repoint %s.%s: %s -> %s : %d rows\n", rp.Table, rp.Column, uid, g.Keep, n)
		}
	}

	if !apply {
		fmt.Println("   PLAN: consolidate IAM bindings/relations; transfer local site_members (merge roles); mark removed deleted")
		return nil
	}
	return applyGroup(iam, tl, g, plan, removedRels, removedSites)
}

func loadRels(db *sql.DB, userID string) ([]relRow, error) {
	rows, err := db.Query(`SELECT org_id, role, COALESCE(functional_roles, '[]'::jsonb) FROM user_org_relations WHERE user_id = $1 AND is_active = true`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []relRow
	for rows.Next() {
		var r relRow
		var fr []byte
		if err := rows.Scan(&r.OrgID, &r.Role, &fr); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(fr, &r.FunctionalRoles)
		out = append(out, r)
	}
	return out, nil
}

type siteRow struct {
	SiteID string
	Role   string
	Roles  []string
}

func loadSiteMembers(db *sql.DB, userID string) ([]siteRow, error) {
	rows, err := db.Query(`SELECT site_id, role, COALESCE(roles, '[]'::jsonb) FROM site_members WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []siteRow
	for rows.Next() {
		var r siteRow
		var rr []byte
		if err := rows.Scan(&r.SiteID, &r.Role, &rr); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(rr, &r.Roles)
		out = append(out, r)
	}
	return out, nil
}

func applyGroup(iam, tl *sql.DB, g Group, plan Plan, removedRels map[string][]relRow, removedSites map[string][]siteRow) error {
	// ---------- IAM ----------
	tx, err := iam.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if !plan.TransferRelations {
		return fmt.Errorf("apply requires transfer_relations=true")
	}
	for _, uid := range g.Remove {
		for _, r := range removedRels[uid] {
			// ensure keep has a relation in this org; merge functional_roles
			var keepFR []byte
			var keepID string
			err := tx.QueryRow(`SELECT id, COALESCE(functional_roles,'[]'::jsonb) FROM user_org_relations WHERE user_id=$1 AND org_id=$2 AND is_active=true`, g.Keep, r.OrgID).Scan(&keepID, &keepFR)
			if err == sql.ErrNoRows {
				if _, err := tx.Exec(`INSERT INTO user_org_relations (id, user_id, org_id, role, functional_roles, is_active, created_at, updated_at) VALUES (gen_random_uuid(), $1, $2, $3, $4::jsonb, true, now(), now())`,
					g.Keep, r.OrgID, r.Role, mustJSON(r.FunctionalRoles)); err != nil {
					return fmt.Errorf("insert keep relation org=%s: %w", r.OrgID, err)
				}
			} else if err != nil {
				return err
			} else {
				var existing []string
				_ = json.Unmarshal(keepFR, &existing)
				merged := unionStrings(existing, r.FunctionalRoles)
				if _, err := tx.Exec(`UPDATE user_org_relations SET functional_roles=$1::jsonb, updated_at=now() WHERE id=$2`, mustJSON(merged), keepID); err != nil {
					return fmt.Errorf("merge relation org=%s: %w", r.OrgID, err)
				}
			}
		}
		if _, err := tx.Exec(`DELETE FROM user_org_relations WHERE user_id=$1`, uid); err != nil {
			return fmt.Errorf("delete relations %s: %w", uid, err)
		}
		// bindings: ensure keep binding, drop removed binding for this openid
		if _, err := tx.Exec(`DELETE FROM wx_user_bindings WHERE openid=$1 AND user_id=$2`, g.OpenID, uid); err != nil {
			return err
		}
	}
	var keepBound bool
	_ = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM wx_user_bindings WHERE openid=$1 AND user_id=$2)`, g.OpenID, g.Keep).Scan(&keepBound)
	if !keepBound {
		if _, err := tx.Exec(`INSERT INTO wx_user_bindings (id, openid, user_id, created_at) VALUES (gen_random_uuid(), $1, $2, now())`, g.OpenID, g.Keep); err != nil {
			return fmt.Errorf("insert keep binding: %w", err)
		}
	}
	if plan.GrantCustomer {
		if err := grantCustomer(tx, g.Keep); err != nil {
			return err
		}
	}
	for _, uid := range g.Remove {
		if _, err := tx.Exec(`UPDATE users SET status='deleted', updated_at=now() WHERE id=$1`, uid); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	fmt.Println("   IAM: relations transferred + bindings consolidated + roles merged")

	// ---------- tuneloop ----------
	ttx, err := tl.Begin()
	if err != nil {
		return err
	}
	defer ttx.Rollback()
	for _, uid := range g.Remove {
		for _, s := range removedSites[uid] {
			var keepID string
			var keepRoles []byte
			err := ttx.QueryRow(`SELECT id, COALESCE(roles,'[]'::jsonb) FROM site_members WHERE user_id=$1 AND site_id=$2`, g.Keep, s.SiteID).Scan(&keepID, &keepRoles)
			if err == sql.ErrNoRows {
				if _, err := ttx.Exec(`INSERT INTO site_members (id, tenant_id, site_id, user_id, role, roles, status, created_at, updated_at) SELECT gen_random_uuid(), tenant_id, site_id, $1, role, roles, status, now(), now() FROM site_members WHERE user_id=$2 AND site_id=$3`,
					g.Keep, uid, s.SiteID); err != nil {
					return fmt.Errorf("transfer site member site=%s: %w", s.SiteID, err)
				}
			} else if err != nil {
				return err
			} else {
				var existing []string
				_ = json.Unmarshal(keepRoles, &existing)
				merged := unionStrings(existing, s.Roles)
				if len(merged) == 0 && s.Role != "" {
					merged = []string{s.Role}
				}
				if _, err := ttx.Exec(`UPDATE site_members SET roles=$1::jsonb, updated_at=now() WHERE id=$2`, mustJSON(merged), keepID); err != nil {
					return fmt.Errorf("merge site member site=%s: %w", s.SiteID, err)
				}
			}
		}
		if _, err := ttx.Exec(`DELETE FROM site_members WHERE user_id=$1`, uid); err != nil {
			return err
		}
		for _, rp := range plan.Repoint {
			q := fmt.Sprintf(`UPDATE %s SET %s=$1 WHERE %s=$2`, rp.Table, rp.Column, rp.Column)
			if _, err := ttx.Exec(q, g.Keep, uid); err != nil {
				return fmt.Errorf("repoint %s.%s: %w", rp.Table, rp.Column, err)
			}
		}
		if _, err := ttx.Exec(`UPDATE users SET status='deleted', updated_at=now() WHERE id=$1`, uid); err != nil {
			return err
		}
	}
	if err := ttx.Commit(); err != nil {
		return err
	}
	fmt.Println("   TUNELOOP: site_members transferred (roles merged) + business rows repointed + removed deleted")
	return nil
}

// grantCustomer mirrors service.MarkUserAsCustomer (#2027/#2031): attach the
// zero-permission "customer" role to the user's member relation in the
// namespace's primary (root) org, ensuring the role template exists.
func grantCustomer(tx *sql.Tx, userID string) error {
	// Resolve the user's root (primary) org + its namespace from the user's own
	// relations — never a global LIMIT 1 (multiple namespaces may exist).
	var orgID, nsID string
	err := tx.QueryRow(`SELECT o.id, o.namespace_id FROM user_org_relations r JOIN organizations o ON o.id = r.org_id
		WHERE r.user_id = $1 AND r.is_active = true
		ORDER BY o.is_primary DESC LIMIT 1`, userID).Scan(&orgID, &nsID)
	if err != nil {
		return fmt.Errorf("resolve root org for %s: %w", userID, err)
	}
	// ensure template
	if _, err := tx.Exec(`INSERT INTO functional_role_templates (id, namespace_id, type, name, code, permissions, description, sys_perm, cus_perm, is_active, created_at, updated_at)
		SELECT gen_random_uuid(), $1, 'system', '顾客', 'customer', '[]'::jsonb, '顾客身份标识（零权限）', 0, 0, true, now(), now()
		WHERE NOT EXISTS (SELECT 1 FROM functional_role_templates WHERE namespace_id=$1 AND code='customer')`, nsID); err != nil {
		return fmt.Errorf("ensure customer template: %w", err)
	}
	// ensure member relation on root org and add 'customer'
	var relID string
	var fr []byte
	err = tx.QueryRow(`SELECT id, COALESCE(functional_roles,'[]'::jsonb) FROM user_org_relations WHERE user_id=$1 AND org_id=$2 AND is_active=true`, userID, orgID).Scan(&relID, &fr)
	if err == sql.ErrNoRows {
		if _, err := tx.Exec(`INSERT INTO user_org_relations (id, user_id, org_id, role, functional_roles, is_active, created_at, updated_at) VALUES (gen_random_uuid(), $1, $2, 'member', '["customer"]'::jsonb, true, now(), now())`, userID, orgID); err != nil {
			return fmt.Errorf("insert customer relation: %w", err)
		}
		return nil
	} else if err != nil {
		return err
	}
	var existing []string
	_ = json.Unmarshal(fr, &existing)
	merged := unionStrings(existing, []string{"customer"})
	if _, err := tx.Exec(`UPDATE user_org_relations SET functional_roles=$1::jsonb, updated_at=now() WHERE id=$2`, mustJSON(merged), relID); err != nil {
		return fmt.Errorf("attach customer role: %w", err)
	}
	return nil
}

func mustJSON(v []string) string {
	if v == nil {
		v = []string{}
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func unionStrings(a, b []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(a)+len(b))
	for _, s := range append(append([]string{}, a...), b...) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
