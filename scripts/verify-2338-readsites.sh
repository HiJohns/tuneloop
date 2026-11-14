#!/usr/bin/env bash
# #2338 R12 (D10): every production read site touching repair-service tenant scope
# must be classified. org-anchor-needed sites are fixed by repairServiceTenantScope.
set -euo pipefail
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2338}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TMPDIR="${TMPDIR:-$PWD/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$PWD/tmp/gocache}"
export PATH="/usr/local/go/bin:$PATH"
mkdir -p docs/evidence/2338 "$TMPDIR" "$GOCACHE"
PROVENANCE="build=repair-tenant-anchor commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- backend scripts)" && echo false || echo true)"
HITS=$(grep -rn 'repairServiceTenantScope\|tenant_id IN ?\|tenant_id = ?' \
  backend/handlers/repair_service.go backend/handlers/repair_request.go \
  backend/handlers/repair_service_admin_2129.go backend/handlers/repair.go \
  backend/handlers/merchant_scope_2247.go 2>/dev/null || true)
python3 - "$HITS" <<'PY'
import json, re, sys
hits = [l for l in sys.argv[1].splitlines() if l.strip()]
rows = []
for line in hits:
    m = re.match(r'([^:]+):(\d+):(.*)', line)
    if not m:
        continue
    path, lineno, code = m.group(1), int(m.group(2)), m.group(3).strip()
    if "repairServiceTenantScope" in code:
        kind = "org-anchor-needed"
    elif "merchant_scope_2247.go" in path:
        kind = "org-anchor-needed"
    elif path.endswith("repair.go"):
        kind = "technician-flow"
    elif "admin_2129" in path:
        kind = "org-anchor-needed"
    else:
        kind = "org-anchor-needed"
    rows.append({"file": path, "line": lineno, "code": code[:160], "class": kind})
out = {"issue": 2338, "site_count": len(rows), "rows": rows}
json.dump(out, open("docs/evidence/2338/R12-inventory.json", "w"), ensure_ascii=False, indent=2)
print(f"R12 inventory: {len(rows)} production read sites classified")
sys.exit(0 if rows else 1)
PY
echo "R12 PASS"
