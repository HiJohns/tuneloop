#!/usr/bin/env bash
# scripts/verify-2459.sh — UpdateMerchant 改名 IAM 优先（#2459）证据生成器。
#
# 用法：bash scripts/verify-2459.sh <r1..r18>
#
# 每行把结果写入 docs/evidence/2459/R*-*.txt（首行为 provenance），
# 并打印机器可判定的断言键值；任一行失败以非 0 退出。
set -uo pipefail

ROW="${1:-}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$REPO_ROOT/docs/evidence/2459"
ISSUE=2459
BUILD_LOCK="${OPENCODE_BUILD_LOCK:-$HOME/.config/opencode/scripts/build-lock}"
PRE_READY="${OPENCODE_PRE_READY:-$HOME/.config/opencode/scripts/pre-ready-check.sh}"
PLAN_SOURCE="${OPENCODE_PLAN_SOURCE:-$HOME/.config/opencode/scripts/plan-source.sh}"

cd "$REPO_ROOT" || exit 2

usage() {
    echo "usage: $0 <r1..r18>" >&2
    exit 2
}
[ -n "$ROW" ] || usage

mkdir -p "$OUT_DIR"

# 测试库凭据：worktree 本地 backend/.env（gitignored）优先，其次环境变量。
if [ -f "$REPO_ROOT/backend/.env" ]; then
    set -a
    # shellcheck disable=SC1091
    . "$REPO_ROOT/backend/.env"
    set +a
fi
export PATH="/usr/local/go/bin:${PATH}"

# provenance 锚定到本单最新的 fix commit：证据提交后再次生成仍保持一致。
FIX_SHA="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
[ -n "$FIX_SHA" ] || FIX_SHA="$(git rev-parse HEAD)"
SHORT="$(git rev-parse --short=8 "$FIX_SHA")"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)docs/evidence/$ISSUE" ':(exclude)tmp')" ]; then DIRTY=true; fi
PROVENANCE="# build=1.0.$SHORT # commit=$FIX_SHA # dirty=$DIRTY"

run_heavy() {
    if [ -x "$BUILD_LOCK" ]; then
        BUILD_LOCK_TIMEOUT="${BUILD_LOCK_TIMEOUT:-1800}" "$BUILD_LOCK" "$@"
    else
        "$@"
    fi
}

finish() { # artifact-name exit-code body
    local name=$1 rc=$2 body=$3
    {
        printf '%s\n' "$PROVENANCE"
        printf '%s\n' "$body"
    } > "$OUT_DIR/$name"
    printf '%s\n' "$body"
    if [ "$rc" = "0" ]; then
        echo "ROW=$ROW PASS artifact=docs/evidence/$ISSUE/$name"
        exit 0
    fi
    echo "ROW=$ROW FAIL artifact=docs/evidence/$ISSUE/$name"
    exit 1
}

grep_key() {
    printf '%s\n' "$1" \
        | grep -oE "(^|[^A-Za-z0-9_])$2=[^[:space:]]+" \
        | sed -E "s/.*$2=//" \
        | tail -1
}

row_handlers() {
    local name=$1 regex=$2
    shift 2
    local log body="" rc=0 pair key want got
    log="$(cd "$REPO_ROOT/backend" && run_heavy go test ./handlers -run "$regex" -v -count=1 2>&1)"
    printf '%s' "$log" | grep -qE '^ok[[:space:]]' || rc=1
    for pair in "$@"; do
        key="${pair%%=*}"
        want="${pair#*=}"
        got="$(grep_key "$log" "$key")"
        body+="$key=${got:-missing}"$'\n'
        [ "${got:-}" = "$want" ] || rc=1
    done
    if [ "$rc" != "0" ]; then
        body+="--- go test output (tail) ---"$'\n'
        body+="$(printf '%s\n' "$log" | tail -30)"$'\n'
    fi
    finish "$name" "$rc" "$body"
}

row_services() {
    local name=$1 regex=$2
    shift 2
    local log body="" rc=0 pair key want got
    log="$(cd "$REPO_ROOT/backend" && run_heavy go test ./services -run "$regex" -v -count=1 2>&1)"
    printf '%s' "$log" | grep -qE '^ok[[:space:]]' || rc=1
    for pair in "$@"; do
        key="${pair%%=*}"
        want="${pair#*=}"
        got="$(grep_key "$log" "$key")"
        body+="$key=${got:-missing}"$'\n'
        [ "${got:-}" = "$want" ] || rc=1
    done
    if [ "$rc" != "0" ]; then
        body+="--- go test output (tail) ---"$'\n'
        body+="$(printf '%s\n' "$log" | tail -30)"$'\n'
    fi
    finish "$name" "$rc" "$body"
}

case "$ROW" in
    r1)
        row_handlers "R1-iam-first.txt" '^TestUpdateMerchant_WritesIAMBeforeLocal_2459$' \
            "IAM_CALLS=1" "ORDER=iam_before_local" 'IAM_BODY={"name":"B"}' "LOCAL_NAME=B"
        ;;
    r2)
        row_handlers "R2-iam-fail.txt" '^TestUpdateMerchant_IAMFailureKeepsLocal_2459$' \
            "STATUS=502" "LOCAL_UNCHANGED=1"
        ;;
    r3)
        row_handlers "R3-idempotent.txt" '^TestUpdateMerchant_NoopRenameSkipsIAM_2459$' \
            "IAM_CALLS=0" "PHONE=updated"
        ;;
    r4)
        row_handlers "R4-escalate.txt" '^TestUpdateMerchant_ForbiddenEscalatesToServiceCredentials_2459$' \
            "IAM_CALLS=2" "ESCALATED=1"
        ;;
    r5)
        row_handlers "R5-conflict.txt" '^TestUpdateMerchant_IAMConflictMapsTo409_2459$' \
            "STATUS=409" "CODE=40900"
        ;;
    r6)
        row_handlers "R6-not-found.txt" '^TestUpdateMerchant_IAMNotFoundMapsTo404_2459$' \
            "STATUS=404" "CODE=40400"
        ;;
    r7)
        row_handlers "R7-empty-name.txt" '^TestUpdateMerchant_EmptyNameKeepsCurrent_2459$' \
            "IAM_CALLS=0" "NAME=unchanged"
        ;;
    r8)
        SRC="$REPO_ROOT/backend/handlers/merchant.go"
        body="" rc=0 hits=0
        grep -qF "UpdateOrganizationNameWithToken" "$SRC" && hits=$((hits + 1))
        grep -qF "UpdateOrganizationName(" "$SRC" && hits=$((hits + 1))
        http_hits="$(grep -cE 'http\.(NewRequest|Client|Post|Get)|\.Do\(' "$SRC" || true)"
        body+="REUSE_HITS=$hits"$'\n'
        body+="NEW_HTTP=${http_hits:-0}"$'\n'
        [ "$hits" = "2" ] || rc=1
        [ "${http_hits:-0}" = "0" ] || rc=1
        finish "R8-reuse.txt" "$rc" "$body"
        ;;
    r9)
        row_services "R9-drift.txt" '^TestComputeMerchantNameDrift_' \
            "MERCHANT_DRIFT_ROWS=2" "MERCHANT_DRIFT_EMPTY=0"
        ;;
    r10)
        DOC="$REPO_ROOT/docs/spec/api/api.md"
        body="" rc=0 tokens=0
        grep -qF "先更新 IAM" "$DOC" && tokens=$((tokens + 1))
        grep -qF "镜像" "$DOC" && tokens=$((tokens + 1))
        grep -qF "不落库" "$DOC" && tokens=$((tokens + 1))
        grep -qF "502/50000" "$DOC" && tokens=$((tokens + 1))
        body+="DOC_TOKENS=$tokens"$'\n'
        body+="DOC_SECTIONS=先更新 IAM,镜像,不落库,502"$'\n'
        [ "$tokens" = "4" ] || rc=1
        finish "R10-doc.txt" "$rc" "$body"
        ;;
    r11)
        body="" rc=0
        build_log="$(cd "$REPO_ROOT/backend" && run_heavy go build ./... 2>&1)"
        if [ $? -eq 0 ]; then body+="BUILD_OK=1"$'\n'; else body+="BUILD_OK=0"$'\n'; rc=1; fi
        vet_log="$(cd "$REPO_ROOT/backend" && go vet ./handlers ./services 2>&1)"
        if [ $? -eq 0 ]; then body+="VET_OK=1"$'\n'; else body+="VET_OK=0"$'\n'; rc=1; fi
        if [ "$rc" != "0" ]; then
            body+="--- build output (tail) ---"$'\n'"$(printf '%s\n' "$build_log" | tail -20)"$'\n'
            body+="--- vet output (tail) ---"$'\n'"$(printf '%s\n' "$vet_log" | tail -20)"$'\n'
        fi
        finish "R11-build.txt" "$rc" "$body"
        ;;
    r12)
        row_handlers "R12-perm.txt" '^TestUpdateMerchant_OnlyMerchantAdminMayRename_2459$' \
            "STATUS=403" "CODE=40301" "IAM_CALLS=0"
        ;;
    r13)
        row_handlers "R13-unique.txt" '^TestUpdateMerchant_DuplicateNameInNamespace_2459$' \
            "STATUS=409" "CODE=40900" "IAM_CALLS=0"
        ;;
    r14)
        # R14 必须是可执行的全量 gate 证据（P5 不得为 N/A）。
        body="" rc=0
        GATE_REPORT="$OUT_DIR/pre-ready-2459.json"
        SUMMARY="$REPO_ROOT/tmp/work-summary-2459.md"
        CHANGED="$REPO_ROOT/tmp/changed-2459.txt"
        SUITE_LOG="$REPO_ROOT/tmp/full-suite-2459.log"
        FULL_CMD='cd backend && go build ./... && go test ./handlers -count=1 -timeout 15m && go test ./services -count=1 -timeout 15m'

        : > "$SUITE_LOG"
        run_heavy bash -c "$FULL_CMD" > "$SUITE_LOG" 2>&1
        suite_exit=$?
        fail_count="$(grep -cE '^--- FAIL' "$SUITE_LOG" || true)"
        panic_count="$(grep -cE '^panic:' "$SUITE_LOG" || true)"
        body+="FULL_SUITE_CMD=$FULL_CMD"$'\n'
        body+="FULL_SUITE_EXIT=$suite_exit"$'\n'
        body+="FAIL_COUNT=$fail_count"$'\n'
        body+="PANIC_COUNT=$panic_count"$'\n'
        if [ "$suite_exit" != "0" ] || [ "$fail_count" != "0" ] || [ "$panic_count" != "0" ]; then
            rc=1
            body+="--- full suite output (tail) ---"$'\n'
            body+="$(tail -40 "$SUITE_LOG")"$'\n'
        fi

        # 首次运行（产物尚未提交）时先落一版 interim（P12 只校验存在性）；
        # 已存在时不在 gate 期间覆写 —— 否则工作树变脏导致 P5 FAIL。
        if [ ! -f "$OUT_DIR/R14-gate.txt" ]; then
            {
                printf '%s\n' "$PROVENANCE"
                printf '%s\n' "$body"
                printf 'GATE_RESULT=PENDING\n'
            } > "$OUT_DIR/R14-gate.txt"
        fi

        if [ -f "$SUMMARY" ] && [ -f "$CHANGED" ]; then
            "$PRE_READY" "$ISSUE" \
                --plan-file "$OUT_DIR/plan.md" \
                --summary-file "$SUMMARY" \
                --changed-files "$CHANGED" \
                --evidence-file "$OUT_DIR/evidence.json" \
                --fix-commit "$FIX_SHA" \
                --affected-cmd "cd backend && go test ./handlers -run '_2459$' -count=1" \
                --affected-cmd "cd backend && go test ./services -run '_2459$' -count=1" \
                --remote-ref origin/issue-2459 \
                --gate-adapter go \
                --full-gate-cmd "$FULL_CMD" \
                --gate-lock-scope host \
                --report "$GATE_REPORT" > "$REPO_ROOT/tmp/pre-ready-2459-r14.log" 2>&1 || true
        fi

        gate_result="$(jq -r '.overall // empty' "$GATE_REPORT" 2>/dev/null || true)"
        if [ -f "$GATE_REPORT" ] && [ "$gate_result" = "PASS" ]; then
            body+="GATE_RESULT=PASS"$'\n'
        else
            body+="GATE_RESULT=${gate_result:-missing}"$'\n'
            rc=1
            body+="$(tail -20 "$REPO_ROOT/tmp/pre-ready-2459-r14.log" 2>/dev/null)"$'\n'
        fi
        plan_sha="$(sha256sum "$OUT_DIR/plan.md" | awk '{print $1}')"
        gate_sha="$(jq -r '.plan_sha256 // empty' "$GATE_REPORT" 2>/dev/null || true)"
        if [ -n "$gate_sha" ] && [ "$gate_sha" = "$plan_sha" ]; then
            body+="GATE_PLAN_SHA_MATCH=1"$'\n'
        else
            body+="GATE_PLAN_SHA_MATCH=0"$'\n'
            rc=1
        fi
        rebuilt=0
        for f in R1-iam-first.txt R2-iam-fail.txt R3-idempotent.txt R4-escalate.txt R5-conflict.txt \
                 R6-not-found.txt R7-empty-name.txt R8-reuse.txt R9-drift.txt R10-doc.txt R11-build.txt \
                 R12-perm.txt R13-unique.txt R15-cross-merchant.txt R16-self-only.txt \
                 R17-local-conflict.txt R18-pagination.txt; do
            [ -f "$OUT_DIR/$f" ] || continue
            head -1 "$OUT_DIR/$f" | grep -q "commit=$FIX_SHA " && rebuilt=$((rebuilt + 1))
        done
        body+="REBUILT_ROWS=$((rebuilt + 1))"$'\n'
        [ "$((rebuilt + 1))" = "18" ] || rc=1

        p5_status="$(jq -r '[.checks[]? | select(.id == "P5")][0].status // "missing"' "$GATE_REPORT" 2>/dev/null || echo missing)"
        body+="P5=${p5_status:-missing}"$'\n'
        [ "$p5_status" = "PASS" ] || rc=1
        if "$PLAN_SOURCE" --issue "$ISSUE" >"$REPO_ROOT/tmp/plan-source-2459.log" 2>&1; then
            body+="PLAN_SOURCE=PASS"$'\n'
        else
            body+="PLAN_SOURCE=FAIL"$'\n'
            body+="$(tail -5 "$REPO_ROOT/tmp/plan-source-2459.log")"$'\n'
            rc=1
        fi
        finish "R14-gate.txt" "$rc" "$body"
        ;;
    r15)
        row_handlers "R15-cross-merchant.txt" '^TestUpdateMerchant_CrossMerchantDuplicateName_2459$' \
            "CROSS_MERCHANT=1" "STATUS=409" "LIST_AUTH=service"
        ;;
    r16)
        row_handlers "R16-self-only.txt" '^TestUpdateMerchant_SelfOnlyUserTokenDoesNotBlockRename_2459$' \
            "SELF_ONLY=1" "STATUS=200" "LIST_AUTH=service"
        ;;
    r17)
        row_handlers "R17-local-conflict.txt" '^TestUpdateMerchant_LocalMerchantDuplicateName_2459$' \
            "LOCAL_CONFLICT=1" "STATUS=409" "IAM_CALLS=0"
        ;;
    r18)
        row_services "R18-pagination.txt" '^TestListOrganizationsPagesUntilTotal_2459$' \
            "PAGES=2" "TOTAL_ORGS=1002"
        ;;
    *)
        usage
        ;;
esac
