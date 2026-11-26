#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2485.sh — 证据生成器（v2 合同：R1–R6）
#
# 口径（#2485 v2 / #2480 D5 rubric）：
#   business-persisted   业务可纠正冲突（40002/40004/40005/40900）已改表单内持久提示
#   keep:instant         瞬时 toast 正确：加载/删除/网络/权限/客户端校验/Modal 引导/无业务码兜底
#   defer:tier2          业务可纠正码在后端导出、但 PC 传输层不可观测
#                        （后端 HTTP 400 → api 客户端 throw 丢码）；按 D3 登记理由并保持
#                        toast，解锁单 #2563（本批 2 行：分类新建/改名重名）
#   residual             业务可纠正码仍走 toast（缺陷，必须为 0）
#
# 关键约束（v2 D1）：判定表每一行引用业务码时必须写 `code_source=<handler:行>`，
# 且该行必须真的导出该码；引用不到 → 该行只能判 keep/defer，不得判 business-persisted。
#
# 用法：bash scripts/verify-2485.sh <r1..r6>
#   r1 docs/evidence/2485/R1-classify.txt  判定表（35 个 message.error 调用点）
#   r2 docs/evidence/2485/R2-codes.txt     码来源可追溯（code_source 校验）
#   r3 docs/evidence/2485/R3-converted.txt 已转换行 + code-first 断言
#   r4 docs/evidence/2485/R4-shared.txt    共享件复用度量
#   r5 docs/evidence/2485/R5-residual.txt  残留度量 + keep 理由完整性
#   r6 docs/evidence/2485/R6-build.txt     PC 构建 + eslint
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`
# =============================================================================
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4|r5|r6) ;;
  *)
    echo "usage: $0 <r1..r6>" >&2
    exit 2
    ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2485
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_REF:-}" ]; then
  FIX_REF="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
fi
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
DIRTY=false
[ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR")" ] && DIRTY=true
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
HEAVY_RUN="${HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"
LOCK_DIR="${HEAVY_LOCK_DIR:-/tmp/opencode/locks}"

# 本批 4 个文件（与 Accepted Plan「范围文件」逐字一致）
TARGETS=(
  "frontend-pc/src/components/SiteMemberManagement.jsx"
  "frontend-pc/src/components/MerchantMemberManagement.jsx"
  "frontend-pc/src/pages/DirectMemberManagement.jsx"
  "frontend-pc/src/pages/admin/category/List.jsx"
)
# 扫描基线：docs/evidence/2480/R11-scan.txt 中本批 4 文件的行数（#2480 Tier 1 产物）
BASELINE_FILE="docs/evidence/2480/R11-scan.txt"
if [ -f "$REPO/$BASELINE_FILE" ]; then
  BASELINE_ROWS="$(grep -cE '^frontend-pc/src/(components/(SiteMemberManagement|MerchantMemberManagement)\.jsx|pages/DirectMemberManagement\.jsx|pages/admin/category/List\.jsx):' "$REPO/$BASELINE_FILE" | tr -d ' ')"
else
  BASELINE_ROWS=33
fi

heavy() {
  if [ -x "$HEAVY_RUN" ]; then bash "$HEAVY_RUN" --dir "$LOCK_DIR" -- "$@"; else "$@"; fi
}

part_http_status() { # <handler file> <line> → StatusXxx (searched on line-2..line)
  local file="$1" line="$2" start st
  [ -f "$REPO/$file" ] || return 1
  start=$((line - 2)); [ "$start" -lt 1 ] && start=1
  st="$(sed -n "${start},${line}p" "$REPO/$file" | grep -oE 'http\.Status[A-Za-z]+' | tail -1)"
  [ -n "$st" ] || return 1
  printf '%s' "${st#http.}"
}

# PC api 客户端可观测性（services/api.js:331-352）：对 409 返回解析后的 body（保留 code），
# 对 404 覆写为 40400，对其余非 2xx 直接 throw → code 丢失。
status_observable() { [ "$1" = "StatusConflict" ] && echo 1 || echo 0; }

require_tokens() { # log tokens...
  local log="$1"; shift
  local missing="" tok
  for tok in "$@"; do
    grep -Fq -- "$tok" "$log" || missing="$missing [$tok]"
  done
  if [ -n "$missing" ]; then
    echo "ASSERT_FAIL: missing tokens:$missing" >&2
    return 1
  fi
}

# ---------------------------------------------------------------------------
# 判定表：file<TAB>anchor(message.error 行的完整文本)<TAB>verdict<TAB>code<TAB>code_source<TAB>reason
#
# anchor 必须是该行去掉前导空白后的完整文本；同一文件内相同 anchor 的多次出现共享
# 同一条判定（本批仅 catch 分支重复，结论一致）。
# code_source 指向后端真正导出该业务码的 handler 行（`:` 分隔，多行用 `+` 连接）。
# ---------------------------------------------------------------------------
TABLE_DATA="$(cat <<'TABLE'
frontend-pc/src/components/SiteMemberManagement.jsx	message.error('获取成员列表失败');	keep:instant	none	-	GET /sites/:id/members 列表加载失败：运输/加载反馈，非表单冲突
frontend-pc/src/components/SiteMemberManagement.jsx	message.error(resp.message || '更新角色失败');	keep:instant	none	-	UpdateMemberRole 仅导出 40001/40400/50000（site_member.go），无业务可纠正码 → 保持 toast
frontend-pc/src/components/SiteMemberManagement.jsx	} catch { message.error('更新角色失败') }	keep:instant	none	-	catch-all（网络/未知），非业务可纠正
frontend-pc/src/components/SiteMemberManagement.jsx	message.error('移除成员失败');	keep:instant	none	-	RemoveMember 仅导出 40400/50000，无业务可纠正码
frontend-pc/src/components/SiteMemberManagement.jsx	message.error(resp.message || '提交申请失败');	business-persisted	40900	backend/handlers/member_invite_2114.go:158	ApplySiteMembership 导出 40900（已有待处理申请）/40002 → 表单内持久提示，此处为已转换分支的非业务兜底 toast
frontend-pc/src/components/SiteMemberManagement.jsx	message.error('提交申请失败');	keep:instant	none	-	catch-all（网络/未知）
frontend-pc/src/components/SiteMemberManagement.jsx	message.error(response.message || '绑定失败');	keep:instant	none	-	AddMember（POST /sites/:id/members）仅导出 40001/40911/40400/20000/20100 → 持久提示分支不可达，保持 toast
frontend-pc/src/components/SiteMemberManagement.jsx	message.error('需填写邮箱或设置密码');	keep:instant	none	-	客户端校验（提交前拦下），非服务端业务冲突
frontend-pc/src/components/SiteMemberManagement.jsx	message.error(`添加成员失败：${data.bind_errors.map(e => e.error || '未知错误').join('；')}`);	keep:instant	none	-	批量 bind_errors（20000/20100 部分失败明细），非单一表单冲突
frontend-pc/src/components/SiteMemberManagement.jsx	message.error(response.message || '添加成员失败');	keep:instant	none	-	AddMember 无业务可纠正码 → 保持 toast
frontend-pc/src/components/SiteMemberManagement.jsx	} catch { message.error('操作失败') }	keep:instant	none	-	catch-all（网络/未知）
frontend-pc/src/components/MerchantMemberManagement.jsx	message.error('获取负责人列表失败');	keep:instant	none	-	GET /admin/merchants/:id/members 列表加载失败
frontend-pc/src/components/MerchantMemberManagement.jsx	message.error('移除负责人失败');	keep:instant	none	-	RemoveMember 仅导出 40400/40009/50000
frontend-pc/src/components/MerchantMemberManagement.jsx	message.error(invite.message || '发送邀请失败')	business-persisted	40900	backend/handlers/member_invite_2114.go:416	InviteToMerchant 导出 40900（已有待处理邀请）/40002 → 表单内持久提示，此处为已转换分支的非业务兜底 toast
frontend-pc/src/components/MerchantMemberManagement.jsx	message.error(response.message || '绑定失败');	keep:instant	none	-	MerchantMemberHandler.AddMember 仅导出 40001/40901/50000 → 持久提示分支不可达，保持 toast
frontend-pc/src/components/MerchantMemberManagement.jsx	message.error(`添加管理员失败：${data.bind_errors.map(e => e.error || '未知错误').join('；')}`);	keep:instant	none	-	批量 bind_errors 明细，非单一表单冲突
frontend-pc/src/components/MerchantMemberManagement.jsx	message.error('未知冲突，请刷新后重试');	keep:instant	none	-	/users/check 冲突确认流的 UI 兜底（Modal 引导），非业务可纠正码
frontend-pc/src/components/MerchantMemberManagement.jsx	message.error(response.message || '添加成员失败');	keep:instant	none	-	AddMember 无业务可纠正码 → 保持 toast
frontend-pc/src/components/MerchantMemberManagement.jsx	} catch { message.error('操作失败') }	keep:instant	none	-	catch-all（网络/未知）
frontend-pc/src/pages/DirectMemberManagement.jsx	message.error('加载直属员工失败: ' + (err.message || ''))	keep:instant	none	-	personnelApi.list 列表加载失败
frontend-pc/src/pages/DirectMemberManagement.jsx	message.error('未获取到商户上下文，无法邀请')	keep:instant	none	-	客户端前置守卫（无网络调用），非服务端业务冲突
frontend-pc/src/pages/DirectMemberManagement.jsx	message.error(resp.message || '邀请失败')	business-persisted	40900	backend/handlers/member_invite_2114.go:416+483	InviteToMerchant / InvitePlatformStaff 均导出 40900（已有待处理邀请）/40002 → 表单内持久提示，此处为非业务兜底 toast
frontend-pc/src/pages/DirectMemberManagement.jsx	message.error('邀请失败: ' + (err.message || ''))	keep:instant	none	-	catch-all（网络/未知）
frontend-pc/src/pages/DirectMemberManagement.jsx	message.error(resp.message || '创建失败')	business-persisted	40900	backend/handlers/user_staff.go:296	CreateUser 导出 40900（用户名已存在，user_staff.go:296）与 40002（邀请模式需邮箱，user_staff.go:421）→ 表单内持久提示，此处为非业务兜底 toast
frontend-pc/src/pages/DirectMemberManagement.jsx	message.error('创建失败: ' + (err.message || ''))	keep:instant	none	-	catch-all（网络/未知）
frontend-pc/src/pages/admin/category/List.jsx	message.error('加载分类失败: ' + err.message)	keep:instant	none	-	GET /categories 列表加载失败
frontend-pc/src/pages/admin/category/List.jsx	message.error('Failed to update sort: ' + err.message)	keep:instant	none	-	PUT /categories/sort（UpdateCategorySort）仅导出 40001/50000，无业务可纠正码
frontend-pc/src/pages/admin/category/List.jsx	message.error('更新失败: ' + err.message)	keep:instant	none	-	排序/可见性切换失败（transport），无业务可纠正码
frontend-pc/src/pages/admin/category/List.jsx	message.error('Failed to delete: ' + err.message)	keep:instant	none	-	DELETE /categories/:id 失败（transport/受限）
frontend-pc/src/pages/admin/category/List.jsx	message.error(updated.message || '更新失败')	defer:tier2	40002	backend/handlers/api.go:955	UpdateCategory 导出 40002（分类重名）但以 HTTP 400 返回；PC api 客户端对非 404/409 的非 2xx 直接 throw（code 丢失，services/api.js:331-352）→ 持久提示在本页不可达，按 D3 defer:tier2（解锁单 #2563）
frontend-pc/src/pages/admin/category/List.jsx	message.error(response.message || '创建失败')	defer:tier2	40002	backend/handlers/api.go:876	CreateCategory 导出 40002（分类重名）但以 HTTP 400 返回；同上，PC 传输层丢码 → 持久提示不可达，按 D3 defer:tier2（解锁单 #2563）
frontend-pc/src/pages/admin/category/List.jsx	message.error(error.message || 'Failed to submit')	keep:instant	none	-	catch-all（网络/未知；表单校验错误已在 errorFields 分支跳过）
TABLE
)"

field() { printf '%s' "$1" | awk -F'\t' -v n="$2" '{print $n}'; }

# verdict_lookup <file> <anchor> → "verdict<TAB>code<TAB>code_source<TAB>reason"
verdict_lookup() {
  local f="$1" a="$2"
  printf '%s\n' "$TABLE_DATA" | awk -F'\t' -v f="$f" -v a="$a" '
    $1 == f && $2 == a { print $3 "\t" $4 "\t" $5 "\t" $6; found = 1; exit }
    END { if (!found) exit 1 }'
}

# scan_sites → file:line<TAB>anchor  （message.error 调用点）
scan_sites() {
  local rel f ln txt
  for rel in "${TARGETS[@]}"; do
    f="$REPO/$rel"
    while IFS=: read -r ln _rest; do
      [ -n "${ln:-}" ] || continue
      txt="$(sed -n "${ln}p" "$f" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
      printf '%s:%s\t%s\n' "$rel" "$ln" "$txt"
    done < <(grep -n 'message\.error(' "$f")
  done
}

LOG="$REPO/tmp/verify-$ISSUE-${MODE}.log"
: > "$LOG"
trap 'rm -f "$LOG"' EXIT
RC=0

case "$MODE" in
  r1)
    NAME="R1-classify.txt"
    {
      printf '%s\n' "$PROV"
      echo "# 判定表：4 个文件的每个 message.error 调用点 → verdict / code / code_source / reason"
      SCAN_ROWS=0; CLASSIFIED=0; UNCLASSIFIED=0; KEEP=0; PERSISTED=0; DEFER=0; BAD=0
      while IFS=$'\t' read -r loc anchor; do
        [ -n "${loc:-}" ] || continue
        rel="${loc%:*}"
        SCAN_ROWS=$((SCAN_ROWS + 1))
        if row="$(verdict_lookup "$rel" "$anchor")"; then
          verdict="$(field "$row" 1)"
          case "$verdict" in
            keep:instant) KEEP=$((KEEP + 1)) ;;
            business-persisted) PERSISTED=$((PERSISTED + 1)) ;;
            defer:tier2) DEFER=$((DEFER + 1)) ;;
            *) BAD=$((BAD + 1)) ;;
          esac
          CLASSIFIED=$((CLASSIFIED + 1))
          printf '%s\tverdict=%s\tcode=%s\tcode_source=%s\treason=%s\n' \
            "$loc" "$verdict" "$(field "$row" 2)" "$(field "$row" 3)" "$(field "$row" 4)"
        else
          UNCLASSIFIED=$((UNCLASSIFIED + 1))
          printf '%s\tverdict=UNCLASSIFIED\tcode=none\tcode_source=-\treason=MISSING TABLE ROW\n' "$loc"
        fi
      done < <(scan_sites)
      printf 'SCAN_ROWS=%s\nCLASSIFIED_ROWS=%s\nUNCLASSIFIED=%s\n' "$SCAN_ROWS" "$CLASSIFIED" "$UNCLASSIFIED"
      printf 'KEEP_ROWS=%s\nPERSISTED_ROWS=%s\nDEFER_ROWS=%s\nINVALID_VERDICT=%s\n' \
        "$KEEP" "$PERSISTED" "$DEFER" "$BAD"
      printf 'BASELINE_ROWS=%s\n' "$BASELINE_ROWS"
      ge=0; [ "$SCAN_ROWS" -ge "$BASELINE_ROWS" ] && ge=1
      printf 'ROWS_GE_BASELINE=%s\n' "$ge"
    } > "$LOG" 2>&1
    grep -q '^UNCLASSIFIED=0$' "$LOG" || RC=1
    grep -q '^INVALID_VERDICT=0$' "$LOG" || RC=1
    grep -q '^ROWS_GE_BASELINE=1$' "$LOG" || RC=1
    require_tokens "$LOG" 'UNCLASSIFIED=0' 'ROWS_GE_BASELINE=1' 'INVALID_VERDICT=0' || RC=1
    ;;
  r2)
    NAME="R2-codes.txt"
    {
      printf '%s\n' "$PROV"
      echo "# 码来源可追溯（D1）：判定表每行引用的 code 必须在 code_source 指定的 handler 行真实导出"
      MISSING=0; UNREACH=0; CHECKED=0
      while IFS=$'\t' read -r rel anchor verdict code src reason; do
        [ "$code" = "none" ] && continue
        CHECKED=$((CHECKED + 1))
        if [ -z "$src" ] || [ "$src" = "-" ]; then
          echo "CODE_SOURCE_MISSING=1 row=$rel code=$code"
          MISSING=$((MISSING + 1))
          continue
        fi
        ok=1
        prevfile=""
        IFS='+' read -ra parts <<< "$src"
        for part in "${parts[@]}"; do
          case "$part" in
            *:*) prevfile="${part%%:*}"; line="${part##*:}" ;;
            *) line="$part" ;;
          esac
          file="$prevfile"
          if [ ! -f "$REPO/$file" ]; then
            echo "UNREACHABLE row=$rel code=$code source=$part (file missing)"
            ok=0; break
          fi
          if ! sed -n "${line}p" "$REPO/$file" | grep -q "\"code\": *$code"; then
            echo "UNREACHABLE row=$rel code=$code source=$part (code not exported on that line)"
            ok=0; break
          fi
        done
        if [ "$ok" = "1" ]; then
          echo "TRACEABLE row=$rel:$(printf '%s' "$anchor" | cut -c1-40)... code=$code source=$src"
        else
          UNREACH=$((UNREACH + 1))
        fi
      done < <(printf '%s\n' "$TABLE_DATA")
      printf 'CODE_ROWS_CHECKED=%s\nCODE_SOURCE_MISSING=%s\nUNREACHABLE_CODES=%s\n' \
        "$CHECKED" "$MISSING" "$UNREACH"
      # R2b：引用的每个 code 都必须能在 backend/handlers 中 grep 到导出点
      GREP_MISSING=0
      for c in $(printf '%s\n' "$TABLE_DATA" | awk -F'\t' '$4 != "none" {print $4}' | sort -u); do
        n="$(grep -rl "\"code\": *$c" "$REPO/backend/handlers" --include='*.go' | grep -v '_test' | wc -l | tr -d ' ')"
        echo "BACKEND_HANDLERS_EXPORT code=$c files=$n"
        [ "$n" -ge 1 ] || GREP_MISSING=$((GREP_MISSING + 1))
      done
      printf 'BACKEND_EXPORT_MISSING=%s\n' "$GREP_MISSING"

      # ---- 传输层可达性（v3）：后端导出 ≠ 前端可观测 ----
      # 只看 handler 有没有该码是不够的：PC 客户端对非 404/409 的非 2xx 直接 throw，
      # code 在传输层丢失。因此每个 business-persisted 行的码必须由 HTTP 409 送达；
      # defer 行的码则必须不是 409（否则「不可达」的理由不成立）。
      echo "# 传输层可达性：business-persisted 的码必须由 HTTP 409 送达（services/api.js 保留 code）"
      CLIENT="frontend-pc/src/services/api.js"
      echo "CLIENT_409_RETURN=$(grep -c 'if (response.status === 409)' "$REPO/$CLIENT")"
      echo "CLIENT_404_RETURN=$(grep -c 'if (response.status === 404)' "$REPO/$CLIENT")"
      echo "CLIENT_OTHER_THROW=$(grep -c 'throw new Error(resolveErrorMessage' "$REPO/$CLIENT")"
      TRANSPORT_CHECKED=0; UNOBS_PERSISTED=0; OBS_DEFERRED=0; UNKNOWN_STATUS=0
      while IFS=$'\t' read -r rel anchor verdict code src reason; do
        [ "$code" = "none" ] && continue
        TRANSPORT_CHECKED=$((TRANSPORT_CHECKED + 1))
        all_obs=1; any_unknown=0; prevfile=""
        IFS='+' read -ra tparts <<< "$src"
        for part in "${tparts[@]}"; do
          case "$part" in
            *:*) prevfile="${part%%:*}"; tline="${part##*:}" ;;
            *) tline="$part" ;;
          esac
          st="$(part_http_status "$prevfile" "$tline")"
          if [ -z "$st" ]; then
            any_unknown=1
            echo "TRANSPORT_UNKNOWN row=$rel code=$code source=$prevfile:$tline (no http.Status on the cited line neighbourhood)"
            UNKNOWN_STATUS=$((UNKNOWN_STATUS + 1))
            all_obs=0
            continue
          fi
          obs="$(status_observable "$st")"
          echo "TRANSPORT row=$rel verdict=$verdict code=$code status=$st observable_by_client=$obs"
          [ "$obs" = "1" ] || all_obs=0
        done
        [ "$any_unknown" = "1" ] && continue
        if [ "$verdict" = "business-persisted" ] && [ "$all_obs" = "0" ]; then
          echo "UNREACHABLE_PERSISTED row=$rel code=$code (code never reaches the client → dead branch)"
          UNOBS_PERSISTED=$((UNOBS_PERSISTED + 1))
        fi
        if [ "$verdict" = "defer:tier2" ] && [ "$all_obs" = "1" ]; then
          echo "MISCLASSIFIED_DEFER row=$rel code=$code (code IS observable → must be converted, not deferred)"
          OBS_DEFERRED=$((OBS_DEFERRED + 1))
        fi
      done < <(printf '%s\n' "$TABLE_DATA")
      printf 'TRANSPORT_ROWS_CHECKED=%s\nTRANSPORT_UNOBSERVABLE_PERSISTED=%s\nTRANSPORT_OBSERVABLE_DEFERRED=%s\nTRANSPORT_UNKNOWN_STATUS=%s\n' \
        "$TRANSPORT_CHECKED" "$UNOBS_PERSISTED" "$OBS_DEFERRED" "$UNKNOWN_STATUS"
    } > "$LOG" 2>&1
    grep -q '^CODE_SOURCE_MISSING=0$' "$LOG" || RC=1
    grep -q '^UNREACHABLE_CODES=0$' "$LOG" || RC=1
    grep -q '^BACKEND_EXPORT_MISSING=0$' "$LOG" || RC=1
    grep -q '^TRANSPORT_UNOBSERVABLE_PERSISTED=0$' "$LOG" || RC=1
    grep -q '^TRANSPORT_OBSERVABLE_DEFERRED=0$' "$LOG" || RC=1
    grep -q '^TRANSPORT_UNKNOWN_STATUS=0$' "$LOG" || RC=1
    require_tokens "$LOG" 'CODE_SOURCE_MISSING=0' 'UNREACHABLE_CODES=0' 'BACKEND_EXPORT_MISSING=0' \
      'TRANSPORT_UNOBSERVABLE_PERSISTED=0' 'TRANSPORT_OBSERVABLE_DEFERRED=0' 'TRANSPORT_UNKNOWN_STATUS=0' \
      'CLIENT_409_RETURN=1' 'CLIENT_404_RETURN=1' 'CLIENT_OTHER_THROW=1' || RC=1
    ;;
  r3)
    NAME="R3-converted.txt"
    {
      printf '%s\n' "$PROV"
      echo "# 已转换行：business-persisted 必须在同一函数内先判 code，再落到持久 state（FormConflictAlert）"
      CONVERTED=0; CODE_FIRST_FAIL=0
      printf '%s\n' "$TABLE_DATA" | awk -F'\t' '$3 == "business-persisted" {print $1}' | sort -u | while read -r rel; do
        [ -n "$rel" ] || continue
        f="$REPO/$rel"
        pred=$(grep -c 'isBusinessCorrectableConflict(' "$f")
        setter=$(grep -cE 'set[A-Za-z]*Conflict\(conflictText\(' "$f")
        alert=$(grep -c '<FormConflictAlert' "$f")
        printf 'FILE=%s PREDICATE=%s PERSIST_SETTER=%s SHARED_ALERT=%s\n' "$rel" "$pred" "$setter" "$alert"
      done
      # code-first：每个持久 setter 必须由紧邻的共享判定触发（判定后 ≤3 行内落到 set*Conflict）
      for rel in "${TARGETS[@]}"; do
        f="$REPO/$rel"
        ok=$(awk '
          /isBusinessCorrectableConflict\(/ {pending = NR; seen++}
          pending && NR > pending && NR - pending <= 3 && /set[A-Za-z]*Conflict\(conflictText/ {matched++; pending = 0}
          END {print (seen > 0 && matched == seen) ? 1 : 0}' "$f")
        n=$(grep -c 'isBusinessCorrectableConflict(' "$f")
        [ "$n" = "0" ] && continue
        printf 'CODE_FIRST_FILE=%s pairs=%s ok=%s\n' "$rel" "$n" "$ok"
        [ "$ok" = "1" ] || CODE_FIRST_FAIL=$((CODE_FIRST_FAIL + 1))
      done
      CONVERTED=$(printf '%s\n' "$TABLE_DATA" | awk -F'\t' '$3 == "business-persisted"' | wc -l | tr -d ' ')
      printf 'CONVERTED_ROWS=%s\nCODE_FIRST=%s\n' "$CONVERTED" "$([ "$CODE_FIRST_FAIL" = "0" ] && echo 1 || echo 0)"
      ge=0; [ "$CONVERTED" -ge 1 ] && ge=1
      printf 'CONVERTED_GE_1=%s\n' "$ge"
    } > "$LOG" 2>&1
    grep -q '^CODE_FIRST=1$' "$LOG" || RC=1
    grep -q '^CONVERTED_GE_1=1$' "$LOG" || RC=1
    require_tokens "$LOG" 'CODE_FIRST=1' 'CONVERTED_GE_1=1' || RC=1
    ;;
  r4)
    NAME="R4-shared.txt"
    {
      printf '%s\n' "$PROV"
      echo "# 共享件复用（D2）：只用 components/FormConflictAlert，禁止本批自建冲突 Alert"
      REUSE=0; LOCAL_DUP=0; LOCAL_CODES=0
      for rel in "${TARGETS[@]}"; do
        f="$REPO/$rel"
        used=0
        if grep -qE "FormConflictAlert'" "$f" && grep -q '<FormConflictAlert' "$f"; then
          used=1; REUSE=$((REUSE + 1))
        fi
        # 自建冲突 Alert：<Alert ...> 邻域出现 Conflict 状态变量
        dup=$(awk '/<Alert/ {for (i = NR - 2; i <= NR + 4; i++) want[i] = 1}
                   NR in want && /Conflict/ {hit++}
                   END {print hit + 0}' "$f")
        # 本地维护业务码白名单（唯一口径必须在 utils/formConflict.js）
        codes=$(grep -cE 'BUSINESS_CORRECTABLE_CODES|\[[[:space:]]*40002' "$f")
        printf 'FILE=%s SHARED_ALERT=%s LOCAL_ALERT_DUP=%s LOCAL_CODE_LIST=%s\n' "$rel" "$used" "$dup" "$codes"
        LOCAL_DUP=$((LOCAL_DUP + dup))
        [ "$codes" != "0" ] && LOCAL_CODES=$((LOCAL_CODES + 1))
      done
      printf 'SHARED_REUSE=%s\nLOCAL_ALERT_DUP=%s\nLOCAL_CODE_LIST_FILES=%s\n' "$REUSE" "$LOCAL_DUP" "$LOCAL_CODES"
      ge=0; [ "$REUSE" -ge 1 ] && ge=1
      printf 'SHARED_REUSE_GE_1=%s\n' "$ge"
      # 每个 business-persisted 行所在文件都必须真的渲染共享件（否则转换点在 UI 上不可见）
      PERSISTED_FILES=0; PERSISTED_MISSING_ALERT=0
      while IFS= read -r rel; do
        [ -n "$rel" ] || continue
        PERSISTED_FILES=$((PERSISTED_FILES + 1))
        if ! grep -q '<FormConflictAlert' "$REPO/$rel"; then
          echo "PERSISTED_FILE_WITHOUT_ALERT=$rel"
          PERSISTED_MISSING_ALERT=$((PERSISTED_MISSING_ALERT + 1))
        fi
      done < <(printf '%s\n' "$TABLE_DATA" | awk -F'\t' '$3 == "business-persisted" {print $1}' | sort -u)
      printf 'PERSISTED_FILES=%s\nPERSISTED_FILES_MISSING_ALERT=%s\n' "$PERSISTED_FILES" "$PERSISTED_MISSING_ALERT"
    } > "$LOG" 2>&1
    grep -q '^LOCAL_ALERT_DUP=0$' "$LOG" || RC=1
    grep -q '^SHARED_REUSE_GE_1=1$' "$LOG" || RC=1
    grep -q '^LOCAL_CODE_LIST_FILES=0$' "$LOG" || RC=1
    grep -q '^PERSISTED_FILES_MISSING_ALERT=0$' "$LOG" || RC=1
    require_tokens "$LOG" 'LOCAL_ALERT_DUP=0' 'SHARED_REUSE_GE_1=1' 'LOCAL_CODE_LIST_FILES=0' \
      'PERSISTED_FILES_MISSING_ALERT=0' || RC=1
    ;;
  r5)
    NAME="R5-residual.txt"
    {
      printf '%s\n' "$PROV"
      echo "# 残留度量：业务可纠正码不得仍由 3 秒 toast 呈现；keep 行必须都有理由"
      RESIDUAL=0; KEEP=0; KEEP_LONG=0; DEFER=0; DEFER_LONG=0
      # 逐调用点统计（与 R1 同口径：同一 anchor 的多次出现分别计数）
      while IFS=$'\t' read -r loc anchor; do
        [ -n "${loc:-}" ] || continue
        rel="${loc%:*}"
        row="$(verdict_lookup "$rel" "$anchor")" || continue
        v="$(field "$row" 1)"
        if [ "$v" = "keep:instant" ]; then
          KEEP=$((KEEP + 1))
          [ -n "$(field "$row" 4)" ] && KEEP_LONG=$((KEEP_LONG + 1))
        fi
        if [ "$v" = "defer:tier2" ]; then
          DEFER=$((DEFER + 1))
          [ -n "$(field "$row" 4)" ] && DEFER_LONG=$((DEFER_LONG + 1))
        fi
      done < <(scan_sites)
      # 残留检测器（可被自测证伪）：message.error 前 10 行仍直接比较业务码
      residual_lines() { # file → stdout: file:line per hit
        local f="$1" ln start
        while IFS=: read -r ln _rest; do
          [ -n "${ln:-}" ] || continue
          start=$((ln - 10)); [ "$start" -lt 1 ] && start=1
          if sed -n "${start},${ln}p" "$f" | grep -qE 'code[[:space:]]*===[[:space:]]*(40002|40004|40005|40900)\b'; then
            printf '%s:%s\n' "$f" "$ln"
          fi
        done < <(grep -n 'message\.error(' "$f")
      }
      for rel in "${TARGETS[@]}"; do
        f="$REPO/$rel"
        while IFS= read -r hit; do
          [ -n "$hit" ] || continue
          echo "RESIDUAL_SITE=$hit (business-correctable code still rendered by toast)"
          RESIDUAL=$((RESIDUAL + 1))
        done < <(residual_lines "$f")
      done
      # 自测（可证伪性）：缺陷形态必须被同一检测器命中
      mkdir -p "$REPO/tmp"
      DEFECT="$REPO/tmp/residual-selftest-$ISSUE.jsx"
      cat > "$DEFECT" <<'SELFTEST'
function legacyConflict(resp) {
  if (resp.code === 40900) {
    message.error(resp.message || '该用户已有待处理的邀请')
  }
}
SELFTEST
      SELF_HITS="$(residual_lines "$DEFECT" | wc -l | tr -d ' ')"
      rm -f "$DEFECT"
      printf 'RESIDUAL_DETECTOR_SELF_TEST=%s\nRESIDUAL_SELF_TEST_HITS=%s\n' \
        "$([ "$SELF_HITS" -ge 1 ] && echo 1 || echo 0)" "$SELF_HITS"
      printf 'BUSINESS_TOAST_RESIDUAL=%s\nKEEP_ROWS=%s\nKEEP_WITH_REASON=%s\nDEFER_ROWS=%s\nDEFER_WITH_REASON=%s\n' \
        "$RESIDUAL" "$KEEP" "$KEEP_LONG" "$DEFER" "$DEFER_LONG"
    } > "$LOG" 2>&1
    grep -q '^BUSINESS_TOAST_RESIDUAL=0$' "$LOG" || RC=1
    grep -q '^RESIDUAL_DETECTOR_SELF_TEST=1$' "$LOG" || RC=1
    REASON_OUT="$(python3 - "$LOG" <<'PY'

import re, sys
text = open(sys.argv[1]).read()
keep = int(re.search(r'^KEEP_ROWS=(\d+)$', text, re.M).group(1))
with_reason = int(re.search(r'^KEEP_WITH_REASON=(\d+)$', text, re.M).group(1))
defer = int(re.search(r'^DEFER_ROWS=(\d+)$', text, re.M).group(1))
defer_reason = int(re.search(r'^DEFER_WITH_REASON=(\d+)$', text, re.M).group(1))
keep_ok = 1 if keep == with_reason else 0
defer_ok = 1 if defer == defer_reason else 0
print(f'KEEP_REASON_COMPLETE={keep_ok}')
print(f'DEFER_REASON_COMPLETE={defer_ok}')
sys.exit(0 if (keep_ok and defer_ok) else 1)
PY
)"
    printf '%s\n' "$REASON_OUT"
    printf '%s\n' "$REASON_OUT" >> "$LOG"
    printf '%s\n' "$REASON_OUT" | grep -q '^KEEP_REASON_COMPLETE=1$' || RC=1
    printf '%s\n' "$REASON_OUT" | grep -q '^DEFER_REASON_COMPLETE=1$' || RC=1
    require_tokens "$LOG" 'BUSINESS_TOAST_RESIDUAL=0' 'RESIDUAL_DETECTOR_SELF_TEST=1' \
      'KEEP_REASON_COMPLETE=1' 'DEFER_REASON_COMPLETE=1' || RC=1
    ;;
  r6)
    NAME="R6-build.txt"
    {
      printf '%s\n' "$PROV"
      echo "BUILD_CMD=cd frontend-pc && npm run build"
      ( cd "$REPO/frontend-pc" && heavy npm run build ) 2>&1 | tail -12
      BUILD=${PIPESTATUS[0]}
      echo "BUILD_EXIT=$BUILD"
      echo "ESLINT_CMD=cd frontend-pc && npx eslint <4 changed files>"
      ( cd "$REPO/frontend-pc" && heavy npx eslint \
          src/components/SiteMemberManagement.jsx \
          src/components/MerchantMemberManagement.jsx \
          src/pages/DirectMemberManagement.jsx \
          src/pages/admin/category/List.jsx ) 2>&1 | tail -12
      LINT=${PIPESTATUS[0]}
      echo "ESLINT_EXIT=$LINT"
      [ "$BUILD" = "0" ] && echo "BUILD_OK" || echo "BUILD_FAIL"
      [ "$LINT" = "0" ] && echo "ESLINT_OK" || echo "ESLINT_FAIL"
    } > "$LOG" 2>&1
    grep -q '^BUILD_OK$' "$LOG" || RC=1
    grep -q '^ESLINT_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'BUILD_OK' 'ESLINT_OK' || RC=1
    ;;
esac

ART="$OUT_DIR/$NAME"
cp "$LOG" "$ART"
echo "ARTIFACT=$ART"
if [ "$RC" != "0" ]; then
  echo "RESULT: FAIL ($MODE)" >&2
  exit 1
fi
echo "RESULT: PASS ($MODE)"
