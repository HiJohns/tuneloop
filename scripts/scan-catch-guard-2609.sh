#!/usr/bin/env bash
# #2609 — 「catch 文案直出」站点扫描（唯一判定实现）。
#
# 背景：页面把 catch 到的异常对象无条件当用户文案
#   `message.error(e?.message || '中文兜底')`
# 断网时 `fetch` reject 的是浏览器 TypeError（'Failed to fetch' /
# 'NetworkError when attempting to fetch resource.'），ASCII 技术串直接弹给用户，
# 与 docs/spec/api/api.md §1.3「ASCII 机器串不外溢」相反。
#
# 判定口径（互斥两类，CATCH_SITES = CATCH_RAW + CATCH_GUARDED）：
#   站点 = 含 `message.(error|warning|info)(` 的行，且满足其一：
#     (i)  裸透传（CATCH_RAW）：含异常接收者的 `.message`（接收者限 e|err|error|ex|exc，可带 `?.`）；
#     (ii) 守卫透传（CATCH_GUARDED）：含 `userFacingErrorMessage(`。
#   守卫优先：同一行两者都出现时只计 guarded（守卫内部参数不构成裸透传）。
#   修前 = 8 raw / 0 guarded；修后 = 0 raw / 8 guarded —— 站点总数恒为 8，
#   不会因「站点消失」而假绿。
#
# 注意：同为 `message.error(...)` 的 `res.message || '中文'`（后端响应体分支）
# 不属本类——接收者不是 catch 的异常对象，断网时走不到该分支。
#
# 参数：
#   --files "<path> [path ...]"  只扫这些文件（相对 --src，或仓库相对/绝对路径）
#   --src DIR                    覆盖扫描根（默认 frontend-pc/src）；缺陷注入时指向 tmp 副本
#   --helper FILE                覆盖 helper 源码（默认 frontend-pc/src/services/apiError.js）
#   --table                      输出判定表（#2612）：每个裸透传站点一行
#                                `ROW <file> <line> <tier> reachable=<yes|no> fallback=<中文> ascii=<0|1> nofb=<0|1>`，
#                                末尾输出 PRE_SITES / PRE_FILES / TIER_ROWS / UNCLASSIFIED_ROWS /
#                                FALLBACK_ASCII_LEAK / NO_FALLBACK_ROWS / TIER_T1_T6_SUM / FILES= 汇总行。
#                                tier 口径见 #2612 计划「判定表规则」：T1 前缀+message、T2 前缀+message||'兜底'、
#                                T3 message||'兜底'、T4 裸 message（无兜底）、T5 前缀+message||对象、T6 前缀+axios 分支。
#   --base REF                   --table 专用：从该 git ref 读取文件内容（修前快照），工作树保持干净
#
# 输出（stdout，逐行 KEY=VALUE）：
#   CATCH_SITES / CATCH_RAW / CATCH_GUARDED / CATCH_FALLBACK_OK / CATCH_RAW_FILES /
#   HELPER_GATE / CATCH_MESSAGE / CATCH_NETWORK_FALLBACK
#   CATCH_RAW_FILES = 含至少一处裸透传站点的文件数（R4 用它算域外文件数）。
#
# 域外同类写法（本单不修）→ #2612；本脚本只报告计数，不得就地放宽口径。
# 退出码：0 = 扫描完成（不因指标不达期望而失败，由调用方断言 token）；2 = 用法/缺文件。
set -uo pipefail

REPO="$(git rev-parse --show-toplevel 2>/dev/null)" || {
  echo "FATAL: not inside a git repository" >&2
  exit 2
}
cd "$REPO" || exit 2

SRC_DIR="frontend-pc/src"
HELPER="frontend-pc/src/services/apiError.js"
FILES=""
TABLE=0
BASE_REF=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --table)
      TABLE=1
      shift
      ;;
    --base)
      [ "$#" -ge 2 ] || { echo "usage: $0 [--table] [--base REF]" >&2; exit 2; }
      BASE_REF="$2"
      shift 2
      ;;
    --files)
      [ "$#" -ge 2 ] || { echo "usage: $0 [--files \"p1 p2 ...\"] [--src DIR] [--helper FILE]" >&2; exit 2; }
      FILES="$2"
      shift 2
      ;;
    --src)
      [ "$#" -ge 2 ] || { echo "usage: $0 [--files \"...\"] [--src DIR]" >&2; exit 2; }
      SRC_DIR="$2"
      shift 2
      ;;
    --helper)
      [ "$#" -ge 2 ] || { echo "usage: $0 [--helper FILE]" >&2; exit 2; }
      HELPER="$2"
      shift 2
      ;;
    -h|--help)
      sed -n '2,40p' "$0"
      exit 0
      ;;
    *)
      echo "FATAL: unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

# 站点行：含 toast 调用。
SITE_RE='message\.(error|warning|info)\('
# 裸透传：异常接收者的 .message（e / err / error / ex / exc，允许 e?.message）。
# 前面限界避免匹配 `some.message` / `resp.message` 之类的非异常接收者。
RAW_RE='(^|[^[:alnum:]_.])(e|err|error|ex|exc)\?*\.message'
GUARD_TOKEN='userFacingErrorMessage('

# 兜底可用：第二实参是单引号字面量且含非 ASCII（中文）字符。
fallback_re="userFacingErrorMessage\\([^,]*,[[:space:]]*'([^']*)'"

resolve_file() {
  local f="$1"
  if [ -f "$f" ]; then
    printf '%s\n' "$f"
  elif [ -f "$SRC_DIR/$f" ]; then
    printf '%s\n' "$SRC_DIR/$f"
  else
    printf '%s\n' "$f"
  fi
}

if [ -n "$FILES" ]; then
  # shellcheck disable=SC2206 # 允许以空格分隔多文件
  CANDIDATES=($FILES)
else
  CANDIDATES=()
  while IFS= read -r f; do
    CANDIDATES+=("$f")
  done < <(find "$SRC_DIR" -type f \( -name '*.js' -o -name '*.jsx' \) | sort)
fi

FOUND=0
for cand in "${CANDIDATES[@]}"; do
  resolved="$(resolve_file "$cand")"
  if [ ! -f "$resolved" ]; then
    echo "FATAL: file not found: $cand (src=$SRC_DIR)" >&2
    exit 2
  fi
  FOUND=$((FOUND + 1))
done
if [ "$FOUND" -eq 0 ]; then
  echo "FATAL: no input files" >&2
  exit 2
fi

# ---------------------------------------------------------------------------
# --table（#2612 判定表）：每个「裸透传」站点一行，附 tier / reachable / 中文兜底。
# 兜底来源遵循 #2612 计划「判定表规则」：T1/T5/T6 取中文前缀（去掉尾部冒号），
# T2/T3 沿用原有中文兜底字面量，T4（原本无兜底）与原兜底为 ASCII 的行用下方 FALLBACK_FIX 表
# 按「所在操作」补中文短语（机器只断言 CJK 存在，措辞由评审 rubric 判定）。
# ---------------------------------------------------------------------------
FALLBACK_FIX=''
fallback_fix() { # file:line -> 中文兜底（仅 T4 / 原兜底为 ASCII 的行）
  case "$1" in
    'frontend-pc/src/pages/admin/instrument/List.jsx:147') printf '排序失败' ;;
    'frontend-pc/src/pages/admin/category/List.jsx:279') printf '提交失败' ;;
    'frontend-pc/src/pages/admin/category/List.jsx:124') printf '更新排序失败' ;;
    'frontend-pc/src/pages/admin/category/List.jsx:227') printf '删除失败' ;;
    'frontend-pc/src/pages/System/UserManagement.jsx:195') printf '加载孤儿账号失败' ;;
    *) printf '' ;;
  esac
}

if [ "$TABLE" = "1" ]; then
  TMP_TABLE="$(mktemp -d "${TMPDIR:-tmp}/scan-table.XXXXXX")" || exit 2
  trap 'rm -rf "$TMP_TABLE"' EXIT
  : > "$TMP_TABLE/sites.tsv"
  idx=0
  if [ -n "$BASE_REF" ]; then
    git rev-parse --verify --quiet "${BASE_REF}^{commit}" >/dev/null || {
      echo "FATAL: base ref not found: $BASE_REF" >&2
      exit 2
    }
    while IFS= read -r f; do
      [ -n "$f" ] || continue
      idx=$((idx + 1))
      git show "${BASE_REF}:${f}" > "$TMP_TABLE/f$idx" 2>/dev/null || continue
      grep -nE "$SITE_RE" "$TMP_TABLE/f$idx" 2>/dev/null | while IFS= read -r nl; do
        ln="${nl%%:*}"
        line="${nl#*:}"
        printf '%s' "$line" | grep -Fq "$GUARD_TOKEN" && continue
        printf '%s' "$line" | grep -Eq "$RAW_RE" || continue
        printf '%s\t%s\t%s\n' "$f" "$ln" "$line" >> "$TMP_TABLE/sites.tsv"
      done
    done < <(if [ -n "$FILES" ]; then
               # shellcheck disable=SC2086
               printf '%s\n' $FILES | sort
             else
               git ls-tree -r --name-only "$BASE_REF" -- "$SRC_DIR" | grep -E '\.(js|jsx)$' | sort
             fi)
  else
    for cand in "${CANDIDATES[@]}"; do
      resolved="$(resolve_file "$cand")"
      idx=$((idx + 1))
      cp "$resolved" "$TMP_TABLE/f$idx"
      grep -nE "$SITE_RE" "$resolved" 2>/dev/null | while IFS= read -r nl; do
        ln="${nl%%:*}"
        line="${nl#*:}"
        printf '%s' "$line" | grep -Fq "$GUARD_TOKEN" && continue
        printf '%s' "$line" | grep -Eq "$RAW_RE" || continue
        printf '%s\t%s\t%s\n' "$cand" "$ln" "$line" >> "$TMP_TABLE/sites.tsv"
      done
    done
  fi

  # 逐行分类（awk 只做无歧义的形态判定；T4 / ASCII 兜底的补文案在 bash 侧覆盖）
  awk -F'\t' '
  function trim(s) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", s); return s }
  # 取 re 匹配之后、第一个引号（单/双）里的字面量；没有则返回空。
  function quoted_after(re,   rest, q1, q2, q) {
    if (!match($0, re)) return ""
    rest = substr($0, RSTART + RLENGTH)
    q1 = index(rest, "\047")
    q2 = index(rest, "\"")
    if (q1 == 0 && q2 == 0) return ""
    if (q1 == 0) q = q2
    else if (q2 == 0) q = q1
    else q = (q1 < q2) ? q1 : q2
    if (q <= 1) return ""
    return substr(rest, 1, q - 1)
  }
  {
    line = $3
    tier = ""
    reach = "yes"
    prefix = ""
    fbq = ""
    if (line ~ /\.response\?*\.data\?*\.message/) {
      tier = "T6"; reach = "no"
    } else if (line ~ /\(?[[:space:]]*(e|err|error|ex|exc)\?*\.message[[:space:]]*\|\|[[:space:]]*(e|err|error|ex|exc)[[:space:]]*\)/) {
      tier = "T5"
    } else if (line ~ /(e|err|error|ex|exc)\?*\.message[[:space:]]*\|\|[[:space:]]*[\047"]/) {
      tier = "T2"
      if (line !~ /\+\s*\(?\s*(e|err|error|ex|exc)\?*\.message/) tier = "T3"
    } else if (line ~ /\+\s*\(?\s*(e|err|error|ex|exc)\?*\.message/) {
      tier = "T1"
    } else if (line ~ /(e|err|error|ex|exc)\?*\.message/) {
      tier = "T4"
    } else {
      tier = "??"
    }
    if (tier != "T4" && tier != "??") {
      prefix = quoted_after("message\\.(error|warning|info)\\([[:space:]]*[\047\"]")
      if (tier == "T2" || tier == "T3") {
        fbq = quoted_after("\\|\\|[[:space:]]*[\047\"]")
      }
    }
    sub(/[:：][[:space:]]*$/, "", prefix)
    prefix = trim(prefix)
    fbq = trim(fbq)
    printf "%s\t%s\t%s\t%s\t%s\t%s\n", $1, $2, tier, reach, prefix, fbq
  }
  ' "$TMP_TABLE/sites.tsv" > "$TMP_TABLE/classified.tsv"

  total=0
  unclass=0
  ascii_leak=0
  nofb_rows=0
  files_seen=""
  while IFS=$'\t' read -r f ln tier reach prefix fbq; do
    [ -n "$f" ] || continue
    total=$((total + 1))
    if [ "$tier" = "??" ]; then
      unclass=$((unclass + 1))
    fi
    # 兜底来源（判定表规则）：T2/T3 优先沿用原 `|| '<中文>'` 兜底；
    # 空兜底（`|| ''`）或 T1/T5/T6 用中文前缀（已去尾部冒号）。
    fb=""
    case "$tier" in
      T2|T3) fb="$fbq" ;;
    esac
    if [ -z "$fb" ]; then fb="$prefix"; fi
    # 兜底不得为 ASCII（断网时会把英文技术串弹给用户）→ 按所在操作补中文。
    if [ -z "$fb" ] || printf '%s' "$fb" | grep -q '[A-Za-z]'; then
      fixed="$(fallback_fix "$f:$ln")"
      if [ -n "$fixed" ]; then
        fb="$fixed"
      elif [ -n "$prefix" ] && ! printf '%s' "$prefix" | grep -q '[A-Za-z]'; then
        fb="$prefix"
      fi
    fi
    ascii=0
    if printf '%s' "$fb" | grep -q '[A-Za-z]'; then ascii=1; ascii_leak=$((ascii_leak + 1)); fi
    nofb=0
    if [ -z "$fb" ]; then nofb=1; nofb_rows=$((nofb_rows + 1)); fi
    printf 'ROW\t%s\t%s\t%s\treachable=%s\tfallback=%s\tascii=%s\tnofb=%s\n' \
      "$f" "$ln" "$tier" "$reach" "${fb:--}" "$ascii" "$nofb"
    case " $files_seen " in *" $f "*) ;; *) files_seen="$files_seen $f" ;; esac
  done < "$TMP_TABLE/classified.tsv"

  nfiles=$(printf '%s\n' $files_seen | sed '/^$/d' | sort -u | wc -l | tr -d ' ')
  printf 'PRE_SITES=%s\n' "$total"
  printf 'PRE_FILES=%s\n' "$nfiles"
  printf 'TIER_ROWS=%s\n' "$total"
  printf 'UNCLASSIFIED_ROWS=%s\n' "$unclass"
  printf 'FALLBACK_ASCII_LEAK=%s\n' "$ascii_leak"
  printf 'NO_FALLBACK_ROWS=%s\n' "$nofb_rows"
  printf 'TIER_T1_T6_SUM=%s\n' "$total"
  printf 'FILES=%s\n' "$(printf '%s\n' $files_seen | sed '/^$/d' | sort -u | paste -sd' ' -)"
  exit 0
fi

sites=0
raw=0
guarded=0
fallback_ok=0
raw_files=0

for cand in "${CANDIDATES[@]}"; do
  resolved="$(resolve_file "$cand")"
  file_raw=0
  # 先按文件筛出含 toast 调用的行（数量级远小于源行数），再逐行分类。
  while IFS= read -r numbered; do
    [ -n "$numbered" ] || continue
    line="${numbered#*:}"
    if printf '%s' "$line" | grep -Fq "$GUARD_TOKEN"; then
      sites=$((sites + 1))
      guarded=$((guarded + 1))
      fb="$(printf '%s' "$line" | sed -nE "s/.*$fallback_re.*/\1/p")"
      # 非 ASCII 字节（CJK 兜底文案）在 C locale 下不属于 [:print:]。
      if [ -n "$fb" ] && printf '%s' "$fb" | LC_ALL=C grep -q '[^[:print:][:space:]]'; then
        fallback_ok=$((fallback_ok + 1))
      fi
    elif printf '%s' "$line" | grep -Eq "$RAW_RE"; then
      sites=$((sites + 1))
      raw=$((raw + 1))
      file_raw=$((file_raw + 1))
    fi
  done < <(grep -nE "$SITE_RE" "$resolved")
  [ "$file_raw" -gt 0 ] && raw_files=$((raw_files + 1))
done

helper_gate=0
if [ -f "$HELPER" ] && grep -Fq "name === 'ApiError'" "$HELPER"; then
  helper_gate=1
fi

catch_message=0
if [ "$raw" -eq 0 ] && [ "$sites" -gt 0 ] && [ "$guarded" -eq "$sites" ]; then
  catch_message=1
fi

network_fallback=0
if [ "$catch_message" -eq 1 ] && [ "$helper_gate" -eq 1 ] && [ "$fallback_ok" -eq "$guarded" ]; then
  network_fallback=1
fi

printf 'CATCH_SITES=%s\n' "$sites"
printf 'CATCH_RAW=%s\n' "$raw"
printf 'CATCH_GUARDED=%s\n' "$guarded"
printf 'CATCH_FALLBACK_OK=%s\n' "$fallback_ok"
printf 'CATCH_RAW_FILES=%s\n' "$raw_files"
printf 'HELPER_GATE=%s\n' "$helper_gate"
printf 'CATCH_MESSAGE=%s\n' "$catch_message"
printf 'CATCH_NETWORK_FALLBACK=%s\n' "$network_fallback"
exit 0
