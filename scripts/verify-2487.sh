#!/usr/bin/env bash
# verify-2487.sh — evidence generator for #2487（Tier 2 人员/成员类页面：业务可纠正错误 → 持久提示）
#
# 用法：bash scripts/verify-2487.sh <r1|r2|r3|r4|r5>
#
# 设计约定（与 Accepted Plan Rev.2 的 R1–R5 一一对应）：
#   * 扫描器 **只读**：不修改任何源码，只枚举 + 归类（READONLY=1）。
#   * 每个 `message.error(` 以 `<file>|<函数>|<该函数内第 n 处>` 为键，避免修复本身
#     造成的行号漂移把判定表打散。
#   * 归类是**数据**而非启发式：下表即人工复核后的判定记录（无窗口猜测）。
#   * 判定口径 = #2480 rubric（frontend-pc/src/utils/formConflict.js）：
#     `40002/40004/40005/40900` 且指向用户可改输入 → 持久提示；其余保持 toast。
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT" || exit 2

EV_DIR="docs/evidence/2487"
BASELINE_SCAN="docs/evidence/2480/R11-scan.txt"
UM="frontend-pc/src/pages/System/UserManagement.jsx"
PS="frontend-pc/src/pages/admin/platform-staff/index.jsx"
TP="frontend-pc/src/pages/System/TechnicianProfiles.jsx"
IN_SCOPE=("$UM" "$PS" "$TP")
TMP_DIR="$ROOT/tmp"
mkdir -p "$TMP_DIR" "$EV_DIR"

provenance() {
  local commit short dirty="false"
  commit="$(git rev-parse HEAD)"
  short="$(git rev-parse --short HEAD)"
  # dirty 只看**已跟踪**文件的改动：证据产物本身是新文件（untracked），
  # 不应把「证据尚未提交」误判成脏树（evidence-artifacts.sh 要求 dirty=false）。
  if [ -n "$(git status --porcelain --untracked-files=no 2>/dev/null)" ]; then dirty="true"; fi
  printf '# build=1.0.%s # commit=%s # dirty=%s\n' "$short" "$commit" "$dirty"
  printf '# tree=%s\n' "$commit"
}

# 判定记录：file|function|nth|code_family|tier|reason
classify_table() {
  cat <<'TABLE'
frontend-pc/src/pages/System/UserManagement.jsx|fetchList|1|none|keep:instant|GET /admin/user-management 仅 40100/50000/网络，无可由用户改输入纠正的冲突
frontend-pc/src/pages/System/UserManagement.jsx|handleExport|1|none|keep:instant|导出走 fetchBlob（非 2xx 直接抛「导出失败」，无 code），属瞬时失败
frontend-pc/src/pages/System/UserManagement.jsx|handleSave|1|40002|keep:instant|PUT /admin/user-management/:id 的 40002 仅 status 枚举/no editable fields（表单不可产生），其余 40400/50000
frontend-pc/src/pages/System/UserManagement.jsx|handleSave|2|40002|keep:instant|同上（catch 兜底：400 族带 code，但可达码集合无用户可改冲突）
frontend-pc/src/pages/System/UserManagement.jsx|handleGrant|1|40002|keep:instant|points-grant 的 40002 = amount/reason 必填或非正数，前端 Form required + InputNumber min 已拦（不可达）
frontend-pc/src/pages/System/UserManagement.jsx|handleGrant|2|40002|keep:instant|同上（catch 兜底）
frontend-pc/src/pages/System/UserManagement.jsx|fetchOrphans|1|none|keep:instant|GET /admin/iam-orphans 仅 40100/50200
frontend-pc/src/pages/System/UserManagement.jsx|fetchOrphans|2|none|keep:instant|网络异常兜底：孤儿列表请求本身失败（无 code），仅瞬时提示
frontend-pc/src/pages/System/UserManagement.jsx|purgeOrphans|1|none|keep:instant|POST /admin/iam-orphans/purge 仅 40001/40100/50200（均非 rubric 码）
frontend-pc/src/pages/System/UserManagement.jsx|purgeOrphans|2|none|keep:instant|网络异常兜底：清理请求本身失败（无 code），仅瞬时提示
frontend-pc/src/pages/System/UserManagement.jsx|handleMarkDeleted|1|none|keep:instant|mark-deleted 仅 40400/40000/50000
frontend-pc/src/pages/System/UserManagement.jsx|handleSaveIdCard|1|40002|keep:instant|非 rubric 残差：rubric 分支已在同函数前部 setFormConflict 拦下（此处仅 40400/50000/网络）
frontend-pc/src/pages/System/UserManagement.jsx|handleSaveIdCard|2|40002|keep:instant|同上（catch 中的非 rubric 残差）
frontend-pc/src/pages/System/UserManagement.jsx|handleRejectIdPhotos|1|none|keep:instant|id-photo/reject 仅 40400/50000
frontend-pc/src/pages/System/UserManagement.jsx|handleRejectIdPhotos|2|none|keep:instant|网络异常兜底：拒绝采用请求本身失败（无 code），仅瞬时提示
frontend-pc/src/pages/admin/platform-staff/index.jsx|fetchList|1|none|keep:instant|GET /admin/platform-staff 仅 40100/40300/50000
frontend-pc/src/pages/admin/platform-staff/index.jsx|fetchList|2|none|keep:instant|网络异常兜底：平台员工列表请求本身失败（无 code），仅瞬时提示
frontend-pc/src/pages/admin/platform-staff/index.jsx|handleCreate|1|40002|keep:instant|非 rubric 残差：40900 重复已在同函数前部拦下；此处只剩 40001（IAM 创建失败）/40002（手动密码空，前端 required 已拦）/50000
frontend-pc/src/pages/admin/platform-staff/index.jsx|handleCreate|2|40002|keep:instant|同上（catch 中的非 rubric 残差）
frontend-pc/src/pages/admin/platform-staff/index.jsx|handleDisable|1|40900|keep:instant|Disable 的 40900 来自 IAM 解绑失败（platform_staff.go:270，服务端事实），非用户改输入可纠正
frontend-pc/src/pages/admin/platform-staff/index.jsx|handleDisable|2|40900|keep:instant|同上（catch 兜底）
frontend-pc/src/pages/System/TechnicianProfiles.jsx|fetchList|1|none|keep:instant|GET /technician-profiles 仅 40300/50000
frontend-pc/src/pages/System/TechnicianProfiles.jsx|submit|1|40002|keep:instant|照片上传分支：40002 仅「file required」（PhotoUploader 必带文件，不可达），其余 50000
frontend-pc/src/pages/System/TechnicianProfiles.jsx|submit|2|40003|keep:instant|非 rubric 残差：40900 重复已在同函数前部拦下；此处剩 40003（账号不可用/不在本商户）/40300/40400/50000
frontend-pc/src/pages/System/TechnicianProfiles.jsx|submit|3|40002|keep:instant|同上（catch 中的非 rubric 残差）
frontend-pc/src/pages/System/TechnicianProfiles.jsx|toggleStatus|1|40002|keep:instant|setStatus 的 40002 = status 枚举（前端固定 active/inactive，不可达），另有 40400/50000
TABLE
}

# 转换记录（business-correctable → 共享件持久提示）：file|function|endpoint|rubric_code|testid|reason
converted_table() {
  cat <<'TABLE'
frontend-pc/src/pages/System/UserManagement.jsx|handleSaveIdCard|PUT /admin/user-management/:id/id-card|40002|user-idcard-conflict|40002「身份证号格式不正确」用户改输入即可重试（user_management_idcard.go:46）
frontend-pc/src/pages/admin/platform-staff/index.jsx|handleCreate|POST /admin/platform-staff|40900|platform-staff-form-conflict|40900 用户名/手机号重复（platform_staff.go:185/189）
frontend-pc/src/pages/System/TechnicianProfiles.jsx|submit|POST /technician-profiles|40900|technician-form-conflict|40900 同一商户下该员工已是维修师（technician_profile.go:171）
TABLE
}

# 枚举每个 message.error( 的 file/line/function/第 n 处
scan_sites() {
  local f
  for f in "${IN_SCOPE[@]}"; do
    awk -v F="$f" '
      /^  const [A-Za-z_][A-Za-z0-9_]*[ \t]*=/ { fn=$2 }
      /^  function [A-Za-z_][A-Za-z0-9_]*/ { fn=$2; sub(/\(.*/, "", fn) }
      /message\.error\(/ {
        key = fn
        seq[key]++
        printf "%s\t%d\t%s\t%d\n", F, NR, fn, seq[key]
      }
    ' "$f"
  done
}

# 函数内首个 rubric 判定所在行
rubric_line_in() { # file function
  awk -v F="$1" -v WANT="$2" '
    /^  const [A-Za-z_][A-Za-z0-9_]*[ \t]*=/ { fn=$2 }
    /^  function [A-Za-z_][A-Za-z0-9_]*/ { fn=$2; sub(/\(.*/, "", fn) }
    fn == WANT && /isBusinessCorrectableConflict\(/ && !found { print NR; found=1 }
  ' "$1"
}

# 函数内最后一个 rubric 判定所在行（「分类先于终态 toast」判据用）
last_rubric_line_in() { # file function
  awk -v WANT="$2" '
    /^  const [A-Za-z_][A-Za-z0-9_]*[ \t]*=/ { fn=$2 }
    /^  function [A-Za-z_][A-Za-z0-9_]*/ { fn=$2; sub(/\(.*/, "", fn) }
    fn == WANT && /isBusinessCorrectableConflict\(/ { last=NR }
    END { if (last) print last }
  ' "$1"
}

# 函数内 rubric 判定出现的次数
rubric_count_in() { # file function
  awk -v WANT="$2" '
    /^  const [A-Za-z_][A-Za-z0-9_]*[ \t]*=/ { fn=$2 }
    /^  function [A-Za-z_][A-Za-z0-9_]*/ { fn=$2; sub(/\(.*/, "", fn) }
    fn == WANT && /isBusinessCorrectableConflict\(/ { c++ }
    END { print c+0 }
  ' "$1"
}

# 函数内最后一个 message.error( 所在行
last_toast_line_in() { # file function
  awk -v WANT="$2" '
    /^  const [A-Za-z_][A-Za-z0-9_]*[ \t]*=/ { fn=$2 }
    /^  function [A-Za-z_][A-Za-z0-9_]*/ { fn=$2; sub(/\(.*/, "", fn) }
    fn == WANT && /message\.error\(/ { last=NR }
    END { if (last) print last }
  ' "$1"
}

# 函数内所有 message.error( 所在行
toast_lines_in() { # file function
  awk -v WANT="$2" '
    /^  const [A-Za-z_][A-Za-z0-9_]*[ \t]*=/ { fn=$2 }
    /^  function [A-Za-z_][A-Za-z0-9_]*/ { fn=$2; sub(/\(.*/, "", fn) }
    fn == WANT && /message\.error\(/ { print NR }
  ' "$1"
}

baseline_count() { # file
  grep -c "^$1:" "$BASELINE_SCAN" 2>/dev/null || echo 0
}

r1() {
  local tier_of code_of reason_of
  declare -A tier_of code_of reason_of
  while IFS='|' read -r f fn seq code tier reason; do
    [ -n "${f:-}" ] || continue
    tier_of["$f|$fn|$seq"]="$tier"
    code_of["$f|$fn|$seq"]="$code"
    reason_of["$f|$fn|$seq"]="$reason"
  done < <(classify_table)

  provenance
  echo "# scanner=verify-2487.sh r1  READONLY=1  strategy=branch-block-key(file|fn|nth)"
  echo "# baseline=$BASELINE_SCAN"

  local scan_rows=0 unclassified=0 classify_hits=0
  local per_file_out
  per_file_out="$TMP_DIR/verify-2487-r1-scan.tsv"
  scan_sites > "$per_file_out"

  while IFS=$'\t' read -r f line fn seq; do
    [ -n "${f:-}" ] || continue
    scan_rows=$((scan_rows + 1))
    local key="$f|$fn|$seq"
    if [ -z "${tier_of[$key]:-}" ]; then
      printf '%s:%s\tfn=%s\toccurrence=%s\tcode=?\ttier=UNCLASSIFIED\treason=missing judgement\n' \
        "$f" "$line" "$fn" "$seq"
      unclassified=$((unclassified + 1))
      continue
    fi
    classify_hits=$((classify_hits + 1))
    printf '%s:%s\tfn=%s\toccurrence=%s\tcode=%s\ttier=%s\treason=%s\n' \
      "$f" "$line" "$fn" "$seq" "${code_of[$key]}" "${tier_of[$key]}" "${reason_of[$key]}"
  done < "$per_file_out"

  # 转换分支（business-correctable）：按设计不再有 message.error，单独列出以便逐条核对
  local conv_rows=0
  while IFS='|' read -r f fn endpoint code testid reason; do
    [ -n "${f:-}" ] || continue
    local rl
    rl="$(rubric_line_in "$f" "$fn" | head -1)"
    conv_rows=$((conv_rows + 1))
    printf '%s:%s\tfn=%s\tbranch=conflict\tendpoint=%s\tcode=%s\ttier=business-correctable\ttestid=%s\treason=%s\n' \
      "$f" "${rl:-0}" "$fn" "$endpoint" "$code" "$testid" "$reason"
  done < <(converted_table)

  # 每文件行数不得少于 #2480 基线扫描（只允许改结论）
  local files_ok=1 f base scanned
  for f in "${IN_SCOPE[@]}"; do
    base="$(baseline_count "$f")"
    scanned="$(awk -F'\t' -v F="$f" '$1 == F {c++} END {print c+0}' "$per_file_out")"
    printf 'FILE_COVERAGE|%s|baseline=%s|scanned=%s\n' "$f" "$base" "$scanned"
    if [ "$scanned" -lt "$base" ]; then files_ok=0; fi
  done

  local baseline_total
  baseline_total=$(awk -F: '/^frontend-pc\// {print $1}' "$BASELINE_SCAN" | sort -u | grep -cE "UserManagement\.jsx|platform-staff/index\.jsx|TechnicianProfiles\.jsx" || true)
  baseline_total=$(for f in "${IN_SCOPE[@]}"; do baseline_count "$f"; done | paste -sd+ - | bc)
  baseline_total="${baseline_total:-0}"

  echo "SCAN_ROWS=$scan_rows"
  echo "CLASSIFIED_ROWS=$((scan_rows - unclassified))"
  echo "CONVERTED_ROWS=$conv_rows"
  echo "KEEP_ROWS=$scan_rows"
  echo "UNCLASSIFIED_ROWS=$unclassified"
  echo "FILES_SCANNED=${#IN_SCOPE[@]}"
  echo "BASELINE_ROWS=$baseline_total"
  echo "DEFER_ROWS=0"
  # Rev.2 只覆盖 3 个页面（ClientManagement 已由 #2518 删除），计划表内「6 文件」为 Rev.1 残留。
  echo "PLAN_FILE_COUNT_NOTE=plan-table-says-6;rev2-scope=3;reason=#2518 deleted ClientManagement.jsx"
  # #2465（holdon）若把维修师档案收敛进员工管理，本文件的转换需复核。
  echo "REVISIT_2465=1 reason=TechnicianProfiles may be folded into staff management by #2465 (holdon)"
  echo "READONLY=1"
  echo "RUN_HEURISTIC=0"

  if [ "$unclassified" -eq 0 ] && [ "$files_ok" -eq 1 ]; then
    echo "RESULT: PASS"
    return 0
  fi
  echo "RESULT: FAIL"
  return 1
}

r2() {
  provenance
  echo "# scanner=verify-2487.sh r2  (converted branches only)"
  local rows=0 fail=0 alert_ok=1 code_first_ok=1
  while IFS='|' read -r f fn endpoint code testid reason; do
    [ -n "${f:-}" ] || continue
    rows=$((rows + 1))
    local has_import=0 has_render=0 has_rubric=0 last_rubric last_toast rubric_checks code_first=1
    grep -q "^import FormConflictAlert" "$f" && has_import=1
    if grep -q "<FormConflictAlert" "$f" && grep -q "testId=\"$testid\"" "$f"; then has_render=1; fi
    rubric_checks="$(rubric_count_in "$f" "$fn")"
    last_rubric="$(last_rubric_line_in "$f" "$fn")"
    [ -n "$last_rubric" ] && has_rubric=1
    last_toast="$(last_toast_line_in "$f" "$fn")"
    # CODE_FIRST：分类（rubric 判定）必须先于该操作的终态兜底 toast，
    # 否则说明冲突会先被 toast 兜走（Round 1 Bug：先 toast 后判 code）。
    if [ -z "$last_rubric" ] || { [ -n "$last_toast" ] && [ "$last_rubric" -gt "$last_toast" ]; }; then
      code_first=0
    fi
    [ "$has_render" -eq 1 ] || alert_ok=0
    [ "$has_rubric" -eq 1 ] && [ "$code_first" -eq 1 ] || code_first_ok=0
    [ "$has_import" -eq 1 ] && [ "$has_render" -eq 1 ] && [ "$has_rubric" -eq 1 ] && [ "$code_first" -eq 1 ] || fail=$((fail + 1))
    printf 'CONVERT|%s|fn=%s|endpoint=%s|rubric_code=%s|alert_testid=%s|import=%s|render=%s|rubric_checks=%s|last_rubric=%s|last_toast=%s|code_first=%s|reason=%s\n' \
      "$f" "$fn" "$endpoint" "$code" "$testid" "$has_import" "$has_render" "$rubric_checks" "${last_rubric:-0}" "${last_toast:-0}" "$code_first" "$reason"
  done < <(converted_table)

  echo "CONVERTED_ROWS=$rows"
  echo "PERSIST_ALERT=$alert_ok"
  echo "CODE_FIRST=$code_first_ok"
  echo "CODE_FIRST_DEF=rubric_check_precedes_terminal_fallback_toast"
  echo "CONVERT_FAILURES=$fail"
  if [ "$rows" -ge 1 ] && [ "$fail" -eq 0 ]; then
    echo "RESULT: PASS"
    return 0
  fi
  echo "RESULT: FAIL"
  return 1
}

r3() {
  provenance
  echo "# scanner=verify-2487.sh r3"
  local f reuse=0
  for f in "${IN_SCOPE[@]}"; do
    if grep -q "^import FormConflictAlert" "$f" && grep -q "<FormConflictAlert" "$f"; then
      reuse=$((reuse + 1))
      printf 'REUSE|%s\n' "$f"
    fi
  done
  local dup
  dup=$(grep -rlE "(export default function|const|function) [A-Za-z0-9_]*ConflictAlert[ (]" \
    frontend-pc/src --include='*.jsx' 2>/dev/null | grep -v '^frontend-pc/src/components/FormConflictAlert.jsx$' | wc -l | tr -d ' ')
  echo "SHARED_COMPONENT=frontend-pc/src/components/FormConflictAlert.jsx"
  echo "SHARED_REUSE=$reuse"
  echo "LOCAL_ALERT_DUP=$dup"
  echo "RUBRIC_OWNER=frontend-pc/src/utils/formConflict.js"
  if [ "$reuse" -ge 1 ] && [ "$dup" -eq 0 ]; then
    echo "RESULT: PASS"
    return 0
  fi
  echo "RESULT: FAIL"
  return 1
}

r4() {
  provenance
  echo "# scanner=verify-2487.sh r4"
  local persisted=0 residual=0 keep_rows keep_with_reason
  while IFS='|' read -r f fn _endpoint _code _testid _reason; do
    [ -n "${f:-}" ] || continue
    persisted=$((persisted + 1))
    local last_rubric last_toast bad=0
    last_rubric="$(last_rubric_line_in "$f" "$fn")"
    last_toast="$(last_toast_line_in "$f" "$fn")"
    # 共享 rubric 分类必须先于该操作的终态兜底 toast；否则可纠正冲突仍会被 toast 兜走。
    # 期望：共享 rubric 分类先于终态兜底 toast（last_rubric < last_toast）。
    # 反例（残留）：缺少 rubric 判定，或 rubric 判定反而在 toast 之后。
    if [ -z "$last_rubric" ] || [ "$last_rubric" -gt "${last_toast:-0}" ]; then
      bad=1
    fi
    if [ "$bad" -ne 0 ]; then
      printf 'RESIDUAL|%s|fn=%s|business_toast_after_rubric=%s|last_rubric=%s|last_toast=%s\n' \
        "$f" "$fn" "$bad" "${last_rubric:-0}" "${last_toast:-0}"
      residual=$((residual + bad))
    else
      printf 'PERSISTED|%s|fn=%s|rubric_precedes_terminal_toast=1|last_rubric=%s|last_toast=%s\n' \
        "$f" "$fn" "$last_rubric" "${last_toast:-0}"
    fi
  done < <(converted_table)

  keep_rows=$(classify_table | grep -c '|keep:instant|' || true)
  keep_with_reason=$(classify_table | grep '|keep:instant|' | awk -F'|' 'length($6) >= 8 {c++} END {print c+0}')
  echo "PERSISTED_FUNCTIONS=$persisted"
  echo "BUSINESS_TOAST_RESIDUAL=$residual"
  echo "KEEP_ROWS=$keep_rows"
  echo "KEEP_WITH_REASON=$keep_with_reason"
  if [ "$residual" -eq 0 ] && [ "$keep_with_reason" -eq "$keep_rows" ]; then
    echo "RESULT: PASS"
    return 0
  fi
  echo "RESULT: FAIL"
  return 1
}

r5() {
  provenance
  echo "# scanner=verify-2487.sh r5"
  echo "# build_cmd=cd frontend-pc && npm run build"
  echo "# lint_cmd=cd frontend-pc && npx eslint src/pages/System/UserManagement.jsx src/pages/admin/platform-staff/index.jsx src/pages/System/TechnicianProfiles.jsx"
  local build_log="$TMP_DIR/verify-2487-build.log"
  local lint_log="$TMP_DIR/verify-2487-lint.log"
  local heavy="$HOME/.config/opencode/scripts/heavy-run.sh"
  local build_ok=0 lint_ok=0

  if [ -x "$heavy" ]; then
    bash "$heavy" --scope host --timeout 1800 -- bash -c "cd '$ROOT/frontend-pc' && npm run build" >"$build_log" 2>&1 && build_ok=1
  else
    ( cd "$ROOT/frontend-pc" && npm run build ) >"$build_log" 2>&1 && build_ok=1
  fi
  if [ "$build_ok" -eq 1 ]; then
    grep -E "built in|modules transformed|dist/index.html" "$build_log" | tail -3
  else
    tail -25 "$build_log"
  fi

  local lint_targets="src/pages/System/UserManagement.jsx src/pages/admin/platform-staff/index.jsx src/pages/System/TechnicianProfiles.jsx"
  if [ -x "$heavy" ]; then
    bash "$heavy" --scope host --timeout 900 -- bash -c "cd '$ROOT/frontend-pc' && npx eslint $lint_targets" >"$lint_log" 2>&1 && lint_ok=1
  else
    ( cd "$ROOT/frontend-pc" && npx eslint $lint_targets ) >"$lint_log" 2>&1 && lint_ok=1
  fi
  grep -E "error|warning" "$lint_log" | tail -5
  local lint_errors
  lint_errors=$(grep -cE "^[^ ]*error" "$lint_log" 2>/dev/null || true)
  lint_errors="${lint_errors:-0}"

  echo "PC_BUILD=$build_ok"
  echo "BUILD_OK=$build_ok"
  echo "ESLINT_OK=$lint_ok"
  echo "LINT_ERRORS=$lint_errors"
  if [ "$build_ok" -eq 1 ] && [ "$lint_ok" -eq 1 ]; then
    echo "RESULT: PASS"
    return 0
  fi
  echo "RESULT: FAIL"
  return 1
}

case "${1:-}" in
  r1) r1 ;;
  r2) r2 ;;
  r3) r3 ;;
  r4) r4 ;;
  r5) r5 ;;
  *)
    echo "Usage: bash scripts/verify-2487.sh <r1|r2|r3|r4|r5>" >&2
    exit 2
    ;;
esac
