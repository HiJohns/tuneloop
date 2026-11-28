#!/usr/bin/env bash
# verify-2647.sh — Evidence Contract 执行器（#2647，Rev.2）。
#
# 用法：bash scripts/verify-2647.sh <r2|r7|r8|r9|r10|r11|all>
#
#   r2   docs/evidence/2647/R2-write-sites.txt        写点登记集合等值（8 点）+ 变量形态负例
#   r7   docs/evidence/2647/R7-users-roles-jsonb.txt  users.roles jsonb 写形状（静态 + 运行期）
#   r8   docs/evidence/2647/R8-role-change-mirror.txt 改角色同步镜像（写 → 读一致）
#   r9   docs/evidence/2647/R9-mirror-source.txt      镜像来源可判定 + 同步不降级
#   r10  docs/evidence/2647/R10-r2-redgreen.txt       R2 红绿实证（红样：改角色只写 role）
#   r11  docs/evidence/2647/R11-token-provenance.txt  8 个写点的 token 来源可追溯
#
# 每个 artifact 首行为 provenance：
#   # build=1.0.<fix短码> # commit=<fix 完整哈希> # dirty=<bool>
# 随后是可 grep 的 TOKEN=VALUE 断言行。
#
# 环境（可用环境变量覆盖）：POSTGRES_HOST=127.0.0.1 POSTGRES_PORT=15433
# POSTGRES_USER=tuneloop_user POSTGRES_PASSWORD=trust
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT" || exit 2

ISSUE=2647
EDIR="docs/evidence/$ISSUE"
HANDLERS_TREE="backend/handlers"
WORK="$ROOT/tmp/evidence-$ISSUE"
mkdir -p "$EDIR" "$WORK"

export PATH="/usr/local/go/bin:$PATH"
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TMPDIR="${TMPDIR:-$ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"

HEAVY_RUN="$HOME/.config/opencode/scripts/heavy-run.sh"
run_heavy() {
  if [ -x "$HEAVY_RUN" ]; then
    bash "$HEAVY_RUN" --scope host --timeout "${HEAVY_LOCK_TIMEOUT:-1800}" -- "$@"
  else
    "$@"
  fi
}

COMMIT="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
[ -n "$COMMIT" ] || COMMIT="$(git rev-parse HEAD)"
SHORT="$(git rev-parse --short=7 "$COMMIT")"
BUILD="1.0.$SHORT"
HEAD_SHA="$(git rev-parse HEAD)"

# 写点登记（D8）：path<TAB>form<TAB>occurrence-index（同 path+form 内按行号升序，1-based）。
# 登记键不含行号 —— 行号随编辑漂移；artifact 打印实际 file:line 供人工核对。
REGISTRY="$(cat <<'TSV'
backend/handlers/iam_callback.go	map-assign	1
backend/handlers/iam_proxy.go	struct-literal	1
backend/handlers/iam_proxy.go	map-assign	1
backend/handlers/merchant.go	map-assign	1
backend/handlers/merchant_member.go	struct-literal	1
backend/handlers/merchant_member.go	struct-literal	2
backend/handlers/user_staff.go	field-assign	1
backend/handlers/user_staff.go	map-assign	1
TSV
)"

# allowlist（D1）：3 个建号初始化写点 —— models.User 字面量只写 Role、不写 Roles。
# 理由：建号初始化值确定（site_admin / site_member），读侧有回退链，镜像由后续
# sync / 改角色刷新。ref = #2626 D1/D7。
ALLOWLIST="$(cat <<'TSV'
backend/handlers/site.go	site_admin
backend/handlers/site.go	site_member
backend/handlers/site_member_common.go	site_member
TSV
)"

dirty_flag() {
  local d
  d="$(git status --porcelain -- ':!docs/evidence' ':!tmp' 2>/dev/null)"
  if [ -n "$d" ]; then echo "true"; else echo "false"; fi
}

provenance() {
  printf '# build=%s # commit=%s # dirty=%s\n# tree=%s' \
    "$BUILD" "$COMMIT" "$(dirty_flag)" "$HEAD_SHA"
}

header() {
  local artifact="$1"
  mkdir -p "$(dirname "$artifact")"
  provenance > "$artifact"
  printf '\n' >> "$artifact"
}

# extract_tokens <log>：把 t.Logf 的 TOKEN=VALUE 行（go test 前缀 `.go:NNN:`）归一化。
extract_tokens() {
  local log="$1"
  grep -E '[A-Z][A-Z0-9_]+=' "$log" \
    | sed -E 's/^.*\.go:[0-9]+: //' \
    | grep -E '^[A-Z][A-Z0-9_]+=' \
    | awk '!seen[$0]++'
}

# ---------------------------------------------------------------------------
# scan_sites <root> <tree> <out-json>
#
# 按 4 种写法穷举 users.roles 写点，并给出每个站点的「写表达式 + 派生来源」：
#   struct-literal   models.User{ ... Roles: <expr> ... }
#   field-assign     <v>.Roles = <expr>，<v> 声明为 models.User
#   map-assign       setUserRolesMirror(<m>, <expr>)（唯一 jsonb 出口）
#                    或 <m>["roles"] = <expr>，<m> 经 Updates 落到 models.User
#   map-literal-key  <m> := map[string]interface{}{... "roles": <expr> ...}（同上）
# 非 users 的 roles 写入逐条进 EXCLUDED 段并标注原因；role 写点（["role"] / "role":）
# 与其镜像写配对判定 has_mirror。
scan_sites() { # <root> <tree> <out-json>
  python3 - "$1" "$2" "$3" <<'PY'
import json
import os
import re
import sys

root, tree, out_path = sys.argv[1], sys.argv[2], sys.argv[3]
hdir = os.path.join(root, tree)


def strip_comments(line):
    """Drop // comments (strings kept, so `"roles"` keys stay matchable)."""
    buf, i, in_str = [], 0, False
    while i < len(line):
        ch = line[i]
        if in_str:
            if ch == '\\':
                buf.append(ch)
                i += 1
                if i < len(line):
                    buf.append(line[i])
                    i += 1
                continue
            if ch == '"':
                in_str = False
            buf.append(ch)
            i += 1
            continue
        if ch == '"':
            in_str = True
            buf.append(ch)
            i += 1
            continue
        if ch == '/' and i + 1 < len(line) and line[i + 1] == '/':
            break
        buf.append(ch)
        i += 1
    return ''.join(buf)


def strip_code(line):
    """Drop // comments and double-quoted strings so brace counting stays sane."""
    buf, i, in_str = [], 0, False
    while i < len(line):
        ch = line[i]
        if in_str:
            if ch == '\\':
                i += 2
                continue
            if ch == '"':
                in_str = False
            i += 1
            continue
        if ch == '"':
            in_str = True
            i += 1
            continue
        if ch == '/' and i + 1 < len(line) and line[i + 1] == '/':
            break
        buf.append(ch)
        i += 1
    return ''.join(buf)


def bal(text):
    return text.count('{') - text.count('}')


def block_end(code, start_idx):
    depth, j = bal(code[start_idx]), start_idx
    while depth > 0 and j + 1 < len(code):
        j += 1
        depth += bal(code[j])
    return j


def func_ranges(code):
    out, i = [], 0
    while i < len(code):
        if re.match(r'^\s*func\s', code[i]):
            j = block_end(code, i)
            nm = re.search(r'func\s+(?:\([^)]*\)\s*)?(\w+)', code[i])
            out.append((i + 1, j + 1, nm.group(1) if nm else '?'))
            i = j + 1
        else:
            i += 1
    return out


def enclosing(lineno, ranges):
    for start, end, name in ranges:
        if start <= lineno <= end:
            return name, start, end
    return '?', 1, len(ranges) and 10 ** 9 or 10 ** 9


def ident_model(ident, upto, fs, raw):
    """Resolve <ident>'s declared model type inside the enclosing function."""
    pats = [
        re.compile(r'\b' + re.escape(ident) + r'\s*:?=\s*&?models\.(\w+)\{'),
        re.compile(r'\bvar\s+' + re.escape(ident) + r'\s+\*?models\.(\w+)'),
        re.compile(r'\b' + re.escape(ident) + r'\s+\*?models\.(\w+)\b'),
    ]
    for k in range(min(upto, fs + 5000) - 1, fs - 2, -1):
        for pat in pats:
            m = pat.search(raw[k])
            if m:
                return m.group(1)
    return 'unknown'


def model_of_expr(expr, upto, fs, raw):
    expr = expr.strip().lstrip('&')
    m = re.match(r'models\.(\w+)\{', expr)
    if m:
        return m.group(1)
    m = re.match(r'^([A-Za-z_]\w*)$', expr)
    if m:
        return ident_model(m.group(1), upto, fs, raw)
    return 'unknown'


def map_model(ident, fs, fe, raw):
    """Model targeted by Updates(<ident>) inside the enclosing function."""
    call = re.compile(r'Updates\(\s*' + re.escape(ident) + r'\s*[,)]')
    for k in range(fs - 1, min(fe, len(raw))):
        if call.search(raw[k]):
            for kk in range(k, fs - 2, -1):
                m = re.search(r'Model\(&([^)]+)\)', raw[kk])
                if m:
                    return model_of_expr(m.group(1), kk + 1, fs, raw)
            return 'unknown'
    return 'unknown'


def derive_source(expr, fs, fe, raw):
    """Which symbol the written roles expression comes from, and how it was produced."""
    expr = expr.strip().rstrip(',')
    m = re.match(r'^([A-Za-z_]\w*)\.(All|all)$', expr)
    if m:
        var = m.group(1)
        for k in range(fs - 1, min(fe, len(raw))):
            if re.search(r'\b' + re.escape(var) + r'\s*(=|\:=)\s*services\.DeriveRoles\(', raw[k]):
                return var, 'DeriveRoles'
        return var, 'unknown'
    if expr.startswith('[]string{') or re.match(r'^"', expr):
        return expr, 'literal'
    return expr, 'unknown'


BUSINESS_ROLES = {'site_member', 'site_admin', 'repair_technician',
                  'merchant_staff', 'merchant_admin', 'namespace_admin'}

sites, excluded, role_assigns, allowlist_hits = [], [], [], []

for fname in sorted(os.listdir(hdir)):
    if not fname.endswith('.go') or fname.endswith('_test.go'):
        continue
    rel = os.path.join(tree, fname)
    with open(os.path.join(hdir, fname), encoding='utf-8') as fh:
        raw = fh.read().split('\n')
    code = [strip_code(line) for line in raw]
    nc = [strip_comments(line) for line in raw]
    ranges = func_ranges(code)

    # ---- struct literals -------------------------------------------------
    for idx0, line in enumerate(nc):
        m = re.search(r'models\.(\w+)\{', line)
        if not m:
            continue
        model = m.group(1)
        end = block_end(code, idx0)
        has_roles = False
        for k in range(idx0, end + 1):
            fld = re.match(r'^\s*Roles:\s*(.+?),?\s*$', nc[k])
            if not fld:
                continue
            has_roles = True
            lineno = k + 1
            if model == 'User':
                fname_, fs, fe = enclosing(lineno, ranges)
                src, kind = derive_source(fld.group(1), fs, fe, raw)
                sites.append({'path': rel, 'line': lineno, 'form': 'struct-literal',
                              'model': model, 'expr': fld.group(1).strip(),
                              'func': fname_, 'source': src, 'source_kind': kind})
            else:
                excluded.append({'path': rel, 'line': lineno, 'model': model,
                                 'reason': 'non-users struct literal'})
        if model == 'User' and not has_roles:
            for k in range(idx0, end + 1):
                fld = re.match(r'^\s*Role:\s*"([^"]+)"', nc[k])
                if fld:
                    role = fld.group(1)
                    # D1 只管业务角色码（镜像目标）；IAM 级 role 原文（USER/STAFF…）
                    # 属 #2626 的 IAM 术语口径，单独留痕、不计入 ALLOWLIST_ENTRIES。
                    allowlist_hits.append({
                        'path': rel, 'line': k + 1, 'role': role,
                        'kind': 'business' if role in BUSINESS_ROLES else 'iam',
                    })

    # ---- field assigns ---------------------------------------------------
    for idx0, line in enumerate(nc):
        m = re.match(r'^\s*([A-Za-z_]\w*)\.Roles\s*=\s*(.+)$', line)
        if not m:
            continue
        lineno = idx0 + 1
        fname_, fs, fe = enclosing(lineno, ranges)
        model = ident_model(m.group(1), lineno, fs, raw)
        if model == 'User':
            src, kind = derive_source(m.group(2), fs, fe, raw)
            sites.append({'path': rel, 'line': lineno, 'form': 'field-assign',
                          'model': model, 'expr': m.group(2).strip(),
                          'func': fname_, 'source': src, 'source_kind': kind})
        else:
            excluded.append({'path': rel, 'line': lineno, 'model': model,
                             'reason': 'non-users field assignment'})

    # ---- setUserRolesMirror(<map>, <expr>) -------------------------------
    for idx0, line in enumerate(nc):
        if re.search(r'^func\s+setUserRolesMirror', line):
            continue
        m = re.search(r'setUserRolesMirror\(\s*([A-Za-z_]\w*)\s*,\s*([^)]+)\)', line)
        if not m:
            continue
        lineno = idx0 + 1
        fname_, fs, fe = enclosing(lineno, ranges)
        src, kind = derive_source(m.group(2), fs, fe, raw)
        sites.append({'path': rel, 'line': lineno, 'form': 'map-assign',
                      'model': 'User', 'expr': m.group(2).strip(),
                      'func': fname_, 'source': src, 'source_kind': kind,
                      'map': m.group(1)})

    # ---- <map>["roles"] = <expr> -----------------------------------------
    for idx0, line in enumerate(nc):
        m = re.search(r'([A-Za-z_]\w*)\["roles"\]\s*=\s*(.+)$', line)
        if not m:
            continue
        ident, expr = m.group(1), m.group(2)
        lineno = idx0 + 1
        fname_, fs, fe = enclosing(lineno, ranges)
        model = map_model(ident, fs, fe, raw)
        if model == 'User':
            src, kind = derive_source(expr, fs, fe, raw)
            sites.append({'path': rel, 'line': lineno, 'form': 'map-assign',
                          'model': model, 'expr': expr.strip(),
                          'func': fname_, 'source': src, 'source_kind': kind,
                          'map': ident})
        else:
            excluded.append({'path': rel, 'line': lineno, 'model': model,
                             'reason': 'non-users map assignment'})

    # ---- <map> := map[string]interface{}{ ... "roles": <expr> ... } ------
    for idx0, line in enumerate(nc):
        m = re.search(r'([A-Za-z_]\w*)\s*:=\s*map\[string\]interface\{\}\{', line)
        if not m:
            continue
        ident = m.group(1)
        end = block_end(code, idx0)
        for k in range(idx0, end + 1):
            if not re.search(r'"roles"\s*:', nc[k]):
                continue
            lineno = k + 1
            fname_, fs, fe = enclosing(lineno, ranges)
            model = map_model(ident, fs, fe, raw)
            expr = re.split(r'"roles"\s*:\s*', raw[k], 1)[1].strip().rstrip(',')
            if model == 'User':
                src, kind = derive_source(expr, fs, fe, raw)
                sites.append({'path': rel, 'line': lineno, 'form': 'map-literal-key',
                              'model': model, 'expr': expr,
                              'func': fname_, 'source': src, 'source_kind': kind,
                              'map': ident})
            else:
                excluded.append({'path': rel, 'line': lineno, 'model': model,
                                 'reason': 'non-users map literal key'})

    # ---- role writes paired with the mirror ------------------------------
    role_hits = []
    for idx0, line in enumerate(nc):
        m = re.search(r'([A-Za-z_]\w*)\["role"\]\s*=\s*(.+)$', line)
        if m:
            role_hits.append((m.group(1), idx0 + 1, m.group(2).strip()))
        m = re.search(r'([A-Za-z_]\w*)\s*:=\s*map\[string\]interface\{\}\{', line)
        if m:
            end = block_end(code, idx0)
            for k in range(idx0, end + 1):
                mk = re.search(r'"role"\s*:\s*(.+?),?\s*$', nc[k])
                if mk:
                    role_hits.append((m.group(1), k + 1, mk.group(1).strip()))
    for ident, lineno, expr in sorted(role_hits, key=lambda t: t[1]):
        fname_, fs, fe = enclosing(lineno, ranges)
        model = map_model(ident, fs, fe, raw)
        if model != 'User':
            continue
        has_mirror = any(s.get('map') == ident and s['func'] == fname_ for s in sites)
        role_assigns.append({'path': rel, 'line': lineno, 'func': fname_,
                             'map': ident, 'expr': expr, 'has_mirror': has_mirror})

result = {
    'tree': tree,
    'sites': sites,
    'excluded': excluded,
    'allowlist_hits': allowlist_hits,
    'role_assigns': role_assigns,
}
with open(out_path, 'w', encoding='utf-8') as fh:
    json.dump(result, fh, ensure_ascii=False, indent=1)
print('SCANNED tree=%s sites=%d excluded=%d allowlist=%d role_assigns=%d'
      % (tree, len(sites), len(excluded), len(allowlist_hits), len(role_assigns)))
PY
}

# ---------------------------------------------------------------------- R2
row_r2() {
  local artifact="$EDIR/R2-write-sites.txt"
  header "$artifact"
  local rc=0 sites_json="$WORK/sites-2647.json"
  scan_sites "$ROOT" "$HANDLERS_TREE" "$sites_json" >> "$artifact" 2>&1 || {
    echo "ASSERT_FAIL: SCANNER_ERROR" >> "$artifact"
    return 1
  }

  local regfile="$WORK/registry-2647.tsv"
  printf '%s\n' "$REGISTRY" > "$regfile"

  local stats enum unregistered stale
  stats="$(python3 - "$sites_json" "$regfile" "$artifact" <<'PY'
import json
import sys

json_path, registry_path, artifact = sys.argv[1], sys.argv[2], sys.argv[3]
data = json.load(open(json_path, encoding='utf-8'))
sites = data['sites']

registry = []
for line in open(registry_path, encoding='utf-8'):
    line = line.rstrip('\n')
    if not line.strip():
        continue
    path, form, idx = line.split('\t')
    registry.append((path, form, int(idx)))

buckets = {}
for s in sorted(sites, key=lambda x: (x['path'], x['line'])):
    buckets.setdefault((s['path'], s['form']), []).append(s)

keyed = []
for (path, form), rows in buckets.items():
    for i, s in enumerate(rows, 1):
        keyed.append((path, form, i, s))

detected = {(p, f, i) for p, f, i, _ in keyed}
registered = set(registry)
unregistered = sorted(detected - registered)
stale = sorted(registered - detected)

with open(artifact, 'a', encoding='utf-8') as fh:
    fh.write('## REGISTRY（D8：8 个语句级写点，等值断言）\n')
    for p, f, i, s in sorted(keyed, key=lambda t: (t[0], t[1], t[2])):
        fh.write('SITE %s:%d form=%s idx=%d expr=%s source=%s(%s) func=%s registered=%s\n'
                 % (p, s['line'], f, i, s['expr'], s['source'], s['source_kind'],
                    s['func'], 'yes' if (p, f, i) in registered else 'NO'))
    fh.write('## EXCLUDED（非 users.roles 写点，穷举留痕）\n')
    for e in sorted(data['excluded'], key=lambda x: (x['path'], x['line'])):
        fh.write('EXCLUDED %s:%d model=%s reason=%s\n'
                 % (e['path'], e['line'], e['model'], e['reason']))
    fh.write('## ALLOWLIST（D1：建号初始化业务角色写点，reason=建号初始化 ref=#2626 D1/D7）\n')
    for a in data['allowlist_hits']:
        if a['kind'] == 'business':
            fh.write('ALLOWLIST %s:%d role=%s\n' % (a['path'], a['line'], a['role']))
    fh.write('## INIT_IAM_ROLE（IAM 级 role 原文初始化，D1 不计；#2626 术语口径）\n')
    for a in data['allowlist_hits']:
        if a['kind'] != 'business':
            fh.write('INIT_IAM_ROLE %s:%d role=%s\n' % (a['path'], a['line'], a['role']))
    for p, f, i in unregistered:
        fh.write('ASSERT_FAIL: WRITE_SITES_UNREGISTERED=%s form=%s idx=%d\n' % (p, f, i))
    for p, f, i in stale:
        fh.write('ASSERT_FAIL: WRITE_SITES_STALE=%s form=%s idx=%d\n' % (p, f, i))

print('ENUM=%d UNREGISTERED=%d STALE=%d' % (len(detected), len(unregistered), len(stale)))
PY
)"
  echo "REGISTRY_CHECK $stats" >> "$artifact"
  enum="$(printf '%s' "$stats" | sed -n 's/.*ENUM=\([0-9]*\).*/\1/p')"
  unregistered="$(printf '%s' "$stats" | sed -n 's/.*UNREGISTERED=\([0-9]*\).*/\1/p')"
  stale="$(printf '%s' "$stats" | sed -n 's/.*STALE=\([0-9]*\).*/\1/p')"

  local inline ram allow_found allow_unlinked=0
  inline="$(python3 - "$sites_json" <<'PY'
import json
import re
import sys
data = json.load(open(sys.argv[1], encoding='utf-8'))
pat = re.compile(r'^(?:\[\]string\{[\s\S]*\}|"[^"]*")$')
print(sum(1 for s in data['sites'] if pat.match(s['expr'].strip())))
PY
)"
  [ "$inline" = "0" ] || { echo "ASSERT_FAIL: WRITE_INLINE_LITERAL=$inline" >> "$artifact"; rc=1; }

  python3 - "$sites_json" >> "$artifact" <<'PY'
import json
import sys
data = json.load(open(sys.argv[1], encoding='utf-8'))
for r in data['role_assigns']:
    print('ROLE_ASSIGN %s:%d map=%s mirror=%s expr=%s'
          % (r['path'], r['line'], r['map'], 'yes' if r['has_mirror'] else 'NO', r['expr']))
print('ROLE_ASSIGN_COUNT=%d' % len(data['role_assigns']))
PY
  ram="$(python3 - "$sites_json" <<'PY'
import json
import sys
data = json.load(open(sys.argv[1], encoding='utf-8'))
print(sum(1 for r in data['role_assigns'] if not r['has_mirror']))
PY
)"
  [ "$ram" = "0" ] || { echo "ASSERT_FAIL: ROLE_ASSIGN_WITHOUT_MIRROR=$ram" >> "$artifact"; rc=1; }

  # delta-only 登记豁免（Rev.2 ②）：SyncUsers 的 role 写是「与既有值不同才写」的
  # 增量写法，镜像写同源于同一 `mirror` 变量（同一 derivation），按登记豁免 —— 但
  # 豁免本身必须可核验（守卫行存在 + 该站点在册）。
  local delta_guard delta_registered delta_exempt=0
  delta_guard="$(grep -c 'if existingUser.Role != mirror.Primary' backend/handlers/iam_proxy.go || true)"
  delta_registered="$(printf '%s\n' "$REGISTRY" | grep -c 'backend/handlers/iam_proxy.go	map-assign	1' || true)"
  if [ "$delta_guard" -ge 1 ] && [ "$delta_registered" = "1" ]; then
    delta_exempt=1
  fi
  echo "DELTA_ONLY_SITE=backend/handlers/iam_proxy.go delta_guard=$delta_guard registered=$delta_registered" >> "$artifact"
  [ "$delta_exempt" = "1" ] || { echo "ASSERT_FAIL: DELTA_ONLY_EXEMPT=0" >> "$artifact"; rc=1; }

  allow_found="$(python3 - "$sites_json" <<'PY'
import json
import sys
data = json.load(open(sys.argv[1], encoding='utf-8'))
print(sum(1 for a in data['allowlist_hits'] if a['kind'] == 'business'))
PY
)"
  while IFS=$'\t' read -r path role; do
    [ -n "$path" ] || continue
    if ! python3 - "$sites_json" "$path" "$role" <<'PY'
import json
import sys
data = json.load(open(sys.argv[1], encoding='utf-8'))
path, role = sys.argv[2], sys.argv[3]
raise SystemExit(0 if [a for a in data['allowlist_hits']
                       if a['kind'] == 'business'
                       and a['path'] == path and a['role'] == role] else 1)
PY
    then
      allow_unlinked=$((allow_unlinked + 1))
      echo "ASSERT_FAIL: ALLOWLIST_UNLINKED=$path role=$role" >> "$artifact"
      rc=1
    fi
  done <<< "$ALLOWLIST"
  local allow_expected
  allow_expected="$(printf '%s\n' "$ALLOWLIST" | sed '/^$/d' | wc -l | tr -d ' ')"
  if [ "$allow_found" != "$allow_expected" ]; then
    echo "ASSERT_FAIL: ALLOWLIST_ENTRIES=$allow_found (expected $allow_expected)" >> "$artifact"
    rc=1
  fi

  # 负例自检（变量形态）：合成 `updates["role"] = roleVar` 无镜像 → 必须被判缺镜像
  local negtree="$WORK/negtree" neg_json="$WORK/sites-negative-2647.json" neg_control
  rm -rf "$negtree"
  mkdir -p "$negtree/$HANDLERS_TREE"
  cat > "$negtree/$HANDLERS_TREE/zz_negative_2647.go" <<'GO'
package handlers

func syntheticVarNegative2647(db *gorm.DB, updates map[string]interface{}, roleVar string) {
	updates["role"] = roleVar
	db.Model(&models.User{}).Where("id = ?", roleVar).Updates(updates)
}
GO
  scan_sites "$negtree" "$HANDLERS_TREE" "$neg_json" > /dev/null 2>&1 || true
  neg_control="$(python3 - "$neg_json" <<'PY'
import json
import sys
try:
    data = json.load(open(sys.argv[1], encoding='utf-8'))
except Exception:
    print(0)
    raise SystemExit(0)
print(1 if [r for r in data['role_assigns'] if not r['has_mirror']] else 0)
PY
)"
  [ "$neg_control" = "1" ] || { echo "ASSERT_FAIL: R2_NEGATIVE_CONTROL_VAR=0" >> "$artifact"; rc=1; }

  {
    echo "WRITE_SITES_ENUM=$enum"
    echo "WRITE_SITES_UNREGISTERED=$unregistered"
    echo "WRITE_SITES_STALE=$stale"
    echo "WRITE_INLINE_LITERAL=$inline"
    echo "ROLE_ASSIGN_WITHOUT_MIRROR=$ram"
    echo "DELTA_ONLY_EXEMPT=$delta_exempt"
    echo "R2_NEGATIVE_CONTROL_VAR=$neg_control"
    echo "ALLOWLIST_ENTRIES=$allow_found"
    echo "ALLOWLIST_UNLINKED=$allow_unlinked"
  } >> "$artifact"

  [ "$rc" = "0" ] || return 1
  [ "$enum" = "8" ] || return 1
  [ "$unregistered" = "0" ] || return 1
  [ "$stale" = "0" ] || return 1
  return 0
}

# ---------------------------------------------------------------------- R7
row_r7() {
  local artifact="$EDIR/R7-users-roles-jsonb.txt" log="$WORK/r7.log"
  header "$artifact"
  ( cd backend && run_heavy go test ./handlers -run 'TestUserRolesMirrorJSONB_2647' -count=1 -v ) > "$log" 2>&1
  local rc=$?
  cat "$log" >> "$artifact"
  extract_tokens "$log" >> "$artifact"

  local sites_json="$WORK/sites-2647.json"
  if [ ! -s "$sites_json" ]; then
    scan_sites "$ROOT" "$HANDLERS_TREE" "$sites_json" > /dev/null 2>&1 || true
  fi

  local jsonb_sites raw_sites expr_ok=0
  jsonb_sites="$(python3 - "$sites_json" <<'PY'
import json
import sys
data = json.load(open(sys.argv[1], encoding='utf-8'))
print(sum(1 for s in data['sites'] if s['form'] == 'map-assign'))
PY
)"
  raw_sites="$(grep -rnE '\["roles"\]\s*=\s*[A-Za-z_][A-Za-z0-9_]*\.All' backend/handlers/*.go 2>/dev/null \
      | grep -v '_test.go' | wc -l | tr -d ' ')"
  [ "$jsonb_sites" -ge 4 ] 2>/dev/null && expr_ok=1
  if [ "$expr_ok" != "1" ]; then
    echo "ASSERT_FAIL: USERS_MIRROR_JSONB_EXPR=0 (map sites=$jsonb_sites)" >> "$artifact"
    rc=1
  fi
  if [ "$raw_sites" != "0" ]; then
    echo "ASSERT_FAIL: USERS_MIRROR_RAW_SLICE=$raw_sites" >> "$artifact"
    rc=1
  fi
  {
    echo "USERS_MIRROR_MAP_SITES=$jsonb_sites"
    echo "USERS_MIRROR_JSONB_EXPR=$expr_ok"
    echo "USERS_MIRROR_RAW_SLICE=$raw_sites"
  } >> "$artifact"

  [ "$rc" = "0" ] || return 1
  grep -Fq 'JSONB_TYPE_IS_ARRAY=1' "$artifact" || return 1
  grep -Fq 'JSONB_IS_NULL=0' "$artifact" || return 1
  grep -Fq 'RAW_SLICE_WRITE_ERROR=1' "$artifact" || return 1
  return 0
}

# ---------------------------------------------------------------------- R8
row_r8() {
  local artifact="$EDIR/R8-role-change-mirror.txt" log="$WORK/r8.log"
  header "$artifact"
  ( cd backend && run_heavy go test ./handlers -run 'TestUpdateUserRoleMirror_2647' -count=1 -v ) > "$log" 2>&1
  local rc=$?
  cat "$log" >> "$artifact"
  extract_tokens "$log" >> "$artifact"
  [ "$rc" = "0" ] || return 1
  grep -Fq 'HTTP=200' "$artifact" || return 1
  grep -Fq 'ROLE_ASSIGN_WITHOUT_MIRROR=0' "$artifact" || return 1
  grep -Fq 'MIRROR_PRIMARY=site_admin' "$artifact" || return 1
  grep -Fq 'MIRROR_ALL_CONTAINS_SITE_ADMIN=1' "$artifact" || return 1
  grep -Fq 'MIRROR_ALL_CONTAINS_OLD=1' "$artifact" || return 1
  grep -Fq 'READBACK_PREFERS_MIRROR=1' "$artifact" || return 1
  return 0
}

# ---------------------------------------------------------------------- R9
row_r9() {
  local artifact="$EDIR/R9-mirror-source.txt" log="$WORK/r9.log"
  header "$artifact"
  ( cd backend && run_heavy go test ./handlers -run 'TestSyncUsersMirrorNonDegrade_2647' -count=1 -v ) > "$log" 2>&1
  local rc=$?
  cat "$log" >> "$artifact"
  extract_tokens "$log" >> "$artifact"

  local sites_json="$WORK/sites-2647.json"
  if [ ! -s "$sites_json" ]; then
    scan_sites "$ROOT" "$HANDLERS_TREE" "$sites_json" > /dev/null 2>&1 || true
  fi

  local derive_ok derive_unreg second source_ok=0
  derive_ok="$(python3 - "$sites_json" <<'PY'
import json
import sys
data = json.load(open(sys.argv[1], encoding='utf-8'))
print(sum(1 for s in data['sites'] if s['source_kind'] == 'DeriveRoles'))
PY
)"
  derive_unreg="$(python3 - "$sites_json" <<'PY'
import json
import sys
data = json.load(open(sys.argv[1], encoding='utf-8'))
for s in data['sites']:
    if s['source_kind'] != 'DeriveRoles':
        print('SOURCE_UNAVAILABLE %s:%d expr=%s kind=%s'
              % (s['path'], s['line'], s['expr'], s['source_kind']))
print(sum(1 for s in data['sites'] if s['source_kind'] != 'DeriveRoles'))
PY
)"
  second="$(grep -rnE '\["roles"\]\s*=\s*[A-Za-z_][A-Za-z0-9_]*\.All' backend/handlers/*.go 2>/dev/null \
      | grep -v '_test.go' | wc -l | tr -d ' ')"
  echo "$derive_unreg" | sed '$d' >> "$artifact"
  local derive_missing
  derive_missing="$(printf '%s\n' "$derive_unreg" | tail -1)"
  if [ "$derive_missing" = "0" ] && [ "$derive_ok" -ge 8 ] 2>/dev/null; then
    source_ok=1
  fi
  if [ "$source_ok" != "1" ]; then
    echo "ASSERT_FAIL: MIRROR_SOURCE_DERIVE_ONLY=0 (ok=$derive_ok missing=$derive_missing)" >> "$artifact"
    rc=1
  fi
  if [ "$second" != "0" ]; then
    echo "ASSERT_FAIL: WRITE_SIDE_SECOND_ASSIGNMENT=$second" >> "$artifact"
    rc=1
  fi
  {
    echo "MIRROR_SOURCE_SITES=$derive_ok"
    echo "MIRROR_SOURCE_DERIVE_ONLY=$source_ok"
    echo "WRITE_SIDE_SECOND_ASSIGNMENT=$second"
  } >> "$artifact"

  [ "$rc" = "0" ] || return 1
  grep -Fq 'SYNC_SKIPS_NONEMPTY_MIRROR_OVERWRITE=1' "$artifact" || return 1
  grep -Fq 'SYNC_DEGRADES_NONEMPTY_MIRROR=0' "$artifact" || return 1
  # 测试保真：镜像没降级不能是因为写库失败被吞 —— 本次同步必须真的写成功。
  grep -Fq 'SYNC_CONFLICTS=0' "$artifact" || return 1
  if ! grep -Eq 'SYNC_WRITES_SUCCEEDED=[1-9]' "$artifact"; then
    echo "ASSERT_FAIL: SYNC_WRITE_NOT_EXERCISED（镜像未降级可能是写失败被掩盖）" >> "$artifact"
    return 1
  fi
  return 0
}

# --------------------------------------------------------------------- R10
# 红样：把 UpdateUser 的改角色分支还原成缺陷版（只写 updates["role"]，不派生镜像）。
# 绿样：真实树。同一个扫描器跑两棵树 —— 红样不红即 FAIL。
row_r10() {
  local artifact="$EDIR/R10-r2-redgreen.txt"
  header "$artifact"
  local rc=0 redtree="$WORK/redtree"
  rm -rf "$redtree"
  mkdir -p "$redtree/$HANDLERS_TREE"
  cp backend/handlers/*.go "$redtree/$HANDLERS_TREE/"

  python3 - "$redtree/$HANDLERS_TREE/user_staff.go" "$artifact" <<'PY'
import sys

path, artifact = sys.argv[1], sys.argv[2]
src = open(path, encoding='utf-8').read()

before = src
src = src.replace(
    '\t\tunion := appendUniqueRoles(existingUser.Roles, newRole)\n'
    '\t\tmirror := services.DeriveRoles("", services.ToIAMRole(newRole), union)\n'
    '\t\tupdates["role"] = mirror.Primary\n',
    '\t\tupdates["role"] = newRole\n',
)
if src == before:
    print('RED_PATCH_FAIL: derive block not found', file=sys.stderr)
    sys.exit(1)

marker = '\t\tif err := setUserRolesMirror(updates, mirror.All); err != nil {'
start = src.find(marker)
if start < 0:
    print('RED_PATCH_FAIL: mirror block not found', file=sys.stderr)
    sys.exit(1)
line_start = src.rfind('\n', 0, start) + 1
i = src.index('{', start)
depth = 0
while i < len(src):
    if src[i] == '{':
        depth += 1
    elif src[i] == '}':
        depth -= 1
        if depth == 0:
            i += 1
            break
    i += 1
end = i
while end < len(src) and src[end] in ' \t':
    end += 1
if end < len(src) and src[end] == '\n':
    end += 1
src = src[:line_start] + src[end:]
open(path, 'w', encoding='utf-8').write(src)
print('RED_PATCH applied to UpdateUser (updates["role"] only, no mirror)')
PY
  [ "$?" = "0" ] || { echo "ASSERT_FAIL: RED_PATCH_FAILED" >> "$artifact"; return 1; }

  local red_json="$WORK/sites-red-2647.json" green_json="$WORK/sites-2647.json"
  scan_sites "$redtree" "$HANDLERS_TREE" "$red_json" > /dev/null 2>&1 || true
  if [ ! -s "$green_json" ]; then
    scan_sites "$ROOT" "$HANDLERS_TREE" "$green_json" > /dev/null 2>&1 || true
  fi

  local red_counts green_counts red_count green_count ok=0
  red_counts="$(python3 - "$red_json" <<'PY'
import json
import sys
data = json.load(open(sys.argv[1], encoding='utf-8'))
for r in data['role_assigns']:
    print('RED_SITE %s:%d map=%s mirror=%s expr=%s'
          % (r['path'], r['line'], r['map'],
             'yes' if r['has_mirror'] else 'NO', r['expr']))
print(sum(1 for r in data['role_assigns'] if not r['has_mirror']))
PY
)"
  red_count="$(printf '%s\n' "$red_counts" | tail -1)"
  printf '%s\n' "$red_counts" | sed '$d' >> "$artifact"

  green_counts="$(python3 - "$green_json" <<'PY'
import json
import sys
data = json.load(open(sys.argv[1], encoding='utf-8'))
for r in data['role_assigns']:
    print('GREEN_SITE %s:%d map=%s mirror=%s expr=%s'
          % (r['path'], r['line'], r['map'],
             'yes' if r['has_mirror'] else 'NO', r['expr']))
print(sum(1 for r in data['role_assigns'] if not r['has_mirror']))
PY
)"
  green_count="$(printf '%s\n' "$green_counts" | tail -1)"
  printf '%s\n' "$green_counts" | sed '$d' >> "$artifact"

  if [ "$red_count" -gt 0 ] 2>/dev/null && [ "$green_count" = "0" ]; then
    ok=1
  fi
  {
    echo "RED_ROLE_WITHOUT_MIRROR=$red_count"
    echo "GREEN_ROLE_WITHOUT_MIRROR=$green_count"
    echo "REDGREEN_OK=$ok"
  } >> "$artifact"
  [ "$ok" = "1" ] || rc=1
  return "$rc"
}

# --------------------------------------------------------------------- R11
# 每个登记写点的写表达式必须追溯到同一个派生来源（services.DeriveRoles）。
row_r11() {
  local artifact="$EDIR/R11-token-provenance.txt"
  header "$artifact"
  local rc=0 sites_json="$WORK/sites-2647.json"
  if [ ! -s "$sites_json" ]; then
    scan_sites "$ROOT" "$HANDLERS_TREE" "$sites_json" > /dev/null 2>&1 || true
  fi

  python3 - "$sites_json" "$artifact" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding='utf-8'))
artifact = sys.argv[2]
sites = sorted(data['sites'], key=lambda s: (s['path'], s['line']))
rows = ['TOKEN path=%s line=%d form=%s expr=%s source=%s source_kind=%s'
        % (s['path'], s['line'], s['form'], s['expr'], s['source'], s['source_kind'])
        for s in sites]
with open(artifact, 'a', encoding='utf-8') as fh:
    fh.write('## PROVENANCE（每行 = 一个写点的 token 来源）\n')
    fh.write('\n'.join(rows) + '\n')
PY

  local enum unreg provfile=0
  enum="$(grep -c '^TOKEN path=' "$artifact" || true)"
  unreg="$(grep '^TOKEN path=' "$artifact" | grep -v 'source_kind=DeriveRoles' | wc -l | tr -d ' ')"
  if [ "$enum" != "8" ]; then
    echo "ASSERT_FAIL: TOKEN_SOURCE_ENUM=$enum" >> "$artifact"
    rc=1
  fi
  if [ "$unreg" != "0" ]; then
    echo "ASSERT_FAIL: TOKEN_SOURCE_UNREGISTERED=$unreg" >> "$artifact"
    rc=1
  fi
  if [ -s "$artifact" ] && [ "$enum" = "8" ] && [ "$(grep -c '^TOKEN path=' "$artifact" || true)" = "8" ]; then
    provfile=1
  fi
  if [ "$provfile" != "1" ]; then
    echo "ASSERT_FAIL: PROVENANCE_FILE=0" >> "$artifact"
    rc=1
  fi
  {
    echo "TOKEN_SOURCE_ENUM=$enum"
    echo "TOKEN_SOURCE_UNREGISTERED=$unreg"
    echo "PROVENANCE_FILE=$provfile"
  } >> "$artifact"
  return "$rc"
}

ROW="${1:-}"
case "$ROW" in
  all)
    FAIL=0
    for r in r2 r7 r8 r9 r10 r11; do
      if "row_$r" > /dev/null 2>&1; then
        echo "$r: PASS"
      else
        echo "$r: FAIL"
        FAIL=1
      fi
    done
    exit "$FAIL"
    ;;
  r2|r7|r8|r9|r10|r11) ;;
  *)
    echo "usage: $0 <r2|r7|r8|r9|r10|r11|all>" >&2
    exit 2
    ;;
esac

rc=0
"row_$ROW" || rc=$?
if [ "$rc" = "0" ]; then echo "$ROW: PASS"; else echo "$ROW: FAIL"; fi
exit "$rc"
