#!/usr/bin/env bash
# #2451：公共乐器列表 N+1 修复的证据生成器（R1–R9）。
# 用法：bash scripts/verify-2451.sh <r1|r2|r3|r4|r5|r6|r7|r8|r9>
#
# 约定（Accepted Plan §5.4 / Evidence Environment）：
# - 在 isolated 工作树内就地执行，不建第二棵 detached 树；
# - 产物写入 docs/evidence/2451/<row>.txt，首行 PROVENANCE（build/commit/dirty）；
# - 前置：除 docs/evidence/2451 与 tmp 外工作树必须干净，否则 exit 3；
# - 环境：STORAGE_MODE=local（本地存储不发查询）、独立测试库、SimpleProtocol 由 TestMain 注入。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4|r5|r6|r7|r8|r9) ;;
  *) echo "usage: $0 <r1|r2|r3|r4|r5|r6|r7|r8|r9>" >&2; exit 2 ;;
esac

ROOT="$(git rev-parse --show-toplevel)" || exit 2
HEAD_FULL="$(git rev-parse HEAD)"
HEAD_SHORT="$(git rev-parse --short HEAD)"
OUT="$ROOT/docs/evidence/2451"
mkdir -p "$OUT"

# 开发库连接（isolated 树不带 .env；存在则读，缺失时用本机默认值）。
if [ -f "$ROOT/backend/.env" ]; then
  set -a
  # shellcheck disable=SC1091
  . "$ROOT/backend/.env"
  set +a
fi

export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$ROOT/tmp/gocache}"
export POSTGRES_PORT="${POSTGRES_PORT:-25432}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_evidence2451}"
export STORAGE_MODE="${STORAGE_MODE:-local}"
mkdir -p "$TMPDIR" "$GOCACHE"

if [ -n "$(git -C "$ROOT" status --porcelain -uall -- . ':(exclude)docs/evidence/2451' ':(exclude)tmp')" ]; then
  echo "FATAL: worktree dirty (除 docs/evidence/2451 与 tmp 外必须为空)" >&2
  git -C "$ROOT" status --porcelain -uall -- . ':(exclude)docs/evidence/2451' ':(exclude)tmp' >&2
  exit 3
fi

PROV="PROVENANCE build=1.0.${HEAD_SHORT} commit=${HEAD_FULL} dirty=false"
FAIL=0
ART=""

# gtest <artifact> <regex>：PROVENANCE 首行 + 命令 + `go test -v` 全量输出（含 t.Logf 标记）。
gtest() {
  local art="$OUT/$1" re="$2"
  ART="$art"
  echo "$PROV" > "$art"
  echo "\$ cd backend && go test ./handlers/ -run '$re' -count=1 -v" >> "$art"
  ( cd "$ROOT/backend" && go test ./handlers/ -run "$re" -count=1 -v ) 2>&1 | tee -a "$art"
  return "${PIPESTATUS[0]}"
}

assert_contains() { # artifact needle label
  local art="$OUT/$1" needle="$2" label="$3"
  if grep -qF -- "$needle" "$art"; then
    echo "RESULT: PASS $label"
  else
    echo "RESULT: FAIL $label (missing: $needle)"
    FAIL=1
  fi
}

assert_num_lt() { # artifact regex threshold label
  local art="$OUT/$1" re="$2" limit="$3" label="$4" n
  n="$(grep -oE "$re" "$art" | tail -1 | grep -oE '[0-9]+$' || true)"
  if [ -z "$n" ]; then
    echo "RESULT: FAIL $label (marker not found: $re)"
    FAIL=1
  elif [ "$n" -lt "$limit" ]; then
    echo "RESULT: PASS $label (${n} < ${limit})"
  else
    echo "RESULT: FAIL $label (${n} >= ${limit})"
    FAIL=1
  fi
}

case "$MODE" in
  r1)
    gtest R1-querycount.txt '^TestPublicInstruments_QueryCountInvariant_2451$' || FAIL=1
    assert_contains R1-querycount.txt 'QUERY_COUNT pageSize=10=6 pageSize=100=6 ENRICH=4' 'R1 query-count-invariant'
    ;;
  r2)
    gtest R2-contract.txt '^TestPublicInstruments_ResponseContract_2451$' || FAIL=1
    assert_contains R2-contract.txt 'CONTRACT_KEYS=21' 'R2 contract-keys'
    assert_contains R2-contract.txt 'CONTRACT_OK' 'R2 contract-values'
    ;;
  r3)
    gtest R3-defaults.txt '^TestPublicInstruments_MissingSiteAndTenantDefaults_2451$' || FAIL=1
    assert_contains R3-defaults.txt 'DEFAULTS_OK site_name=- site_address=- site_phone=- tenant_name=' 'R3 defaults'
    assert_contains R3-defaults.txt 'queries=6' 'R3 no-extra-queries'
    ;;
  r4)
    gtest R4-merchant-parity.txt '^TestGetMerchantTransitInfos_BatchMatchesSingle_2451$' || FAIL=1
    assert_contains R4-merchant-parity.txt 'MERCHANT_PARITY tenants=3 ok=3' 'R4 merchant-parity'
    ;;
  r5)
    gtest R5-regression.txt '^TestGetPublicInstruments_DailyRateCentsContract$' || FAIL=1
    assert_contains R5-regression.txt '--- PASS: TestGetPublicInstruments_DailyRateCentsContract' 'R5 price-contract'
    assert_contains R5-regression.txt 'ok  	tuneloop-backend/handlers' 'R5 package-ok'
    ;;
  r6)
    gtest R6-latency.txt '^TestPublicInstruments_100ItemsLatency_2451$' || FAIL=1
    assert_num_lt R6-latency.txt 'LATENCY pageSize=100 elapsed_ms=[0-9]+' 1000 'R6 latency-100'
    ;;
  r7)
    ART="$OUT/R7-build.txt"
    {
      echo "$PROV"
      echo "\$ cd backend && go build ./..."
    } > "$ART"
    if ( cd "$ROOT/backend" && go build ./... ) >> "$ART" 2>&1; then
      echo "BUILD_OK" >> "$ART"
      echo "RESULT: PASS R7 build"
    else
      echo "RESULT: FAIL R7 build" >> "$ART"
      echo "RESULT: FAIL R7 build"
      FAIL=1
    fi
    ;;
  r8)
    ART="$OUT/R8-callsites.txt"
    {
      echo "$PROV"
      echo "\$ grep -r 'GetMerchantTransitInfo(' backend/handlers (non-test) | wc -l"
      echo "\$ grep -c 'GetMerchantTransitInfo(' backend/handlers/public.go"
      echo "\$ grep -c 'GetMerchantTransitInfos(' backend/handlers/public.go"
    } > "$ART"
    SINGLE_TOTAL="$(grep -rn 'GetMerchantTransitInfo(' "$ROOT/backend/handlers" --include='*.go' | grep -v '_test.go' | wc -l | tr -d ' ')"
    PUBLIC_SINGLE="$(grep -c 'GetMerchantTransitInfo(' "$ROOT/backend/handlers/public.go")"
    PUBLIC_BATCH="$(grep -c 'GetMerchantTransitInfos(' "$ROOT/backend/handlers/public.go")"
    echo "CALLSITES single_total=${SINGLE_TOTAL} public_single=${PUBLIC_SINGLE} public_batch=${PUBLIC_BATCH}" | tee -a "$ART"
    if [ "$SINGLE_TOTAL" = "6" ] && [ "$PUBLIC_SINGLE" = "1" ] && [ "$PUBLIC_BATCH" = "1" ]; then
      echo "RESULT: PASS R8 callsites"
    else
      echo "RESULT: FAIL R8 callsites (expected 6/1/1)"
      FAIL=1
    fi
    ;;
  r9)
    gtest R9-fullload.txt '^TestPublicInstruments_FullLoad219_2451$' || FAIL=1
    assert_contains R9-fullload.txt 'FULLLOAD pages=3 items=219' 'R9 fullload-items'
    assert_num_lt R9-fullload.txt 'FULLLOAD pages=3 items=219 total_ms=[0-9]+' 1300 'R9 fullload-latency'
    ;;
esac

if [ "$FAIL" -ne 0 ]; then
  echo "RESULT: FAIL ${MODE}" | tee -a "$ART"
  exit 1
fi
echo "RESULT: PASS ${MODE}" | tee -a "$ART"
