#!/usr/bin/env bash
# #2350: 把 done 级联守卫落盘到共享 opencode_gh.sh（幂等）。
#
# 共享配置不在 git 里，无法随仓库分发；本脚本让任意节点可复现同一状态：
#   1) 已含守卫 → ALREADY-APPLIED（exit 0）
#   2) 首次应用 → 备份 .bak-2350 → 插入 parent_contract_green() + done 分支守卫
#                  → bash -n → grep 自检 → APPLIED（exit 0）
# 失败（语法/自检）→ 从备份恢复并 exit 1，避免把共享配置改坏。
set -euo pipefail
REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
TARGET="${OPENCODE_GH_SCRIPT:-$HOME/.config/opencode/scripts/opencode_gh.sh}"
[ -f "$TARGET" ] || { echo "ERROR: not found: $TARGET" >&2; exit 2; }

if grep -q "parent_contract_green" "$TARGET"; then
  echo "ALREADY-APPLIED"
  exit 0
fi

BACKUP="$TARGET.bak-2350"
[ -f "$BACKUP" ] || cp "$TARGET" "$BACKUP"

python3 - "$TARGET" <<'PY'
import sys
p = sys.argv[1]
s = open(p).read()

helper = '''
# #2350: 父级联收口守卫——父任务存在 docs/evidence/<parent>/evidence.json 时，
# 仅当全部合同行 pass=true 才允许级联关闭父（D1）；无 evidence 维持现行为（D2）；
# 不级联时调用方必须输出 INFO:PARENT_CONTRACT_NOT_GREEN:<parent>（D3）。
parent_contract_green() {
  local parent="$1"
  local evidence="docs/evidence/${parent}/evidence.json"
  [ -f "$evidence" ] || return 0
  if command -v jq >/dev/null 2>&1; then
    jq -e '(.rows | type == "array") and ((.rows | length) > 0) and (all(.rows[]; .pass == true))' "$evidence" >/dev/null 2>&1
    return $?
  fi
  if command -v python3 >/dev/null 2>&1; then
    python3 -c 'import json,sys
d=json.load(open(sys.argv[1]))
rows=d.get("rows") or []
sys.exit(0 if rows and all(r.get("pass") is True for r in rows) else 1)' "$evidence" >/dev/null 2>&1
    return $?
  fi
  # 无 jq/python3：无法证明全绿 → 不级联（fail closed）。
  return 1
}

'''
anchor = 'clear_dep_cache() {'
if anchor not in s:
    raise SystemExit("ERROR: anchor clear_dep_cache() not found")
s = s.replace(anchor, helper.lstrip('\n') + anchor, 1)

old = '''      elif [ -z "$CHILD_ISSUES" ]; then
        echo "INFO:ALL_CHILDREN_DONE:$PARENT_MATCH"
'''
new = '''      elif [ -z "$CHILD_ISSUES" ]; then
        echo "INFO:ALL_CHILDREN_DONE:$PARENT_MATCH"
        
        # #2350 D1/D3: 父任务合同行未全绿 → 不级联，显式留痕保持父原状态。
        if ! parent_contract_green "$PARENT_MATCH"; then
          echo "INFO:PARENT_CONTRACT_NOT_GREEN:$PARENT_MATCH"
        else
'''
if old not in s:
    raise SystemExit("ERROR: anchor ALL_CHILDREN_DONE not found")
s = s.replace(old, new, 1)

old2 = '''        echo "SUCCESS:PARENT_DONE:$PARENT_MATCH"
      else'''
new2 = '''        echo "SUCCESS:PARENT_DONE:$PARENT_MATCH"
        fi
      else'''
if old2 not in s:
    raise SystemExit("ERROR: anchor PARENT_DONE not found")
s = s.replace(old2, new2, 1)
open(p, 'w').write(s)
PY

if ! bash -n "$TARGET" || ! grep -q "INFO:PARENT_CONTRACT_NOT_GREEN" "$TARGET"; then
  echo "ERROR: post-apply validation failed; restoring backup" >&2
  cp "$BACKUP" "$TARGET"
  exit 1
fi
echo "APPLIED"
echo "BACKUP: $BACKUP"
