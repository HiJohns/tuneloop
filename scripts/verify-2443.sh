#!/usr/bin/env bash
# #2443 Rev.5：handlers 严格 schema 全量恢复。
# 用法：bash scripts/verify-2443.sh <r1|r2|r3|r4|r5-gate|r6|r7>
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1) ART_NAME="R1-inventory.txt" ;;
  r2) ART_NAME="R2-schema.txt" ;;
  r3) ART_NAME="R3-fixtures.txt" ;;
  r4) ART_NAME="R4-mock-panic.txt" ;;
  r5-gate) ART_NAME="R5-handlers-full.txt" ;;
  r6) ART_NAME="R6-product-clean.txt" ;;
  r7) ART_NAME="R7-hygiene.txt" ;;
  *) echo "usage: $0 <r1|r2|r3|r4|r5-gate|r6|r7>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
OUT_DIR="${OUT_DIR:-$REPO_ROOT/docs/evidence/2443}"
mkdir -p "$OUT_DIR" "$REPO_ROOT/tmp"
ART="$OUT_DIR/$ART_NAME"

FIX_REF="${FIX_REF:-HEAD}"
FIX_SHA="$(git rev-parse "$FIX_REF")"
FIX_SHORT="$(git rev-parse --short "$FIX_REF")"
BUILD="1.0.$FIX_SHORT"
COMMIT="$FIX_SHA"
DIRTY=false
[ -n "$(git status --porcelain -uall)" ] && DIRTY=true
HEAD_SHA="$(git rev-parse --short HEAD)"
PROV="# build=$BUILD commit=$COMMIT dirty=$DIRTY tree=$HEAD_SHA"

# Rev.5 rollback commit: product revert is mandated; the fix after it must not
# touch product files. Hygiene/product checks use this as the baseline.
BASELINE_REV="${BASELINE_REV:-0ccc9e733}"
FULL_LOG="${FULL_LOG:-$REPO_ROOT/tmp/2443-full.log}"

run_backend() {
  cd "$REPO_ROOT/backend"
  set -a
  # shellcheck disable=SC1091
  . ./.env
  set +a
  export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}" \
         GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}" \
         TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2443}" \
         IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}" \
         IAM_SECRET="${IAM_SECRET:-test-secret-2443}" \
         TUNELOOP_DB_SIMPLE_PROTOCOL=1
  mkdir -p "$TMPDIR" "$GOCACHE"
}

run_full_suite() {
  run_backend
  go test ./handlers/ -count=1 -timeout 15m > "$FULL_LOG" 2>&1
  return $?
}

schema_counts() {
  local gofile="$REPO_ROOT/tmp/schema_check_2443.go"
  cat > "$gofile" <<'GO'
package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	db, err := sql.Open("postgres", os.Getenv("DSN"))
	if err != nil {
		panic(err)
	}
	defer db.Close()
	var fk, uq int
	if err := db.QueryRow(`SELECT count(*) FROM pg_constraint WHERE contype='f' AND connamespace='public'::regnamespace`).Scan(&fk); err != nil {
		panic(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM pg_constraint WHERE contype='u' AND connamespace='public'::regnamespace`).Scan(&uq); err != nil {
		panic(err)
	}
	fmt.Printf("FK=%d UNIQUE=%d\n", fk, uq)
}
GO
  run_backend
  DSN="host=${POSTGRES_HOST} port=${POSTGRES_PORT} user=${POSTGRES_USER} password=${POSTGRES_PASSWORD} dbname=tuneloop_test_template sslmode=disable" \
    go run "$gofile"
}

case "$MODE" in
  r1)
    {
      echo "$PROV"
      run_backend
      go test ./handlers/ -run '^TestGetInventoryRentSetting($|_Filters$)' -count=1 -v
      rc=$?
      echo "R1_EXIT=$rc"
      exit "$rc"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r2)
    {
      echo "$PROV"
      # 1) 禁止“测试库去 FK”手段
      if grep -Rqs 'dropTestForeignKeys\|ensureLegacyTestConstraints' "$REPO_ROOT/backend"; then
        echo "FAIL: FK-dropping helper still present"
        exit 1
      fi
      echo "OK: no dropTestForeignKeys/ensureLegacyTestConstraints"
      # 2) 测试模板保留生产 FK/唯一约束
      counts="$(schema_counts)" || exit 1
      echo "SCHEMA: $counts"
      fk="$(printf '%s' "$counts" | sed -n 's/.*FK=\([0-9]*\).*/\1/p')"
      uq="$(printf '%s' "$counts" | sed -n 's/.*UNIQUE=\([0-9]*\).*/\1/p')"
      [ "${fk:-0}" -gt 0 ] || { echo "FAIL: FK count=0"; exit 1; }
      [ "${uq:-0}" -gt 0 ] || { echo "FAIL: UNIQUE count=0"; exit 1; }
      # 3) 全量日志：无 42704（缺表）
      if [ ! -s "$FULL_LOG" ]; then run_full_suite || true; fi
      n42704="$(grep -c 'SQLSTATE 42704' "$FULL_LOG" || true)"
      echo "LOG_42704=${n42704:-0}"
      [ "${n42704:-0}" -eq 0 ] || { echo "FAIL: undefined_table found"; exit 1; }
      echo "SCHEMA_OK"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r3)
    {
      echo "$PROV"
      if ! grep -q 'func SeedOrderParents' "$REPO_ROOT/backend/handlers/testfixtures/seed.go"; then
        echo "FAIL: SeedOrderParents missing"
        exit 1
      fi
      echo "OK: seed single exit SeedOrderParents present"
      if [ ! -s "$FULL_LOG" ]; then run_full_suite || true; fi
      n23503="$(grep -c 'SQLSTATE 23503' "$FULL_LOG" || true)"
      n23505="$(grep -c 'SQLSTATE 23505' "$FULL_LOG" || true)"
      nuser="$(grep -c "Key: 'Username' Error:Field validation" "$FULL_LOG" || true)"
      echo "LOG_23503=${n23503:-0} LOG_23505=${n23505:-0} LOG_USERNAME_REQUIRED=${nuser:-0}"
      [ "${n23503:-0}" -eq 0 ] || { echo "FAIL: FK violation残留"; exit 1; }
      [ "${n23505:-0}" -eq 0 ] || { echo "FAIL: unique violation残留"; exit 1; }
      [ "${nuser:-0}" -eq 0 ] || { echo "FAIL: username required残留"; exit 1; }
      echo "FIXTURES_OK"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r4)
    {
      echo "$PROV"
      run_backend
      go test ./handlers/ -run '^(Test_CreateMerchant_CusPerm|TestTransitSiteMemberRoleRestriction)$' -count=1 -v
      rc=$?
      echo "R4_EXIT=$rc"
      exit "$rc"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r5-gate)
    {
      echo "$PROV"
      run_full_suite
      rc=$?
      fails="$(grep -c '^--- FAIL' "$FULL_LOG" || true)"
      panics="$(grep -c '^panic:' "$FULL_LOG" || true)"
      echo "FULL_SUITE_EXIT=$rc FAIL_COUNT=$fails PANIC_COUNT=$panics"
      tail -5 "$FULL_LOG"
      [ "$rc" -eq 0 ] || exit 1
      [ "${fails:-1}" -eq 0 ] || exit 1
      [ "${panics:-1}" -eq 0 ] || exit 1
      echo "GATE_OK"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r6)
    {
      echo "$PROV"
      changed="$(git diff --name-only "$BASELINE_REV..$FIX_SHA" -- backend | grep -vE '(_test\.go$|testfixtures/)' || true)"
      if [ -n "$changed" ]; then
        echo "FAIL: product files changed since Rev.5 baseline:"
        printf '%s\n' "$changed"
        exit 1
      fi
      echo "OK: no product file changed in fix chain $BASELINE_REV..$FIX_SHORT"
      if grep -Rqs 'requestedDepositTier' "$REPO_ROOT/backend"; then
        echo "FAIL: requestedDepositTier still present"
        exit 1
      fi
      echo "OK: requestedDepositTier absent"
      echo "PRODUCT_CLEAN_OK"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r7)
    {
      echo "$PROV"
      diff="$(git diff "$BASELINE_REV..$FIX_SHA" -- '*_test.go')"
      added_skip="$(printf '%s\n' "$diff" | grep -c '^+.*t\.Skip' || true)"
      removed_tests="$(printf '%s\n' "$diff" | grep -c '^-func Test' || true)"
      p1="$(grep -v 'p1=' "$REPO_ROOT/scripts/verify-2443.sh" | grep -Ec '(go test|gotestsum).* -p 1' || true)"
      echo "ADDED_SKIP=${added_skip:-0} REMOVED_TEST_FUNCS=${removed_tests:-0} P1_USAGE=${p1:-0}"
      [ "${added_skip:-0}" -eq 0 ] || { echo "FAIL: added t.Skip"; exit 1; }
      [ "${removed_tests:-0}" -eq 0 ] || { echo "FAIL: removed test funcs"; exit 1; }
      [ "${p1:-0}" -eq 0 ] || { echo "FAIL: -p 1 usage"; exit 1; }
      echo "HYGIENE_OK"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
esac
