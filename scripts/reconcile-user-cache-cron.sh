#!/usr/bin/env bash
# #2625/#2631 用户缓存对账运行器（薄壳）：调用 #2623 的只读对账，归档报告并按差异/失败分流告警。
#
# 用法：
#   bash scripts/reconcile-user-cache-cron.sh [--date YYYY-MM-DD] [--dry-run]
#
# 分支（逐条契约，见 #2625 / #2631 计划）：
#   无差异      → 归档，不建 Issue、不发通知（GH_CALLS=0）
#   有差异      → 归档 + 建 `[Ops] 用户缓存对账差异 <date>`（标签仅 waitForConfirm）；当日去重
#                  （本地 marker 优先，搜索兜底）
#   同类差异未降 → 在既有单追加**恰好 1 条**升级评论（不新建第二单）；当日只升级 1 次
#   对账失败    → 归档失败原因 + 建 `[Ops] 用户缓存对账失败 <date>`，本脚本 exit 2
#   参数/环境缺失 → exit 2 并打印 MISSING_ENV=<name>（禁止静默成功）
#
# 升级基准口径（#2631 H2）：基准 = 「最近一份更早的**有效**归档」（跳过 failed:true 与损坏归档）。
#   基准日期 = 昨日 → `连续 2 天未下降`（ESCALATE_BASELINE_GAP=0）；基准非昨日 →
#   `较上次归档未下降`（GAP=1，不冒充连续）；无任何有效基准 → 不升级。
#
# 当日 marker（#2631 H1/H3）：${ARCHIVE_DIR}/.<date>.escalated / .<date>.issue /
#   .<date>.failure-issue；只在成功路径写入，失败不写以保证下轮重试。
#
# 永不调用 --apply（判定与修复属于 #2623/#2624），定时任务只读；判定逻辑不在此复制，只解析 tokens/rows。
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RECONCILE_BIN="${RECONCILE_BIN:-$REPO/scripts/reconcile-user-cache.sh}"
GH_BIN="${GH_BIN:-gh}"
STATE_ROOT="${XDG_STATE_HOME:-$HOME/.local/state}/tuneloop"
ARCHIVE_DIR="$STATE_ROOT/reconcile"
ENV_FILE="$HOME/.config/tuneloop/reconcile.env"

RECONCILE_DATE="$(date +%F)"
DRY_RUN=0

while [ $# -gt 0 ]; do
  case "$1" in
    --date) RECONCILE_DATE="${2:-}"; shift 2 ;;
    --dry-run) DRY_RUN=1; shift ;;
    -h|--help) sed -n '2,21p' "$0"; exit 0 ;;
    *) echo "MISSING_ENV=argument"; echo "RESULT: FAIL (unknown argument: $1)"; exit 2 ;;
  esac
done
if ! printf '%s' "$RECONCILE_DATE" | grep -qE '^[0-9]{4}-[0-9]{2}-[0-9]{2}$'; then
  echo "MISSING_ENV=date"
  echo "RESULT: FAIL (--date must be YYYY-MM-DD)"
  exit 2
fi

FILES_WRITTEN=0
ISSUE="none"
FAILURE_ISSUE="none"
ESCALATED=0
ESCALATE_COMMENTS=0
# 告警送达失败必须显式（红线：外部调用失败不得静默吞掉）。
# 这些 token 用来区分「告警已送达」与「告警没送到」，运维可据此判断告警链是否健全。
ISSUE_CREATE_FAILED=0
ESCALATE_FAILED=0
FAILURE_ISSUE_CREATE_FAILED=0
GH_LIST_FAILED=0
DIFFS=0
ARCHIVE_JSON=""
ARCHIVE_TEXT=""
LAST_SUMMARY=""
# #2631：当日 marker 与升级基准口径的可观测 token（默认值 = 「未解析/无基准」）。
ISSUE_SOURCE=""
ISSUE_MARKER="none"
FAILURE_ISSUE_SOURCE=""
ESCALATE_BASELINE="none"
ESCALATE_BASELINE_DAYS="unknown"
ESCALATE_BASELINE_GAP="1"
ESCALATE_SKIPPED="none"
ESCALATE_MARKER="none"

# 凭据由 EnvironmentFile 提供（unit 只写路径）；此处只在存在时加载，随后校验必需项。
if [ -f "$ENV_FILE" ]; then
  # shellcheck disable=SC1090
  . "$ENV_FILE" >/dev/null 2>&1 || true
fi

missing_env() { # name
  echo "MISSING_ENV=$1"
  echo "RESULT: FAIL (missing $1)"
  exit 2
}

echo "RECONCILE_DATE=$RECONCILE_DATE"
command -v "$GH_BIN" >/dev/null 2>&1 || [ -x "$GH_BIN" ] || missing_env GH_BIN
[ -x "$RECONCILE_BIN" ] || [ -f "$RECONCILE_BIN" ] || missing_env RECONCILE_BIN
[ -n "${BEACONIAM_DB_NAME:-${BEACONIAM_DB:-}}" ] || missing_env BEACONIAM_DB_NAME
[ -n "${TUNELOOP_DB_NAME:-${TUNELOOP_DB:-}}" ] || missing_env TUNELOOP_DB_NAME

# --dry-run 零写入：连归档目录都不创建（否则「只打印」的契约不成立）。
if [ "$DRY_RUN" = "1" ]; then
  :
else
  mkdir -p "$ARCHIVE_DIR" 2>/dev/null || { echo "RESULT: FAIL (cannot create archive dir: $ARCHIVE_DIR)"; exit 2; }
fi

# GH 调用计数落文件：gh_call 常在 $( ) 子 shell 里执行，变量自增不会传回父 shell。
GH_CALLS_FILE="$(mktemp "${TMPDIR:-/tmp}/gh-calls.XXXXXX")" || exit 2
gh_call() { # args... -> stdout, records one call
  printf 'x\n' >> "$GH_CALLS_FILE"
  "$GH_BIN" "$@"
}
gh_call_count() { wc -l < "$GH_CALLS_FILE" 2>/dev/null | tr -d ' '; }

# ---------------------------------------------------------------- 当日 marker
# #2631：本地 marker 是 GitHub 搜索之外的**当日去重真值**（搜索索引最终一致、
# 标题含 `[Ops]` 方括号等特殊字符都可能匹配失败）。marker 落在归档目录（不写仓库）。
# 语义（D5）：只在成功路径写入——建单/评论失败时**不写**，保证下轮重试。
marker_path() { # kind -> path
  case "$1" in
    escalated)     printf '%s/.%s.escalated' "$ARCHIVE_DIR" "$RECONCILE_DATE" ;;
    issue)         printf '%s/.%s.issue' "$ARCHIVE_DIR" "$RECONCILE_DATE" ;;
    failure-issue) printf '%s/.%s.failure-issue' "$ARCHIVE_DIR" "$RECONCILE_DATE" ;;
    *)             printf '%s/.%s.%s' "$ARCHIVE_DIR" "$RECONCILE_DATE" "$1" ;;
  esac
}

marker_exists() { [ -f "$1" ]; }

read_num_marker() { # path -> digits on stdout; 非纯数字视为不存在（不猜单号）
  local p="$1" v
  marker_exists "$p" || return 0
  v="$(head -n 1 "$p" 2>/dev/null | tr -d '[:space:]')"
  case "$v" in
    ''|*[!0-9]*) return 0 ;;
  esac
  printf '%s' "$v"
}

write_marker() { # kind line... -> rc 0; 写失败只降级并显式打印 MARKER_WRITE_FAILED=<kind>
  local kind="$1"; shift
  local p
  p="$(marker_path "$kind")"
  [ "$DRY_RUN" = "1" ] && return 0
  if ! mkdir -p "$ARCHIVE_DIR" 2>/dev/null; then
    echo "MARKER_WRITE_FAILED=$kind"
    return 1
  fi
  if ! printf '%s\n' "$@" > "$p" 2>/dev/null; then
    echo "MARKER_WRITE_FAILED=$kind"
    return 1
  fi
  return 0
}

baseline_days() { # prev_date today -> "<n>|unknown"（TZ=UTC 求差，避开 DST 抖动）
  local a b
  a="$(TZ=UTC date -d "$1" +%s 2>/dev/null)"
  b="$(TZ=UTC date -d "$2" +%s 2>/dev/null)"
  case "${a}${b}" in
    ''|*[!0-9]*) printf 'unknown'; return 0 ;;
  esac
  printf '%s' $(( (b - a) / 86400 ))
}

# 最近一份更早的**有效**归档：跳过 failed:true 与无法解析/无 tokens 的 JSON。
latest_valid_archive() {
  local cand
  [ -d "$ARCHIVE_DIR" ] || return 0
  while IFS= read -r cand; do
    [ -n "$cand" ] || continue
    if jq -e '.failed == true' "$cand" >/dev/null 2>&1; then continue; fi
    if ! jq -e '.tokens | type == "object"' "$cand" >/dev/null 2>&1; then continue; fi
    printf '%s' "$cand"
    return 0
  done < <(find "$ARCHIVE_DIR" -maxdepth 1 -name '*.json' -type f 2>/dev/null \
    | awk -F/ -v today="$RECONCILE_DATE" '{ d=$NF; sub(/\.json$/, "", d); if (d < today) print d"\t"$0 }' \
    | LC_ALL=C sort -r | cut -f2)
  return 0
}

issue_number_from() { # text (gh issue create prints URL) -> number
  printf '%s' "$1" | grep -oE '[0-9]+$' | head -n 1
}

find_issue_by_title() { # title-fragment -> number on stdout；rc 2 = gh list / jq 失败（调用方必须显式记账）
  local out num
  out="$(gh_call issue list --search "${1} in:title" --state all --json number 2>/dev/null)"
  [ "$?" = "0" ] || return 2
  num="$(printf '%s' "$out" | jq -r 'if type == "array" and length > 0 then (.[0].number | tostring) else "" end' 2>/dev/null)" || return 2
  printf '%s' "$num"
  return 0
}

write_archive() { # json_body text_body
  ARCHIVE_JSON="$ARCHIVE_DIR/$RECONCILE_DATE.json"
  ARCHIVE_TEXT="$ARCHIVE_DIR/$RECONCILE_DATE.txt"
  printf '%s\n' "$1" > "$ARCHIVE_JSON" || return 1
  printf '%s\n' "$2" > "$ARCHIVE_TEXT" || return 1
  FILES_WRITTEN=$((FILES_WRITTEN + 2))
  LAST_SUMMARY="$ARCHIVE_TEXT"
  return 0
}

emit_tokens() { # result
  echo "DIFFS=$DIFFS"
  echo "GH_CALLS=$(gh_call_count)"
  echo "ISSUE=$ISSUE"
  echo "ISSUE_SOURCE=$ISSUE_SOURCE"
  echo "ISSUE_MARKER=$ISSUE_MARKER"
  echo "ISSUE_CREATE_FAILED=$ISSUE_CREATE_FAILED"
  echo "ESCALATED=$ESCALATED"
  echo "ESCALATE_COMMENTS=$ESCALATE_COMMENTS"
  echo "ESCALATE_FAILED=$ESCALATE_FAILED"
  echo "ESCALATE_BASELINE=$ESCALATE_BASELINE"
  echo "ESCALATE_BASELINE_DAYS=$ESCALATE_BASELINE_DAYS"
  echo "ESCALATE_BASELINE_GAP=$ESCALATE_BASELINE_GAP"
  echo "ESCALATE_SKIPPED=$ESCALATE_SKIPPED"
  echo "ESCALATE_MARKER=$ESCALATE_MARKER"
  echo "FAILURE_ISSUE=$FAILURE_ISSUE"
  echo "FAILURE_ISSUE_SOURCE=$FAILURE_ISSUE_SOURCE"
  echo "FAILURE_ISSUE_CREATE_FAILED=$FAILURE_ISSUE_CREATE_FAILED"
  echo "GH_LIST_FAILED=$GH_LIST_FAILED"
  echo "ARCHIVE=$ARCHIVE_JSON"
  echo "LAST_SUMMARY=$LAST_SUMMARY"
  echo "DRY_RUN=$DRY_RUN"
  echo "FILES_WRITTEN=$FILES_WRITTEN"
  echo "RESULT: $1"
}

# ------------------------------------------------------------------ reconcile
RAWS_OUT="$(mktemp "${TMPDIR:-/tmp}/reconcile-json.XXXXXX")" || exit 2
ERR_OUT="$(mktemp "${TMPDIR:-/tmp}/reconcile-err.XXXXXX")" || exit 2
trap 'rm -f "$RAWS_OUT" "$ERR_OUT" "${GH_CALLS_FILE:-}"' EXIT

"$RECONCILE_BIN" --json --limit 20 > "$RAWS_OUT" 2> "$ERR_OUT"
RECON_RC=$?

handle_failure() { # reason
  local reason="$1" title body num
  title="[Ops] 用户缓存对账失败 $RECONCILE_DATE"
  body="$(mktemp "${TMPDIR:-/tmp}/reconcile-fail.XXXXXX")"
  {
    echo "## 对账失败（$RECONCILE_DATE）"
    echo ""
    echo "原因：$reason"
    echo ""
    echo "对账脚本退出码：$RECON_RC"
    echo ""
    echo '```'
    sed -n '1,40p' "$ERR_OUT" 2>/dev/null
    sed -n '1,40p' "$RAWS_OUT" 2>/dev/null
    echo '```'
    echo ""
    echo "本单仅等待人工处置回执（标签 waitForConfirm，无 status:*，不进入流水线）。"
    echo "判定与只读工具见 #2623；修复流程见 #2624（需 Owner 授权）。"
  } > "$body"
  if [ "$DRY_RUN" = "1" ]; then
    # 审计 B3：dry-run 必须零写入。此处曾调用 write_archive 覆盖同日既有真实归档，
    # 随后又把 FILES_WRITTEN 复位为 0（失真 token）；被覆盖的归档使次日「连续 2 天不降」
    # 比较基准读不到 token → 升级机制静默失效。dry-run 只打印动作，不落盘。
    echo "DRY_RUN=1"
    echo "DRY_RUN_ACTION=create-issue title=$title"
    rm -f "$body"
    FAILURE_ISSUE="would-create"
    ARCHIVE_JSON="(dry-run)"
    ARCHIVE_TEXT="(dry-run)"
    FILES_WRITTEN=0
    emit_tokens FAIL
    exit 2
  fi
  # #2631 H3：本地 marker 优先（搜索索引最终一致 + 标题含方括号都可能匹配失败）。
  FAIL_MARKER="$(marker_path failure-issue)"
  num="$(read_num_marker "$FAIL_MARKER")"
  if [ -n "$num" ]; then
    FAILURE_ISSUE_SOURCE="local-marker"
  else
    num="$(find_issue_by_title "$title")" || { GH_LIST_FAILED=1; num=""; }
    if [ -n "$num" ]; then
      FAILURE_ISSUE_SOURCE="search"
      write_marker failure-issue "$num" || true
    fi
  fi
  if [ -z "$num" ]; then
    CREATE_ERR="$(mktemp "${TMPDIR:-/tmp}/reconcile-fail-err.XXXXXX")"
    FAILURE_ISSUE_SOURCE="create"
    CREATE_OUT="$(gh_call issue create --title "$title" --body-file "$body" --label waitForConfirm 2>"$CREATE_ERR")"
    CREATE_RC=$?
    num="$(issue_number_from "$CREATE_OUT")"
    if [ "$CREATE_RC" != "0" ] || [ -z "$num" ]; then
      # 失败单都没建成：仍 exit 2，但必须让运维区分「已建失败单」与「告警也没送出」。
      FAILURE_ISSUE_CREATE_FAILED=1
      num=""
      echo "FAILURE_ISSUE_CREATE_RC=$CREATE_RC"
      sed -n '1,5p' "$CREATE_ERR" 2>/dev/null
    else
      # 成功才写 marker（失败写了会让次日「当天已建单」假象吞掉告警）。
      write_marker failure-issue "$num" || true
    fi
    rm -f "$CREATE_ERR"
  fi
  rm -f "$body"
  FAILURE_ISSUE="${num:-none}"
  write_archive "{\"date\":\"$RECONCILE_DATE\",\"failed\":true,\"reason\":\"$reason\"}" "RECONCILE_DATE=$RECONCILE_DATE
RESULT: FAIL ($reason)" >/dev/null 2>&1 || echo "ARCHIVE_WRITE_FAILED=1"
  emit_tokens FAIL
  exit 2
}

if [ "$RECON_RC" != "0" ]; then
  handle_failure "reconcile exit $RECON_RC"
fi
if ! jq -e '.tokens | type == "object"' "$RAWS_OUT" >/dev/null 2>&1; then
  handle_failure "reconcile output is not valid JSON with tokens"
fi

REQUIRED_TOKENS="USERS_LOCAL USERS_IAM_ACTIVE TENANT_MISMATCH ORG_MISMATCH NIL_UUID MISSING_LOCAL ORPHAN_LOCAL ROLE_MISMATCH ROLE_UNVERIFIED CONTEXT_AMBIGUOUS MEMBER_ONLY"
for t in $REQUIRED_TOKENS; do
  if [ -z "$(jq -r --arg k "$t" '.tokens[$k] // ""' "$RAWS_OUT")" ]; then
    handle_failure "reconcile output missing token $t"
  fi
done

DIFFS=$(( $(jq -r '.tokens.TENANT_MISMATCH // "0"' "$RAWS_OUT") \
        + $(jq -r '.tokens.ORG_MISMATCH // "0"' "$RAWS_OUT") \
        + $(jq -r '.tokens.NIL_UUID // "0"' "$RAWS_OUT") \
        + $(jq -r '.tokens.MISSING_LOCAL // "0"' "$RAWS_OUT") \
        + $(jq -r '.tokens.ORPHAN_LOCAL // "0"' "$RAWS_OUT") \
        + $(jq -r '.tokens.ROLE_MISMATCH // "0"' "$RAWS_OUT") ))

TEXT_REPORT="$(
  for t in RECONCILE_DATE RECONCILE_SOURCE USERS_LOCAL USERS_IAM_ACTIVE TENANT_MISMATCH ORG_MISMATCH \
           NIL_UUID MISSING_LOCAL ORPHAN_LOCAL ROLE_MISMATCH ROLE_UNVERIFIED CONTEXT_AMBIGUOUS \
           MEMBER_ONLY IAM_MISSING ORG_UNRESOLVED ROLE_REPAIR_SUPPORTED DB_WRITES; do
    printf '%s=%s\n' "$t" "$(jq -r --arg k "$t" '.tokens[$k] // ""' "$RAWS_OUT")"
  done
)"

if [ "$DIFFS" = "0" ]; then
  if [ "$DRY_RUN" = "1" ]; then
    echo "DRY_RUN=1"
    echo "DRY_RUN_ACTION=archive-only"
    ARCHIVE_JSON="(dry-run)"
    ARCHIVE_TEXT="(dry-run)"
    emit_tokens PASS
    exit 0
  fi
  write_archive "$(jq -c . "$RAWS_OUT")" "$TEXT_REPORT" || { echo "RESULT: FAIL (cannot write archive)"; exit 2; }
  emit_tokens PASS
  exit 0
fi

# ------------------------------------------------------------------ 有差异分支
TITLE="[Ops] 用户缓存对账差异 $RECONCILE_DATE"
BODY_FILE="$(mktemp "${TMPDIR:-/tmp}/reconcile-diff.XXXXXX")" || exit 2
{
  echo "## 用户缓存对账差异（$RECONCILE_DATE）"
  echo ""
  echo '```'
  printf '%s\n' "$TEXT_REPORT"
  echo '```'
  echo ""
  echo "### 差异明细（最多 20 条，按 iam_sub 升序）"
  echo ""
  jq -r '.rows[]? | "- iam_sub=\(.iam_sub) category=\(.category) local[\(.local)] iam[\(.iam)]"' "$RAWS_OUT" 2>/dev/null | head -n 20
  echo ""
  echo "### 人工处置（S2）"
  echo ""
  echo '- 判定口径与只读工具：见 #2623（`scripts/reconcile-user-cache.sh`）'
  echo '- 修复动作需 Owner 授权，流程见 #2624；本单只等待人工处置回执'
  echo '- 复核命令：`bash scripts/reconcile-user-cache.sh`（只读）'
  echo ""
  echo "本单标签仅 waitForConfirm（无 status:*，不进入流水线）。"
} > "$BODY_FILE"

if [ "$DRY_RUN" = "1" ]; then
  echo "DRY_RUN=1"
  echo "DRY_RUN_ACTION=create-or-reuse-issue title=$TITLE"
  echo "DRY_RUN_DIFFS=$DIFFS"
  rm -f "$BODY_FILE"
  ARCHIVE_JSON="(dry-run)"
  ARCHIVE_TEXT="(dry-run)"
  emit_tokens PASS
  exit 0
fi

# 归档先行：无论告警是否送达，对账报告都必须落盘（次日比较基准依赖它）。
write_archive "$(jq -c . "$RAWS_OUT")" "$TEXT_REPORT" || { echo "RESULT: FAIL (cannot write archive)"; exit 2; }

# #2631 H3：本地 marker 优先（命中即跳过 gh issue list，GH_CALLS=0），搜索兜底，再建单。
# marker 只在成功路径写入；写失败只降级去重（显式 MARKER_WRITE_FAILED=<kind>），不得静默。
ISSUE_MARKER_PATH="$(marker_path issue)"
EXISTING="$(read_num_marker "$ISSUE_MARKER_PATH")"
if [ -n "$EXISTING" ]; then
  ISSUE_SOURCE="local-marker"
  ISSUE="$EXISTING"
  ISSUE_MARKER="$EXISTING"
else
  EXISTING="$(find_issue_by_title "$TITLE")" || { GH_LIST_FAILED=1; EXISTING=""; }
  if [ -n "$EXISTING" ]; then
    ISSUE_SOURCE="search"
    ISSUE="$EXISTING"
    if write_marker issue "$EXISTING"; then ISSUE_MARKER="$EXISTING"; fi
  else
    CREATE_ERR="$(mktemp "${TMPDIR:-/tmp}/reconcile-diff-err.XXXXXX")" || exit 2
    ISSUE_SOURCE="create"
    # 不能吞掉建单失败：差异存在但告警没送达 = 本单要防的静默故障。
    CREATE_OUT="$(gh_call issue create --title "$TITLE" --body-file "$BODY_FILE" --label waitForConfirm 2>"$CREATE_ERR")"
    CREATE_RC=$?
    ISSUE="$(issue_number_from "$CREATE_OUT")"
    if [ "$CREATE_RC" != "0" ] || [ -z "$ISSUE" ]; then
      ISSUE="none"
      ISSUE_CREATE_FAILED=1
      echo "ISSUE_CREATE_RC=$CREATE_RC"
      sed -n '1,5p' "$CREATE_ERR" 2>/dev/null
    else
      # 成功才写 marker（失败写了会让次日「当天已建单」假象吞掉告警）。
      if write_marker issue "$ISSUE"; then ISSUE_MARKER="$ISSUE"; fi
    fi
    rm -f "$CREATE_ERR"
  fi
fi
rm -f "$BODY_FILE"

if [ "$ISSUE_CREATE_FAILED" = "1" ]; then
  emit_tokens FAIL
  exit 2
fi

# ------------------------------------------------- 升级基准（#2631 H2）
# 基准 = 「最近一份更早的**有效**归档」：跳过 failed:true 与损坏归档（否则会把
# 「不可比」伪装成「未下降」，或让失败归档永远不复位升级）。无有效基准 → 不升级。
PREV_ARCHIVE="$(latest_valid_archive)"
ESCALATE_REASON=""
if [ -n "$PREV_ARCHIVE" ]; then
  PREV_DATE="$(basename "$PREV_ARCHIVE" .json)"
  ESCALATE_BASELINE="$(basename "$PREV_ARCHIVE")"
  ESCALATE_BASELINE_DAYS="$(baseline_days "$PREV_DATE" "$RECONCILE_DATE")"
  YESTERDAY="$(TZ=UTC date -d "$RECONCILE_DATE -1 day" +%F 2>/dev/null || true)"
  if [ -n "$YESTERDAY" ] && [ "$PREV_DATE" = "$YESTERDAY" ] && [ "$ESCALATE_BASELINE_DAYS" != "unknown" ]; then
    ESCALATE_BASELINE_GAP=0
  else
    # 基准非昨日（缺口）或天数不可算：保守按缺口处理，绝不冒充「连续 2 天」。
    ESCALATE_BASELINE_GAP=1
  fi
  for cat in TENANT_MISMATCH ORG_MISMATCH NIL_UUID MISSING_LOCAL ORPHAN_LOCAL ROLE_MISMATCH; do
    TODAY_V="$(jq -r --arg k "$cat" '.tokens[$k] // "0"' "$RAWS_OUT")"
    PREV_V="$(jq -r --arg k "$cat" '.tokens[$k] // "0"' "$PREV_ARCHIVE")"
    case "$TODAY_V$PREV_V" in
      ''|*[!0-9]*) continue ;;
    esac
    if [ "$TODAY_V" -gt 0 ] && [ "$PREV_V" -gt 0 ] && [ "$TODAY_V" -ge "$PREV_V" ]; then
      ESCALATE_REASON="${ESCALATE_REASON}${cat}:${PREV_V}->${TODAY_V} "
    fi
  done
fi

if [ -n "$ESCALATE_REASON" ]; then
  # #2631 H1：同一「当日」只升级一次（手动补跑 + 夜间定时不得重复打扰）。
  ESCALATE_MARKER_PATH="$(marker_path escalated)"
  if marker_exists "$ESCALATE_MARKER_PATH"; then
    ESCALATE_SKIPPED="already-escalated"
    ESCALATE_MARKER="$(basename "$ESCALATE_MARKER_PATH")"
    ESCALATED=0
    ESCALATE_COMMENTS=0
    echo "ESCALATE_SKIPPED=$ESCALATE_SKIPPED"
    emit_tokens PASS
    exit 0
  fi
  ESCALATE_FILE="$(mktemp "${TMPDIR:-/tmp}/reconcile-escalate.XXXXXX")" || exit 2
  ESCALATE_ERR="$(mktemp "${TMPDIR:-/tmp}/reconcile-escalate-err.XXXXXX")" || exit 2
  {
    # 措辞与基准严格对应：只有基准 = 昨日才能声称「连续 2 天」。
    if [ "$ESCALATE_BASELINE_GAP" = "0" ]; then
      echo "## 连续 2 天未下降（升级提醒）"
    else
      echo "## 较上次归档未下降（升级提醒）"
    fi
    echo ""
    echo "比较基准：$(basename "$PREV_ARCHIVE")"
    if [ "$ESCALATE_BASELINE_GAP" != "0" ]; then
      echo ""
      echo "基准间隔：${ESCALATE_BASELINE_DAYS} 天（$(basename "$PREV_ARCHIVE") → $RECONCILE_DATE）；昨日归档缺失或不可用，请检查例行任务是否漏跑"
    fi
    echo ""
    echo '```'
    echo "ESCALATE_REASON=$ESCALATE_REASON"
    echo '```'
    echo ""
    echo "请在人工处置后回评本单；自动化不会再建第二个 Issue。"
  } > "$ESCALATE_FILE"
  gh_call issue comment "$ISSUE" --body-file "$ESCALATE_FILE" >/dev/null 2>"$ESCALATE_ERR"
  COMMENT_RC=$?
  rm -f "$ESCALATE_FILE"
  if [ "$COMMENT_RC" = "0" ]; then
    ESCALATED=1
    ESCALATE_COMMENTS=1
    # 评论成功才写当日 marker；失败不写 → 下轮仍会重试（既有 B2 语义不变）。
    if write_marker escalated \
        "date=$RECONCILE_DATE" \
        "issue=$ISSUE" \
        "baseline=$(basename "$PREV_ARCHIVE")" \
        "gap=$ESCALATE_BASELINE_GAP" \
        "reason=$ESCALATE_REASON" \
        "ts=$(date -u +%FT%TZ)"; then
      ESCALATE_MARKER="$(basename "$ESCALATE_MARKER_PATH")"
    fi
  else
    # 升级评论没写进去就不能声称「已升级」：否则连续 2 天不降的升级机制会被静默架空。
    ESCALATE_FAILED=1
    echo "ESCALATE_COMMENT_RC=$COMMENT_RC"
    sed -n '1,5p' "$ESCALATE_ERR" 2>/dev/null
  fi
  rm -f "$ESCALATE_ERR"
fi

if [ "$ESCALATE_FAILED" = "1" ]; then
  emit_tokens FAIL
  exit 2
fi

emit_tokens PASS
exit 0
