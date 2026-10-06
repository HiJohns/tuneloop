#!/usr/bin/env bash
# #2131 journal watchdog：FATAL/panic 扫描 + 磁盘/内存阈值 → SMTP 邮件
# cron: * * * * * /opt/monitoring/watchdog.sh（每分钟；内部自带节流）
set -u
DIR="$(cd "$(dirname "$0")" && pwd)"
. "$DIR/.env"
STATE="$DIR/.watchdog.state"
[ -f "$STATE" ] && . "$STATE" 2>/dev/null || LAST_FATAL_ALERT=""
NOW=$(date +%s)
UNIT="${WATCH_UNIT:-tuneloop-pre}"
HITS=$(journalctl -u "$UNIT" --since "2 min ago" --no-pager 2>/dev/null | grep -ciE "FATAL|panic" || true)
if [ "${HITS:-0}" -gt 0 ] && [ "${LAST_FATAL_ALERT:-0}" -lt $((NOW - 1800)) ]; then
  BODY=$(journalctl -u "$UNIT" --since "10 min ago" --no-pager 2>/dev/null | grep -iE "FATAL|panic" | tail -20)
  python3 - "$BODY" <<'PYEOF'
import sys, os, smtplib
from email.mime.text import MIMEText
from email.header import Header
body = sys.argv[1] or "(empty)"
msg = MIMEText("tuneloop-pre 检测到 FATAL/panic：\n\n" + body, "plain", "utf-8")
msg["Subject"] = Header("[CRITICAL] tuneloop-pre FATAL/panic", "utf-8")
msg["From"] = os.environ.get("SMTP_USER", "")
msg["To"] = os.environ.get("ALERT_TO", "linwx1978@gmail.com")
s = smtplib.SMTP_SSL(os.environ.get("SMTP_HOST", "smtp.qq.com"), int(os.environ.get("SMTP_PORT", "465")), timeout=20)
s.login(os.environ["SMTP_USER"], os.environ["SMTP_PASSWORD"])
s.sendmail(msg["From"], [msg["To"]], msg.as_string())
s.quit()
PYEOF
  echo "LAST_FATAL_ALERT=$NOW" > "$STATE"
fi
# 磁盘/内存阈值
USAGE=$(df / | awk 'NR==2 {print int($5)}')
MEM=$(free | awk '/Mem:/ {print int($3/$2*100)}')
if [ "${USAGE:-0}" -ge 85 ] && [ "${LAST_DISK_ALERT:-0}" -lt $((NOW - 86400)) ]; then
  python3 - "磁盘使用率 ${USAGE}%" <<'PYEOF'
import sys, os, smtplib
from email.mime.text import MIMEText
msg = MIMEText("服务器磁盘告警：" + (sys.argv[1] or ""), "plain", "utf-8")
msg["Subject"] = Header("[HIGH] 磁盘使用率", "utf-8"); msg["From"] = os.environ["SMTP_USER"]; msg["To"] = os.environ.get("ALERT_TO", "linwx1978@gmail.com")
s = smtplib.SMTP_SSL(os.environ.get("SMTP_HOST", "smtp.qq.com"), int(os.environ.get("SMTP_PORT", "465")), timeout=20)
s.login(os.environ["SMTP_USER"], os.environ["SMTP_PASSWORD"]); s.sendmail(msg["From"], [msg["To"]], msg.as_string()); s.quit()
PYEOF
  echo "LAST_DISK_ALERT=$NOW" >> "$STATE"
fi
