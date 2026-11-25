## Accepted Plan — #2504 [Core Update Required] 40303 未映射导致英文 `access denied` 直出（PC/mobile 双端解析器收敛 + ASCII 不外溢兜底）

> Tags: bug, code:js, api-contract
> 关联：#2480（错误文案层序 + CJK 透传）/ #2497（解析器）/ #2447（入口可见性）。本单**不改后端消息**（机器码口径，见 D1）。

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| 后端 `personnel.List` 对非 merchant/site/system admin（含 `site_member`）返回 `403/40303` + `"access denied"`（英文硬编码） | `backend/handlers/personnel.go:77,83,94` |
| `40303` 也是全站「权限不足」通用码（`RequireSysPerm`/`RequireCusPerm` 等），后端共 7 处引用 | `backend/middleware/permissions.go:145` + 上述 3 处 + 测试 2 处 |
| PC `ERROR_CODE_MAP` **无 40303** → 落到 L4 `raw_message` → 英文直出 | `frontend-pc/src/services/errorMessages.js:6-34`（map）、`:113-122`（解析：`if (hasMessage(message)) return raw`） |
| mobile `ERROR_CODE_MAP` **无 40303**，纯核心同样有 L4 raw 透传 | `frontend-mobile/src/services/errorMessages.js:11-35`；`frontend-mobile/src/services/errorMessageResolution.mjs:43` |
| 真实链路会把后端 message 经解析器转成 `Error.message`，页面再展示 | `frontend-pc/src/services/api.js:352`；`frontend-pc/src/pages/StaffManagement.jsx:112-113` |
| `/staff` 菜单项已具备角色门控（`requireAll` + sysPermBits[15]），`site_member` 的 sys_perm 为空 → 菜单不展示（仅直链/陈旧菜单可达） | `frontend-pc/src/App.jsx:359`；`frontend-pc/src/config/menuPermissions.js:39-62`；`backend/services/role_templates.go:62`（site_member SysPermBits 为空） |
| 双端解析顺序须完全一致，漂移由平价脚本兜住 | `scripts/check-error-message-parity-2480.mjs`（10 例） |

**结论**：报错文本走「后端机器码 + 前端按 code 映射」是既有架构（#1675/#2480）。本单 (1) 双端补 `40303` 映射；(2) L4 只在 **含 CJK** 时透传原文，ASCII 机器串一律 `code_map`/`fallback`（与 #2480 的 CJK 透传规则对称，堵住「任何未映射 code 都直出英文」的同类漏洞）；(3) 同步翻转受影响的既有断言。

### 语义变更（必须翻转的既有断言，强制清单）

| 文件:行 | 旧期望 | 新期望 | 理由 |
|---|---|---|---|
| `frontend-pc/tests/errorMessages.test.mjs:24`（fixture `UNKNOWN`） | `layer: raw_message` / `text: 'unknown conflict'` | `layer: fallback` / `text: '操作失败，请重试'` | L4 仅透传含 CJK 的可读文案（D2） |
| `frontend-mobile/src/services/errorMessageResolution.test.mjs:21`（fixture `UNKNOWN`） | `raw_message` | `fallback` | 同上（双端同序） |
| `frontend-mobile/src/services/errorMessageResolution.test.mjs:72-74`（专项用例） | `raw_message` / `'unknown conflict'` | `fallback` / `'操作失败，请重试'` | 同上 |
| `scripts/check-error-message-parity-2480.mjs:54`（case `49999+ASCII`） | 仅比对平价（LAYER 会打印 `raw_message`） | 仍比对平价，但须新增断言 `LAYER=fallback`（两端） | 平价脚本是双端唯一交叉校验 |

> 后端 `access denied` **保持不变**（机器码，供前端映射与日志检索）；`docs/spec/api/api.md` 明确「后端 message 为机器码，前端按 code 映射，ASCII 不透传」。

### 变更点

1. `frontend-pc/src/services/errorMessages.js`：
   - `ERROR_CODE_MAP` 增 `40303: '暂无权限访问该页面'`（与 40300/40301 同族但语义更准确）。
   - L4 改为：`if (hasMessage(message) && isHumanReadableMessage(message)) return { text: message, layer: 'raw_message' }`；否则 `fallback`。
2. `frontend-mobile/src/services/errorMessages.js`：`ERROR_CODE_MAP` 增 `40303: '暂无权限访问该页面'`（文案与 PC 逐字一致）。
3. `frontend-mobile/src/services/errorMessageResolution.mjs`：L4 同 PC 守卫（ASCII 不透传）。
4. 测试与平价脚本：
   - 翻转上表 4 处断言；PC/mobile 各新增 `40303 + "access denied"` 用例（`code_map` → 中文）与 `49999 + ASCII` 负例（`fallback`）。
   - `scripts/check-error-message-parity-2480.mjs`：抽取 mobile `40303` 文案、新增 `40303+access denied` / `40303+空串` 用例，并在输出中断言 `49999+ASCII` 两端 `LAYER=fallback`。
5. 文档（Core Update）：`docs/spec/api/api.md` §1.3 用户可见文案注、§14.4 错误码表。
6. 证据：`scripts/verify-2504.sh`（r1–r6 生成 `docs/evidence/2504/R*.txt`）。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| PC 解析器：40303 映射 + ASCII 兜底 | `frontend-pc/src/services/errorMessages.js` | `node --test frontend-pc/tests/errorMessages.test.mjs` | `{code:40303,message:'access denied'}`；`{code:49999,message:'unknown conflict'}` | PC | 前者 `code_map` → 「暂无权限访问该页面」；后者 `fallback` → 「操作失败，请重试」；八条 fixture 层序更新为 `…,fallback,…` | R1（合同 R1） |
| mobile 解析核心同上 | `frontend-mobile/src/services/errorMessageResolution.mjs` | `node --test frontend-mobile/src/services/errorMessageResolution.test.mjs` | 同上 | mobile | 与 PC 逐字同文案、同层序（`L40303=code_map`、`L49999_ASCII=fallback`） | R2（合同 R2） |
| 双端平价（含新码与 ASCII 负例） | `scripts/check-error-message-parity-2480.mjs` | `node scripts/check-error-message-parity-2480.mjs` | 12+ 用例（新增 40303+access denied / 40303+空串） | PC + mobile | `PARITY=1`、`DIFFS=0`；`CASE=49999+ASCII` 两端 `LAYER=fallback` | R3（合同 R3） |
| 旧断言翻转无残留 | PC/mobile 测试 + 平价脚本 | `bash scripts/verify-2504.sh r4` | 静态：检查 4 处翻转点 | PC + mobile | `STALE_RAW_PC=0`、`STALE_RAW_MOBILE=0`、`FLIP_COUNT=4`、`OLD_BEHAVIOR_GONE=1` | R4（合同 R4） |
| 入口可见性 + 真实链路接线（静态） | `frontend-pc/src/services/errorMessages.js`（被门控/接线消费的唯一出口） | 源码结构断言（见 G0 R5 的四个接线点） | `site_member`（sys_perm 空）打开 `/staff` | PC | `/staff` 菜单 `requireAll` + sysPermBits[15] → 菜单隐藏；非 2xx 由 `resolveErrorMessage({code,message})` 转 `Error.message`，页面展示该中文（无 `access denied` 直出） | R5（合同 R5） |
| 构建 + lint | `frontend-pc`、`frontend-mobile` | `npm run build`（PC）+ `npx eslint` 改动文件 | 修后树 | PC + mobile | `PC_BUILD=0`、`LINT_PC=0`、`LINT_MOBILE=0` | R6（合同 R6） |

## Input Truth & Tool Preflight

- **Source of truth**：解析器层序以 `frontend-pc/src/services/errorMessages.js` 与 `frontend-mobile/src/services/errorMessageResolution.mjs` 为准（#2480 唯一口径）；双端一致性以平价脚本为准（不是人工比对）。
- **差异核对**：`grep -c 40303 frontend-pc/src/services/errorMessages.js frontend-mobile/src/services/errorMessages.js` 当前均为 0；`grep -rn "access denied" backend` 确认后端为机器码；`grep -rn "raw_message" frontend-pc/tests frontend-mobile/src/services` 命中 4 处（上表翻转清单）。
- **损坏输入拒绝**：`message` 非字符串/空串/仅空白 → 不进入 raw 层（沿用 `hasMessage`）；`code` 为字符串（如 `"40303"`）→ `codeMap[code]` 兼容 JS 对象键；`result` 为 null → fallback。
- **工具盲区**：PC 无组件测试运行器（`package.json` 无 vitest/@testing-library）→ 解析器用 `node --test` 直接 import 生产模块；页面接线用源码结构断言（`evidence_type: static`，Owner 豁免见 D4）。mobile 侧 `errorMessages.js` 依赖 `../i18n`（JSON 导入），因此测试只覆盖纯核心 `errorMessageResolution.mjs` + 平价脚本抽取 map 文本。
- **影响面预检**：L4 收紧后，**未知 code + 英文 message** 从「原样英文」变为「通用中文 fallback」——这是本单目标；已知高频英文 message 均在 L1 `ERROR_MESSAGE_MAP`（`order not found` 等），不受影响。`raw_message` 仍保留给含 CJK 的后端文案。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 后端 `access denied` 保持英文机器码；用户可见文案由前端按 code 映射（符合 #1675/#2480 架构，避免后端文案成为展示层真源） | Architect | 本计划锁定 | 若 Owner 要求后端改中文 → 回 Architect（需同步 `ERROR_MESSAGE_MAP` 与两端平价） |
| D2 | L4 只在含 CJK 时透传原文；ASCII 机器串走 `code_map` → `fallback`（与 #2480「CJK 优先」对称） | Architect | 本计划锁定（#2504 期望 3） | 若要求保留部分 ASCII 透传 → 必须先登记白名单（新 Issue） |
| D3 | `40303` 文案双端逐字一致：`暂无权限访问该页面` | Architect | 本计划锁定 | 改文案须同时改两端 + 平价脚本断言 |
| D4 | 页面接线行（R5）为纯前端结构断言，无 PC 组件测试运行器 → `evidence_type: static`，**Owner 豁免（Architect 授予）**；真实浏览器走查属部署侧 | Architect | 本计划锁定 | 若审计要求 render → 另立前端测试基建 Issue |
| D5 | `/staff` 菜单已按角色门控（`requireAll` + sysPermBits[15]），本单**不改**菜单/路由 | Architect | 本计划锁定（代码为据） | 若产品要求改为路由级重定向 → 另立 Issue（涉及 #2447） |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R5 | static | 入口门控与真实链路接线的源码结构断言，Owner 豁免见 D4 |

## Out of Scope

- 后端 `personnel.go` / `permissions.go` 的 message 文案（D1）。
- `/staff` 菜单与路由新增/调整（已具备门控，D5）；#2447 的入口优化。
- 其它未映射错误码的全量盘点（本单只补 40303 + ASCII 兜底；后续新增码按 api.md 约定随手补）。
- 小程序真机文案走查、预生产 `site_member` 直链 `/staff` 的人工验收 → 关闭时以 `[Ops-Followup of #2504]`（`waitForConfirm`）承载。
- `frontend-mobile` 打包构建（本单只改解析器/service 模块，未新增模块依赖；由发布 gate 的 `build:h5`/`build:weapp` 覆盖）。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `{code:40303,message:'access denied'}`；`{code:49999,message:'unknown conflict'}`；八条既定 fixture | `node --test frontend-pc/tests/errorMessages.test.mjs` | `docs/evidence/2504/R1-pc-resolver.txt` | `L40303=code_map`、`C40303=暂无权限访问该页面`、`L49999_ASCII=fallback`、`C49999_ASCII=操作失败，请重试`、`LAYERS=human_message,code_map,code_map,human_message,message_map,fallback,fallback,human_message` | 英文 `access denied` 出现在输出 → FAIL |
| R2 | 同上 | `node --test frontend-mobile/src/services/errorMessageResolution.test.mjs` | `docs/evidence/2504/R2-mobile-resolver.txt` | `L40303=code_map`、`C40303=暂无权限访问该页面`、`L49999_ASCII=fallback`、`LAYERS` 与 R1 完全一致 | 层序与 PC 不同 → FAIL |
| R3 | 12+ 平价用例（新增 `40303+access denied`、`40303+空串`） | `node scripts/check-error-message-parity-2480.mjs` | `docs/evidence/2504/R3-parity.txt` | `PARITY=1`、`DIFFS=0`、`CASES>=12`、`CASE=49999+ASCII PARITY=1 LAYER=fallback` | `DIFFS>0` → FAIL |
| R4 | 4 处翻转点（测试/平价脚本） | `bash scripts/verify-2504.sh r4` | `docs/evidence/2504/R4-flip.txt` | `STALE_RAW_PC=0`、`STALE_RAW_MOBILE=0`、`FLIP_COUNT=4`、`OLD_BEHAVIOR_GONE=1` | 仍存在 `raw_message` 的 ASCII 期望 → FAIL |
| R5 | `App.jsx:359` 菜单项、`menuPermissions.js:39-62`、`api.js:352`、`StaffManagement.jsx:112-113` | `bash scripts/verify-2504.sh r5` | `docs/evidence/2504/R5-wiring-static.txt` | `MENU_GATED=1`、`API_RESOLVES=1`、`PAGE_MESSAGE_FROM_RESOLVER=1`、`DIRECT_ENGLISH_LEAK=0` | 页面直出 `error.response.message` 且未经 resolver → FAIL |
| R6 | 修后树 | `bash scripts/verify-2504.sh r6` | `docs/evidence/2504/R6-build-lint.txt` | `PC_BUILD=0`、`LINT_PC=0`、`LINT_MOBILE=0` | 任一非 0 → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2504.sh r1` | `docs/evidence/2504/R1-pc-resolver.txt` | exit 0；grep `L40303=code_map`、`C40303=暂无权限访问该页面`、`L49999_ASCII=fallback`、`LAYERS=human_message,code_map,code_map,human_message,message_map,fallback,fallback,human_message` | PASS | 输出含 `access denied` → FAIL | dev |
| R2 | `bash scripts/verify-2504.sh r2` | `docs/evidence/2504/R2-mobile-resolver.txt` | exit 0；grep `L40303=code_map`、`C40303=暂无权限访问该页面`、`L49999_ASCII=fallback` | PASS | 层序与 PC 不一致 → FAIL | dev |
| R3 | `bash scripts/verify-2504.sh r3` | `docs/evidence/2504/R3-parity.txt` | exit 0；grep `PARITY=1`、`DIFFS=0`、`CASE=49999+ASCII PARITY=1 LAYER=fallback` | PASS | `DIFFS=1` → FAIL | dev |
| R4 | `bash scripts/verify-2504.sh r4` | `docs/evidence/2504/R4-flip.txt` | exit 0；grep `STALE_RAW_PC=0`、`STALE_RAW_MOBILE=0`、`FLIP_COUNT=4`、`OLD_BEHAVIOR_GONE=1` | PASS | `STALE_RAW_PC=1` → FAIL | dev |
| R5 | `bash scripts/verify-2504.sh r5` | `docs/evidence/2504/R5-wiring-static.txt` | exit 0；grep `MENU_GATED=1`、`API_RESOLVES=1`、`PAGE_MESSAGE_FROM_RESOLVER=1`、`DIRECT_ENGLISH_LEAK=0` | PASS | 直出英文分支命中 → FAIL | dev |
| R6 | `bash scripts/verify-2504.sh r6` | `docs/evidence/2504/R6-build-lint.txt` | exit 0；grep `PC_BUILD=0`、`LINT_PC=0`、`LINT_MOBILE=0` | PASS | `PC_BUILD=1` → FAIL | dev |

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2504/R1-pc-resolver.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2504/R2-mobile-resolver.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2504/R3-parity.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2504/R4-flip.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2504/R5-wiring-static.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2504/R6-build-lint.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| # | 缺陷面 symptom | 触发条件 trigger | 根因 root cause | 测试盲区 test blind spot | 同类路径 same-class | 修复层 fix layer | 红绿 red-green | 预防 prevention |
|---|---|---|---|---|---|---|---|---|
| B1 | `site_member` 打开人员管理看到英文 `access denied` | 非 admin 角色访问受保护接口（40303） | `ERROR_CODE_MAP` 缺 40303 → L4 透传英文 | 双端 fixture 无 40303 | 全站所有 40303（7 处） | 双端 map | R1/R2 修复后 `code_map` 中文；修复前 `raw_message` 英文 | 平价脚本纳入 40303 |
| B2 | 任意未映射 code 直出后端英文技术串 | 未知 code + ASCII message | L4 无条件透传 | `UNKNOWN` 用例反而**固化**了 raw_message | 所有未映射码 | 纯核心 L4 守卫 | R4 翻转 4 处旧断言（旧 → 新） | D2 + R4 |
| B3 | PC 与 mobile 文案/层序漂移 | 只改一端 | 两份 map/核心 | 无交叉校验（仅单端测试） | 双端解析器 | 平价脚本 | R3 `DIFFS=0` | 平价脚本进 CI/gate |
| B4 | 收紧 L4 误伤后端中文文案 | 后端返回可读中文（#2480） | 守卫写成「非 ASCII 即丢」 | 无 CJK 用例 | 40900/40901 中文 | `isHumanReadableMessage` 复用 | R1/R2 `human_message` 仍为 `human_message` | 复用 #2480 判定 |
| B5 | 入口仍对 `site_member` 可见（产品口径） | 陈旧菜单/直链 | 菜单门控需 sysPerm+cusPerm `requireAll` | 无入口门控断言 | `/staff` 及同类页面 | 前端菜单配置（已具备） | R5 `MENU_GATED=1` | D5 + R5 |
| B6 | 页面绕过解析器直出后端原文 | 页面用 `error.response.data.message` | 调用点未走唯一出口 | 无接线断言 | 其余 PC 页面 | `api.js` 统一解析 | R5 `PAGE_MESSAGE_FROM_RESOLVER=1` | R5 静态断言 |
| B7 | 修文案却漏翻测试 → 绿灯假象 | 语义变更未翻断言 | 旧断言与需求相反 | 旧 fixture 固化 raw | PC/mobile/平价 3 处 | 同批翻转 | R4 `FLIP_COUNT=4` | 语义变更清单（本计划） |
| B8 | mobile `t()` 包装吞掉/改写文案 | 包装层对 raw 套 `t` | 层序 owner 分裂 | 无包装层断言 | mobile 出口 | 纯核心 + 包装保持 | R2 层序一致 | #2480 D5 口径沿用 |

### Workload

- Size: M
- Basis: 3 个实现文件（PC 解析器、mobile map、mobile 纯核心）+ 2 个测试文件 + 1 个平价脚本 + 1 个核心文档 + 6 条证据行；无后端改动、无 migration、无前端构建产物提交
- Confidence: high

### Files（变更文件清单）

- `frontend-pc/src/services/errorMessages.js`
- `frontend-mobile/src/services/errorMessages.js`
- `frontend-mobile/src/services/errorMessageResolution.mjs`
- `frontend-pc/tests/errorMessages.test.mjs`
- `frontend-mobile/src/services/errorMessageResolution.test.mjs`
- `scripts/check-error-message-parity-2480.mjs`
- `scripts/verify-2504.sh`
- `docs/spec/api/api.md`
- `docs/evidence/2504/plan.md`
- `docs/evidence/2504/evidence.json`
- `docs/evidence/2504/pre-ready-2504.json`
- `docs/evidence/2504/R1-pc-resolver.txt`
- `docs/evidence/2504/R2-mobile-resolver.txt`
- `docs/evidence/2504/R3-parity.txt`
- `docs/evidence/2504/R4-flip.txt`
- `docs/evidence/2504/R5-wiring-static.txt`
- `docs/evidence/2504/R6-build-lint.txt`

## Core Update

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` §1.3（用户可见文案注，约 L30） | 只写「三层回退（L1 message 精确匹配 → L2 code 家族 → L3 调用点 fallback）」，与 #2480 的四层实现不一致，且未说明 ASCII 不外溢 | 更新为 `message_map → human_message（含 CJK 透传）→ code_map → raw_message（仅 CJK）→ fallback`；补「后端英文 message 视为机器码，不得原样展示给用户」 | #2504 期望 3 的口径必须落到契约文档，避免再次出现英文直出 |
| `docs/spec/api/api.md` §14.4 错误码定义表 | 表中无 `40303`（仅有 `40300 权限不足`） | 新增 `40303`：权限不足（角色/权限范围不匹配；前端映射「暂无权限访问该页面」） | 后端实际使用 40303（7 处），错误码表需完整 |

## Dependencies

- 无

### 依赖与关联说明（非硬依赖）

- **#2480 / #2497**：四层解析顺序与 CJK 透传口径（本单在其上补 40303 + ASCII 守卫，不改变既有层序优先级）。
- **#2447**：入口可见性相关；本单确认 `/staff` 菜单门控已生效（D5），不新增路由改动。
- **部署侧**：预生产 `site_member` 直链 `/staff` 的人工走查 + 小程序真机文案，由关闭时创建的 `[Ops-Followup of #2504]`（仅 `waitForConfirm`）承载。

*Model: DeepSeek V4.1 Flash 1017*

