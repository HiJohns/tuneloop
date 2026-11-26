#!/usr/bin/env bash
# check-doc-fences.sh — Markdown 代码围栏结构守卫（#2590）
#
# 用途：发现「围栏奇偶失衡」「标题被吞进代码块」「EOF 仍在代码块内」「闭合围栏带 info string」
# 这类结构性缺陷——它们不会让 grep 失败，却会让渲染后的整节内容消失。
#
# 用法：
#   check-doc-fences.sh [--root DIR]... [--file PATH]...
#                       [--require-heading-outside TEXT]... [--self-test]
#
# 扫描规则（与 #2590 Accepted Plan「变更点 2」一致，CLI 即契约）：
#   - 围栏行 = 行首 0–3 个空格 + 至少 3 个反引号；开启行允许 info string，闭合行不得带 info string。
#   - 4 空格及以上缩进属缩进代码块，不计入围栏；`~~~` 波浪号围栏不识别。
#   - 标题判定：行首 `#{2,6} `（level ≥2）才算文档标题——单 `#` 在代码块里通常是注释（`# 提取后端路由`）；
#     ```markdown / ```md 围栏内是「示例标题」，不计入。其余围栏内出现的 `##`+ 行 = 标题被围栏吞掉（缺陷）。
#
# 输出：
#   每个文件一行：FILE=<path> FENCES=<n> EVEN=<0|1> HEADINGS_INSIDE=<n> EOF_INSIDE=<0|1>
#   缺陷行：     VIOLATION file=<path> kind=<odd_fence|heading_inside|eof_inside|tagged_close> line=<n>
#   标题断言：   HEADING_STATE=<text>=<outside|inside|missing>
#   汇总行：     DOC_FILES_SCANNED=<n> ODD_FENCE_FILES=<n> HEADINGS_INSIDE_FILES=<n> EOF_INSIDE_FILES=<n> RESULT=<PASS|FAIL>
#
# 退出码：0 = 全部 EVEN=1 且零标题入块、零 EOF 未闭合、所有标题断言为 outside；
#         1 = 检出缺陷；2 = 用法/基础设施错误（路径不存在等，显式失败不静默通过）。
set -uo pipefail

ROOTS=()
FILES=()
REQ_HEADINGS=()
SELFTEST=0

usage() {
    sed -n '2,30p' "$0" | sed 's/^# \{0,1\}//' >&2
    exit 2
}

while [ $# -gt 0 ]; do
    case "$1" in
        --root) [ $# -ge 2 ] || usage; ROOTS+=("$2"); shift 2 ;;
        --file) [ $# -ge 2 ] || usage; FILES+=("$2"); shift 2 ;;
        --require-heading-outside) [ $# -ge 2 ] || usage; REQ_HEADINGS+=("$2"); shift 2 ;;
        --self-test) SELFTEST=1; shift ;;
        --help|-h) usage ;;
        *) echo "ERROR: unknown argument: $1" >&2; usage ;;
    esac
done

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

# scan_file <path> — 输出该文件的结构行 + 违规行；返回 0（无违规）/1（有违规）。
#
# 标题判定（#2590 Rev.2「围栏内的行首 `#` 不计为标题」的可用实现）：
#   - 只认行首 `#{2,6} `（level ≥2）：单 `#` 在代码块里几乎都是注释（`# 提取后端路由`），不判为标题；
#   - ```markdown / ```md 围栏内的 `#` 是「示例标题」，不计入（docs/archive 的示例块）；
#   - 其余语言的围栏内出现 `##`+ 行 = 文档标题被围栏吞掉，正是要检出的缺陷。
scan_file() {
    local file=$1
    awk '
        BEGIN { fences = 0; inside = 0; info = ""; headings_inside = 0; first_heading_line = 0; rc = 0 }
        {
            line = $0
            if (line ~ /^ {0,3}```/) {
                fences++
                rest = line
                sub(/^ {0,3}`{3,}/, "", rest)
                if (inside == 0) {
                    inside = 1
                    info = rest
                    gsub(/^[ \t]+|[ \t]+$/, "", info)
                } else {
                    inside = 0
                    info = ""
                    if (rest ~ /[^ \t]/) {
                        printf "VIOLATION file=%s kind=tagged_close line=%d\n", FILENAME, NR
                        rc = 1
                    }
                }
                next
            }
            if (inside == 1 && line ~ /^#{2,6} / && info !~ /^(markdown|md)([ \t,]|$)/) {
                headings_inside++
                if (first_heading_line == 0) first_heading_line = NR
            }
        }
        END {
            even = (fences % 2 == 0) ? 1 : 0
            printf "FILE=%s FENCES=%d EVEN=%d HEADINGS_INSIDE=%d EOF_INSIDE=%d\n",
                   FILENAME, fences, even, headings_inside, inside
            if (even == 0) {
                printf "VIOLATION file=%s kind=odd_fence line=%d\n", FILENAME, NR
                rc = 1
            }
            if (headings_inside > 0) {
                printf "VIOLATION file=%s kind=heading_inside line=%d\n", FILENAME, first_heading_line
                rc = 1
            }
            if (inside != 0) {
                printf "VIOLATION file=%s kind=eof_inside line=%d\n", FILENAME, NR
                rc = 1
            }
            exit rc
        }
    ' "$file"
}

# heading_state <path> <text> — 输出 HEADING_STATE=<text>=<outside|inside|missing>
heading_state() {
    local file=$1 text=$2
    awk -v TEXT="$text" '
        BEGIN { state = "missing"; inside = 0 }
        {
            if ($0 ~ /^ {0,3}```/) { inside = 1 - inside; next }
            if (state == "missing" && index($0, TEXT) > 0) {
                state = (inside == 1) ? "inside" : "outside"
            }
        }
        END { printf "HEADING_STATE=%s=%s\n", TEXT, state }
    ' "$file"
}

run_selftest() {
    local dir="$REPO_ROOT/tmp/fence-selftest"
    rm -rf "$dir"
    mkdir -p "$dir" || { echo "ERROR: cannot create $dir" >&2; exit 2; }

    cat > "$dir/good.md" <<'EOF'
# 正常文件

```json
{"code": 20000}
```

## 正常标题

正文。
EOF

    cat > "$dir/odd.md" <<'EOF'
# 奇数围栏

```json
{"code": 20000}

## 这个标题被吞了
EOF

    cat > "$dir/inside.md" <<'EOF'
# 偶数围栏但标题入块

```json
## 藏在代码块里的标题
{}
```
EOF

    cat > "$dir/eof.md" <<'EOF'
# EOF 仍未闭合

```json
{}
```

```bash
echo never-closed
EOF

    local cases=0 detected=0 false_positive=0
    local f out
    for f in good.md odd.md inside.md eof.md; do
        cases=$((cases + 1))
        out="$(scan_file "$dir/$f" 2>&1)"
        local bad
        bad="$(printf '%s\n' "$out" | grep -c '^VIOLATION ' || true)"
        if [ "$f" = "good.md" ]; then
            [ "${bad:-0}" -eq 0 ] || false_positive=$((false_positive + 1))
        else
            [ "${bad:-0}" -gt 0 ] && detected=$((detected + 1))
        fi
        printf 'SELFTEST_FILE=%s VIOLATIONS=%s\n' "$f" "${bad:-0}"
        printf '%s\n' "$out" | sed -n 's/^VIOLATION file=\([^ ]*\) kind=\([^ ]*\).*/SELFTEST_DETECT: \2/p'
    done

    local result="FAIL"
    if [ "$cases" -eq 4 ] && [ "$detected" -eq 3 ] && [ "$false_positive" -eq 0 ]; then
        result="PASS"
    fi
    printf 'SELFTEST_CASES=%d SELFTEST_DETECTED=%d SELFTEST_FALSE_POSITIVE=%d RESULT=%s\n' \
        "$cases" "$detected" "$false_positive" "$result"
    [ "$result" = "PASS" ] || exit 1
    exit 0
}

if [ "$SELFTEST" = "1" ]; then
    run_selftest
fi

[ "${#ROOTS[@]}" -gt 0 ] || [ "${#FILES[@]}" -gt 0 ] || usage

TARGETS=()
for d in "${ROOTS[@]:-}"; do
    [ -n "$d" ] || continue
    [ -d "$d" ] || { echo "ERROR: root not found: $d" >&2; exit 2; }
    while IFS= read -r p; do TARGETS+=("$p"); done < <(find "$d" -type f -name '*.md' | sort)
done
for f in "${FILES[@]:-}"; do
    [ -n "$f" ] || continue
    [ -f "$f" ] || { echo "ERROR: file not found: $f" >&2; exit 2; }
    TARGETS+=("$f")
done

# 去重（同一文件重复传入只报一次）
UNIQ=()
while IFS= read -r p; do [ -n "$p" ] && UNIQ+=("$p"); done < <(printf '%s\n' "${TARGETS[@]:-}" | awk 'NF' | sort -u)

if [ "${#UNIQ[@]}" -eq 0 ]; then
    echo "ERROR: no markdown files matched" >&2
    exit 2
fi

scanned=0 odd=0 inside_files=0 eof_files=0
RC=0
SCAN_OUT="$(mktemp)" || exit 2
trap 'rm -f "$SCAN_OUT"' EXIT
for p in "${UNIQ[@]}"; do
    out="$(scan_file "$p" 2>&1)"
    printf '%s\n' "$out" >> "$SCAN_OUT"
    scanned=$((scanned + 1))
    printf '%s\n' "$out" | grep -Eq '^VIOLATION .* kind=odd_fence' && odd=$((odd + 1))
    line="$(printf '%s\n' "$out" | grep -m1 '^FILE=')"
    hi="$(printf '%s' "$line" | sed -n 's/.*HEADINGS_INSIDE=\([0-9]*\).*/\1/p')"
    ei="$(printf '%s' "$line" | sed -n 's/.*EOF_INSIDE=\([0-9]*\).*/\1/p')"
    [ "${hi:-0}" -gt 0 ] && inside_files=$((inside_files + 1))
    [ "${ei:-0}" -gt 0 ] && eof_files=$((eof_files + 1))
    printf '%s\n' "$out" | grep -q '^VIOLATION ' && RC=1
done

cat "$SCAN_OUT"

HEAD_RC=0
for t in "${REQ_HEADINGS[@]:-}"; do
    [ -n "$t" ] || continue
    st="$(heading_state "${UNIQ[0]}" "$t")"
    printf '%s\n' "$st"
    case "$st" in
        *"=outside") ;;
        *) HEAD_RC=1 ;;
    esac
done

RESULT="PASS"
if [ "$RC" != "0" ] || [ "$HEAD_RC" != "0" ] || [ "$odd" -gt 0 ] || [ "$inside_files" -gt 0 ] || [ "$eof_files" -gt 0 ]; then
    RESULT="FAIL"
fi
printf 'DOC_FILES_SCANNED=%d ODD_FENCE_FILES=%d HEADINGS_INSIDE_FILES=%d EOF_INSIDE_FILES=%d RESULT=%s\n' \
    "$scanned" "$odd" "$inside_files" "$eof_files" "$RESULT"

[ "$RESULT" = "PASS" ] && exit 0
exit 1
