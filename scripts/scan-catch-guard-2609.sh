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

while [ "$#" -gt 0 ]; do
  case "$1" in
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
