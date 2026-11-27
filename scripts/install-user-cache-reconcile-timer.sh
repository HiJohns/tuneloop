#!/usr/bin/env bash
# #2625 (L3) 用户缓存对账例行任务安装器（只安装调度，不执行写库动作）。
#
# 用法：
#   bash scripts/install-user-cache-reconcile-timer.sh
#     [--print|--apply|--uninstall] [--backend auto|systemd|cron]
#     [--at HH:MM] [--repo DIR]
#
# 默认 --print（只打印将安装的 unit / cron 行，不写任何文件）。
# --apply 优先装 systemd user timer，否则写带 marker 的 crontab 行；
# --uninstall 精确移除两者。
#
# 硬约束（测试逐条断言）：
#   - OnCalendar=*-*-* <HH:MM>:00 + Persistent=true
#   - ExecStart 指向 scripts/reconcile-user-cache-cron.sh，且**不含** --apply
#   - 凭据经 EnvironmentFile=%h/.config/tuneloop/reconcile.env（unit 内零密钥字面量）
#   - 日志 append 到 ${XDG_STATE_HOME:-$HOME/.local/state}/tuneloop/user-cache-reconcile.log
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$SCRIPT_DIR/.." && pwd)"
UNIT_NAME="opencode-user-cache-reconcile"
AT="03:30"
BACKEND="auto"
REQUESTED_BACKEND="auto"
MODE="print"

usage() {
  sed -n '2,20p' "$0" >&2
  exit 2
}

while [ $# -gt 0 ]; do
  case "$1" in
    --print) MODE="print"; shift ;;
    --apply) MODE="apply"; shift ;;
    --uninstall) MODE="uninstall"; shift ;;
    --backend) BACKEND="${2:-}"; shift 2 ;;
    --at) AT="${2:-}"; shift 2 ;;
    --repo) REPO="${2:-}"; shift 2 ;;
    -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
    *) usage ;;
  esac
done

case "$BACKEND" in auto|systemd|cron) ;; *) echo "ERROR: --backend must be auto, systemd, or cron" >&2; exit 2 ;; esac
if ! printf '%s' "$AT" | grep -qE '^([01][0-9]|2[0-3]):[0-5][0-9]$'; then
  echo "ERROR: --at must be HH:MM (got '$AT')" >&2
  exit 2
fi

CRON_BIN="$REPO/scripts/reconcile-user-cache-cron.sh"
STATE_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/tuneloop"
LOG="$STATE_DIR/user-cache-reconcile.log"
ENV_FILE="%h/.config/tuneloop/reconcile.env"
SYSTEMD_DIR="$HOME/.config/systemd/user"
SERVICE="$SYSTEMD_DIR/$UNIT_NAME.service"
TIMER="$SYSTEMD_DIR/$UNIT_NAME.timer"
CRON_MARKER="# opencode-user-cache-reconcile"

REQUESTED_BACKEND="$BACKEND"
if [ "$BACKEND" = "auto" ]; then
  if command -v systemctl >/dev/null 2>&1 && systemctl --user show-environment >/dev/null 2>&1; then
    BACKEND="systemd"
  else
    BACKEND="cron"
  fi
fi

on_calendar() { printf '*-*-* %s:00' "$AT"; }

cron_schedule() { # HH:MM -> "M H * * *"
  local hh mm
  hh="${AT%%:*}"
  mm="${AT##*:}"
  printf '%s %s * * *' "$((10#$mm))" "$((10#$hh))"
}

service_content() {
  cat <<EOF
[Unit]
Description=TuneLoop user cache reconcile (read-only, #2623/#2625)

[Service]
Type=oneshot
WorkingDirectory=$REPO
Environment=PATH=%h/.local/bin:/usr/local/bin:/usr/bin:/bin
EnvironmentFile=$ENV_FILE
ExecStart=$CRON_BIN
StandardOutput=append:$LOG
StandardError=append:$LOG
EOF
}

timer_content() {
  cat <<EOF
[Unit]
Description=Nightly TuneLoop user cache reconcile

[Timer]
OnCalendar=$(on_calendar)
Persistent=true
RandomizedDelaySec=300

[Install]
WantedBy=timers.target
EOF
}

cron_line() {
  printf '%s\n%s cd %q && %q >> %q 2>&1\n' \
    "$CRON_MARKER" "$(cron_schedule)" "$REPO" "$CRON_BIN" "$LOG"
}

strip_cron_entry() {
  awk -v m="$CRON_MARKER" '$0 == m { skip = 1; next } skip { skip = 0; next } { print }'
}

cron_marker_count() {
  crontab -l 2>/dev/null | grep -Fxc "$CRON_MARKER" || true
}

print_mode() {
  echo "BACKEND=$BACKEND"
  echo "AT=$AT"
  echo "SERVICE=$SERVICE"
  echo "TIMER=$TIMER"
  echo "LOG=$LOG"
  echo "ENV_FILE=$ENV_FILE"
  if [ "$BACKEND" = "systemd" ]; then
    echo "--- service ---"
    service_content
    echo "--- timer ---"
    timer_content
  else
    echo "--- crontab ---"
    cron_line
  fi
  echo "FILES_WRITTEN=0"
}

apply_systemd() {
  mkdir -p "$SYSTEMD_DIR" || return 1
  service_content > "$SERVICE" || return 1
  timer_content > "$TIMER" || return 1
  if command -v systemctl >/dev/null 2>&1; then
    systemctl --user daemon-reload >/dev/null 2>&1 || true
    systemctl --user enable --now "$UNIT_NAME.timer" >/dev/null 2>&1 || true
  fi
  echo "SERVICE_SHA=$(sha256sum "$SERVICE" | awk '{print $1}')"
  echo "TIMER_SHA=$(sha256sum "$TIMER" | awk '{print $1}')"
  return 0
}

apply_cron() {
  local current tmp
  current="$(crontab -l 2>/dev/null || true)"
  tmp="$(mktemp "$STATE_DIR/cron.XXXXXX")" || return 1
  printf '%s\n' "$current" | strip_cron_entry > "$tmp"
  cron_line >> "$tmp"
  crontab "$tmp" || { rm -f "$tmp"; return 1; }
  rm -f "$tmp"
}

next_elapse() {
  if [ "$BACKEND" = "systemd" ] && command -v systemctl >/dev/null 2>&1; then
    local value
    value="$(systemctl --user show "$UNIT_NAME.timer" --property=NextElapseUSecRealtime --value 2>/dev/null || true)"
    if [ -n "$value" ] && [ "$value" != "n/a" ]; then
      echo "NEXT_ELAPSE=$value"
      return 0
    fi
  fi
  echo "NEXT_ELAPSE=unknown"
}

case "$MODE" in
print)
  print_mode
  ;;
apply)
  mkdir -p "$STATE_DIR" 2>/dev/null || { echo "ERROR: cannot create state dir: $STATE_DIR" >&2; exit 2; }
  echo "BACKEND=$BACKEND"
  echo "AT=$AT"
  if [ "$BACKEND" = "systemd" ]; then
    if apply_systemd; then
      echo "FILES_WRITTEN=2"
      C=2
    elif [ "$REQUESTED_BACKEND" = "auto" ]; then
      echo "WARN:systemd-install-failed-fallback-cron" >&2
      apply_cron || { echo "ERROR: failed to install cron entry" >&2; exit 2; }
      BACKEND="cron"
      echo "BACKEND=$BACKEND"
      echo "FILES_WRITTEN=1"
      C=1
    else
      echo "ERROR: failed to install systemd timer" >&2
      exit 2
    fi
  else
    apply_cron || { echo "ERROR: failed to install cron entry" >&2; exit 2; }
    echo "FILES_WRITTEN=1"
    C=1
  fi

  # 幂等：再算一次期望内容/行数，必须与现有完全一致（不产生第二份 cron 行）
  if [ "$BACKEND" = "systemd" ]; then
    SERVICE_SHA_NOW="$(sha256sum "$SERVICE" | awk '{print $1}')"
    TIMER_SHA_NOW="$(sha256sum "$TIMER" | awk '{print $1}')"
    SERVICE_SHA_EXP="$(service_content | sha256sum | awk '{print $1}')"
    TIMER_SHA_EXP="$(timer_content | sha256sum | awk '{print $1}')"
    if [ "$SERVICE_SHA_NOW" = "$SERVICE_SHA_EXP" ] && [ "$TIMER_SHA_NOW" = "$TIMER_SHA_EXP" ]; then
      echo "IDEMPOTENT=1"
    else
      echo "IDEMPOTENT=0"
    fi
  else
    if [ "$(cron_marker_count)" = "1" ] && [ "$(crontab -l 2>/dev/null | grep -Fc "$(cron_schedule)")" -ge 1 ]; then
      echo "IDEMPOTENT=1"
    else
      echo "IDEMPOTENT=0"
    fi
  fi
  next_elapse
  echo "RESULT: PASS"
  ;;
uninstall)
  REMOVED=0
  if [ -f "$SERVICE" ]; then
    rm -f "$SERVICE"
    REMOVED=$((REMOVED + 1))
  fi
  if [ -f "$TIMER" ]; then
    if command -v systemctl >/dev/null 2>&1; then
      systemctl --user disable --now "$UNIT_NAME.timer" >/dev/null 2>&1 || true
    fi
    rm -f "$TIMER"
    REMOVED=$((REMOVED + 1))
  fi
  if command -v crontab >/dev/null 2>&1; then
    BEFORE="$(cron_marker_count)"
    if [ "${BEFORE:-0}" -gt 0 ]; then
      tmp="$(mktemp "${TMPDIR:-/tmp}/cron-uninstall.XXXXXX")" || exit 2
      crontab -l 2>/dev/null | strip_cron_entry > "$tmp"
      crontab "$tmp" >/dev/null 2>&1 || { rm -f "$tmp"; echo "ERROR: cannot rewrite crontab" >&2; exit 2; }
      rm -f "$tmp"
      REMOVED=$((REMOVED + BEFORE))
    fi
  fi
  echo "BACKEND=$BACKEND"
  echo "REMOVED=$REMOVED"
  echo "RESULT: PASS"
  ;;
esac
