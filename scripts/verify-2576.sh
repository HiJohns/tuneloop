#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2576.sh — 证据生成器（#2576 R1–R4 + affected）
#
# 口径（Accepted Plan #2576 / Decision Lock D1–D9）：
#   R1 结构红绿对照：修前 `FIX_SHA^` 的 Confirm / 回调 accept 无状态守卫（整行 Save +
#      无条件 executeAction）；修后白名单单一定义、CAS 为唯一写入口、未命中只重读分类。
#   R2 `Confirm` 状态矩阵行为（waiting/failed/confirmed/rejected/expired/未知/过期/错 token）
#      ——负例同时断言「状态未改写 + 动作未执行（IAM 调用 0 次）」。
#   R3 IAM 回调 accept 分类行为（不得把 rejected 翻回、不得对已 confirmed 重复执行）
#      + 既有 #2575 之外的既有用例（Confirm 三态 / 回调 accept / reject / failed）不翻转。
#   R4 契约文档 §2.8.2 与实现逐句一致（守卫顺序码、failed 重试、回调分类、无过期 deferral）。
#   affected：`go build ./...` + 定向测试（供 pre-ready `--affected-cmd` 复用；D8 验证层级）。
#
# 用法：bash scripts/verify-2576.sh <r1|r2|r3|r4|affected>
# 产物：docs/evidence/2576/R*-*.txt（首行 provenance）
# =============================================================================
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3|r4|affected) ;;
  *) echo "usage: $0 <r1|r2|r3|r4|affected>" >&2; exit 2 ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2576
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
SRC="backend/handlers/confirmation_session.go"
TEST_FILE="backend/handlers/confirmation_session_confirm_2576_test.go"
API_DOC="docs/spec/api/api.md"

# --- 测试库连接（显式 env > backend/.env > 兜底），避免把端口写死成某台机器的值 ------
dotenv_get() { # key
  local f="$REPO/backend/.env"
  [ -f "$f" ] || return 1
  sed -n "s/^[[:space:]]*$1[[:space:]]*=[[:space:]]*//p" "$f" | tail -1 | tr -d '"'
}
def_env() { # key fallback
  local cur; eval "cur=\"\${$1:-}\""
  if [ -z "$cur" ]; then
    local v; v="$(dotenv_get "$1" 2>/dev/null || true)"
    [ -n "$v" ] || v="$2"
    export "$1=$v"
  fi
}
export PATH="/usr/local/go/bin:$PATH"
def_env POSTGRES_HOST 127.0.0.1
def_env POSTGRES_PORT 25432
def_env POSTGRES_USER tuneloop_user
def_env POSTGRES_PASSWORD trust
export TMPDIR="${TMPDIR:-$REPO/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO/tmp/gocache}"
export GOTMPDIR="${GOTMPDIR:-$TMPDIR}"
mkdir -p "$TMPDIR" "$GOCACHE"
HEAVY_RUN="${HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"

HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_REF:-}" ]; then
  FIX_REF="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
fi
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor of HEAD" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR")" ]; then
  DIRTY=true
fi
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
TREE_LINE="GENERATED_AT_HEAD=$HEAD_SHA"

BODY="$(mktemp "$REPO/tmp/verify-$ISSUE-$ROW.XXXXXX")" || exit 2
trap 'rm -f "$BODY"' EXIT
RC=0
emit() { printf '%s\n' "$*" | tee -a "$BODY"; }
require() { grep -Fq -- "$1" "$BODY" || { echo "ASSERT_FAIL: missing [$1]" >&2; RC=1; }; }
finish() { # artifact-name
  local name="$1"
  {
    printf '%s\n%s\n' "$PROV" "$TREE_LINE"
    cat "$BODY"
  } > "$OUT_DIR/$name"
  cat "$OUT_DIR/$name"
  [ "$RC" = "0" ] || return 1
}
heavy() { if [ -x "$HEAVY_RUN" ]; then bash "$HEAVY_RUN" --timeout 3600 -- "$@"; else "$@"; fi; }
# Go 模块根在 backend/（仓库根无 go.mod），所有 go 命令都在该目录下经 heavy 锁执行（D8）。
heavy_go() { heavy bash -c 'cd "$1/backend" || exit 2; shift; exec "$@"' _ "$REPO" "$@"; }
ok() { # row-token
  if [ "$RC" = "0" ]; then emit "$1"; else emit "${1/_OK/_FAILED}=1"; fi
}
# 取顶层函数体（函数以行首 `}` 结束）。
func_body() { # file funcname
  sed -n "/^func (h \*ConfirmationSessionHandler) $2(/,/^}/p" "$1"
}

# ---------------------------------------------------------------------------
row_r1() {
  emit "== R1 structure red/green =="
  local prev_file="$REPO/tmp/verify-$ISSUE-prev.go"
  git show "${FIX_SHA}^:$SRC" > "$prev_file" 2>/dev/null || { emit "RED_SAMPLE_MISSING=1"; RC=1; return; }

  local red_confirm red_cb red_accept green_confirm green_cb green_accept
  red_confirm="$(func_body "$prev_file" Confirm)"
  red_cb="$(func_body "$prev_file" IAMConfirmationCallback)"
  green_confirm="$(func_body "$SRC" Confirm)"
  green_cb="$(func_body "$SRC" IAMConfirmationCallback)"
  # 回调 accept 分支单独取样：reject / failed 分支不在本单改动面（Out of Scope，保持不变）。
  red_accept="$(printf '%s\n' "$red_cb" | sed -n '/case "accept":/,/case "reject":/p')"
  green_accept="$(printf '%s\n' "$green_cb" | sed -n '/case "accept":/,/case "reject":/p')"
  emit "CALLBACK_ACCEPT_LINES_RED=$(printf '%s\n' "$red_accept" | wc -l | tr -d ' ')"
  emit "CALLBACK_ACCEPT_LINES_GREEN=$(printf '%s\n' "$green_accept" | wc -l | tr -d ' ')"
  rm -f "$prev_file"

  # 红：修前两处都没有状态判定（只有过期/幂等/整行 Save）。
  if printf '%s' "$red_confirm" | grep -q 'db.Save(&session)'; then
    if ! printf '%s' "$red_confirm" | grep -q 'status IN'; then emit "RED_NO_STATUS_GUARD=1"; fi
  fi
  if printf '%s' "$red_cb" | grep -q 'session.Status = "confirmed"' && \
     ! printf '%s' "$red_cb" | grep -q 'status IN'; then emit "RED_NO_CALLBACK_GUARD=1"; fi

  # 绿：白名单单一定义（map + accessor 各 1），生产调用点 2 处。
  local map_defs accessor_defs accessor_total uses
  map_defs="$(grep -c 'var confirmableStatuses = map\[string\]bool{' "$SRC")"
  accessor_defs="$(grep -c '^func ConfirmableStatuses() \[\]string {' "$SRC")"
  accessor_total="$(grep -o 'ConfirmableStatuses()' "$SRC" | wc -l | tr -d ' ')"
  uses=$((accessor_total - accessor_defs))
  emit "WHITELIST_MAP_DEFS=$map_defs"
  emit "WHITELIST_ACCESSOR_DEFS=$accessor_defs"
  emit "WHITELIST_USES=$uses"
  [ "$map_defs" = "1" ] && [ "$accessor_defs" = "1" ] && emit "GREEN_WHITELIST_DEFS=1"
  [ "$uses" = "2" ] && emit "GREEN_WHITELIST_USES=2"

  # 绿：CAS 谓词在 handler 与回调 accept 各一处；回调侧 CAS 的 WHERE 不含 expires_at（D5）。
  local cas_confirm cas_cb cb_expiry_in_where
  cas_confirm="$(printf '%s\n' "$green_confirm" | grep -c 'AND status IN ?')"
  cas_cb="$(printf '%s\n' "$green_accept" | grep -c 'AND status IN ?')"
  cb_expiry_in_where="$(printf '%s\n' "$green_accept" | grep -E 'Where\(' | grep -c 'expires_at' || true)"
  emit "CAS_PREDICATE_CONFIRM=$cas_confirm"
  emit "CAS_PREDICATE_CALLBACK=$cas_cb"
  emit "CALLBACK_CAS_EXPIRY_IN_WHERE=$cb_expiry_in_where"
  [ "$cas_confirm" = "1" ] && [ "$cas_cb" = "1" ] && emit "GREEN_CAS_PREDICATE=1"
  if [ "$cas_cb" = "1" ] && [ "$cb_expiry_in_where" = "0" ]; then
    emit "CALLBACK_CAS_NO_EXPIRY=1"
  fi

  # 绿：Confirm + 回调 accept 不再整行 Save（写入口只有条件更新）；未命中只重读分类。
  # （reject / failed 分支的 db.Save 属 Out of Scope，不在断言范围。）
  if ! printf '%s\n' "$green_confirm" | grep -q 'db.Save(' && \
     ! printf '%s\n' "$green_accept" | grep -q 'db.Save('; then
    emit "GREEN_NO_FULL_ROW_SAVE=1"
  fi
  if printf '%s' "$green_confirm" | grep -q 'RowsAffected == 0' && \
     printf '%s' "$green_confirm" | grep -q 'First(&current)'; then
    emit "GREEN_POSTCHECK_REREAD=1"
  fi
  # 绿：Confirm 内不得再写状态字面量（判定只能来自白名单）。
  if ! printf '%s' "$green_confirm" | grep -q '"waiting"'; then
    emit "NO_INLINE_WAITING_IN_CONFIRM=1"
  fi

  require "RED_NO_STATUS_GUARD=1"
  require "RED_NO_CALLBACK_GUARD=1"
  require "GREEN_WHITELIST_DEFS=1"
  require "GREEN_WHITELIST_USES=2"
  require "GREEN_CAS_PREDICATE=1"
  require "GREEN_NO_FULL_ROW_SAVE=1"
  require "GREEN_POSTCHECK_REREAD=1"
  require "CALLBACK_CAS_NO_EXPIRY=1"
  require "NO_INLINE_WAITING_IN_CONFIRM=1"
  ok "R1_OK"
}

# ---------------------------------------------------------------------------
go_test_tokens() { # selector
  local out rc
  out="$(heavy_go go test ./handlers -run "$1" -count=1 -v 2>&1)"; rc=$?
  printf '%s\n' "$out" | grep -E 'CASE_|CB_|^(--- |ok |FAIL)' \
    | sed -E 's/^[[:space:]]+[a-z0-9_]+\.go:[0-9]+:[[:space:]]*//' | tee -a "$BODY"
  return "$rc"
}

row_r2() {
  emit "== R2 Confirm status matrix =="
  heavy_go go build ./... > "$BODY.build" 2>&1
  local build_rc=$?
  emit "BUILD_EXIT=$build_rc"
  if [ "$build_rc" != "0" ]; then sed -n 1,40p "$BODY.build" | tee -a "$BODY"; RC=1; fi
  go_test_tokens 'TestConfirmSessionStatusGuard2576'; local test_rc=$?
  emit "TEST_EXIT=$test_rc"
  [ "$test_rc" = "0" ] || RC=1
  require "BUILD_EXIT=0"
  require "TEST_EXIT=0"
  for tok in \
    "CASE_WAITING=20000" "CASE_WAITING_STATUS=confirmed" "CASE_WAITING_ACTION=1" \
    "CASE_FAILED_RETRY=20000" \
    "CASE_CONFIRMED_IDEMPOTENT=20000" "CASE_CONFIRMED_NO_DUP=1" \
    "CASE_REJECTED=40901" "CASE_REJECTED_STATUS=rejected" "CASE_REJECTED_NO_WRITE=1" "CASE_REJECTED_IAM_CALLS=0" \
    "CASE_EXPIRED_FLAG=40901" "CASE_UNKNOWN=40901" "CASE_PAST=40002" "CASE_BAD_TOKEN=404"; do
    require "$tok"
  done
  rm -f "$BODY.build"
  ok "R2_OK"
}

row_r3() {
  emit "== R3 IAM callback classification + legacy regressions =="
  go_test_tokens 'TestConfirmSessionCallback2576'; local cb_rc=$?
  emit "TEST_EXIT=$cb_rc"
  [ "$cb_rc" = "0" ] || RC=1
  for tok in \
    "CB_WAITING=success" "CB_WAITING_STATUS=confirmed" \
    "CB_FAILED_STATUS=confirmed" \
    "CB_CONFIRMED=already_confirmed" "CB_CONFIRMED_STATUS=confirmed" "CB_CONFIRMED_NO_DUP=1" \
    "CB_REJECTED=not_confirmable" "CB_REJECTED_STATUS=rejected" "CB_REJECTED_IAM_CALLS=0" \
    "CB_EXPIRED_FLAG=not_confirmable"; do
    require "$tok"
  done

  # 既有断言（语义未翻转）：Confirm 三态 + 回调 accept/reject/failed。
  go_test_tokens 'TestConfirmationSessionHandler_Confirm|TestIAMIntegration_ConfirmationCallback|TestIAMIntegration_ConfirmationCallback_Reject|TestIAMMock_ConfirmationCallback_Failed'
  local legacy_rc=$?
  emit "LEGACY_EXIT=$legacy_rc"
  [ "$legacy_rc" = "0" ] || RC=1
  require "LEGACY_EXIT=0"
  if grep -Fq -- "--- FAIL" "$BODY"; then emit "LEGACY_FAIL_SEEN=1"; RC=1; fi
  ok "R3_OK"
}

# ---------------------------------------------------------------------------
row_r4() {
  emit "== R4 api.md §2.8.2 contract =="
  local sect
  sect="$(awk '/^### 2\.8\.2 /{f=1} f && /^## 三、/{exit} f{print}' "$API_DOC")"
  [ -n "$sect" ] || { emit "DOC_SECTION_282_MISSING=1"; RC=1; return; }
  emit "DOC_SECTION_282_LINES=$(printf '%s\n' "$sect" | wc -l | tr -d ' ')"
  emit "DOC_SECTION_282=1"
  printf '%s' "$sect" | grep -q '40002' && emit "DOC_EXPIRED_40002=1"
  printf '%s' "$sect" | grep -q '40901' && printf '%s' "$sect" | grep -q 'rejected' && emit "DOC_REJECTED_40901=1"
  printf '%s' "$sect" | grep -q 'failed' && printf '%s' "$sect" | grep -q '重试' && emit "DOC_FAILED_RETRY=1"
  printf '%s' "$sect" | grep -q 'already_confirmed' && emit "DOC_CALLBACK_ALREADY_CONFIRMED=1"
  printf '%s' "$sect" | grep -q 'not_confirmable' && emit "DOC_CALLBACK_NOT_CONFIRMABLE=1"
  # 过期 deferral 必须清除（不得把守卫语义 deferred 到其它 Issue）。
  local stale deferrals
  stale="$(printf '%s' "$sect" | grep -c '见 #' || true)"
  deferrals="$(printf '%s' "$sect" | grep -c 'deferr' || true)"
  emit "DOC_STALE_DEFERRAL=$((stale + deferrals))"
  emit "DOC_2543_REF=$(printf '%s' "$sect" | grep -c '#2543' || true)"

  # 实现↔文档白名单集合一致（code 的 map keys 必须逐项出现在文档表）。
  local keys key missing=0
  keys="$(sed -n '/var confirmableStatuses = map\[string\]bool{/,/^}/p' "$SRC" | sed -n 's/^[[:space:]]*"\([a-z_]*\)":[[:space:]]*true,.*/\1/p')"
  emit "CODE_WHITELIST_KEYS=$(printf '%s' "$keys" | paste -sd, -)"
  for key in $keys; do
    printf '%s' "$sect" | grep -q "\`$key\`" || { missing=$((missing + 1)); emit "DOC_MISSING_KEY=$key"; }
  done
  [ "$missing" = "0" ] && [ -n "$keys" ] && emit "DOC_CODE_WHITELIST_MATCH=1"

  require "DOC_SECTION_282=1"
  require "DOC_REJECTED_40901=1"
  require "DOC_FAILED_RETRY=1"
  require "DOC_EXPIRED_40002=1"
  require "DOC_CALLBACK_ALREADY_CONFIRMED=1"
  require "DOC_CALLBACK_NOT_CONFIRMABLE=1"
  require "DOC_STALE_DEFERRAL=0"
  require "DOC_2543_REF=0"
  require "DOC_CODE_WHITELIST_MATCH=1"
  ok "R4_OK"
}

# ---------------------------------------------------------------------------
row_affected() {
  emit "== affected: go build ./... + targeted tests =="
  heavy_go go build ./... > "$BODY.b1" 2>&1; local b1=$?
  emit "GO_BUILD_EXIT=$b1"
  [ "$b1" = "0" ] || { sed -n 1,40p "$BODY.b1" | tee -a "$BODY"; RC=1; }
  heavy_go go test ./handlers -run '2576' -count=1 > "$BODY.b2" 2>&1; local b2=$?
  emit "GO_TEST_TARGETED_EXIT=$b2"
  [ "$b2" = "0" ] || { sed -n 1,60p "$BODY.b2" | tee -a "$BODY"; RC=1; }
  tail -2 "$BODY.b2" | tee -a "$BODY"
  rm -f "$BODY.b1" "$BODY.b2"
  require "GO_BUILD_EXIT=0"
  require "GO_TEST_TARGETED_EXIT=0"
  ok "AFFECTED_OK"
}

case "$ROW" in
  r1) row_r1; finish "R1-confirm-guards.txt" ;;
  r2) row_r2; finish "R2-confirm-behavior.txt" ;;
  r3) row_r3; finish "R3-callback-behavior.txt" ;;
  r4) row_r4; finish "R4-doc-contract.txt" ;;
  affected) row_affected; finish "affected-$ISSUE.txt" ;;
esac
