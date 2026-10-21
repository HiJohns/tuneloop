#!/usr/bin/env bash
# tenant-audit-coverage.sh — #2246 覆盖率校验（只读，可复跑）
#
# 比对：
#   docs/ops/tenant-id-audit-files.csv（backend 层引用清单）
#   docs/ops/tenant-id-audit.md（逐模块审核结论 + 补充判定 + 测试同组清单）
#
# 判定标准：CSV 中每个 backend .go 文件必须以文件名出现在审核文档中。
# 输出：计数 + 未覆盖文件清单；有未覆盖 → exit 1。
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CSV="$ROOT/docs/ops/tenant-id-audit-files.csv"
DOC="$ROOT/docs/ops/tenant-id-audit.md"

if [ ! -f "$CSV" ] || [ ! -f "$DOC" ]; then
  echo "missing input: $CSV or $DOC" >&2
  exit 2
fi

python3 - "$CSV" "$DOC" <<'PY'
import csv
import sys

csv_path, doc_path = sys.argv[1], sys.argv[2]
doc = open(doc_path, encoding="utf-8").read()
rows = [
    r
    for r in csv.DictReader(open(csv_path, encoding="utf-8-sig"))
    if r["layer"] == "backend"
]
files = sorted({r["file"] for r in rows if r["file"].endswith(".go")})
missing = [f for f in files if f.rsplit("/", 1)[-1] not in doc]
covered = len(files) - len(missing)

print(f"backend files: {len(files)} | covered: {covered} | uncovered: {len(missing)}")
for f in missing:
    print(f"  UNCOVERED: {f}")

sys.exit(1 if missing else 0)
PY
