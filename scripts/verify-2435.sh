#!/usr/bin/env bash
# #2435：实名审核通过且存在未定第二证件 → 必须补建 second_doc 复审批次；UI 显式「未定（待平台核定）」。
set -euo pipefail
MODE="${1:-}"
case "$MODE" in r1-go|r2-smoke|r3-probe) ;; *) echo "usage: $0 {r1-go|r2-smoke|r3-probe}" >&2; exit 2 ;; esac
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
echo "# build=1.0.$(git rev-parse --short HEAD) commit=$(git rev-parse HEAD) dirty=$([ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2435/' || true)" ] && echo true || echo false)"
case "$MODE" in
  r1-go)
    export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}" POSTGRES_PORT="${POSTGRES_PORT:-5432}" \
           POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}" POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}" \
           TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_verify2435}" IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}" IAM_SECRET="${IAM_SECRET:-test-secret}"
    cd "$REPO_ROOT/backend"
    go test ./handlers/ -run 'TestFaceReview_Registration_(AutoEnqueuesSecondDoc|NoRequeueWhenSecondDocVerified)_2435' -count=1 -v
    ;;
  r2-smoke) cd "$REPO_ROOT/frontend-mobile" && npm run smoke ;;
  r3-probe)
    grep -q "未定（待平台核定）" "$REPO_ROOT/frontend-mobile/src/pages-weapp/profile/edit/EditProfile.jsx"
    grep -q "ensureSecondDocBatch(tx, batch.UserID)" "$REPO_ROOT/backend/handlers/face_review.go"
    grep -q "id_photo_other_verified" "$REPO_ROOT/backend/handlers/face_review.go"
    echo "PROBE-OK: backend auto-enqueue + UI fallback text present"
    ;;
esac
