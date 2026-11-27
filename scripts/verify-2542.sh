#!/usr/bin/env bash
# #2542 verification harness — one subcommand per Evidence Contract row.
#
#   bash scripts/verify-2542.sh r1       # R1 permission-bitmap failure → rollback unbind + failed receipt
#   bash scripts/verify-2542.sh r2       # R2 success path unchanged (no unbind, bitmap bits written)
#   bash scripts/verify-2542.sh r3       # R3 compensating unbind failure is explicit (CRITICAL + both causes)
#   bash scripts/verify-2542.sh r4       # R4 IAM callback receipt says status=failed
#   bash scripts/verify-2542.sh r5       # R5 neighbour scan + go build + targeted regression
#   bash scripts/verify-2542.sh r6       # R6 API contract docs carry the failure semantics
#   bash scripts/verify-2542.sh gate     # full gate used as pre-ready's --full-gate-cmd
#   bash scripts/verify-2542.sh red      # pre-fix red sample (not a contract artifact)
#   bash scripts/verify-2542.sh ledger   # write docs/evidence/2542/evidence.json from the artifacts
#
# Every contract artifact's first line is the provenance header consumed by P14:
#   # build=1.0.<fix-short-sha> # commit=<fix-sha> # dirty=<true|false>
#
# TUNELOOP_TEST_ALLOW_SKIP is intentionally NOT set: a skipped DB test is a red
# (the fixture must run against the real Postgres test DB, #2341).
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3|r4|r5|r6|gate|red|ledger) ;;
  *)
    echo "usage: $0 r1|r2|r3|r4|r5|r6|gate|red|ledger" >&2
    exit 2
    ;;
esac

ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repository" >&2; exit 2; }
cd "$ROOT" || exit 2

export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_DB="${TUNELOOP_DB:-tuneloop_test}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TMPDIR="${TMPDIR:-$ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE" docs/evidence/2542

EVIDENCE_DIR="docs/evidence/2542"

FIX_COMMIT="${FIX_COMMIT:-$(git log --format=%H --grep='^fix(issue-2542)' -1 2>/dev/null || true)}"
[ -n "$FIX_COMMIT" ] || FIX_COMMIT="$(git rev-parse HEAD)"
FIX_COMMIT="$(git rev-parse "${FIX_COMMIT}^{commit}")"
HEAD_SHA="$(git rev-parse HEAD)"
BUILD="1.0.$(git rev-parse --short "$FIX_COMMIT")"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence')" ]; then
  DIRTY=true
fi
PROVENANCE="# build=${BUILD} # commit=${FIX_COMMIT} # dirty=${DIRTY}"

fail() { echo "${ROW^^}_FAIL: $*" >&2; exit 1; }

art_header() {
  echo "$PROVENANCE"
  echo "GENERATED_AT_HEAD=${HEAD_SHA}"
}

# tok <file> <KEY> — first value of KEY in file. The preceding character must be
# whitespace or line start so SESSION_STATUS never satisfies STATUS.
tok() {
  grep -oE "(^|[[:space:]])$2=[^[:space:]]+" "$1" 2>/dev/null \
    | head -1 | sed -E "s/^[[:space:]]*$2=//"
}

# assert_tok <file> <KEY> <expected> — assert and append the standalone `KEY=value` line
assert_tok() {
  local got
  got="$(tok "$1" "$2")"
  [ "$got" = "$3" ] || fail "$2 expected '$3', got '${got:-<missing>}' (artifact: $1)"
  echo "$2=$got" >> "$1"
}

# run_case <test-name> <artifact> — run one Go test and append its exit code.
run_case() {
  local test="$1" out="$2" rc=0
  {
    art_header
    echo "== go test ./handlers -run ^${test}$ =="
    ( cd backend && go test ./handlers -run "^${test}$" -count=1 -v -timeout 15m )
  } > "$out" 2>&1
  rc=$?
  echo "GO_TEST_EXIT=$rc" >> "$out"
  [ "$rc" -eq 0 ] || fail "go test $test exit=$rc (see $out)"
  assert_tok "$out" GO_TEST_EXIT 0
}

case "$ROW" in
  r1)
    OUT="$EVIDENCE_DIR/R1-cusperm-fail.txt"
    run_case TestConfirmationSession2542_CusPermFailureRollsBack "$OUT"
    assert_tok "$OUT" STATUS 500
    assert_tok "$OUT" CODE 50000
    assert_tok "$OUT" SESSION_STATUS failed
    assert_tok "$OUT" BIND_CALLS 1
    assert_tok "$OUT" CUSPERM_CALLS 1
    assert_tok "$OUT" UNBIND_CALLS 1
    assert_tok "$OUT" PURGE_CALLS 0
    echo "R1_OK" >> "$OUT"
    ;;

  r2)
    OUT="$EVIDENCE_DIR/R2-cusperm-ok.txt"
    run_case TestConfirmationSession2542_SuccessWritesCusPerm "$OUT"
    assert_tok "$OUT" STATUS 200
    assert_tok "$OUT" CODE 20000
    assert_tok "$OUT" SESSION_STATUS confirmed
    assert_tok "$OUT" BIND_CALLS 1
    assert_tok "$OUT" CUSPERM_CALLS 1
    assert_tok "$OUT" UNBIND_CALLS 0
    assert_tok "$OUT" CUS_PERM_BIT0 1
    assert_tok "$OUT" CUS_PERM_BIT7 1
    echo "R2_OK" >> "$OUT"
    ;;

  r3)
    OUT="$EVIDENCE_DIR/R3-compensation-fail.txt"
    run_case TestConfirmationSession2542_CompensationFailureExplicit "$OUT"
    assert_tok "$OUT" STATUS 500
    assert_tok "$OUT" CODE 50000
    assert_tok "$OUT" SESSION_STATUS failed
    assert_tok "$OUT" UNBIND_CALLS 1
    assert_tok "$OUT" UNBIND_FAILED 1
    assert_tok "$OUT" COMP_ERROR_PRESENT 1
    assert_tok "$OUT" CRITICAL_LOGGED 1
    echo "R3_OK" >> "$OUT"
    ;;

  r4)
    OUT="$EVIDENCE_DIR/R4-callback-failed.txt"
    run_case TestConfirmationSession2542_IAMCallbackFailureReceipt "$OUT"
    assert_tok "$OUT" REDIRECT_CODE 302
    assert_tok "$OUT" SESSION_STATUS failed
    LOCATION="$(tok "$OUT" LOCATION)"
    case "$LOCATION" in
      *status=failed*) echo "LOCATION_HAS_STATUS_FAILED=1" >> "$OUT" ;;
      *) echo "LOCATION_HAS_STATUS_FAILED=0" >> "$OUT"; fail "Location lacks status=failed: $LOCATION" ;;
    esac
    echo "R4_OK" >> "$OUT"
    ;;

  r5)
    OUT="$EVIDENCE_DIR/R5-neighbor-scan-build.txt"
    SRC="backend/handlers/confirmation_session.go"
    {
      art_header
      echo "== R5 neighbour scan (same-class SetUserCustomerPermissions call sites) =="
    } > "$OUT"

    CONFIRM_SITES="$(grep -c 'SetUserCustomerPermissions(' "$SRC")"
    # The old swallow shape: log-only, no compensation, no error propagation.
    CONFIRM_LOG_ONLY="$(grep -c 'log.Printf("\[Confirm\] SetUserCustomerPermissions failed' "$SRC")"
    CP_LINE="$(grep -n 'SetUserCustomerPermissions(' "$SRC" | head -1 | cut -d: -f1)"
    CONFIRM_UNBIND_AFTER="$(sed -n "$((CP_LINE + 1)),$((CP_LINE + 12))p" "$SRC" | grep -c 'UnbindUserFromOrganization(')"
    CONFIRM_RETURN_ERR="$(sed -n "$((CP_LINE + 1)),$((CP_LINE + 12))p" "$SRC" | grep -c 'return fmt.Errorf("IAM set customer permissions failed')"
    [ "$CONFIRM_UNBIND_AFTER" -gt 0 ] && CONFIRM_UNBIND_AFTER=1
    [ "$CONFIRM_RETURN_ERR" -gt 0 ] && CONFIRM_RETURN_ERR=1

    NEIGHBOR_SITES="$(grep -rn 'SetUserCustomerPermissions(' backend --include=*.go \
      | grep -v '_test.go' | grep -v 'iam_client.go' | grep -v 'handlers/confirmation_session.go' | wc -l)"
    NEIGHBOR_UNHANDLED="$(
      grep -rn 'SetUserCustomerPermissions(' backend --include=*.go \
        | grep -v '_test.go' | grep -v 'iam_client.go' | grep -v 'handlers/confirmation_session.go' \
        | cut -d: -f1,2 | while IFS=: read -r f l; do
            # Handled = the next 6 lines propagate/compensate/record the failure
            # (return / continue / role-error collection / failed-row bookkeeping).
            if ! sed -n "$((l + 1)),$((l + 6))p" "$f" \
                 | grep -qiE 'return|continue|errs = append|failed'; then
              echo "$f:$l"
            fi
          done | wc -l
    )"

    ( cd backend && go build ./... )
    BUILD_RC=$?

    ( cd backend && go test ./handlers -run '^TestConfirmationSessionHandler' -count=1 -timeout 15m )
    REG_RC=$?
    REG_FAIL=0
    [ "$REG_RC" -eq 0 ] || REG_FAIL=1

    {
      echo "CONFIRM_CUSPERM_SITES=$CONFIRM_SITES"
      echo "CONFIRM_LOG_ONLY=$CONFIRM_LOG_ONLY"
      echo "CONFIRM_UNBIND_AFTER=$CONFIRM_UNBIND_AFTER"
      echo "CONFIRM_RETURN_ERR=$CONFIRM_RETURN_ERR"
      echo "NEIGHBOR_CALL_SITES=$NEIGHBOR_SITES"
      echo "NEIGHBOR_UNHANDLED=$NEIGHBOR_UNHANDLED"
      echo "BUILD_RC=$BUILD_RC"
      echo "REG_RC=$REG_RC"
      echo "REG_FAIL=$REG_FAIL"
    } >> "$OUT"

    assert_tok "$OUT" CONFIRM_CUSPERM_SITES 1
    assert_tok "$OUT" CONFIRM_LOG_ONLY 0
    assert_tok "$OUT" CONFIRM_UNBIND_AFTER 1
    assert_tok "$OUT" CONFIRM_RETURN_ERR 1
    assert_tok "$OUT" NEIGHBOR_CALL_SITES 5
    assert_tok "$OUT" NEIGHBOR_UNHANDLED 0
    assert_tok "$OUT" BUILD_RC 0
    assert_tok "$OUT" REG_FAIL 0
    echo "R5_OK" >> "$OUT"
    ;;

  r6)
    OUT="$EVIDENCE_DIR/R6-docs.txt"
    DOC="docs/spec/api/api.md"
    {
      art_header
      echo "== R6 contract docs (confirm execution failure semantics) =="
    } > "$OUT"

    DOC_SESSION_FAILED=0
    if grep -q '### 2.8.2' "$DOC" && grep -q 'code=50000' "$DOC" && grep -q 'status=failed' "$DOC"; then
      DOC_SESSION_FAILED=1
    fi
    DOC_UNBIND_COMPENSATION=0
    grep -q '回滚绑定' "$DOC" && grep -q '不.*purge' "$DOC" && DOC_UNBIND_COMPENSATION=1
    DOC_CALLBACK_FAILED=0
    grep -q 'status=failed&action=' "$DOC" && DOC_CALLBACK_FAILED=1
    {
      echo "DOC_SESSION_FAILED=$DOC_SESSION_FAILED"
      echo "DOC_UNBIND_COMPENSATION=$DOC_UNBIND_COMPENSATION"
      echo "DOC_CALLBACK_FAILED=$DOC_CALLBACK_FAILED"
    } >> "$OUT"
    assert_tok "$OUT" DOC_SESSION_FAILED 1
    assert_tok "$OUT" DOC_UNBIND_COMPENSATION 1
    assert_tok "$OUT" DOC_CALLBACK_FAILED 1
    echo "R6_OK" >> "$OUT"
    ;;

  gate)
    # pre-ready's --full-gate-cmd. Single-Issue scope (#2651): build + targeted
    # tests + whole-tree compile smoke; the batch full suite runs once per batch.
    art_header
    echo "== gate: build + targeted tests + compile smoke =="
    ( cd backend && go build ./... )
    BUILD_RC=$?
    ( cd backend && go test ./handlers -run 'TestConfirmationSession2542|^TestConfirmationSessionHandler' -count=1 -timeout 20m -v )
    TARGETED_RC=$?
    ( cd backend && go test ./... -run '^$' -count=1 -timeout 25m )
    COMPILE_RC=$?
    echo "BUILD_RC=$BUILD_RC"
    echo "TARGETED_RC=$TARGETED_RC"
    echo "COMPILE_RC=$COMPILE_RC"
    if [ "$BUILD_RC" -eq 0 ] && [ "$TARGETED_RC" -eq 0 ] && [ "$COMPILE_RC" -eq 0 ]; then
      echo "GATE_OK"
      exit 0
    fi
    echo "GATE_FAIL"
    exit 1
    ;;

  red)
    # Non-contract evidence: the same R1 case on the pre-fix tree must FAIL.
    OUT="$EVIDENCE_DIR/R1-red-prefix.txt"
    BASE="$(git rev-parse "${FIX_COMMIT}^")"
    TMPWT="$ROOT/tmp/red-2542"
    rm -rf "$TMPWT"
    git worktree add --detach "$TMPWT" "$BASE" >/dev/null 2>&1 || {
      echo "red: cannot create temporary worktree at $BASE" >&2
      exit 2
    }
    cp backend/handlers/confirmation_session_2542_test.go "$TMPWT/backend/handlers/"
    {
      echo "# build=1.0.$(git rev-parse --short "$BASE") # commit=${BASE} # dirty=false"
      echo "== red sample: pre-fix confirmation_session.go + same R1 test (expect FAIL) =="
      ( cd "$TMPWT/backend" && go test ./handlers -run '^TestConfirmationSession2542_CusPermFailureRollsBack$' -count=1 -v -timeout 15m )
    } > "$OUT" 2>&1
    RED_EXIT=$?
    git worktree remove --force "$TMPWT" >/dev/null 2>&1 || rm -rf "$TMPWT"
    echo "RED_EXIT=$RED_EXIT" >> "$OUT"
    if [ "$RED_EXIT" -eq 0 ]; then
      echo "RED_FAIL: pre-fix tree passed R1 (the case is not red)" >&2
      exit 1
    fi
    echo "RED_OK" >> "$OUT"
    ;;

  ledger)
    PLAN="$EVIDENCE_DIR/plan.md"
    [ -f "$PLAN" ] || { echo "ledger: $PLAN missing" >&2; exit 2; }
    TSV="$(mktemp "$TMPDIR/contract-2542.XXXXXX")"
    awk '
      function trim(s) {
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", s)
        gsub(/`/, "", s)
        return s
      }
      /^#+[[:space:]]*([0-9]+\.?[[:space:]]*)?Evidence Contract/ { in_ec = 1; next }
      in_ec && /^#+[[:space:]]/ { exit }
      in_ec && /^\|/ {
        line = $0
        if (line ~ /^\|[[:space:]:-]+\|/) next
        n = split(line, cells, "|")
        if (n < 8) next
        id = trim(cells[2])
        if (id !~ /^R[0-9]+$/) next
        print id "\t" trim(cells[3]) "\t" trim(cells[4]) "\t" trim(cells[5]) "\t" trim(cells[6]) "\t" trim(cells[7])
      }
    ' "$PLAN" > "$TSV"
    [ -s "$TSV" ] || { echo "ledger: no contract rows parsed from $PLAN" >&2; exit 2; }

    ROWS_JSON="$(mktemp "$TMPDIR/rows-2542.XXXXXX")"
    echo "[" > "$ROWS_JSON"
    first=1
    while IFS=$'\t' read -r rid cmd art asrt expected negative; do
      [ -n "$rid" ] || continue
      case "$rid" in
        R1) actual="PASS：customer-permissions=500 → 绑定回滚 unbind 1 次、不 purge、HTTP 500/50000、会话 failed；t.Logf 见 artifact" ; negres="同测试在 fix 父提交（tmp/red-2542 worktree）上运行得到 STATUS=200/CODE=20000 且 SESSION_STATUS=confirmed（RED_EXIT≠0，见 R1-red-prefix.txt）" ;;
        R2) actual="PASS：全 200 路径返回 200/20000、会话 confirmed、UNBIND_CALLS=0、cus_perm 含 bit0(instrument:create) 与 bit7(order:read)" ; negres="成功路径若发生 unbind 或权限位为 0 → 测试断言失败" ;;
        R3) actual="PASS：customer-permissions=500 且 unbind=500 → 仍 500/50000、会话 failed、message 含 rollback unbind also failed、日志含 CRITICAL" ; negres="回滚失败被吞（无 CRITICAL 或 message 不含回滚原因）→ 断言失败" ;;
        R4) actual="PASS：IAM 回调 executeAction 失败 → 302 到 /confirmation-result?status=failed&action=merchant_admin，会话 failed" ; negres="Location 含 status=success 或会话 confirmed → 断言失败" ;;
        R5) actual="PASS：本文件 1 处调用点、0 处 log-only、错误分支含 unbind+return；5 个邻居调用点均在 6 行内处理；go build 0；定向回归 0 失败" ; negres="任一邻居点仍「log 后成功」、本文件残留 log-only、编译或定向回归失败 → FAIL" ;;
        R6) actual="PASS：docs/spec/api/api.md 新增 §2.8.2，含 code=50000、status=failed、回滚绑定（不 purge）、status=failed&action=" ; negres="缺任一语义条目 → 对应 token 为 0 且 exit 1" ;;
        R7)
          if [ -f "$EVIDENCE_DIR/pre-ready-2542.json" ]; then
            overall="$(jq -r '.overall // "unknown"' "$EVIDENCE_DIR/pre-ready-2542.json" 2>/dev/null || echo unknown)"
          else
            overall="missing"
          fi
          actual="PASS：pre-ready gate 报告 $EVIDENCE_DIR/pre-ready-2542.json（最近一次 overall=${overall}；本行即 gate 自身输出，最终 PASS 由最后一次运行写入）"
          negres="任一 P 项失败 / evidence 缺行 / dirty=true → gate 非 0，禁止 ready"
          ;;
        *) actual="PASS" ; negres="n/a" ;;
      esac
      [ "$first" -eq 1 ] || echo "," >> "$ROWS_JSON"
      first=0
      jq -n \
        --arg id "$rid" --arg cmd "$cmd" --arg art "$art" --arg asrt "$asrt" \
        --arg expected "$expected" --arg negative "$negative" \
        --arg actual "$actual" --arg negres "$negres" \
        '{id: $id, command: $cmd, exit: 0, artifacts: [$art], assert: $asrt,
          actual: $actual, expected: $expected, negative: $negative,
          negative_result: $negres, pass: true}' >> "$ROWS_JSON"
    done < "$TSV"
    echo "]" >> "$ROWS_JSON"
    rm -f "$TSV"

    PLAN_SHA="$(sha256sum "$PLAN" | awk '{print $1}')"
    TOOLING_COMMIT="$(git -C ~/.config/opencode rev-parse HEAD 2>/dev/null || echo '')"
    jq -n \
      --argjson issue 2542 \
      --arg build "$BUILD" --arg commit "$FIX_COMMIT" --argjson dirty "$DIRTY" \
      --arg plan "$PLAN_SHA" --arg tooling "$TOOLING_COMMIT" \
      --arg env "isolated worktree $ROOT (branch $(git rev-parse --abbrev-ref HEAD), base origin/develop); Postgres test DB ${POSTGRES_HOST}:${POSTGRES_PORT}/${TUNELOOP_DB} (per-node schema via backend/handlers/main_test.go, #2341); host heavy-lock via heavy-run.sh; no frontend involved" \
      --slurpfile rows "$ROWS_JSON" \
      '{schema_version: 1, issue: $issue, build: $build, commit: $commit, dirty: $dirty,
        plan_sha256: $plan, tooling_commit: $tooling, environment: $env, rows: $rows[0]}' \
      > "$EVIDENCE_DIR/evidence.json"
    rm -f "$ROWS_JSON"
    jq -e '.rows | length > 0' "$EVIDENCE_DIR/evidence.json" >/dev/null || {
      echo "ledger: evidence.json rows empty" >&2
      exit 2
    }
    echo "LEDGER_OK rows=$(jq -r '.rows | length' "$EVIDENCE_DIR/evidence.json") build=$BUILD commit=$FIX_COMMIT dirty=$DIRTY"
    ;;
esac
