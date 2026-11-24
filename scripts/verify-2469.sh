#!/usr/bin/env bash
# #2469 证据驱动：R1 纯函数用例、R2–R7 源码结构断言、R8 构建/lint、R9 gate/plan-source。
set -uo pipefail
MODE="${1:-}"
case "$MODE" in
  r1) NAME="R1-pure.txt" ;;
  r2) NAME="R2-empty-static.txt" ;;
  r3) NAME="R3-data-static.txt" ;;
  r4) NAME="R4-fields-static.txt" ;;
  r5) NAME="R5-loading-static.txt" ;;
  r6) NAME="R6-tooltip-static.txt" ;;
  r7) NAME="R7-purge-static.txt" ;;
  r8) NAME="R8-build.txt" ;;
  r9) NAME="R9-gate.txt" ;;
  *) echo "usage: $0 <r1..r9>" >&2; exit 2 ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO"
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/2469}"; mkdir -p "$OUT_DIR"
ART="$OUT_DIR/$NAME"

ISSUE=2469
EVIDENCE_DIR="docs/evidence/$ISSUE"
SRC="frontend-pc/src/pages/System/UserManagement.jsx"
UTIL="frontend-pc/src/utils/orphanPanel.js"
HEAD_SHA="$(git rev-parse HEAD)"
# provenance 锚定本单 fix commit（调用方传 FIX_REF=<fix-commit>）；证据收尾提交不再改变锚点。
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
DIRTY=false
[ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR")" ] && DIRTY=true
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
TREE_LINE="# tree=$HEAD_SHA"

# block <start-marker> <end-marker> <file> — 提取被注释标记包围的代码段
block() {
  awk -v s="$1" -v e="$2" 'index($0,s){f=1} f{print} f&&index($0,e){exit}' "$3"
}
# has <needle> <haystack-file-or-string via stdin>：命中输出 1，否则 0
count_hit() { grep -qF -- "$1" "$2" && echo 1 || echo 0; }

case "$MODE" in
  r1)
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      ( cd frontend-pc && node --test tests/orphanPanel.test.mjs ) 2>&1
      rc=${PIPESTATUS[0]}
      grep -q 'SHOW_EMPTY=false' "$ART" && grep -q 'SHOW_ONE=true' "$ART" \
        && grep -q 'ISEMPTY_LOADING=false' "$ART" && grep -q 'ISEMPTY_ERROR=false' "$ART" \
        && grep -q 'EMPTY_TEXT_HAS_SCANNED=1' "$ART" && grep -q 'TITLE_HIT=命中 1 个' "$ART" \
        && grep -q '^# fail 0$' "$ART" \
        && echo "MARKERS_OK" || { echo "MARKERS_MISSING"; rc=1; }
      exit "$rc"; } 2>&1 | tee "$ART"
    grep -q '^MARKERS_OK$' "$ART"; exit $? ;;

  r2)
    R2="$(block '#2469 R2 空态' '#2469 R3 有数据' "$SRC")"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      fail=0
      echo "$R2" > "$REPO/tmp/r2-block.txt"
      echo "ALERT_CONDITIONAL=$(grep -qF 'shouldShowWarning(orphans) && (' "$SRC" && echo 1 || echo 0)"
      grep -qF 'shouldShowWarning(orphans) && (' "$SRC" || fail=1
      empty_ok=1
      echo "$R2" | grep -qF '<Empty' || empty_ok=0
      echo "$R2" | grep -qF '未发现孤儿账号' || empty_ok=0
      echo "$R2" | grep -qF '>刷新<' || empty_ok=0
      echo "EMPTY_BLOCK=$empty_ok"; [ "$empty_ok" = 1 ] || fail=1
      empty_table_absent=1
      echo "$R2" | grep -qF '<Table' && empty_table_absent=0
      echo "$R2" | grep -qF 'rowSelection' && empty_table_absent=0
      echo "EMPTY_TABLE_ABSENT=$empty_table_absent"; [ "$empty_table_absent" = 1 ] || fail=1
      echo "$R2"
      [ "$fail" = 0 ] && echo "R2_OK" || echo "R2_FAIL"
    } 2>&1 | tee "$ART"
    grep -q '^R2_OK$' "$ART"; exit $? ;;

  r3)
    R3="$(block '#2469 R3 有数据' '</Modal>' "$SRC")"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      fail=0
      title_ok=0
      grep -qF 'title={orphanTitle(orphans, orphanScanned)}' "$SRC" && title_ok=1
      echo "TITLE_USES_helper=$title_ok"; [ "$title_ok" = 1 ] || fail=1
      one_line=1
      echo "$R3" | grep -qF '不可撤销' || one_line=0
      # 负例：旧长文/判定清单不得再出现在红条正文
      echo "$R3" | grep -qF '同邮箱/用户名重试' && one_line=0
      echo "$R3" | grep -qF '本地 users 表无本地记录' && one_line=0
      echo "WARNING_ONE_LINE=$one_line"; [ "$one_line" = 1 ] || fail=1
      echo "$R3"
      [ "$fail" = 0 ] && echo "R3_OK" || echo "R3_FAIL"
    } 2>&1 | tee "$ART"
    grep -q '^R3_OK$' "$ART"; exit $? ;;

  r4)
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      fail=0
      s=0; grep -qF 'setOrphanScanned(resp.data?.scanned)' "$SRC" && s=1
      echo "SET_SCANNED=$s"; [ "$s" = 1 ] || fail=1
      m=0; grep -qF 'setOrphanMinAge(resp.data?.min_age_minute)' "$SRC" && m=1
      echo "SET_MIN_AGE=$m"; [ "$m" = 1 ] || fail=1
      d=0; grep -qE '^\s+setOrphanScanned\(' "$SRC" && d=1
      echo "SCANNED_STATE_DECL=$d"; [ "$d" = 1 ] || fail=1
      [ "$fail" = 0 ] && echo "R4_OK" || echo "R4_FAIL"
    } 2>&1 | tee "$ART"
    grep -q '^R4_OK$' "$ART"; exit $? ;;

  r5)
    LOADING="$(block '#2469 R5 加载中' '#2469 R2 空态' "$SRC")"
    ERRBLOCK="$(block '#2469 R5 失败态' '#2469 R2 空态' "$SRC")"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      fail=0
      spin=1; echo "$LOADING" | grep -qF '<Spin' || spin=0
      echo "$LOADING" | grep -qF '正在扫描 IAM 账号' || spin=0
      echo "SPIN_BRANCH=$spin"; [ "$spin" = 1 ] || fail=1
      err=1; echo "$ERRBLOCK" | grep -qF 'type="error"' || err=0
      echo "ERROR_BRANCH=$err"; [ "$err" = 1 ] || fail=1
      absent=1; echo "$ERRBLOCK" | grep -vE '^\s*\{?/\*' | grep -qF '未发现孤儿账号' && absent=0
      echo "EMPTY_TEXT_ABSENT_IN_ERROR=$absent"; [ "$absent" = 1 ] || fail=1
      echo "--- loading block ---"; echo "$LOADING"
      echo "--- error block ---"; echo "$ERRBLOCK"
      [ "$fail" = 0 ] && echo "R5_OK" || echo "R5_FAIL"
    } 2>&1 | tee "$ART"
    grep -q '^R5_OK$' "$ART"; exit $? ;;

  r6)
    R3="$(block '#2469 R3 有数据' '</Modal>' "$SRC")"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      fail=0
      t=0; echo "$R3" | grep -qF '<Tooltip title={orphanConditions}>' && t=1
      echo "TOOLTIP=$t"; [ "$t" = 1 ] || fail=1
      c=1
      grep -qF '孤儿判定（四条全部满足）' "$SRC" || c=0
      grep -qF '无本地记录' "$SRC" || c=0
      grep -qF '无活跃组织关系' "$SRC" || c=0
      grep -qF '账号状态为 pending' "$SRC" || c=0
      grep -qF '已挂起 ≥' "$SRC" || c=0
      # 负例：判定四条不得出现在红条 Alert 的 message/description 中
      echo "$R3" | grep -qF 'description={' && c=0
      echo "$R3" | grep -qF '孤儿判定（四条全部满足）' && c=0
      echo "CONDITIONS_IN_TOOLTIP=$c"; [ "$c" = 1 ] || fail=1
      echo "--- data block ---"; echo "$R3"
      [ "$fail" = 0 ] && echo "R6_OK" || echo "R6_FAIL"
    } 2>&1 | tee "$ART"
    grep -q '^R6_OK$' "$ART"; exit $? ;;

  r7)
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      fail=0
      confirm=1
      grep -qF 'const confirmPurgeSelected = () => {' "$SRC" || confirm=0
      grep -qF 'onOk: () => purgeOrphans(orphanSelected)' "$SRC" || confirm=0
      grep -qF 'const confirmPurgeOne = (record) => {' "$SRC" || confirm=0
      echo "CONFIRM_KEPT=$confirm"; [ "$confirm" = 1 ] || fail=1
      batch=1
      grep -qF 'disabled={orphanSelected.length === 0}' "$SRC" || batch=0
      grep -qF '批量清理（{orphanSelected.length}）' "$SRC" || batch=0
      echo "BATCH_DISABLED_KEPT=$batch"; [ "$batch" = 1 ] || fail=1
      [ "$fail" = 0 ] && echo "R7_OK" || echo "R7_FAIL"
    } 2>&1 | tee "$ART"
    grep -q '^R7_OK$' "$ART"; exit $? ;;

  r8)
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      ( cd frontend-pc && npm run build ) 2>&1
      build_rc=${PIPESTATUS[0]}
      echo "PC_BUILD=$build_rc"
      ( cd frontend-pc && npx eslint src/pages/System/UserManagement.jsx src/utils/orphanPanel.js tests/orphanPanel.test.mjs ) 2>&1
      lint_rc=${PIPESTATUS[0]}
      echo "LINT_EXIT=$lint_rc"
      [ "$build_rc" = "0" ] && [ "$lint_rc" = "0" ] && echo "BUILD_OK" || echo "BUILD_FAIL"
    } 2>&1 | tee "$ART"
    grep -q '^BUILD_OK$' "$ART"; exit $? ;;

  r9)
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      fail=0
      GATE="$REPO/docs/evidence/2469/pre-ready-2469.json"
      overall="$(jq -r '.overall // empty' "$GATE" 2>/dev/null)"
      if [ -f "$GATE" ] && [ "$overall" = "PASS" ]; then
        echo "GATE_RESULT=PASS"
      elif [ ! -f "$GATE" ]; then
        echo "GATE_RESULT=PENDING(no report yet)"; fail=1
      else
        echo "GATE_RESULT=MISSING_OR_FAIL(${overall:-missing})"; fail=1
      fi
      p5=$(jq -r '.checks[]? | select(.id=="P5") | .status' "$GATE" 2>/dev/null | head -1)
      echo "P5=${p5:-MISSING}"
      if [ -n "$p5" ] && [ "$p5" != "N/A" ]; then
        echo "P5_DETAIL=$(jq -r '.checks[]? | select(.id=="P5") | .detail // ""' "$GATE" 2>/dev/null | head -1)"
      else
        echo "P5_IS_NA"; fail=1
      fi
      if [ "$p5" = "PASS" ]; then
        echo "FULL_SUITE_EXIT=0 (derived from P5 PASS)"
      else
        echo "FULL_SUITE_EXIT=PENDING"; fail=1
      fi
      gate_sha="$(jq -r '.plan_sha256 // empty' "$GATE" 2>/dev/null)"
      plan_sha="$(sha256sum docs/evidence/2469/plan.md | awk '{print $1}')"
      echo "GATE_PLAN_SHA=$gate_sha"
      if [ -n "$gate_sha" ] && [ "$gate_sha" = "$plan_sha" ]; then
        echo "GATE_PLAN_SHA_MATCH=1"
      else
        echo "GATE_PLAN_SHA_MATCH=0"; fail=1
      fi
      bash "$HOME/.config/opencode/scripts/plan-source.sh" --issue 2469 2>&1 | tail -6 | tee "$REPO/tmp/2469-r9-plan-source.log"
      if grep -q 'PLAN_SOURCE: PASS' "$REPO/tmp/2469-r9-plan-source.log"; then
        echo "PLAN_SOURCE=PASS"
      else
        echo "PLAN_SOURCE=FAIL"; fail=1
      fi
      [ "$fail" = "0" ] && echo "GATE_OK" || echo "GATE_FAIL"
    } 2>&1 | tee "$ART"
    grep -q '^GATE_OK$' "$ART"; exit $? ;;
esac
