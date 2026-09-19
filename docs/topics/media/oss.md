# 阿里云 OSS 媒体存储迁移（#1914）— 资源与进度

> 权威计划：GitHub Issue [#1914](https://github.com/HiJohns/tuneloop/issues/1914)（分阶段 P0-P6、风险与对策、验收标准以该 Issue 为准）。本文档记录 P0 资源申请状态、环境决策与环境变量契约。

## 1. 决策记录（动工前 5 项）

| # | 决策点 | 结论 |
|---|--------|------|
| 1 | 读权限 | 业务素材**公开读**；核身等敏感素材**私有读**（签名 URL，独立私有 bucket） |
| 2 | 域名/CDN | 先直连 bucket 域名开发联调；自定义域名 `img.*.cadenzayueqi.com` CNAME + HTTPS 证书后补（见 §3） |
| 3 | 配置方式 | AK/SK 放各环境 `.env`（不进仓库/聊天）；后续再评估 #1910 式 UI 加密配置 |
| 4 | 写入策略 | 双写（本地+OSS）→ 历史回填 → 切读 → 观察期（计划 P3-P6 推荐路线） |
| 5 | 敏感素材 | **同批迁移**，独立私有 bucket + 私有读（修复核身素材现公开可猜路径的安全问题） |

## 2. 资源清单（P0 现状）

**Region：华北2（北京）`oss-cn-beijing`**

| Bucket | 环境 | ACL | 用途 | 直连域名 |
|--------|------|-----|------|----------|
| `tuneloop-media` | 生产 | 公共读 | 业务图/视频 | `tuneloop-media.oss-cn-beijing.aliyuncs.com` |
| `tuneloop-media-sec` | 生产 | 私有 | 核身素材 | `tuneloop-media-sec.oss-cn-beijing.aliyuncs.com` |
| `tuneloop-media-pre` | 预生产 | 公共读 | 业务图/视频 | `tuneloop-media-pre.oss-cn-beijing.aliyuncs.com` |
| `tuneloop-media-sec-pre` | 预生产 | 私有 | 核身素材 | `tuneloop-media-sec-pre.oss-cn-beijing.aliyuncs.com` |

- ✅ 四个 bucket 已创建并按上表设置 ACL
- ⚠️ **预生产与生产物理隔离**（与 DB `tuneloop_pre_snapshot`/`tuneloop`、uploads 目录隔离策略一致），杜绝跨环境误删/串写

### Endpoint 一览（以 `tuneloop-media` 控制台属性为准，其余 bucket 同构）

| 访问端口 | Endpoint | Bucket 域名 | 用途 |
|---------|----------|------------|------|
| 外网 | `oss-cn-beijing.aliyuncs.com` | `tuneloop-media.oss-cn-beijing.aliyuncs.com` | **默认**：后端读写 + 用户/小程序访问 URL |
| CNAME 域名 | `cn-beijing.taihangcda.cn` | `tuneloop-media.cn-beijing.taihangcda.cn` | 专属云 CNAME 端点（⚠️ 非标准公有云行）；**绑定自定义域名时以控制台「域名管理」给出的 CNAME 目标为准** |
| ECS 经典网络/VPC（内网） | `oss-cn-beijing-internal.aliyuncs.com` | `tuneloop-media.oss-cn-beijing-internal.aliyuncs.com` | 仅当 cadenza 为同 region 阿里云 ECS 时用于服务端上传/回填（免外网流量费） |
| 传输加速 | 未开启 | — | 暂不需要 |

**GetURL 输出原则**：用户可见 URL 一律用**外网 bucket 域名**（或绑定的自定义域名），**绝不用内网 endpoint**（客户端不可达）。服务端上传 endpoint 通过 env 可选内网优化（仅 ECS 同 region 部署时）。

### P0 待办（剩余）

| # | 事项 | 状态 |
|---|------|------|
| 1 | RAM 用户 ×2（`tuneloop-oss-prod` / `tuneloop-oss-pre`）+ 最小权限策略（各自 bucket 的 Put/Get/Delete/List） | ✅ 已创建 **且权限实测通过（2026-09-16）**：四桶 RAM 读写删全 OK、匿名层 media=公共读/sec=私有；**ECS 实例角色 `tuneloop-oss-prod-role` 已创建 + 策略 `TuneLoopOSSProdMinimal` v2 + 授予 cadenza 实例 → 2026-09-17 生产两桶零 AK 冒烟 ALL PASS（见 §4.2 / §4.3）** |
| 2 | AccessKey 已保存至本机 `./oss-accounts.md`（**已 .gitignore，严禁提交**；仅用于本地 dev 联调与兜底） | ✅ |
| 3 | 微信公众平台 downloadFile 合法域名（单 appid `wxcb44a1be70e356ed`，加 4 个直连域名或绑自定义域名后加 `img*` 域） | ⏳ 待办 |
| 4 | 自定义域名 CNAME + 所有权验证 + HTTPS 证书（可后补，见 §3） | ⏳ 可选后补 |

### 2.1 凭证策略（#1914 P1，2026-09-16 决策）

> 阿里云建议优先 STS。本项目落地采用**更彻底的 ECS 实例角色方案**：cadenza（生产+预生产同机）为阿里云 ECS，服务端凭据由 SDK 经 metadata 自动获取**临时凭证并自动轮换，零 AK 落盘**。

**凭证解析顺序（`services/oss_credentials.go resolveCredentialsProvider`，2026-09-17 核对实现）**：
1. **环境变量 AK 优先**（`OSS_ACCESS_KEY_ID` 存在即用——预生产 / 本地 dev 联调走各自 RAM key）
2. **ECS 实例 RAM 角色**（env AK 缺省时经 metadata 取临时 STS，自动轮换，零 AK 落盘——生产 cadenza）
3. 均无 → 启动 WARN + 回退 `LocalStorage`

> ⚠️ **顺序订正**：实现是 env AK 存在时**优先 env AK**，仅在无 AK 时才用实例角色——这正是**同机双环境隔离**的实现方式：预生产 `.env` 有 `tuneloop-oss-pre` AK → 走 AK；生产 `.env` 不填 AK → 落实例角色（角色仅授生产两桶）。本节旧文曾误写为「角色优先」，已按实现订正。

> ⚠️ **阿里云 ECS 元数据地址是 `100.100.100.200`（不是 `.100`）**：`.100` 对所有路径返回 404，会导致生产取不到角色凭证并静默回退本地。`services/oss_credentials.go` 与 `tools/oss_smoke` 已于 2026-09-17 修正为 `.200`（`100.100.100.100` 残留 → REJECT 级 bug）。

**生产角色（2026-09-17 落地）**：`acs:ram::1761850082287035:role/tuneloop-oss-prod-role`（普通服务角色，可信实体 ECS），附加自定义策略 `TuneLoopOSSProdMinimal`（**v2**）；已授予 cadenza 实例，生产两桶零 AK 冒烟 **ALL PASS**（见 §4.2 / §4.3）。

**RAM 用户角色降级**：仅本地开发联调 + 应急兜底；**不承载生产/预生产服务端运行时**。STS AssumeRole 不引入（实例角色内部即 STS 且自动续期）。

## 3. 自定义域名 CNAME 计划

**⚠️ 注意：OSS endpoint 域名后缀是 `aliyuncs.com`，不是 `aliyuncs.cn`**（此前讨论中的 `.cn` 为笔误）。

| 自定义域名 | CNAME 目标 |
|-----------|-----------|
| `img.cadenzayueqi.com` | `tuneloop-media.oss-cn-beijing.aliyuncs.com` |
| `imgsec.cadenzayueqi.com` | `tuneloop-media-sec.oss-cn-beijing.aliyuncs.com` |
| `preimg.cadenzayueqi.com` | `tuneloop-media-pre.oss-cn-beijing.aliyuncs.com` |
| `preimgsec.cadenzayueqi.com` | `tuneloop-media-sec-pre.oss-cn-beijing.aliyuncs.com` |

生效三步（每个域名）：
1. DNS 服务商添加 CNAME 记录（上表）；
2. OSS 控制台对应 bucket → 传输配置 → 域名管理 → **添加并验证**自定义域名（阿里云会要求域名所有权验证）；
3. HTTPS 证书：上传证书至该 bucket 域名，或经 CDN 接管自动配证书。

**私有 bucket 绑自定义域名后签名 URL 换用自定义域签发**；未绑前签名 URL 用直连域名即可工作。开发联调**不依赖** CNAME——先用直连域名。

## 4. 环境变量契约（P2，实现 `OSSStorage` 时落地）

工厂 `services/media_storage.go NewMediaStorage()` 现有切换条件 `OSS_ENDPOINT + OSS_BUCKET`（#1914 P1/P2 扩展为下表）：

| 变量 | 含义 | 示例 |
|------|------|------|
| `OSS_ENDPOINT` | endpoint | `oss-cn-beijing.aliyuncs.com` |
| `OSS_BUCKET` | 公开业务 bucket | `tuneloop-media` |
| `OSS_REGION` | region | `cn-beijing` |
| `OSS_ACCESS_KEY_ID` / `OSS_ACCESS_KEY_SECRET` | RAM AK（可选——dev 联调；生产走实例角色） | （各环境 `.env` 可选填写） |
| `OSS_SIGNED_URL_TTL` | 私有签名 URL 有效期秒（默认 900） | `900` |
| `OSS_CDN_PREFIX` | 公开读域名前缀（可空=直连） | `https://img.cadenzayueqi.com` |
| `OSS_PRIVATE_BUCKET` | 敏感素材私有 bucket（可空=不启用私有区） | `tuneloop-media-sec` |
| `OSS_PRIVATE_CDN_PREFIX` | 私有区签名 URL 域名前缀（可空=直连） | `https://imgsec.cadenzayueqi.com` |

**回退语义（P2）**：缺任一必填项 → 启动 WARN + 回退 `LocalStorage`（保持本地可跑）；`OSS_ENDPOINT/OSS_BUCKET` 同时为空 = 显式本地模式。凭据解析顺序：env AK → ECS 实例角色（见 §2.1）。

> ⚠️ **强制 HTTPS（冒烟实证 2026-09-16）**：四个桶均拒绝明文 HTTP（HTTP 请求返回误导性的 `403 because of bucket acl`，HTTPS 正常）。SDK endpoint **必须带 `https://`**（实现已在 `normalizeEndpoint` 强制注入），`.env` 建议也写成 `OSS_ENDPOINT=https://oss-cn-beijing.aliyuncs.com`。

**核身路径约定**：`face_capture_batches` 素材 key 前缀 `face_captures/`（现状同）→ 迁移后写入 `OSS_PRIVATE_BUCKET`，读走签名 URL。

**各环境 `.env` 差异**：生产 `OSS_BUCKET=tuneloop-media` / `OSS_PRIVATE_BUCKET=tuneloop-media-sec`；预生产 `OSS_BUCKET=tuneloop-media-pre` / `OSS_PRIVATE_BUCKET=tuneloop-media-sec-pre` + 各自 AK（两把 RAM key 互不可见对方 bucket）。

### 4.1 各环境 `.env` 配置模板

```ini
# ===== 生产 /opt/tuneloop/apps/tuneloop/.env =====
OSS_ENDPOINT=https://oss-cn-beijing.aliyuncs.com
OSS_REGION=cn-beijing
OSS_BUCKET=tuneloop-media
OSS_PRIVATE_BUCKET=tuneloop-media-sec
# 生产凭据走 ECS 实例角色（不填 AK；见 §4.2）
# OSS_ACCESS_KEY_ID=/OSS_ACCESS_KEY_SECRET=（不设置）

# ===== 预生产 /opt/tuneloop-pre/apps/tuneloop-pre/.env =====
OSS_ENDPOINT=https://oss-cn-beijing.aliyuncs.com
OSS_REGION=cn-beijing
OSS_BUCKET=tuneloop-media-pre
OSS_PRIVATE_BUCKET=tuneloop-media-sec-pre
# 预生产凭据用 pre RAM 用户（tuneloop-oss-pre 的 AK）
OSS_ACCESS_KEY_ID=<tuneloop-oss-pre 的 ID>
OSS_ACCESS_KEY_SECRET=<tuneloop-oss-pre 的 Secret>

# ===== 本地开发 backend/.env（可选，联调预生产桶） =====
# OSS_ENDPOINT=https://oss-cn-beijing.aliyuncs.com
# OSS_REGION=cn-beijing
# OSS_BUCKET=tuneloop-media-pre
# OSS_PRIVATE_BUCKET=tuneloop-media-sec-pre
# OSS_ACCESS_KEY_ID=<tuneloop-oss-pre 的 ID>
# OSS_ACCESS_KEY_SECRET=<Secret>
# 不配置以上任意项 = 本地 LocalStorage 模式
```

### 4.2 ECS 实例角色配置指南（生产）✅ 已完成（2026-09-17）

> 背景：生产（+预生产同机）cadenza 为阿里云 ECS。按 §2.1，**生产服务凭据 = ECS 实例角色（零 AK 落盘）**；预生产用 env AK——**实例角色按“实例”粒度绑定、无法按进程区分**，故角色只授予**生产两桶**权限，达到同机双环境权限隔离（实现上靠 env AK 优先，见 §2.1）。

**控制台操作（已完成，供复现/换机参考）**：
1. RAM 控制台 → 角色 → 创建角色：**可信实体「阿里云服务」→ 服务类型 ECS**（普通服务角色），名称 `tuneloop-oss-prod-role`；
2. 创建/附加**最小权限策略**（自定义，名称建议 `TuneLoopOSSProdMinimal`），内容见下；**注意必须同时包含桶级 ARN 与对象级 `bucket/*` ARN**——只写桶级 ARN 会导致 Put/Get/Delete 全部「资源无匹配操作」而 403；
3. ECS 控制台 → 实例（cadenza）→ 更多 → 实例设置 → **授予/修改 RAM 角色** → 选中该角色；无需重启实例，1-2 分钟生效。

**最小权限策略 JSON（`TuneLoopOSSProdMinimal` v2，仅生产两桶）**：

```json
{
  "Version": "1",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "oss:PutObject",
        "oss:GetObject",
        "oss:DeleteObject",
        "oss:ListObjects",
        "oss:AbortMultipartUpload",
        "oss:ListMultipartUploads",
        "oss:ListParts"
      ],
      "Resource": [
        "acs:oss:*:*:tuneloop-media",
        "acs:oss:*:*:tuneloop-media/*",
        "acs:oss:*:*:tuneloop-media-sec",
        "acs:oss:*:*:tuneloop-media-sec/*"
      ]
    }
  ]
}
```

> 动作说明：分片 `Initiate/UploadPart/Complete/UploadPartCopy` 由 `oss:PutObject` 覆盖；`Abort/ListMultipartUploads/ListParts` 单列。`Copy`=Get+Put；`DeletePrefix`=List+Delete。

**验证（绑定后）**：

```bash
# 1) 元数据：应输出角色名（注意是 100.100.100.200）
ssh cadenza 'curl -s -m 5 http://100.100.100.200/latest/meta-data/ram/security-credentials/'
# → tuneloop-oss-prod-role

# 2) 零 AK 冒烟（生产两桶，强制走实例角色）
ssh cadenza 'env -u OSS_ACCESS_KEY_ID -u OSS_ACCESS_KEY_SECRET \
  OSS_ENDPOINT=https://oss-cn-beijing.aliyuncs.com \
  OSS_BUCKET=tuneloop-media OSS_PRIVATE_BUCKET=tuneloop-media-sec /tmp/oss-smoke'
# → credential chain: ECS instance role (metadata) ... ALL PASS (exit 0)
```

**2026-09-17 实测结果**：元数据返回 `tuneloop-oss-prod-role`（ECS owner 账号 `1761850082287035` = 角色 ARN 账号，排除跨账号）；生产两桶零 AK 冒烟 **0 失败 / ALL PASS**（公开上传→匿名读一致；私有上传→签名读→无签名 403；删除；DeletePrefix）。

**踩坑记录（避免重犯）**：
- ❌ 策略只给桶级 ARN（`acs:oss:*:*:tuneloop-media-sec`）→ 对象级动作「资源无匹配操作」→ 403；✅ 必须带 `/*`。
- ❌ 策略漏掉公开桶 `tuneloop-media` → 该桶 ListObjects 报 `The bucket you access does not belong to you`。
- ❌ 元数据 IP 写 `100.100.100.100`（404）；✅ 正确为 `100.100.100.200`。

**回滚**：ECS → 实例 → 更多 → 实例设置 → 授予/修改 RAM 角色 → 清除；或解绑策略。生产 `.env` 不填 AK 时会 WARN 并回退 `LocalStorage`，不影响服务。

### 4.3 冒烟验证（P2）

独立工具 `backend/tools/oss_smoke`（不依赖 services 包，避免 cgo/webp 牵连）：

```bash
# 构建机（本仓库 backend/ 下）
CGO_ENABLED=0 GOOS=linux go build -o /tmp/oss-smoke ./tools/oss_smoke
scp /tmp/oss-smoke cadenza:/tmp/oss-smoke
# cadenza 上执行（预生产桶示例；凭据链自动选择 env AK 或实例角色）
ssh cadenza "OSS_ENDPOINT=https://oss-cn-beijing.aliyuncs.com OSS_BUCKET=tuneloop-media-pre OSS_PRIVATE_BUCKET=tuneloop-media-sec-pre /tmp/oss-smoke"
```

**覆盖项**：公开上传→匿名读内容一致；私有上传→签名 URL 读→无签名 403；Copy；DeletePrefix 清理；退出码 0=ALL PASS。

**2026-09-16 实测结果**：
- 预生产两桶（pre AK）：🔴→🟢 修复后 **ALL PASS**（先因强制 HTTPS 问题 403，见下）
- ⚠️ **强制 HTTPS**：桶拒 HTTP（误导性 `403 bucket acl`）→ endpoint 必须 `https://`；实现 `normalizeEndpoint` 已强制注入

**2026-09-17 实测结果（生产，ECS 实例角色，零 AK）**：
- 生产两桶 `tuneloop-media` + `tuneloop-media-sec`：**ALL PASS**（`credential chain: ECS instance role (metadata)`，exit 0）
- 工具同轮修正元数据 IP（`.100`→`.200`，见 §4.2 踩坑记录）

## 5. 阶段路线图（摘要，详见 #1914）

| 阶段 | 内容 | 状态 |
|------|------|:---:|
| P0 | OSS 开通 / bucket / RAM-AK / 域名 / 微信白名单 | ✅ 完成（权限实测通过，2026-09-16）；**ECS 实例角色 + 策略 v2 + 实例绑定完成（2026-09-17）**；微信白名单/CNAME 待办 |
| P1 | `OSSStorage` 实现（凭证链、分片、幂等删除、私有签名）+ 单测 | ✅ 完成（`5b7051e4`）；元数据 IP 修正（`.200`，2026-09-17） |
| P2 | env 配置与缺项回退 + 冒烟 | 🔶 实现✅；冒烟✅（预生产桶 + **生产桶实例角色均 ALL PASS**）；**.env 落地待运维** |
| P3 | 双写开关（OSS 写失败显式报错，可配置阻断） | ⏳ |
| P4 | CLI `--migrate-media-oss`（dry-run 先行、幂等、断点续传、失败清单） | ✅ 已实现（#1991） |
| P5 | `GetURL` 切读开关 + nginx `/uploads/*` 未命中重定向兜底 | ⏳ |
| P6 | 观察期 → 停本地写 / `gc-media` 适配 / 更新 `docs/topics/media/media_directory.md` | ⏳ |

### 5.1 P4 回填 CLI 用法（#1991）

```bash
# 预检（不写 OSS）：扫描 + 幂等判定 + 对账报告
./service/tuneloop --migrate-media-oss --dry-run

# 真实回填（并发 4；失败清单落 tmp/oss_backfill_failures.jsonl）
./service/tuneloop --migrate-media-oss --oss-concurrency 4

# 断点续跑（仅重跑失败清单内的 key）
./service/tuneloop --migrate-media-oss --oss-resume tmp/oss_backfill_failures.jsonl

# 覆盖 size 不一致对象（默认报错跳过）
./service/tuneloop --migrate-media-oss --oss-overwrite
```
- 扫描 `uploads/media/**`（key=相对路径，含 `_display.webp`/`_thumb.jpg` 变体）+ `uploads/batch/**`（key=`batch/...` 导入暂存）；
- **幂等判据 = size 一致**（ETag 因分片不稳定不作主判据）；上传后再次 `Stat` 校验；
- **私有前缀**：`face_captures/**` 经 `MediaStorage` 路由到私有桶（`OSS_PRIVATE_BUCKET`）；
- **对账**：报告含 `media_assets` 引用但本地缺失的 key 清单 + 未引用（orphan）计数；
- ⚠️ 真实回填依赖 #1990 部署（ECS 角色凭证生效）。

## 6. 关键约束（迁移红线）

- `storage_key` 语义不变：key 不换、DB 不改，仅换存储后端与 `GetURL` 输出；
- OSS 写失败**不得静默**（显式报错/阻断，红线规则）；
- 回滚 = 切 `GetURL` 开关回本地（本地文件在观察期内保留为冷备）；
- 核身素材迁移完成后必须验证**未登录不可访问**。
