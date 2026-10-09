#!/usr/bin/env bash
# h5-weapp-parity.sh — H5 ↔ weapp 一致性「机械侦察」快照（只读，可复跑）
# 用法: bash scripts/h5-weapp-parity.sh [> docs/topics/h5-weapp-parity.md]
# 目的: 产出 #2172 Phase 1 的机械快照——登记对称性 / 双实现 / 分叉点 / 组件分裂 /
#       平台层对称 / 交互热点 / 样式禁区 / wxml 探针。不修改任何业务代码。
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="$ROOT/frontend-mobile/src"
CFG="$SRC/app.config.ts"
APP_H5="$SRC/App-H5.jsx"
NAV="$SRC/platform/navigation.js"

hdr() { echo; echo "## $1"; echo; }

echo "# H5 ↔ weapp 一致性机械快照（自动生成）"
echo
echo "- 生成时间: $(date '+%F %T')"
echo "- 源: \`frontend-mobile/src\`"
echo "- 说明: 本文件由 \`scripts/h5-weapp-parity.sh\` 生成（只读侦察），**请勿手改**。"

# ---------- 1. 登记对称性 ----------
hdr "1. 登记对称性（h5Pages ↔ weappPages ↔ App-H5 路由 ↔ ROUTE_MAP）"
python3 - "$CFG" "$APP_H5" "$NAV" <<'PY'
import re, sys
cfg, apph5, nav = (open(p).read() for p in sys.argv[1:4])
def arr(name, s):
    m = re.search(name + r"\s*=\s*\[(.*?)\]", s, re.S)
    return re.findall(r"'([^']+)'", m.group(1)) if m else []
w = arr("weappPages", cfg); h = arr("h5Pages", cfg)
norm = lambda x: x.replace("pages-weapp/", "").replace("pages/", "").replace("/index", "")
W = {norm(x) for x in w}; H = {norm(x) for x in h}
print(f"- weappPages: {len(w)} 条目 | h5Pages: {len(h)} 条目")
print(f"- **weapp 有 / H5 无**（{len(W-H)}）: {', '.join(sorted(W-H)) or '无'}")
print(f"- **H5 有 / weapp 无**（{len(H-W)}）: {', '.join(sorted(H-W)) or '无'}")
routes = re.findall(r'<Route\s+path="([^"]+)"', apph5)
def first_seg(p):
    segs = [s for s in p.split('/') if s and not s.startswith(':')]
    return segs[0] if segs else ''
rmap = re.findall(r"match:\s*'([^']+)'", nav)
rmap_segs = {first_seg(m) for m in rmap}
route_segs = {first_seg(r) for r in routes}
missing = sorted(s for s in route_segs if s and s not in rmap_segs)
print(f"- App-H5 路由: {len(routes)} 条 | ROUTE_MAP: {len(rmap)} 条")
print(f"- **H5 路由无 weapp 映射（首段）**（{len(missing)}，候选：H5-only / 页面内显式跳转 / 遗漏，需人工判定）: {', '.join('/'+m for m in missing) or '无'}")
PY

# ---------- 2. 双实现清单 ----------
hdr "2. 双实现清单（pages/X.jsx ↔ pages-weapp/**/X.jsx，漂移风险最高）"
common=""
for wf in "$SRC"/pages-weapp/*.jsx "$SRC"/pages-weapp/*/*.jsx; do
  [ -f "$wf" ] || continue
  b=$(basename "$wf")
  [ -f "$SRC/pages/$b" ] && common="$common $wf|$b"
done
if [ -z "$common" ]; then echo "- 无"; else
  printf "| 页面 | H5 行数 | weapp 行数 | weapp 路径 | 行差 |\n|------|--------:|----------:|------------|-----:|\n"
  for pair in $common; do
    wf="${pair%%|*}"; b="${pair##*|}"
    a=$(wc -l < "$SRC/pages/$b"); c=$(wc -l < "$wf")
    printf "| %s | %s | %s | %s | %s |\n" "$b" "$a" "$c" "${wf#"$SRC/"}" "$((a>c?a-c:c-a))"
  done
fi

# ---------- 3. env.isMiniProgram 分叉点 ----------
hdr "3. 两端行为分叉点（env.isMiniProgram / isWeapp）"
n_files=$(grep -rlE "isMiniProgram" "$SRC/pages" "$SRC/pages-weapp" "$SRC/components" "$SRC/components-weapp" 2>/dev/null | wc -l)
n_hits=$(grep -roE "isMiniProgram" "$SRC/pages" "$SRC/pages-weapp" "$SRC/components" "$SRC/components-weapp" 2>/dev/null | wc -l)
echo "- 含分叉的文件: **$n_files** | 分叉点总数: **$n_hits**（每处都需判定：合理适配 / 潜在分歧）"

# ---------- 4. 组件分裂 ----------
hdr "4. 组件分裂（components/ vs components-weapp/）"
s="$(ls "$SRC"/components/*.jsx 2>/dev/null | xargs -n1 basename | sort)"
wl="$(ls "$SRC"/components-weapp/*.jsx 2>/dev/null | xargs -n1 basename | sort)"
both="$(comm -12 <(echo "$s") <(echo "$wl") | tr '\n' ' ')"
echo "- 双实现: ${both:-无}"
echo "- 仅 shared: $(comm -23 <(echo "$s") <(echo "$wl") | tr '\n' ' ')"
echo "- 仅 weapp: $(comm -13 <(echo "$s") <(echo "$wl") | tr '\n' ' ')"

# ---------- 5. 平台层对称 ----------
hdr "5. 平台抽象层对称（platform/browser.js ↔ platform/index.weapp.js）"
python3 - "$SRC" <<'PY'
import re, sys
src = sys.argv[1]
def exps(p):
    try: s = open(p).read()
    except Exception: return set()
    return set(re.findall(r"export\s+(?:const|function|async function)\s+([A-Za-z0-9_]+)", s))
b = exps(f"{src}/platform/browser.js"); w = exps(f"{src}/platform/index.weapp.js")
print(f"- browser: {len(b)} | weapp: {len(w)}")
print(f"- **仅 browser**: {', '.join(sorted(b-w)) or '无'}")
print(f"- **仅 weapp**: {', '.join(sorted(w-b)) or '无'}")
PY

# ---------- 6. 交互热点 ----------
hdr "6. 交互热点（每页交互密度，供 Phase 2.5 行为走查排期）"
printf "| 页面 | onClick | dialog | nav | useEffect | useDidShow | setState |\n|------|--------:|-------:|----:|----------:|-----------:|---------:|\n"
for f in "$SRC"/pages/*.jsx; do
  b=$(basename "$f")
  oc=$(grep -oE "onClick=" "$f" | wc -l)
  [ "$oc" -lt 3 ] && continue
  dg=$(grep -oE "dialog\.(alert|confirm)" "$f" | wc -l)
  nv=$(grep -oE "(navigate|switchTab|redirectTo|reLaunch)\(" "$f" | wc -l)
  ue=$(grep -oE "useEffect\(" "$f" | wc -l)
  uds=$(grep -oE "useDidShow\(" "$f" | wc -l)
  st=$(grep -oE "set[A-Z][A-Za-z]+\(" "$f" | wc -l)
  printf "| %s | %s | %s | %s | %s | %s | %s |\n" "$b" "$oc" "$dg" "$nv" "$ue" "$uds" "$st"
done | sort -t'|' -k3 -rn

# ---------- 7. 样式禁区探针（#1831）----------
hdr "7. weapp 样式禁区命中（#1831：硬禁区=必删类；软禁区=禁新增）"
echo "- 硬禁区（space-y/x、分数类）:"
hard=$(grep -rhoE '(space-[xy]-[0-9]+|[a-z]{1,6}-(1/2|1/3|2/3|1/4|3/4|1/5|2/5|3/5|4/5|1/6|5/6))' "$SRC/pages" "$SRC/pages-weapp" "$SRC/components" "$SRC/components-weapp" 2>/dev/null | sort | uniq -c | sort -rn)
[ -n "$hard" ] && echo "$hard" | sed 's/^/  /' || echo "  - 0 命中"
echo "- 软禁区（变体类 / 任意值）:"
soft=$(grep -rhoE '(active:|hover:|focus:|first:|last:|sm:|md:|lg:|xl:|(text|bg|w|h|max-h|min-h|top|left|right|bottom|gap|border|rounded|leading|tracking|p|px|py|pt|pb|pl|pr|mx|my|mt|mb|ml|mr)-\[[^]]+\])' "$SRC/pages" "$SRC/pages-weapp" "$SRC/components" "$SRC/components-weapp" 2>/dev/null | sort | uniq -c | sort -rn | head -18)
[ -n "$soft" ] && echo "$soft" | sed 's/^/  /' || echo "  - 0 命中"

# ---------- 8. wxml 原生标签探针（可选，需 dist-weapp）----------
hdr "8. weapp 产物原生标签探针（需先 npm run build:weapp）"
if [ -d "$ROOT/frontend-mobile/dist-weapp" ]; then
  out=$(grep -rlE '<(div|span|img|button|input|textarea|p|select|a)\b' "$ROOT/frontend-mobile/dist-weapp" --include='*.wxml' 2>/dev/null | grep -v '/base.wxml$')
  if [ -z "$out" ]; then echo "- ✅ 无原生 HTML 交互/媒体标签（通过）"; else echo "- ❌ 命中（weapp 不渲染）:"; echo "$out" | sed 's/^/  - /'; fi
else
  echo "- ⏭ 跳过（无 dist-weapp，先 \`cd frontend-mobile && npm run build:weapp\`）"
fi

echo
echo "---"
echo "_本快照仅为机械化事实，\"合理适配 vs 分歧\" 判定见白名单与 #2172 矩阵。_"
