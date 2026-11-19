#!/usr/bin/env bash
# ============================================================
# release-prerelease.sh — 预生产一键发布（可在任意构建服务器执行）
#
# 作用：构建后端 + PC + H5 → 打包 → Seafile 上传 → 触发 cadenza 部署预生产
#       （即 `make release` 的可复用封装，含前置检查、日志、内存上限）
#
# 用法：
#   bash scripts/release-prerelease.sh                  # 前台
#   setsid nohup bash scripts/release-prerelease.sh >/dev/null 2>&1 &   # 后台/脱离会话
#   tail -f tmp/release-<VERSION>-*.log                 # 查看进度
#
# 前置条件（缺一会给出明确报错）：
#   - 在 tuneloop 仓库根目录执行（脚本会自动定位）
#   - VERSION 已 bump（高于线上/预生产当前版本）
#   - node 22（nvm 可用则自动切换）+ zip + go
#   - ~/scripts/upload_to_seafile.sh 存在（Seafile 上传）
#   - ssh cadenza 可用（触发 download.sh 部署）
#
# 注意：本脚本**只发布预生产**；生产提升需另行执行
#   ssh cadenza 'cd /opt/flow && ./release.sh <pkg>.zip'
# ============================================================
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

VERSION="$(tr -d '[:space:]' < VERSION)"
COMMIT="$(git rev-parse --short HEAD)"
BRANCH="$(git rev-parse --abbrev-ref HEAD)"
STAMP="$(date +%Y%m%d-%H%M%S)"
LOG="tmp/release-${VERSION}-${STAMP}.log"
mkdir -p tmp

say() { echo "$*" | tee -a "$LOG"; }

say "== release: VERSION=${VERSION} branch=${BRANCH} commit=${COMMIT} =="
say "== log: ${LOG} =="

fatal() { say "ERROR: $*"; exit 1; }

# ---- 前置检查（失败即退出，不产生半成品） ----
[ -f frontend-mobile/package.json ] || fatal "not a tuneloop repo root: $REPO_ROOT"
[ -f Makefile ] || fatal "Makefile missing"
command -v zip >/dev/null 2>&1 || fatal "zip not installed (apt install zip)"
command -v go >/dev/null 2>&1 || say "WARN: go not in PATH（如构建机用 /usr/local/go/bin，请先 export PATH）"
[ -x "$HOME/scripts/upload_to_seafile.sh" ] || say "WARN: ~/scripts/upload_to_seafile.sh 缺失 → Seafile 上传会失败"
if ! timeout 15 ssh -o BatchMode=yes -o ConnectTimeout=8 cadenza 'true' >/dev/null 2>&1; then
  say "WARN: ssh cadenza 不可用（免密/Host 配置检查）→ 最后一步部署触发会失败"
fi
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
  say "WARN: 工作树有未提交改动（继续发布，但产物含本地修改）"
fi

# ---- node 22 + 内存上限（避免 OOM 杀掉构建进程/宿主机） ----
if [ -s "$HOME/.nvm/nvm.sh" ]; then
  # shellcheck disable=SC1091
  source "$HOME/.nvm/nvm.sh"
  nvm use 22 >/dev/null 2>&1 || say "WARN: nvm use 22 失败（Taro v4 需 node 22）"
fi
export NODE_OPTIONS="${NODE_OPTIONS:---max-old-space-size=2048}"
say "== node: $(node -v 2>/dev/null || echo 'n/a') NODE_OPTIONS=${NODE_OPTIONS} =="

# ---- 构建 + 打包 + 上传 + 触发部署 ----
say "== make release =="
set +e
make release 2>&1 | tee -a "$LOG"
rc=${PIPESTATUS[0]}
set -e
say "EXIT:${rc}"
say "== package: releases/tuneloop-pre_${STAMP}_${COMMIT}.zip（若 rc=0 则应已部署到预生产） =="
exit "$rc"
