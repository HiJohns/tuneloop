#!/usr/bin/env bash
# scripts/verify-2592.sh — #2592「商户/成员链路 5xx 文案固定字面量 + 诊断只进日志」证据生成器。
#
# 用法：bash scripts/verify-2592.sh <r1|r2|r3|r4|r5>
#
# 每行把结果写入 docs/evidence/2592/R*-*.txt（首行 provenance），并打印机器可判定的
# 断言键值；任一行断言不成立以非 0 退出（2 = 基础设施错误）。
#
# 判定口径（Accepted Plan / D5 / D6 / D8）：
#   R1 `"message": "中文…" + x.Error()` 全仓归零（目录级扫描 backend/handlers/*.go），
#      且 origin/develop 基线中的每一行旧拼接在交付树中都不再存在（站点级）。
#   R2 变体形态归零（`Message:` 结构体字段 / `Message: fmt.Sprintf(...)`）+ 5 个固定
#      字面量的站点计数 5/1/1/3/1（锁定 11 处，脚本不依赖行号）。
#   R3 诊断仍进日志：11 个站点逐一在固定文案行上方 ≤6 行内找到引用同一 error 接收者的
#      log 调用（禁止文件级 grep 兜底）。
#   R4 运行时只回固定文案：9 个可达站点（merchant.go:632 / merchant_member.go:960 为
#      防御性不可达，由 R2/R3 static 断言，见 D6）。
#   R5 编译烟测（cd backend && go build ./...）。
#
# 重命令（go build/test）经 heavy-run.sh 持 host 锁；已处锁内（gate）时用
# VERIFY_2592_NO_LOCK=1 跳过二次取锁。
set -uo pipefail

ROW="${1:-}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$REPO_ROOT/docs/evidence/2592"
ISSUE=2592
SCAN_RE='"message":\s*"[^"]*[\x{4e00}-\x{9fff}][^"]*"\s*\+\s*[A-Za-z_][\w.]*\.Error\(\)'
BASELINE_REF="${VERIFY_2592_BASELINE_REF:-origin/develop}"
TARGET_FILES=(backend/handlers/merchant.go backend/handlers/merchant_member.go)
# 固定文案 → 期望计数（D5 唯一口径）
LIT_OWNER='读取商户所有人信息失败，请稍后重试'
LIT_VIEWER='读取当前登录账号失败，请稍后重试'
LIT_TARGET='查询目标成员失败，请稍后重试'
LIT_TRANSFER='转让所有权失败（IAM），请稍后重试'
LIT_SYNC='IAM 已转让，但本地同步失败，请联系平台人工核对'
EXPECT_OWNER=5
EXPECT_VIEWER=1
EXPECT_TARGET=1
EXPECT_TRANSFER=3
EXPECT_SYNC=1
EXPECT_SITES=11
# 运行时可达站点（9）：Issue 表中的 11 行减去 D6 声明的 2 个防御性不可达分支。
DEFENSIVE_SITES=(merchant.go:632 merchant_member.go:960)
RUNTIME_SITES=(
    merchant.go:621
    merchant_member.go:55
    merchant_member.go:62
    merchant_member.go:497
    merchant_member.go:614
    merchant_member.go:811
    merchant_member.go:854
    merchant_member.go:879
    merchant_member.go:916
)
HEAVY="${OPENCODE_HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"

cd "$REPO_ROOT" || exit 2
mkdir -p "$OUT_DIR" "$REPO_ROOT/tmp"

if ! command -v go >/dev/null 2>&1 && [ -x /usr/local/go/bin/go ]; then
    export PATH="/usr/local/go/bin:$PATH"
fi
# 后端测试库与构建缓存（与 #2497 口径一致；禁止 TUNELOOP_TEST_ALLOW_SKIP=1 —— 缺库必须
# 显式失败，不得变成静默 skip）。
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_local}"
export TUNELOOP_DB="${TUNELOOP_DB:-tuneloop}"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
unset TUNELOOP_TEST_ALLOW_SKIP
mkdir -p "$TMPDIR" "$GOCACHE"

usage() { echo "usage: $0 <r1|r2|r3|r4|r5>" >&2; exit 2; }
[ -n "$ROW" ] || usage

for f in "${TARGET_FILES[@]}"; do
    if [ ! -f "$f" ]; then
        echo "ERROR:missing-handler-file:$f" >&2
        exit 2
    fi
done
if ! git rev-parse --verify "$BASELINE_REF" >/dev/null 2>&1; then
    echo "ERROR:baseline-ref-unresolvable:$BASELINE_REF" >&2
    exit 2
fi

FIX_SHA="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
[ -n "$FIX_SHA" ] || FIX_SHA="$(git rev-parse HEAD)"
SHORT="$(git rev-parse --short=8 "$FIX_SHA")"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)docs/evidence/$ISSUE" ':(exclude)tmp')" ]; then
    DIRTY=true
fi
PROVENANCE="# build=1.0.$SHORT # commit=$FIX_SHA # dirty=$DIRTY"

finish() { # artifact rc body
    local name=$1 rc=$2 body=$3
    {
        printf '%s\n' "$PROVENANCE"
        printf '%s\n' "$body"
    } > "$OUT_DIR/$name" || { echo "ERROR:artifact-write-failed:$name" >&2; exit 2; }
    printf '%s\n' "$body"
    [ "$rc" = "0" ] || return 1
}

heavy() { # run a heavy command under the host lock (standalone rows only)
    if [ "${VERIFY_2592_NO_LOCK:-0}" = "1" ]; then
        "$@"
    else
        bash "$HEAVY" -- "$@"
    fi
}

handler_files() { # directory-wide scan set: backend/handlers/*.go
    find backend/handlers -maxdepth 1 -type f -name '*.go' | LC_ALL=C sort
}

cjk_err_hits() { # directory-wide `"message": "中文…" + x.Error()` count
    local total=0 f n
    while IFS= read -r f; do
        n="$(grep -Pc "$SCAN_RE" "$f" 2>/dev/null || true)"
        [ -n "$n" ] || n=0
        total=$((total + n))
    done < <(handler_files)
    printf '%s' "$total"
}

cjk_err_hit_lines() {
    local f
    while IFS= read -r f; do
        grep -Pn "$SCAN_RE" "$f" 2>/dev/null | sed "s|^|$f:|"
    done < <(handler_files)
}

baseline_sites() { # "file|line|content" for every hit on the baseline ref
    git grep -nP "$SCAN_RE" "$BASELINE_REF" -- 'backend/handlers/*.go' 2>/dev/null \
        | sed "s|^$BASELINE_REF:||" \
        | awk -F: '{file=$1; line=$2; sub(/^[^:]*:[^:]*:/, ""); print file"|"line"|"$0}'
}

# baseline_covered: how many baseline concatenation lines no longer exist in the delivery tree.
baseline_covered() { # $1: file with baseline_sites rows → "COVERED=/TOTAL=/MISSING_SITE="
    python3 - "$1" <<'PY'
import re
import sys

rows = [line.rstrip("\n") for line in open(sys.argv[1], encoding="utf-8") if line.strip()]
norm = lambda s: re.sub(r"\s+", "", s)
files = {}
missing = []
for row in rows:
    path, bline, content = row.split("|", 2)
    if path not in files:
        try:
            with open(path, encoding="utf-8") as fh:
                files[path] = {norm(l) for l in fh.read().splitlines()}
        except OSError:
            files[path] = None
    seen = files[path]
    if seen is None:
        missing.append(f"{path}:{bline}:missing-file")
    elif norm(content) in seen:
        missing.append(f"{path}:{bline}:old-concat-still-present")
print(f"COVERED={len(rows) - len(missing)}")
print(f"TOTAL={len(rows)}")
for m in missing:
    print(f"MISSING_SITE={m}")
raise SystemExit(0 if rows and not missing else 1)
PY
}

# log_sites: per-site "SITE <file>:<line> covered=0/1" using the literal → receiver map.
log_sites() { # stdin: none
    LIT_OWNER="$LIT_OWNER" LIT_VIEWER="$LIT_VIEWER" LIT_TARGET="$LIT_TARGET" \
        LIT_TRANSFER="$LIT_TRANSFER" LIT_SYNC="$LIT_SYNC" \
        python3 - "${TARGET_FILES[@]}" <<'PY'
import os
import re
import sys

# literal -> acceptable error receiver identifiers found in the log call.
SPECS = [
    (os.environ["LIT_OWNER"], {"ownerErr"}),
    (os.environ["LIT_VIEWER"], {"viewerErr"}),
    (os.environ["LIT_TARGET"], {"err"}),
    (os.environ["LIT_TRANSFER"], {"err", "transferErr"}),
    (os.environ["LIT_SYNC"], {"syncErr"}),
]
WINDOW = 6
LOG_RE = re.compile(r"log\.(Printf|Println|Print)\(")

covered = 0
total = 0
lost = 0
for path in sys.argv[1:]:
    with open(path, encoding="utf-8") as fh:
        lines = fh.read().splitlines()
    for idx, line in enumerate(lines):
        if line.lstrip().startswith("//"):
            continue
        for literal, receivers in SPECS:
            if literal not in line:
                continue
            total += 1
            hit = None
            # Nearest log call within WINDOW lines above the fixed literal; the
            # receiver may sit on a continuation line of a multi-line log call.
            for k in range(idx - 1, max(-1, idx - 1 - WINDOW), -1):
                if not LOG_RE.search(lines[k]):
                    continue
                for j in range(k, min(idx, k + 3)):
                    if any(re.search(rf"\b{re.escape(r)}\b", lines[j]) for r in receivers):
                        hit = k
                        break
                if hit is not None:
                    break
            if hit is None:
                lost += 1
                print(f"SITE {path}:{idx + 1} covered=0 reason=no-log-in-window literals={sorted(receivers)}")
            else:
                covered += 1
                print(f"SITE {path}:{idx + 1} covered=1 log={hit + 1}")
            break
print(f"LOG_PER_SITE={covered}")
print(f"SITE_TOTAL={total}")
print(f"LOST_DETAIL={lost}")
raise SystemExit(0 if lost == 0 and total > 0 and covered == total else 1)
PY
}

rc=0
case "$ROW" in
r1)
    mapfile -t SCANNED < <(handler_files)
    hits="$(cjk_err_hits)"
    hit_lines="$(cjk_err_hit_lines)"
    baseline_rows="$(baseline_sites)"
    baseline_hits="$(printf '%s\n' "$baseline_rows" | sed '/^$/d' | wc -l | tr -d ' ')"
    baseline_sites_file="$REPO_ROOT/tmp/verify-$ISSUE-baseline-sites.txt"
    printf '%s\n' "$baseline_rows" > "$baseline_sites_file"
    baseline_scan="$(baseline_covered "$baseline_sites_file")"
    baseline_scan_rc=$?
    covered="$(printf '%s\n' "$baseline_scan" | sed -n 's/^COVERED=//p')"
    missing_sites="$(printf '%s\n' "$baseline_scan" | sed -n '/^MISSING_SITE=/p')"

    body="SCAN_SCOPE=backend/handlers/*.go（目录级，不硬编码文件清单）"$'\n'
    body+="SCAN_RE=$SCAN_RE"$'\n'
    body+="BASELINE_REF=$BASELINE_REF"$'\n'
    body+="BASELINE_HITS=$baseline_hits"$'\n'
    body+="SCANNED_FILES=${#SCANNED[@]}"$'\n'
    body+="BASELINE_SITES_COVERED=${covered:-0}/$baseline_hits（站点级：基线旧拼接行在交付树中不再出现）"$'\n'
    body+="CJK_ERR_CONCAT=$hits"$'\n'
    if [ "$hits" != "0" ]; then
        body+="HIT_LINES:"$'\n'"$hit_lines"$'\n'
    fi
    if [ -n "$missing_sites" ]; then
        body+="MISSING_BASELINE_SITES:"$'\n'"$missing_sites"$'\n'
    fi
    [ "$hits" = "0" ] || rc=1
    [ "$baseline_scan_rc" = "0" ] || rc=1
    [ "${covered:-0}" = "$baseline_hits" ] || rc=1
    [ "$baseline_hits" -ge 1 ] 2>/dev/null || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R1-scan.txt "$rc" "$body"
    ;;
r2)
    msg_field=0
    msg_sprintf=0
    gin_details=""
    for f in "${TARGET_FILES[@]}"; do
        n="$(grep -Pc 'Message:\s*"[^"]*[\x{4e00}-\x{9fff}][^"]*"\s*\+\s*[A-Za-z_][\w.]*\.Error\(\)' "$f" 2>/dev/null || true)"
        [ -n "$n" ] || n=0
        msg_field=$((msg_field + n))
        s="$(grep -Pc 'Message:\s*fmt\.Sprintf\(\s*"[^"]*[\x{4e00}-\x{9fff}]' "$f" 2>/dev/null || true)"
        [ -n "$s" ] || s=0
        msg_sprintf=$((msg_sprintf + s))
        d="$(grep -Pn "$SCAN_RE" "$f" 2>/dev/null | sed "s|^|$f:|" || true)"
        [ -z "$d" ] || gin_details+="$d"$'\n'
    done
    lit_owner=0
    lit_viewer=0
    lit_target=0
    lit_transfer=0
    lit_sync=0
    for f in "${TARGET_FILES[@]}"; do
        c="$(grep -Fc -- "$LIT_OWNER" "$f" 2>/dev/null || true)"; [ -n "$c" ] || c=0; lit_owner=$((lit_owner + c))
        c="$(grep -Fc -- "$LIT_VIEWER" "$f" 2>/dev/null || true)"; [ -n "$c" ] || c=0; lit_viewer=$((lit_viewer + c))
        c="$(grep -Fc -- "$LIT_TARGET" "$f" 2>/dev/null || true)"; [ -n "$c" ] || c=0; lit_target=$((lit_target + c))
        c="$(grep -Fc -- "$LIT_TRANSFER" "$f" 2>/dev/null || true)"; [ -n "$c" ] || c=0; lit_transfer=$((lit_transfer + c))
        c="$(grep -Fc -- "$LIT_SYNC" "$f" 2>/dev/null || true)"; [ -n "$c" ] || c=0; lit_sync=$((lit_sync + c))
    done
    site_sum=$((lit_owner + lit_viewer + lit_target + lit_transfer + lit_sync))

    body="SCAN_FILES=$(printf '%s' "${TARGET_FILES[*]}")"$'\n'
    body+="MSG_FIELD_CONCAT=$msg_field（Message: \"中文\"+x.Error()）"$'\n'
    body+="MSG_SPRINTF_CONCAT=$msg_sprintf（Message: fmt.Sprintf(\"中文…\")）"$'\n'
    body+="LIT_OWNER=$lit_owner（期望 $EXPECT_OWNER）"$'\n'
    body+="LIT_VIEWER=$lit_viewer（期望 $EXPECT_VIEWER）"$'\n'
    body+="LIT_TARGET=$lit_target（期望 $EXPECT_TARGET）"$'\n'
    body+="LIT_TRANSFER=$lit_transfer（期望 $EXPECT_TRANSFER）"$'\n'
    body+="LIT_SYNC=$lit_sync（期望 $EXPECT_SYNC）"$'\n'
    body+="LIT_SITES=$site_sum（期望 $EXPECT_SITES）"$'\n'
    body+="GIN_CONCAT_DETAILS=${gin_details}"$'\n'
    [ "$msg_field" = "0" ] || rc=1
    [ "$msg_sprintf" = "0" ] || rc=1
    [ "$lit_owner" = "$EXPECT_OWNER" ] || rc=1
    [ "$lit_viewer" = "$EXPECT_VIEWER" ] || rc=1
    [ "$lit_target" = "$EXPECT_TARGET" ] || rc=1
    [ "$lit_transfer" = "$EXPECT_TRANSFER" ] || rc=1
    [ "$lit_sync" = "$EXPECT_SYNC" ] || rc=1
    [ "$site_sum" = "$EXPECT_SITES" ] || rc=1
    [ -z "$gin_details" ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R2-forms.txt "$rc" "$body"
    ;;
r3)
    site_out="$(log_sites)"
    site_rc=$?
    log_per_site="$(printf '%s\n' "$site_out" | sed -n 's/^LOG_PER_SITE=//p')"
    site_total="$(printf '%s\n' "$site_out" | sed -n 's/^SITE_TOTAL=//p')"
    lost_detail="$(printf '%s\n' "$site_out" | sed -n 's/^LOST_DETAIL=//p')"
    body="WINDOW=6（固定文案行上方 ≤6 行内的 log 调用）"$'\n'
    body+="LOG_PER_SITE=${log_per_site:-0}（期望 $EXPECT_SITES）"$'\n'
    body+="SITE_TOTAL=${site_total:-0}"$'\n'
    body+="LOST_DETAIL=${lost_detail:-0}"$'\n'
    body+="$(printf '%s\n' "$site_out" | sed -n '/^SITE /p')"$'\n'
    [ "$site_rc" = "0" ] || rc=1
    [ "${log_per_site:-0}" = "$EXPECT_SITES" ] || rc=1
    [ "${lost_detail:-1}" = "0" ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R3-logs.txt "$rc" "$body"
    ;;
r4)
    run_log="$REPO_ROOT/tmp/verify-$ISSUE-r4-runtime.log"
    ( cd "$REPO_ROOT/backend" && heavy go test ./handlers \
        -run 'Sanitized_2592|LocalSyncFailure_2546' -count=1 -v -timeout 10m ) > "$run_log" 2>&1
    run_exit=$?

    found_sites="$(grep -oE 'SITE=[a-z_]+\.go:[0-9]+' "$run_log" | sed 's/^SITE=//' | LC_ALL=C sort -u)"
    runtime_count="$(printf '%s\n' "$found_sites" | sed '/^$/d' | wc -l | tr -d ' ')"
    missing_runtime=""
    for want in "${RUNTIME_SITES[@]}"; do
        printf '%s\n' "$found_sites" | grep -qx "$want" || missing_runtime+="MISSING_RUNTIME_SITE=$want"$'\n'
    done
    defensive_marked=""
    for d in "${DEFENSIVE_SITES[@]}"; do
        if printf '%s\n' "$found_sites" | grep -qx "$d"; then
            defensive_marked+="DEFENSIVE_SITE_CLAIMED=$d"$'\n'
        fi
    done
    leak=0
    grep -qE 'LEAK_SENTINEL=0.*FAIL|LEAK=1|LEAK_IAM_ADDR=1|LEAK_INTERNAL_ID=1|LEAK_ROWS_DIAG=1' "$run_log" && leak=1
    fail_count="$(grep -cE '^(--- FAIL|FAIL[[:space:]])' "$run_log" || true)"
    [ -n "$fail_count" ] || fail_count=0
    pass_count="$(grep -cE '^--- PASS' "$run_log" || true)"
    [ -n "$pass_count" ] || pass_count=0
    uuid_leak=0
    grep -q 'UUID_LEAK=1' "$run_log" && uuid_leak=1
    # 916 分支必须显式给出 UUID_LEAK=0（既有 #2546 断言已加强）。
    uuid_ok=0
    grep -qE 'SITE=merchant_member\.go:916 .*UUID_LEAK=0' "$run_log" && uuid_ok=1

    body="RUNTIME_CMD=cd backend && go test ./handlers -run 'Sanitized_2592|LocalSyncFailure_2546' -count=1 -v -timeout 10m"$'\n'
    body+="TEST_DB_ENV=TUNELOOP_DB=$TUNELOOP_DB TUNELOOP_TEST_DB=$TUNELOOP_TEST_DB POSTGRES_HOST=$POSTGRES_HOST POSTGRES_PORT=$POSTGRES_PORT"$'\n'
    body+="RUNTIME_EXIT=$run_exit"$'\n'
    body+="RUNTIME_SITES=$runtime_count（期望 ${#RUNTIME_SITES[@]}）"$'\n'
    body+="RUNTIME_SITE_LIST=$(printf '%s' "$found_sites" | paste -sd, -)"$'\n'
    body+="DEFENSIVE_SITES=$(printf '%s' "${DEFENSIVE_SITES[*]}")"$'\n'
    [ -z "$missing_runtime" ] || body+="$missing_runtime"
    [ -z "$defensive_marked" ] || body+="$defensive_marked"
    body+="LEAK=$leak"$'\n'
    body+="UUID_LEAK=$uuid_leak"$'\n'
    body+="UUID_SITE_MARKER=$uuid_ok"$'\n'
    body+="# pass $pass_count"$'\n'
    body+="# fail $fail_count"$'\n'
    body+="--- runtime tail ---"$'\n'
    body+="$(grep -E '^(--- (PASS|FAIL)|=== RUN)' "$run_log" | tail -24)"$'\n'
    [ "$run_exit" = "0" ] || rc=1
    [ "$runtime_count" = "${#RUNTIME_SITES[@]}" ] || rc=1
    [ -z "$missing_runtime" ] || rc=1
    [ -z "$defensive_marked" ] || rc=1
    [ "$leak" = "0" ] || rc=1
    [ "$uuid_leak" = "0" ] || rc=1
    [ "$uuid_ok" = "1" ] || rc=1
    [ "$fail_count" = "0" ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R4-runtime.txt "$rc" "$body"
    ;;
r5)
    build_log="$REPO_ROOT/tmp/verify-$ISSUE-r5-build.log"
    ( cd "$REPO_ROOT/backend" && heavy go build ./... ) > "$build_log" 2>&1
    build_exit=$?
    body="BUILD_CMD=cd backend && go build ./..."$'\n'
    body+="BUILD_EXIT=$build_exit"$'\n'
    if [ "$build_exit" = "0" ]; then
        body+="BUILD_OK=1"$'\n'
    else
        body+="BUILD_OK=0"$'\n'
        rc=1
    fi
    body+="$(tail -8 "$build_log" | sed 's/^/LOG_TAIL: /')"$'\n'
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R5-build.txt "$rc" "$body"
    ;;
*)
    usage
    ;;
esac

exit "$rc"
