#!/usr/bin/env bash
# #2631 证据/回归测试：对账告警**升级去重**（H1）与「连续 2 天」**基准口径**（H2）+ 当日单号 marker（H3）。
#
# 全离线：fake gh（记录调用 / 可注入失败码）、fake reconcile（zero / diff3 / fail）、
# 沙箱 HOME + XDG_STATE_HOME。绝不发真实 Issue、绝不读真实库、绝不写仓库外状态。
#
# 用法：
#   bash scripts/tests/test_reconcile_user_cache_cron_2631.sh h1      # 同日升级评论去重
#   bash scripts/tests/test_reconcile_user_cache_cron_2631.sh h2      # 基准缺口措辞（GAP 分流）
#   bash scripts/tests/test_reconcile_user_cache_cron_2631.sh h2b     # 失败/损坏归档不作基准 + 无基准不升级
#   bash scripts/tests/test_reconcile_user_cache_cron_2631.sh h3      # 当日单号本地 marker（搜索双保险）
#   bash scripts/tests/test_reconcile_user_cache_cron_2631.sh guard   # dry-run / 零差异零 marker
#   bash scripts/tests/test_reconcile_user_cache_cron_2631.sh regress # #2625 夹具回归 + 断言/日期隔离守门
#   bash scripts/tests/test_reconcile_user_cache_cron_2631.sh docs    # 文档口径同步
set -uo pipefail

REPO="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$REPO" || exit 2

ISSUE=2631
RUNNER="scripts/reconcile-user-cache-cron.sh"
HARNESS_2625="scripts/tests/test_reconcile_user_cache_cron_2625.sh"
DOC="docs/ops/user-cache-reconcile.md"
BASE_REF="${BASE_REF:-origin/issue-2625}"
ROW="${1:-}"

# ---------------------------------------------------------------- provenance
HEAD_SHA="$(git rev-parse HEAD)"
FIX_REF="${FIX_COMMIT:-$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)}"
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}" 2>/dev/null || git rev-parse HEAD)"
if ! git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" 2>/dev/null; then
  echo "FATAL: FIX_COMMIT is not an ancestor of HEAD" >&2
  exit 3
fi
FIX_SHORT="$(git rev-parse --short=8 "$FIX_SHA")"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)docs/evidence/$ISSUE" ':(exclude)tmp')" ]; then
  DIRTY=true
fi
echo "# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
echo "GENERATED_AT_HEAD=$HEAD_SHA"

# ------------------------------------------------------------------- helpers
TMP="$(mktemp -d "${TMPDIR:-$REPO/tmp}/reconcile-cron-$ISSUE.XXXXXX")" || exit 2
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT

FAILED=0
expect() { # name expected actual
  local name="$1" expected="$2" actual="$3"
  if [ "$expected" = "$actual" ]; then
    printf '%s=%s\n' "$name" "$actual"
  else
    printf '%s=MISMATCH(expected=%s actual=%s)\n' "$name" "$expected" "$actual"
    FAILED=1
  fi
}
tok() { # file name
  grep -E "^$2=" "$1" 2>/dev/null | head -n 1 | cut -d= -f2-
}
count_lines() { # file pattern -> 出现行数（无匹配为 0）
  grep -cE "$2" "$1" 2>/dev/null | tr -d ' ' || true
}
has() { # file pattern -> 1 if 命中, else 0（不受出现次数影响）
  if grep -qE "$2" "$1" 2>/dev/null; then echo 1; else echo 0; fi
}
finish() {
  if [ "$FAILED" = "0" ]; then
    echo "TEST_EXIT=0"
    echo "RESULT: PASS"
    exit 0
  fi
  echo "TEST_EXIT=1"
  echo "RESULT: FAIL"
  exit 1
}

mkdir -p "$TMP/fakebin" "$TMP/home" "$TMP/state"
SANDBOX_HOME="$TMP/home"
SANDBOX_STATE="$TMP/state"
GH_LOG="$TMP/gh.log"
GH_BODY_DIR="$TMP/gh-bodies"
mkdir -p "$GH_BODY_DIR"
: > "$GH_LOG"

cat > "$TMP/fakebin/gh" <<'FAKEGH'
#!/usr/bin/env bash
# fake gh：记录每次调用；list 结果由 FAKE_GH_LIST_JSON 控制，失败码由 FAKE_GH_LIST_RC 控制；
# create/comment 返回固定 URL，失败码由 FAKE_GH_CREATE_RC / FAKE_GH_COMMENT_RC 控制。
printf '%s\n' "$*" >> "$GH_LOG"
cmd="${1:-}"; sub="${2:-}"
case "$cmd $sub" in
  "issue list")
    if [ "${FAKE_GH_LIST_RC:-0}" != "0" ]; then
      echo "HTTP 500: search unavailable (fake)" >&2
      exit "${FAKE_GH_LIST_RC}"
    fi
    printf '%s\n' "${FAKE_GH_LIST_JSON:-[]}"
    exit 0
    ;;
  "issue create")
    body=""
    prev=""
    for a in "$@"; do
      [ "$prev" = "--body-file" ] && body="$a"
      prev="$a"
    done
    if [ -n "$body" ] && [ -d "$GH_BODY_DIR" ]; then
      cp "$body" "$GH_BODY_DIR/create-$(date +%s%N).md" 2>/dev/null || true
    fi
    if [ "${FAKE_GH_CREATE_RC:-0}" != "0" ]; then
      echo "HTTP 401: Bad credentials (fake)" >&2
      exit "${FAKE_GH_CREATE_RC}"
    fi
    printf 'https://github.com/fake/repo/issues/%s\n' "${FAKE_GH_CREATE_NUMBER:-777}"
    exit 0
    ;;
  "issue comment")
    body=""
    prev=""
    for a in "$@"; do
      [ "$prev" = "--body-file" ] && body="$a"
      prev="$a"
    done
    if [ -n "$body" ] && [ -d "$GH_BODY_DIR" ]; then
      cp "$body" "$GH_BODY_DIR/comment-$(date +%s%N).md" 2>/dev/null || true
    fi
    if [ "${FAKE_GH_COMMENT_RC:-0}" != "0" ]; then
      echo "HTTP 403: Resource not accessible (fake)" >&2
      exit "${FAKE_GH_COMMENT_RC}"
    fi
    exit 0
    ;;
esac
exit 0
FAKEGH
chmod +x "$TMP/fakebin/gh"

cat > "$TMP/fake-reconcile.sh" <<'FAKERECON'
#!/usr/bin/env bash
# fake 对账：按 FAKE_RECONCILE_MODE 输出 JSON token 或失败。
mode="${FAKE_RECONCILE_MODE:-zero}"
case "$mode" in
  fail)
    echo "RESULT: FAIL (fake unreachable db)" >&2
    echo "RESULT: FAIL (fake unreachable db)"
    exit 2
    ;;
esac
zero_tokens='{"RECONCILE_DATE":"fixture","RECONCILE_SOURCE":"fixture","USERS_LOCAL":"10","USERS_IAM_ACTIVE":"10","TENANT_MISMATCH":"0","ORG_MISMATCH":"0","NIL_UUID":"0","MISSING_LOCAL":"0","ORPHAN_LOCAL":"0","ROLE_MISMATCH":"0","ROLE_UNVERIFIED":"0","CONTEXT_AMBIGUOUS":"0","MEMBER_ONLY":"0","IAM_MISSING":"0","ORG_UNRESOLVED":"0","ROLE_REPAIR_SUPPORTED":"0","DB_WRITES":"0"}'
diff_tokens='{"RECONCILE_DATE":"fixture","RECONCILE_SOURCE":"fixture","USERS_LOCAL":"10","USERS_IAM_ACTIVE":"10","TENANT_MISMATCH":"2","ORG_MISMATCH":"1","NIL_UUID":"0","MISSING_LOCAL":"0","ORPHAN_LOCAL":"0","ROLE_MISMATCH":"0","ROLE_UNVERIFIED":"0","CONTEXT_AMBIGUOUS":"0","MEMBER_ONLY":"0","IAM_MISSING":"0","ORG_UNRESOLVED":"0","ROLE_REPAIR_SUPPORTED":"0","DB_WRITES":"0"}'
case "$mode" in
  zero)
    jq -n --argjson t "$zero_tokens" '{tokens:$t, rows:[]}'
    ;;
  diff3)
    jq -n --argjson t "$diff_tokens" '{tokens:$t, rows:[
      {iam_sub:"u-a",category:"TENANT_MISMATCH",local:"tenant=P",iam:"tenant=R",email:"a@x"},
      {iam_sub:"u-b",category:"TENANT_MISMATCH",local:"tenant=P",iam:"tenant=R",email:"b@x"},
      {iam_sub:"u-c",category:"ORG_MISMATCH",local:"org=X",iam:"org=Y",email:"c@x"}]}'
    ;;
  *)
    echo "unknown fake mode" >&2
    exit 2
    ;;
esac
FAKERECON
chmod +x "$TMP/fake-reconcile.sh"

run_runner() { # outfile args...
  local out="$1"; shift
  HOME="$SANDBOX_HOME" XDG_STATE_HOME="$SANDBOX_STATE" \
  PATH="$TMP/fakebin:$PATH" GH_LOG="$GH_LOG" GH_BODY_DIR="$GH_BODY_DIR" \
  GH_BIN="$TMP/fakebin/gh" RECONCILE_BIN="$TMP/fake-reconcile.sh" \
  BEACONIAM_DB_NAME="beaconiam_x" TUNELOOP_DB_NAME="tuneloop_x" \
  FAKE_RECONCILE_MODE="${FAKE_RECONCILE_MODE:-zero}" \
  FAKE_GH_LIST_JSON="${FAKE_GH_LIST_JSON:-[]}" \
  FAKE_GH_LIST_RC="${FAKE_GH_LIST_RC:-0}" \
  FAKE_GH_CREATE_NUMBER="${FAKE_GH_CREATE_NUMBER:-777}" \
  FAKE_GH_CREATE_RC="${FAKE_GH_CREATE_RC:-0}" \
  FAKE_GH_COMMENT_RC="${FAKE_GH_COMMENT_RC:-0}" \
  bash "$RUNNER" "$@" > "$out" 2>&1
  printf '%s' "$?"
}

# 归档夹具：{tokens:{TENANT_MISMATCH,ORG_MISMATCH,...}}
seed_archive() { # path tenant_mismatch org_mismatch [extra-json]
  local path="$1" tenant="$2" org="$3" extra="${4:-}"
  printf '{"tokens":{"RECONCILE_DATE":"fixture","RECONCILE_SOURCE":"fixture","USERS_LOCAL":"10","USERS_IAM_ACTIVE":"10","TENANT_MISMATCH":"%s","ORG_MISMATCH":"%s","NIL_UUID":"0","MISSING_LOCAL":"0","ORPHAN_LOCAL":"0","ROLE_MISMATCH":"0","ROLE_UNVERIFIED":"0","CONTEXT_AMBIGUOUS":"0","MEMBER_ONLY":"0","IAM_MISSING":"0","ORG_UNRESOLVED":"0","ROLE_REPAIR_SUPPORTED":"0","DB_WRITES":"0"},"rows":[]%s}\n' \
    "$tenant" "$org" "$extra" > "$path"
}

marker_count() { # archive_dir date-prefix -> 当日 marker 文件数
  find "$1" -maxdepth 1 -type f -name ".$2.*" 2>/dev/null | wc -l | tr -d ' '
}

ARCHIVE_DIR="$SANDBOX_STATE/tuneloop/reconcile"

case "$ROW" in
h1)
  # 同日两次有差异运行（基准 = 昨日，同类差异同值）→ 只允许 1 条升级评论。
  mkdir -p "$ARCHIVE_DIR"
  seed_archive "$ARCHIVE_DIR/2027-01-05.json" 2 1
  : > "$GH_LOG"; rm -f "$GH_BODY_DIR"/*.md

  R1_OUT="$TMP/h1-run1.txt"
  R1_RC="$(FAKE_RECONCILE_MODE=diff3 run_runner "$R1_OUT" --date 2027-01-06)"
  sed 's/^/  /' "$R1_OUT"
  expect H1_RUN1_EXIT 0 "$R1_RC"
  expect H1_RUN1_ESCALATED 1 "$(tok "$R1_OUT" ESCALATED)"
  expect H1_RUN1_ESCALATE_COMMENTS 1 "$(tok "$R1_OUT" ESCALATE_COMMENTS)"
  expect H1_RUN1_BASELINE 2027-01-05.json "$(tok "$R1_OUT" ESCALATE_BASELINE)"
  expect H1_RUN1_GAP 0 "$(tok "$R1_OUT" ESCALATE_BASELINE_GAP)"
  expect H1_RUN1_ISSUE 777 "$(tok "$R1_OUT" ISSUE)"
  expect H1_RUN1_ISSUE_SOURCE create "$(tok "$R1_OUT" ISSUE_SOURCE)"
  expect H1_RUN1_ESCALATE_MARKER .2027-01-06.escalated "$(tok "$R1_OUT" ESCALATE_MARKER)"
  expect H1_RUN1_ESCALATE_MARKER_EXISTS 1 "$([ -f "$ARCHIVE_DIR/.2027-01-06.escalated" ] && echo 1 || echo 0)"

  R2_OUT="$TMP/h1-run2.txt"
  R2_RC="$(FAKE_RECONCILE_MODE=diff3 run_runner "$R2_OUT" --date 2027-01-06)"
  sed 's/^/  /' "$R2_OUT"
  expect H1_RUN2_EXIT 0 "$R2_RC"
  expect H1_RUN2_ESCALATE_SKIPPED already-escalated "$(tok "$R2_OUT" ESCALATE_SKIPPED)"
  expect H1_RUN2_ESCALATED 0 "$(tok "$R2_OUT" ESCALATED)"
  expect H1_RUN2_ESCALATE_COMMENTS 0 "$(tok "$R2_OUT" ESCALATE_COMMENTS)"
  expect H1_RUN2_GH_CALLS 0 "$(tok "$R2_OUT" GH_CALLS)"
  expect H1_RUN2_ISSUE 777 "$(tok "$R2_OUT" ISSUE)"
  expect H1_RUN2_ISSUE_SOURCE local-marker "$(tok "$R2_OUT" ISSUE_SOURCE)"

  expect H1_COMMENT_CALLS_TOTAL 1 "$(count_lines "$GH_LOG" '^issue comment')"
  expect H1_COMMENT_BODIES 1 "$(ls -1 "$GH_BODY_DIR"/comment-*.md 2>/dev/null | wc -l | tr -d ' ')"
  # 负例守门：同日重复升级评论必须为 0（≥2 即 FAIL）
  expect H1_NEGATIVE_DUPLICATE_COMMENTS 0 "$( [ "$(count_lines "$GH_LOG" '^issue comment')" -le 1 ] && echo 0 || echo 1)"
  finish
  ;;

h2)
  # 基准非昨日（缺口 3 天）→ 措辞「较上次归档未下降」；基准 = 昨日 → 保留「连续 2 天」。
  mkdir -p "$ARCHIVE_DIR"
  seed_archive "$ARCHIVE_DIR/2027-01-10.json" 2 1
  : > "$GH_LOG"; rm -f "$GH_BODY_DIR"/*.md

  G_OUT="$TMP/h2-gap.txt"
  G_RC="$(FAKE_RECONCILE_MODE=diff3 run_runner "$G_OUT" --date 2027-01-13)"
  sed 's/^/  /' "$G_OUT"
  expect H2_EXIT 0 "$G_RC"
  expect H2_GAP 1 "$(tok "$G_OUT" ESCALATE_BASELINE_GAP)"
  expect H2_BASELINE 2027-01-10.json "$(tok "$G_OUT" ESCALATE_BASELINE)"
  expect H2_DAYS 3 "$(tok "$G_OUT" ESCALATE_BASELINE_DAYS)"
  expect H2_ESCALATED 1 "$(tok "$G_OUT" ESCALATED)"
  GAP_BODY="$(ls -1 "$GH_BODY_DIR"/comment-*.md 2>/dev/null | head -n 1)"
  expect H2_GAP_BODY_PRESENT 1 "$([ -n "$GAP_BODY" ] && echo 1 || echo 0)"
  expect H2_GAP_BODY_HAS_GAP_TITLE 1 "$(count_lines "$GAP_BODY" '较上次归档未下降（升级提醒）')"
  expect H2_GAP_BODY_HAS_INTERVAL 1 "$(count_lines "$GAP_BODY" '基准间隔：3 天')"
  expect H2_GAP_BODY_CONTIGUOUS_CLAIMS 0 "$(count_lines "$GAP_BODY" '连续 2 天')"

  seed_archive "$ARCHIVE_DIR/2027-01-20.json" 2 1
  : > "$GH_LOG"; rm -f "$GH_BODY_DIR"/*.md
  C_OUT="$TMP/h2-contiguous.txt"
  C_RC="$(FAKE_RECONCILE_MODE=diff3 run_runner "$C_OUT" --date 2027-01-21)"
  sed 's/^/  /' "$C_OUT"
  expect H2_CONTIGUOUS_EXIT 0 "$C_RC"
  expect H2_CONTIGUOUS_GAP 0 "$(tok "$C_OUT" ESCALATE_BASELINE_GAP)"
  expect H2_CONTIGUOUS_BASELINE 2027-01-20.json "$(tok "$C_OUT" ESCALATE_BASELINE)"
  expect H2_CONTIGUOUS_DAYS 1 "$(tok "$C_OUT" ESCALATE_BASELINE_DAYS)"
  CONT_BODY="$(ls -1 "$GH_BODY_DIR"/comment-*.md 2>/dev/null | head -n 1)"
  expect H2_CONTIGUOUS_BODY_PRESENT 1 "$([ -n "$CONT_BODY" ] && echo 1 || echo 0)"
  expect H2_CONTIGUOUS_BODY_HAS_CONTIGUOUS_TITLE 1 "$(count_lines "$CONT_BODY" '连续 2 天未下降（升级提醒）')"
  expect H2_CONTIGUOUS_BODY_GAP_CLAIMS 0 "$(count_lines "$CONT_BODY" '较上次归档')"
  finish
  ;;

h2b)
  # 失败归档（failed:true）不作基准；无任何有效基准时不升级但仍建差异单。
  mkdir -p "$ARCHIVE_DIR"
  seed_archive "$ARCHIVE_DIR/2027-01-15.json" 2 1
  printf '{"date":"2027-01-16","failed":true,"reason":"reconcile exit 2"}\n' > "$ARCHIVE_DIR/2027-01-16.json"
  : > "$GH_LOG"; rm -f "$GH_BODY_DIR"/*.md

  B_OUT="$TMP/h2b-failed-baseline.txt"
  B_RC="$(FAKE_RECONCILE_MODE=diff3 run_runner "$B_OUT" --date 2027-01-17)"
  sed 's/^/  /' "$B_OUT"
  expect H2B_EXIT 0 "$B_RC"
  expect H2B_BASELINE 2027-01-15.json "$(tok "$B_OUT" ESCALATE_BASELINE)"
  expect H2B_DAYS 2 "$(tok "$B_OUT" ESCALATE_BASELINE_DAYS)"
  expect H2B_GAP 1 "$(tok "$B_OUT" ESCALATE_BASELINE_GAP)"
  expect H2B_FAILED_BASELINE_USED 0 "$( [ "$(tok "$B_OUT" ESCALATE_BASELINE)" = "2027-01-16.json" ] && echo 1 || echo 0)"
  expect H2B_ESCALATED 1 "$(tok "$B_OUT" ESCALATED)"

  # 早于所有预置归档 → 无有效基准：不升级，但差异单仍要建（告警不丢）。
  : > "$GH_LOG"
  N_OUT="$TMP/h2b-no-baseline.txt"
  N_RC="$(FAKE_RECONCILE_MODE=diff3 run_runner "$N_OUT" --date 2027-01-02)"
  sed 's/^/  /' "$N_OUT"
  expect H2B_NO_BASELINE_EXIT 0 "$N_RC"
  expect H2B_NO_BASELINE 1 "$( [ "$(tok "$N_OUT" ESCALATE_BASELINE)" = "none" ] && echo 1 || echo 0)"
  expect H2B_NO_BASELINE_GAP 1 "$(tok "$N_OUT" ESCALATE_BASELINE_GAP)"
  expect H2B_NO_BASELINE_ESCALATED 0 "$(tok "$N_OUT" ESCALATED)"
  expect H2B_NO_BASELINE_COMMENTS 0 "$(tok "$N_OUT" ESCALATE_COMMENTS)"
  expect H2B_NO_BASELINE_ISSUE_CREATED 1 "$(count_lines "$GH_LOG" '^issue create')"
  finish
  ;;

h3)
  # 搜索命中 → 写 marker；marker 命中 → 跳过 gh；搜索失败 → 降级建单并写 marker；
  # 建单失败 → 不写 marker（下轮重试）。
  mkdir -p "$ARCHIVE_DIR"
  seed_archive "$ARCHIVE_DIR/2027-01-31.json" 0 0

  # 1) 搜索命中 811：不建单，写 marker
  : > "$GH_LOG"; rm -f "$GH_BODY_DIR"/*.md
  S_OUT="$TMP/h3-search.txt"
  S_RC="$(FAKE_RECONCILE_MODE=diff3 FAKE_GH_LIST_JSON='[{"number":811}]' FAKE_GH_CREATE_NUMBER=999 run_runner "$S_OUT" --date 2027-02-01)"
  sed 's/^/  /' "$S_OUT"
  expect H3_SEARCH_EXIT 0 "$S_RC"
  expect H3_SEARCH_ISSUE 811 "$(tok "$S_OUT" ISSUE)"
  expect H3_SEARCH_SOURCE search "$(tok "$S_OUT" ISSUE_SOURCE)"
  expect H3_SEARCH_CREATES 0 "$(count_lines "$GH_LOG" '^issue create')"
  expect H3_SEARCH_MARKER_EXISTS 1 "$([ -f "$ARCHIVE_DIR/.2027-02-01.issue" ] && echo 1 || echo 0)"
  expect H3_SEARCH_MARKER_ISSUE 811 "$(head -n 1 "$ARCHIVE_DIR/.2027-02-01.issue" 2>/dev/null)"

  # 2) marker 命中（搜索此时不可用）：GH_CALLS=0，且完全不调用 gh issue list
  : > "$GH_LOG"
  M_OUT="$TMP/h3-marker.txt"
  M_RC="$(FAKE_RECONCILE_MODE=diff3 FAKE_GH_LIST_RC=1 run_runner "$M_OUT" --date 2027-02-01)"
  sed 's/^/  /' "$M_OUT"
  expect H3_MARKER_EXIT 0 "$M_RC"
  expect H3_MARKER_ISSUE 811 "$(tok "$M_OUT" ISSUE)"
  expect H3_MARKER_ISSUE_MARKER 811 "$(tok "$M_OUT" ISSUE_MARKER)"
  expect H3_MARKER_SOURCE local-marker "$(tok "$M_OUT" ISSUE_SOURCE)"
  expect H3_MARKER_GH_CALLS 0 "$(tok "$M_OUT" GH_CALLS)"
  expect H3_MARKER_LIST_CALLS 0 "$(count_lines "$GH_LOG" '^issue list')"

  # 3) 搜索失败 → 降级建单 812 并写 marker；重跑复用 marker
  : > "$GH_LOG"; rm -f "$GH_BODY_DIR"/*.md
  D_OUT="$TMP/h3-degrade.txt"
  D_RC="$(FAKE_RECONCILE_MODE=diff3 FAKE_GH_LIST_RC=1 FAKE_GH_CREATE_NUMBER=812 run_runner "$D_OUT" --date 2027-02-02)"
  sed 's/^/  /' "$D_OUT"
  expect H3_DEGRADE_EXIT 0 "$D_RC"
  expect H3_DEGRADE_ISSUE 812 "$(tok "$D_OUT" ISSUE)"
  expect H3_DEGRADE_LIST_FAILED 1 "$(tok "$D_OUT" GH_LIST_FAILED)"
  expect H3_DEGRADE_CREATES 1 "$(count_lines "$GH_LOG" '^issue create')"
  expect H3_DEGRADE_MARKER_EXISTS 1 "$([ -f "$ARCHIVE_DIR/.2027-02-02.issue" ] && echo 1 || echo 0)"
  : > "$GH_LOG"
  DR_OUT="$TMP/h3-degrade-rerun.txt"
  DR_RC="$(FAKE_RECONCILE_MODE=diff3 FAKE_GH_LIST_RC=1 run_runner "$DR_OUT" --date 2027-02-02)"
  sed 's/^/  /' "$DR_OUT"
  expect H3_DEGRADE_RERUN_EXIT 0 "$DR_RC"
  expect H3_DEGRADE_RERUN_ISSUE 812 "$(tok "$DR_OUT" ISSUE)"
  expect H3_DEGRADE_RERUN_CREATES 0 "$(count_lines "$GH_LOG" '^issue create')"

  # 4) 搜索失败 + 建单失败 → 不写 marker，下轮仍会重试建单
  : > "$GH_LOG"
  F_OUT="$TMP/h3-create-fail.txt"
  F_RC="$(FAKE_RECONCILE_MODE=diff3 FAKE_GH_LIST_RC=1 FAKE_GH_CREATE_RC=1 run_runner "$F_OUT" --date 2027-02-03)"
  sed 's/^/  /' "$F_OUT"
  expect H3_FAIL_EXIT 2 "$F_RC"
  expect H3_FAIL_ISSUE none "$(tok "$F_OUT" ISSUE)"
  expect H3_FAIL_CREATE_FAILED 1 "$(tok "$F_OUT" ISSUE_CREATE_FAILED)"
  expect H3_FAIL_MARKER_EXISTS 0 "$([ -f "$ARCHIVE_DIR/.2027-02-03.issue" ] && echo 1 || echo 0)"
  FR_OUT="$TMP/h3-create-retry.txt"
  FR_RC="$(FAKE_RECONCILE_MODE=diff3 FAKE_GH_LIST_RC=1 FAKE_GH_CREATE_NUMBER=813 run_runner "$FR_OUT" --date 2027-02-03)"
  sed 's/^/  /' "$FR_OUT"
  expect H3_FAIL_RETRY_EXIT 0 "$FR_RC"
  expect H3_FAIL_RETRY_ISSUE 813 "$(tok "$FR_OUT" ISSUE)"
  expect H3_FAIL_RETRY_CREATES 2 "$(count_lines "$GH_LOG" '^issue create')"
  expect H3_FAIL_RETRY_MARKER_EXISTS 1 "$([ -f "$ARCHIVE_DIR/.2027-02-03.issue" ] && echo 1 || echo 0)"
  finish
  ;;

guard)
  # dry-run 零写入（含 marker、含 gh 调用）；零差异实跑只写 2 个归档、0 个 marker。
  mkdir -p "$ARCHIVE_DIR"
  seed_archive "$ARCHIVE_DIR/2027-02-28.json" 2 1
  : > "$GH_LOG"

  DRY_OUT="$TMP/guard-dry-run.txt"
  DRY_RC="$(FAKE_RECONCILE_MODE=diff3 run_runner "$DRY_OUT" --date 2027-03-01 --dry-run)"
  sed 's/^/  /' "$DRY_OUT"
  expect GUARD_DRY_RUN_EXIT 0 "$DRY_RC"
  expect GUARD_DRY_RUN_FILES_WRITTEN 0 "$(tok "$DRY_OUT" FILES_WRITTEN)"
  expect GUARD_DRY_RUN_GH_CALLS 0 "$(tok "$DRY_OUT" GH_CALLS)"
  expect GUARD_DRY_RUN_MARKERS 0 "$(marker_count "$ARCHIVE_DIR" 2027-03-01)"
  expect GUARD_DRY_RUN_ARCHIVE_ABSENT 1 "$([ ! -f "$ARCHIVE_DIR/2027-03-01.json" ] && echo 1 || echo 0)"

  ZERO_OUT="$TMP/guard-zero.txt"
  ZERO_RC="$(FAKE_RECONCILE_MODE=zero run_runner "$ZERO_OUT" --date 2027-03-02)"
  sed 's/^/  /' "$ZERO_OUT"
  expect GUARD_ZERO_EXIT 0 "$ZERO_RC"
  expect GUARD_ZERO_GH_CALLS 0 "$(tok "$ZERO_OUT" GH_CALLS)"
  expect GUARD_ZERO_MARKERS 0 "$(marker_count "$ARCHIVE_DIR" 2027-03-02)"
  expect GUARD_ZERO_ARCHIVES 2 "$(find "$ARCHIVE_DIR" -maxdepth 1 -type f -name '2027-03-02.*' 2>/dev/null | wc -l | tr -d ' ')"
  finish
  ;;

regress)
  # #2625 夹具回归：三条用例全绿 + 断言计数不低于基线 + 仅日期/注释行隔离。
  for r in r1 r2 r3; do
    LOG="$TMP/regress-$r.log"
    bash "$HARNESS_2625" "$r" > "$LOG" 2>&1
    RC=$?
    # 缩进后打印，避免子夹具的 RESULT/TEST_EXIT 与本体断言混淆。
    sed 's/^/  /' "$LOG"
    case "$r" in
      r1) expect R2625_R1_TEST_EXIT 0 "$RC" ;;
      r2) expect R2625_R2_TEST_EXIT 0 "$RC" ;;
      r3) expect R2625_R3_TEST_EXIT 0 "$RC" ;;
    esac
  done

  BASE_HARNESS="$TMP/base-2625-harness.sh"
  BASE_OK=0
  if git show "$BASE_REF:$HARNESS_2625" > "$BASE_HARNESS" 2>/dev/null && [ -s "$BASE_HARNESS" ]; then
    BASE_OK=1
  fi
  echo "HARNESS_BASE_REF=$BASE_REF"
  expect HARNESS_BASE_PRESENT 1 "$BASE_OK"

  CUR_ASSERTS="$(count_lines "$HARNESS_2625" '^[[:space:]]*(expect|at_least) ')"
  BASE_ASSERTS=0
  [ "$BASE_OK" = "1" ] && BASE_ASSERTS="$(count_lines "$BASE_HARNESS" '^[[:space:]]*(expect|at_least) ')"
  echo "HARNESS_CUR_ASSERTS=$CUR_ASSERTS"
  echo "HARNESS_BASE_ASSERTS=$BASE_ASSERTS"
  PARITY=0
  if [ "$BASE_OK" = "1" ] && [ "${CUR_ASSERTS:-0}" -ge "${BASE_ASSERTS:-0}" ]; then PARITY=1; fi
  expect HARNESS_ASSERT_PARITY 1 "$PARITY"

  # 非日期行 diff：去掉日期差异、允许新增注释行，其余任何差异 = 放宽/改写既有断言。
  : > "$TMP/harness.diff"
  if [ "$BASE_OK" = "1" ]; then
    git diff --no-index --unified=0 "$BASE_HARNESS" "$HARNESS_2625" > "$TMP/harness.diff" 2>/dev/null || true
  fi
  grep -E '^-[^-]' "$TMP/harness.diff" 2>/dev/null | sed 's/^-//' \
    | sed -E 's/[0-9]{4}-[0-9]{2}-[0-9]{2}/DATE/g' | LC_ALL=C sort > "$TMP/harness-removed.txt"
  grep -E '^\+[^+]' "$TMP/harness.diff" 2>/dev/null | sed 's/^+//' \
    | sed -E 's/[0-9]{4}-[0-9]{2}-[0-9]{2}/DATE/g' | LC_ALL=C sort > "$TMP/harness-added.txt"
  comm -3 "$TMP/harness-removed.txt" "$TMP/harness-added.txt" 2>/dev/null | sed 's/^\t//' > "$TMP/harness-unmatched.txt"
  NON_DATE_LINES="$(grep -vE '^[[:space:]]*#|^[[:space:]]*$' "$TMP/harness-unmatched.txt" 2>/dev/null | wc -l | tr -d ' ')"
  echo "HARNESS_UNMATCHED_LINES:"
  sed 's/^/  /' "$TMP/harness-unmatched.txt"
  expect HARNESS_DIFF_NON_DATE_LINES 0 "${NON_DATE_LINES:-0}"
  finish
  ;;

docs)
  expect DOC_PRESENT 1 "$([ -f "$DOC" ] && echo 1 || echo 0)"
  expect DOC_GAP_WORDING 1 "$(has "$DOC" '较上次归档未下降')"
  expect DOC_CONTIGUOUS_WORDING 1 "$(has "$DOC" '连续 2 天')"
  expect DOC_GAP_TOKEN 1 "$(has "$DOC" 'ESCALATE_BASELINE_GAP')"
  expect DOC_BASELINE_TOKEN 1 "$(has "$DOC" 'ESCALATE_BASELINE=')"
  expect DOC_ESCALATE_MARKER 1 "$(has "$DOC" '\.escalated')"
  expect DOC_ISSUE_MARKER 1 "$(has "$DOC" '\.issue')"
  expect DOC_ALREADY_ESCALATED 1 "$(has "$DOC" 'already-escalated')"
  expect DOC_FAILED_ARCHIVE_SKIP 1 "$(has "$DOC" 'failed:true')"
  # 旧的无条件「不低于昨日」表述必须已移除：出现次数为 0 时 GONE=1
  expect DOC_OLD_UNCONDITIONAL_ROW_GONE 1 "$([ "$(count_lines "$DOC" '不低于昨日')" = "0" ] && echo 1 || echo 0)"
  finish
  ;;

*)
  echo "usage: $0 <h1|h2|h2b|h3|guard|regress|docs>" >&2
  exit 2
  ;;
esac
