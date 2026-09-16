# TuneLoop 设计文档地图

> 本文件是 `docs/` 设计文档体系的结构性索引。它只描述体系的**目标结构**——每份文档的长期角色与权威归属，不承载文档管理的过程与当前状态信息。文档生命周期与治理规则见 `documentation-governance.md`。

## 文档分类体系

`docs/` 中的文档按**职能**归类（而非业务域）：

| 分类 | 角色 | 说明 |
|------|------|------|
| 横切规范 | 跨全业务域的设计真相 | UI / API / DB / 权限 / 需求 |
| 域聚合索引 | 单一业务域导航 | 聚合域的维度索引，指向横切/用例对应章节 |
| 结构化用例 | AI 消费的逐步行为 | YAML front-matter + 步骤表 |
| 集成/专题 | 外部系统或架构专题 | 单一主题设计说明 |
| 运维/交付 | 发布与运维规则 | 部署、账号生命周期 |
| 归档 | 只读历史 | 已闭环的调查报告与镜像 |

## 横切规范（as-built）

| 文档 | 职责 | 权威性 |
|------|------|:---:|
| `features.md` | 功能需求规格 | 需求真相 |
| `ui.md` | UI 设计（页面/交互/路由/组件） | UI 真相 |
| `api.md` | API 接口定义 | API 真相 |
| `database.md` | 数据库表结构 | 数据模型真相 |
| `permissions.md` | 权限-人员矩阵 | 权限真相 |

## 域聚合索引

| 文档 | 业务域 | 说明 |
|------|--------|------|
| `repair.md` | 维修 | 维修域全维度索引（数据模型→database / API→api / 页面→ui / 流程→cases） |
| `features/membership.md` | 会员与促销 | 会员域全维度索引（数据模型→database / 权限→permissions / 页面→ui） |
| `documentation-governance.md` | 文档治理 | 本文档体系的管理规则（生命周期/归档/迁移计划） |

## 结构化用例

| 文档 | 说明 |
|------|------|
| `cases/README.md` | 用例目录（域前缀：B/I/L/R/O/T/C）与编号规范 |
| `cases/*.md`（16 文件） | 各业务域逐步行为用例（权威） |
| `cases.md` | 用例合集入口索引 |

## 集成与专题

| 文档 | 主题 |
|------|------|
| `weapp.md` | 微信小程序架构与部署（Taro） |
| `wechat-login.md` | 微信登录架构（三通道/身份合并） |
| `wechat-pay-integration.md` | 微信支付集成架构 |
| `media_directory.md` | 媒体存储架构（instrument_media / media_assets） |
| `iam.md` | IAM 权威文档（symlink → beaconiam README，禁本地修改） |
| `iam-notes.md` | tuneloop 侧 IAM 过渡记录 |
| `oss.md` | 阿里云 OSS 媒体存储迁移（#1914） |
| `frontpage.md` | 首页实现技术文档 |
| `account-lifecycle.md` | 账户生命周期与数据完整性 |
| `release-checklist.md` | 发布检查清单 |

## 用例与工艺子目录

| 目录 | 内容 |
|------|------|
| `page_design/` | 页面设计稿（home/cart/checkout/instrument/profile） |
| `review/` | 测试用例 tc-*（按 Issue 归集） |
| `test-cases/` | 专项测试设计（如 settlement-tdd） |
| `templates/` | 批量导入模板 CSV |
| `deploy/` | 部署说明（如 beaconiam-deployment） |

## 归档

| 位置 | 内容 |
|------|------|
| `_archive/`（规划） | 已闭环调查报告 + 旧英文镜像 `en/`（分类规则见 `documentation-governance.md` §3） |
| 历史报告 | 随迁移计划逐步移入（见 `documentation-governance.md` §5） |