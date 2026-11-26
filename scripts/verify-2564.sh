#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2564.sh — 证据生成器（#2564 分类页 40002 重名表单内持久提示）
#
# 行定义（Accepted Plan #2564 R1–R3）：
#   r1  消费端接线（catch code-first + 共享件 + 改输入清空） → docs/evidence/2564/R1-consumer-wiring.txt
#   r2  提交路径判码（create 接 20100 / update 失败不假成功）+ 红绿对照 → docs/evidence/2564/R2-code-first.txt
#   r3  构建 + ESLint（改动文件） → docs/evidence/2564/R3-build-lint.txt
#
# 用法：bash scripts/verify-2564.sh <r1|r2|r3>
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`
# =============================================================================
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3) ;;
  *) echo "usage: $0 <r1|r2|r3>" >&2; exit 2 ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2564
SRC="frontend-pc/src/pages/admin/category/List.jsx"
EVIDENCE_DIR="docs/evidence/$ISSUE"
OUT_DIR="${OUT_DIR:-$REPO/$EVIDENCE_DIR}"; mkdir -p "$OUT_DIR"

export PATH="/usr/local/go/bin:$PATH"
export TMPDIR="${TMPDIR:-$REPO/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"
HEAVY_RUN="${HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"

HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_REF:-}" ]; then
  FIX_REF="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
fi
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
DIRTY=false
[ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR")" ] && DIRTY=true
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"

heavy() {
  if [ -x "$HEAVY_RUN" ]; then bash "$HEAVY_RUN" --timeout 1800 -- "$@"; else "$@"; fi
}

WORK="$(mktemp -d "$REPO/tmp/verify-$ISSUE-XXXXXX")" || exit 2
trap 'rm -rf "$WORK"' EXIT

LOG="$WORK/$MODE.log"; : > "$LOG"
RC=0
NAME=""; PASS=0; FAIL=0

finish() {
  local art="$OUT_DIR/$NAME" missing=0
  { printf '%s\n%s\n' "$PROV" "# generated_at=$(date -u +%Y-%m-%dT%H:%M:%SZ) HEAD=$HEAD_SHA"; cat "$LOG"; } > "$art"
  echo "ARTIFACT=$art"
  [ "$RC" = "0" ] || exit 1
  exit 0
}

mark() { # name expected actual
  local name=$1 expected=$2 actual=$3
  if [ "$actual" = "$expected" ]; then
    echo "${name}=${actual}"
    PASS=$((PASS + 1))
  else
    echo "${name}=${actual} (expected ${expected})"
    echo "ASSERT_FAIL:${name}"
    FAIL=$((FAIL + 1)); RC=1
  fi
}

# submit_body <file> <end-pattern>：从 handleFormSubmit 起，到 end-pattern（含）为止。
submit_body() {
  awk -v end="$2" '
    /const handleFormSubmit = async \(\) => \{/ { f = 1 }
    f { print }
    f && index($0, end) > 0 { exit }
  ' "$1"
}

catch_body() { # 提取 handleFormSubmit 的 catch 块
  awk '
    /} catch \(error\) \{/ { f = 1 }
    f { print }
    f && /^[[:space:]]*} finally \{/ { exit }
  ' "$1"
}

case "$MODE" in
r1)
  NAME="R1-consumer-wiring.txt"
  FILE="$REPO/$SRC"
  CATCH="$(catch_body "$FILE")"

  # 1) catch 内 code-first 判定
  n=$(printf '%s\n' "$CATCH" | grep -c 'isBusinessCorrectableConflict' || true)
  mark CONFLICT_BRANCH_IN_CATCH 1 "$([ "$n" -ge 1 ] && echo 1 || echo 0)"

  # 2) 持久提示文案走调用点 fallback
  n=$(grep -c "conflictText(.*请修改后重新提交" "$FILE" || true)
  mark CONFLICT_TEXT_FALLBACK 1 "$([ "$n" -ge 1 ] && echo 1 || echo 0)"

  # 3) 共享 Alert 渲染点（Modal 表单内）
  n=$(grep -c '<FormConflictAlert text={formConflict} testId="category-form-conflict"' "$FILE" || true)
  mark ALERT_RENDERED 1 "$([ "$n" -ge 1 ] && echo 1 || echo 0)"

  # 4) 改输入即清空陈旧提示（#2480 D8）
  n=$(grep -c "onValuesChange={() => setFormConflict('')}" "$FILE" || true)
  mark CLEAR_ON_CHANGE 1 "$([ "$n" -ge 1 ] && echo 1 || echo 0)"

  # 5) 复用共享件（判定 + 组件），不自写第二份
  impc=0; impa=0
  grep -q "from '../../../utils/formConflict'" "$FILE" && impc=1
  grep -q "from '../../../components/FormConflictAlert'" "$FILE" && impa=1
  mark SHARED_REUSE 1 "$([ "$impc" = "1" ] && [ "$impa" = "1" ] && echo 1 || echo 0)"

  # 6) 本页不得自写 Alert markup（唯一 owner = FormConflictAlert）
  n=$(grep -c '<Alert' "$FILE" || true)
  mark LOCAL_ALERT_DUP 0 "$n"

  # 7) 非冲突错误仍保留瞬时兜底 toast
  n=$(printf '%s\n' "$CATCH" | grep -c 'message.error' || true)
  mark FALLBACK_TOAST 1 "$([ "$n" -ge 1 ] && echo 1 || echo 0)"

  if [ "$FAIL" = "0" ]; then echo "R1_OK"; else echo "R1_FAIL"; fi
  ;;

r2)
  NAME="R2-code-first.txt"
  FILE="$REPO/$SRC"
  GREEN="$(git show "$FIX_SHA:$SRC" 2>/dev/null || cat "$FILE")"
  if git rev-parse --verify "$FIX_SHA^" >/dev/null 2>&1; then
    RED="$(git show "$FIX_SHA^:$SRC" 2>/dev/null || true)"
  else
    RED=""
  fi
  GREEN_FILE="$WORK/green.jsx"; RED_FILE="$WORK/red.jsx"
  printf '%s\n' "$GREEN" > "$GREEN_FILE"
  printf '%s\n' "$RED" > "$RED_FILE"

  # 创建分支接受 20100（D1）
  n=$(grep -c 'response.code === 20000 || response.code === 20100' "$GREEN_FILE" || true)
  mark CREATE_ACCEPTS_20100 1 "$([ "$n" -ge 1 ] && echo 1 || echo 0)"

  # 更新分支判码（成功 20000）
  n=$(grep -c 'updated.code !== 20000' "$GREEN_FILE" || true)
  mark UPDATE_CODE_GUARD 1 "$([ "$n" -ge 1 ] && echo 1 || echo 0)"

  # 失败分支必须在关闭弹窗前 return：handleFormSubmit 内、setModalVisible(false) 之前恰好 2 个
  n=$(submit_body "$GREEN_FILE" 'setModalVisible(false)' | grep -cE '^[[:space:]]*return[[:space:]]*$' || true)
  mark NONOK_RETURN_BEFORE_CLOSE 2 "$n"

  # 绿样：catch 内 code-first 判定先于兜底 toast
  order=$(catch_body "$GREEN_FILE" | grep -nE 'isBusinessCorrectableConflict|message\.error' | head -1 | cut -d: -f2-)
  case "$order" in
    *isBusinessCorrectableConflict*) mark GREEN_CODE_FIRST 1 1 ;;
    *) mark GREEN_CODE_FIRST 1 0 ;;
  esac

  # 红样（fix^）：只判一个成功码且 catch 无 code 判定
  if [ -s "$RED_FILE" ]; then
    a=0; b=0
    grep -q 'response.code === 20000' "$RED_FILE" && ! grep -q '20100' "$RED_FILE" && a=1
    mark RED_SINGLE_CODE_CHECK 1 "$a"
    grep -q 'isBusinessCorrectableConflict' "$RED_FILE" || b=1
    mark RED_NO_CATCH_BRANCH 1 "$b"
  else
    mark RED_SINGLE_CODE_CHECK 1 0
    mark RED_NO_CATCH_BRANCH 1 0
  fi

  echo "RED_SAMPLE=$FIX_SHA^:$SRC"
  if [ "$FAIL" = "0" ]; then echo "R2_OK"; else echo "R2_FAIL"; fi
  ;;

r3)
  NAME="R3-build-lint.txt"
  ( cd "$REPO/frontend-pc" && heavy npm run build ) > "$WORK/build.log" 2>&1
  BUILD=$?
  ( cd "$REPO/frontend-pc" && heavy npx eslint "$(printf '%s' "$SRC" | sed 's#^frontend-pc/##')" ) > "$WORK/eslint.log" 2>&1
  LINT=$?
  NO_UNDEF="$(grep -c 'no-undef' "$WORK/eslint.log" || true)"
  printf 'BUILD_EXIT=%s\nESLINT_EXIT=%s\nESLINT_NO_UNDEF=%s\n' "$BUILD" "$LINT" "$NO_UNDEF" >> "$LOG"
  tail -3 "$WORK/build.log" >> "$LOG"
  tail -5 "$WORK/eslint.log" >> "$LOG"
  [ "$BUILD" = "0" ] && [ "$LINT" = "0" ] && [ "$NO_UNDEF" = "0" ] || RC=1
  mark BUILD_EXIT 0 "$BUILD"
  mark ESLINT_NO_UNDEF 0 "$NO_UNDEF"
  if [ "$FAIL" = "0" ]; then echo "R3_OK"; else echo "R3_FAIL"; fi
  ;;
esac

finish
