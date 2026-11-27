#!/usr/bin/env bash
# #2625 (L3) 用户缓存对账运行器（薄壳）：调用 #2623 的只读对账，归档报告并按差异/失败分流告警。
#
# 用法：
#   bash scripts/reconcile-user-cache-cron.sh [--date YYYY-MM-DD] [--dry-run]
#
# 分支（逐条契约，见 #2625 计划）：
#   无差异      → 归档，不建 Issue、不发通知（GH_CALLS=0）
#   有差异      → 归档 + 建 `[Ops] 用户缓存对账差异 <date>`（标签仅 waitForConfirm）；同日去重
#   连续 2 天不降 → 在既有单追加**恰好 1 条**升级评论（不新建第二单）
#   对账失败    → 归档失败原因 + 建 `[Ops] 用户缓存对账失败 <date>`，本脚本 exit 2
#   参数/环境缺失 → exit 2 并打印 MISSING_ENV=<name>（禁止静默成功）
#
# 永不调用 --apply（判定与修复属于 #2623/#2624），定时任务只读。
# 判定逻辑不在此复制：只解析对账输出的 tokens/rows。
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
    -h|--help) sed -n '2,18p' "$0"; exit 0 ;;
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
DIFFS=0
ARCHIVE_JSON=""
ARCHIVE_TEXT=""
LAST_SUMMARY=""

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

mkdir -p "$ARCHIVE_DIR" 2>/dev/null || { echo "RESULT: FAIL (cannot create archive dir: $ARCHIVE_DIR)"; exit 2; }

# GH 调用计数落文件：gh_call 常在 $( ) 子 shell 里执行，变量自增不会传回父 shell。
GH_CALLS_FILE="$(mktemp "${TMPDIR:-/tmp}/gh-calls.XXXXXX")" || exit 2
gh_call() { # args... -> stdout, records one call
  printf 'x\n' >> "$GH_CALLS_FILE"
  "$GH_BIN" "$@"
}
gh_call_count() { wc -l < "$GH_CALLS_FILE" 2>/dev/null | tr -d ' '; }

issue_number_from() { # text (gh issue create prints URL) -> number
  printf '%s' "$1" | grep -oE '[0-9]+$' | head -n 1
}

find_issue_by_title() { # title-fragment -> number or ""
  local out
  out="$(gh_call issue list --search "${1} in:title" --state all --json number 2>/dev/null || true)"
  printf '%s' "$out" | jq -r 'if type == "array" and length > 0 then (.[0].number | tostring) else "" end' 2>/dev/null || printf ''
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
  echo "ESCALATED=$ESCALATED"
  echo "ESCALATE_COMMENTS=$ESCALATE_COMMENTS"
  echo "FAILURE_ISSUE=$FAILURE_ISSUE"
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
    echo "DRY_RUN_ACTION=create-issue title=$title"
    rm -f "$body"
    FAILURE_ISSUE="would-create"
    write_archive "{\"date\":\"$RECONCILE_DATE\",\"failed\":true,\"reason\":\"$reason\"}" "RECONCILE_DATE=$RECONCILE_DATE
RESULT: FAIL ($reason)" >/dev/null 2>&1 || true
    FILES_WRITTEN=0
    ARCHIVE_JSON="(dry-run)"
    ARCHIVE_TEXT="(dry-run)"
    emit_tokens FAIL
    exit 2
  fi
  num="$(find_issue_by_title "$title")"
  if [ -z "$num" ]; then
    num="$(issue_number_from "$(gh_call issue create --title "$title" --body-file "$body" --label waitForConfirm 2>/dev/null || true)")"
  fi
  rm -f "$body"
  FAILURE_ISSUE="${num:-unknown}"
  write_archive "{\"date\":\"$RECONCILE_DATE\",\"failed\":true,\"reason\":\"$reason\"}" "RECONCILE_DATE=$RECONCILE_DATE
RESULT: FAIL ($reason)" >/dev/null 2>&1 || true
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

EXISTING="$(find_issue_by_title "$TITLE")"
if [ -n "$EXISTING" ]; then
  ISSUE="$EXISTING"
else
  ISSUE="$(issue_number_from "$(gh_call issue create --title "$TITLE" --body-file "$BODY_FILE" --label waitForConfirm 2>/dev/null || true)")"
  [ -n "$ISSUE" ] || ISSUE="unknown"
fi
rm -f "$BODY_FILE"

# 连续 2 天不降 → 在既有单追加恰好 1 条升级评论（不新建第二单）
PREV_ARCHIVE=""
if [ -d "$ARCHIVE_DIR" ]; then
  PREV_ARCHIVE="$(find "$ARCHIVE_DIR" -maxdepth 1 -name '*.json' -type f 2>/dev/null \
    | awk -F/ -v today="$RECONCILE_DATE" '{ d=$NF; sub(/\.json$/, "", d); if (d < today) print d"\t"$0 }' \
    | LC_ALL=C sort | tail -n 1 | cut -f2)"
fi

ESCALATE_REASON=""
if [ -n "$PREV_ARCHIVE" ] && jq -e . "$PREV_ARCHIVE" >/dev/null 2>&1; then
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
  ESCALATE_FILE="$(mktemp "${TMPDIR:-/tmp}/reconcile-escalate.XXXXXX")" || exit 2
  {
    echo "## 连续 2 天未下降（升级提醒）"
    echo ""
    echo "比较基准：$(basename "$PREV_ARCHIVE")"
    echo ""
    echo '```'
    echo "ESCALATE_REASON=$ESCALATE_REASON"
    echo '```'
    echo ""
    echo "请在人工处置后回评本单；自动化不会再建第二个 Issue。"
  } > "$ESCALATE_FILE"
  gh_call issue comment "$ISSUE" --body-file "$ESCALATE_FILE" >/dev/null 2>&1 || true
  rm -f "$ESCALATE_FILE"
  ESCALATED=1
  ESCALATE_COMMENTS=1
fi

write_archive "$(jq -c . "$RAWS_OUT")" "$TEXT_REPORT" || { echo "RESULT: FAIL (cannot write archive)"; exit 2; }
emit_tokens PASS
exit 0
