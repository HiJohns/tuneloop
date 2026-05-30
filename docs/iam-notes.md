# 🛡️ BeaconIAM 系统设计文档 (V1.0)

## 一、 系统架构与设计哲学

### 1.1 设计原则

* **Identity-first (身份优先)**：废除固定 `admin` 账号，所有操作权限基于 `User + Role + Organization`。
* **Delegated Admin (分权管理)**：贯彻“管人不管事”，支持组织内部自治。
* **Lightweight & Embedded (轻量内嵌)**：后端 Go 二进制文件内嵌前端静态资源，单文件即可运行。
* **Multi-organization Ready (多组织就绪)**：支持命名空间隔离，每个命名空间拥有独立的品牌定制和应用生态。

### 1.2 系统架构图

系统由 **Core Engine (Go)**、**Embedded UI (React)** 和 **Database (PG/SQLite)** 组成。

---

## 二、 核心数据模型

### 2.1 命名空间 (Clients/Namespaces)

命名空间是顶级逻辑隔离单元，每个命名空间对应一个独立应用生态。

* `client_id`: 命名空间唯一标识（如：`tuneloop`）
* `client_secret`: 命名空间秘钥（用于 OAuth 认证）
* `old_secret`: 轮换期间的旧秘钥（10分钟有效期）
* `css_style`: 自定义 CSS 样式（支持品牌深度定制）
* `is_active`: 启用状态（软删除标志）

### 2.2 组织 (Organizations)

> **v1.2 术语说明**：在 Issue #138 架构设计中，"租户"和"组"都是 Organization 实体的不同形态：

> 此文件为 tuneloop 对 beaconiam API 的补充说明和过渡期记录。
> IAM 权威文档请直接阅读 ../beaconiam/README.md（通过 docs/iam.md symlink）。

[Line 32] > **v1.2 术语说明**：在 Issue #138 架构设计中，"租户"和"组"都是 Organization 实体的不同形态：
[Line 33] > - **租户**（Tenant）：顶级组织，`parent_id` 为空的组织记录
[Line 34] > - **组**（Group）：下级组织，`parent_id` 非空的组织记录
[Line 487] > **v1.2 扩展**：根据 Issue #138 架构设计，JWT Payload 已扩展。
[Line 505] > **说明**：`tid` 和 `gid` 均来自 `organizations` 表。租户是顶级组织（parent_id 为空），组是下级组织（parent_id 非空）。
[Line 522] > **设计决策（#651）**：废除 `tenants` 表 `default:gen_random_uuid()`。所有 ID 字段均为 IAM org 体系中的 UUID，无本地标识符。管理页面同步 IAM 组织时按需创建租户记录。
[Line 570] ⚠️ JWT Payload 不加密，严禁存放敏感信息。
[Line 574] > ⚠️ 客户端开发必读：以下 API 的行为存在关键差异，错误调用会导致静默失败。
[Line 594] > ⚠️ beaconiam 当前实现使用 query param，[beaconiam#313](https://github.com/HiJohns/beaconiam/issues/313) 计划改为 JSON body。接入方当前应发送 query param（或同时发送两者过渡）。
