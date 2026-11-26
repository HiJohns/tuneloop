#!/usr/bin/env bash
# scripts/verify-2518.sh — #2518 收敛证据生成器（删除客户端管理；租户管理并入商户管理）。
#
# 用法：bash scripts/verify-2518.sh <r1|r2|r3|r4|r5>
#
# 每行把结果写入 docs/evidence/2518/R*-*.txt（首行为 PROVENANCE），并打印机器可
# 判定的断言键值；任一行断言不成立以非 0 退出。
#
# - r1/r2 静态核对 frontend-pc/src 是否仍有客户端/租户页面或路由引用；
# - r3 核对商户管理菜单入口（App.jsx menuConfig）与页面「新建」入口；
# - r4 核对 docs/spec/ui/ui.md §5 两页条目已删且含「租户即商户」说明；
# - r5 前端构建 + eslint（重命令经 host 锁 heavy-run.sh）。
set -uo pipefail

ROW="${1:-}"
REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT" || exit 2
ISSUE=2518
OUT_DIR="$REPO_ROOT/docs/evidence/$ISSUE"
PC_SRC="frontend-pc/src"
UI_DOC="docs/spec/ui/ui.md"
APP_JSX="$PC_SRC/App.jsx"
MERCHANT_PAGE="$PC_SRC/pages/MerchantManagement.jsx"
HEAVY_RUN="${OPENCODE_HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"

mkdir -p "$OUT_DIR" "$REPO_ROOT/tmp"

usage() { echo "usage: $0 <r1|r2|r3|r4|r5>" >&2; exit 2; }
[ -n "$ROW" ] || usage

NODE22="$(ls -d "$HOME"/.nvm/versions/node/v22*/bin 2>/dev/null | tail -1)"
[ -n "$NODE22" ] && export PATH="$NODE22:$PATH"

FIX_SHA="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
[ -n "$FIX_SHA" ] || FIX_SHA="$(git rev-parse HEAD)"
SHORT="$(git rev-parse --short=8 "$FIX_SHA")"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)docs/evidence/$ISSUE" ':(exclude)tmp')" ]; then
    DIRTY=true
fi
PROVENANCE="# build=1.0.$SHORT # commit=$FIX_SHA # dirty=$DIRTY"

run_heavy() {
    if [ -x "$HEAVY_RUN" ]; then
        "$HEAVY_RUN" -- "$@"
    else
        "$@"
    fi
}

finish() { # artifact-file exit-code body
    local name=$1 rc=$2 body=$3
    {
        printf '%s\n' "$PROVENANCE"
        printf '%s\n' "$body"
    } > "$OUT_DIR/$name"
    printf '%s\n' "$body"
    [ "$rc" = "0" ] || return 1
}

case "$ROW" in
r1)
    body="" rc=0
    hits="$(grep -rnE "ClientManagement|system/clients" "$PC_SRC" --include='*.js' --include='*.jsx' 2>/dev/null || true)"
    count=0
    [ -n "$hits" ] && count="$(printf '%s\n' "$hits" | wc -l | tr -d ' ')"
    file_exists=0
    [ -f "$PC_SRC/pages/ClientManagement.jsx" ] && file_exists=1
    body+="GREP_CMD=grep -rnE \"ClientManagement|system/clients\" $PC_SRC"$'\n'
    body+="CLIENT_REFS=$count"$'\n'
    body+="CLIENT_PAGE_FILE=$file_exists"$'\n'
    [ "$count" = "0" ] || rc=1
    [ "$file_exists" = "0" ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R1-clients-removed.txt "$rc" "$body"
    ;;
r2)
    body="" rc=0
    hits="$(grep -rnE "TenantManagement|system/tenants" "$PC_SRC" --include='*.js' --include='*.jsx' 2>/dev/null || true)"
    count=0
    [ -n "$hits" ] && count="$(printf '%s\n' "$hits" | wc -l | tr -d ' ')"
    file_exists=0
    [ -f "$PC_SRC/pages/TenantManagement.jsx" ] && file_exists=1
    body+="GREP_CMD=grep -rnE \"TenantManagement|system/tenants\" $PC_SRC"$'\n'
    body+="TENANT_REFS=$count"$'\n'
    body+="TENANT_PAGE_FILE=$file_exists"$'\n'
    [ "$count" = "0" ] || rc=1
    [ "$file_exists" = "0" ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R2-tenants-removed.txt "$rc" "$body"
    ;;
r3)
    body="" rc=0
    menu="$(grep -nE "key: '/merchants'.*sysPermBits" "$APP_JSX" 2>/dev/null || true)"
    menu_line="$(printf '%s\n' "$menu" | head -1 | cut -d: -f1)"
    page_exists=0
    [ -f "$MERCHANT_PAGE" ] && page_exists=1
    create_entry=0
    grep -qE "创建商户" "$MERCHANT_PAGE" 2>/dev/null && create_entry=1
    body+="MENU_ENTRY=${menu_line:-missing}"$'\n'
    body+="MENU_LINE=$menu"$'\n'
    body+="PAGE_EXISTS=$page_exists"$'\n'
    body+="CREATE_ENTRY=$create_entry"$'\n'
    [ -n "$menu_line" ] || rc=1
    [ "$page_exists" = "1" ] || rc=1
    [ "$create_entry" = "1" ] || rc=1
    body+="MERCHANTS_MENU=$([ -n "$menu_line" ] && echo 1 || echo 0)"$'\n'
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R3-merchants-kept.txt "$rc" "$body"
    ;;
r4)
    body="" rc=0
    removed=1
    grep -qF '客户端管理 (`/system/clients`)' "$UI_DOC" && removed=0
    grep -qF '租户管理 (`/system/tenants`)' "$UI_DOC" && removed=0
    alias=0
    grep -qF '租户即商户（#2518）' "$UI_DOC" && alias=1
    body+="DOC=$UI_DOC"$'\n'
    body+="DOC_REMOVED=$removed"$'\n'
    body+="TENANT_ALIAS=$alias"$'\n'
    body+="--- §5 系统管理 ---"$'\n'
    body+="$(grep -n -A14 '^\*\*5\. 系统管理\*\*' "$UI_DOC" || true)"$'\n'
    [ "$removed" = "1" ] || rc=1
    [ "$alias" = "1" ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R4-docs.txt "$rc" "$body"
    ;;
r5)
    body="" rc=0
    build_log="$REPO_ROOT/tmp/verify-${ISSUE}-r5-build.log"
    lint_log="$REPO_ROOT/tmp/verify-${ISSUE}-r5-lint.log"
    (
        cd "$REPO_ROOT/frontend-pc" || exit 2
        run_heavy npm run build
    ) > "$build_log" 2>&1
    build_exit=$?
    (
        cd "$REPO_ROOT/frontend-pc" || exit 2
        run_heavy npx eslint src/App.jsx src/services/api.js
    ) > "$lint_log" 2>&1
    lint_exit=$?
    no_undef="$(grep -cE 'no-undef' "$lint_log" 2>/dev/null || true)"
    body+="BUILD_CMD=cd frontend-pc && npm run build"$'\n'
    body+="BUILD_EXIT=$build_exit"$'\n'
    body+="ESLINT_CMD=cd frontend-pc && npx eslint src/App.jsx src/services/api.js"$'\n'
    body+="ESLINT_EXIT=$lint_exit"$'\n'
    body+="NO_UNDEF=${no_undef:-0}"$'\n'
    [ "$build_exit" = "0" ] || rc=1
    [ "$lint_exit" = "0" ] || rc=1
    [ "${no_undef:-0}" = "0" ] || rc=1
    [ "$rc" = "0" ] && body+="BUILD_OK"$'\n'
    [ "$rc" = "0" ] && body+="ESLINT_OK"$'\n'
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R5-build-lint.txt "$rc" "$body"
    ;;
*)
    usage
    ;;
esac
