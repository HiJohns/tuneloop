# TuneLoop 设计文档地图

> 本文件是 `docs/` 设计文档体系的结构性索引。它只描述体系的**目标结构**——每份文档的长期角色与权威归属，不承载文档管理的过程与当前状态信息。文档生命周期与治理规则见治理 Issue（`documentation-governance.md` 已迁移为 GitHub Issue，本文档不再保留其本地副本）。

## 目标目录树

> 目标结构一览（纵向树状）。文件物理落地随治理 Issue §5 迁移计划逐步执行，本地图所示为其**最终目标形态**。

```
docs/
├── README.md                        # 文档地图（本文件）
├── blog.md                          # 变更/操作日志（根级，工具链强制绑定，不可迁移）
├── en/                              # 英文镜像区（根级，工具链强制绑定，不可迁移）
│
├── spec/                            # 横切规范
│   ├── features.md                  # 需求真相
│   ├── permissions.md               # 权限真相
│   ├── ui/                          # UI 真相（大文档，前瞻拆分）
│   │   └── ui.md                    # 总入口（页面增多时可拆出分文件）
│   ├── api/                         # API 真相（大文档，前瞻拆分）
│   │   └── api.md                   # 总入口（端点增多时可按域拆分）
│   └── database/                    # 数据模型真相（大文档，前瞻拆分）
│       └── database.md              # 总入口（表增多时可按域拆分）
│
├── domains/                         # 域聚合索引
│   ├── repair.md                    # 维修域索引
│   └── membership.md                # 会员域索引
│
├── cases/                           # 结构化用例
│   ├── README.md                    # 用例目录（域前缀/编号规范）
│   ├── _template.md                 # 用例模板
│   ├── bootstrapping.md ... transit.md   # 16 个用例
│   └── cases.md                     # 用例合集入口索引
│
├── topics/                          # 集成/专题
│   ├── wechat/                      # 微信小程序生态
│   │   ├── weapp.md                 # 架构与部署
│   │   ├── wechat-login.md          # 登录架构
│   │   └── wechat-pay-integration.md # 支付集成
│   ├── media/                       # 媒体存储
│   │   ├── media_directory.md       # 存储架构
│   │   └── oss.md                   # OSS 迁移
│   ├── iam/                         # IAM/身份认证
│   │   ├── iam.md                   # symlink → beaconiam README
│   │   └── iam-notes.md             # tuneloop 侧补充
│   ├── frontpage.md                 # 首页技术文档（待同类主题扩大再建子目录）
│
├── ops/                             # 运维/交付
│   ├── release-checklist.md         # 发布检查清单
│   └── account-lifecycle.md         # 账户生命周期与数据完整性
│
├── archive/                         # 归档（终态只读）
│   ├── reports/                     # 已闭环调查报告
│   ├── audit/                       # 历史审计记录与产物（见治理 Issue §1.1）
│   └── plans/                       # 已闭环 Issue 执行计划（见治理 Issue §1.1）
│
├── page_design/                     # 页面设计稿
├── review/                          # 测试用例 tc-*
├── test-cases/                      # 专项测试设计
├── templates/                       # 批量导入模板 CSV
├── deploy/                          # 部署说明
└── dummy/                           # 示例/临时数据
```

## 文档分类体系

`docs/` 中的文档按**职能**归类（而非业务域），每类对应一个目标目录：

| 分类 | 目标目录 | 角色 | 说明 |
|------|---------|------|------|
| 横切规范 | `spec/` | 跨全业务域的设计真相 | UI / API / DB / 权限 / 需求 |
| 域聚合索引 | `domains/` | 单一业务域导航 | 聚合域的维度索引，指向横切/用例对应章节 |
| 结构化用例 | `cases/` | AI 消费的逐步行为 | YAML front-matter + 步骤表 |
| 集成/专题 | `topics/` | 外部系统或架构专题 | 单一主题设计说明，按主题子目录聚集 |
| 运维/交付 | `ops/` | 发布与运维规则 | 部署、账号生命周期 |
| 工具链绑定 | `docs/` 根（blog.md / en/） | 会话启动强制读 + 脚本维护 | 变更/操作日志与英文镜像，**不可迁移**（`scripts/opencode_gh.sh`、`scripts/prepare.sh` 及会话启动引用其原位置） |
| 归档 | `archive/` | 只读历史 | 已闭环的调查报告 |

## 横切规范（as-built）

目标目录：`spec/`。大文档（ui / api / database）各建子目录，单文件作总入口，未来拆分分文件不破坏引用；小文档（features / permissions）暂留 `spec/` 根。

| 文档 | 职责 | 权威性 |
|------|------|:---:|
| `spec/features.md` | 功能需求规格 | 需求真相 |
| `spec/ui/ui.md` | UI 设计（页面/交互/路由/组件） | UI 真相 |
| `spec/api/api.md` | API 接口定义 | API 真相 |
| `spec/database/database.md` | 数据库表结构 | 数据模型真相 |
| `spec/permissions.md` | 权限-人员矩阵 | 权限真相 |

## 域聚合索引

目标目录：`domains/`。

| 文档 | 业务域 | 说明 |
|------|--------|------|
| `domains/repair.md` | 维修 | 维修域全维度索引（数据模型→spec/database / API→spec/api / 页面→spec/ui / 流程→cases） |
| `domains/membership.md` | 会员与促销 | 会员域全维度索引（数据模型→spec/database / 权限→spec/permissions / 页面→spec/ui） |

> `documentation-governance.md` 治理规则已迁移至 GitHub Issue（仓库 `HiJohns/tuneloop`，见提交历史 / 文档治理入口），不再于 `docs/` 内保留文件副本。

## 结构化用例

目标目录：`cases/`。

| 文档 | 说明 |
|------|------|
| `cases/README.md` | 用例目录（域前缀：B/I/L/R/O/T/C）与编号规范 |
| `cases/cases.md` | 用例合集入口索引 |
| `cases/_template.md` | 用例模板 |
| `cases/*.md`（16 个用例） | 各业务域逐步行为用例（权威） |

## 集成与专题

目标目录：`topics/`，按**主题域**建子目录：

| 文档 | 主题 |
|------|------|
| `topics/wechat/weapp.md` | 微信小程序架构与部署（Taro） |
| `topics/wechat/wechat-login.md` | 微信登录架构（三通道/身份合并） |
| `topics/wechat/wechat-pay-integration.md` | 微信支付集成架构 |
| `topics/media/media_directory.md` | 媒体存储架构（instrument_media / media_assets） |
| `topics/media/oss.md` | 阿里云 OSS 媒体存储迁移（#1914） |
| `topics/iam/iam.md` | IAM 权威文档（symlink → beaconiam README，禁本地修改） |
| `topics/iam/iam-notes.md` | tuneloop 侧 IAM 过渡记录 |
| `topics/frontpage.md` | 首页实现技术文档 |

## 工具链绑定文档（根级，不可迁移）

目标目录：`docs/` 根。**保持原位、不参与落位迁移**——`scripts/opencode_gh.sh`（append 变更日志）、`scripts/prepare.sh`（mkdir+touch `en/blog.md`）及会话启动强制读（`prompts/instructions.md`）均按原位置引用，移动即失效。

| 文档 | 内容 |
|------|------|
| `blog.md` | 变更/操作日志 |
| `en/blog.md` | 英文镜像占位（prepare.sh 维护，0 字节） |
| `en/` | 英文镜像区（保留原位，不归 `archive/`） |

## 运维/交付

目标目录：`ops/`。

| 文档 | 主题 |
|------|------|
| `ops/release-checklist.md` | 发布检查清单 |
| `ops/account-lifecycle.md` | 账户生命周期与数据完整性 |

## 用例与工艺子目录

不参与本文治理，维持现状：

| 目录 | 内容 |
|------|------|
| `page_design/` | 页面设计稿（home/cart/checkout/instrument/profile） |
| `review/` | 测试用例 tc-*（按 Issue 归集） |
| `test-cases/` | 专项测试设计（如 settlement-tdd） |
| `templates/` | 批量导入模板 CSV |
| `deploy/` | 部署说明（如 beaconiam-deployment） |
| `dummy/` | 示例/临时数据 |

## 归档

目标目录：`archive/`，终态只读归宿。

| 位置 | 内容 |
|------|------|
| `archive/reports/` | 已闭环调查报告（按 §3 规则移入，见治理 Issue §5） |
| `archive/audit/` | 历史审计记录与产物：`audit_reject_233.md`、`audit_reject_233_v2.md`、`audit_report_234.md`、`api_response_format.md`（来自 `backend/docs/`） |
| `archive/plans/` | 已闭环 Issue 执行计划：`260.md`、`263.md`（来自 `plans/`） |

## 全仓库文档归属（非 docs/ 目录的散落文档）

文档治理范围 = **全仓库文档资产**（见治理 Issue），非仅 `docs/` 目录：

| 文档 | 归属 |
|------|------|
| `AGENTS.md`、根 `README.md` | 原位保留（核心规约/门面） |
| `prompts/*`、`.opencode/commands/check-flow.md` | 原位保留（全局镜像工具链） |
| `frontend-mobile/README.md`、`project.md` | 原位保留（脚手架模板/占位） |
| `audit_reject_233.md`、`audit_reject_233_v2.md`、`audit_report_234.md`、`backend/docs/api_response_format.md` | → `archive/audit/` |
| `plans/260.md`、`plans/263.md` | → `archive/plans/` |
| `.opencode_temp/*`、`.tmp_*` | 保持不动（2026-09-18 决策：不迁移不删除） |