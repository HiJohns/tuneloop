#!/usr/bin/env bash
# #2448 Phase D: documentation gate (Owner review required before code).
set -uo pipefail
MODE="${1:-}"
if [ "$MODE" != "d1-docs" ]; then echo "usage: $0 d1-docs" >&2; exit 2; fi
REPO="$(git rev-parse --show-toplevel)"; cd "$REPO"
OUT="${OUT_DIR:-$REPO/docs/evidence/2448}"; mkdir -p "$OUT"
ART="$OUT/D1-docs.txt"; BUILD="1.0.$(git rev-parse --short HEAD)"; COMMIT="$(git rev-parse HEAD)"
DIRTY=false; [ -n "$(git status --porcelain --untracked-files=no)" ] && DIRTY=true
{
  echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"
  fail=0
  for spec in \
    'docs/cases/user-management.md:网点管理员' \
    'docs/cases/user-management.md:商户管理员' \
    'docs/cases/user-management.md:pending_approval' \
    'docs/cases/user-management.md:transfer' \
    'docs/cases/user-management.md:multi_site' \
    'docs/cases/user-management.md:该账号未配置邮箱' \
    'docs/cases/organization.md:members/apply' \
    'docs/cases/organization.md:multi_site' \
    'docs/spec/ui/ui.md:仅' \
    'docs/spec/ui/ui.md:该账号未配置邮箱' \
    'docs/spec/api/api.md:403' \
    'docs/spec/api/api.md:pending_approval' \
    'docs/spec/api/api.md:multi_site' \
    'docs/cases/cases.md:网点管理员申请人员加入'; do
    f="${spec%%:*}"; key="${spec#*:}"
    if grep -Fq -- "$key" "$f"; then echo "OK $f :: $key"; else echo "MISS $f :: $key"; fail=1; fi
  done
  [ "$fail" = "0" ] && echo "DOCS_OK" || echo "DOCS_FAIL"
  exit "$fail"
} 2>&1 | tee "$ART"
exit "${PIPESTATUS[0]}"
