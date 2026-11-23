#!/usr/bin/env bash
# scripts/verify-2457.sh — UpdateSite 改名 IAM 优先（#2457 v3）证据生成器。
#
# 用法：bash scripts/verify-2457.sh <r1|r2|r3|r4|r5|r6|r7|r8|r9|r10|r11|r12|r14|r15>
#
# 每行把结果写入 docs/evidence/2457/R*-*.txt（首行为 provenance），
# 并打印机器可判定的断言键值；任一行失败以非 0 退出。
set -uo pipefail

ROW="${1:-}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$REPO_ROOT/docs/evidence/2457"
ISSUE=2457
BUILD_LOCK="${OPENCODE_BUILD_LOCK:-$HOME/.config/opencode/scripts/build-lock}"
PRE_READY="${OPENCODE_PRE_READY:-$HOME/.config/opencode/scripts/pre-ready-check.sh}"
PLAN_SOURCE="${OPENCODE_PLAN_SOURCE:-$HOME/.config/opencode/scripts/plan-source.sh}"

cd "$REPO_ROOT" || exit 2

usage() {
    echo "usage: $0 <r1|r2|r3|r4|r5|r6|r7|r8|r9|r10|r11|r12|r14|r15>" >&2
    exit 2
}
[ -n "$ROW" ] || usage

mkdir -p "$OUT_DIR"

# provenance 锚定到本单最新的 fix commit：证据提交后再次生成仍保持一致。
FIX_SHA="$(git log --format=%H --grep='issue-2457' -1 2>/dev/null || true)"
FIX_SUBJECT="$(git log --format=%s --grep='issue-2457' -1 2>/dev/null || true)"
case "$FIX_SUBJECT" in
    fix*) : ;;
    *) FIX_SHA="" ;;
esac
[ -n "$FIX_SHA" ] || FIX_SHA="$(git rev-parse HEAD)"
SHORT="$(git rev-parse --short=8 "$FIX_SHA")"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ':(exclude)docs/evidence/2457')" ]; then DIRTY=true; fi
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
        echo "ROW=$ROW PASS artifact=docs/evidence/2457/$name"
        exit 0
    fi
    echo "ROW=$ROW FAIL artifact=docs/evidence/2457/$name"
    exit 1
}

has() { grep -qF -- "$2" "$1" 2>/dev/null; }

grep_key() {
    printf '%s\n' "$1" \
        | grep -oE "(^|[^A-Za-z0-9_])$2=[^[:space:]]+" \
        | sed -E "s/.*$2=//" \
        | tail -1
}

# row_handlers <artifact> <regex> <key=value>...
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
        if [ "$key" = "PATH" ]; then
            case "${got:-}" in
                "$want"*) : ;;
                *) rc=1 ;;
            esac
        else
            [ "${got:-}" = "$want" ] || rc=1
        fi
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
        if [ "$key" = "PATH" ]; then
            case "${got:-}" in
                "$want"*) : ;;
                *) rc=1 ;;
            esac
        else
            [ "${got:-}" = "$want" ] || rc=1
        fi
    done
    if [ "$rc" != "0" ]; then
        body+="--- go test output (tail) ---"$'\n'
        body+="$(printf '%s\n' "$log" | tail -30)"$'\n'
    fi
    finish "$name" "$rc" "$body"
}

case "$ROW" in
    r1)
        row_handlers "R1-iam-first.txt" '^TestUpdateSite_WritesIAMBeforeLocal_2457$' \
            "IAM_CALLS=1" "ORDER=iam_before_local"
        ;;
    r2)
        row_handlers "R2-iam-fail.txt" '^TestUpdateSite_IAMFailureKeepsLocal_2457$' \
            "STATUS=500" "LOCAL_NAME=A"
        ;;
    r3)
        row_handlers "R3-conflict.txt" '^TestUpdateSite_IAMConflictMapsTo409_2457$' \
            "STATUS=409" "CODE=40900"
        ;;
    r4)
        row_handlers "R4-escalate.txt" '^TestUpdateSite_ForbiddenEscalatesToServiceCredentials_2457$' \
            "IAM_CALLS=2" "ESCALATED=1"
        ;;
    r5)
        row_handlers "R5-idempotent.txt" '^TestUpdateSite_NoopRenameSkipsIAM_2457$' \
            "IAM_CALLS=0" "ADDRESS=updated"
        ;;
    r6)
        row_handlers "R6-anchor-guard.txt" '^TestUpdateSite_AnchorGuard_2457$' \
            "STATUS=400" "IAM_CALLS=0"
        ;;
    r7)
        row_handlers "R7-duplicate.txt" '^TestUpdateSite_DuplicateNamePrecheck_2457$' \
            "STATUS=409" "IAM_CALLS=0"
        ;;
    r8)
        row_handlers "R8-empty-name.txt" '^TestUpdateSite_EmptyNameRejected_2457$' \
            "STATUS=400" "IAM_CALLS=0"
        ;;
    r9)
        row_services "R9-client.txt" '_2457$' \
            "METHOD=PUT" "PATH=/api/v1/organizations/"
        ;;
    r10)
        row_services "R10-drift.txt" '^TestComputeSiteNameDrift_' \
            "DRIFT_ROWS=3" "DRIFT_ROWS_EMPTY=0"
        ;;
    r11)
        DOC="$REPO_ROOT/docs/spec/api/api.md"
        body="" rc=0 tokens=0
        grep -qF "先更新 IAM" "$DOC" && tokens=$((tokens + 1))
        grep -qF "镜像" "$DOC" && tokens=$((tokens + 1))
        grep -qF "锚点" "$DOC" && tokens=$((tokens + 1))
        body+="DOC_TOKENS=$tokens"$'\n'
        body+="DOC_SECTIONS=先更新 IAM,镜像,锚点"$'\n'
        [ "$tokens" = "3" ] || rc=1
        finish "R11-doc.txt" "$rc" "$body"
        ;;
    r12)
        body="" rc=0
        build_log="$(cd "$REPO_ROOT/backend" && run_heavy go build ./... 2>&1)"
        if [ $? -eq 0 ]; then body+="BUILD_OK=1"$'\n'; else body+="BUILD_OK=0"$'\n'; rc=1; fi
        vet_log="$(cd "$REPO_ROOT/backend" && go vet ./handlers ./services 2>&1)"
        if [ $? -eq 0 ]; then body+="VET_OK=1"$'\n'; else body+="VET_OK=0"$'\n'; rc=1; fi
        if [ "$rc" != "0" ]; then
            body+="--- build output (tail) ---"$'\n'"$(printf '%s\n' "$build_log" | tail -20)"$'\n'
            body+="--- vet output (tail) ---"$'\n'"$(printf '%s\n' "$vet_log" | tail -20)"$'\n'
        fi
        finish "R12-build.txt" "$rc" "$body"
        ;;
    r14)
        log="$(cd "$REPO_ROOT/backend" && run_heavy go test ./handlers -run '^TestMembershipDomain_SitesExplicitScope_2261$' -v -count=1 2>&1)"
        body="" rc=0
        if printf '%s' "$log" | grep -q -- '--- PASS: TestMembershipDomain_SitesExplicitScope_2261'; then
            body+="LEGACY_2261=PASS"$'\n'
        else
            body+="LEGACY_2261=FAIL"$'\n'
            rc=1
        fi
        skipped="$(printf '%s\n' "$log" | grep -c -- '--- SKIP' || true)"
        body+="SKIPPED=$skipped"$'\n'
        if [ "$skipped" != "0" ]; then rc=1; fi
        if grep -qF "http.StatusNotFound" "$REPO_ROOT/backend/handlers/membership_domain_explicit_2261_test.go"; then
            body+="CROSS_TENANT_404_ASSERT=kept"$'\n'
        else
            body+="CROSS_TENANT_404_ASSERT=missing"$'\n'
            rc=1
        fi
        if [ "$rc" != "0" ]; then
            body+="--- go test output (tail) ---"$'\n'
            body+="$(printf '%s\n' "$log" | tail -30)"$'\n'
        fi
        finish "R14-legacy-regression.txt" "$rc" "$body"
        ;;
    r15)
        body="" rc=0
        GATE_REPORT="$OUT_DIR/pre-ready-2457.json"
        SUMMARY="$REPO_ROOT/tmp/work-summary-2457.md"
        if [ ! -f "$GATE_REPORT" ] || [ ! -s "$GATE_REPORT" ]; then
            if [ -f "$SUMMARY" ]; then
                "$PRE_READY" "$ISSUE" \
                    --plan-file "$OUT_DIR/plan.md" \
                    --summary-file "$SUMMARY" \
                    --changed-files "$REPO_ROOT/tmp/changed-2457.txt" \
                    --evidence-file "$OUT_DIR/evidence.json" \
                    --fix-commit "$FIX_SHA" \
                    --remote-ref origin/issue-2457 \
                    --report "$GATE_REPORT" \
                    --dry-run >/dev/null 2>&1 || true
            fi
        fi
        gate_result="$(jq -r '.overall // empty' "$GATE_REPORT" 2>/dev/null || true)"
        if [ -f "$GATE_REPORT" ] && [ "$gate_result" = "PASS" ]; then
            body+="GATE_RESULT=PASS"$'\n'
        else
            body+="GATE_RESULT=${gate_result:-missing}"$'\n'
            rc=1
        fi
        plan_sha="$(sha256sum "$OUT_DIR/plan.md" | awk '{print $1}')"
        gate_sha="$(jq -r '.plan_sha256 // empty' "$GATE_REPORT" 2>/dev/null || true)"
        if [ -n "$gate_sha" ] && [ "$gate_sha" = "$plan_sha" ]; then
            body+="GATE_PLAN_SHA_MATCH=1"$'\n'
        else
            body+="GATE_PLAN_SHA_MATCH=0"$'\n'
            rc=1
        fi
        if "$PLAN_SOURCE" --issue "$ISSUE" >/tmp/plan-source-2457.log 2>&1; then
            body+="PLAN_SOURCE=PASS"$'\n'
        else
            body+="PLAN_SOURCE=FAIL"$'\n'
            body+="$(tail -5 /tmp/plan-source-2457.log)"$'\n'
            rc=1
        fi
        finish "R15-gate.txt" "$rc" "$body"
        ;;
    *)
        usage
        ;;
esac
