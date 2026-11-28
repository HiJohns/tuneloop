## Accepted Plan — #2429 Rev.2（审计返工：干净证据 + 多网点/双身份口径 + R4 hold）｜Rev.3（双身份优先级按 #2483 翻转）
> **Rev.3 说明（语义变更，翻转本单 R6/D3）**：`#2483` 已把 `resolveStaffFormMode` 的优先级从「direct 优先」改为 **technician → site → direct**，理由是「有网点归属」必须优先于「有 merchant_members 行」，否则双归属员工编辑页被锁成只读「直属商户」，**无法调动网点**（而列表却显示所属网点，口径矛盾）。
> 因此本单 R6 的期望由 `mode=direct` 改为 **`mode=site`**（两 flag 仍均 true；`site_memberships` 仍全量返回）。D3 同步翻转：
> 双身份 → **site 优先渲染**（网点必填/可选网点/不显示只读商户），同时保留「双身份显式提示」`isDualIdentity`（只做提示，不参与字段模式判定）。
> 负例同步翻转：**判为 `direct` → FAIL**（此前是判为 site 才 FAIL）。

### 背景（承接 2026-11-20 Audit REJECT）

阻断：证据 build=ada06f938 / dirty=true（早于 fix f9839430b）；R4 人工回执未签；标签 accepted+ready 脏。
语义挑战：`site_members` 可多行，`Scan` 单值取值不确定；`is_direct_staff = site_id==""` 对「直属+网点双身份」判定错误。

### 变更点

1. **站点变更判定改为 org 级**（`backend/handlers/user_staff.go`）：
   - 不再用 `site_members` 单值 `Scan`；取目标 site 的 `org_id`，与 `existingUser.OrgID`（空则 tenant）比较；
   - org 相同 → **零 IAM 调用**（多网点/同商户移动/直属员工场景一致）；org 不同 → 保留 unbind/bind + 补偿；
   - 显式 null 仍走 unbind；`omitted` 不改。
2. **详情接口多网点/双身份**（`GetStaffUser`）：
   - 返回 `site_memberships[]`（site_id/name/role/org_id，按创建时间）；
   - 主网点 = 与 `users.org_id` 匹配的第一条，否则第一条；
   - `is_site_staff = len(site_memberships)>0`；`is_direct_staff = merchant_members 存在 || 无网点`；
   - 双身份两者均为 true。
3. **前端字段模式单一出口**（新增 `frontend-pc/src/utils/staffForm.js`）：
   - `resolveStaffFormMode`：technician → `technician`；`is_site_staff || site_id` → `site`；否则 `direct`（#2483）；
   - 双身份按 **site 优先**（#2483 翻转：网点必填/可选网点、不显示只读直属商户）；`staffFormRules` 输出 show/require；
   - `StaffEdit.jsx` 改用该 helper。
4. **测试**：R5 多网点同 org → IAM 零调用 + 详情 `site_memberships`；R6 双身份 → **site** 模式（前端纯函数）+ 后端两 flag 均 true。
5. **R4 人工回执**：1.0.90 已部署；创建 waitForConfirm 子 Issue，本人签后恢复；本单 R4 保持 PENDING，状态 `holdon`（不 ready）。
6. **证据**：在 fix 的干净树重生成 R1/R2/R3/R5/R6；R4 由 Owner 签。

### 口径锁定（回应审计挑战）

| 项 | 裁定 |
|---|---|
| 多网点变更判定 | 以目标 site 的 org 与用户 current org 比较；同 org 零 IAM，不同 org 才重绑 |
| 主网点展示 | 优先 `users.org_id` 匹配的 membership，否则最早一条 |
| 双身份 | **`site` 优先（#2483）**：网点必填、不显示只读直属商户；`site_memberships` 仍完整返回；`isDualIdentity` 仅做显式提示 |
| 直属判定 | `merchant_members` 存在或无任何 active 网点 = 直属 |
| R4 | 人工回执未签 → holdon + waitForConfirm，禁止 ready |

### 既有断言保持/翻转

- 保持：同 site 保存不触发 IAM；编辑页按 id 加载；维修师傅无职位；职位 AutoComplete；列表入口统一。
- 调整：same-site 测试的 site org 设为用户 org（此前随机 org 与生产语义不符）；新增多网点/双身份用例。
- 无其他翻转。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| 同 org 零 IAM | `backend/handlers/user_staff.go` | `PUT /users/:id` | 同 site / 多网点同 org | 后端 | 200；org 不变；无 IAM | R1 |
| PC 构建 | `frontend-pc/` | build | — | PC | exit 0 | R2 |
| 静态探针 | 前后端 | grep | — | 双端 | 无硬编码、按 id 加载、AutoComplete | R3 |
| 人工验收 | 员工编辑页 | 人工 | 7 项 | PC | 全部 PASS | R4 |
| 多网点 | 后端单测 | `GET /admin/staff/:id` + PUT | 2 个 active 网点 | 后端 | memberships=2；同 org 零 IAM | R5 |
| 双身份 | 前端纯函数 + 后端 | node test | direct+site | 双端 | **mode=site**；两 flag true | R6 |

## Input Truth & Tool Preflight

- **Source of truth**：ui.md L2565/L2586、#2375 角色契约、审计 2026-11-20。
- **差异核对**：`site_members` 唯一约束 `(site_id,user_id)` 允许多行；IAM org 只有一层 → 用 org 比较。
- **损坏输入拒绝**：非法/不存在 site → 400；IAM 失败 500（保留补偿）。
- **工具盲区**：真机/PC 人工回执由 Owner；单测覆盖纯函数与接口。

## Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 站点变更 | org 级比较；同 org 零 IAM |
| D2 | 主网点 | org 匹配优先，否则最早 |
| D3 | 双身份 | **site 优先渲染（#2483 翻转）**；`isDualIdentity` 仅提示；memberships 全量返回 |
| D4 | R4 | 未签 → holdon + waitForConfirm |
| D5 | 证据 | 干净树、含 fix、dirty=false |

## Out of Scope

- 多网点成员的增删/切换 UI（仅展示与判定）。
- 角色白名单变更（#2375）。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 同 site / 多网点同 org | 后端单测 | `docs/evidence/2429/R1-go.txt` | 200；org 不变；无 IAM | IAM 调用/500 → FAIL |
| R2 | PC 源码 | build | `docs/evidence/2429/R2-pc-build.txt` | exit 0 | 失败 → FAIL |
| R3 | 源码探针 | grep | `docs/evidence/2429/R3-probe.txt` | 4 项探针 | 缺项 → FAIL |
| R4 | 7 项编辑场景 | 人工 | `docs/evidence/2429/R4-manual.md` | RESULT: PASS | PENDING → holdon |
| R5 | 多网点用户 | 后端单测 | `docs/evidence/2429/R5-multisite.txt` | memberships=2；同 org 零 IAM | 任意取值/误绑 → FAIL |
| R6 | 双身份 | node test | `docs/evidence/2429/R6-dual.txt` | **mode=site**；两 flag true；`isDualIdentity=true` | 判为 direct → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2429.sh r1-go` | docs/evidence/2429/R1-go.txt | exit 0；同 org 用例 PASS | PASS | IAM 调用 → FAIL | dev |
| R2 | `bash scripts/verify-2429.sh r2-pc-build` | docs/evidence/2429/R2-pc-build.txt | exit 0；built in | PASS | 失败 → FAIL | dev |
| R3 | `bash scripts/verify-2429.sh r3-probe` | docs/evidence/2429/R3-probe.txt | exit 0；PROBE-OK | PASS | 缺项 → FAIL | dev |
| R4 | `bash scripts/verify-2429.sh r4-manual` | docs/evidence/2429/R4-manual.md | exit 0；RESULT: PASS | PASS | PENDING → holdon | architect |
| R5 | `bash scripts/verify-2429.sh r5-multisite` | docs/evidence/2429/R5-multisite.txt | exit 0；memberships=2 且零 IAM | PASS | 误绑 → FAIL | dev |
| R6 | `bash scripts/verify-2429.sh r6-dual` | docs/evidence/2429/R6-dual.txt | exit 0；mode=site + flags true | PASS | 判为 direct → FAIL | dev |

## Artifact Provenance

- R1/R2/R3/R5/R6 首行：`# build=1.0.<短码> commit=<40位> dirty=false`（三 token 逐字保留：`# build=1.0.` / `# commit=` / `# dirty=false`）。
- `evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$`（与上述首行一致）。
- R4 人工回执（Owner 签）。
- 证据产物清单（合同逐行对应）：
  - `docs/evidence/2429/R1-go.txt`
  - `docs/evidence/2429/R2-pc-build.txt`
  - `docs/evidence/2429/R3-probe.txt`
  - `docs/evidence/2429/R4-manual.md`
  - `docs/evidence/2429/R5-multisite.txt`
  - `docs/evidence/2429/R6-dual.txt`

## Dependencies

### 依赖与关联说明（非硬依赖，机器可读区无条目 = 无未完成硬依赖）

- #2375（角色分配）已 closed；#2065/#2261（人员视图与租户隔离观察期）已合入 `develop`。
- R4 为人工回执（`waitForConfirm` 子 Issue），不阻塞 R1/R2/R3/R5/R6 的代码与证据。

### Files（变更文件清单）

- `backend/handlers/user_staff.go`
- `backend/handlers/user_staff_site_noop_2429_test.go`
- `backend/main.go`
- `frontend-pc/src/pages/StaffEdit.jsx`
- `frontend-pc/src/pages/StaffManagement.jsx`
- `frontend-pc/src/utils/staffForm.js`
- `frontend-pc/src/utils/staffForm.test.js`
- `frontend-pc/src/services/api.js`
- `docs/spec/ui/ui.md`
- `scripts/verify-2429.sh`
- `docs/evidence/2429/R1-go.txt`
- `docs/evidence/2429/R2-pc-build.txt`
- `docs/evidence/2429/R3-probe.txt`
- `docs/evidence/2429/R4-manual.md`
- `docs/evidence/2429/R5-multisite.txt`
- `docs/evidence/2429/R6-dual.txt`
- `docs/evidence/2429/evidence.json`

*Model: DeepSeek V4.1 Flash 1017*


