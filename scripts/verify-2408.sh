#!/usr/bin/env bash
# #2408：PAYSCORE_MOCK_RESULT（approved/rejected/pending）验收脚本。
# 用法：bash scripts/verify-2408.sh <r1-default|r2-rejected|r3-pending|r4-unknown|r5-isolation|r6-build>
# 每个模式写 docs/evidence/2408/ 对应产物（首行 provenance）。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1-default)   ART="docs/evidence/2408/R1-default.txt"; TEST='^TestMockPayscoreResult_2408$/^default-approved$' ;;
  r2-rejected)  ART="docs/evidence/2408/R2-rejected.txt"; TEST='^TestMockPayscoreResult_2408$/^rejected$' ;;
  r3-pending)   ART="docs/evidence/2408/R3-pending.txt"; TEST='^TestMockPayscoreResult_PendingStaysPending_2408$' ;;
  r4-unknown)   ART="docs/evidence/2408/R4-unknown.txt"; TEST='^TestMockPayscoreResult_2408$/^unknown-falls-back$' ;;
  r5-isolation) ART="docs/evidence/2408/R5-isolation.txt"; TEST="" ;;
  r6-build)     ART="docs/evidence/2408/R6-build.txt"; TEST="" ;;
  *) echo "usage: $0 <r1-default|r2-rejected|r3-pending|r4-unknown|r5-isolation|r6-build>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE" "$(dirname "$ART")"

BUILD="1.0.$(git rev-parse --short HEAD)"
COMMIT="$(git rev-parse HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2408/' || true)" ]; then
  DIRTY=true
fi

{
  echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"
  case "$MODE" in
    r1-default|r2-rejected|r3-pending|r4-unknown)
      cd "$REPO_ROOT/backend"
      go test ./services/wechatpay/ -run "$TEST" -count=1 -v
      ;;
    r5-isolation)
      cd "$REPO_ROOT"
      f="backend/services/wechatpay/payscore_provider.go"
      echo "--- grep -n PAYSCORE_MOCK_RESULT ---"
      grep -n 'PAYSCORE_MOCK_RESULT' "$f" || true
      echo "--- grep -n mockResultFromEnv ---"
      grep -n 'mockResultFromEnv' "$f" || true
      echo "--- grep -n real provider return ---"
      grep -n 'return client, nil' "$f" || true
      env_reads=$(grep -c 'os.Getenv("PAYSCORE_MOCK_RESULT")' "$f" | tr -d ' ')
      call_sites=$(grep -c ':= mockResultFromEnv()' "$f" | tr -d ' ')
      echo "env_reads=$env_reads mockResultFromEnv_call_sites=$call_sites"
      grep -q "return client, nil" "$f" && echo "real_provider_returns_client=yes"
      if [ "$env_reads" != "1" ] || [ "$call_sites" != "1" ]; then
        echo "ISOLATION FAIL: env read or mock constructor leaked"
        exit 1
      fi
      echo "ISOLATION PASS: PAYSCORE_MOCK_RESULT only read by mockResultFromEnv; real branch returns PayScoreClient"
      ;;
    r6-build)
      cd "$REPO_ROOT/backend"
      go build ./...
      ;;
  esac
} 2>&1 | tee "$ART"

exit "${PIPESTATUS[0]}"
