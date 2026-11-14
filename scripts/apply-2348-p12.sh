#!/usr/bin/env bash
# #2348: 把 P12 command-drift 校验落盘到共享 pre-ready-check.sh（幂等）。
#
# 共享配置不在 git（机器本地状态），本脚本让任意节点可复现同一行为：
#   已应用 → ALREADY-APPLIED；首次 → 备份 .bak-2348 → 插入 normalize_contract_text()
#   + P12 规范化比对 + self-test fixtures T73/T74 → bash -n + 符号自检 → APPLIED。
# 任一自检失败 → 从备份恢复并 exit 1。
set -euo pipefail
REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
SCRIPTS_DIR="${OPENCODE_SCRIPTS_DIR:-$HOME/.config/opencode/scripts}"
TARGET="$SCRIPTS_DIR/pre-ready-check.sh"
TESTS="$SCRIPTS_DIR/tests/test_pre_ready_check.sh"
[ -f "$TARGET" ] || { echo "ERROR: not found: $TARGET" >&2; exit 2; }
[ -f "$TESTS" ] || { echo "ERROR: not found: $TESTS" >&2; exit 2; }

if grep -q "normalize_contract_text" "$TARGET" && grep -q "T73 command whitespace" "$TESTS"; then
  echo "ALREADY-APPLIED"
  exit 0
fi

BACKUP="$SCRIPTS_DIR/.pre-ready-check.sh.bak-2348"
TESTS_BACKUP="$SCRIPTS_DIR/tests/test_pre_ready_check.sh.bak-2348"
[ -f "$BACKUP" ] || cp "$TARGET" "$BACKUP"
[ -f "$TESTS_BACKUP" ] || cp "$TESTS" "$TESTS_BACKUP"

python3 - "$TARGET" "$TESTS" <<'PY'
import sys

target, tests = sys.argv[1], sys.argv[2]

# ---------------------------------------------------------------- pre-ready-check.sh
s = open(target).read()
if "normalize_contract_text" not in s:
    helper = '''
# #2348 D1: command 单元格比对前的规范化（空白折叠 + 统一弯引号）；
# 仅消除格式差异，实质文本（如截图子句/--no-ignore）仍会被判为 drift。
normalize_contract_text() {
  printf '%s' "$1" \\
    | sed -e 's/[“”]/"/g' -e "s/[‘’]/'/g" \\
    | tr -s '[:space:]' ' ' \\
    | sed -e 's/^ //' -e 's/ $//'
}

'''
    anchor = 'P12_ROOT="$(cd "$REPO_ROOT" && pwd -P)"'
    if anchor not in s:
        raise SystemExit("ERROR: pre-ready anchor P12_ROOT not found")
    s = s.replace(anchor, helper.lstrip('\n') + anchor, 1)

old = '        [ -z "$P12_CMD" ] || [ "$P12_CMD" = "$plan_cmd" ] || add_p12_error "$rid: command does not match the plan contract"'
new = '''        if [ -n "$P12_CMD" ]; then
          # #2348 D1: 规范化后比对（空白/引号风格差异不算 drift；实质差异 → command drift）。
          [ "$(normalize_contract_text "$P12_CMD")" = "$(normalize_contract_text "$plan_cmd")" ] \\
            || add_p12_error "$rid: command drift: evidence command does not match the plan contract"
        fi'''
if old in s:
    s = s.replace(old, new, 1)
elif "command drift" not in s:
    raise SystemExit("ERROR: P12 command comparison anchor not found")
open(target, "w").write(s)

# ---------------------------------------------------------------- self-test fixtures
t = open(tests).read()
if "T73 command whitespace" not in t:
    fixture_anchor = 'jq \'.build="0000000"\' "$EVIDENCE_GOOD" > "$EVIDENCE_BUILD_MISMATCH"'
    fixtures = fixture_anchor + '''
# #2348: command 单元格规范化 fixtures
PLAN_WS="$WORK/plan-whitespace.md"
sed 's/| R1 | `true` |/| R1 | `true     ` |/' "$PLAN_CONTRACT" > "$PLAN_WS"
EVIDENCE_CMD_DRIFT="$EVIDENCE_DIR/evidence-command-drift.json"
jq '(.rows[] | select(.id=="R1") | .command) = "true && echo drifted"' "$EVIDENCE_GOOD" > "$EVIDENCE_CMD_DRIFT"'''
    if fixture_anchor not in t:
        raise SystemExit("ERROR: self-test fixture anchor not found")
    t = t.replace(fixture_anchor, fixtures, 1)

    case_anchor = '''run_case "T64 build/commit mismatch fails P12" 1 'P12.*FAIL' "$REPO_OK" \\
  "${CONTRACT_COMMON[@]}" --evidence-file "$EVIDENCE_BUILD_MISMATCH"'''
    cases = case_anchor + '''

# #2348 D1/D4: 空白/引号风格差异不算 drift；实质差异必须报 command drift。
run_case "T73 command whitespace normalization passes P12" 0 'P12.*PASS' "$REPO_OK" \\
  "${CONTRACT_COMMON[@]}" --plan-file "$PLAN_WS" --evidence-file "$EVIDENCE_GOOD"

run_case "T74 substantive command drift fails P12" 1 'P12.*command drift' "$REPO_OK" \\
  "${CONTRACT_COMMON[@]}" --evidence-file "$EVIDENCE_CMD_DRIFT"'''
    if case_anchor not in t:
        raise SystemExit("ERROR: self-test case anchor not found")
    t = t.replace(case_anchor, cases, 1)
open(tests, "w").write(t)
print("patched shared config")
PY

if ! bash -n "$TARGET" || ! bash -n "$TESTS" \
   || ! grep -q "command drift" "$TARGET" \
   || ! grep -q "T73 command whitespace" "$TESTS"; then
  echo "ERROR: post-apply validation failed; restoring backups" >&2
  cp "$BACKUP" "$TARGET"
  cp "$TESTS_BACKUP" "$TESTS"
  exit 1
fi
echo "APPLIED"
echo "BACKUP: $BACKUP"
echo "BACKUP: $TESTS_BACKUP"
