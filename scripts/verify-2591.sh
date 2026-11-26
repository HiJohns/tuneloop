#!/usr/bin/env bash
# verify-2591.sh — evidence generator for #2591（删除 api.md 中 §5.8.1–5.8.3 的陈旧重复副本）
#
# 用法：bash scripts/verify-2591.sh <r1|r2|r3|r4>
#
# 设计约定（与 Accepted Plan 的 R1–R4 一一对应）：
#   * 折叠行**基线定位**，不硬编码行号：在 `git show <fix>^:docs/spec/api/api.md`
#     中找「同时含字面 \n 与 #### 5.6.1」的行；命中 0 行或多行 → FAIL（不猜）。
#   * r3 红样**现场采集**自 fix^ blob，不是手写常量。
#   * 所有判定 token 一律 KEY=VALUE 单行输出，便于机器 grep 断言。
#   * 纯文档改动：无重命令，不取 host 锁，也不新增 go/npm 合同行。
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT" || exit 2

TARGET="docs/spec/api/api.md"
FIX_COMMIT="${FIX_COMMIT:-}"
if [ -z "$FIX_COMMIT" ]; then
  FIX_COMMIT="$(git log --format=%H --grep='^fix(issue-2591)' -1 2>/dev/null || true)"
fi
[ -n "$FIX_COMMIT" ] || FIX_COMMIT="$(git rev-parse HEAD)"
FIX_SHORT="$(git rev-parse --short "$FIX_COMMIT")"
HEAD_SHA="$(git rev-parse HEAD)"

provenance() {
  local dirty="false"
  if [ -n "$(git status --porcelain --untracked-files=no 2>/dev/null)" ]; then dirty="true"; fi
  printf '# build=1.0.%s # commit=%s # dirty=%s\n' "$FIX_SHORT" "$FIX_COMMIT" "$dirty"
  printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
  printf 'EVIDENCE_TARGET=%s\n' "$TARGET"
}

require_python() {
  command -v python3 >/dev/null 2>&1 || { echo "ERROR:python3_missing" >&2; exit 2; }
}

baseline_blob() { # -> stdout
  git show "${FIX_COMMIT}^:${TARGET}" 2>/dev/null || {
    echo "ERROR:BASELINE_BLOB_UNAVAILABLE" >&2
    exit 1
  }
}

r1() {
  require_python
  provenance
  BASELINE_FILE="$(mktemp)"; export BASELINE_FILE
  baseline_blob > "$BASELINE_FILE"
  export TARGET_FILE="$ROOT/$TARGET"
  python3 - <<'PY'
import os, sys
head = open(os.environ['TARGET_FILE'], encoding='utf-8').read().split('\n')

RETAINED = '- 严格数值验证: 价格字段必须为有效数字'

def find_target(lines):
    hits = [i for i, l in enumerate(lines) if '\\n' in l and '#### 5.6.1' in l]
    return hits

def section_range(lines, start_pat, end_pat=None):
    start = None
    for i, l in enumerate(lines):
        if start is None and l.startswith(start_pat):
            start = i
        elif start is not None and end_pat and l.startswith(end_pat):
            return start, i
    return start, len(lines)

def security_bullets_58_3(lines):
    s, e = section_range(lines, '#### 5.8.3 ', '### 5.9 ')
    if s is None:
        return -1
    marker = None
    for i in range(e - 1, s - 1, -1):
        if lines[i].strip() == '**安全特性**:':
            marker = i
            break
    if marker is None:
        return -2
    n = 0
    for i in range(marker + 1, e):
        if lines[i].startswith('- '):
            # 折叠行（含字面 \n）不算「可见的合法 bullet」——它的第 3 条被
            # 3060 字符的陈旧副本截断在同一行里，渲染上等于不存在。
            if '\\n' not in lines[i]:
                n += 1
        elif lines[i].strip() == '':
            break
    return n

target_hits = [i for i, l in enumerate(head) if l.strip() == RETAINED]
folded = find_target(head)
ok = True

target_line = (target_hits[0] + 1) if target_hits else 0
target_len = len(head[target_hits[0]]) if target_hits else -1
if len(target_hits) != 1 or folded:
    ok = False

literal_lines = sum(1 for l in head if '\\n' in l)
bullets = security_bullets_58_3(head)
headings58 = sum(1 for l in head if l.startswith('#### 5.8.1 ') or l.startswith('#### 5.8.2 ') or l.startswith('#### 5.8.3 '))
stale_titles = sum(1 for l in head if l.startswith('#### 5.6.1 ') or l.startswith('#### 5.6.2 ') or l.startswith('#### 5.6.3 '))

print(f'TARGET_LINE={target_line}')
print(f'TARGET_LINE_LEN={target_len}')
print(f'LITERAL_BSLASH_N_LINES={literal_lines}')
print(f'SECURITY_BULLETS_58_3={bullets}')
print(f'HEADINGS_58X={headings58}')
print(f'STALE_58_TITLES={stale_titles}')

if target_len != 21 or literal_lines != 0 or bullets != 3 or headings58 != 3 or stale_titles != 0:
    ok = False
print('RESULT: PASS' if ok else 'RESULT: FAIL')
sys.exit(0 if ok else 1)
PY
  local rc=$?
  rm -f "$BASELINE_FILE"
  return $rc
}

r2() {
  require_python
  provenance
  BASELINE_FILE="$(mktemp)"; export BASELINE_FILE
  baseline_blob > "$BASELINE_FILE"
  export TARGET_FILE="$ROOT/$TARGET"
  NUMSTAT="$(git diff --numstat "${FIX_COMMIT}^" "$FIX_COMMIT" -- "$TARGET" 2>/dev/null || true)"
  export NUMSTAT
  python3 - <<'PY'
import os, sys
base = open(os.environ['BASELINE_FILE'], encoding='utf-8').read().split('\n')
head = open(os.environ['TARGET_FILE'], encoding='utf-8').read().split('\n')
ok = True

def fences(lines): return sum(1 for l in lines if l.startswith('```'))
def headings(lines): return sum(1 for l in lines if l.startswith('#'))

folded = [i for i, l in enumerate(base) if '\\n' in l and '#### 5.6.1' in l]
if len(folded) != 1:
    print('RESULT: FAIL')
    print('ERROR:FOLDED_LINE_NOT_UNIQUE')
    sys.exit(1)
t = folded[0]
expected = list(base)
expected[t] = base[t].split('\\n')[0]

line_count_unchanged = 1 if len(base) == len(head) else 0
fence_unchanged = 1 if fences(base) == fences(head) else 0
heading_unchanged = 1 if headings(base) == headings(head) else 0
prefix_identical = 1 if base[:t] == head[:t] else 0
suffix_identical = 1 if base[t+1:] == head[t+1:] else 0

def sec58_range(lines):
    s = None
    for i, l in enumerate(lines):
        if l.startswith('#### 5.8.1 '):
            s = i
            break
    e = len(lines)
    if s is not None:
        for i in range(s + 1, len(lines)):
            if lines[i].startswith('### 5.9 '):
                e = i
                break
    return s, e

bs, be = sec58_range(base)
hs, he = sec58_range(head)
# 期望块 = fix^ 块把折叠行替换为其首段后的形态（即「§5.8 正文只允许目标行变短」）
sec58_identical = 1 if (
    bs is not None and bs == hs and (be - bs) == (he - hs) and head[bs:be] == expected[bs:be]
) else 0

numstat = os.environ.get('NUMSTAT', '').strip().split('\n')[0]
parts = numstat.split('\t') if numstat else []
added = int(parts[0]) if len(parts) >= 1 and parts[0].isdigit() else -1
deleted = int(parts[1]) if len(parts) >= 2 and parts[1].isdigit() else -1

print(f'LINE_COUNT_UNCHANGED={line_count_unchanged}')
print(f'FENCE_COUNT_UNCHANGED={fence_unchanged}')
print(f'HEADING_COUNT_UNCHANGED={heading_unchanged}')
print(f'PREFIX_IDENTICAL={prefix_identical}')
print(f'SUFFIX_IDENTICAL={suffix_identical}')
print(f'SEC_58_BLOCK_IDENTICAL={sec58_identical}')
print(f'DIFF_ADDED_LINES={added}')
print(f'DIFF_DELETED_LINES={deleted}')
print(f'LINE_COUNT={len(head)}')
print(f'FENCE_COUNT={fences(head)}')
print(f'HEADING_COUNT={headings(head)}')

if not (line_count_unchanged and fence_unchanged and heading_unchanged and prefix_identical
        and suffix_identical and sec58_identical and added == 1 and deleted == 1):
    ok = False
print('RESULT: PASS' if ok else 'RESULT: FAIL')
sys.exit(0 if ok else 1)
PY
  local rc=$?
  rm -f "$BASELINE_FILE"
  return $rc
}

r3() {
  require_python
  provenance
  echo "# source=git show ${FIX_SHORT}^:${TARGET}（红样现场采集，非手写常量）"
  BASELINE_FILE="$(mktemp)"; export BASELINE_FILE
  baseline_blob > "$BASELINE_FILE"
  python3 - <<'PY'
import os, sys
base = open(os.environ['BASELINE_FILE'], encoding='utf-8').read().split('\n')
ok = True

def section_range(lines, start_pat, end_pat):
    s = None
    for i, l in enumerate(lines):
        if s is None and l.startswith(start_pat):
            s = i
        elif s is not None and l.startswith(end_pat):
            return s, i
    return s, len(lines)

def security_bullets_58_3(lines):
    s, e = section_range(lines, '#### 5.8.3 ', '### 5.9 ')
    if s is None:
        return -1
    marker = None
    for i in range(e - 1, s - 1, -1):
        if lines[i].strip() == '**安全特性**:':
            marker = i
            break
    if marker is None:
        return -2
    n = 0
    for i in range(marker + 1, e):
        if lines[i].startswith('- '):
            # 折叠行（含字面 \n）不算「可见的合法 bullet」——它的第 3 条被
            # 3060 字符的陈旧副本截断在同一行里，渲染上等于不存在。
            if '\\n' not in lines[i]:
                n += 1
        elif lines[i].strip() == '':
            break
    return n

folded = [i for i, l in enumerate(base) if '\\n' in l and '#### 5.6.1' in l]
if len(folded) != 1:
    print('RED_FOLDED_LINE_UNIQUE=0')
    print('RESULT: FAIL')
    sys.exit(1)
t = folded[0]
line = base[t]
literal_lines = sum(1 for l in base if '\\n' in l)
bullets = security_bullets_58_3(base)
stale_title_map = sum(line.count(f'#### 5.6.{n}') for n in (1, 2, 3))

print(f'RED_FOLDED_LINE_UNIQUE=1')
print(f'RED_TARGET_LINE={t+1}')
print(f'RED_LITERAL_BSLASH_N_LINES={literal_lines}')
print(f'RED_TARGET_LINE_LEN={len(line)}')
print(f'RED_EXPANDED_LINES={len(line.split(chr(92) + "n"))}')
print(f'RED_SECURITY_BULLETS_58_3={bullets}')
print(f'RED_STALE_TITLE_MAP={stale_title_map}')

if not (literal_lines == 1 and len(line) == 3060 and bullets == 2 and stale_title_map == 3):
    ok = False
print('RESULT: PASS' if ok else 'RESULT: FAIL')
sys.exit(0 if ok else 1)
PY
  local rc=$?
  rm -f "$BASELINE_FILE"
  return $rc
}

r4() {
  require_python
  provenance
  echo "# source=fix^ 折叠块展开 vs 修复后全文（集合差 + 逐行 rubric 映射）"
  BASELINE_FILE="$(mktemp)"; export BASELINE_FILE
  baseline_blob > "$BASELINE_FILE"
  export TARGET_FILE="$ROOT/$TARGET"
  python3 - <<'PY'
import os, re, sys
base = open(os.environ['BASELINE_FILE'], encoding='utf-8').read().split('\n')
head = open(os.environ['TARGET_FILE'], encoding='utf-8').read().split('\n')
head_set = set(head)
ok = True

folded = [i for i, l in enumerate(base) if '\\n' in l and '#### 5.6.1' in l]
if len(folded) != 1:
    print('ERROR:FOLDED_LINE_NOT_UNIQUE')
    print('RESULT: FAIL')
    sys.exit(1)
segs = base[folded[0]].split('\\n')
retained = segs[0]
stale = segs[1:]

stale_only = [s for s in stale if s not in head_set]

def norm_curl(s):
    s = s.replace('\\\\', '\\')
    return re.sub(r'\s+', ' ', s).strip()

head_curls = {norm_curl(l) for l in head if l.strip().startswith('curl ')}

def first_two_cells(s):
    cells = [c.strip() for c in s.split('|')]
    return tuple(cells[1:3]) if len(cells) >= 3 else None

head_cells = {first_two_cells(l) for l in head if l.startswith('| ')}

def param_of(s):
    m = re.match(r'^- `([A-Za-z0-9_]+)`', s)
    return m.group(1) if m else None

head_params = {param_of(l) for l in head if param_of(l)}

unmapped = []
for s in stale_only:
    target = None
    m = re.match(r'^#### 5\.6\.(\d) (.+)$', s)
    if m:
        want = f'#### 5.8.{m.group(1)} {m.group(2)}'
        if want in head_set:
            target = want
    elif s.strip().startswith('curl '):
        if norm_curl(s) in head_curls:
            target = 'curl（现行单反斜杠变体）'
    elif s.startswith('| '):
        c = first_two_cells(s)
        if c and c in head_cells:
            target = '| ' + ' | '.join(c) + ' |（现行行）'
    elif param_of(s):
        if param_of(s) in head_params:
            target = f'- `{param_of(s)}`（现行条目）'
    if target is None:
        unmapped.append(s)
        print(f'STALE_UNMAPPED_LINE|{s}')
    else:
        print(f'STALE|{s}|-> {target}')

stale_title_map = sum(base[folded[0]].count(f'#### 5.6.{n}') for n in (1, 2, 3))
retained_present = 1 if retained in head_set else 0

print(f'STALE_ONLY_LINES={len(stale_only)}')
print(f'STALE_TITLE_MAP={stale_title_map}')
print(f'STALE_UNMAPPED={len(unmapped)}')
print(f'RETAINED_BULLET_PRESENT={retained_present}')
print(f'EXPANDED_LINES={len(segs)}')

if not (len(stale_only) == 12 and stale_title_map == 3 and not unmapped and retained_present == 1):
    ok = False
print('RESULT: PASS' if ok else 'RESULT: FAIL')
sys.exit(0 if ok else 1)
PY
  local rc=$?
  rm -f "$BASELINE_FILE"
  return $rc
}

case "${1:-}" in
  r1) r1 ;;
  r2) r2 ;;
  r3) r3 ;;
  r4) r4 ;;
  *)
    echo "Usage: bash scripts/verify-2591.sh <r1|r2|r3|r4>" >&2
    exit 2
    ;;
esac
