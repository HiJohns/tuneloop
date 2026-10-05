# 监控栈（#2131）
- `docker-compose.yml`：gatus（拨测/告警，config-as-code）+ beszel hub/agent（资源/容器）
- `gatus/config.yaml`：拨测与告警配置（SMTP 凭据经环境变量注入，不入 repo）
- `watchdog.sh`：journal FATAL/panic + 磁盘/内存阈值 → 邮件（cron 每分钟）

## 部署（cadenza）
1. `mkdir -p /opt/monitoring && scp monitoring/{docker-compose.yml,gatus/config.yaml,watchdog.sh}` 至 `/opt/monitoring/`（gatus 配置放 `gatus/` 子目录）
2. `/opt/monitoring/.env`：`SMTP_HOST/SMTP_PORT/SMTP_USER/SMTP_PASSWORD/ALERT_TO`（+ `BESZEL_AGENT_KEY` 注册后回填）
3. `cd /opt/monitoring && docker compose up -d`
4. watchdog 入 cron：`* * * * * /opt/monitoring/watchdog.sh`
5. UI（仅 SSH 隧道）：gatus http://127.0.0.1:8080；beszel http://127.0.0.1:8090
