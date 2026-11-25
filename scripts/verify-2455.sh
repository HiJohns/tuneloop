#!/usr/bin/env bash
# verify-2455.sh — Evidence Contract 执行器（#2455）。
#
# 用法：bash scripts/verify-2455.sh <r1|r2|r3|r4|r5|r6|r7|r8|r9|r10|all>
#
# 每行产出 docs/evidence/2455/<artifact>，首行为 provenance：
#   # build=1.0.<short> # commit=<full> # dirty=false
# 并在 artifact 末尾追加可 grep 的 TOKEN=VALUE 断言行。
#
# 环境（可用环境变量覆盖）：POSTGRES_HOST=127.0.0.1 POSTGRES_PORT=15433
# POSTGRES_USER=tuneloop_user POSTGRES_PASSWORD=trust TUNELOOP_TEST_DB=tuneloop_test_local
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT" || exit 2

ISSUE=2455
EDIR="docs/evidence/$ISSUE"
COMMIT="$(git rev-parse HEAD)"
SHORT="$(git rev-parse --short=7 HEAD)"
BUILD="1.0.$SHORT"

export PATH="/usr/local/go/bin:$PATH"
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_local}"
export TMPDIR="${TMPDIR:-$ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$ROOT/tmp/gocache}"
mkdir -p "$EDIR" "$TMPDIR" "$GOCACHE" "$ROOT/tmp/evidence-$ISSUE"

HEAVY_RUN="$HOME/.config/opencode/scripts/heavy-run.sh"

# run_heavy: one heavy command under the host lock (falls back to direct exec).
run_heavy() {
  if [ -x "$HEAVY_RUN" ]; then
    bash "$HEAVY_RUN" --scope host --timeout "${HEAVY_LOCK_TIMEOUT:-1800}" -- "$@"
  else
    "$@"
  fi
}

dirty_flag() {
  local d
  d="$(git status --porcelain -- ':!docs/evidence' ':!tmp' 2>/dev/null)"
  if [ -n "$d" ]; then echo "true"; else echo "false"; fi
}

header() {
  local artifact="$1"
  mkdir -p "$(dirname "$artifact")"
  printf '# build=%s # commit=%s # dirty=%s\n' "$BUILD" "$COMMIT" "$(dirty_flag)" > "$artifact"
}

# extract_tokens <log> — pull `TOKEN=VALUE` lines emitted by t.Logf/echo.
extract_tokens() {
  local log="$1"
  grep -E '[A-Z][A-Z0-9_]+=' "$log" \
    | sed -E 's/^.*\.go:[0-9]+: //' \
    | grep -E '^[A-Z][A-Z0-9_]+=' \
    | awk '!seen[$0]++'
}

go_test_row() { # <artifact> <test-regex> <log-name>
  local artifact="$1" regex="$2" log="$ROOT/tmp/evidence-$ISSUE/$3.log"
  header "$artifact"
  ( cd backend && run_heavy go test ./handlers -run "$regex" -count=1 -v ) > "$log" 2>&1
  local rc=$?
  cat "$log" >> "$artifact"
  extract_tokens "$log" >> "$artifact"
  return $rc
}

row_r1() {
  local artifact="$EDIR/R1-index.txt" log="$ROOT/tmp/evidence-$ISSUE/r1.log"
  header "$artifact"
  ( cd backend && run_heavy go test ./handlers -run 'TestSiteNameUniqueIndex_2455' -count=1 -v ) > "$log" 2>&1
  local rc=$?
  cat "$log" >> "$artifact"
  extract_tokens "$log" >> "$artifact"
  [ "$rc" = "0" ] || return 1
  grep -Fq 'INDEX_FOUND=1' "$artifact" || return 1
  grep -Fq 'UP_IDEMPOTENT=1' "$artifact" || return 1
  grep -Fq 'DOWN_DROPS=1' "$artifact" || return 1
  grep -Fq 'deleted_at IS NULL' "$artifact" || return 1
  return 0
}

row_r2() {
  local artifact="$EDIR/R2-create-conflict.txt"
  go_test_row "$artifact" 'TestCreateSite_DuplicateNameRejectedBeforeIAM_2455' r2 || return 1
  grep -Fq 'HTTP=409' "$artifact" || return 1
  grep -Fq 'CODE=40900' "$artifact" || return 1
  grep -Fq 'IAM_CALLS=0' "$artifact" || return 1
  grep -Fq 'TRIMMED_CASE_HTTP=409' "$artifact" || return 1
  return 0
}

row_r3() {
  local artifact="$EDIR/R3-cross-tenant.txt"
  go_test_row "$artifact" 'TestCreateSite_CrossTenantSameNameAllowed_2455' r3 || return 1
  # 计划令牌 CROSS_TENANT_HTTP=200 表示「跨商户创建被接受（2xx）」；CreateSite 契约本身是
  # 201 Created，真实状态码单列在 CROSS_TENANT_STATUS（见 artifact 内注释与 Work Summary）。
  grep -Fq 'CROSS_TENANT_HTTP=200' "$artifact" || return 1
  grep -Fq 'SAME_TENANT_DUP_REJECTED=1' "$artifact" || return 1
  grep -Fq 'RACE_MAPPED=409' "$artifact" || return 1
  return 0
}

row_r4() {
  local artifact="$EDIR/R4-update-conflict.txt"
  go_test_row "$artifact" 'TestUpdateSite_DuplicateNameRejectedBeforeIAM_2455' r4 || return 1
  grep -Fq 'HTTP=409' "$artifact" || return 1
  grep -Fq 'CODE=40900' "$artifact" || return 1
  grep -Fq 'IAM_CALLS=0' "$artifact" || return 1
  grep -Fq 'DB_NAME=北京中转站' "$artifact" || return 1
  return 0
}

row_r5() {
  local artifact="$EDIR/R5-duplicates.txt" log="$ROOT/tmp/evidence-$ISSUE/r5.log"
  header "$artifact"
  bash scripts/tests/test_site_name_duplicates.sh > "$log" 2>&1
  local rc=$?
  cat "$log" >> "$artifact"
  extract_tokens "$log" >> "$artifact"
  [ "$rc" = "0" ] || return 1
  # 真实库只读复跑（空库/预生产同口径查询）；失败不伪装成功。
  ( bash scripts/site-name-duplicates.sh --report --threshold 2 ) >> "$artifact" 2>&1 || true
  grep -Fq 'DUP_GROUPS=0' "$artifact" || return 1
  grep -Fq 'FIXTURE_DUP_GROUPS=1' "$artifact" || return 1
  grep -Fq 'FIXTURE_IDS=2' "$artifact" || return 1
  grep -Fq 'READONLY=1' "$artifact" || return 1
  return 0
}

row_r6() {
  local artifact="$EDIR/R6-context-label.txt"
  go_test_row "$artifact" 'TestWxAccounts_ContextLabelDisambiguatesSameNamedSites_2455' r6 || return 1
  grep -Fq 'DISTINCT=1' "$artifact" || return 1
  grep -Fq 'LABEL_B=卡丹萨 · 卡丹萨乐器' "$artifact" || return 1
  grep -Fq 'ORG_NAME_B=卡丹萨西铁营' "$artifact" || return 1
  return 0
}

row_r7() {
  local artifact="$EDIR/R7-mobile-label.txt"
  local src="frontend-mobile/src/pages-weapp/account-select/AccountSelect.jsx"
  header "$artifact"
  {
    echo "SOURCE=$src"
    { grep -n "label: ct.label" "$src" || true
      grep -n "label: a.is_customer ? '顾客' : (a.label" "$src" || true
      grep -n "join(' · ')" "$src" || true
    } | awk '!seen[$0]++'
  } >> "$artifact"

  local mobile=0 legacy=0 site_only=0
  grep -Fq "label: ct.label" "$src" && mobile=1
  grep -Fq "a.label ||" "$src" && legacy=1
  # 负例：legacy/contexts 里出现「商户名-网点名」旧拼接或仅 site_name 单源
  grep -Fq "].join('-')" "$src" && site_only=1
  grep -Eq "label: [^\n]*\bsite_name\b[^\n]*\)\s*$" "$src" && site_only=1

  {
    echo "MOBILE_USES_SERVER_LABEL=$mobile"
    echo "LEGACY_USES_SERVER_LABEL=$legacy"
    echo "SITE_NAME_ONLY_CONCAT=$site_only"
  } >> "$artifact"

  [ "$mobile" = "1" ] || return 1
  [ "$legacy" = "1" ] || return 1
  [ "$site_only" = "0" ] || return 1
  return 0
}

row_r8() {
  local artifact="$EDIR/R8-pc-profile.txt"
  local src="frontend-pc/src/pages/UserProfile.jsx"
  header "$artifact"
  {
    echo "SOURCE=$src"
    { grep -n "identityLabel" "$src" || true
      grep -n "关联网点" "$src" || true
    } | awk '!seen[$0]++'
  } >> "$artifact"

  local shows=0 fallback=0 undefined_guard=0
  grep -Fq "[user?.tenant_name, user?.site_name].filter(Boolean).join(' · ')" "$src" && shows=1
  grep -Fq "|| '-'" "$src" && fallback=1
  # 负例：直接渲染 user.site_name（无商户名、无 fallback）
  if grep -Fq 'user?.site_name ||' "$src" && grep -Fq '关联网点">{user?.site_name' "$src"; then
    undefined_guard=1
  fi

  {
    echo "PC_SHOWS_MERCHANT_SITE=$shows"
    echo "FALLBACK_SINGLE=$fallback"
    echo "PC_SITE_NAME_ONLY=$undefined_guard"
  } >> "$artifact"

  [ "$shows" = "1" ] || return 1
  [ "$fallback" = "1" ] || return 1
  [ "$undefined_guard" = "0" ] || return 1
  return 0
}

row_r9() {
  local artifact="$EDIR/R9-build.txt" log="$ROOT/tmp/evidence-$ISSUE/r9.log"
  header "$artifact"
  : > "$log"

  ( cd backend && run_heavy go build ./... ) >> "$log" 2>&1
  local build_exit=$?
  echo "--- go build ./... exit=$build_exit" >> "$log"

  ( cd backend && run_heavy go test ./handlers \
      -run 'TestSiteNameUnique|TestCreateSite_DuplicateNameRejectedBeforeIAM_2455|TestCreateSite_CrossTenantSameNameAllowed_2455|TestUpdateSite_DuplicateNameRejectedBeforeIAM_2455|TestWxAccounts|TestUpdateSite_' \
      -count=1 -timeout 15m ) >> "$log" 2>&1
  local test_exit=$?

  ( cd frontend-pc && run_heavy npm run build ) >> "$log" 2>&1
  local pc_exit=$?

  cat "$log" >> "$artifact"

  local fail_count panic_count
  fail_count="$(grep -c -- '--- FAIL' "$log" || true)"
  panic_count="$(grep -ci 'panic:' "$log" || true)"
  {
    echo "BUILD_EXIT=$build_exit"
    echo "TESTS_EXIT=$test_exit"
    echo "TESTS_FAIL=$fail_count"
    echo "PANIC_COUNT=$panic_count"
    echo "PC_BUILD_EXIT=$pc_exit"
  } >> "$artifact"

  [ "$build_exit" = "0" ] || return 1
  [ "$test_exit" = "0" ] || return 1
  [ "$fail_count" = "0" ] || return 1
  [ "$pc_exit" = "0" ] || return 1
  return 0
}

row_r10() {
  local artifact="$EDIR/R10-gate.txt" log="$ROOT/tmp/evidence-$ISSUE/r10.log"
  local report="$EDIR/pre-ready-$ISSUE.json"
  header "$artifact"
  : > "$log"

  # 1) 全量套件（与 gate P5 同口径）：跨 handlers + services。
  ( cd backend && run_heavy go test ./handlers -count=1 -timeout 20m ) >> "$log" 2>&1
  local suite_exit=$?
  echo "--- go test ./handlers exit=$suite_exit" >> "$log"
  ( cd backend && run_heavy go test ./services -count=1 -timeout 15m ) >> "$log" 2>&1
  local services_exit=$?
  echo "--- go test ./services exit=$services_exit" >> "$log"

  local fail_count panic_count
  fail_count="$(grep -c -- '--- FAIL' "$log" || true)"
  panic_count="$(grep -ci 'panic:' "$log" || true)"

  # 2) plan-source 四源一致（Accepted Plan 评论 / plan.md / gate / evidence.json）。
  bash "$HOME/.config/opencode/scripts/plan-source.sh" --issue "$ISSUE" > "$log.plan-source" 2>&1
  local plan_source_rc=$?
  local plan_source=PASS
  [ "$plan_source_rc" = "0" ] || plan_source=FAIL

  # 3) gate 报告（P5 必须真跑，不允许 N/A）。
  local gate_result=MISSING p5=N/A gate_sha= want_sha= sha_match=0
  if [ -f "$report" ]; then
    gate_result="$(jq -r '.overall // "MISSING"' "$report")"
    p5="$(jq -r '.checks[] | select(.id=="P5") | .status' "$report" 2>/dev/null | head -1)"
    gate_sha="$(jq -r '.plan_sha256 // ""' "$report")"
    want_sha="$(sha256sum "$EDIR/plan.md" 2>/dev/null | awk '{print $1}')"
    [ -n "$gate_sha" ] && [ "$gate_sha" = "$want_sha" ] && sha_match=1
  fi

  local full_suite_exit=1
  { [ "$suite_exit" = "0" ] && [ "$services_exit" = "0" ]; } && full_suite_exit=0

  cat "$log" >> "$artifact"
  cat "$log.plan-source" >> "$artifact"
  {
    echo "FULL_SUITE_CMD=cd backend && go test ./handlers -count=1 -timeout 20m && go test ./services -count=1 -timeout 15m"
    echo "FULL_SUITE_EXIT=$full_suite_exit"
    echo "SERVICES_EXIT=$services_exit"
    echo "FAIL_COUNT=$fail_count"
    echo "PANIC_COUNT=$panic_count"
    echo "GATE_RESULT=$gate_result"
    echo "P5=$p5"
    echo "GATE_PLAN_SHA_MATCH=$sha_match"
    echo "PLAN_SOURCE=$plan_source"
  } >> "$artifact"

  [ "$full_suite_exit" = "0" ] || return 1
  [ "$fail_count" = "0" ] || return 1
  [ "$panic_count" = "0" ] || return 1
  [ "$gate_result" = "PASS" ] || return 1
  [ "$p5" = "PASS" ] || return 1
  [ "$sha_match" = "1" ] || return 1
  [ "$plan_source" = "PASS" ] || return 1
  return 0
}

ROW="${1:-}"
case "$ROW" in
  r1) row_r1 ;;
  r2) row_r2 ;;
  r3) row_r3 ;;
  r4) row_r4 ;;
  r5) row_r5 ;;
  r6) row_r6 ;;
  r7) row_r7 ;;
  r8) row_r8 ;;
  r9) row_r9 ;;
  r10) row_r10 ;;
  all)
    FAIL=0
    for r in r1 r2 r3 r4 r5 r6 r7 r8 r9; do
      if "row_$r"; then echo "$r: PASS"; else echo "$r: FAIL"; FAIL=1; fi
    done
    exit "$FAIL"
    ;;
  *)
    echo "usage: $0 <r1|r2|r3|r4|r5|r6|r7|r8|r9|r10|all>" >&2
    exit 2 ;;
esac

rc=$?
if [ "$rc" = "0" ]; then
  echo "$ROW: PASS"
else
  echo "$ROW: FAIL"
fi
exit "$rc"
