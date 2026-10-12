# 界面文案集中化与合规审核策略（i18n）— 草案

> 状态：**草案（待评审）**｜关联：#2172（H5↔weapp 一致性）
> 目标：全端**界面可见字符串**抽取为**集中文案清单**，实现：(1) **合规审核**——提审前对全部可见文案做敏感词扫描与人工复核（事故：注册页协议勾选文案含『保费』→ 提审失败，被要求提供金融资质证明）；(2) 为未来多语言留好机制（**当前仅需中文，不做翻译**）。**weapp 优先，H5 第二（两端共享清单），管理后台第三**；核心是"如何确保覆盖率（清单完整且与代码一致）"。

## 1. 现状量化（2026-10 实测）

| 端 | 含中文文件 | 中文字符 | 备注 |
|----|-----------|---------:|------|
| frontend-mobile（weapp+H5 共库） | **176** | **31,175** | 最大：FaceVerify 1942 / RepairServiceDetail 1669 / payment(Payment.jsx) 1578 / order-detail 1544 / OrderDetail 1477 / Checkout 1456 |
| frontend-pc | **84** | **22,324** | StaffManagement 1110 / face-review 935 / App.jsx 928；已有集中错误文案表 `services/errorMessages.js`（48 key） |
| 合计 | **260** | **~53,500** | 全人工替换不现实 → 必须**脚本辅助 + 集中拦截 + ESLint 把关** |

**集中拦截点**（改一处覆盖一片，杠杆最高）：

| 拦截层 | 调用数 | 说明 |
|--------|-------:|------|
| mobile `platform` 的 `dialog.alert/toast/confirm` | **374** | 平台层单点包装即可全覆盖 |
| mobile `Taro.showToast/showModal` | **110** | 应收敛到 `dialog`（顺带消灭绕过） |
| mobile `resolveErrorMessage`（含兜底文案） | **145** | 错误文案单点 |
| PC antd `message.*` | **504** | Phase 3 处理 |
| 两端 `*_LABELS` 枚举映射表 | **14 个文件** | 值需抽、内部 key 不抽 |

## 2. 范围定义（覆盖率的"计分规则"，先定死）

**必须抽取（UI 可见）**：JSX 文本、`placeholder/title/label/alt/tooltip`、`dialog.alert/toast/confirm`、`Taro.showToast/showModal`、表单校验消息、`*_LABELS/STATUS_*` 枚举的**显示值**、页面标题、空态文案、确认弹窗文案。

**不抽取（黑名单）**：
- `console.*` / 前端日志；
- **后端返回的 `message`**（服务端文案——第一版原样透传；前端**兜底文案**才抽；见 §6 决策点）；
- 内部 key/枚举名、正则、常量名、路由 path、storage key；
- 注释、README、测试；
- 非用户可见的错误码匹配串（如 `resp.code === 40310`）。

## 3. 技术选型

| 决策 | 选择 | 理由 |
|------|------|------|
| i18n 实现 | **自研极小 `t(key, params)`**（字典查找 + `{name}` 插值 + 缺失回退原串），纯 JS | Taro weapp + H5 双端零风险；不引 react-i18next（包体/兼容/SSR 顾虑） |
| **Key 方案** | **源中文串即 key**（`t('确认支付')`） | 5.3 万规模下语义 key 命名不现实；zh 为源语言，缺失时回退原串=天然兜底不缺文案 |
| 插值 | `{name}` 具名占位：`t('取消失败: {msg}', { msg })` | 消灭 `'取消失败: '+msg` 拼接 |
| 资源组织 | `frontend-mobile/src/i18n/`（`index.js` + `locales/zh-CN.json`）；weapp/H5 **天然共享**（同库双端编译） | PC 独立代码库 → Phase 3 再定（同构复制或共享包） |
| 字典规模 | 起步单文件 `zh-CN.json`（预计 4000–6000 key），>3000 key 后按域拆分 | 避免过早拆分 |
| 语言 | **仅中文**（清单即中文文案全集；`t()` 单 locale，多语言留作机制扩展） | 用户裁定：目的是合规审核，非翻译 |

## 4. 覆盖率保障（核心，四道闸）

### 闸 1 — 抽取脚本（AST，不手抄）
Babel 遍历 `frontend-mobile/src`：产出**清单**（文件×行×串）+ 直接**自动改写**（JSXText/字符串字面量 → `t('…')`），黑名单（§2）与动态拼接（`'a'+x` → `t('a{msg}')`）自动处理；**人工复核 diff**。
- 产出：`scripts/i18n-extract.mjs`（--dry-run / --write / --report）。

### 闸 2 — 集中拦截（一处覆盖一片）
1. `platform/dialog` 的 `alert/toast/confirm` 内部先 `t()` → **374 处零改动即翻译**；
2. `Taro.showToast/showModal` 收敛进 `dialog`（110 处，顺带消灭绕过）；
3. `resolveErrorMessage(result, fallback)` → `t(fallback)` + 已知错误码映射进字典（145 处）；
4. `*_LABELS` 表的值改 `t()`（14 文件）。
> 以上 4 步 ≈ **1100+ 调用点**不经逐个替换即被覆盖。

### 闸 3 — ESLint 残留检测（CI 把关，DoD 的硬指标）
自定义 rule（**保证"清单完整"的闸**：新 UI 中文不经清单即拦截）
自定义 rule `i18n/no-hardcoded-ui-string`：扫 **JSXText + JSX 属性（placeholder/title/label/alt）+ dialog/showModal/message 实参**中的中文字符；
- 黑名单目录/调用豁免（console、注释、测试）；
- **验收口径：按目录残留数 = 0**（脚本出覆盖率报告：`scripts/i18n-report.mjs` → 每目录 `残留/已抽/百分比`）。

### 闸 4 — 敏感词自动扫描（提审前必跑，新增）
- `scripts/copy-audit.mjs`：对**集中清单（zh 字典）**按**可配置敏感词表**扫描（保费/保险/金融/贷款/利息/收益/理财/投资/证券/基金/返利/佣金/分红/提现…可运营维护）；输出 `词 → 文件:行 → 文案` 报告；**提审 checklist 必跑，命中=人工复核**。
- 首扫基线（2026-10-12）：UI 文案命中仅 **1 处**——`pages/Register.jsx:217`「如果继续支付保费，代表已接受并同意」（提审失败根因，措辞待业务/法务修正）；其余命中均为**注释**（非用户可见）。
- 清单集中后，审核面从 **260 个文件**收敛为**一份清单**；扫描把人工审核从"全量逐条"收敛为"只看命中项"。

### 闸 5 — 字典完整性
- 代码中 `t('K')` 的 K 必须存在于 `zh-CN.json`（脚本校验，防手误改串导致翻译丢失）；
- 反向：字典中未被引用的 key 定期清理；
- 插值参数校验：字典占位符与调用参数一致（脚本）。

**完成定义（DoD，每页/每目录）**：闸 3 残留=0 + 闸 4 通过 + `build:h5`/`build:weapp` 0 + wxml 探针空 + 真机走查（默认语言下**文案与抽取前逐字一致**——i18n 第一版不改任何文案内容）。

## 5. 分阶段推进（对齐"weapp 优先，H5 第二，PC 第三"）

| Phase | 范围 | 交付 | 预估 Issue 粒度 |
|-------|------|------|----------------|
| **0 基建** | `t()` 实现 + zh 字典骨架 + 抽取脚本 + ESLint rule + 覆盖率报告 + `dialog`/`resolveErrorMessage` 集中拦截 | 机制与仪表就绪；**集中拦截即覆盖 ~1100 调用点** | 1–2 个 Issue |
| **1 weapp 核心流程** | 登录/注册/完善资料、首页、商城、我的租赁、结算/支付、订单详情、个人中心（**H5/weapp 共享页一次抽取两端受益**） | 核心流程残留=0 | 按页 8–10 个 |
| **2 mobile 全量 + H5 独有** | 其余 mobile 页面 + `App-H5` + H5 独有页 + 双实现页（Home/Checkout/OrderDetail/Payment/Profile/…）| mobile 残留=0 | 按页 10–12 个 |
| **3 管理后台** | frontend-pc（复用闸 1–3 方法；`message.*` 包装 + `errorMessages.js` 迁移） | PC 残留=0 | 按页 8–10 个 |

> 顺序内每 Issue：抽取该页 → ESLint 0 → 构建 → `zh-CN.json` 提交。**任何时刻可暂停**（残留报告显示剩余），不产生半成品态（未抽页面仍硬编码但功能不变）。

## 6. 决策点（需确认）

| # | 问题 | 建议 |
|---|------|------|
| D-A | ~~目标语言~~ **已定：仅中文**（清单用于合规审核，不做翻译） | 已确认 |
| D-B | 后端返回的 `message` 是否 i18n？ | 第一版**不**（原样透传），前端**兜底文案**抽；后续如需，后端按 Accept-Language 返回 |
| D-C | Key 方案确认：**源串作 key** | 建议 A（理由 §3） |
| D-D | 自研 `t()` vs react-i18next | 建议自研（Taro 兼容 + 包体） |

## 7. 风险

| 风险 | 缓解 |
|------|------|
| 5.3 万字符人工替换不可行 | 闸 1 AST 自动改写 + 人工只复核 diff |
| 机械抽取误伤（非 UI 中文被包） | §2 黑名单 + 闸 3 白名单机制 + 抽取 --dry-run 人工复核 |
| 动态拼接串遗漏 | 闸 1 识别 `'字面'+var` → `t('字面{var}')` |
| 双实现页两端字典不同步 | **同一字典文件**（同库）天然同步；PC 独立 |
| 抽取引入运行时错误 | 每批 `build:h5`/`build:weapp` + 真机走查（文案逐字一致） |
| 翻译质量（后续 en） | 机制与翻译解耦；en 由业务审校后补入 `en-US.json` |

## 8. 与既有约定的一致性

- weapp/H5 共享资源 = `frontend-mobile/src/i18n/*`（同库双端编译，符合 AGENTS「跨端代码复用」原则）；
- `docs/cases/h5-weapp-differences.md` 不受影响（i18n 不改行为，仅文案来源）；
- 每批构建验证沿用既有约定（`build:h5`/`build:weapp` + wxml 探针）。

---
_草案：待评审后按 Phase 0 起步，Phase 1 起按页拆 Issue。_
