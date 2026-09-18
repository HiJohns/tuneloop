# 生产发布 Checklist（上生产前逐项勾选）

> 目标：**上生产一步成功**。原则：一切变更先在预生产完整验证 + 结构变化必须伴随迁移逻辑（教训：wx_user_bindings #483 新增表未迁移旧 `users.wx_openid` → 生产所有旧绑定用户微信登录 `wx_user_not_found`）。

---

## 0. 预生产就绪（发布前 ≤24h 内完成）

- [ ] 预生产已部署最新代码（tuneloop-pre / beaconiam-pre / weapp 体验版）
- [ ] 核心业务回归：登录（PC H5 / 微信）、下单支付、报修、退款结算
- [ ] 后端日志无 `panic` / `FATAL` / `Invalid issuer` / 静默吞错
- [ ] **结构变化专项**：本次涉及表结构/字段变化 → 迁移逻辑已在预生产执行且验证通过（含幂等重启验证），见 §3 专项卡

## 1. 构建（构建服务器 /home/coder）

```bash
# 微信小程序：先构建归档，再上传（上传不重新编译，保证发布内容=已验证内容）
cd frontend-mobile          # nvm use 22 必须！
nvm use 22
make weapp-build-prod VERSION=YYYYMMDD-HHMMSS_COMMITID   # 生成 releases/weapp-prod/<VERSION>/dist-weapp
make weapp-upload-prod VERSION=YYYYMMDD-HHMMSS_COMMITID DESC="release note"
# 注意：VERSION 必须命令行显式传（Makefile 的 $(origin VERSION) 判断，否则回退 1.0.0）

# tuneloop（后端 + PC + H5）——make release 默认产出预生产包并自动部署到 pre！
make release

# beaconiam（build + ui + 自动 scp 到 cadenza:/opt/flow）
cd ../beaconiam && make release
```

## 2. 部署（cadenza）

```bash
# 预生产：make release 已自动触发（Seafile 迂回 + download.sh）
# 生产：预发布包 promote 为正式包
ssh cadenza "cd /opt/flow && ./release.sh <tuneloop-pre_xxx.zip|beaconiam_xxx.zip>"
# 或直接生产构建包（若已单独出产生产版）：
ssh cadenza "cd /opt/flow && ./deploy.sh <pkg>"
```

**deploy.sh 规则（2026-08-13 修）**：
- systemd 单元跟随 `TUNELOOP_APPS_BASE`：pre 目录 → `beaconiam-pre`/`tuneloop-pre`，生产 → `beaconiam`/`tuneloop`（不再误停生产单元）
- 二进制名自动检测（tuneloop/beaconiam），不再写死
- 部署前先备份：`cp /opt/flow/deploy.sh /opt/flow/deploy.sh.bak`

## 3. 验证清单（部署后逐项）

### 3.1 服务与端口

```bash
ssh cadenza
systemctl is-active tuneloop tuneloop-pre beaconiam beaconiam-pre   # 全 active
ss -tlnp | grep -E '55[6][0-9]'
# 生产: 5558(web前端 outbound?) / 5560(iam) / 5566(wx)  ；预生产: 5562/5563/5564
```

### 3.2 IAM 密钥与 Token 链（混用必报 crypto/rsa verification error）

```bash
curl -s http://localhost:5560/api/v1/auth/public-key.pem | sha256sum   # 生产
curl -s http://localhost:5562/api/v1/auth/public-key.pem | sha256sum   # 预生产
journalctl -u tuneloop --since "30 min ago" --no-pager | grep -c "Invalid issuer"   # 必须 0
curl -s -o /dev/null -w "%{http_code}" "https://web.cadenzayueqi.com/callback..."  # OAuth 302 正常
```

### 3.3 前端版本（内容 hash 命名）

```bash
# 生产 index.html 引用的 JS 文件名 必须 = 本地构建产物
curl -s https://wx.cadenzayueqi.com/index.html | grep -oP 'index-[a-zA-Z0-9]+\.js'
grep -oP 'index-[a-zA-Z0-9]+\.js' frontend-mobile/dist/index.html
# hash 一致性（可选加强）：
curl -s https://wx.cadenzayueqi.com/assets/<name>.js | sha256sum   vs  sha256sum frontend-mobile/dist/assets/<name>.js
```

### 3.4 数据库迁移

```bash
# tuneloop：schema_migrations 最高版本 = 发版包内最新 migration 编号，dirty=false
docker exec <postgres> psql -U tuneloop_user -d tuneloop -c "SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 3"
# beaconiam：启动日志确认迁移完成
journalctl -u beaconiam --since "5 min ago" --no-pager | grep MIGRATION
# 明确无未执行迁移（如 tuneloop 的 --migrate-* flag 迁移要在 WorkingDirectory 下执行 + 重启服务）
```

### 3.5 上传目录隔离（预生产/生产不得互指）

```bash
readlink /opt/tuneloop-pre/apps/tuneloop-pre/uploads   # → /opt/tuneloop-pre/uploads
readlink /opt/tuneloop/apps/tuneloop/uploads           # → /opt/uploads
```

### 3.6 微信小程序（上传≠可用）

- [ ] 微信公众平台「版本管理」将归档版本选为**体验版** → 真机回归
- [ ] 生产 appid `wxcb44a1be70e356ed`、预生产 `wx9f96827856269a6c` 域名与 apiBaseUrl 与环境匹配
- [ ] 合法域名（request/uploadFile）已配好（prewx.cadenzayueqi.com / wx.cadenzayueqi.com）

## 4. 专项验证卡（本次涉及的结构/数据迁移）

> 每次发版若含结构变化，复制本卡填入本次变更内容，预生产执行后粘贴结果到发布记录。

| 项 | 预生产实况 | 生产预期 | 验证命令 |
|----|-----------|---------|---------|
| 旧字段用户数 | 2 | = 生产 legacy 数 | `SELECT count(*) FROM users WHERE wx_openid != ''` |
| 回填后绑定数 | 2 | = legacy 数 | `SELECT count(*) FROM wx_user_bindings` |
| 迁移标记 | 20260813_wx_binding_backfill | 部署后出现 | `journalctl -u beaconiam \| grep wx_binding_backfill` |
| 幂等 | 重启后 Skip，无重复行 | 同样 Skip | 重启 + 计数不变 |
| 旧用户登录 | 林维训 (linwx1978@163.com) 微信登录直达 | 抽 1-2 个老用户真机登录 | 体验版/生产小程序 |

## 5. 回滚方案

| 组件 | 方式 |
|------|------|
| weapp | 上传更早归档：`make weapp-upload-prod VERSION=<旧归档>`（归档保留 ≥180 天） |
| tuneloop / beaconiam | `./deploy.sh` 更早的 zip 包（旧包仍留在 /opt/flow/）|
| DB 迁移 | 先确认回滚版本，需要 DDL 反向操作 → **必须先与用户确认** |

## 6. 发布后 24h 观察

- [ ] journalctl ERROR/FATAL 计数 0
- [ ] 支付/退款回调失败率无异常（微信支付日志）
- [ ] 旧绑定用户微信登录无 `wx_user_not_found` 上报
- [ ] 用户反馈渠道无新问题类型

---

*Last updated: 2026-08-13（wx binding 迁移专项 + deploy.sh 单元修复）*