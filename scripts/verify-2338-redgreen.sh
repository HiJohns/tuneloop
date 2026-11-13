#!/usr/bin/env bash
# #2338 R4: every product-defect fix needs a pre-fix FAIL record and a post-fix PASS.
# RED   = baseline isolated run (docs/evidence/2336/R3-baseline-pre-existing.txt, 3e50bc494)
# GREEN = current Group A evidence (docs/evidence/2338/R2-group-a.txt)
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
OUT=docs/evidence/2338/R4-redgreen.txt
{
  echo "PROVENANCE $PROVENANCE"
  echo "RED   docs/evidence/2336/R3-baseline-pre-existing.txt (baseline 3e50bc494)"
  echo "GREEN docs/evidence/2338/R2-group-a.txt (fix ad8d12a2d)"
  python3 - <<'PY'
import json, re, sys
triage = json.load(open("docs/evidence/2338/R1-triage.json"))
red_text = open("docs/evidence/2336/R3-baseline-pre-existing.txt", errors="replace").read()
green_text = open("docs/evidence/2338/R2-group-a.txt", errors="replace").read()
red = set(re.findall(r"^--- FAIL: ([^ ]+)", red_text, re.M))
green = set(re.findall(r"^--- PASS: ([^ ]+)", green_text, re.M))
missing_red, missing_green = [], []
rows = []
for r in triage["rows"]:
    if r["class"] != "product-defect":
        continue
    name = r["test"]
    if name not in red:
        missing_red.append(name)
    if name not in green:
        missing_green.append(name)
    rows.append({"test": name, "red": name in red, "green": name in green})
print(f"product-defect={len(rows)} red={sum(1 for x in rows if x['red'])} green={sum(1 for x in rows if x['green'])}")
if missing_red:
    print("missing RED:", missing_red)
if missing_green:
    print("missing GREEN:", missing_green)
sys.exit(0 if not missing_red and not missing_green else 1)
PY
} 2>&1 | tee "$OUT"
grep -q "product-defect=" "$OUT" || { echo "R4 FAIL: no product-defect rows" >&2; exit 1; }
echo "R4 PASS: red/green pairs complete"
