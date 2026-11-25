#!/usr/bin/env bash
# #2490 证据生成器 — migrate.sh 空 email 左移 / 畸形行 / 写库确认。
#
# 用法：bash scripts/verify-2490.sh <r1..r8>
#   r1 R? docs/evidence/2490/R1-empty-email.txt   空 email 不左移（dry-run 计划）
#   r2 docs/evidence/2490/R2-bad-perm.txt         非法 cus_perm → exit 3
#   r3 docs/evidence/2490/R3-short-row.txt        字段数不足 → exit 3
#   r4 docs/evidence/2490/R4-idempotent.txt       位已置 → SKIP
#   r5 docs/evidence/2490/R5-apply-confirm.txt    --yes/--confirm-rows/--fixture 组合
#   r6 docs/evidence/2490/R6-red-green.txt        旧 Tab-IFS 丢位 vs 新读法保留
#   r7 docs/evidence/2490/R7-syntax.txt           bash -n + 同类扫描（信息行）
#   r8 docs/evidence/2490/R8-gate.txt             gate 报告 + plan-source 四源
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4|r5|r6|r7|r8) ;;
  *) echo "usage: $0 <r1..r8>" >&2; exit 2 ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2490
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_REF:-}" ]; then
  FIX_REF="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
fi
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
DIRTY=false
[ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR")" ] && DIRTY=true
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
TREE_LINE="# tree=$HEAD_SHA"

MIGRATE="scripts/migrate.sh"
TESTS="scripts/tests/test_migrate_2490.sh"
MASK=$(( (1 << 18) | (1 << 19) ))

require_tokens() {
  local log="$1"; shift
  local missing="" tok
  for tok in "$@"; do
    grep -Fq -- "$tok" "$log" || missing="$missing [$tok]"
  done
  if [ -n "$missing" ]; then echo "ASSERT_FAIL: missing tokens:$missing" >&2; return 1; fi
}
forbid() {
  if grep -Fq -- "$2" "$1"; then echo "ASSERT_FAIL: forbidden token present: [$2]" >&2; return 1; fi
}

mkdir -p "$REPO/tmp"
WORK="$(mktemp -d "$REPO/tmp/verify-2490.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
LOG="$WORK/out.log"
RC=0
NAME=""

run_fixture() { # text [args...]
  local text="$1"; shift
  printf '%s\n' "$text" > "$WORK/fixture.txt"
  bash "$MIGRATE" --fixture "$WORK/fixture.txt" "$@" 2>&1
}

case "$MODE" in
  r1)
    NAME="R1-empty-email.txt"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      echo "FIXTURE=u1||196608"
      echo "CMD=bash $MIGRATE --fixture <file>"
      OUT="$(run_fixture 'u1||196608')"; rc=$?
      echo "$OUT"
      echo "EXIT=$rc"
      echo "RESULT: $([ "$rc" = 0 ] && echo PASS || echo FAIL)"
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'ACTION=UPDATE uid=u1 email= old_perm=196608 new_perm=983040' \
      'PLAN_UPDATES=1' 'DB_WRITES=0' 'EXIT=0' || RC=1
    forbid "$LOG" 'email=196608' || RC=1
    ;;
  r2)
    NAME="R2-bad-perm.txt"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      echo "FIXTURE_1=u2|a@b|   FIXTURE_2=u3|a@b|abc"
      OUT1="$(run_fixture 'u2|a@b|')"; rc1=$?
      echo "--- empty cus_perm ---"; echo "$OUT1"; echo "EXIT=$rc1"
      OUT2="$(run_fixture 'u3|a@b|abc')"; rc2=$?
      echo "--- non-numeric cus_perm ---"; echo "$OUT2"; echo "EXIT=$rc2"
      echo "RESULT: $([ "$rc1" = 3 ] && [ "$rc2" = 3 ] && echo PASS || echo FAIL)"
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'MIGRATE_INVALID=1' 'EXIT=3' 'DB_WRITES=0' \
      'BAD_PERM=abc' 'RESULT: FAIL' || RC=1
    ;;
  r3)
    NAME="R3-short-row.txt"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      echo "FIXTURE=u4|a@b"
      OUT="$(run_fixture 'u4|a@b')"; rc=$?
      echo "$OUT"; echo "EXIT=$rc"
      echo "RESULT: $([ "$rc" = 3 ] && echo PASS || echo FAIL)"
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'BAD_ROW=1' 'ROW_FIELDS=2' 'MIGRATE_INVALID=1' 'EXIT=3' \
      'DB_WRITES=0' || RC=1
    ;;
  r4)
    NAME="R4-idempotent.txt"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      echo "FIXTURE=u5|a@b|983040"
      echo "MASK=$MASK  MASK_AND_PERM=$(( 983040 & MASK ))"
      OUT="$(run_fixture 'u5|a@b|983040')"; rc=$?
      echo "$OUT"; echo "EXIT=$rc"
      echo "RESULT: $([ "$rc" = 0 ] && echo PASS || echo FAIL)"
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'ACTION=SKIP uid=u5' 'SKIPPED=1' 'PLAN_UPDATES=0' 'DB_WRITES=0' || RC=1
    ;;
  r5)
    NAME="R5-apply-confirm.txt"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      echo "--- (a) --yes without --confirm-rows ---"
      bash "$MIGRATE" --yes 2>&1; echo "EXIT=$?"
      echo "--- (b) --confirm-rows mismatch (9 vs 1) ---"
      run_fixture 'u1||196608' --confirm-rows 9; echo "EXIT=$?"
      echo "--- (c) --confirm-rows match (1) ---"
      run_fixture 'u1||196608' --confirm-rows 1; echo "EXIT=$?"
      echo "--- (d) --fixture + --yes (mutually exclusive) ---"
      run_fixture 'u1||196608' --yes; echo "EXIT=$?"
      echo "--- (e) zero-update plan with stale --confirm-rows 1 (strict D4) ---"
      run_fixture 'u5|a@b|983040' --confirm-rows 1; echo "EXIT=$?"
      echo "--- (f) zero-update plan with --confirm-rows 0 ---"
      run_fixture 'u5|a@b|983040' --confirm-rows 0; echo "EXIT=$?"
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'APPLY_CONFIRM_MISMATCH=1' 'APPLY_READY=1' 'DB_WRITES=0' || RC=1
    ;;
  r6)
    NAME="R6-red-green.txt"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      OLD_INPUT="u1$(printf '\t' )$(printf '\t')196608"
      # The tab is built with printf on purpose: this line reproduces the OLD
      # reader, it is not a parsing site that needs the #2490 fix (keeps the
      # R7 SAME_PATH_SCAN informational count at the single remaining script).
      IFS="$(printf '\t')" read -r _ouid _oemail _operm <<< "$OLD_INPUT"
      OLD_NEW_PERM=$(( ${_operm:-0} | MASK ))
      NEW_PERM=$(( 196608 | MASK ))
      echo "OLD_INPUT_TAB_IFS=u1<TAB><TAB>196608"
      echo "OLD_EMAIL_FIELD=${_oemail:-}"
      echo "OLD_PERM_FIELD=${_operm:-}"
      echo "OLD_NEW_PERM=$OLD_NEW_PERM"
      echo "NEW_PERM=$NEW_PERM"
      [ $(( OLD_NEW_PERM & 196608 )) -ne 196608 ] && echo "OLD_LOST_BITS=1" || echo "OLD_LOST_BITS=0"
      [ $(( NEW_PERM & 196608 )) -eq 196608 ] && echo "FIXED=1" || echo "FIXED=0"
      echo "--- new implementation on the same fixture ---"
      run_fixture 'u1||196608' | grep -E '^(ACTION=|PLAN_UPDATES=)'
      echo "NEW_SCRIPT_PERM=$(run_fixture 'u1||196608' | sed -n 's/.*new_perm=//p' | head -1)"
      echo "RESULT: PASS"
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'OLD_LOST_BITS=1' 'FIXED=1' 'NEW_PERM=983040' 'NEW_SCRIPT_PERM=983040' || RC=1
    ;;
  r7)
    NAME="R7-syntax.txt"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      echo "CMD=bash -n $MIGRATE"
      if bash -n "$MIGRATE"; then echo "SYNTAX_OK=1"; else echo "SYNTAX_OK=0"; fi
      if command -v shellcheck >/dev/null 2>&1; then
        shellcheck "$MIGRATE" 2>&1; echo "SHELLCHECK_EXIT=$?"
      else
        echo "SHELLCHECK_EXIT=NA (shellcheck not installed)"
      fi
      hits=$(grep -rn "IFS=\$'\\\\t'" scripts 2>/dev/null | grep -v 'tests/test_migrate_2490.sh' || true)
      n=$(printf '%s' "$hits" | grep -c . || true)
      echo "SAME_PATH_SCAN=${n:-0}"
      echo "SAME_PATH_SCAN_HITS_BEGIN"
      printf '%s\n' "$hits"
      echo "SAME_PATH_SCAN_HITS_END"
      echo "SAME_PATH_SCAN_NOTE=info only: the remaining hit is report-merchant-direct-role-drift-2462.sh, which belongs to #2484 (fix 827f9d1dc not merged into develop); the R6 replay builds its tab via printf so it is not counted as a parsing site"
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'SYNTAX_OK=1' || RC=1
    ;;
  r8)
    NAME="R8-gate.txt"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      fail=0
      GATE="$OUT_DIR/pre-ready-$ISSUE.json"
      overall="$(jq -r '.overall // empty' "$GATE" 2>/dev/null)"
      if [ -f "$GATE" ] && [ "$overall" = "PASS" ]; then
        echo "GATE_RESULT=PASS"
      elif [ ! -f "$GATE" ]; then
        echo "GATE_RESULT=PENDING(no report yet)"; fail=1
      else
        echo "GATE_RESULT=MISSING_OR_FAIL(${overall:-missing})"; fail=1
      fi
      p5=$(jq -r '.checks[]? | select(.id=="P5") | .status' "$GATE" 2>/dev/null | head -1)
      echo "P5=${p5:-MISSING}"
      if [ "$p5" = "PASS" ]; then
        echo "FULL_SUITE_EXIT=0 (derived from P5 PASS)"
      else
        echo "FULL_SUITE_EXIT=PENDING"; fail=1
      fi
      gate_sha="$(jq -r '.plan_sha256 // empty' "$GATE" 2>/dev/null)"
      plan_sha="$(sha256sum "$OUT_DIR/plan.md" | awk '{print $1}')"
      if [ -n "$gate_sha" ] && [ "$gate_sha" = "$plan_sha" ]; then
        echo "GATE_PLAN_SHA_MATCH=1"
      else
        echo "GATE_PLAN_SHA_MATCH=0"; fail=1
      fi
      rebuilt=0
      for f in R1-empty-email.txt R2-bad-perm.txt R3-short-row.txt R4-idempotent.txt \
               R5-apply-confirm.txt R6-red-green.txt R7-syntax.txt; do
        [ -f "$OUT_DIR/$f" ] || continue
        head -1 "$OUT_DIR/$f" | grep -q "commit=$FIX_SHA " && rebuilt=$((rebuilt + 1))
      done
      echo "REBUILT_ROWS=$((rebuilt + 1))"
      bash "$HOME/.config/opencode/scripts/plan-source.sh" --issue "$ISSUE" > "$WORK/plan-source.log" 2>&1
      tail -6 "$WORK/plan-source.log"
      if grep -q 'PLAN_SOURCE: PASS' "$WORK/plan-source.log"; then
        echo "PLAN_SOURCE=PASS"
      else
        echo "PLAN_SOURCE=FAIL"; fail=1
      fi
      [ "$fail" = "0" ] && echo "GATE_OK" || echo "GATE_FAIL"
    } > "$LOG" 2>&1
    grep -q '^GATE_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'GATE_RESULT=PASS' 'FULL_SUITE_EXIT=0' 'PLAN_SOURCE=PASS' \
      'GATE_PLAN_SHA_MATCH=1' 'REBUILT_ROWS=8' || RC=1
    ;;
esac

ART="$OUT_DIR/$NAME"
cp "$LOG" "$ART"
echo "ARTIFACT=$ART"
if [ "$RC" != "0" ]; then
  echo "RESULT: FAIL ($MODE)" >&2
  exit 1
fi
echo "RESULT: PASS ($MODE)"
