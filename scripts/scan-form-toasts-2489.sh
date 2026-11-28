#!/usr/bin/env bash
# #2489 R1 判定表（只读）—— 配置/定价/权限/商户类页面的每个 message.error(...)
#
# 分类口径（#2480 rubric）：
#   business-correctable = rubric 码（40002/40004/40005/40900）指向用户可改输入 → 持久提示
#   keep:instant         = 瞬时提示（网络 / 权限 / 5xx / 客户端校验 / 非业务兜底）
#   needs-code-first     = 分支未读取 code，无法判定（本批应为 0）
#
# 分支识别（Rev.8 要求）：从 message.error 行向上回溯到**分支头**
# （if / else if / else / catch），并沿同缩进的 if/else 链收集条件文本；
# **不使用固定 N 行窗口**（WINDOW_HEURISTIC=0），因此相邻分支不会被误归到本行。
#
# KEEP_ROWS 语义（Rev.8 D8 配套，写死在 artifact 里避免每轮争议）：
#   该页「读取服务端 code 的提交分支（submit：POST/PUT 形态调用）」条数，
#   Rev.7 逐行裁定为 keep:instant（不接常驻提示）。
#
# 只读：只 grep / awk 读取源码，不写业务文件、不改 DB。
set -uo pipefail

REPO="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "ERROR:not a git repository" >&2; exit 2; }
cd "$REPO" || exit 2

SRC_DIR="frontend-pc/src"
TARGETS=(
  "pages/admin/PermissionManage/index.jsx"
  "pages/admin/pricing/MerchantPricingConfig.jsx"
  "pages/admin/transit/TransitCenter.jsx"
  "pages/System/MembershipLevelsPage.jsx"
  "pages/MerchantManagement.jsx"
)

# 分支头：返回 "<line>\t<header text>"，找不到返回 "0\t(no-branch)"。
branch_header() { # file line
  awk -v target="$2" '
    { src[NR] = $0 }
    function indent(s,   _) { _ = 0; while (substr(s, _ + 1, 1) == " " || substr(s, _ + 1, 1) == "\t") _++; return _ }
    END {
      line = src[target]
      stripped = line; sub(/^[ \t]+/, "", stripped)
      p = index(stripped, "message.error(")
      prefix = p > 1 ? substr(stripped, 1, p - 1) : stripped
      if (prefix ~ /(^|[^A-Za-z0-9_])(if|else[ ]+if|else|catch)[ ]*(\(|\{)/) {
        gsub(/\t/, " ", prefix)
        printf "%d\t%s\n", target, prefix
        exit
      }
      ti = indent(line)
      hdr = 0
      for (i = target - 1; i >= 1; i--) {
        s = src[i]; st = s; sub(/^[ \t]+/, "", st)
        if (st ~ /^(\}[ ]*)?(else([ ]+if)?[ ]*(\(|\{)|if[ ]*\(|(\}[ ]*)?catch[ ]*\()/) {
          if (indent(s) <= ti) { hdr = i; break }
        }
      }
      if (hdr == 0) { printf "0\t(no-branch)\n"; exit }
      h = src[hdr]; gsub(/\t/, " ", h); sub(/^[ \t]+/, "", h); sub(/[ \t]+$/, "", h)
      printf "%d\t%s\n", hdr, h
    }
  ' "$1"
}

# if/else 链条件文本（同缩进的分支头；用于判定该行所在分支是否读取服务端 code）。
branch_chain() { # file header_line
  awk -v hdr="$2" '
    { src[NR] = $0 }
    function indent(s,   _) { _ = 0; while (substr(s, _ + 1, 1) == " " || substr(s, _ + 1, 1) == "\t") _++; return _ }
    END {
      if (hdr == 0) exit
      hi = indent(src[hdr])
      out = ""
      for (i = hdr; i >= 1 && hdr - i <= 40; i--) {
        s = src[i]; st = s; sub(/^[ \t]+/, "", st)
        if (i < hdr && indent(s) < hi) break
        if (i == hdr || (indent(s) == hi && st ~ /^(if[ ]*\(|else[ ]*(if|\{)|(\}[ ]*)?else[ ]*(if)?[ ]*(\(|\{)|(\}[ ]*)?catch[ ]*\()/)) {
          out = out " " st
        }
      }
      print out
    }
  ' "$1"
}

# 函数名（最近的 const/let/function 定义）。
enclosing_fn() { # file line
  awk -v target="$2" '
    NR <= target && (/^[[:space:]]*(const|let)[[:space:]]+[A-Za-z0-9_]+[[:space:]]*=[[:space:]]*(async[[:space:]]*)?(useCallback[[:space:]]*)?\(/ || /^[[:space:]]*function[[:space:]]+[A-Za-z0-9_]+/) {
      line = $0
      sub(/^[[:space:]]+/, "", line)
      if (line ~ /^function[[:space:]]+/) {
        sub(/^function[[:space:]]+/, "", line)
      } else {
        sub(/^(const|let)[[:space:]]+/, "", line)
        sub(/[[:space:]]*=.*/, "", line)
      }
      sub(/[^A-Za-z0-9_].*/, "", line)
      if (line != "") { found = line; foundline = NR }
    }
    END { printf "%s\t%d\n", (found == "" ? "unknown" : found), (foundline + 0) }
  ' "$1"
}

RUBRIC_RE='(40002|40004|40005|40900)'
MUTATION_RE='(api\.(post|put)\(|\.post\(|\.put\(|adminApi\.(create|update|set)[A-Za-z]*\(|method\()'
READ_RE='(api\.get\(|\.list\(|\.get\(|\.download\()'

rows=0
keep=0
fallback=0
residual=0
unclassified=0
page_keep=""
ROW_FILE="$(mktemp "$REPO/tmp/scan-2489.XXXXXX")" || exit 2
trap 'rm -f "$ROW_FILE"' EXIT

for rel in "${TARGETS[@]}"; do
  file="$SRC_DIR/$rel"
  if [ ! -f "$file" ]; then
    echo "ERROR: target missing: $rel" >&2
    exit 2
  fi
  page_keep_n=0
  while IFS= read -r hit; do
    line="${hit%%:*}"
    [ -n "$line" ] || continue
    fn_info="$(enclosing_fn "$file" "$line")"
    fn="${fn_info%%$'\t'*}"
    fn_line="${fn_info##*$'\t'}"
    hdr_info="$(branch_header "$file" "$line")"
    hdr_line="${hdr_info%%$'\t'*}"
    hdr_text="${hdr_info#*$'\t'}"
    chain="$(branch_chain "$file" "$hdr_line")"
    chain_flat="$(printf '%s' "$chain" | tr '\t' ' ' | tr -s ' ')"
    line_text="$(sed -n "${line}p" "$file")"

    reads_code=0
    if printf '%s' "$chain_flat" | grep -qE '(^|[^A-Za-z0-9_])code([^A-Za-z0-9_]|$)|isBusinessCorrectableConflict'; then
      reads_code=1
    fi

    # submit：函数起点到该 error 行之间出现 POST/PUT 形态调用（不是纯 get/delete 的读取/删除）。
    submit=0
    win_start="$fn_line"
    if [ "${win_start:-0}" -lt 1 ]; then win_start=$((line - 30)); [ "$win_start" -lt 1 ] && win_start=1; fi
    window="$(sed -n "${win_start},${line}p" "$file")"
    if printf '%s' "$window" | grep -qE "$MUTATION_RE"; then
      submit=1
    fi
    if [ "$reads_code" = "0" ] && [ "$submit" = "1" ] && printf '%s' "$window" | grep -qE "$READ_RE"; then
      : # 读取 + 提交混合的函数（如 load+save），以是否出现 POST/PUT 形态调用为准
    fi

    rubric_in_chain=0
    if printf '%s' "$chain_flat" | grep -q 'isBusinessCorrectableConflict'; then
      rubric_in_chain=1
    fi
    rubric_codes_toasted=0
    if printf '%s' "$chain_flat" | grep -qE "$RUBRIC_RE" && [ "$rubric_in_chain" = "0" ]; then
      rubric_codes_toasted=1
    fi

    if [ "$rel" = "pages/MerchantManagement.jsx" ] && printf '%s' "$fn" | grep -qE '^handleDelete$'; then
      # Rev.9 D9（Owner 裁定）：删除是不可纠正前置条件（商户还有网点/未完成订单），
      # 列表视图不做常驻提示 → 30 秒 toast。reason 按真实后端契约写。
      tier="keep:instant"
      reason="delete conflict: 40400 not found / 40002 active sites or unfinished orders → list toast 30s (Rev.9 D9 exception)"
    elif [ "$rubric_in_chain" = "1" ]; then
      tier="keep:instant"
      reason="rubric-fallback: non-business fallback of a code-classified submit branch"
      fallback=$((fallback + 1))
    elif [ "$rubric_codes_toasted" = "1" ]; then
      tier="residual:business-toast"
      reason="rubric code compared in-branch but still rendered by a 3s toast"
      residual=$((residual + 1))
    elif [ "$reads_code" = "1" ] && [ "$submit" = "1" ]; then
      tier="keep:instant"
      reason="code-read submit branch; measured error set has no business-correctable code (R2/R3/R5 keep)"
      keep=$((keep + 1))
      page_keep_n=$((page_keep_n + 1))
    elif [ "$reads_code" = "1" ]; then
      tier="keep:instant"
      reason="code-read load/delete branch; no user-editable input to correct"
    elif printf '%s %s' "$chain_flat" "$line_text" | grep -qE '(不能为空|需要大于|必须在|必填|errorFields)'; then
      tier="keep:instant"
      reason="client-side validation feedback"
    elif [ "$hdr_line" != "0" ] && printf '%s' "$chain_flat" | grep -q 'catch'; then
      tier="keep:instant"
      reason="catch-all fallback (network / unknown / permission)"
    elif printf '%s' "$chain_flat" | grep -qE '(加载|获取|删除)'; then
      tier="keep:instant"
      reason="load / delete feedback"
    else
      tier="needs-code-first"
      reason="branch does not read a server code; per-page judgement required"
      unclassified=$((unclassified + 1))
    fi

    rows=$((rows + 1))
    printf '%s:%s\tfn=%s\tbranch=%s\treads_code=%s\tsubmit=%s\ttier=%s\treason=%s\n' \
      "$rel" "$line" "${fn:-unknown}" "${hdr_text:-(no-branch)}" "$reads_code" "$submit" "$tier" "$reason" \
      >> "$ROW_FILE"
  done < <(grep -n 'message\.error(' "$file")
  page_keep="${page_keep}PAGE_KEEP_ROWS=$rel=$page_keep_n"$'\n'
done

echo "SCAN_TARGETS=${#TARGETS[@]}"
echo "READONLY=1"
echo "DB_WRITES=0"
echo "WINDOW_HEURISTIC=0"
echo "KEEP_ROWS_SEMANTICS=code-reading POST/PUT submit branches judged keep (Rev.8 D8 companion)"
echo "SCAN_ROWS=$rows"
echo "KEEP_ROWS=$keep"
echo "BUSINESS_FALLBACK_ROWS=$fallback"
echo "RESIDUAL_ROWS=$residual"
echo "UNCLASSIFIED=$unclassified"
printf '%s' "$page_keep"
echo "--- rows ---"
cat "$ROW_FILE"
