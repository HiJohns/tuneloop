# TuneLoop 预发布操作手册

> 版本: v1.0 | 最后更新: 2026-05-15 | 基于 Issue #545

## 一、架构概览

```
                       ┌──────────────────────────────────────┐
                       │          Nginx (HTTPS)               │
                       │  web.cadenzayueqi.com  →  :5558      │
                       │  wx.cadenzayueqi.com   →  :5559      │
                       │  iam.cadenzayueqi.com  →  :5560      │
                       └──────────┬───────────────────────────┘
                                  │
          ┌───────────────────────┼───────────────────────┐
          │                       │                       │
     ┌────▼────┐            ┌────▼────┐            ┌─────▼─────┐
     │ Tuneloop │            │ Tuneloop │            │ BeaconIAM │
     │  PC API  │            │ Mobile   │            │  NGINX    │
     │  :5558   │            │  :5559   │            │  :5560    │
     └────┬─────┘            └────┬─────┘            └─────┬─────┘
          │                       │                       │
┌─────────▼───────────────────────▼───────────────────────▼─────────┐
│                         /opt/                                     │
│  ├── tuneloop/     (deploy:deploy)    ← symlink → /opt/flow/... │
│  ├── beaconiam/    (deploy:deploy)    ← symlink → /opt/flow/... │
│  ├── flow/         (coder:coder)      构建包仓库                 │
│  └── uploads/      (deploy:deploy)    用户上传文件               │
└──────────────────────────────────────────────────────────────────┘
          │
┌─────────▼─────────────────────────────────────────────────────────┐
│                     PostgreSQL (Docker)                           │
│  tuneloop_db  (预生产)    beaconiam_db  (预生产)                  │
│  tuneloop_debug (测试)    beaconiam_debug (测试)                  │
└───────────────────────────────────────────────────────────────────┘
```

---

## 二、端口分配

| 端口 | 项目 | 用途 | 环境 |
|------|------|------|------|
| 5554 | tuneloop PC | Vite dev server | 开发 |
| 5556 | tuneloop Mobile | 后端 API + 移动前端 | 开发 |
| 5557 | tuneloop PC | 后端 API + PC 前端 | 开发 |
| **5558** | tuneloop PC | 后端 + PC 前端 | **预生产** |
| **5559** | tuneloop Mobile | 后端 + 移动前端 | **预生产** |
| **5560** | beaconiam | NGINX 代理 | **预生产** |
| 5561 | beaconiam | 后端 API | 开发 |
| 5432 | PostgreSQL | 数据库 | 全部 |

---

## 三、目录结构

```
/opt/
├── tuneloop/              # 预生产运行目录 (owner: deploy)
│   ├── .env               # 预生产环境变量
│   ├── www/      → flow/YYYYMMDD-HHMMSS/tuneloop/www/     (symlink)
│   ├── mobile/   → flow/YYYYMMDD-HHMMSS/tuneloop/mobile/  (symlink)
│   ├── service/  → flow/YYYYMMDD-HHMMSS/tuneloop/service/ (symlink)
│   ├── database/ → flow/YYYYMMDD-HHMMSS/tuneloop/database/ (symlink)
│   └── uploads/  → /opt/uploads/                          (symlink)
├── beaconiam/             # 预生产运行目录 (owner: deploy)
│   ├── .env
│   ├── www/      → flow/YYYYMMDD-HHMMSS/beaconiam/www/    (symlink)
│   ├── service/  → flow/YYYYMMDD-HHMMSS/beaconiam/service/(symlink)
│   └── jwt_*.pem          # JWT 密钥对
├── flow/                  # 发布包仓库 (owner: coder)
│   ├── deploy.sh          # 部署脚本
│   └── YYYYMMDD-HHMMSS/   # 版本目录
└── uploads/               # 用户上传文件 (owner: deploy)
```

---

## 四、数据库

| 数据库 | 用途 | 环境 |
|--------|------|------|
| `tuneloop_db` | Tuneloop 业务数据 | **预生产** |
| `tuneloop_debug` | Tuneloop 业务数据 | 测试/开发 |
| `beaconiam_db` | IAM 认证数据 | **预生产** |
| `beaconiam_debug` | IAM 认证数据 | 测试/开发 |

访问方式：
```bash
# 预生产数据库
docker exec -it jobmaster-postgres psql -U tuneloop_user -d tuneloop_db
docker exec -it jobmaster-postgres psql -U iam_user -d beaconiam_db

# 测试数据库
docker exec -it jobmaster-postgres psql -U tuneloop_user -d tuneloop_debug
docker exec -it jobmaster-postgres psql -U iam_user -d beaconiam_debug
```

---

## 五、构建与发布

### 5.1 构建

在开发目录执行：

```bash
cd /home/coder/tuneloop
make release
```

**构建内容**：
- tuneloop PC 前端 (`frontend-pc/dist/`)
- tuneloop Mobile 前端 (`frontend-mobile/dist/`)
- tuneloop 后端 (`backend/` → Go binary)
- tuneloop 数据库迁移 (`backend/database/migrations/`)
- beaconiam 后端 (`../beaconiam/cmd/api` → Go binary)
- beaconiam 前端 (`../beaconiam/ui/dist/`)

**输出**：`/opt/flow/YYYYMMDD-HHMMSS.zip`

### 5.2 部署

```bash
cd /opt/flow
sudo ./deploy.sh YYYYMMDD-HHMMSS
```

`deploy.sh` 执行流程：
1. 解压 zip → `/opt/flow/YYYYMMDD-HHMMSS/`
2. 停止服务 (`systemctl stop tuneloop-prerelease beaconiam-prerelease`)
3. 更新 `/opt/tuneloop/` 和 `/opt/beaconiam/` 的 symlink
4. 复制 `.env` 文件（不 symlink，防误改）
5. 生成 JWT 密钥（首次部署）
6. 输出部署结果

### 5.3 启动服务

```bash
sudo systemctl start tuneloop-prerelease
sudo systemctl start beaconiam-prerelease
```

---

## 六、回滚

```bash
# 1. 列出已部署版本
ls /opt/flow/

# 2. 部署指定旧版本（symlink 自动更新）
cd /opt/flow
sudo ./deploy.sh YYYYMMDD-HHMMSS

# 3. 启动服务
sudo systemctl start tuneloop-prerelease
```

---

## 七、域名与 NGINX

| 域名 | 代理 | 配置文件 |
|------|------|---------|
| `web.cadenzayueqi.com` | :5558 (PC) | `/etc/nginx/conf.d/web.conf` |
| `wx.cadenzayueqi.com` | :5559 (Mobile) | `/etc/nginx/conf.d/wx.conf` |
| `iam.cadenzayueqi.com` | :5560 (BeaconIAM) | `/etc/nginx/conf.d/iam.conf` |

所有域名均使用 Let's Encrypt 自动 TLS（certbot）。

NGINX 静态文件路径已更新为 `/opt/tuneloop/`（通过 symlink 解析到 `/opt/flow/YYYYMMDD-HHMMSS/tuneloop/`）。

### 重载 NGINX 配置

```bash
sudo nginx -t && sudo systemctl reload nginx
```

---

## 八、服务管理

### 8.1 预生产服务

| 服务 | 端口 | systemd 单元 |
|------|------|-------------|
| tuneloop PC | 5558 | `tuneloop-prerelease.service` |
| tuneloop Mobile | 5559 | (同上，多端口) |
| beaconiam | 5560 | `beaconiam-prerelease.service` |

### 8.2 常用命令

```bash
# 查看状态
sudo systemctl status tuneloop-prerelease beaconiam-prerelease

# 查看日志
sudo journalctl -u tuneloop-prerelease -f
sudo journalctl -u beaconiam-prerelease -f

# 重启
sudo systemctl restart tuneloop-prerelease beaconiam-prerelease

# 停止
sudo systemctl stop tuneloop-prerelease beaconiam-prerelease
```

---

## 九、日常运维检查清单

### 部署后必查

- [ ] `make release` 成功，zip 在 `/opt/flow/` 下
- [ ] `deploy.sh` 执行无报错
- [ ] `/opt/tuneloop/` 和 `/opt/beaconiam/` 的 symlink 指向正确版本
- [ ] 服务启动成功：`systemctl status tuneloop-prerelease beaconiam-prerelease`
- [ ] HTTPS 可访问：`curl -I https://web.cadenzayueqi.com`
- [ ] IAM 可访问：`curl -I https://iam.cadenzayueqi.com`
- [ ] 数据库迁移已执行（查看服务日志：`[Bootstrap]` 段）
- [ ] JWT 密钥存在：`ls -la /opt/beaconiam/jwt_*.pem`

### 定期检查

- [ ] 磁盘空间：`df -h /opt/`
- [ ] 数据库连接：`docker exec jobmaster-postgres psql -U tuneloop_user -d tuneloop_db -c "SELECT 1"`
- [ ] SSL 证书有效期：`certbot certificates`
- [ ] 清理旧版本包（保留最近 5 个）：`ls -t /opt/flow/*.zip | tail -n +6 | xargs rm -f`

---

## 十、开发 vs 预生产

| 维度 | 开发 | 预生产 |
|------|------|--------|
| 代码位置 | `/home/coder/tuneloop/` | `/opt/tuneloop/` (symlink) |
| 启动方式 | `go run main.go` / `npm run dev` | systemd service |
| 访问域名 | `opencode.linxdeep.com:5554` | `web.cadenzayueqi.com` (HTTPS) |
| 数据库 | `tuneloop_debug` | `tuneloop_db` |
| 进程用户 | `coder` | `deploy` |
| 前端 | Vite HMR 热更新 | 构建后静态文件 |
| NGINX | 不经过 | 443 SSL 代理 |

---

*基于 Issue #545 架构设计方案，由 #542–#549 迭代修订*
