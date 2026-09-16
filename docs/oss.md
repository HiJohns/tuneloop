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
| 1 | RAM 用户 ×2（`tuneloop-oss-prod` / `tuneloop-oss-pre`）+ 最小权限策略（各自 bucket 的 Put/Get/Delete/List） | ✅ 已创建 **且权限实测通过（2026-09-16）**：四桶 RAM 读写删全 OK、匿名层 media=公共读/sec=私有；分片项留待 P1 联调复验；**ECS 实例角色方案已采纳（见 §2.1）** |
| 2 | AccessKey 已保存至本机 `./oss-accounts.md`（**已 .gitignore，严禁提交**；仅用于本地 dev 联调与兜底） | ✅ |
| 3 | 微信公众平台 downloadFile 合法域名（单 appid `wxcb44a1be70e356ed`，加 4 个直连域名或绑自定义域名后加 `img*` 域） | ⏳ 待办 |
| 4 | 自定义域名 CNAME + 所有权验证 + HTTPS 证书（可后补，见 §3） | ⏳ 可选后补 |

### 2.1 凭证策略（#1914 P1，2026-09-16 决策）

> 阿里云建议优先 STS。本项目落地采用**更彻底的 ECS 实例角色方案**：cadenza（生产+预生产同机）为阿里云 ECS，服务端凭据由 SDK 经 metadata 自动获取**临时凭证并自动轮换，零 AK 落盘**。

**凭证解析顺序（OSSStorage 初始化实现）**：
1. **ECS 实例 RAM 角色**（生产/预生产 cadenza 生效；控制台绑定角色一次，无 AK）
2. **环境变量 AK 兜底**（`OSS_ACCESS_KEY_ID/SECRET`，仅本地 dev 联调用 `tuneloop-oss-pre` 这把）
3. 均无 → 启动 WARN + 回退 `LocalStorage`

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

## 5. 阶段路线图（摘要，详见 #1914）

| 阶段 | 内容 | 状态 |
|------|------|:---:|
| P0 | OSS 开通 / bucket / RAM-AK / 域名 / 微信白名单 | 🔶 bucket ✅，RAM ⛔ 阻塞 |
| P1 | `OSSStorage` 实现（SDK v2、分片、幂等覆盖、私有签名）+ fake/mock 单测 | ⏳ 可先行（mock） |
| P2 | env 配置与缺项回退 | ⏳ |
| P3 | 双写开关（OSS 写失败显式报错，可配置阻断） | ⏳ |
| P4 | CLI `--migrate-media-oss`（dry-run 先行、幂等、断点续传、失败清单） | ⏳ |
| P5 | `GetURL` 切读开关 + nginx `/uploads/*` 未命中重定向兜底 | ⏳ |
| P6 | 观察期 → 停本地写 / `gc-media` 适配 / 更新 `docs/media_directory.md` | ⏳ |

## 6. 关键约束（迁移红线）

- `storage_key` 语义不变：key 不换、DB 不改，仅换存储后端与 `GetURL` 输出；
- OSS 写失败**不得静默**（显式报错/阻断，红线规则）；
- 回滚 = 切 `GetURL` 开关回本地（本地文件在观察期内保留为冷备）；
- 核身素材迁移完成后必须验证**未登录不可访问**。
