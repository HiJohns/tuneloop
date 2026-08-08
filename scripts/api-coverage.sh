#!/bin/bash
# api-coverage.sh — route coverage verification (Phase 4, #1583)
#
# Compares all routes registered in backend/main.go against API paths
# referenced in docs/cases/*.md YAML front-matter. Reports covered and
# uncovered routes. Exit code 1 when uncovered routes exist (usable as
# a CI gate).
#
# Usage:
#   bash scripts/api-coverage.sh [--verbose]

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MAIN_GO="$REPO_ROOT/backend/main.go"
CASES_DIR="$REPO_ROOT/docs/cases"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[0;33m'; NC='\033[0m'

# 1. Extract all routes from main.go: METHOD + path (path params → :id).
#    Routes are registered under r.Group("/api"), so prefix accordingly.
extract_routes() {
  grep -oE '(GET|POST|PUT|DELETE|PATCH)\("[^"]+"' "$MAIN_GO" \
    | sed -E 's/^([A-Z]+)\("([^"]+)"/\1 \2/' \
    | awk '{print $1, "/api" $2}'
}

# 2. Extract API paths referenced in cases YAML (api.path + ops paths).
#    Normalize: strip template placeholders (/api/path), ensure /api prefix.
extract_case_apis() {
  grep -hoE '(path: /)[^,}"]+' "$CASES_DIR"/*.md 2>/dev/null \
    | sed -E 's/^.*path: //; s/[,"}]$//' \
    | grep -vE '^/api/path$' \
    | sed -E 's|^/api||; s|^|/api|' \
    | sort -u
}

# 3. Normalize a path: /user/orders/:id → /user/orders/:id (keep as-is),
#    and strip query strings.
normalize() { echo "$1" | sed -E 's/\?.*$//'; }

echo "=== API 路由覆盖矩阵 ==="
ROUTES=$(extract_routes)
APIS=$(extract_case_apis)

COVERED=0
UNCOVERED=0
EXEMPT=0
UNCOVERED_LIST=()

# Routes that are intentionally outside the use-case scope (infra/auth/
# admin/system). These do not fail the gate.
is_exempt() {
  case "$1" in
    /api/health|/api/config*|/api/auth/*|/api/wx/*|/api/wechatpay/*|/api/setup/*|/api/iam/*|/api/wechat-bind/*|/api/admin/*|/api/merchant/*|/api/audit*|/api/users/*) return 0 ;;
    *) return 1 ;;
  esac
}

while IFS= read -r route; do
  [ -z "$route" ] && continue
  method=$(echo "$route" | awk '{print $1}')
  path=$(echo "$route" | awk '{print $2}')
  norm=$(normalize "$path")

  # Match: any case API with same path (method-agnostic for now, since
  # YAML may omit the method on pure-display GET steps).
  if echo "$APIS" | grep -qF "$norm"; then
    COVERED=$((COVERED + 1))
    if [ "${1:-}" = "--verbose" ]; then
      echo -e "  ${GREEN}✅ $method $path${NC}"
    fi
  elif is_exempt "$norm"; then
    EXEMPT=$((EXEMPT + 1))
  else
    UNCOVERED=$((UNCOVERED + 1))
    UNCOVERED_LIST+=("$method $path")
    echo -e "  ${YELLOW}⚠️  $method $path${NC}"
  fi
done <<< "$ROUTES"

TOTAL=$((COVERED + UNCOVERED))
PCT=0
if [ "$TOTAL" -gt 0 ]; then
  PCT=$((COVERED * 100 / TOTAL))
fi

echo ""
echo "=== 结果: $COVERED 已覆盖 / $EXEMPT 豁免 / $UNCOVERED 未覆盖 (业务覆盖 $PCT%) ==="
if [ "$UNCOVERED" -gt 0 ]; then
  echo "⚠️  存在 $UNCOVERED 条未覆盖业务路由（见上方清单）"
  exit 1
fi
echo "✅ 全部业务路由已被用例覆盖"
exit 0
