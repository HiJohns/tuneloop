#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2575.sh — 证据生成器（#2575 确认会话 reject 守卫）
#
# 行定义（Accepted Plan #2575 D1–D10）：
#   r1  reject 守卫结构 + 条件更新唯一写入口（红绿对照） → R1-reject-guards.txt
#   r2  行为用例（5 handler + 3 回调）+ go build           → R2-reject-behavior.txt
#   r3  api.md §2.8.2 / §2.8.3 契约与实现一致              → R3-doc-contract.txt
#       （含 §2.8.2 deferral 引用有效性：必须指向 OPEN 且语义匹配的跟踪 Issue）
#
# 用法（合同命令自带 stdout 重定向到 docs/evidence/2575/）：
#   bash scripts/verify-2575.sh r1 > docs/evidence/2575/R1-reject-guards.txt
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=<bool>`
# dirty 计算忽略 docs/evidence/2575/ 自身 churn。
# =============================================================================
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3) ;;
  *) echo "usage: $0 <r1|r2|r3>" >&2; exit 2 ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2575
EVIDENCE_DIR="docs/evidence/$ISSUE"

export PATH="/usr/local/go/bin:$PATH"
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_local}"
export TMPDIR="${TMPDIR:-$REPO/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"
HEAVY_RUN="${HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"

HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_REF:-}" ]; then
  FIX_REF="$(git log --format=%H --grep='^fix(issue-2575)' -1 2>/dev/null || true)"
fi
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF is not an ancestor of HEAD" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
DIRTY=false
[ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR")" ] && DIRTY=true

# First line is the provenance contract (P14).
printf '# build=1.0.%s # commit=%s # dirty=%s\n' "$FIX_SHORT" "$FIX_SHA" "$DIRTY"

heavy() {
  if [ -x "$HEAVY_RUN" ]; then bash "$HEAVY_RUN" --timeout 1800 -- "$@"; else "$@"; fi
}

# Red sample anchor: the FIRST fix commit of this Issue. Deriving the pre-fix
# revision from `$FIX_SHA^` would compare against an already-guarded revision as
# soon as a later audit-follow-up commit becomes the newest `fix(issue-2575)`
# commit, silently turning the red assertions green. Provenance (first printed
# line) still carries the latest fix commit.
RED_BASE="$(git log --format=%H --grep='^fix(issue-2575)' --reverse 2>/dev/null | head -1)"
[ -n "$RED_BASE" ] || RED_BASE="$FIX_SHA"

REJECT_BODY_OLD="$(git show "$RED_BASE^:backend/handlers/confirmation_session.go" 2>/dev/null \
  | sed -n '/^func (h \*ConfirmationSessionHandler) Reject(/,/^}/p')"
REJECT_BODY_NEW="$(sed -n '/^func (h \*ConfirmationSessionHandler) Reject(/,/^}/p' backend/handlers/confirmation_session.go)"
CALLBACK_OLD="$(git show "$RED_BASE^:backend/handlers/confirmation_session.go" 2>/dev/null \
  | sed -n '/case "reject":/,/case "failed":/p')"
CALLBACK_NEW="$(sed -n '/case "reject":/,/case "failed":/p' backend/handlers/confirmation_session.go)"

RC=0
fail() { echo "ASSERT_FAIL: $*" >&2; RC=1; }

case "$MODE" in
r1)
  # ---- red: the pre-fix Reject had no expiry guard, no status guard, and the
  # ---- callback reject branch wrote unconditionally.
  RED_NO_EXPIRY_GUARD=0
  printf '%s\n' "$REJECT_BODY_OLD" | grep -q 'ExpiresAt' || RED_NO_EXPIRY_GUARD=1
  RED_NO_STATUS_GUARD=0
  printf '%s\n' "$REJECT_BODY_OLD" | grep -q '"waiting"' || RED_NO_STATUS_GUARD=1
  RED_CALLBACK_NO_GUARD=0
  if printf '%s\n' "$CALLBACK_OLD" | grep -q 'db.Save(&session)' && \
     ! printf '%s\n' "$CALLBACK_OLD" | grep -q '"waiting"'; then
    RED_CALLBACK_NO_GUARD=1
  fi

  # ---- green: post-fix structure.
  GREEN_EXPIRY_GUARD=0
  printf '%s\n' "$REJECT_BODY_NEW" | grep -q 'expires_at >' && GREEN_EXPIRY_GUARD=1
  GREEN_CONFIRMED_40901=0
  if printf '%s\n' "$REJECT_BODY_NEW" | grep -q '"confirmed"' && \
     printf '%s\n' "$REJECT_BODY_NEW" | grep -q '40901'; then
    GREEN_CONFIRMED_40901=1
  fi
  GREEN_IDEMPOTENT_REJECTED=0
  printf '%s\n' "$REJECT_BODY_NEW" | grep -q 'already rejected' && GREEN_IDEMPOTENT_REJECTED=1
  GREEN_CAS_PREDICATE=0
  if printf '%s\n' "$REJECT_BODY_NEW" | grep -q 'status = ?' && \
     printf '%s\n' "$REJECT_BODY_NEW" | grep -q 'expires_at > ?'; then
    GREEN_CAS_PREDICATE=1
  fi
  GREEN_SINGLE_WRITE_POINT=0
  if [ "$(printf '%s\n' "$REJECT_BODY_NEW" | grep -c 'Updates(')" = "1" ] && \
     ! printf '%s\n' "$REJECT_BODY_NEW" | grep -q 'db.Save'; then
    GREEN_SINGLE_WRITE_POINT=1
  fi
  GREEN_POSTCHECK_REREAD=0
  if printf '%s\n' "$REJECT_BODY_NEW" | grep -q 'RowsAffected == 0' && \
     printf '%s\n' "$REJECT_BODY_NEW" | grep -q 'First(&current)'; then
    GREEN_POSTCHECK_REREAD=1
  fi
  GREEN_CALLBACK_CLASSIFY=0
  if printf '%s\n' "$CALLBACK_NEW" | grep -q 'already_confirmed' && \
     printf '%s\n' "$CALLBACK_NEW" | grep -q 'not_rejectable'; then
    GREEN_CALLBACK_CLASSIFY=1
  fi
  CAS_PREDICATES=0
  [ "$(printf '%s\n' "$REJECT_BODY_NEW" | grep -c 'status = ?')" -ge 1 ] && CAS_PREDICATES=$((CAS_PREDICATES + 1))
  [ "$(printf '%s\n' "$CALLBACK_NEW" | grep -c 'status = ?')" -ge 1 ] && CAS_PREDICATES=$((CAS_PREDICATES + 1))
  NO_SAVE_IN_REJECT_PATHS=1
  if printf '%s\n' "$REJECT_BODY_NEW$CALLBACK_NEW" | grep -q 'db.Save'; then NO_SAVE_IN_REJECT_PATHS=0; fi

  echo "RED_NO_EXPIRY_GUARD=$RED_NO_EXPIRY_GUARD"
  echo "RED_NO_STATUS_GUARD=$RED_NO_STATUS_GUARD"
  echo "RED_CALLBACK_NO_GUARD=$RED_CALLBACK_NO_GUARD"
  echo "GREEN_EXPIRY_GUARD=$GREEN_EXPIRY_GUARD"
  echo "GREEN_CONFIRMED_40901=$GREEN_CONFIRMED_40901"
  echo "GREEN_IDEMPOTENT_REJECTED=$GREEN_IDEMPOTENT_REJECTED"
  echo "GREEN_CAS_PREDICATE=$GREEN_CAS_PREDICATE"
  echo "GREEN_SINGLE_WRITE_POINT=$GREEN_SINGLE_WRITE_POINT"
  echo "GREEN_POSTCHECK_REREAD=$GREEN_POSTCHECK_REREAD"
  echo "GREEN_CALLBACK_CLASSIFY=$GREEN_CALLBACK_CLASSIFY"
  echo "CAS_PREDICATES=$CAS_PREDICATES"
  echo "NO_SAVE_IN_REJECT_PATHS=$NO_SAVE_IN_REJECT_PATHS"

  [ "$RED_NO_EXPIRY_GUARD" = "1" ] || fail "pre-fix Reject unexpectedly had an expiry guard"
  [ "$RED_NO_STATUS_GUARD" = "1" ] || fail "pre-fix Reject unexpectedly had a status guard"
  [ "$RED_CALLBACK_NO_GUARD" = "1" ] || fail "pre-fix callback reject unexpectedly had a guard"
  [ "$GREEN_EXPIRY_GUARD" = "1" ] || fail "post-fix Reject is missing the expires_at predicate"
  [ "$GREEN_CONFIRMED_40901" = "1" ] || fail "post-fix Reject does not classify confirmed as 40901"
  [ "$GREEN_IDEMPOTENT_REJECTED" = "1" ] || fail "post-fix Reject is missing the idempotent rejected branch"
  [ "$GREEN_CAS_PREDICATE" = "1" ] || fail "post-fix Reject is missing the CAS predicate"
  [ "$GREEN_SINGLE_WRITE_POINT" = "1" ] || fail "reject path must have exactly one write point (Updates) and no db.Save"
  [ "$GREEN_POSTCHECK_REREAD" = "1" ] || fail "post-fix Reject is missing the post-check re-read"
  [ "$GREEN_CALLBACK_CLASSIFY" = "1" ] || fail "callback reject branch is missing its classification"
  [ "$CAS_PREDICATES" = "2" ] || fail "expected exactly 2 CAS predicates (handler + callback)"
  [ "$NO_SAVE_IN_REJECT_PATHS" = "1" ] || fail "db.Save still present in a reject path"
  [ "$RC" = "0" ] && echo "R1_OK"
  ;;

r2)
  echo "=== go build ./... ==="
  ( cd backend && heavy go build ./... ) 2>&1
  BUILD_EXIT=$?
  echo "BUILD_EXIT=$BUILD_EXIT"

  echo "=== go test (reject behavior) ==="
  OUT="$(cd backend && heavy go test ./handlers \
    -run 'TestRejectConfirmationSession_2575|TestIAMCallbackReject_2575' -count=1 -v -timeout 10m 2>&1)"
  TEST_EXIT=$?
  printf '%s\n' "$OUT" | grep -E '^(=== RUN|--- (PASS|FAIL)|PASS|FAIL|ok)' || true
  printf '%s\n' "$OUT" | grep -oE '(CASE_[A-Z_]+=[A-Za-z0-9_]+|CB_[A-Z_]+=[A-Za-z0-9_]+|NO_ACTION_EXECUTED=[01])' | sort -u
  echo "TEST_EXIT=$TEST_EXIT"

  for token in CASE_WAITING=20000 CASE_WAITING_STATUS=rejected CASE_CONFIRMED=40901 \
               CASE_CONFIRMED_STATUS=confirmed CASE_EXPIRED=40002 CASE_EXPIRED_STATUS=waiting \
               CASE_REJECTED_IDEMPOTENT=1 CASE_FAILED=40901 CASE_BAD_TOKEN=404 \
               CB_CONFIRMED=already_confirmed CB_CONFIRMED_STATUS=confirmed \
               CB_WAITING_STATUS=rejected CB_FAILED=not_rejectable NO_ACTION_EXECUTED=1; do
    printf '%s\n' "$OUT" | grep -q -- "$token" || fail "missing behavior marker $token"
  done
  [ "$BUILD_EXIT" = "0" ] || fail "go build ./... failed"
  [ "$TEST_EXIT" = "0" ] || fail "reject behavior tests failed"
  [ "$RC" = "0" ] && echo "R2_OK"
  ;;

r3)
  DOC=docs/spec/api/api.md
  has() { grep -q -- "$1" "$DOC" && echo 1 || echo 0; } # has <pattern>
  DOC_282="$(grep -q '^### 2\.8\.2' "$DOC" && echo 1 || echo 0)"
  DOC_283="$(grep -q '^### 2\.8\.3' "$DOC" && echo 1 || echo 0)"
  DOC_EXPIRED_40002="$(has 'confirmation link has expired')"
  DOC_CONFIRMED_40901="$(has 'session already confirmed')"
  DOC_IDEMPOTENT_REJECTED="$(has 'already rejected')"
  DOC_NO_ROLLBACK="$(has '不回收')"
  DOC_CALLBACK_ALREADY_CONFIRMED="$(has 'already_confirmed')"
  DOC_CALLBACK_NOT_REJECTABLE="$(has 'not_rejectable')"

  # Round 1 audit Bug 1: a deferral declared in the contract must cite an OPEN,
  # semantically matching tracking Issue. Citing a closed issue (#2543) or an
  # unrelated one (#2542) is an unregistered deferral.
  DEFERRAL_LINE="$(grep -n '本段只记录' "$DOC" | head -1 | cut -d: -f2-)"
  DEFERRAL_ISSUE="$(printf '%s\n' "$DEFERRAL_LINE" | sed -n 's/.*跟踪 Issue：*#\([0-9][0-9]*\).*/\1/p')"
  DEFERRAL_REF=0
  DEFERRAL_ISSUE_OPEN=0
  DEFERRAL_ISSUE_MATCH=0
  if [ -n "$DEFERRAL_ISSUE" ] && [ "$DEFERRAL_ISSUE" != "$ISSUE" ]; then
    DEFERRAL_REF=1
    DEFERRAL_STATE="$(gh issue view "$DEFERRAL_ISSUE" --repo HiJohns/tuneloop \
      --json state --jq '.state' 2>/dev/null)"
    DEFERRAL_TITLE="$(gh issue view "$DEFERRAL_ISSUE" --repo HiJohns/tuneloop \
      --json title --jq '.title' 2>/dev/null)"
    [ "$DEFERRAL_STATE" = "OPEN" ] && DEFERRAL_ISSUE_OPEN=1
    case "$DEFERRAL_TITLE" in
      *Confirm*|*confirm*)
        case "$DEFERRAL_TITLE" in
          *守卫*|*guard*) DEFERRAL_ISSUE_MATCH=1 ;;
        esac
        ;;
    esac
  fi

  echo "DOC_282=$DOC_282"
  echo "DOC_283=$DOC_283"
  echo "DOC_EXPIRED_40002=$DOC_EXPIRED_40002"
  echo "DOC_CONFIRMED_40901=$DOC_CONFIRMED_40901"
  echo "DOC_IDEMPOTENT_REJECTED=$DOC_IDEMPOTENT_REJECTED"
  echo "DOC_NO_ROLLBACK=$DOC_NO_ROLLBACK"
  echo "DOC_CALLBACK_ALREADY_CONFIRMED=$DOC_CALLBACK_ALREADY_CONFIRMED"
  echo "DOC_CALLBACK_NOT_REJECTABLE=$DOC_CALLBACK_NOT_REJECTABLE"
  echo "DEFERRAL_ISSUE=#${DEFERRAL_ISSUE:-none}"
  echo "DEFERRAL_REF=$DEFERRAL_REF"
  echo "DEFERRAL_ISSUE_OPEN=$DEFERRAL_ISSUE_OPEN"
  echo "DEFERRAL_ISSUE_MATCH=$DEFERRAL_ISSUE_MATCH"

  [ "$DOC_282" = "1" ] || fail "api.md missing §2.8.2"
  [ "$DOC_283" = "1" ] || fail "api.md missing §2.8.3"
  [ "$DOC_EXPIRED_40002" = "1" ] || fail "api.md missing the expired 40002 contract"
  [ "$DOC_CONFIRMED_40901" = "1" ] || fail "api.md missing the confirmed 40901 contract"
  [ "$DOC_IDEMPOTENT_REJECTED" = "1" ] || fail "api.md missing the idempotent rejected contract"
  [ "$DOC_NO_ROLLBACK" = "1" ] || fail "api.md missing the 'no rollback of executed writes' statement"
  [ "$DOC_CALLBACK_ALREADY_CONFIRMED" = "1" ] || fail "api.md missing the callback already_confirmed redirect"
  [ "$DOC_CALLBACK_NOT_REJECTABLE" = "1" ] || fail "api.md missing the callback not_rejectable redirect"
  [ "$DEFERRAL_REF" = "1" ] || fail "api.md §2.8.2 deferral does not cite a separate tracking Issue (跟踪 Issue：#N)"
  [ "$DEFERRAL_ISSUE_OPEN" = "1" ] || fail "api.md §2.8.2 deferral cites #${DEFERRAL_ISSUE:-none}, which is not OPEN (unregistered deferral)"
  [ "$DEFERRAL_ISSUE_MATCH" = "1" ] || fail "api.md §2.8.2 deferral cites #${DEFERRAL_ISSUE:-none}, whose title does not match the confirm-side guard gap"
  [ "$RC" = "0" ] && echo "R3_OK"
  ;;
esac

if [ "$RC" != "0" ]; then
  echo "VERIFY_FAILED ($MODE)" >&2
  exit 1
fi
exit 0
