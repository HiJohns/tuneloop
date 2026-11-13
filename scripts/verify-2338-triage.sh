#!/usr/bin/env bash
# #2338 R1: triage mapping must cover all 22 Group A tests with a legal class.
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
python3 - docs/evidence/2338/R1-triage.json <<'PY'
import json, os, sys
path = sys.argv[1]
d = json.load(open(path))
rows = d.get("rows", [])
legal = {"product-defect", "stale-assertion", "order-dependent"}
errors = []
if len(rows) != 22:
    errors.append(f"row count {len(rows)} != 22")
for r in rows:
    if r.get("class") not in legal:
        errors.append(f"{r.get('test')}: illegal class {r.get('class')}")
    if r.get("class") == "stale-assertion" and not r.get("note"):
        errors.append(f"{r.get('test')}: stale-assertion without production-code evidence")
    if not r.get("test"):
        errors.append("row without test name")
summary = d.get("summary", {})
if summary.get("product_defect", 0) + summary.get("stale_assertion", 0) != 22:
    errors.append(f"summary split {summary} != 22")
if errors:
    print("R1 FAIL:", "; ".join(errors))
    sys.exit(1)
print(f"R1 PASS: 22/22 classified (product-defect={summary['product_defect']}, stale-assertion={summary['stale_assertion']})")
PY
