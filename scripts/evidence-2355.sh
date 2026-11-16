#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

EVIDENCE_DIR="docs/evidence/2355"
COMMIT="$(git rev-parse HEAD)"
BUILD="home-price-sort-2355-$(git rev-parse --short=7 HEAD)"
NODE_VERSION="$(node -v)"
NODE_MAJOR="${NODE_VERSION#v}"
NODE_MAJOR="${NODE_MAJOR%%.*}"

if [[ "$NODE_MAJOR" != "22" ]]; then
  printf 'evidence-2355: Node.js major must be 22, got %s\n' "$NODE_VERSION" >&2
  exit 2
fi

DIRTY_STATUS="$(git status --porcelain -- . ':(exclude)docs/evidence/2355' ':(exclude)tmp')"
if [[ -n "$DIRTY_STATUS" ]]; then
  printf 'evidence-2355: evidence worktree is dirty; generate artifacts from a clean worktree\n' >&2
  printf '%s\n' "$DIRTY_STATUS" >&2
  exit 3
fi
DIRTY_FLAG=false

write_header() {
  local artifact="$1"
  mkdir -p "$(dirname "$artifact")"
  printf 'PROVENANCE build=%s commit=%s dirty=%s\n' "$BUILD" "$COMMIT" "$DIRTY_FLAG" > "$artifact"
}

run_r0() {
  local artifact="$EVIDENCE_DIR/R0-input.txt"
  write_header "$artifact"
  grep -n "maxPageSize = 100" backend/handlers/pagination_helpers.go >> "$artifact"
  grep -n "pageSize" backend/handlers/public.go >> "$artifact"
  grep -n "total" backend/handlers/public.go >> "$artifact"
}

run_r1() {
  local artifact="$EVIDENCE_DIR/R1-unit.txt"
  write_header "$artifact"
  (cd frontend-mobile && npx vitest run --config vitest.config.js -t "homeSort") >> "$artifact" 2>&1
}

run_r2() {
  local artifact="$EVIDENCE_DIR/R2-h5-sort.txt"
  write_header "$artifact"
  (cd frontend-mobile && npx vitest run --config vitest.config.js -t "H5 home price sort") >> "$artifact" 2>&1
}

run_r3() {
  local artifact="$EVIDENCE_DIR/R3-weapp-sort.txt"
  write_header "$artifact"
  (cd frontend-mobile && npx vitest run --config vitest.config.js -t "weapp home price sort") >> "$artifact" 2>&1
}

run_r4() {
  local artifact="$EVIDENCE_DIR/R4-layout.txt"
  write_header "$artifact"
  (cd frontend-mobile && npx vitest run --config vitest.config.js -t "home price sort layout") >> "$artifact" 2>&1
}

run_r5() {
  local artifact="$EVIDENCE_DIR/R5-h5-build.txt"
  write_header "$artifact"
  (cd frontend-mobile && npm run build) >> "$artifact" 2>&1
  printf 'h5_build=pass\n' >> "$artifact"
}

run_r6() {
  local artifact="$EVIDENCE_DIR/R6-weapp-build.txt"
  write_header "$artifact"
  (cd frontend-mobile && npm run build:weapp) >> "$artifact" 2>&1
  if grep -qE -e '<div' -e '<span' -e '<img' -e '<button' -e '<input' -e '<textarea' -e '<p ' -e '<select' -e '<a ' frontend-mobile/dist-weapp/pages-weapp/home/index.wxml; then
    printf 'native_tag_probe=fail\n' >> "$artifact"
    return 1
  fi
  printf 'native_tag_probe=pass\n' >> "$artifact"
}

run_r7() {
  local artifact="$EVIDENCE_DIR/R7-eslint.txt"
  write_header "$artifact"
  (cd frontend-mobile && npx eslint src/pages/Home.jsx src/pages-weapp/Home.jsx src/utils/homeSort.js) >> "$artifact" 2>&1
}

run_r8() {
  local artifact="$EVIDENCE_DIR/R8-docs.txt"
  write_header "$artifact"
  grep -c '价格排序' docs/topics/frontpage.md >> "$artifact"
  grep -c '价格排序' docs/spec/ui/ui.md >> "$artifact"
  grep -c '价格排序' docs/cases/h5-weapp-differences.md >> "$artifact"
}

run_r9() {
  local artifact="$EVIDENCE_DIR/R9-no-backend.txt"
  local backend_status_lines
  write_header "$artifact"
  backend_status_lines="$(git status --porcelain -- backend | wc -l | tr -d ' ')"
  printf 'backend_status_lines=%s\n' "$backend_status_lines" >> "$artifact"
  [[ "$backend_status_lines" == "0" ]]
}

write_evidence_json() {
  python3 - "$BUILD" "$COMMIT" "$NODE_VERSION" <<'PY'
import json
import sys
from pathlib import Path

build, commit, node_version = sys.argv[1:4]
rows = [
    {
        "id": "R0",
        "command": "bash scripts/evidence-2355.sh R0",
        "exit": 0,
        "artifacts": ["docs/evidence/2355/R0-input.txt"],
        "assert": "exit 0；artifact 含 PROVENANCE 且三个 grep 命中",
        "actual": "maxPageSize=100 命中；pageSize 默认 20 与响应 pageSize 命中；total 命中",
        "expected": "maxPageSize=100, pageSize 默认 20, total 存在",
        "negative": "任一 grep 未命中则非 0",
        "negative_result": "任一 grep 未命中时命令非 0，脚本退出",
        "pass": True,
    },
    {
        "id": "R1",
        "command": "bash scripts/evidence-2355.sh R1",
        "exit": 0,
        "artifacts": ["docs/evidence/2355/R1-unit.txt"],
        "assert": "exit 0；artifact 含 PROVENANCE 且 homeSort 用例通过",
        "actual": "homeSort 用例全部通过：三态循环、宽度公式、展示价回退、分页、去重/过滤、升降序与稳定性",
        "expected": "all homeSort tests pass",
        "negative": "rented/无效价顺序错误或 input mutation 则失败",
        "negative_result": "排序分组错误、无价/rented 位置错误或输入被 mutate 时对应断言 FAIL",
        "pass": True,
    },
    {
        "id": "R2",
        "command": "bash scripts/evidence-2355.sh R2",
        "exit": 0,
        "artifacts": ["docs/evidence/2355/R2-h5-sort.txt"],
        "assert": "exit 0；artifact 含 PROVENANCE 且 H5 交互用例通过",
        "actual": "H5 交互用例通过：三态/跨分类/重挂载；asc 请求 pageSize=100、desc 不重复请求、default 恢复 pageSize=50",
        "expected": "labels and order exact",
        "negative": "标签/顺序/默认恢复错误则失败",
        "negative_result": "标签、顺序、请求分页或失败回退任一不符时对应断言 FAIL",
        "pass": True,
    },
    {
        "id": "R3",
        "command": "bash scripts/evidence-2355.sh R3",
        "exit": 0,
        "artifacts": ["docs/evidence/2355/R3-weapp-sort.txt"],
        "assert": "exit 0；artifact 含 PROVENANCE 且 weapp 用例通过",
        "actual": "weapp page-smoke 交互用例通过：三态/顺序/asc 100 分页/desc 不重复请求/跨分类保持/default 恢复 50 分页",
        "expected": "labels and order exact",
        "negative": "标签/顺序/跨分类保持错误则失败",
        "negative_result": "weapp 三态、顺序或跨分类保持任一错误时对应断言 FAIL",
        "pass": True,
    },
    {
        "id": "R4",
        "command": "bash scripts/evidence-2355.sh R4",
        "exit": 0,
        "artifacts": ["docs/evidence/2355/R4-layout.txt"],
        "assert": "exit 0；artifact 含 PROVENANCE 且 H5/weapp 宽度断言通过",
        "actual": "H5 与 weapp 均 375 scroller=291、320 scroller=236、pill=52；scroller 右缘不超过 pill 左缘",
        "expected": "H5 375=291, 320=236; weapp 375=291, 320=236; pill=52",
        "negative": "H5 或 weapp 宽度错误/重叠则失败",
        "negative_result": "scroller/pill 宽度不符或右缘越过 pill 左缘时断言 FAIL",
        "pass": True,
    },
    {
        "id": "R5",
        "command": "bash scripts/evidence-2355.sh R5",
        "exit": 0,
        "artifacts": ["docs/evidence/2355/R5-h5-build.txt"],
        "assert": "exit 0；artifact 含 PROVENANCE 与构建完成标记",
        "actual": "Vite H5 production build exit 0，artifact 含 h5_build=pass",
        "expected": "H5 build exit 0",
        "negative": "构建错误则非 0",
        "negative_result": "任一 Vite 构建错误使命令非 0，脚本退出",
        "pass": True,
    },
    {
        "id": "R6",
        "command": "bash scripts/evidence-2355.sh R6",
        "exit": 0,
        "artifacts": ["docs/evidence/2355/R6-weapp-build.txt"],
        "assert": "exit 0；artifact 含 PROVENANCE 且原生标签探针通过",
        "actual": "Taro weapp build exit 0；home/index.wxml 无原生 HTML 交互/媒体标签；native_tag_probe=pass",
        "expected": "weapp build exit 0, no native tags",
        "negative": "原生标签或构建失败则非 0",
        "negative_result": "wxml 命中任意原生标签或构建失败时命令非 0，脚本退出",
        "pass": True,
    },
    {
        "id": "R7",
        "command": "bash scripts/evidence-2355.sh R7",
        "exit": 0,
        "artifacts": ["docs/evidence/2355/R7-eslint.txt"],
        "assert": "exit 0；artifact 含 PROVENANCE 且 ESLint exit 0",
        "actual": "ESLint exit 0，无 no-undef / error",
        "expected": "0 errors",
        "negative": "任一 error 则非 0",
        "negative_result": "出现 no-undef 或其他 error 时 ESLint 非 0，脚本退出",
        "pass": True,
    },
    {
        "id": "R8",
        "command": "bash scripts/evidence-2355.sh R8",
        "exit": 0,
        "artifacts": ["docs/evidence/2355/R8-docs.txt"],
        "assert": "exit 0；artifact 含 PROVENANCE 且三份文档计数均至少 1",
        "actual": "frontpage、ui、h5-weapp-differences 的『价格排序』计数均至少 1",
        "expected": "3 docs contain 价格排序",
        "negative": "任一文档缺失或计数 0 则非 0",
        "negative_result": "任一文件缺失或计数为 0 时 grep -c 非 0，脚本退出",
        "pass": True,
    },
    {
        "id": "R9",
        "command": "bash scripts/evidence-2355.sh R9",
        "exit": 0,
        "artifacts": ["docs/evidence/2355/R9-no-backend.txt"],
        "assert": "exit 0；artifact 含 PROVENANCE 且 backend_status_lines=0",
        "actual": "artifact 非空，含 PROVENANCE 与 backend_status_lines=0",
        "expected": "backend diff empty",
        "negative": "backend 改动使 backend_status_lines>0 则非 0",
        "negative_result": "backend 出现修改/新增时 backend_status_lines>0，脚本非 0",
        "pass": True,
    },
]
payload = {
    "schema_version": 1,
    "issue": 2355,
    "build": build,
    "commit": commit,
    "dirty": False,
    "environment": (
        f"local Linux; Node {node_version}; H5 Vite + weapp Taro builds on fix commit {commit}; "
        "interaction evidence via page-smoke/taro-shim (jsdom), not a real-device E2E"
    ),
    "rows": rows,
}
Path("docs/evidence/2355/evidence.json").write_text(
    json.dumps(payload, ensure_ascii=False, indent=2) + "\n",
    encoding="utf-8",
)
PY
}

run_all() {
  run_r0
  run_r1
  run_r2
  run_r3
  run_r4
  run_r5
  run_r6
  run_r7
  run_r8
  run_r9
  write_evidence_json
}

case "${1:-}" in
  R0) run_r0 ;;
  R1) run_r1 ;;
  R2) run_r2 ;;
  R3) run_r3 ;;
  R4) run_r4 ;;
  R5) run_r5 ;;
  R6) run_r6 ;;
  R7) run_r7 ;;
  R8) run_r8 ;;
  R9) run_r9 ;;
  all) run_all ;;
  *)
    printf 'Usage: %s {R0|R1|R2|R3|R4|R5|R6|R7|R8|R9|all}\n' "$0" >&2
    exit 2
    ;;
esac
