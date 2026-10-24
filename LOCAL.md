# 本地开发环境变更记录

> 记录日期：2026-08-31
> 分支：develop

## 大前提：开发环境支持服务架构变更

| 维度 | 变更前 | 变更后 | 驱动原因 |
|------|--------|--------|---------|
| **数据库** | 本地 Docker PostgreSQL（`tuneloop_debug`） | 远程预生产数据库（`tuneloop_pre_snapshot` via SSH 隧道） | 本地 Docker 不可用 / 需与预生产数据一致 |
| **BEACONIAM** | 本地 Docker BEACONIAM（`:5561`） | 远程预生产 BEACONIAM（`preiam.cadenzayueqi.com`） | 本地 Docker 不可用 / 复用预生产用户和权限数据 |
| **多媒体文件** | 本地 `uploads/` 目录 | 远程 `/opt/tuneloop-pre/uploads`（455 文件 via SSHFS 挂载） | 本地无媒体文件，需调试图片展示 |
| **小程序验证** | 真机（微信扫码） | 微信开发者工具 + `dist-weapp/` 构建产物 | 本地无法直接上传真机验证 |

---

## 变更清单

### 一、Makefile 与 Makefile.local（Windows 本地适配）

> **结构**：本机适配全部集中在 `Makefile.local`（222 行，`.gitignore:70` 忽略，**不提交**），由共享 `Makefile` 末尾的 `-include Makefile.local`（`Makefile:273-274`）加载——后者的 3 行是共享 `Makefile` 的**唯一**本地改动，**按决定不提交**。Makefile 变量/目标「后定义覆盖先定义」，故 `Makefile.local` 可覆写共享定义。

| # | 变更内容 | 位置 | 变更原因 |
|---|---------|------|---------|
| 1 | 覆写 `NVM22` 为 `export PATH="/c/nvm4w/nodejs:$$PATH" &&` | `Makefile.local:12` | 共享版（`Makefile:4`）依赖 `~/.nvm/nvm.sh`，本机为 nvm4w、**该文件不存在**→ 共享版在本机必然失败。**为何不用 `nvm use 22`**：本机并存两套 Node——nvm4w 的 `/c/nvm4w/nodejs`（v22.23.2）与**真实安装目录** `/c/Program Files/nodejs`（v26.4.0，Taro 4 不兼容）；`C:\Program Files\nodejs` 非 junction，nvm4w 1.2.2 已移除 `nvm symlink` 子命令故无法接管，而 `nvm use 22` 会把该目录**前置到 PATH 首位**（实测：调用前 `/c/nvm4w/nodejs/node` → 调用后 `/c/Program Files/nodejs/node`，退出码仍为 0）。即 `nvm use 22` 在本机是**反向操作**，会把构建从 v22 推进 v26，故改为绝对路径注入（实测非交互 shell 内 3/3 = v22.23.2）。⚠️ 注入**只保证「nvm 当前版本」**：nvm4w 已装 16.20.2/22.23.2/24.18.0，当前版本会漂移（2026-09-25 曾变为 v24.18.0 → 注入即得 v24，Taro 4 不兼容），故构建前须 `nvm current` 核验为 v22.23.2（详见 §八#31） |
| 2 | 未新增 `.PHONY` | — | 原记「新增 `.PHONY`：weapp-build-local/tunnel-ssh/…」**不成立**：`Makefile.local` 无 `.PHONY` 声明，共享 `Makefile:1` 列表亦不含这些目标。当前无同名文件故不影响执行；如需严谨可另行补声明（本次未做） |
| 3 | `include backend/.env` + `include frontend-mobile/.env.local` + `export` | `Makefile.local:7-9` | 注入 SSH 隧道 / SSHFS 变量（来自 `backend/.env`）与小程序本地地址、版本（来自 `frontend-mobile/.env.local`），避免硬编码 |
| 4 | `check-tunnel` 目标 | `Makefile.local:18-21` | 按 `SSH_TUNNEL_LOCAL_PORT` 检查隧道是否 LISTENING，用于调试与脚本预检 |
| 5 | `tunnel-ssh` 目标 | `Makefile.local:23-28` | `ssh cadenza -L 5432:localhost:5432` 建立到 cadenza 的隧道，使本地可访问远程 PostgreSQL |
| 6 | `mount-uploads` 目标（+ `check-uploads:34`、`mount-sshfs:43`、`ensure-junction:59`） | `Makefile.local:34-79` | SSHFS 将远程 `/opt/tuneloop-pre/uploads` 挂载到盘符，再以 NTFS junction 映射为 `backend/uploads`；含 15s 就绪轮询与失败清理 |
| 7 | `unmount-uploads`（+ `unmount-sshfs:81`） | `Makefile.local:81-92` | `taskkill sshfs` + `net use <盘符>: /delete` + 释放 5556/5557 端口 |
| 8 | 覆写 `run-backend`：`run-backend: kill-port mount-uploads`，隧道检测/启动**内联**于 recipe | `Makefile.local:100-115` | 一键启动（挂载 + 隧道 + 后端）。注意隧道逻辑是内联复制，**不依赖** `tunnel-ssh` 目标 |
| 9 | 覆写 `stop`：先 `unmount-sshfs`，再按 `SSH_TUNNEL_LOCAL_PORT` 反查 PID 终止，最后 `pkill` | `Makefile.local:121-128` | 完整清理：卸载挂载 → 终止隧道 → 停后端/前端 |
| 10 | `weapp-build-local` 目标 | `Makefile.local:134-138` | 本地编译小程序（`$(TARO_APP_API_BASE_URL)`=localhost:5556、`$(TARO_APP_VERSION)`，均来自 `.env.local`），供微信开发者工具联调本地后端；编译前清缓存 |
| 11 | 覆写 `weapp-archive-pre` | `Makefile.local:164-170` | 共享版（`Makefile:99-105`）的 wxss 清理 sed `s/\\\//-/g` 对分隔符的转义写法**在 GNU sed 4.9 下不合法**，报 `char 33: unknown option to 's'` → 归档步骤失败（`make weapp-build-pre` 退出码 2），且失败时已建目录+已拷贝产物，形成**未清理的半成品归档**。覆写后改用 `\|` 作分隔符并以 `[\\]` 括号表达式匹配反斜杠，实测退出码 0 |
| 12 | 覆写 `weapp-archive-prod` | `Makefile.local:172-178` | 同 #11（共享 `Makefile:107-113` 同一份 sed 缺陷 → 正式版归档同样会失败）。本地一并覆写以保证 `make weapp-build-prod` 可用 |

### 二、backend/main.go（1 项）

| # | 变更内容 | 变更原因 |
|---|---------|---------|
| 11 | 第849行：`BootstrapDatabase` 前增加 `SKIP_MIGRATION` 环境变量检查 | 本地连接远程数据库，迁移应由远程管理，本地跳过避免误操作或迁移文件不兼容导致启动失败 |

### 三、frontend-pc/package-lock.json（1 项）

| # | 变更内容 | 变更原因 |
|---|---------|---------|
| 12 | 多个依赖的 `"peer": true` 被移除（react, react-dom, dayjs, postcss, vite 等） | 执行 `npm install` 时 npm 自动重写 lockfile，将 peer dependencies 转为普通依赖记录 |

### 四、backend/.env（gitignored，不提交，7 项）

| # | 变更内容 | 变更原因 |
|---|---------|---------|
| 13 | `POSTGRES_HOST=localhost`, `POSTGRES_PORT=5432`, `TUNELOOP_DB=tuneloop_pre_snapshot`, `DB_SSLMODE=disable` | 本地开发通过 SSH 隧道连接远程预生产数据库，使用独立快照库避免污染预生产数据 |
| 14 | `SSH_TUNNEL_HOST=39.96.35.47`, `SSH_TUNNEL_KEY=...cadenza.key`, 端口 `5432→5432` | SSH 隧道配置：通过 cadenza 服务器转发 PostgreSQL 端口 |
| 15 | `BEACONIAM_EXTERNAL_URL=https://preiam.cadenzayueqi.com`, `IAM_NAMESPACE=tuneloop-pre`, `IAM_SECRET=...` | 本地后端直接对接远程预生产 BEACONIAM，复用预生产用户数据和权限 |
| 16 | `SSHFS_EXEC=...sshfs.exe`, `SSHFS_REMOTE_PATH=/opt/tuneloop-pre/uploads` | SSHFS 挂载配置：将远程媒体文件目录映射到本地 |
| 17 | ~~`NODE22_PATH=C:/Users/Ken/AppData/Local/nvm/v22.23.2`~~ —— **已移除**（现存 32 键中无此项） | 原方案为 Node 22 绝对路径注入；现改由 nvm4w PATH shim 提供（`nvm use 22`，见 §一#1 与 §八#31），绝对路径注入不再需要 |
| 18 | `SKIP_MIGRATION=true` | 配合 `backend/main.go` 的检查逻辑，跳过本地迁移执行 |
| 19 | 保留原有 `WX_APPID`, `WX_SECRET`, `WECHAT_PAY_*` 等配置 | 本地后端仍需微信登录/支付配置 |

### 五、frontend-mobile/.env.local（gitignored，不提交，2 项）

| # | 变更内容 | 变更原因 |
|---|---------|---------|
| 20 | `TARO_APP_API_BASE_URL=http://localhost:5556` | 小程序编译时注入本地后端地址，使 `platform/index.weapp.js` 中的 `request()` 指向本地 |
| 21 | `TARO_APP_VERSION=dev` | 本地开发版本标识，区分预生产/生产构建 |

### 六、frontend-mobile/.nvmrc（未跟踪，1 项）

| # | 变更内容 | 变更原因 |
|---|---------|---------|
| 22 | 内容 `22.23.2` | 记录项目所需 Node.js 版本，IDE 可自动切换 |

### 七、node_modules 补丁（每次 npm install 后需重做，2 项）

| # | 变更内容 | 变更原因 |
|---|---------|---------|
| 23 | `@tarojs/webpack5-prebundle/dist/utils/index.js:42`：`roots: appPath` → `roots: [appPath]` | enhanced-resolve@5.24.0 要求 `roots` 为数组类型，Taro 传入字符串导致预编译阶段 TypeError 崩溃 |
| 24 | `frontend-mobile/src/App.css`：`git update-index --assume-unchanged` | 文件名大小写冲突（Windows 不区分大小写），ghost 文件干扰 git 状态显示 |

---

## 变更统计

| 类别 | 变更数 | 提交状态 |
|------|:------:|---------|
| Makefile（仅末尾 `-include Makefile.local` 钩子，`Makefile:273-274`） | 1 | 未提交（按决定不提交） |
| Makefile.local（Windows 适配全集，222 行） | 3（文件级：NVM22 覆写 + weapp-archive-pre/prod 覆写） | gitignore，不提交 |
| backend/main.go | 1 | 未提交 |
| frontend-pc/package-lock.json | 1 | 未提交 |
| 配置文件（gitignored） | 7 | 不提交 |
| .nvmrc | 1 | 未跟踪 |
| node_modules 补丁 | 2 | 不提交 |
| **合计（已跟踪文件改动）** | **3** | — |

---

## 八、本机工具与新 Seafile 链路（2026-09-12）

> 目的：记录与实际文件目录不一致（AGENTS.md 记载 ≠ 本机实际）的路径与凭证，以及本机预生产发布所需的工具安装状态。**不修改 AGENTS.md**，本地差异统一在此记载。

| # | 事项 | 记载 |
|---|------|------|
| 23 | **cadenza SSH 密钥实际路径** | `D:/Work/AI/rent/certs/cadenza.key`（AGENTS.md 记载 `~/cert/cadenza.key` **不存在**）；`~/.ssh/config` 新增 `cadenza` 别名（Host 39.96.35.47，User root，IdentityFile 如上）承载连接，已实测连通 |
| 24 | **Seafile 凭证文件** | `D:/Work/AI/rent/certs/seafile.key`（`export SEAFILE_USERNAME/PASSWORD/SERVER_URL/REPO_ID`，shell 格式）；`Makefile.local:218-219` 在 release 上传段以 `source <绝对路径> && bash ~/scripts/upload_to_seafile.sh …` 同行注入（共享 `Makefile:231` 无 source，Linux 侧凭证由构建环境自行注入）。**不需要 SEAFILE_PATH**（新脚本用位置参数 dest-folder） |
| 25 | **Seafile 上传脚本** | `~/scripts/upload_to_seafile.sh`（参数式：`<target-filename> <dest-folder>`；仅需上述 4 个环境变量；调用 `<dest>/<file>` 复用/创建 share link）；仓库 `scripts/push_seafile.sh` **保留不动、不使用** |
| 26 | **jq（Seafile 上传解析依赖）** | winget 装 `jqlang.jq` 1.8.2 → `C:\Users\Ken\AppData\Local\Microsoft\WinGet\Packages\jqlang.jq_Microsoft.WinGet.Source_8wekyb3d8bbwe\jq.exe`；User PATH 已加该目录，**新开 shell 生效**（当前 shell 需全路径） |
| 27 | **zip（发布打包）** | `C:\Program Files (x86)\GnuWin32\bin\zip.exe`（GnuWin32.Zip 3.0，winget 装）；User PATH 已加，**新开 shell 生效** |
| 28 | **后端交叉编译**（预生产发布用） | `Makefile.local:196`：`cd backend && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-X main.Version=$(VERSION) -X main.Build=$(GIT_SHORT)" -o …/service/tuneloop .`；backend 纯 Go（无 `import "C"`），CGO_ENABLED=0 可行。共享 `Makefile:204` 为原生 `go build`（Linux 上无需交叉编译） |
| 29 | **SSH 隧道改别名** | `ssh cadenza -L …`（隧道建立）位于 `Makefile.local:27`（`tunnel-ssh` 目标）与 `:112`（`run-backend` 内联复制）；mount-sshfs 的 SSH 身份走 `:45` 的 `IdentityFile=$(SSH_TUNNEL_KEY)`。key/主机信息交由 `~/.ssh/config` 承载 |
| 30 | **小程序私钥绝对路径** | `Makefile.local:149`（`weapp-upload-dev`）/ `:159`（`weapp-upload-prod`）：`--pkp D:/Work/AI/rent/certs/private.wxcb44a1be70e356ed.key`（`frontend-mobile/` 下无该私钥，共享 `Makefile:130/:140` 的相对路径在本机上传必失败，故必须覆写） |
| 31 | **Node 版本** | 构建 Taro/weapp 需 **node v22**。本机并存两套 Node：nvm4w 的 `/c/nvm4w/nodejs`（**v22.23.2**）与**真实安装目录** `/c/Program Files/nodejs`（**v26.4.0**，Taro 4 不兼容）。⚠️ **`nvm use 22` 在本机是反向操作**：它把 `C:\Program Files\nodejs` 前置到 PATH 首位（实测调用前 `/c/nvm4w/nodejs/node` → 调用后 `/c/Program Files/nodejs/node`，**退出码仍为 0**，故不会报错、只会静默用错版本）；`~/.nvm/nvm.sh` 不存在 + nvm4w 1.2.2 已移除 `nvm symlink`，无法接管该真实目录。共享 `Makefile:4` 的 `NVM22`（`nvm use 22`）在本机双重失效，现由 `Makefile.local:12` 覆写为绝对路径注入 `export PATH="/c/nvm4w/nodejs:$$PATH" &&`。**人工执行发布时不要手动 `nvm use 22`**——直接跑 make 即可（recipe 自行注入正确 PATH）。⚠️ **注入只保证「nvm 当前版本」，不保证是 v22**：nvm4w 已装 3 个版本（16.20.2 / 22.23.2 / 24.18.0），当前版本会漂移——2026-09-25 拉取后曾变为 **v24.18.0**，此时注入路径即解析到 v24，Taro 4 在 Node 24 下报 `module is not defined in ES module scope`。**每次 weapp 构建前先核验 `nvm current` = v22.23.2**（等价于软链 `/c/nvm4w/nodejs -> /c/Users/Ken/AppData/Local/nvm/v22.23.2`）；若不符，只能改软链指向（**不要用 `nvm use`**，见上）。仓库有 `frontend-mobile/.nvmrc` = 22.23.2，但 nvm4w **不遵守** `.nvmrc`（实测在仓库根执行 `nvm use` 无效，仍为原版本） |

---

## 九、发布流程（2026-09-12）

### 9.1 版本管理原则（VERSION 必须提交）

> **核心判断**：`VERSION` 文件**不属于「仅本地不提交」范围**。本地改动它的目的是**向远程传达版本信息**，与因本地环境修改的内容（Makefile 交叉编译/隧道别名/私钥绝对路径、jq/zip 安装、SSH config 等）性质不同——纯本地内容无需也不应通知远程。因此 `VERSION` bump 必须随发布 **`git add VERSION && git commit && git push`**，防止多个开发者基于旧版本号各自 bump 导致版本混乱。

| 改动类型 | 例子 | 提交策略 |
|---------|------|:--:|
| 远程相关（发布状态） | `VERSION` 文件 | ✅ 必须提交（随发布） |
| 纯本地环境适配 | Makefile 交叉编译/隧道别名/私钥绝对路径；jq/zip 安装；`~/.ssh/config`；`backend/.env` | ❌ 不提交 |

### 9.2 后端发布流程（预生产，`make release` 全自动链路）

发布末尾的自动动作（`Makefile.local:216-221`）：`:216` `cp <pkg>.zip ~/test.zip` → `:218-219` `source /d/Work/AI/rent/certs/seafile.key && bash ~/scripts/upload_to_seafile.sh ~/test.zip /debug/uem-core/5.2/test` → `:221` `ssh cadenza "~/download.sh <pkg>.zip"`（cadenza 下载 + deploy.sh 部署）。共享 `Makefile:228-235` 为 Linux 等价段（不含 source 注入）。

```bash
# ── 前置 ──
nvm current                    # 必须 v22.23.2，否则中止（注入只跟随 nvm 当前版本，见 §八#31）
#                            # ⚠️ 禁止手动 nvm use —— 会把 v26.4.0 前置（反向操作）
#                            # 新开 shell 确保 jq/zip 在 PATH（winget 已加 User PATH）

# ── 1. 版本 bump + 提交（VERSION 必须提交）──
make version                  # 确认当前（2026-09-25 = 1.0.24）
make bump-build               # 1.0.24 → 1.0.25（bump-build/bump-minor/bump-major 三选一）
make version                  # 确认新版本
git add VERSION && git commit -m "chore: bump VERSION 1.0.24 -> 1.0.25" && git push

# ── 2. 发布（全自动）──
make release
#   自动依次：clean → build backend(GOOS=linux CGO_ENABLED=0，Makefile.local:196) + PC + H5(mode prerelease)
#            → 打包 → cp ~/test.zip → source seafile.key → 上传 Seafile
#            → ssh cadenza download.sh（下载 + deploy.sh 部署）

# ── 3. 预生产验证 ──
ssh cadenza "systemctl is-active tuneloop-pre"                      # active
ssh cadenza "ss -tlnp | grep -E '556[234]'"                          # 端口监听
ssh cadenza "curl -s http://localhost:5563/api/config"               # version 应 = 新值
ssh cadenza "sudo journalctl -u tuneloop-pre --since '1 min ago' --no-pager | grep -iE 'FATAL|panic'"
# 前端 JS hash：curl -sk https://prewx.cadenzayueqi.com/index.html | grep index-*.js
```

### 9.3 小程序发布流程（预生产 §9.3.1 / 生产 §9.3.2）

**生产 / 预生产异同**：

| 维度 | 预生产 §9.3.1 | 生产 §9.3.2 |
|------|--------------|------------|
| apiBaseUrl | `prewx.cadenzayueqi.com` | `wx.cadenzayueqi.com` |
| 构建 | `weapp-build-pre` | `weapp-build-prod` |
| 归档库 | `releases/weapp-pre/` | `releases/weapp-prod/` |
| 上传命令 | `weapp-upload-dev` | `weapp-upload-prod` |
| APP_VERSION | `1.0.0-dev`（固定、可覆盖） | `1.0.x`（**必须 > 线上+审核中**） |
| 微信侧动作 | 设为体验版分发 | 提交审核 → 通过后发布 |
| 数据末端 | prewx → 预生产快照库 | wx → 生产库 |
| 回退 | 上传更早归档（可覆盖重传） | 上传更早归档（需重新过审） |

**通用注意事项**：
- ⚠️ **`make release` 不含 weapp**（教训 2026-09-07 续期支付明细只跑 make release，小程序仍旧版）→ 小程序发布必须单独执行本节。
- ⚠️ **单 appid 策略（#1694）**：同一 `wxcb44a1be70e356ed`，预生产=开发版、生产=正式版，无独立预生产小程序。
- ⚠️ **上传不重新编译**：归档即成品（wxss 清理在归档时做一次），发布内容 = 已验证内容（同样本回退可靠）。
- ⚠️ **微信 CI IP 白名单（上传前置门）**：`miniprogram-ci upload` 要求**出口 IP 在小程序后台白名单内**，否则报 `errCode -10008 invalid ip: <当前出口IP>`（2026-09-25 实测被拒，出口 `219.237.112.50`）。**上传前确认白名单含当前出口 IP**（微信后台 UI，AI 无权限）；出口 IP 取决于本机代理节点（`127.0.0.1:10808`）——被拒时需加白名单或换已加白节点，**重试只重跑上传命令、无需重新构建**。
- 两流程共用前置：**Node22 前置核验** = `nvm current` 必须为 **v22.23.2**（`Makefile.local:12` 只做绝对路径注入，跟随 nvm 当前版本；nvm4w 装有 16.20.2/22.23.2/24.18.0，当前版本会漂移至 v24 → Taro 4 在 Node 24 下报 `module is not defined in ES module scope`）。⚠️ **禁止手动 `nvm use`**（会把 `C:\Program Files\nodejs` v26.4.0 前置到 PATH 首位，是反向操作，见 §八#31）+ 归档号自动捕获防错。

### 9.3.1 预生产发布流程（开发版/体验版）

> ⚠️ **`make release` 不含 weapp**（教训 2026-09-07 续期支付明细只跑 make release，小程序仍旧版）；小程序发布必须单独执行本流程。差异见 §9.3 顶部异同表。

```bash
# ── 前置 ──
nvm current                    # 必须 v22.23.2，否则中止（Node 前置核验，见 §9.3 通用注意事项）
#                            # ⚠️ 上传前确认微信 CI IP 白名单含当前出口 IP（否则 -10008）

# ── 1. 构建 + 归档，自动捕获归档号（不靠人工抄录）──
BUILD_OUT=$(make weapp-build-pre 2>&1)                        # ① weapp-check → 清缓存 → prewx 构建 → 归档 → wxss 清理
echo "$BUILD_OUT"                                             # ② 完整回显（人工可审）
WEAPP_ARCHIVE=$(echo "$BUILD_OUT" | grep -oP 'releases/weapp-pre/\K[^/]+(?=/dist-weapp)' | tail -1)
[ -n "$WEAPP_ARCHIVE" ] && echo "▶ 捕获归档号: $WEAPP_ARCHIVE" \
  || { echo "❌ 未捕获归档号（构建失败？），已停止"; exit 1; }   # ③ 防错：错过即停，不携错号上传

# ── 2. 上传（消费同一变量，绝不手打）──
make weapp-upload-dev VERSION="$WEAPP_ARCHIVE" APP_VERSION=1.0.0-dev DESC="预生产体验版"
#   Makefile.local:145-152 覆写：归档不存在 → ERROR + exit 1
#   上传成功标准：miniprogram-ci 输出 0 error
#   （appid wxcb44a1be70e356ed；--pkp D:/Work/AI/rent/certs/private.wxcb44a1be70e356ed.key）

# ── 3. 人工分发（关键，缺此步测试者看不到新版；微信后台 UI 无法代劳）──
#   微信公众平台「版本管理 → 开发版本」→ 找到 1.0.0-dev → 【设为体验版】→ 二维码/白名单分发

# ── 4. 验证 ──
#   手机微信 → 体验版 → 「我的」页底部 v1.0.<短码>；短码须 = WEAPP_ARCHIVE 内嵌短码（旧短码 = 未重传）

# ── 回退 ──
make weapp-upload-dev VERSION=<更早归档号> APP_VERSION=1.0.0-dev DESC="rollback"
```

**防错保障**：① 归档号由 AI 从 `Archived:` 行自动提取、同会话变量传递，消除抄录/错位；② 提取失败立即 `exit 1`；③ Makefile `test -d` 二次校验；④ 短码内嵌核对验证发布可信；⑤ 回退 = 上传更早归档（可显式指定）。

版本要点：归档号 `时间戳_短码` 本地唯一（无跨开发者冲突，无 VERSION 依赖）；`APP_VERSION` 预生产固定 `1.0.0-dev`（可重复覆盖）；生产正式版才需 `1.0.x` 且人工核对高于微信后台线上/审核中。

---

#### 9.3.1.1 预生产小程序发布记录台账（每次上传后追加一行）

| 日期 | 归档号 | 短码 | APP_VERSION | 人工状态 |
|------|--------|------|:--:|---------|
| 2026-09-04 | 20260904-083534_923a9053 | 923a9053 | 1.0.0-dev | 已设体验版 |
| 2026-09-15 | 20260915-075704_4b08acd5 | 4b08acd5 | 1.0.0-dev | 待设体验版 |
| 2026-09-25 | 20260925-075629_3cc908ec | 3cc908ec | 1.0.0-dev | 已回退至 4b08acd5（#2066 首页菜单跟手行程） |
| 2026-09-25 | 20260915-075704_4b08acd5 | 4b08acd5 | 1.0.0-dev | rollback 重传（147 文件），待设体验版 |

### 9.3.2 生产发布流程（正式版，构建归档与上传分离）

> ⚠️ 与预生产 §9.3.1 的差异见 §9.3 顶部异同表；本流程使用独立归档库 `releases/weapp-prod/`。

```bash
# ── 前置 ──
# ① Node 前置核验：`nvm current` 必须 v22.23.2（同 §9.3.1；⚠️ 禁止手动 nvm use）
# ①' ⚠️ 微信 CI IP 白名单须含当前出口 IP（同 §9.3.1 通用注意事项）
# ② ⚠️ 版本号核对门（微信后台「版本管理」）：读取【线上版】+【审核中】版本号，
#    APP_VERSION 定为 1.0.<x+1>，必须高于两者；不符 → 停止，先处理线上/审核中版本再继续。

# ── 1. 构建 + 归档（生产 wx apiBaseUrl），自动捕获归档号 ──
BUILD_OUT=$(make weapp-build-prod 2>&1)   # weapp-check → 清缓存 → wx 构建 → weapp-archive-prod → wxss 清理
echo "$BUILD_OUT"                          # 完整回显（人工可审）
WEAPP_PROD_ARCHIVE=$(echo "$BUILD_OUT" | grep -oP 'releases/weapp-prod/\K[^/]+(?=/dist-weapp)' | tail -1)
[ -n "$WEAPP_PROD_ARCHIVE" ] && echo "▶ 捕获归档号: $WEAPP_PROD_ARCHIVE" \
  || { echo "❌ 未捕获归档号（构建失败？），已停止"; exit 1; }   # 防错：错过即停，不携错号上传

# ── 2. 上传正式版（消费变量，绝不手打）──
make weapp-upload-prod VERSION="$WEAPP_PROD_ARCHIVE" APP_VERSION=1.0.<x+1> DESC="正式发布"
#   Makefile.local:155-162 覆写：归档不存在 → ERROR + exit 1
#   上传成功标准：miniprogram-ci 输出 0 error
#   （appid wxcb44a1be70e356ed；--pkp D:/Work/AI/rent/certs/private.wxcb44a1be70e356ed.key）

# ── 3. 提审 + 发布（微信后台 UI，无法代劳）──
#   微信公众平台「版本管理」→ 找到 1.0.<x+1> → 【提交审核】(1-7 天)
#   审核通过 → 手动【发布】→ 正式版全量生效（请求走 wx 生产后端）

# ── 4. 验证 ──
#   手机微信 → 线上版 → 「我的」页底部 v1.0.<短码>；短码须 = WEAPP_PROD_ARCHIVE 内嵌短码（旧短码 = 未重传）

# ── 回退 ──
make weapp-upload-prod VERSION=<更早归档号> APP_VERSION=1.0.<更高值> DESC="rollback"
#   ⚠️ 回退版需重新提交审核（与预生产可覆盖重传不同）
```

**防错保障**：同 §9.3.1（① 归档号自动提取、② 失败即停、③ Makefile `test -d` 二次校验、④ 短码内嵌核对、⑤ 回退上传旧归档）。

版本要点：归档号 `时间戳_短码` 本地唯一（无跨开发者冲突，无 VERSION 依赖）；`APP_VERSION` 语义化 `1.0.x`，**必须高于微信后台线上/审核中**（微信强校验，最常见审核退回原因）；生产/预生产归档库相互独立、互不提升。

#### 9.3.2.1 生产小程序发布记录台账（每次上传后追加一行）

| 日期 | 归档号 | 短码 | APP_VERSION | 审核状态 | 人工状态 |
|------|--------|------|:--:|:--:|---------|
| （首次发布时填充） | — | — | — | — | — |

### 9.4 发布「按需调用」矩阵

| 改动范围 | 执行流程 | 版本动作 |
|---------|:---:|---------|
| 仅后端 / PC / H5 | **9.2 后端流程** | bump + **提交** VERSION |
| 仅小程序 | **9.3 小程序流程** | 无 bump（时间戳_短码自动） |
| 服务端 + 小程序 | 9.2 + 9.3 两套都跑 | 后端 bump+提交；小程序自动 |
| 生产提升 / 正式版小程序 | 维护流程（本次范围外） | `/opt/flow/release.sh`；`weapp-upload-prod APP_VERSION=1.0.x` |

### 9.5 首次正式发布待办（当前状态）

| # | 事项 | 状态 |
|---|------|:--:|
| 1 | 工具安装（zip / jq） | ✅ 完成 |
| 2 | cadenza SSH config / 隧道别名 | ✅ 完成 |
| 3 | 交叉编译（`Makefile.local:196`） | ✅ 完成 |
| 4 | Seafile 凭证 + 上传脚本 + source 注入（`Makefile.local:218-219`） | ✅ 完成 |
| 5 | 小程序私钥绝对路径（`Makefile.local:149`/`:159`） | ✅ 完成 |
| 6 | Seafile 可达性验证 | ✅ 通过（未上传） |
| 7 | `Makefile.local:12` NVM22 覆写（原 `nvm use 22 >` 缺重定向目标；后查明 `nvm use 22` 本身会前置 v26.4.0 → 改为绝对路径注入） | ✅ 完成（2026-09-25） |
| 8 | `Makefile.local:164-178` weapp-archive-pre/prod 覆写（共享 sed 在 GNU sed 4.9 下 `char 33: unknown option`，归档必失败） | ✅ 完成（2026-09-25） |
| 9 | 微信 CI IP 白名单含本机出口 IP（`219.237.112.50`，否则上传报 -10008） | ✅ 已加白（2026-09-25，上传成功） |
| 10 | **nvm 当前版本漂移**：nvm4w 装有 16.20.2/22.23.2/24.18.0，当前版本曾漂到 v24.18.0（注入即得 v24，Taro 4 报 `module is not defined`）。按约定**不改 Makefile**，改为每次 weapp 构建前 `nvm current` 核验 = v22.23.2；不符时改 nvm4w 软链指向，**禁用 `nvm use`** | ⏳ 流程已加前置门（2026-09-25） |
| 11 | **VERSION bump (1.0.24→1.0.25) + 提交** | ⏳ 发布时执行 |
| 12 | **`make release` 后预生产验证** | ⏳ 发布时执行 |

### 9.6 语音指令 → 全自动发布协议

> 用户只说「发布后端 / 发布小程序 / 发布两者」，AI 自动完成除微信设体验版、手机真机验证外的全部发布工作。

**触发指令映射**：

| 用户指令 | AI 自动执行流程 |
|---------|----------------|
| 发布后端 | ① 环境注入（Node22 + jq/zip PATH）→ ② §9.2 流程（bump → 提交 VERSION → `make release` → 预生产验证） |
| 发布小程序 | ① 环境注入（Node22）→ ② §9.3.1 预生产流程（构建 → 捕获归档号 → 上传 → 台账追加）→ ③ 提醒用户设体验版 |
| 发布小程序正式版 | ① 环境注入（Node22）→ ② §9.3.2 生产流程（版本号核对门 → 构建 wx → 捕获归档号 → 上传正式版 → 台账追加）→ ③ 提醒用户提审+发布 |
| 发布两者 | 先后端再小程序（同一会话，环境注入一次） |

**发布前的 2 个实时确认门（每次必须逐项现场询问，不预设、不跳过）**：

| # | 确认门 | 用户指示后 AI 动作 |
|---|--------|-------------------|
| 1 | **VERSION 是否 push** | 「提交」→ `git add VERSION && commit && push`；「不提交」→ 仅 commit 或仅 bump（按用户当场指示） |
| 2 | **发布基准（工作区含 `backend/main.go` 本地改动）** | 「接受现状」→ 直接以当前工作区发布；「排除改动」→ 先剥离 `backend/main.go`（stash 或按用户指定基准）再发布 |

**AI 自动化边界**：
- ✅ 可自动：环境注入、版本 bump/提交、构建、归档号捕获与传递、上传、Seafile 上传与部署触发、预生产服务端验证（systemctl/端口/`/api/config` 版本/journalctl）
- ❌ 不可自动（需用户）：微信公众平台设体验版（UI，无 API）；手机真机「我的」页版本核对（AI 仅给验证指令，展示层需用户确认）

**push 遵循既有 Git 策略**：推送前先 `git fetch` 只读确认领先/落后，无并发分叉才 push；推送失败自动重试仅 1 次，随后停止并询问。

## 十、opencode 工具链实体镜像（2026-09-16）

> **背景**：以下 5 个文件是全局 opencode 配置仓库（`HiJohns/opencode`，本机 `~/.config/opencode/`）的实体镜像副本。权威源在全局；本项目在 Windows 下因 `core.symlinks=false` 无法建真软链接，git HEAD 记录为软链接、工作区被实体化（`git status` 显示 `M` 属预期）。

| 文件 | 实际加载方式 | 类型 |
|------|-------------|------|
| `scripts/opencode_gh.sh` | 命令以相对路径调用 → 本副本生效 | **活副本（须同步）** |
| `scripts/find_issue.sh` | 命令以绝对路径 `~/.config/opencode/...` 调用 → 全局版生效 | 死副本（不调用） |
| `scripts/prepare.sh` | 手动运行（环境初始化） | 低频 |
| `prompts/instructions.md` | 命令以相对路径加载 → 本副本生效 | **活副本（须同步）** |
| `prompts/task-profiles.md` | 命令以绝对路径加载 → 全局版生效 | 死副本（不调用） |

**规则**：
- 禁止 commit 这 5 个文件（提交实体内容会造成双源分叉）
- 禁止反复运行 `prepare.sh`（含 `rm -f + ln -s` 强制重建，覆盖有意识变更）
- 活副本同步：全局更新后运行 `scripts/sync-opencode.sh`（本机脚本，已 gitignore）
- 死副本：不删不改（删除后 git 按软链接 blob 重建，内容为路径字符串、无害）
- `M` 状态勿误操作（内容与全局一致，仅类型差异）
