#!/usr/bin/env bash
# #2354 R1-R3：weapp 首页搜索层层级 / 构建 + lint / 文档一致性。
#
# 用法：bash scripts/verify-2354.sh <layers|build|docs>
# 产物：docs/evidence/2354/R{1,2,3}-*.txt（首行盖 PROVENANCE）
#
# 背景：weapp `pages-weapp/Home.jsx` 的 banner 滑动层 zIndex=10002（未滚动时）
# 覆盖了搜索层 zIndex=10000，导致搜索栏点击被手势层吞掉（H5 为 10003 不受影响）。
# 修复 = 搜索层提到 10003（与 H5 对齐），滑动层逻辑不动。
set -euo pipefail

MODE="${1:-}"
case "$MODE" in layers|build|docs) ;; *) echo "usage: $0 layers|build|docs" >&2; exit 2 ;; esac

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"
mkdir -p docs/evidence/2354

WEAPP_HOME="frontend-mobile/src/pages-weapp/Home.jsx"
H5_HOME="frontend-mobile/src/pages/Home.jsx"
PROVENANCE="build=home-layers-2354 commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- "$WEAPP_HOME" "$H5_HOME" docs scripts)" && echo false || echo true)"

case "$MODE" in
  # ---------- R1：源码静态断言（层级关系） ----------
  layers)
    OUT=docs/evidence/2354/R1-layers.txt
    {
      echo "PROVENANCE $PROVENANCE"
      echo "CMD grep -nE 'zIndex|z-\[' $WEAPP_HOME $H5_HOME"
      echo "--- weapp search / swipe / sticky-menu ---"
      grep -nE "zIndex: (10002|10003|10004)|zIndex: scrolled \? 0 : 10002" "$WEAPP_HOME"
      echo "--- H5 search / swipe / sticky-menu ---"
      grep -nE "zIndex: (10002|10003|10004)|z-\[1000[0-9]\]" "$H5_HOME"
    } > "$OUT" 2>&1

    # 期望：weapp 搜索层 = 10003；weapp 滑动层仍为 10002（且仍是 scrolled ? 0 : 10002 语义）
    grep -q "zIndex: 10003" "$WEAPP_HOME" \
      || { echo "R1 FAIL: weapp search layer is not zIndex 10003" >&2; exit 1; }
    grep -q "zIndex: scrolled ? 0 : 10002" "$WEAPP_HOME" \
      || { echo "R1 FAIL: weapp swipe layer no longer uses scrolled ? 0 : 10002" >&2; exit 1; }

    # 负例：旧值 10000 不得残留在 weapp 首页（否则搜索层又被覆盖）
    if grep -q "zIndex: 10000" "$WEAPP_HOME"; then
      echo "R1 FAIL: legacy zIndex 10000 still present in weapp home" >&2
      exit 1
    fi

    # 结构断言：搜索层（含 onClick 跳搜索页的 fixed 容器）必须是 10003
    python3 - "$WEAPP_HOME" <<'PY' || { echo "R1 FAIL: search container is not zIndex 10003" >&2; exit 1; }
import re, sys
src = open(sys.argv[1]).read()
# 抓取「A: Search button」注释之后的第一个 View style 块
m = re.search(r"A: Search button.*?zIndex:\s*(\d+)", src, re.S)
assert m, 'search container not found'
assert m.group(1) == '10003', f"search zIndex={m.group(1)}, expected 10003"
# 滑动层必须仍为 {scrolled ? 0 : 10002}
m2 = re.search(r"zIndex:\s*scrolled\s*\?\s*0\s*:\s*(\d+)", src)
assert m2 and m2.group(1) == '10002', 'swipe layer zIndex changed'
PY

    echo "PASS: weapp search layer 10003 > swipe layer 10002 (H5 10003 unchanged)"
    ;;

  # ---------- R2：weapp 构建 + eslint ----------
  build)
    OUT=docs/evidence/2354/R2-build-lint.txt
    NODE22="$(ls -d "$HOME"/.nvm/versions/node/v22*/bin 2>/dev/null | tail -1)"
    [ -n "$NODE22" ] || { echo "R2 FAIL: node 22 not found" >&2; exit 1; }
    export PATH="$NODE22:$PATH"
    cd frontend-mobile
    {
      echo "PROVENANCE $PROVENANCE"
      echo "CMD npm run build:weapp (heavy-lock) ; npx eslint --no-ignore src/pages-weapp/Home.jsx"
      echo "node=$(node -v)"
    } > "../$OUT"
    BUILD_SLICE_TIMEOUT="${BUILD_SLICE_TIMEOUT:-1800}" \
      ~/.config/opencode/scripts/build-slice bash -c "npm run build:weapp" >> "../$OUT" 2>&1
    npx eslint --no-ignore src/pages-weapp/Home.jsx >> "../$OUT" 2>&1
    cd ..
    grep -qE "^ERROR in|Module build failed" "$OUT" && { echo "R2 FAIL: weapp build error" >&2; exit 1; }
    grep -qE "[1-9][0-9]* errors?" "$OUT" && { echo "R2 FAIL: eslint errors" >&2; exit 1; }
    test -f frontend-mobile/dist-weapp/pages-weapp/home/index.js \
      || { echo "R2 FAIL: weapp home artifact missing" >&2; exit 1; }
    echo "PASS: weapp build exit 0 + eslint 0 errors"
    ;;

  # ---------- R3：文档 z-index 一致性 ----------
  docs)
    OUT=docs/evidence/2354/R3-docs.txt
    {
      echo "PROVENANCE $PROVENANCE"
      echo "CMD grep -nE '1000[0-9]' docs/topics/frontpage.md docs/spec/ui/ui.md"
      grep -nE "1000[0-9]" docs/topics/frontpage.md
      echo "--- ui.md ---"
      grep -nE "1000[0-9]" docs/spec/ui/ui.md
    } > "$OUT" 2>&1

    # 正向：两文档均记录搜索层 10003
    grep -q "10003" docs/topics/frontpage.md || { echo "R3 FAIL: frontpage.md missing search layer 10003" >&2; exit 1; }
    grep -q "10003" docs/spec/ui/ui.md        || { echo "R3 FAIL: ui.md missing search layer 10003" >&2; exit 1; }

    # 负例：文档层级表不得再把搜索层写成 10000
    if grep -nE "\| A \| 10000 \||Z=10000 " docs/topics/frontpage.md; then
      echo "R3 FAIL: frontpage.md still documents search layer as 10000" >&2
      exit 1
    fi
    if grep -nE "^\| 10000 \| 搜索" docs/spec/ui/ui.md; then
      echo "R3 FAIL: ui.md still documents search layer as 10000" >&2
      exit 1
    fi

    echo "PASS: both docs document search layer 10003 (no stale 10000)"
    ;;

esac
