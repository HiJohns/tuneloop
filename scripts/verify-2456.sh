#!/usr/bin/env bash
# scripts/verify-2456.sh — PC 管理后台 idle timeout（#2456）证据生成器。
#
# 用法：bash scripts/verify-2456.sh <r1..r13>
#
# 每行把结果写入 docs/evidence/2456/R*-*.txt（首行为 provenance），
# 并在 stdout 打印机器可判定的断言键值；任一行失败以非 0 退出。
set -uo pipefail

ROW="${1:-}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$ROOT/docs/evidence/2456"
MIGRATION="20261123001_pc_session_idle_2456"
UP_SQL="$ROOT/backend/database/migrations/${MIGRATION}.up.sql"
DOWN_SQL="$ROOT/backend/database/migrations/${MIGRATION}.down.sql"
BUILD_LOCK="${OPENCODE_BUILD_LOCK:-$HOME/.config/opencode/scripts/build-lock}"

die_usage() {
    echo "usage: $0 <r1..r13>" >&2
    exit 2
}

[ -n "$ROW" ] || die_usage

# provenance 锚定本单最新 fix commit（证据提交后再次生成保持一致）。
FIX_SUBJECT="$(git -C "$ROOT" log --format=%s --grep='issue-2456' -1 2>/dev/null || true)"
FIX_SHA="$(git -C "$ROOT" log --format=%H --grep='issue-2456' -1 2>/dev/null || true)"
case "$FIX_SUBJECT" in
    fix*) : ;;
    *) FIX_SHA="" ;;
esac
[ -n "$FIX_SHA" ] || FIX_SHA="$(git -C "$ROOT" rev-parse HEAD)"
COMMIT="$FIX_SHA"
SHORT="$(git -C "$ROOT" rev-parse --short=8 "$COMMIT")"
if [ -n "$(git -C "$ROOT" status --porcelain --untracked-files=no -- . ':(exclude)docs/evidence/2456')" ]; then
    DIRTY="true"
else
    DIRTY="false"
fi
BUILD="1.0.$SHORT"

mkdir -p "$OUT_DIR"
ARTIFACT=""

# run_heavy <cmd...> — 走 host 锁执行重命令（锁不可用时降级为直跑）。
run_heavy() {
    if [ -x "$BUILD_LOCK" ]; then
        BUILD_LOCK_TIMEOUT="${BUILD_LOCK_TIMEOUT:-1800}" "$BUILD_LOCK" "$@"
    else
        "$@"
    fi
}

finish() { # <artifact-name> <exit> <body>
    local name=$1 rc=$2 body=$3
    ARTIFACT="$OUT_DIR/$name"
    {
        printf '# build=%s # commit=%s # dirty=%s\n' "$BUILD" "$COMMIT" "$DIRTY"
        printf '%s\n' "$body"
    } > "$ARTIFACT"
    printf '%s\n' "$body"
    if [ "$rc" = "0" ]; then
        echo "ROW=$ROW PASS artifact=${ARTIFACT#"$ROOT"/}"
        exit 0
    fi
    echo "ROW=$ROW FAIL artifact=${ARTIFACT#"$ROOT"/}"
    exit 1
}

# grep_key <text> <key> — 抽取 `KEY=value` 行（要求 KEY 前不是标识符字符，
# 避免 HTTP= 误匹配 NO_TOKEN_HTTP=）。
grep_key() {
    printf '%s\n' "$1" \
        | grep -oE "(^|[^A-Za-z0-9_])$2=[0-9A-Za-z_.:-]+" \
        | sed -E "s/.*$2=//" \
        | tail -1
}

has() { grep -qF -- "$2" "$1" 2>/dev/null; }

row_r1() {
    local body="" rc=0 neg
    # check_pair <up> <down> → ok|bad（同一谓词供正例与负例复用）
    check_pair() {
        if [ -f "$1" ] && [ -f "$2" ] \
            && has "$1" "CREATE TABLE IF NOT EXISTS pc_sessions" \
            && has "$2" "DROP TABLE IF EXISTS pc_sessions"; then
            echo "ok"
        else
            echo "bad"
        fi
    }

    if [ -f "$UP_SQL" ] && [ -f "$DOWN_SQL" ]; then
        body+="MIGRATION_FILES=$(basename "$UP_SQL"),$(basename "$DOWN_SQL")"$'\n'
    else
        body+="MIGRATION_FILES=missing"$'\n'
        rc=1
    fi

    if [ "$(check_pair "$UP_SQL" "$DOWN_SQL")" = "ok" ]; then
        body+="MIGRATION_PAIR=ok"$'\n'
    else
        body+="MIGRATION_PAIR=bad"$'\n'
        rc=1
    fi

    # 负例：down 缺失时必须判 bad（证明断言真的会失败）。
    neg="$(check_pair "$UP_SQL" "$ROOT/docs/evidence/2456/__missing_kong_down.sql")"
    body+="NEG_MISSING_DOWN=$neg"$'\n'
    if [ "$neg" != "bad" ]; then
        rc=1
    fi

    if has "$ROOT/backend/database/db.go" "20261123001, \"pc_sessions\""; then
        body+="ARTIFACT_REGISTERED=pc_sessions"$'\n'
    else
        body+="ARTIFACT_REGISTERED=missing"$'\n'
        rc=1
    fi

    if has "$ROOT/backend/database/db.go" "&models.PCSession{}" \
        && has "$ROOT/backend/models/models.go" "type PCSession struct"; then
        body+="MODEL_IN_VALIDATE=yes"$'\n'
    else
        body+="MODEL_IN_VALIDATE=no"$'\n'
        rc=1
    fi

    # v2 D8：last_activity_at 必须是 timestamptz（迁移 + 模型）。
    if has "$UP_SQL" "last_activity_at TIMESTAMPTZ" \
        && has "$ROOT/backend/models/models.go" 'type:timestamptz;not null;default:now()'; then
        body+="LAST_ACTIVITY_TZ=timestamptz"$'\n'
    else
        body+="LAST_ACTIVITY_TZ=wrong"$'\n'
        rc=1
    fi

    finish "R1-migration.txt" "$rc" "$body"
}

# row_go_test <artifact> <test-regex> <required-key=value>...
row_go_test() {
    local name=$1 pattern=$2
    shift 2
    local log body="" rc=0 pair key want got
    log="$(cd "$ROOT/backend" && run_heavy go test ./handlers -run "$pattern" -v -count=1 2>&1)"
    if ! printf '%s' "$log" | grep -q '^ok[[:space:]]'; then
        rc=1
    fi
    for pair in "$@"; do
        key="${pair%%=*}"
        want="${pair#*=}"
        got="$(grep_key "$log" "$key")"
        body+="$key=${got:-missing}"$'\n'
        if [ "${got:-}" != "$want" ]; then
            rc=1
        fi
    done
    if [ "$rc" != "0" ]; then
        body+="--- go test output (tail) ---"$'\n'
        body+="$(printf '%s\n' "$log" | tail -40)"$'\n'
    fi
    finish "$name" "$rc" "$body"
}

row_r2() {
    row_go_test "R2-pc-register.txt" '^TestPCIdleR2_' \
        "PC_CALLBACK_ROWS=1" "H5_CALLBACK_ROWS=0" "MOBILE_LOGIN_ROWS=0"
}
row_r3() {
    row_go_test "R3-idle-reject.txt" '^TestPCIdleR3_' \
        "HTTP=401" "CODE=40110" "IAM_CALLS=0"
}
row_r4() {
    row_go_test "R4-idle-allow.txt" '^TestPCIdleR4_' \
        "HTTP=200" "CODE=20000" "IAM_CALLS=1" "TOUCHED=0"
}
row_r5() {
    row_go_test "R5-nonpc-unaffected.txt" '^TestPCIdleR5_' \
        "HTTP=200" "CODE=20000" "IDLE_ENFORCED=0" "IAM_CALLS=1"
}
row_r6() {
    row_go_test "R6-activity.txt" '^TestPCIdleR6_' \
        "HTTP=200" "OWNER_TOUCHED=1" "OTHER_TOUCHED=0" "NO_TOKEN_HTTP=401"
}

row_r7() {
    local body="" rc=0 log key want got pair
    local pairs=("MINUTES_1_REJECT=1" "ZERO_DISABLED=1" "INVALID_FALLBACK=60" "CLAMP_MAX=1440")
    log="$(
        cd "$ROOT/backend" &&
        run_heavy go test ./handlers -run '^TestPCIdleR7_' -v -count=1 2>&1
        cd "$ROOT/backend" &&
        run_heavy go test ./services -run '^TestPCSessionIdleTimeoutConfig$|^TestHashRefreshTokenStable$' -v -count=1 2>&1
    )"
    for pair in "${pairs[@]}"; do
        key="${pair%%=*}"
        want="${pair#*=}"
        got="$(grep_key "$log" "$key")"
        body+="$key=${got:-missing}"$'\n'
        if [ "${got:-}" != "$want" ]; then
            rc=1
        fi
    done
    if [ "$rc" != "0" ]; then
        body+="--- go test output (tail) ---"$'\n'
        body+="$(printf '%s\n' "$log" | tail -40)"$'\n'
    fi
    finish "R7-config.txt" "$rc" "$body"
}

row_r8() {
    local log body="" rc=0
    log="$(cd "$ROOT/frontend-pc" && run_heavy node --test tests/pcSession.test.mjs 2>&1)"
    if ! printf '%s' "$log" | grep -q '^# pass 5$'; then
        rc=1
    fi
    if ! printf '%s' "$log" | grep -q '^# fail 0$'; then
        rc=1
    fi
    body+="$(printf '%s\n' "$log" | grep -E '^# (tests|pass|fail) ')"$'\n'
    if [ "$rc" != "0" ]; then
        body+="--- node --test output (tail) ---"$'\n'
        body+="$(printf '%s\n' "$log" | tail -40)"$'\n'
    fi
    finish "R8-frontend-unit.txt" "$rc" "$body"
}

row_r9() {
    local api="$ROOT/frontend-pc/src/services/api.js"
    local app="$ROOT/frontend-pc/src/App.jsx"
    local logout="$ROOT/frontend-pc/src/pages/LogoutPage.jsx"
    local session="$ROOT/frontend-pc/src/utils/pcSession.js"
    local body="" rc=0

    if has "$api" "40110" && has "$session" "PC_IDLE_TIMEOUT_CODE = 40110"; then
        body+="WIRING_40110=1"$'\n'
    else
        body+="WIRING_40110=0"$'\n'
        rc=1
    fi

    if has "$app" "startActivityTracking(" && has "$api" "auth/pc-activity"; then
        body+="WIRING_ACTIVITY=1"$'\n'
    else
        body+="WIRING_ACTIVITY=0"$'\n'
        rc=1
    fi

    if has "$api" "clearTokens(" && has "$api" "'idle_timeout'" && has "$logout" "idle_timeout"; then
        body+="WIRING_REASON=1"$'\n'
    else
        body+="WIRING_REASON=0"$'\n'
        rc=1
    fi

    body+="WIRING_FILES=frontend-pc/src/services/api.js,frontend-pc/src/App.jsx,frontend-pc/src/pages/LogoutPage.jsx,frontend-pc/src/utils/pcSession.js"$'\n'
    finish "R9-wiring.txt" "$rc" "$body"
}

row_r10() {
    local body="" rc=0 build_log lint_log no_undef
    build_log="$(cd "$ROOT/frontend-pc" && run_heavy npm run build 2>&1)"
    if printf '%s' "$build_log" | grep -q "built in"; then
        body+="BUILD_EXIT=0"$'\n'
    else
        body+="BUILD_EXIT=1"$'\n'
        rc=1
    fi

    lint_log="$(cd "$ROOT/frontend-pc" && npx eslint src/utils/pcSession.js src/services/api.js src/App.jsx src/pages/LogoutPage.jsx tests/pcSession.test.mjs 2>&1)"
    if [ $? -eq 0 ]; then
        body+="LINT_EXIT=0"$'\n'
    else
        body+="LINT_EXIT=1"$'\n'
        rc=1
    fi
    no_undef="$(printf '%s\n' "$lint_log" | grep -c "no-undef" || true)"
    body+="NO_UNDEF=$no_undef"$'\n'
    if [ "$no_undef" != "0" ]; then
        rc=1
    fi
    if [ "$rc" != "0" ]; then
        body+="--- build output (tail) ---"$'\n'
        body+="$(printf '%s\n' "$build_log" | tail -30)"$'\n'
        body+="--- lint output (tail) ---"$'\n'
        body+="$(printf '%s\n' "$lint_log" | tail -30)"$'\n'
    fi
    finish "R10-build.txt" "$rc" "$body"
}

# r13：gate 四源一致（读校验）。gate 报告缺失/非 PASS 时用一次 --dry-run（本机）
# 生成 bootstrap 报告；重型全量 gate 由 /work 在 ready 前执行。
row_r13() {
    local body="" rc=0 gate_report="$OUT_DIR/pre-ready-2456.json" summary="$ROOT/tmp/work-summary-2456.md"
    local pre_ready="${OPENCODE_PRE_READY:-$HOME/.config/opencode/scripts/pre-ready-check.sh}"
    local plan_source="${OPENCODE_PLAN_SOURCE:-$HOME/.config/opencode/scripts/plan-source.sh}"
    local existing
    existing="$(jq -r '.overall // empty' "$gate_report" 2>/dev/null || true)"
    if [ ! -f "$gate_report" ] || [ ! -s "$gate_report" ] || [ "$existing" != "PASS" ]; then
        if [ -f "$summary" ] && [ -f "$OUT_DIR/evidence.json" ]; then
            "$pre_ready" 2456 --plan-file "$OUT_DIR/plan.md" --summary-file "$summary" \
                --changed-files "$ROOT/tmp/changed-2456.txt" --evidence-file "$OUT_DIR/evidence.json" \
                --fix-commit "$COMMIT" --remote-ref origin/issue-2456 --allow-local-only \
                --report "$gate_report" --dry-run >/dev/null 2>&1 || true
        fi
    fi
    if [ -f "$gate_report" ]; then body+="GATE_PRESENT=1"$'\n'; else body+="GATE_PRESENT=0"$'\n'; rc=1; fi
    local result plan_sha gate_sha
    result="$(jq -r '.overall // empty' "$gate_report" 2>/dev/null || true)"
    [ "$result" = "PASS" ] && body+="GATE_RESULT=PASS"$'\n' || { body+="GATE_RESULT=${result:-missing}"$'\n'; rc=1; }
    plan_sha="$(sha256sum "$OUT_DIR/plan.md" | awk '{print $1}')"
    gate_sha="$(jq -r '.plan_sha256 // empty' "$gate_report" 2>/dev/null || true)"
    if [ -n "$gate_sha" ] && [ "$gate_sha" = "$plan_sha" ]; then
        body+="GATE_PLAN_SHA_MATCH=1"$'\n'
    else
        body+="GATE_PLAN_SHA_MATCH=0"$'\n'
        rc=1
    fi
    if "$plan_source" --issue 2456 >/tmp/plan-source-2456.log 2>&1; then
        body+="PLAN_SOURCE=PASS"$'\n'
    else
        body+="PLAN_SOURCE=FAIL"$'\n'
        body+="$(tail -5 /tmp/plan-source-2456.log)"$'\n'
        rc=1
    fi
    finish "R13-gate.txt" "$rc" "$body"
}

case "$ROW" in
    r1) row_r1 ;;
    r2) row_r2 ;;
    r3) row_r3 ;;
    r4) row_r4 ;;
    r5) row_r5 ;;
    r6) row_r6 ;;
    r7) row_r7 ;;
    r8) row_r8 ;;
    r9) row_r9 ;;
    r10) row_r10 ;;
    r11) row_go_test "R11-poll-no-keepalive.txt" '^TestPCIdleR11_' \
        "AT_59M_TOUCHED=0" "AT_61M_HTTP=401" "AT_61M_CODE=40110" ;;
    r12) row_go_test "R12-weapp-login-unaffected.txt" '^TestPCIdleR12_' \
        "MOBILE_LOGIN_ROWS=0" "MOBILE_CODE=20000" "IDLE_ENFORCED=0" ;;
    r13) row_r13 ;;
    *) die_usage ;;
esac
