## Accepted Plan — #2597 [Bug] 中转网点创建失败提示不可诊断（配置不变量被压平成瞬时故障 + 该页绕过统一错误层）

### 背景与现状核对（代码为据）

| # | 事实 | 位置 |
|---|---|---|
| 1 | `CreateAdminTransitSite` 把 `ResolveNamespacePrimaryOrg` 的**全部**错误压平为 `500/50000` +「无法解析命名空间主组织，请稍后再试」 | `backend/handlers/transit_site_admin.go:103-108` |
| 2 | `ResolveNamespacePrimaryOrg` 已区分三类原因：`no primary organization` / `N primary organizations (expected exactly 1)` / `primary organization has empty id`（另有 `list organizations: %w` 传输层错误） | `backend/services/iam_client.go:504-530` |
| 3 | 前端该页 5 处错误分支全部直取 `r.message` / `e.message`，未走 `resolveErrorCore` | `frontend-pc/src/pages/admin/transit/TransitCenter.jsx:46,73,83,100,107` |
| 4 | PC 客户端对**其余非 2xx（含 5xx）抛 `Error`（`name=ApiError`）**；`api.post/put/delete` 因此会 **reject**，而 `handleSiteSubmit` / `handleDeactivate` / `handleRouteCreate` / `handleRouteDelete` **无 try/catch** → 抛出的提示层异常无人消费（用户看不到任何信息） | `frontend-pc/src/services/api.js:330-339`、`frontend-pc/src/services/apiError.js:23-58`、`TransitCenter.jsx:66-108` |
| 5 | 编辑分支 `await api.put(...)` 后直接 `message.success('已更新')`，**不判** `code`；而 404/409 由 `resolveHttpFailure` 返回对象（不抛） → 冲突/不存在时误报「已更新」 | `TransitCenter.jsx:69-70`、`apiError.js:27-49` |
| 6 | `resolveErrorCore` 对 `code >= 50000` + CJK **不透传** message：有 `ERROR_CODE_MAP` 条目走 code_map，否则 fallback（#2497） | `frontend-pc/src/services/errorMessages.js:120-127` |
| 7 | 5xxxx 码已占用到 `50013`（`instrument_photo.go`），另有 `50200` / `50202`；`50014` 空闲 | `grep -rhoE '"code": *5[0-9]{4}' backend` |
| 8 | 既有断言把「主组织不可解析」钉在 `50000`：测试内 `unresolvedToken = 50000` + `verify-2552.sh` 期望 `PRIMARY_ORG_UNRESOLVED=50000` | `backend/handlers/transit_site_admin_2552_test.go:502-531`、`scripts/verify-2552.sh:165` |
| 9 | 停用被拒文案含内部 Issue 号「（#1935 完整性约束）」，接入统一错误层后该串会**真正到达用户** | `backend/handlers/transit_site_admin.go:209` |

**结论**：功能逻辑（fail-fast 不猜、不回落租户）正确，缺陷集中在**提示层**：后端错误分类被压平、前端页面绕过统一错误层（且 5xx 抛错后无人接），两层叠加导致用户既不能定位也不能行动。

### 变更点

#### 1. 后端：把「配置不变量违反」与「瞬时故障」拆成两条可区分路径

`backend/services/iam_client.go`（新增类型化错误，唯一判定出口）：

```go
// PrimaryOrgConfigError 表示命名空间主组织不满足「恰好 1 个」的持久配置不变量。
// 与传输层错误（list organizations: %w）互斥：config 类重试永不成功。
type PrimaryOrgConfigError struct {
    Detail string   // none | multiple | empty_id
    Count  int      // 命中的 is_primary 组织数（0 / >1 / 1 但 id 为空）
    OrgIDs []string // 命中的组织 id，仅用于服务端日志
}
func (e *PrimaryOrgConfigError) Error() string // 文本与现有三串逐字一致（零语义漂移）
```

- `ResolveNamespacePrimaryOrg` 的 0 个 / ≥2 个 / id 为空 → 返回 `*PrimaryOrgConfigError`（`Detail` 分别为 `none` / `multiple` / `empty_id`，`OrgIDs` 收集命中项）；`list organizations: %w` 保持普通 error（瞬时）。
- 判定只用 `errors.As`，**禁止**字符串匹配。

`backend/handlers/transit_site_admin.go`：

```go
parentOrgID, err := services.ResolveNamespacePrimaryOrg(iamClient)
if err != nil {
    var cfgErr *services.PrimaryOrgConfigError
    if errors.As(err, &cfgErr) {
        log.Printf("[CreateAdminTransitSite] namespace primary org misconfigured detail=%s count=%d expected=1 org_ids=[%s]: %v",
            cfgErr.Detail, cfgErr.Count, strings.Join(cfgErr.OrgIDs, ","), err)
        c.JSON(http.StatusInternalServerError, gin.H{"code": 50014,
            "message": "命名空间主组织配置异常（应为 1 个），请联系平台管理员处理"})
        return
    }
    log.Printf("[CreateAdminTransitSite] resolve namespace primary org transient failure: %v", err)
    c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "身份服务暂时不可用，请稍后再试"})
    return
}
```

- 配置类：`500/50014` + 可行动文案（**不含**「请稍后再试」）；计数 / 组织 id 只进日志。
- 瞬时类：`500/50000` + 「请稍后再试」（HTTP 码与 code **均不变**，只去掉误导性的「无法解析」措辞）。
- 建组织失败（`:127`）与角色更新（`:387`）**本单不动**，见 Out of Scope / #2598。

#### 2. 后端：停用被拒文案去掉内部 Issue 号

`backend/handlers/transit_site_admin.go:209`：「该中转网点仍被路由引用，请先解除相关路由（#1935 完整性约束）」→「该中转网点仍被路由引用，请先解除相关路由」（该串接入统一错误层后会真实到达用户；保留「路由引用」关键词，#1935 既有断言只断言该关键词）。

#### 3. 前端：登记新错误码（PC + mobile 文案逐字一致）

- `frontend-pc/src/services/errorMessages.js`：`ERROR_CODE_MAP` 增 `50014: '命名空间主组织配置异常（应为 1 个），请联系平台管理员处理',`
- `frontend-mobile/src/services/errorMessages.js`：同一条同文案（mobile `t()` 缺键回退原串，`i18n/index.js:16-23` 恒等，无需 locale 条目）。

> 文案与后端 `message` **逐字一致**：`#2497` 规则下 5xx+CJK 走 `code_map`，两端文案不同会让「用户看到的」与「后端写的」漂移。

#### 4. 前端：`TransitCenter.jsx` 接入统一错误层（5 处分支 + 4 个 handler 兜底 + 编辑分支判码）

```jsx
import { resolveErrorMessage } from '../../../services/errorMessages'

// #2597：本页所有失败提示走唯一出口（#2497/#2489 口径）——5xx 中文内部诊断不透传，
// 业务码走 code_map、可读中文透传；ASCII 机器串不外溢。
const showError = (err, fallback) => {
  const code = err && typeof err === 'object' ? err.code : undefined
  const msg = err && typeof err === 'object' ? err.message : String(err || '')
  message.error(resolveErrorMessage({ code, message: msg }, fallback))
}
```

| 位置 | 现状 | 目标 |
|---|---|---|
| `fetchAll` catch（:46） | `message.error(e.message \|\| '加载失败')` | `showError(e, '加载失败，请重试')` |
| 创建分支（:73） | `if (r.code !== 20000) message.error(r.message \|\| '创建失败')` | 保留判码 + `showError(r, '创建失败，请重试')`，整段包 `try/catch`（500 会抛） |
| 编辑分支（:69-70） | 不判码，直接 `message.success('已更新')` | 补 `if (r.code !== 20000) { showError(r, '更新失败，请重试'); return }` |
| 停用分支（:83） | `message.error(r.message \|\| '停用失败（需先解除路由引用）')` | `try/catch` + `showError(e/r, '停用失败，请先解除路由引用')` |
| 创建路由（:100） | `message.error(r.message \|\| '创建失败')` | `try/catch` + `showError(e/r, '创建路由失败，请重试')` |
| 删除路由（:107） | `message.error(r.message \|\| '删除失败')` | `try/catch` + `showError(e/r, '删除路由失败，请重试')` |

不改页面结构 / 表单字段 / 成员管理区；不改其它页面（全站错误层收敛另属 #2489 族）。

#### 5. 测试与证据

- 新增 `backend/handlers/transit_site_admin_2597_test.go`（自带最小 fake IAM `transitIAM2597`，支持 `primaries ∈ {0,1,2}`、`listMode ∈ {ok,error}`；复用同包 `transitActorRouter2552()` 的 actor 注入）：
  - `TestTransitSiteCreate2597_PrimaryConfigInvariant`（0 个 / 2 个）、
  - `TestTransitSiteCreate2597_PrimaryTransient`（list 返回 500）、
  - `TestTransitSiteDeactivate2597_MessageNoInternalRef`（停用文案不含 `#1935`）。
  - 日志断言：测试内 `log.SetOutput(&buf)` 后断言 `count=` / `org_ids=` 出现。
- 翻转既有断言（见下节）+ 更新 `scripts/verify-2552.sh:165`。
- 新增 `frontend-pc/tests/transitErrorText-2597.test.mjs`（`node --test`，纯函数 + 真实抛出链 `resolveHttpFailure(500, …)`）：
  - `50014` 层胜出 = `code_map`，文案为可行动串、不含「请稍后再试」；
  - 后端若误把英文内部串放 message（`namespace has 2 primary organizations (expected exactly 1)`）→ 输出仍为中文（`ASCII_LEAK=0`）；
  - `resolveHttpFailure(500, {code:50014,…})` 抛出的 `Error.code === 50014`；
  - 40002 中文串走 `human_message`（停用分支）；
  - mobile map 的 `50014` 文案与 PC 逐字一致（静态提取比对）。
- 新增 `scripts/verify-2597.sh`（`r1`–`r7`，每行一个子命令，输出 `KEY=VALUE` token）。

#### 6. 文档（见 Core Update）

### 被翻转 / 被强化的既有断言（强制清单）

| 文件:行 | 旧期望 | 新期望 | 理由 |
|---|---|---|---|
| `backend/handlers/transit_site_admin_2552_test.go:519-529` | 0/2 个 primary → `status==500` 且 token `PRIMARY_ORG_UNRESOLVED=50000`（`unresolvedToken=50000`） | 同状态 500，但 token 取响应 `code`：`PRIMARY_ORG_UNRESOLVED=50014`，并断言 message 含「联系平台管理员」且**不含**「请稍后再试」 | #2597 要求配置不变量使用专用码；旧断言把两类失败钉在同一码上，不翻转即与新语义冲突 |
| `scripts/verify-2552.sh:165` | `"PRIMARY_ORG_UNRESOLVED=50000"` | `"PRIMARY_ORG_UNRESOLVED=50014"` | 同上（#2552 证据预期随语义变更翻转；该测试的其余 token 全部保留，不改判定规则） |
| `backend/handlers/transit_site_admin_test.go:234` | `assert.Contains(resp["message"], "路由引用")` | **保持不变**，新增 `assert.NotContains(resp["message"], "#1935")` | 文案去掉内部 Issue 号；关键词「路由引用」保留，故旧断言无需改写 |

> 除上表外，**不改**任何既有断言；`TestTransitSiteCreate2552` 的其余 token（`CREATE_TYPE_BRANCH` / `CREATE_PARENT_IS_NAMESPACE` / `DUPLICATE_NAME_409` / `NO_SITE_ON_EMPTY_TENANT` / `ORPHAN_ORG_COMPENSATED` / `SITE_ROWS_ON_FAILURE`）必须保持原值。

### 同批扫描判定（18 处「请稍后再试」）

> Issue 正文估 15 处；实测 18 处（`grep -rn "请稍后再试" backend/handlers`）。判定口径：该失败的**原因是否持久**（配置缺失/冲突/权限不足 → 重试永不成功）或**是否为冲突却提示可重试**。

| # | 位置 | 判定 | 处置 |
|---|---|---|---|
| 1 | `transit_site_admin.go:106` | **配置（0/≥2 primary）与瞬时混在同一分支** | **本单修复**（专用码 50014 + 可行动文案） |
| 2 | `transit_site_admin.go:127` | 建组织失败：`CreateOrganization` 的 400/403（父组织非法/权限）属持久，网络属瞬时，全部压平 502/50200 | 另立 **#2598 项 1** |
| 3 | `user_staff.go:2271` | `WxBind` 任意失败（含 IAM 不可达）→ **409/40900** +「请稍后再试」（冲突码 + 可重试文案双错配） | 另立 **#2598 项 2** |
| 4 | `user_staff.go:2315` | 同上（`WxMerge` 任意失败 → 409/40900 +「请稍后再试」） | 另立 **#2598 项 3** |
| 5 | `registration_session.go:454` | 瞬时（副作用重放失败） | 保持（40002 + 稍后再试 语义正确） |
| 6 | `payscore_handler.go:147` | 瞬时（腾讯信用评估外部依赖） | 保持 |
| 7 | `user_management.go:372` | 瞬时（IAM deactivate 失败） | 保持 |
| 8 | `merchant.go:719` | 瞬时（级联解绑 IAM 失败；「部分完成」属另一类问题） | 保持 |
| 9 | `merchant.go:732` | 瞬时（同上，网点成员） | 保持 |
| 10 | `transit_site_admin.go:387` | 瞬时（IAM 角色更新）；附低危观察：HTTP 502 配 code 50000（码/状态不一致，用户不可见） | 保持 |
| 11 | `site_member.go:326` | 瞬时（IAM unbind 失败） | 保持 |
| 12 | `merchant_member.go:536` | 瞬时（ListRoleTemplates 失败） | 保持 |
| 13 | `merchant_member.go:543` | 瞬时（AssignRoleTemplate 失败） | 保持 |
| 14 | `merchant_member.go:640` | 瞬时（IAM unbind 失败） | 保持 |
| 15 | `user_staff.go:918` | 瞬时（切换网点 unbind 失败） | 保持 |
| 16 | `user_staff.go:926` | 瞬时（bind 新组织失败） | 保持 |
| 17 | `user_staff.go:937` | 瞬时（设权限失败） | 保持 |
| 18 | `user_staff.go:947` | 瞬时（同步角色模板失败） | 保持 |

结论：**仅 1 处落在本单**（:106），**3 处另立 #2598**，其余 14 处为真瞬时故障（文案正确，保持）。

### Files（变更文件清单）

- `backend/services/iam_client.go`
- `backend/handlers/transit_site_admin.go`
- `backend/handlers/transit_site_admin_2597_test.go`
- `backend/handlers/transit_site_admin_2552_test.go`
- `backend/handlers/transit_site_admin_test.go`
- `frontend-pc/src/services/errorMessages.js`
- `frontend-mobile/src/services/errorMessages.js`
- `frontend-pc/src/pages/admin/transit/TransitCenter.jsx`
- `frontend-pc/tests/transitErrorText-2597.test.mjs`
- `scripts/verify-2597.sh`
- `scripts/verify-2552.sh`
- `docs/spec/api/api.md`
- `docs/spec/ui/ui.md`
- `docs/evidence/2597/plan.md`
- `docs/evidence/2597/R1-primary-config.txt`
- `docs/evidence/2597/R2-primary-transient.txt`
- `docs/evidence/2597/R3-legacy-flip.txt`
- `docs/evidence/2597/R4-frontend-resolver.txt`
- `docs/evidence/2597/R5-page-wiring.txt`
- `docs/evidence/2597/R6-docs.txt`
- `docs/evidence/2597/R7-build-lint.txt`
- `docs/evidence/2597/evidence.json`
- `docs/evidence/2597/pre-ready-2597.json`

### Core Update

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | §14.4 错误码表只列到 `50000`（并注明 `40912` 占用），无 `50014`；无「配置不变量 vs 瞬时故障」口径 | §14.4 增行 `\| 50014 \| 命名空间主组织配置异常（命名空间主组织数 ≠ 1，持久配置不变量违反；`POST /api/admin/transit-sites` 返回，需平台管理员修数据，**不可重试**） \|`；错误码占用说明补 `50014`；§14.4 追加一行口径「配置类失败用专用码 + 可行动文案，禁止写成『请稍后再试』」 | 新增错误码；且 `§1.3` 规则要求新码同步 PC + mobile 的 `ERROR_CODE_MAP`，文档需给出该码语义与用户文案 |
| `docs/spec/ui/ui.md` | §3.20「中转中心」只写端点/列/操作，未写失败提示口径（隐含「透出文案」） | §3.20 增一条「失败提示」：创建/编辑/停用/路由操作统一走 `resolveErrorMessage`（#2497 口径，5xx 中文内部诊断不透传）；配置类错误显示「命名空间主组织配置异常（应为 1 个），请联系平台管理员处理」，瞬时类只给「稍后再试」；页面禁止直出 `r.message`/`e.message` | 该页错误层行为变为用户可见契约（含新文案），UI 规格需同步 |

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 后端配置类失败分流（0 个 / 2 个 primary） | `backend/handlers/transit_site_admin.go`、`backend/services/iam_client.go`、`backend/handlers/transit_site_admin_2597_test.go` | `POST /api/admin/transit-sites` | fake IAM `organizations` 返回 0 个 / 2 个 `is_primary` | backend（Postgres 测试库） | `PRIMARY_CONFIG_CODE=50014`、文案含「联系平台管理员」且不含「请稍后再试」、不建组织、不建站、日志含 `count=`/`org_ids=` | R1 |
| R2 后端瞬时失败不分流（IAM 组织列表不可用） | `backend/handlers/transit_site_admin.go`、`backend/handlers/transit_site_admin_2597_test.go` | `POST /api/admin/transit-sites` | fake IAM `organizations` 返回 500 | backend（Postgres 测试库） | `TRANSIENT_STATUS=500`、`TRANSIENT_CODE=50000`、文案含「稍后再试」、**不出现** 50014、不建站 | R2 |
| R3 存量断言翻转与强化（#2552 主组织用例 + 停用文案） | `backend/handlers/transit_site_admin_2552_test.go`、`backend/handlers/transit_site_admin_test.go`、`scripts/verify-2552.sh` | `POST /api/admin/transit-sites`、`DELETE /api/admin/transit-sites/:id` | 0/2 primary；被路由引用的站点停用 | backend（Postgres 测试库） | `PRIMARY_ORG_UNRESOLVED=50014`、`HARNESS_FLIPPED=1`、`DEACTIVATE_MSG_NO_INTERNAL_REF=1`，其余 #2552 token 原值 | R3 |
| R4 前端解析链（新码映射 / 内部串不外溢 / 真实抛出链） | `frontend-pc/tests/transitErrorText-2597.test.mjs` | `node --test` | `{code:50014}` 中英文 message、`resolveHttpFailure(500, …)`、40002 中文串 | PC（Node 纯函数） | `E50014_LAYER=code_map`、`E50014_RETRY_HINT=0`、`HTTP_THROW_CODE=50014`、`ASCII_LEAK=0`、`HUMAN_40002=1`、`MOBILE_MAP_MATCH=1`、`NODE_TEST_FAIL=0` | R4 |
| R5 页面接线（5 处分支走统一出口 + 4 个 handler 兜底 + 编辑判码 + 双端码表） | `frontend-pc/src/pages/admin/transit/TransitCenter.jsx`、`frontend-pc/src/services/errorMessages.js`、`frontend-mobile/src/services/errorMessages.js` | 源码结构断言（`evidence_type: static`，Owner 豁免） | 修后源码 | PC | `RESOLVE_HELPER=1`、`RAW_MESSAGE_BRANCHES=0`、`TRY_CATCH_HANDLERS=5`、`EDIT_CODE_CHECK=1`、`PC_MAP_50014=1`、`MOBILE_MAP_50014=1` | R5 |
| R6 文档同步（新码 + 该页失败提示口径） | `docs/spec/api/api.md`、`docs/spec/ui/ui.md` | 文档结构断言（`evidence_type: static`，Owner 豁免） | 修后文档 | 文档 | `API_CODE_50014=1`、`API_TEXT_ACTIONABLE=1`、`UI_ERROR_LAYER=1` | R6 |
| R7 构建 / gofmt / lint | `scripts/verify-2597.sh` | `cd backend && go build ./...` + `gofmt -l` + `npx eslint` | 修后工作树 | 双端 | `GO_BUILD_OK=1`、`GOFMT_DIFF=0`、`ESLINT_NOUNDEF=0` | R7 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | integration | Go 定向测试 + Postgres 测试库真实建站路径 + fake IAM 注入 0/2 个主组织，断言响应码/文案/副作用/日志 |
| R2 | integration | 同上，fake IAM 组织列表返回 500（瞬时类不分流） |
| R3 | integration | 既有测试翻转后的 token + 停用文案断言（#1935 回归不破） |
| R4 | unit | `node --test` 纯函数 + 真实 `resolveHttpFailure` 抛出链（无浏览器依赖） |
| R5 | static | PC 无渲染测试运行器（`frontend-pc/package.json` 无 vitest/jsdom/@testing-library）；按 **Owner 豁免**（沿用 #2580 D6 / #2480 D6 / #2578 D7）以源码结构断言 + R4 纯函数单测 + R7 构建为证据；真实浏览器点击归部署侧复验 |
| R6 | static | 文档断言（Owner 豁免同上） |
| R7 | build | `go build ./...` + `gofmt -l` + `npx eslint`（受影响文件） |

### Input Truth & Tool Preflight

- **Source of truth**：
  - 错误分类唯一出口 = `backend/services/iam_client.go:504-530`（本单改为类型化错误 `*PrimaryOrgConfigError`，`Error()` 文本保持逐字一致）；
  - 前端文案唯一出口 = `frontend-pc/src/services/errorMessages.js`（`resolveErrorCore`，层序与 mobile `errorMessageResolution.mjs` 同序）；
  - 5xxxx 码占用表 = `grep -rhoE '"code": *5[0-9]{4}' backend --include=*.go | sort -n | uniq -c`（当前最大 `50013`；`50200`/`50202` 亦占用）→ 本单取 `50014`；
  - 既有断言 = `backend/handlers/transit_site_admin_2552_test.go`、`scripts/verify-2552.sh:165`（翻转清单见上）。
- **实施前差异核对（先跑，结果入 Work Summary）**：
  - `grep -n "50014" backend frontend-pc/src frontend-mobile/src` → 修前 **0 命中**（本单新增）；
  - `grep -n "PrimaryOrgConfigError" backend/services/iam_client.go` → 修前 **0 命中**；
  - `grep -c "resolveErrorMessage" frontend-pc/src/pages/admin/transit/TransitCenter.jsx` → 修前 **0**；
  - `grep -c "try {" frontend-pc/src/pages/admin/transit/TransitCenter.jsx` → 修前 **1**（仅 `fetchAll`），修后 **5**；
  - `grep -rn "请稍后再试" backend/handlers | wc -l` → 修前 **18**，修后 **17**（:106 拆分后瞬时仍保留「稍后再试」措辞但改为「身份服务暂时不可用，请稍后再试」→ 行数保持 18，改由文案断言判定，不用行数当证据）。
- **损坏输入拒绝规则**：
  - fake IAM 返回 `organizations` 非 JSON / 缺 `is_primary` → `ListOrganizations` 解析失败视作**瞬时**（走 50000），不得误判为配置类；
  - `is_primary=true` 但 `id` 为空 → 配置类（`Detail=empty_id`），不得回落租户；
  - 前端 `e` 非对象（字符串/null）→ `showError` 必须仍给出 fallback 文案，不得抛二次异常。
- **工具盲区**：
  - fake IAM 只证明我方判定与响应，不证明 beaconiam 侧「主组织唯一性」加固（已另立 beaconiam#513）；
  - PC 无渲染测试运行器（见 R5 evidence_type），真实点击/提示条形态归 `[Ops-Followup of #2597]`（`waitForConfirm`，无 `status:*`）；
  - `log.SetOutput` 断言只覆盖本 handler 的日志片段，不覆盖 systemd/journald 采集链路。
- **契约影响**：新增错误码 `50014` 与两条文档 → Work Summary 必须含 `### 契约三向核对`（pre-ready P7）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 专用码 = `50014`（5xxxx 已占用到 `50013`），HTTP 500；配置类判定 = 0 个 / ≥2 个 / 唯一主组织 id 为空 | 本计划锁定 |
| D2 | 用户文案固定为「命名空间主组织配置异常（应为 1 个），请联系平台管理员处理」，后端 `message` 与 PC/mobile `ERROR_CODE_MAP[50014]` **逐字一致**；计数/组织 id 只进日志（`count=` / `org_ids=`） | 本计划锁定（#2497 规则下 5xx+CJK 走 code_map，故两端必须同串） |
| D3 | 瞬时类保持 HTTP 500 + code `50000`（不改码/状态，避免扩散 #2497 影响面），文案改「身份服务暂时不可用，请稍后再试」；不新增 `50200` 到 map（全站影响另议） | 本计划锁定 |
| D4 | 类型化错误 `*services.PrimaryOrgConfigError{Detail,Count,OrgIDs}`，`errors.As` 判定，`Error()` 三串逐字保持；`transit_site_org_backfill.go` 的 `%w` 包装不受影响（不改其语义） | 本计划锁定 |
| D5 | 前端：5 处错误分支统一 `resolveErrorMessage`（唯一出口），4 个 async handler 加 `try/catch`（5xx 抛 `ApiError`），编辑分支补 `r.code !== 20000` 判定（404/409 返回对象 → 误报成功） | 本计划锁定 |
| D6 | 停用被拒文案去掉内部 Issue 号（保留「路由引用」），`#1935` 既有断言不改写 | 本计划锁定 |
| D7 | PC 证据形态 = 纯函数单测 + 源码结构断言 + 构建；真实渲染/点击归部署侧复验。**Owner 豁免**（沿用 #2580 D6 / #2480 D6 / #2578 D7），真实浏览器回执进 `[Ops-Followup of #2597]` | Owner 已豁免（同族先例） |
| D8 | 重命令（`go test` / `go build` / `eslint` / `node --test`）经 `heavy-run.sh` 取锁；合同行禁止全量 `go test ./...`（P5 baseline 差分承担） | 本计划锁定 |
| D9 | 扫描判定产出：本单只修 `:106`；`:127`/`user_staff.go:2271`/`:2315` 进 **#2598**；其余 14 处保持（真瞬时） | 本计划锁定 |
| D10 | 不新增/修改 `POST /api/admin/transit-sites` 端点在 `api.md` 的全文规格（该端点尚未收录，属既有文档债，不在本单扩面） | 本计划锁定 |

### Out of Scope

- `transit_site_admin.go:127`（建组织失败压平 400/403）与 `user_staff.go:2271` / `:2315`（409 冲突 + 可重试文案）→ 由 **#2598** 承载。
- 其余 14 处「请稍后再试」（`registration_session` / `payscore` / `user_management` / `merchant` / `site_member` / `merchant_member` / `user_staff` 切换网点族）→ 判定为真瞬时故障，保持现状。
- `transit_site_admin.go:387` 的 HTTP 502 + code 50000 码/状态不一致（用户不可见的低危项）→ 本单仅记录，不修。
- 全站错误层收敛（`ERROR_CODE_MAP` 增补 `50200` 等）、其它页面 `r.message` 直出清理 → #2489 族。
- IAM 侧主组织唯一性约束加固 → beaconiam#513。
- `POST /api/admin/transit-sites` 端点在 `api.md` 的全文规格补录（文档债，另立文档条目时一并处理）。
- H5 / 微信小程序任何改动（中转中心为 PC-only，`ui.md` §3.20 明示「仅 PC」）。
- 全量 `go test ./...` 作为合同行（由 pre-ready P5 baseline 差分承担）。

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 租户 T（`SeedTenantFK`）；fake IAM：`primaries=0` 与 `primaries=2`，`createMode=ok` | `POST /api/admin/transit-sites`（`TestTransitSiteCreate2597_PrimaryConfigInvariant`） | `docs/evidence/2597/R1-primary-config.txt` | `PRIMARY_CONFIG_CODE=50014`、`PRIMARY_CONFIG_MSG_HAS_ACTIONABLE=1`（含「联系平台管理员」）、`PRIMARY_CONFIG_MSG_NO_RETRY=0`、`PRIMARY_CONFIG_ORGS_CREATED=0`、`PRIMARY_CONFIG_SITES_CREATED=0`、`PRIMARY_CONFIG_LOG_DETAIL=1` | 响应出现「请稍后再试」、出现英文 `namespace has` / `expected exactly 1`、出现组织 id、返回 50000、建出组织或站点 → 任一即 FAIL |
| R2 | 同上，fake IAM `/api/v1/organizations` 返回 500（`listMode=error`） | `POST /api/admin/transit-sites`（`TestTransitSiteCreate2597_PrimaryTransient`） | `docs/evidence/2597/R2-primary-transient.txt` | `TRANSIENT_STATUS=500`、`TRANSIENT_CODE=50000`、`TRANSIENT_MSG_RETRY=1`、`TRANSIENT_NOT_CONFIG_CODE=1`、`TRANSIENT_SITES_CREATED=0` | 瞬时失败返回 50014、返回 4xx、建出站点、文案缺失「稍后再试」→ 任一即 FAIL |
| R3 | 同上（0/2 primary）；另：被路由引用的中转站点 | `TestTransitSiteCreate2552_PrimaryUnresolved` + `TestDeleteTransitSiteRejectsWhenReferenced` | `docs/evidence/2597/R3-legacy-flip.txt` | `PRIMARY_ORG_UNRESOLVED=50014`、`HARNESS_FLIPPED=1`、`DEACTIVATE_MSG_NO_INTERNAL_REF=1`，且 #2552 其余 token 原值 | token 仍为 50000、`verify-2552.sh` 仍期望 50000、停用文案仍含 `#1935`、「路由引用」断言失败 → 任一即 FAIL |
| R4 | 纯函数输入：`{code:50014, message:CJK}`、`{code:50014, message:'namespace has 2 primary organizations (expected exactly 1)'}`、`resolveHttpFailure(500,{code:50014,...})`、`{code:40002, message:'该中转网点仍被路由引用，请先解除相关路由'}` | `node --test frontend-pc/tests/transitErrorText-2597.test.mjs` | `docs/evidence/2597/R4-frontend-resolver.txt` | `E50014_LAYER=code_map`、`E50014_RETRY_HINT=0`、`HTTP_THROW_CODE=50014`、`ASCII_LEAK=0`、`HUMAN_40002=1`、`MOBILE_MAP_MATCH=1`、`NODE_TEST_FAIL=0` | 输出含英文内部串、含「请稍后再试」、层为 `human_message`/`fallback`、抛出链丢 `code`、双端文案不一致 → 任一即 FAIL |
| R5 | 修后 `TransitCenter.jsx` + 双端 `errorMessages.js` | `bash scripts/verify-2597.sh r5` | `docs/evidence/2597/R5-page-wiring.txt` | `RESOLVE_HELPER=1`、`RAW_MESSAGE_BRANCHES=0`、`TRY_CATCH_HANDLERS=5`、`EDIT_CODE_CHECK=1`、`PC_MAP_50014=1`、`MOBILE_MAP_50014=1` | 残留 `message.error(r.message` / `e.message` 直出、handler 缺 try/catch、编辑分支不判码、任一端缺 50014 → 任一即 FAIL |
| R6 | 修后 `docs/spec/api/api.md` + `docs/spec/ui/ui.md` | `bash scripts/verify-2597.sh r6` | `docs/evidence/2597/R6-docs.txt` | `API_CODE_50014=1`、`API_TEXT_ACTIONABLE=1`、`UI_ERROR_LAYER=1` | 文档无 50014 行 / 未写「不可重试」语义 / UI 未写统一错误层口径 → 任一即 FAIL |
| R7 | 修后工作树 | `bash scripts/verify-2597.sh r7` | `docs/evidence/2597/R7-build-lint.txt` | `GO_BUILD_OK=1`、`GOFMT_DIFF=0`、`ESLINT_NOUNDEF=0` | 构建失败、gofmt 有差异、eslint `no-undef` → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2597.sh r1 > docs/evidence/2597/R1-primary-config.txt` | `docs/evidence/2597/R1-primary-config.txt` | exit 0 且含 `PRIMARY_CONFIG_CODE=50014` `PRIMARY_CONFIG_MSG_HAS_ACTIONABLE=1` `PRIMARY_CONFIG_MSG_NO_RETRY=0` `PRIMARY_CONFIG_ORGS_CREATED=0` `PRIMARY_CONFIG_SITES_CREATED=0` `PRIMARY_CONFIG_LOG_DETAIL=1` | PASS（六项 token 全为期望值） | 文案含「请稍后再试」/英文内部串/组织 id、返回 50000、建出组织或站点 → FAIL | dev |
| R2 | `bash scripts/verify-2597.sh r2 > docs/evidence/2597/R2-primary-transient.txt` | `docs/evidence/2597/R2-primary-transient.txt` | exit 0 且含 `TRANSIENT_STATUS=500` `TRANSIENT_CODE=50000` `TRANSIENT_MSG_RETRY=1` `TRANSIENT_NOT_CONFIG_CODE=1` `TRANSIENT_SITES_CREATED=0` | PASS（五项 token 全为期望值） | 瞬时失败返 50014/4xx、建出站点 → FAIL | dev |
| R3 | `bash scripts/verify-2597.sh r3 > docs/evidence/2597/R3-legacy-flip.txt` | `docs/evidence/2597/R3-legacy-flip.txt` | exit 0 且含 `PRIMARY_ORG_UNRESOLVED=50014` `HARNESS_FLIPPED=1` `DEACTIVATE_MSG_NO_INTERNAL_REF=1` | PASS（三项 token 全为期望值） | token 仍 50000、harness 未翻转、文案仍含 `#1935` → FAIL | dev |
| R4 | `bash scripts/verify-2597.sh r4 > docs/evidence/2597/R4-frontend-resolver.txt` | `docs/evidence/2597/R4-frontend-resolver.txt` | exit 0 且含 `E50014_LAYER=code_map` `E50014_RETRY_HINT=0` `HTTP_THROW_CODE=50014` `ASCII_LEAK=0` `HUMAN_40002=1` `MOBILE_MAP_MATCH=1` `NODE_TEST_FAIL=0` | PASS（七项 token 全为期望值） | 层错、英文外溢、文案含「请稍后再试」、双端文案漂移 → FAIL | dev |
| R5 | `bash scripts/verify-2597.sh r5 > docs/evidence/2597/R5-page-wiring.txt` | `docs/evidence/2597/R5-page-wiring.txt` | exit 0 且含 `RESOLVE_HELPER=1` `RAW_MESSAGE_BRANCHES=0` `TRY_CATCH_HANDLERS=5` `EDIT_CODE_CHECK=1` `PC_MAP_50014=1` `MOBILE_MAP_50014=1` | PASS（六项 token 全为期望值） | 残留原样直出、缺 try/catch、编辑不判码、缺码表条目 → FAIL | dev |
| R6 | `bash scripts/verify-2597.sh r6 > docs/evidence/2597/R6-docs.txt` | `docs/evidence/2597/R6-docs.txt` | exit 0 且含 `API_CODE_50014=1` `API_TEXT_ACTIONABLE=1` `UI_ERROR_LAYER=1` | PASS（三项 token 全为期望值） | 文档缺新码/缺口径 → FAIL | dev |
| R7 | `bash scripts/verify-2597.sh r7 > docs/evidence/2597/R7-build-lint.txt` | `docs/evidence/2597/R7-build-lint.txt` | exit 0 且含 `GO_BUILD_OK=1` `GOFMT_DIFF=0` `ESLINT_NOUNDEF=0` | PASS（三项 token 全为期望值） | 构建失败/gofmt 差异/eslint no-undef → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2597.sh`（`r1`–`r7`）；每个文本 artifact 首行：`# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，另起一行 `GENERATED_AT_HEAD=<40 位 sha>`；证据须在 fix commit **之后**生成（`FIX_COMMIT` 可覆盖锚点）。
- 逐条清单（全部位于 `docs/evidence/2597/`）：
  - `docs/evidence/2597/R1-primary-config.txt`
  - `docs/evidence/2597/R2-primary-transient.txt`
  - `docs/evidence/2597/R3-legacy-flip.txt`
  - `docs/evidence/2597/R4-frontend-resolver.txt`
  - `docs/evidence/2597/R5-page-wiring.txt`
  - `docs/evidence/2597/R6-docs.txt`
  - `docs/evidence/2597/R7-build-lint.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2597 --summary-file tmp/pre-ready-2597-summary.md --changed-files tmp/pre-ready-2597-changed.txt --evidence-file docs/evidence/2597/evidence.json --report docs/evidence/2597/pre-ready-2597.json`
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Dependencies

- 无

### Workload

- Size: M
- Basis: 后端 2 个实现文件 + 3 个测试文件（新增 1、翻转 2）、前端 4 文件（页面 + PC/mobile 码表 + 新单测）、2 份核心文档、1 个 7 行证据 harness + 1 处既有 harness 断言翻转；无 migration、无新端点、无跨仓依赖
- Confidence: high

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状（symptom） | 预生产平台管理员创建中转网点必失败，界面提示「无法解析命名空间主组织，请稍后再试」（或什么都看不到），无法定位也无法行动 | #2597 正文、`backend/backend.log` `namespace has 2 primary organizations` |
| 2 | 触发条件（trigger） | 命名空间存在 0 个或 ≥2 个 `is_primary=true` 组织（持久配置异常）；或 IAM 组织列表不可用（瞬时） | `ResolveNamespacePrimaryOrg` 分支 `iam_client.go:525-529` |
| 3 | 根因（root cause） | 两层提示层缺陷叠加：后端把「持久配置不变量违反」压平成与瞬时故障同一码同一文案；前端该页 5 处分支绕过统一错误层，且 5xx 会抛 `ApiError` 而 handler 无 `try/catch` → 连原始文案都到不了用户 | `transit_site_admin.go:103-108`、`TransitCenter.jsx:46,73,83,100,107`、`api.js:330-339` |
| 4 | 测试盲点（test blind spot） | 既有测试只断言「非恰好 1 个 → 500/50000 且不建站」，**从未断言文案可行动性 / 不含「请稍后再试」**；前端无该页错误路径单测（`frontend-pc/tests` 无 transit 用例） | `transit_site_admin_2552_test.go:502-531` |
| 5 | 同类扫描（same-class paths） | 18 处「请稍后再试」中 1 处本类（`:106`）、3 处同类（`:127`/`user_staff.go:2271`/`:2315` → #2598）、14 处真瞬时（保持） | 本计划「同批扫描判定」表 |
| 6 | 修复层（fix layer） | 修在「错误分类（services 类型化错误）+ handler 文案/码分流 + 前端唯一解析出口 + 双端码表」四层；不碰功能逻辑（fail-fast 不猜、不回落租户保持不变） | 变更点 1–4 |
| 7 | 红绿实证（red-green） | 红：修前 `TestTransitSiteCreate2597_PrimaryConfigInvariant` 断言 50014 必失败（实际 50000），且 message 含「请稍后再试」；既有 `PRIMARY_ORG_UNRESOLVED` token 为 50000。绿：R1/R3 token 翻转成立；Work Summary 需贴修前 red 运行片段 | 本计划 R1/R3 + 翻转清单 |
| 8 | 防复发（prevention） | 文案断言（含「联系平台管理员」、不含「请稍后再试」）+ 类型化错误（禁止字符串匹配）+ 前端「禁用 `r.message`/`e.message` 直出」静态断言 + 双端码表平价断言 | 本计划 R1/R4/R5 |
| 9 | 影响面（blast radius） | 仅创建路径的**失败分支**；成功路径、建组织契约（`type=branch`/父组织）、租户作用域、成员管理、路由管理均不变（#2552/#1935 既有 token 保留） | R3、`transit_site_admin_2552_test.go` 其余 token |
| 10 | 未修边界（deferred） | `:127` 建组织失败分类、`user_staff.go:2271`/`:2315` 的 409 冲突错配 → **#2598**；`api.md` 端点全文补录（文档债）；预生产真机回执 → `[Ops-Followup of #2597]`（`waitForConfirm`，无 `status:*`） | Out of Scope、D9、D10 |

*Model: deepseek/deepseek-flash*

