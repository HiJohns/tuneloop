// merge_duplicate_accounts merges legacy multi-account rows that share one
// WeChat openid into a single kept account (#2029 S3).
//
// SAFETY: dry-run by default. Pass -apply to execute. The tool only touches the
// accounts listed in the -plan JSON file; business-data re-pointing is opt-in
// per table (plan.repoint), so nothing is moved implicitly.
//
// Usage:
//
//	go run ./tools/merge_duplicate_accounts -plan plan.json            # dry-run report
//	go run ./tools/merge_duplicate_accounts -plan plan.json -apply     # execute
//
// Env:
//
//	IAM_DSN       beaconiam postgres DSN (wx_user_bindings + users)
//	TUNELOOP_DSN  tuneloop postgres DSN (users + business tables)
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
	Note   string `json:"note,omitempty"`
}

type Group struct {
	OpenID string   `json:"openid"`
	Keep   string   `json:"keep"`
	Remove []string `json:"remove"`
}

type Plan struct {
	Groups  []Group   `json:"groups"`
	Repoint []Repoint `json:"repoint,omitempty"`
}

func main() {
	planPath := flag.String("plan", "", "path to merge plan JSON")
	apply := flag.Bool("apply", false, "execute (default: dry-run)")
	flag.Parse()
	if *planPath == "" {
		log.Fatal("missing -plan")
	}

	planBytes, err := os.ReadFile(*planPath)
	if err != nil {
		log.Fatalf("read plan: %v", err)
	}
	var plan Plan
	if err := json.Unmarshal(planBytes, &plan); err != nil {
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
	tuneloop, err := sql.Open("postgres", os.Getenv("TUNELOOP_DSN"))
	if err != nil {
		log.Fatalf("open tuneloop: %v", err)
	}
	defer tuneloop.Close()

	mode := "DRY-RUN"
	if *apply {
		mode = "APPLY"
	}
	fmt.Printf("=== merge_duplicate_accounts [%s] ===\n", mode)

	for _, g := range plan.Groups {
		if err := processGroup(iam, tuneloop, g, plan.Repoint, *apply); err != nil {
			log.Fatalf("[group openid=%s] %v", g.OpenID, err)
		}
	}
	fmt.Println("done.")
}

func processGroup(iam, tuneloop *sql.DB, g Group, repoints []Repoint, apply bool) error {
	fmt.Printf("\n-- openid=%s keep=%s remove=%v\n", g.OpenID, g.Keep, g.Remove)

	// Report every binding of this openid.
	rows, err := iam.Query(`SELECT user_id FROM wx_user_bindings WHERE openid = $1`, g.OpenID)
	if err != nil {
		return fmt.Errorf("query bindings: %w", err)
	}
	var bound []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return err
		}
		bound = append(bound, uid)
	}
	rows.Close()
	fmt.Printf("   bindings=%v\n", bound)

	keepBound := false
	for _, b := range bound {
		if b == g.Keep {
			keepBound = true
		}
	}

	// Business-data counts per removed user (dry-run visibility).
	for _, uid := range g.Remove {
		for _, rp := range repoints {
			var n int64
			q := fmt.Sprintf(`SELECT count(*) FROM %s WHERE %s = $1`, rp.Table, rp.Column)
			if err := tuneloop.QueryRow(q, uid).Scan(&n); err != nil {
				fmt.Printf("   [skip] %s.%s: %v\n", rp.Table, rp.Column, err)
				continue
			}
			fmt.Printf("   %s.%s rows for %s: %d\n", rp.Table, rp.Column, uid, n)
		}
	}

	if !apply {
		fmt.Printf("   PLAN: ensure binding(keep); delete bindings(remove); mark users deleted; repoint %d table(s)\n", len(repoints))
		return nil
	}

	// --- APPLY ---
	tx, err := iam.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if !keepBound {
		if _, err := tx.Exec(`INSERT INTO wx_user_bindings (id, openid, user_id, created_at) VALUES (gen_random_uuid(), $1, $2, now())`, g.OpenID, g.Keep); err != nil {
			return fmt.Errorf("insert keep binding: %w", err)
		}
	}
	for _, uid := range g.Remove {
		if _, err := tx.Exec(`DELETE FROM wx_user_bindings WHERE openid = $1 AND user_id = $2`, g.OpenID, uid); err != nil {
			return fmt.Errorf("delete binding %s: %w", uid, err)
		}
	}
	for _, uid := range g.Remove {
		if _, err := tx.Exec(`UPDATE users SET status = 'deleted', updated_at = now() WHERE id = $1`, uid); err != nil {
			return fmt.Errorf("mark IAM user deleted %s: %w", uid, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	fmt.Println("   IAM: bindings consolidated + removed users marked deleted")

	ttx, err := tuneloop.Begin()
	if err != nil {
		return err
	}
	defer ttx.Rollback()
	for _, rp := range repoints {
		for _, uid := range g.Remove {
			q := fmt.Sprintf(`UPDATE %s SET %s = $1 WHERE %s = $2`, rp.Table, rp.Column, rp.Column)
			if _, err := ttx.Exec(q, g.Keep, uid); err != nil {
				return fmt.Errorf("repoint %s.%s: %w", rp.Table, rp.Column, err)
			}
		}
	}
	for _, uid := range g.Remove {
		if _, err := ttx.Exec(`UPDATE users SET status = 'deleted', updated_at = now() WHERE id = $1`, uid); err != nil {
			return fmt.Errorf("mark local user deleted %s: %w", uid, err)
		}
	}
	if err := ttx.Commit(); err != nil {
		return err
	}
	fmt.Println("   TUNELOOP: business rows repointed + removed users marked deleted")
	return nil
}
