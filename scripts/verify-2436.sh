#!/usr/bin/env bash
# #2436 Rev.3：确认订单页合计标签中文 + i18n + 中文排版字距口径（干净证据版）。
# 用法：bash scripts/verify-2436.sh <r0-red|r1-scan|r1b-scan|r2-smoke|r3-typography>
# 产物写入 OUT_DIR（默认 docs/evidence/2436）；隔离 worktree 生成时把 OUT_DIR 指到树外，
# 使 provenance 的 dirty=false 为真实快照（dirty 判定含 untracked）。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r0-red) ART_NAME="R0-red.txt" ;;
  r1-scan) ART_NAME="R1-scan.txt" ;;
  r1b-scan) ART_NAME="R2-dynamic-scan.txt" ;;
  r2-smoke) ART_NAME="R3-smoke.txt" ;;
  r3-typography) ART_NAME="R4-typography.md" ;;
  r4-provenance) ART_NAME="R5-provenance.txt" ;;
  r5-build) ART_NAME="R6-build.txt" ;;
  r6-clean) ART_NAME="R7-clean.txt" ;;
  *) echo "usage: $0 <r0-red|r1-scan|r1b-scan|r2-smoke|r3-typography|r4-provenance|r5-build|r6-clean>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
OUT_DIR="${OUT_DIR:-$REPO_ROOT/docs/evidence/2436}"
ART="$OUT_DIR/$ART_NAME"
mkdir -p "$OUT_DIR"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp}"
mkdir -p "$TMPDIR"

BUILD="1.0.$(git rev-parse --short HEAD)"
COMMIT="$(git rev-parse HEAD)"
DIRTY=false
# #2436 Rev.3 D3：dirty 必须包含 untracked，且不得整体排除证据目录。
if [ -n "$(git status --porcelain -uall)" ]; then
  DIRTY=true
fi
PROVENANCE="# build=$BUILD commit=$COMMIT dirty=$DIRTY"

run_vitest() { # $1 = optional -t pattern
  local pattern="${1:-}"
  cd "$REPO_ROOT/frontend-mobile"
  if [ -n "$pattern" ]; then
    npx vitest run --config vitest.config.js -t "$pattern" scripts/page-smoke/no-uppercase-label-2436.test.jsx
  else
    npx vitest run --config vitest.config.js scripts/page-smoke/no-uppercase-label-2436.test.jsx
  fi
}

case "$MODE" in
  r0-red)
    # 红绿实证：pre-fix 2eed6f8a9^ 的两个页面 + 当前（fix 后）的测试用例 → 必须 FAIL。
    RED_ROOT="$TMPDIR/2436-red/src"
    rm -rf "$TMPDIR/2436-red"
    mkdir -p "$RED_ROOT/pages" "$RED_ROOT/pages-weapp"
    git show 2eed6f8a9^:frontend-mobile/src/pages/Checkout.jsx > "$RED_ROOT/pages/Checkout.jsx" || {
      echo "R0_SETUP_FAIL: cannot read pre-fix Checkout.jsx"; exit 1; }
    git show 2eed6f8a9^:frontend-mobile/src/pages-weapp/Checkout.jsx > "$RED_ROOT/pages-weapp/Checkout.jsx" || {
      echo "R0_SETUP_FAIL: cannot read pre-fix weapp Checkout.jsx"; exit 1; }
    {
      echo "$PROVENANCE"
      echo "# R0 红例：pre-fix 2eed6f8a9^ + 当前测试用例（期望 FAIL）"
      echo "## pre-fix 残留"
      grep -n "TOTAL PAYABLE" "$RED_ROOT/pages/Checkout.jsx" "$RED_ROOT/pages-weapp/Checkout.jsx" || {
        echo "R0_SETUP_FAIL: pre-fix 源码未含 TOTAL PAYABLE"; exit 1; }
      echo "## pre-fix 无 t('应付合计')"
      if grep -rnF "t('应付合计')" "$RED_ROOT"; then
        echo "R0_SETUP_FAIL: pre-fix 已含 t('应付合计')（红例不成立）"; exit 1
      fi
      echo "PREFIX_HIT: TOTAL PAYABLE 存在且无 t('应付合计')"
      echo "## 运行 fix 后的静态用例（SCAN_ROOT=pre-fix）"
      export SCAN_ROOT="$RED_ROOT"
      inner_rc=0
      run_vitest || inner_rc=$?
      echo "INNER_EXIT=$inner_rc"
      if [ "$inner_rc" = "0" ]; then
        echo "RED_NOT_CONFIRMED: pre-fix 用例竟通过"
      else
        echo "RED_CONFIRMED: pre-fix 用例失败（红）"
      fi
    } 2>&1 | tee "$ART"
    if grep -q "RED_CONFIRMED" "$ART"; then exit 0; fi
    exit 1
    ;;

  r1-scan)
    {
      echo "$PROVENANCE"
      echo "## 裸英文扫描"
      if grep -rn "TOTAL PAYABLE" frontend-mobile/src frontend-pc/src 2>/dev/null; then
        echo "SCAN_FAIL: TOTAL PAYABLE 残留"; exit 1
      fi
      echo "SCAN_OK: 无 TOTAL PAYABLE"
      echo "## t() 调用"
      for f in frontend-mobile/src/pages/Checkout.jsx frontend-mobile/src/pages-weapp/Checkout.jsx; do
        grep -nF "t('应付合计')" "$f" || { echo "T_FAIL: $f"; exit 1; }
      done
      echo "## 静态用例（R1+R2 两例）"
      run_vitest
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;

  r1b-scan)
    {
      echo "$PROVENANCE"
      echo "## 动态/样式大写扫描（uppercase + 文案字面量）"
      if grep -rn "uppercase" frontend-mobile/src 2>/dev/null; then
        echo "DYNAMIC_FAIL: uppercase 残留"; exit 1
      fi
      run_vitest "R2:"
      echo "DYNAMIC_SCAN_OK: 0 未登记 uppercase/动态英文大写"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;

  r2-smoke)
    {
      echo "$PROVENANCE"
      echo "## page-smoke（4 条单乐器路由「应付总额」+ Batch 专用「应付合计」）"
      ( cd frontend-mobile && npx vitest run --config vitest.config.js scripts/page-smoke/checkout-batch-total-2436.test.jsx )
      brc=$?
      echo "BATCH_TEST_EXIT=$brc"
      [ "$brc" = "0" ] || exit "$brc"
      ( cd frontend-mobile && npm run smoke )
      rc=$?
      echo "SMOKE_EXIT=$rc"
      [ "$rc" = "0" ] || exit "$rc"
      echo "## checkout markers"
      python3 - "$REPO_ROOT/frontend-mobile/scripts/page-smoke/routes.js" <<'PY'
import re, sys
src = open(sys.argv[1], encoding='utf-8').read()
blocks = re.findall(r"name:\s*'([^']*checkout[^']*)'.*?markers:\s*\[([^\]]*)\]", src, re.S)
missing = []
for name, body in blocks:
    if '应付总额' not in body:
        missing.append(name)
if missing:
    print('MARKERS_FAIL: ' + ', '.join(missing))
    sys.exit(1)
print('MARKERS_OK: ' + ', '.join(n for n, _ in blocks))
PY
      [ $? -eq 0 ] || exit 1
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;

  r4-provenance)
    # R5：隔离树干净 + 证据三字段一致（证据文件默认取 OUT_DIR，生成期不污染仓库）。
    EV_FILE="${EVIDENCE_JSON:-$OUT_DIR/evidence.json}"
    {
      echo "$PROVENANCE"
      echo "## provenance 自证（隔离树 + 证据一致性）"
      fail=0
      status="$(git status --porcelain -uall)"
      if [ -n "$status" ]; then
        echo "STATUS_DIRTY:"; printf '%s\n' "$status" | head -5; fail=1
      else
        echo "STATUS_CLEAN"
      fi
      HEAD_SHA="$(git rev-parse HEAD)"
      HEAD_SHORT="$(git rev-parse --short HEAD)"
      echo "HEAD=$HEAD_SHA"
      if [ -f "$EV_FILE" ]; then
        python3 - "$EV_FILE" "$HEAD_SHA" "1.0.$HEAD_SHORT" "$REPO_ROOT" <<'PYEOF' || fail=1
import json, os, sys
ev_path, head, build, root = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
ev = json.load(open(ev_path, encoding='utf-8'))
fail = 0
for k, v in (('commit', head), ('build', build), ('dirty', False)):
    got = ev.get(k)
    if got != v:
        print(f'EV_MISMATCH: {k}={got!r} want {v!r}'); fail = 1
for row in ev.get('rows', []):
    for art in row.get('artifacts', []):
        p = os.path.join(root, art)
        alt = os.path.join(os.environ.get('OUT_DIR', ''), os.path.basename(art))
        if os.path.exists(alt):
            p = alt
        if not os.path.exists(p):
            print(f'ARTIFACT_MISSING: {art}'); fail = 1; continue
        first = open(p, encoding='utf-8').readline().strip()
        want = f'# build={build} commit={head} dirty=false'
        if first != want:
            print(f'PROVENANCE_MISMATCH: {art}: {first}'); fail = 1
print('EVIDENCE_OK' if fail == 0 else 'EVIDENCE_FAIL')
sys.exit(fail)
PYEOF
      else
        echo "EVIDENCE_MISSING: $EV_FILE"; fail=1
      fi
      [ "$fail" = "0" ] && echo "PROVENANCE_OK" || echo "PROVENANCE_FAIL"
    } 2>&1 | tee "$ART"
    grep -q "PROVENANCE_OK" "$ART"
    exit $?
    ;;

  r5-build)
    # R6：独立 UI Issue 的双端构建 gate（H5 vite + weapp Taro）
    {
      echo "$PROVENANCE"
      echo "## node"
      node -v 2>/dev/null || true
      case "$(node -v 2>/dev/null)" in
        v22.*) echo "NODE22_OK" ;;
        *) echo "NODE22_MISSING: build:weapp requires Node 22 (Taro v4)"; exit 1 ;;
      esac
      echo "## H5 build"
      ( cd frontend-mobile && NODE_OPTIONS="--max-old-space-size=2048" npm run build )
      h5=$?
      echo "H5_BUILD_EXIT=$h5"
      [ "$h5" = "0" ] || { echo "BUILD_FAIL"; exit 1; }
      echo "## weapp build"
      ( cd frontend-mobile && NODE_OPTIONS="--max-old-space-size=2048" npm run build:weapp )
      wa=$?
      echo "WEAPP_BUILD_EXIT=$wa"
      [ "$wa" = "0" ] || { echo "WEAPP_BUILD_FAIL"; exit 1; }
      echo "BUILD_OK"
      echo "WEAPP_BUILD_OK"
    } 2>&1 | tee "$ART"
    grep -q "BUILD_OK" "$ART" && grep -q "WEAPP_BUILD_OK" "$ART"
    exit $?
    ;;

  r6-clean)
    # R7：旧 artifact 清理校验（git 索引 + 当前证据集）
    {
      echo "$PROVENANCE"
      echo "## 旧 artifact 不得在 git 索引中"
      fail=0
      for old in docs/evidence/2436/R2-smoke.txt docs/evidence/2436/R3-typography.md; do
        if git ls-files --error-unmatch "$old" >/dev/null 2>&1; then
          echo "STALE_INDEXED: $old"; fail=1
        else
          echo "REMOVED_OK: $old"
        fi
      done
      echo "## 当前证据集"
      for f in R0-red.txt R1-scan.txt R2-dynamic-scan.txt R3-smoke.txt R4-typography.md R5-provenance.txt R6-build.txt evidence.json; do
        if [ -f "docs/evidence/2436/$f" ] || [ -f "$OUT_DIR/$f" ]; then
          echo "PRESENT: $f"
        else
          echo "MISSING: $f"; fail=1
        fi
      done
      [ "$fail" = "0" ] && echo "CLEAN_OK" || echo "CLEAN_FAIL"
    } 2>&1 | tee "$ART"
    grep -q "CLEAN_OK" "$ART"
    exit $?
    ;;

  r3-typography)
    {
      echo "$PROVENANCE"
      echo "# R4 — 中文合计标签字距口径"
      echo
      echo "- 结论：中文标签不保留英文大写排版字距（uppercase/tracking-widest/letterSpacing）。"
      fail=0
      h5="$(grep -nF "应付合计" frontend-mobile/src/pages/Checkout.jsx || true)"
      if printf '%s' "$h5" | grep -qE "uppercase|tracking-widest"; then
        echo "- FAIL: H5 标签仍含 uppercase/tracking-widest"; fail=1
      else
        echo "- PASS: H5 标签无 uppercase/tracking-widest"
      fi
      weapp="$(grep -nF "应付合计" frontend-mobile/src/pages-weapp/Checkout.jsx || true)"
      if printf '%s' "$weapp" | grep -q "letterSpacing"; then
        echo "- FAIL: weapp 标签仍含 letterSpacing"; fail=1
      else
        echo "- PASS: weapp 标签无 letterSpacing"
      fi
      if grep -rn "TOTAL PAYABLE" frontend-mobile/src frontend-pc/src 2>/dev/null; then
        echo "- FAIL: TOTAL PAYABLE 残留"; fail=1
      else
        echo "- PASS: 无 TOTAL PAYABLE"
      fi
      echo
      echo "## 证据"
      printf '%s\n' "$h5" "$weapp"
      [ "$fail" = "0" ] && echo "TYPOGRAPHY_OK" || echo "TYPOGRAPHY_FAIL"
    } 2>&1 | tee "$ART"
    if grep -q "TYPOGRAPHY_FAIL" "$ART"; then exit 1; fi
    exit 0
    ;;
esac
