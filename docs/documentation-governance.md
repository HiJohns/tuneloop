# 文档管理体系设计（Documentation Governance）

> 本文档定义 TuneLoop 项目 `docs/` 设计文档体系的结构、生命周期与治理规则。
> 配套产物：`docs/README.md`（文档地图，面向读者的结构性索引）。
> 最后更新：2026-09-16

---

## 1. 文档分类体系

`docs/` 中每一份文档按其在体系中的**长期角色**归类，分类键是「文档职能」，而非业务域：

| 分类 | 角色 | 职责 | 当前文档 |
|------|------|------|---------|
| **横切规范** | as-built | 跨全业务域的设计真相（UI / API / DB / 权限 / 需求） | `ui.md`、`api.md`、`database.md`、`permissions.md`、`features.md` |
| **域聚合索引** | 域导航 | 聚合单一业务域的全维度设计，降级为索引后指向横切/用例对应章节 | `repair.md`、`features/membership.md`、`documentation-governance.md`（本文档） |
| **结构化用例** | cases | 面向 AI 消费的逐步行为用例（YAML front-matter + 步骤表） | `docs/cases/`（16 文件，权威） |
| **集成/专题** | 专题 | 单一外部系统或架构专题的设计说明 | `weapp.md`、`wechat-login.md`、`wechat-pay-integration.md`、`media_directory.md`、`iam.md`(symlink)、`iam-notes.md`、`oss.md`、`frontpage.md` |
| **运维/交付** | 运维 | 发布、部署、账号生命周期等运维规则 | `release-checklist.md`、`account-lifecycle.md` |
| **归档** | 历史 | 已闭环的调查/报告/镜像，只读 | `docs/en/`（14 个 .md）、28 份历史报告 |

**用例与工艺子目录**（不参与本文治理，维持现状）：`docs/page_design/`、`docs/review/`、`docs/test-cases/`、`docs/templates/`、`docs/deploy/`。

## 2. 文档生命周期状态机

按项目最高规约（`prompts/checks/core-doc-check.md`、`prompts/checks/design-doc-coverage.md`），**设计文档是开发的输入与唯一真相，文档变更必须先于代码变更**。因此状态迁移是「文档前置式」，不存在「落地后移交」：

```
需求产生
   ↓
① docs 变更    设计知识写入体系文档（横切 as-built 或域聚合索引）
   ↓
② /analyze     core-doc scan：确认文档已覆盖任务，不足则 REJECT 补文档
   ↓
③ /work        开发参照体系文档实现
   ↓
④ /review      design-doc-coverage 审计：code-beyond-docs 检查
   ↓
⑤ as-built     体系文档随实现持续更新 = 现状真相
```

**语义要点**：

- 知识入体系文档发生在**开发之前**，而非功能落地之后。
- 任何「聚合蓝本」（如 `repair.md` 早期的待建端点）都不是独立生命周期态——其内容应逆迁移拆入横切/用例文档，聚合文档**降级为域索引**（保留但不承载权威知识）。
- `_archive` 不在此状态机内，是终态的只读归宿。

## 3. 归档规则

待归档文档按内容性质分两类判定，判定规则独立于人工逐个判断：

| 类别 | 判定依据 | 判定动作 |
|------|---------|---------|
| 项目设计信息（含迁移/系统结构/状态机等） | 其内容是否已被现行规范文档吸收 | 全部被吸收 → 归档；部分被吸收 → 保留未吸收部分，登记补迁 |
| 调查报告类（非项目设计要素） | 对应 GitHub Issue 是否已闭环 | Issue 已 done/closed → 归档；未闭环 → 保留并关联 Issue |

## 4. 当前文档状态盘点

> 本表为 **as-is**（过渡状态）快照，仅存于此设计文档；`docs/README.md` 不承载状态信息。

| 文档 | 分类角色 | 当前状态 | 最后协同更新 |
|------|---------|---------|:---:|
| `ui.md` | 横切规范 | 现状（编号结构病已登记，见 §5） | 2026-09-13 |
| `api.md` | 横切规范 | 现状（编号重复病已登记，见 §5） | 2026-09-13 |
| `database.md` | 横切规范 | 现状（维修域/会员域表存在空洞） | 2026-09-10 |
| `permissions.md` | 横切规范 | 现状（会员 5 个 cus_perm bit21-25 未收录） | 2026-09-13 |
| `features.md` | 横切规范 | 现状 | 2026-09-14 |
| `repair.md` | 域聚合 | **待逆迁移**：拆入 database/api/ui/cases，降级为域索引 | 2026-09-11 |
| `features/membership.md` | 域聚合 | **待逆迁移**：拆入 database/permissions/ui，降级为域索引 | （同 features.md 体系，未见独立提交） |
| `cases.md` | 用例合集 | **待降级为索引**：主体向 `docs/cases/` 收敛 | 2026-09-11 |
| `docs/cases/`（16 文件） | 结构化用例 | ✅ 权威（#1583） | — |
| `weapp.md` | 集成/专题 | ✅ 现状 | 2026-08-29 |
| `wechat-login.md` | 集成/专题 | ✅ 现状 | 2026-08-07 |
| `wechat-pay-integration.md` | 集成/专题 | ✅ 现状 | 2026-09-14 |
| `media_directory.md` | 集成/专题 | ✅ 现状 | 2026-08-29 |
| `iam.md` | 集成/专题 | ✅ symlink → beaconiam README，禁本地修改 | 2026-05-30 |
| `iam-notes.md` | 集成/专题 | ✅ 现状 | 2026-08-21 |
| `oss.md` | 集成/专题 | ✅ 现状 | 2026-09-15 |
| `frontpage.md` | 集成/专题 | ✅ 现状 | 2026-09-08 |
| `release-checklist.md` | 运维/交付 | ✅ 现状 | 2026-08-12 |
| `account-lifecycle.md` | 运维/交付 | ✅ 现状 | 2026-06-06 |
| `docs/en/`（14 个 .md） | 归档 | **待决策**：与主文已失同步的镜像快照 | — |
| 28 份历史报告 | 归档 | **待归档**：按 §3 规则逐份判定 | — |

## 5. 迁移计划表

按「一步一步落实」原则，以下项为后续 Step，逐项登记、逐项消化：

| # | 迁移内容 | 目标 | 触发/备注 |
|---|---------|------|----------|
| 1 | `repair.md` 逆迁移 | 表结构→`database.md`；🆕 端点→`api.md` §7；页面→`ui.md` §2.9；流程→`docs/cases/repair.md`；`repair.md` 降级为域索引 | 纯文档任务，不进入 /analyze→/work 流水线 |
| 2 | `features/membership.md` 逆迁移 | 表结构→`database.md`；cus_perm bit21-25→`permissions.md`；页面→`ui.md` §2.6；`membership.md` 降级为域索引 | 同上 |
| 3 | `cases.md` 降级 | 主体收敛至 `docs/cases/`，`cases.md` 保留索引 | 参考 §2 语义 |
| 4 | `docs/en/` 处置 | 淘汰 / 重新同步 / 归档 三选一 | 待决策 |
| 5 | 28 份历史报告归档 | 按 §3 两类规则逐份判定 → 移入归档区 | 可脚本辅助 |
| 6 | 编号清理 | `ui.md` 两套 3.x / 附录插位；`api.md` 重复 1.5/2.3/3.1；`cases.md` 重复 4.2 | 随文档变更顺带修正 |

## 6. 文档类任务流程范型

按 `prompts/instructions.md` §5 工作流速查表，**文档/测试类任务「无需进入流水线」**（保持 `status:todo`，产出 Markdown 文档后由用户决定状态）。因此本文档及其配套产物的执行路径是：

```
/todo 登记 Issue（含 ## Core Update 说明，status:todo）
   → 产出设计文档（本文档）
   → 产出产物（docs/README.md）
   → 单 commit
```

不再走 `/analyze → /work → /review`：`/analyze` 的三层合约校验、前端走查等检查模块针对用户可见业务行为，对纯文档产物不适用（`design-doc-coverage.md` Scope limitation 已预留此豁免）。

## 7. 待补充最高规约登记

| 缺口 | 说明 | 处理路径 |
|------|------|---------|
| 「产物不含过程/状态信息」约束 | 最高规约（`prompts/instructions.md`）与检查模块中均无「代码/产物不得含有设计过程与当前状态信息」的显式条款 | 按 `instructions.md` §4.4 配置变更流程，在 `HiJohns/opencode` 仓库立项补充 |

---

*Model: {MODEL_NAME}*