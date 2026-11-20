#!/usr/bin/env bash
# #2427：人员筛选——直属开关即时生效 + site_id 空值省略/UUID 校验。
# 用法：bash scripts/verify-2427.sh {r1|r2|r3|r4|r5|r6|r7}
set -euo pipefail
MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4|r5|r6|r7) ;;
  *) echo "usage: $0 {r1|r2|r3|r4|r5|r6|r7}" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"

BUILD="1.0.$(git rev-parse --short HEAD)"
COMMIT="$(git rev-parse HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2427/' || true)" ]; then
  DIRTY=true
fi
echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"

SITE_MARK='site_id: values.siteId || null'
STAFF_MARK='handleDirectToggle'

case "$MODE" in
  r1)
    # 前端查询构造单测：siteId null/'' 省略；direct 仅 true；page/page_size 保留。
    cd "$REPO_ROOT/frontend-pc"
    node --test src/utils/personnelQuery.test.js
    ;;

  r2)
    # 直属开关即时生效：Checkbox onChange → handleDirectToggle → handleSearch → 回第 1 页。
    # 负例：仍把空值塞进查询参数（旧写法）→ FAIL。
    F="$REPO_ROOT/frontend-pc/src/pages/StaffManagement.jsx"
    if grep -qF "$SITE_MARK" "$F"; then
      echo "FAIL: 仍存在 \`$SITE_MARK\`（空值依赖下游过滤的旧写法）" >&2
      exit 1
    fi
    grep -q 'onChange={e => handleDirectToggle(e.target.checked)}' "$F" \
      || { echo "FAIL: 直属 Checkbox 缺少 onChange → 不会即时重查" >&2; exit 1; }
    grep -q "const $STAFF_MARK = (checked) => {" "$F" \
      || { echo "FAIL: 缺少 handleDirectToggle" >&2; exit 1; }
    awk '/const handleDirectToggle/,/^  }/' "$F" | grep -q 'handleSearch(' \
      || { echo "FAIL: handleDirectToggle 未调用 handleSearch" >&2; exit 1; }
    awk '/const handleSearch =/,/^  }/' "$F" | grep -q 'buildPersonnelQuery(' \
      || { echo "FAIL: handleSearch 未走 buildPersonnelQuery" >&2; exit 1; }
    awk '/const handleSearch =/,/^  }/' "$F" | grep -q 'current: 1' \
      || { echo "FAIL: handleSearch 未回到第 1 页" >&2; exit 1; }
    grep -q 'form={searchForm}' "$F" \
      || { echo "FAIL: 搜索表单未绑定实例，onChange 无法读取当前筛选值" >&2; exit 1; }
    grep -q 'buildPersonnelQuery({' "$F" \
      || { echo "FAIL: 列表请求未使用 buildPersonnelQuery" >&2; exit 1; }
    echo "PROBE-OK: direct toggle immediate (onChange → handleDirectToggle → handleSearch → page 1); no site_id=null path"
    ;;

  r3)
    # 非法 site_id → 40002 + WARN，不把非法值带进 SQL 静默返回 0 行。
    cd "$REPO_ROOT/backend"
    go test ./handlers/ -run '^TestPersonnelFilter2427_InvalidSiteID_40002$' -count=1 -v
    ;;

  r4)
    # 省略 site_id → 全量（total>0）；合法 site_id 仍按网点过滤。
    cd "$REPO_ROOT/backend"
    go test ./handlers/ -run '^TestPersonnelFilter2427_(EmptySiteReturnsAll|ValidSiteFilterStillWorks)$' -count=1 -v
    ;;

  r5)
    # 同类扫描：URLSearchParams 直接展开的调用点清单 + 处理结论（D4）。
    echo "## URLSearchParams call sites (frontend-pc/src)"
    SCAN="$(grep -rn 'new URLSearchParams(' "$REPO_ROOT/frontend-pc/src" --include='*.js' --include='*.jsx' || true)"
    echo "$SCAN"
    echo
    # 人员管理链路禁止自行序列化（唯一入口 = utils/personnelQuery.js 的 personnelQueryString）。
    if echo "$SCAN" | grep -q 'src/pages/StaffManagement.jsx'; then
      echo "FAIL: StaffManagement 仍在自行序列化查询串" >&2
      exit 1
    fi
    # personnelQuery.js 中只允许 personnelQueryString 使用 URLSearchParams。
    PQ="$REPO_ROOT/frontend-pc/src/utils/personnelQuery.js"
    [ "$(grep -c 'new URLSearchParams(' "$PQ")" = "1" ] \
      || { echo "FAIL: personnelQuery.js 的 URLSearchParams 用法超出预期" >&2; exit 1; }
    grep -q 'export function personnelQueryString' "$PQ"
    echo "## 结论"
    echo "- 人员管理链路：已收口到 utils/personnelQuery.js（buildPersonnelQuery + personnelQueryString），无空值入参。"
    echo "- api.get / fetchBlob：common 层已过滤 null/undefined/''（本次不重复修改）。"
    echo "- 其余命中（如 instrumentsApi.list 直接展开 params 对象）：不在 #2427 范围（D4 仅记录）。"
    echo "PROBE-OK: personnel chain has a single serializer; remaining call sites recorded"
    ;;

  r6)
    # 双端构建 + lint。
    echo "## go build"
    cd "$REPO_ROOT/backend"
    go build ./...
    echo "## eslint (changed files)"
    cd "$REPO_ROOT/frontend-pc"
    npx eslint --no-warn-ignored src/pages/StaffManagement.jsx src/utils/personnelQuery.js src/utils/personnelQuery.test.js
    echo "## vite build"
    NODE_OPTIONS="--max-old-space-size=2048" npm run build
    echo "BUILD-OK: backend go build + pc eslint + pc vite build"
    ;;

  r7)
    # 人工回执（Owner/Architect 签）：勾选/取消即时生效；空网点搜索非空。
    F="$REPO_ROOT/docs/evidence/2427/R7-manual.md"
    [ -f "$F" ] || { echo "FAIL: 缺少人工回执 $F" >&2; exit 1; }
    if ! grep -qE '^RESULT: PASS$' "$F"; then
      echo "FAIL: 人工回执未通过（需 Owner/Architect 按步骤执行后写 RESULT: PASS）" >&2
      grep -E '^RESULT: ' "$F" >&2 || true
      exit 1
    fi
    grep -qE '^executor: (architect|owner|ops)$' "$F" \
      || { echo "FAIL: 回执缺少 executor 署名" >&2; exit 1; }
    echo "RECEIPT-OK: R7 manual receipt signed"
    ;;
esac
