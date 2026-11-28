## Accepted Plan — #2605 [Bug] 其余绑定失败前缀仍回传 Go 英文原文（22 处 / 9 文件收口 `bindmsg`，含 `error` 泄漏键移除）

### 背景与现状核对（代码为据）

| # | 事实 | 位置 / 命令 |
|---|---|---|
| 1 | 本单共 **22 处 / 9 文件**，全部是 `c.ShouldBindJSON` 失败分支直出原文 | `grep -rn '"invalid request body: " *+ *err.Error()' backend/handlers/*.go \| wc -l` → 6；`"Invalid request data: "` → 3；`"Invalid request: "` → 4；`"Invalid input: "` → 9 |
| 2 | code 分布：**40001 ×18**、**40000 ×4**（`lease.go` 全部 4 处） | 逐点实测（见「22 处落点」表） |
| 3 | `api.go:866` 有**第二个泄漏点**：同一 `gin.H` 还带 `"error": err.Error()`，响应体非 `message` 键也外泄原文 | `sed -n '860,872p' backend/handlers/api.go` |
| 4 | 前端/文档无任何消费方依赖该 `error` 键 | `grep -rn '\\.error' frontend-pc/src/services/apiError.js frontend-pc/src/services/api.js` → 0 命中响应体 error 字段；`docs/spec/api/api.md` 的 `error` 仅出现在乐器导入「逐行失败明细」，与本端点无关 |
| 5 | 唯一出口 `bindmsg` 由 #2603 落地、#2604 扩面（55 称呼 + `ScopePerson`/`ScopeRole`）；本单沿用并追加 27 条称呼 + 2 个作用域 | `backend/bindmsg/bindmsg.go`（依赖 #2604） |
| 6 | 本单 22 端点共 **53 个 JSON 字段名**，其中 **26 条已登记**（address/amount/auto_generate/code/email/force_password_change/id/id_card_no/id_photo_back/id_photo_front/id_photo_other_type/items/name/nickname/notes/parent_id/password/phone/real_name/role/skip_activation/status/type/user_id/user_ids/username），**27 条需新增** | 逐端点请求体解析（见「字段称呼新增表」） |
| 7 | 同名跨域冲突在本单新增 2 处：`code`（`WxBindCurrentUser` 的 wx.login code ≠ 优惠码）、`items`（`UpdateCategorySort` 的分类列表 ≠ 乐器列表） | `user_staff.go:2236`、`api.go:1058` vs #2603 锁定的 `code`→优惠码 / `items`→乐器列表 |
| 8 | `instrument.go` 的 6 处 `invalid images/specifications/pricing format: ` 属**不可达分支**（`json.Marshal` 对 JSON 解码产物不可能失败）→ 拆到 **#2606**（中文常量，无运行时断言） | `backend/handlers/instrument.go:60,67,70,85,86,92`（字段类型）与 `:404,420,444,654,666,678` |
| 9 | 9 个端点在 bind 之前有**前置守卫**，测试必须满足（system-admin / 商户归属 / 网点归属 / 已存在行） | 见「22 处落点」表「前置 / seed」列 |
| 10 | 既有测试**无**断言这 22 处文案 | `grep -rn 'invalid request body\\|Invalid request data\\|Invalid request:\\|Invalid input' --include=*_test.go backend` → 0 |
| 11 | 前端零改动：PC / mobile `resolveErrorCore` L2 = 含 CJK 透传（先于 L3 code 家族）；`ERROR_MESSAGE_MAP` 的英文键全部保留 | `frontend-pc/src/services/errorMessages.js:118-140`、`frontend-mobile/src/services/errorMessageResolution.mjs` |

**结论**：与 #2603 / #2604 同根因（`err.Error()` 直出 + 前端 L2 只透传 CJK → 降级通用文案）。修复 = 22 处收敛到 `bindmsg` 唯一出口 + 27 条称呼 + 2 个作用域 + 移除 `api.go:866` 的 `error` 泄漏键，用「22 端点 × 2 用例」handler 契约测试钉死「无 Go/JSON 原文 / 无 ASCII / code 不漂移」。

### 变更点

#### 1. 基础层：`backend/bindmsg/bindmsg.go`（在 #2604 基础上扩面，唯一 owner）

1. **称呼表新增 27 条**（见「字段称呼新增表」），称呼**不含 ASCII 字母**；已有 26 条直接复用，禁止重复登记（Go map 字面量重复键即编译失败，作为兜底）。
2. **新增 2 个作用域**（追加常量，不改 #2604 已有取值顺序）：

```go
const (
	ScopeDefault  Scope = iota // #2604
	ScopePerson                // #2604: name→姓名, company→工作单位, title→职务
	ScopeRole                  // #2604: name→角色名称, code→角色编码
	ScopeWx                    // #2605: code→微信授权码
	ScopeCategory              // #2605: items→分类列表
)
```

3. `scopeLabels` 追加：`ScopeWx` = `{code: 微信授权码}`；`ScopeCategory` = `{items: 分类列表}`。查表优先级保持「作用域 → 全局 → JSON snake 名回显」。

#### 2. 22 处替换（按域 4 批，每批以 grep 收口）

| 批 | 域 | 文件（处数） | 收口断言 |
|---|---|---|---|
| B1 | 分类 / 租约押金 | `api.go`(3)、`lease.go`(4) = **7** | `grep -c` 对应字面量 = 0 |
| B2 | 商户 / 网点成员 / 中转 / 初始化 | `merchant.go`(2)、`merchant_member.go`(3)、`setup.go`(1)、`site_member.go`(2)、`transit_site_admin.go`(1) = **9** | 同上 |
| B3 | 员工与本人资料 / 微信 | `user_staff.go`(5) = **5** | 同上 |
| B4 | IAM 孤儿清理 | `iam_orphan.go`(1) = **1** | 同上 |

替换规则（逐点机械，**只换 message 表达式**）：

```go
// before
c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "Invalid input: " + err.Error()})
// after（作用域默认；微信/分类/人员/角色域见「22 处落点」表）
c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": bindmsg.Message(err)})
```

- **额外一处**：`api.go:866` 删除 `"error": err.Error(),`（连同其注释），保持 `code` / `message` / HTTP 不变；
- **禁止**改动任何 400 族判定分支、HTTP 状态、code（40000/40001 混杂现状保留）；
- `instrument.go` 的 6 处（#2606 范围）**不得**在本单改动。

#### 3. 22 处落点（file:line / handler / code / Scope / 前置 / 用例 2 / 期望 message）

用例 1 统一为**损坏请求体**（含 `0x17` 控制字符）→ 期望 `请求体格式不正确，请检查后重试`；用例 2 为**单一点名**用例（下表），保证只有 1 个字段出错。

| # | 位置 | handler | code | Scope | 前置 / seed | 用例 2 请求体 | 期望 message |
|---|---|---|---|---|---|---|---|
| 1 | `backend/handlers/iam_orphan.go:163` | `Purge` | 40001 | default | 系统管理员 actor（`TenantID:""` + `Role:"ADMIN"`） | `{"user_ids":"x"}` | `参数不合法：用户列表类型不正确` |
| 2 | `backend/handlers/user_staff.go:205` | `CreateUser` | 40001 | person | 注入 tenantID | `{"username":"staff"}` | `参数不合法：姓名不能为空` |
| 3 | `backend/handlers/user_staff.go:1188` | `UpdateUser` | 40001 | person | seed `User{ID, TenantID:tid, OrgID:oid, Role:"staff", Status:"active"}` + actor `{TenantID:tid, OrgID:oid, Role:"NAMESPACE_ADMIN"}` | `{"name":123}` | `参数不合法：姓名类型不正确` |
| 4 | `backend/handlers/user_staff.go:1651` | `UpdateCurrentUser` | 40001 | default | 注入 userID | `{"nickname":123}` | `参数不合法：昵称类型不正确` |
| 5 | `backend/handlers/user_staff.go:2238` | `WxBindCurrentUser` | 40001 | wx | 注入 userID | `{}` | `参数不合法：微信授权码不能为空` |
| 6 | `backend/handlers/user_staff.go:2294` | `WxMergeCurrentUser` | 40001 | default | 注入 userID | `{"openid":"o"}` | `参数不合法：目标用户不能为空` |
| 7 | `backend/handlers/api.go:866` | `CreateCategory` | 40001 | default | 无 | `{"icon":"x"}` | `参数不合法：名称不能为空` |
| 8 | `backend/handlers/api.go:934` | `UpdateCategory` | 40001 | default | 路由 `:id` | `{"visible":"x"}` | `参数不合法：是否可见类型不正确` |
| 9 | `backend/handlers/api.go:1066` | `UpdateCategorySort` | 40001 | category | 无 | `{}` | `参数不合法：分类列表不能为空` |
| 10 | `backend/handlers/lease.go:129` | `CreateLease` | 40000 | default | 注入 tenantID | `{"user_id":"u","start_date":"2026-01-01","end_date":"2026-02-01","monthly_rent":1,"deposit_amount":1}` | `参数不合法：乐器不能为空` |
| 11 | `backend/handlers/lease.go:188` | `UpdateLease` | 40000 | default | seed `Lease{ID, TenantID:tid, UserID, InstrumentID, StartDate, EndDate}` | `{"monthly_rent":"x"}` | `参数不合法：月租金类型不正确` |
| 12 | `backend/handlers/lease.go:344` | `CreateDeposit` | 40000 | default | 注入 tenantID | `{"lease_id":"l","user_id":"u","type":"rent","transaction_date":"2026-01-01"}` | `参数不合法：金额不能为空` |
| 13 | `backend/handlers/lease.go:400` | `UpdateDeposit` | 40000 | default | seed `Deposit{ID, TenantID:tid, LeaseID, UserID, Type:"rent", TransactionDate}` | `{"status":123}` | `参数不合法：状态类型不正确` |
| 14 | `backend/handlers/merchant.go:200` | `CreateMerchant` | 40001 | default | 注入 tenantID | `{"phone":"1"}` | `参数不合法：名称不能为空` |
| 15 | `backend/handlers/merchant.go:531` | `UpdateMerchant` | 40001 | default | 路由 `:id` | `{"transit_phone":123}` | `参数不合法：中转联系电话类型不正确` |
| 16 | `backend/handlers/merchant_member.go:144` | `AddMember` | 40001 | default | seed `Merchant{ID, TenantID:tid}` + actor tenant=tid | `{"new_users":"x"}` | `参数不合法：新成员列表类型不正确` |
| 17 | `backend/handlers/merchant_member.go:469` | `UpdateMemberRole` | 40001 | default | seed `Merchant{ID, TenantID:tid}` + actor tenant=tid | `{}` | `参数不合法：角色不能为空` |
| 18 | `backend/handlers/merchant_member.go:935` | `TransferOwnership` | 40001 | default | seed `Merchant{ID, TenantID:tid}` + actor tenant=tid | `{"user_id":123}` | `参数不合法：用户类型不正确` |
| 19 | `backend/handlers/setup.go:64` | `InitializeSystem` | 40001 | default | 用例内先清空 `users`（`Unscoped().Delete`），且**必须在 seed 用户之前执行** | `{"email":"bad","password":"12345678"}` | `参数不合法：邮箱格式不正确` |
| 20 | `backend/handlers/site_member.go:90` | `AddMember` | 40001 | default | seed `Site{ID, TenantID:tid}` + actor tenant=tid | `{"new_users":"x"}` | `参数不合法：新成员列表类型不正确` |
| 21 | `backend/handlers/site_member.go:180` | `UpdateMemberRole` | 40001 | default | seed `Site{ID, TenantID:tid}` + actor tenant=tid | `{"roles":"x"}` | `参数不合法：角色列表类型不正确` |
| 22 | `backend/handlers/transit_site_admin.go:304` | `AddTransitSiteMember` | 40001 | default | seed `Site{ID, TenantID:tid, Type:"transit"}` + actor tenant=tid | `{"user_ids":"x"}` | `参数不合法：用户列表类型不正确` |

**fixture → 状态 → 期望值推演**（守卫链，逐类）：
- `Purge`：`requireSystemAdmin` → `GetBusinessRole` 在 `tid==""` 时返回 `system_admin` → 通过 → bind 失败；
- `UpdateUser`：`loadVisibleStaffUser` → `personnelUserVisible` 的 `BusinessRoleSystemAdmin` 分支 → `Role:"staff"` 且 `OrgID==oid` → 可见 → bind 失败；
- `AddMember`/`UpdateMemberRole`/`TransferOwnership`(商户)：`hasMerchantAccess` 查询 `merchants WHERE id=? AND (tenant_id=? OR org_id=?)` → seed 命中 → 通过 → bind 失败；
- `AddMember`/`UpdateMemberRole`(网点)：`hasSiteAccess` 查询 `sites WHERE id=? AND tenant_id=?` → seed 命中 → 通过 → bind 失败；
- `AddTransitSiteMember`：先查 `sites WHERE id=? AND type='transit'` → seed 命中 → 通过 → bind 失败；
- `UpdateLease`/`UpdateDeposit`：先按 `id + tenant_id` 取行 → seed 命中 → 通过 → bind 失败；
- `InitializeSystem`：`users` 计数必须为 0 → 用例内清空 → bind 失败；
- 其余端点 bind 前无 DB 依赖（仅取参 / 取上下文）。

**Lease / Deposit 表处理**：`testfixtures.allTables` 未包含 `models.Lease` / `models.Deposit` → 测试内 `db.AutoMigrate(&models.Lease{}, &models.Deposit{})` 建表，并在 seed 前 `Unscoped().Where("1=1").Delete(...)` 清空（幂等，避免重复运行主键冲突）；**不修改**共享 `testfixtures`（最小爆炸半径）。

#### 4. 字段称呼新增表（27 条，唯一 owner：`backend/bindmsg/bindmsg.go`；**不得含 ASCII 字母**）

| 域 | 新增条目 |
|---|---|
| 人员 | `position`→职位、`site_id`→网点、`user_type`→用户类型、`bio`→个人简介、`new_users`→新成员列表、`roles`→角色列表 |
| 商户 | `merchant_type`→商户类型、`transit_address`→中转地址、`transit_phone`→中转联系电话、`transit_contact_name`→中转联系人、`admin_uid`→管理员标识、`admin_name`→管理员姓名、`admin_username`→管理员账号、`admin_email`→管理员邮箱、`admin_phone`→管理员手机号 |
| 分类 | `icon`→图标、`visible`→是否可见、`sort`→排序值 |
| 租约押金 | `instrument_id`→乐器、`start_date`→起租日期、`end_date`→到期日期、`monthly_rent`→月租金、`deposit_amount`→押金金额、`lease_id`→租约、`transaction_date`→交易日期 |
| 微信 | `openid`→微信身份标识、`to_user_id`→目标用户 |

（上表 27 条逐字锁定；称呼文本改动需重开分析。）

#### 5. 作用域称呼追加表（2 条）

| Scope | 覆盖条目 | 使用端点 |
|---|---|---|
| `ScopeWx` | `code`→微信授权码 | `user_staff.go:2238` |
| `ScopeCategory` | `items`→分类列表 | `api.go:1066` |

`ScopePerson` 复用（#2604 定义）：`user_staff.go:205`、`user_staff.go:1188`。

#### 6. 测试

- `backend/bindmsg/bindmsg_2605_test.go`（新增）：
  - `TestBindMsgLabels2605`：27 条新增称呼逐条等值 + `LABEL_ASCII_LEAK=0`；
  - `TestBindMsgScope2605`：`ScopeWx` 的 `code`=`微信授权码`、`ScopeCategory` 的 `items`=`分类列表`；**默认作用域不漂移**：`code`=`优惠码`、`items`=`乐器列表`；
  - `TestBindMsgCoverage2605`：22 端点 53 个 JSON 名全覆盖（`LABEL_COVERAGE_MISSING=0`）。
- `backend/handlers/bind_error_2605_test.go`（新增）：`TestBindErrorMessages2605` = **22 端点 × 2 用例 = 44 例**，表驱动（序号与「22 处落点」表一致，便于审计对照）；每例断言：
  1. HTTP 400 且 `code` = 表中基线（40000/40001 不漂移）；
  2. `message` 无 ASCII 字母（`[A-Za-z]` 计数 0）；
  3. `message` 不含 `invalid character` / `invalid request` / `Invalid input` / `Invalid request data` / `cannot unmarshal` / `Field validation` / `json:`；
  4. 与期望 `wantMsg` **逐字相等**；`api.go:866` 两例额外断言响应体**不含** `error` 键（`BIND2605_ERROR_KEY_LEAK=0`）；
  5. 聚合 token：`BIND2605_CASES_TOTAL=44`、`BIND2605_CASES_FAIL=0`、`BIND2605_ASCII_LEAK=0`、`BIND2605_RAW_PHRASE_LEAK=0`，抽检 `BIND2605_WX_CODE_MSG`、`BIND2605_CATEGORY_ITEMS_MSG`、`BIND2605_LEASE_MSG`、`BIND2605_SYNTAX_MSG`。
- **红绿实证**：先只落 bindmsg 扩面 + 测试、**不做 22 处替换** → R2 全 44 例 FAIL（message 为 `Invalid input: …` 等原文）；再落替换 → PASS。红样由 R0（base 提交静态证据）持久化，Work Summary 贴红样片段。
- 测试库：一次性 PostgreSQL（`127.0.0.1:15433`，库 `tuneloop_test`，`#2341` 口径），**不使用** `jobmaster-postgres`；不可达即 `exit 2`/FAIL，禁止静默跳过。

#### 7. 证据生成器 `scripts/verify-2605.sh`（`r0`–`r4`，沿用 `scripts/verify-2581.sh` 结构）

- 每个文本 artifact 首行 provenance：`# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，随后 `GENERATED_AT_HEAD=<sha>`；
- 重命令（`go test` / `go build` / `gofmt`）一律经 `~/.config/opencode/scripts/heavy-run.sh` 取锁；
- `r0` 用 `git merge-base origin/develop HEAD` 取 base 提交做「修前」静态计数（确定性，post-fix 可重放）。

### 既有断言影响（0 翻转）

| 检查 | 结果 | 依据 |
|---|---|---|
| Go 测试断言这 22 处文案 | **0 处** | `grep -rn 'invalid request body\|Invalid request data\|Invalid request:\|Invalid input' --include=*_test.go backend` → 0 |
| #2603 / #2604 的 `bindmsg` 断言 | **0 翻转** | 仅追加常量与表项，默认作用域取值不变（R1 断言 `code`=`优惠码`、`items`=`乐器列表`） |
| 前端/文档依赖 `api.go:866` 的 `error` 键 | **0 处** | 背景第 4 条 |
| 需要翻转的语义断言 | **无** | 只改 `message` 文本 + 删除一个未登记的 `error` 键 |

### Files（变更文件清单）

- `backend/bindmsg/bindmsg.go`
- `backend/bindmsg/bindmsg_2605_test.go`
- `backend/handlers/api.go`
- `backend/handlers/iam_orphan.go`
- `backend/handlers/lease.go`
- `backend/handlers/merchant.go`
- `backend/handlers/merchant_member.go`
- `backend/handlers/setup.go`
- `backend/handlers/site_member.go`
- `backend/handlers/transit_site_admin.go`
- `backend/handlers/user_staff.go`
- `backend/handlers/bind_error_2605_test.go`
- `scripts/verify-2605.sh`
- `docs/evidence/2605/plan.md`
- `docs/evidence/2605/R0-red-baseline.txt`
- `docs/evidence/2605/R1-bindmsg-unit.txt`
- `docs/evidence/2605/R2-handler-messages.txt`
- `docs/evidence/2605/R3-residual-scan.txt`
- `docs/evidence/2605/R4-build-compile.txt`
- `docs/evidence/2605/evidence.json`
- `docs/evidence/2605/pre-ready-2605.json`

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R0 修前红样基线（22 处 / 9 文件外泄原文） | `scripts/verify-2605.sh` | base 提交静态计数 | merge-base 提交 vs 修后 HEAD | repo（静态） | `RED_BASE_SITES=22`、`RED_BASE_FILES=9`、`RED_BASE_ASCII_MESSAGES=22`、`RED_HEAD_SITES=0` | R0 |
| R1 bindmsg 扩面（27 条称呼 + `ScopeWx`/`ScopeCategory`） | `backend/bindmsg/bindmsg.go`、`backend/bindmsg/bindmsg_2605_test.go` | `go test ./bindmsg` | 27 条称呼 + 2 作用域 + 默认作用域不漂移 + 53 字段覆盖 | backend（纯函数） | `BINDMSG_UNIT_EXIT=0`、`LABEL_NEW_2605=27`、`LABEL_ASCII_LEAK=0`、`LABEL_COVERAGE_MISSING=0`、`SCOPE_WX_CODE=微信授权码`、`SCOPE_CATEGORY_ITEMS=分类列表`、`SCOPE_DEFAULT_CODE=优惠码`、`SCOPE_DEFAULT_ITEMS=乐器列表` | R1 |
| R2 22 端点绑定失败文案（44 例契约测试） | `backend/handlers/bind_error_2605_test.go` 及 9 个 handler 文件 | `POST/PUT` 22 端点（handler 直调） | 损坏请求体（控制字符）/ 单一点名（缺必填或类型不匹配），共 44 例 | backend（一次性 PG 15433） | `BIND2605_CASES_TOTAL=44`、`BIND2605_CASES_FAIL=0`、`BIND2605_ASCII_LEAK=0`、`BIND2605_RAW_PHRASE_LEAK=0`、`BIND2605_ERROR_KEY_LEAK=0`，且 HTTP 400 + code 与修前一致 | R2 |
| R3 22 处收口 / 其它族不外溢（静态扫描） | `scripts/verify-2605.sh` | 静态扫描 `backend/handlers`（base vs head） | 修后工作树 | repo（静态） | `RESIDUAL_RB_RAW=0`、`RESIDUAL_IRD_RAW=0`、`RESIDUAL_IR_RAW=0`、`RESIDUAL_II_RAW=0`、`SITES_REPLACED_2605=22`、`HANDLER_FILES_2605=9`、`BINDMSG_DELTA_2605=22`、`OUT_OF_SCOPE_NOT_INCREASED=1` | R3 |
| R4 构建 / gofmt / 测试包编译 | `scripts/verify-2605.sh` | `cd backend && go build ./...` 等（经 heavy-run） | 修后工作树 | backend | `GO_BUILD_EXIT=0`、`GOFMT_DIFF=0`、`TEST_COMPILE_EXIT=0` | R4 |

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R0 | `git merge-base origin/develop HEAD` 指向的 base 提交 + 修后 HEAD | `bash scripts/verify-2605.sh r0` | `docs/evidence/2605/R0-red-baseline.txt` | `RED_BASE_SITES=22`、`RED_BASE_FILES=9`、`RED_BASE_ASCII_MESSAGES=22`、`RED_HEAD_SITES=0` | base 计数 ≠22/9、或修后仍残留原文拼接 → FAIL |
| R1 | 27 条新增称呼字面量 + 2 条作用域条目 + 默认作用域对照项 | `go test ./bindmsg -count=1 -v` | `docs/evidence/2605/R1-bindmsg-unit.txt` | `BINDMSG_UNIT_EXIT=0`、`LABEL_NEW_2605=27`、`LABEL_ASCII_LEAK=0`、`LABEL_COVERAGE_MISSING=0`、`SCOPE_WX_CODE=微信授权码`、`SCOPE_CATEGORY_ITEMS=分类列表`、`SCOPE_DEFAULT_CODE=优惠码`、`SCOPE_DEFAULT_ITEMS=乐器列表` | 任一条称呼含 ASCII、默认作用域被改成微信/分类语义、字段漏登记 → 任一即 FAIL |
| R2 | `testfixtures.SetupTestDB` + Lease/Deposit 局部 AutoMigrate + 表驱动 22 端点 × 2 用例（seed/前置见「22 处落点」表） | `go test ./handlers -run '^TestBindErrorMessages2605$' -count=1 -v` | `docs/evidence/2605/R2-handler-messages.txt` | `BIND2605_CASES_TOTAL=44`、`BIND2605_CASES_FAIL=0`、`BIND2605_ASCII_LEAK=0`、`BIND2605_RAW_PHRASE_LEAK=0`、`BIND2605_ERROR_KEY_LEAK=0`、`BIND2605_WX_CODE_MSG=参数不合法：微信授权码不能为空`、`BIND2605_CATEGORY_ITEMS_MSG=参数不合法：分类列表不能为空`、`BIND2605_LEASE_MSG=参数不合法：乐器不能为空`、`BIND2605_SYNTAX_MSG=请求体格式不正确，请检查后重试` | 任一端点返回 `Invalid input`/`Invalid request`/`invalid character`/`cannot unmarshal`、返回非 400、code 漂移（40000/40001 互换）、消息含 ASCII 字母、响应体仍带 `error` 原文键、用例数 <44 → 任一即 FAIL |
| R3 | 修后 `backend/handlers` 全目录 + base 提交对照 | `bash scripts/verify-2605.sh r3` | `docs/evidence/2605/R3-residual-scan.txt` | `RESIDUAL_RB_RAW=0`、`RESIDUAL_IRD_RAW=0`、`RESIDUAL_IR_RAW=0`、`RESIDUAL_II_RAW=0`、`SITES_REPLACED_2605=22`、`HANDLER_FILES_2605=9`、`BINDMSG_DELTA_2605=22`、`OUT_OF_SCOPE_NOT_INCREASED=1` | 残留任一目标字面量、替换点数 ≠22、文件数 ≠9、bindmsg 调用点增量 ≠22、把 `instrument.go` 6 处（#2606）或其它族改成更差 → 任一即 FAIL |
| R4 | 修后工作树 | `bash scripts/verify-2605.sh r4` | `docs/evidence/2605/R4-build-compile.txt` | `GO_BUILD_EXIT=0`、`GOFMT_DIFF=0`、`TEST_COMPILE_EXIT=0` | 构建失败 / gofmt 有差异 / 测试包编译失败 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R0 | `bash scripts/verify-2605.sh r0 > docs/evidence/2605/R0-red-baseline.txt` | `docs/evidence/2605/R0-red-baseline.txt` | exit 0 且含 `RED_BASE_SITES=22` `RED_BASE_FILES=9` `RED_BASE_ASCII_MESSAGES=22` `RED_HEAD_SITES=0` | PASS（4 项 token 全为期望值） | base 计数 ≠22/9、修后仍残留原文拼接 → FAIL | dev |
| R1 | `bash scripts/verify-2605.sh r1 > docs/evidence/2605/R1-bindmsg-unit.txt` | `docs/evidence/2605/R1-bindmsg-unit.txt` | exit 0 且含 `BINDMSG_UNIT_EXIT=0` `LABEL_NEW_2605=27` `LABEL_ASCII_LEAK=0` `LABEL_COVERAGE_MISSING=0` `SCOPE_WX_CODE=微信授权码` `SCOPE_CATEGORY_ITEMS=分类列表` `SCOPE_DEFAULT_CODE=优惠码` `SCOPE_DEFAULT_ITEMS=乐器列表` | PASS（8 项 token 全为期望值） | 称呼含 ASCII、漏登记字段、默认作用域漂移 → FAIL | dev |
| R2 | `bash scripts/verify-2605.sh r2 > docs/evidence/2605/R2-handler-messages.txt` | `docs/evidence/2605/R2-handler-messages.txt` | exit 0 且含 `GO_TEST_EXIT=0` `BIND2605_CASES_TOTAL=44` `BIND2605_CASES_FAIL=0` `BIND2605_ASCII_LEAK=0` `BIND2605_RAW_PHRASE_LEAK=0` `BIND2605_ERROR_KEY_LEAK=0` `BIND2605_WX_CODE_MSG=参数不合法：微信授权码不能为空` `BIND2605_CATEGORY_ITEMS_MSG=参数不合法：分类列表不能为空` `BIND2605_LEASE_MSG=参数不合法：乐器不能为空` `BIND2605_SYNTAX_MSG=请求体格式不正确，请检查后重试` | PASS（10 项 token 全为期望值） | 任一端点含 Go/JSON 原文或 ASCII 字母、HTTP/code 漂移、响应体仍带 `error` 键、用例数 <44 → FAIL | dev |
| R3 | `bash scripts/verify-2605.sh r3 > docs/evidence/2605/R3-residual-scan.txt` | `docs/evidence/2605/R3-residual-scan.txt` | exit 0 且含 `RESIDUAL_RB_RAW=0` `RESIDUAL_IRD_RAW=0` `RESIDUAL_IR_RAW=0` `RESIDUAL_II_RAW=0` `SITES_REPLACED_2605=22` `HANDLER_FILES_2605=9` `BINDMSG_DELTA_2605=22` `OUT_OF_SCOPE_NOT_INCREASED=1` | PASS（8 项 token 全为期望值） | 残留目标字面量、替换点/文件数不符、越界改动其它族 → FAIL | dev |
| R4 | `bash scripts/verify-2605.sh r4 > docs/evidence/2605/R4-build-compile.txt` | `docs/evidence/2605/R4-build-compile.txt` | exit 0 且含 `GO_BUILD_EXIT=0` `GOFMT_DIFF=0` `TEST_COMPILE_EXIT=0` | PASS（3 项 token 全为期望值） | 构建失败 / gofmt 差异 / 测试包编译失败 → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2605.sh`（`r0`–`r4`）；每个文本 artifact 首行：`# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，随后 `GENERATED_AT_HEAD=<40 位 sha>`；证据必须在 fix commit **之后**生成（`FIX_REF` 可覆盖锚点，默认 `git log --grep="^fix(issue-2605)"`）。
- 重命令（`go test` / `go build` / `gofmt`）一律经 `~/.config/opencode/scripts/heavy-run.sh` 取锁；handler 测试用一次性 PostgreSQL（`127.0.0.1:15433`，库 `tuneloop_test`，`#2341` 口径），**不**使用 `jobmaster-postgres`。
- 逐条清单（全部位于 `docs/evidence/2605/`）：
  - `docs/evidence/2605/R0-red-baseline.txt`
  - `docs/evidence/2605/R1-bindmsg-unit.txt`
  - `docs/evidence/2605/R2-handler-messages.txt`
  - `docs/evidence/2605/R3-residual-scan.txt`
  - `docs/evidence/2605/R4-build-compile.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2605 --summary-file tmp/pre-ready-2605-summary.md --changed-files tmp/pre-ready-2605-changed.txt --evidence-file docs/evidence/2605/evidence.json --report docs/evidence/2605/pre-ready-2605.json`
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Input Truth & Tool Preflight

- **Source of truth**：
  - 22 处清单 = 4 条 `grep -rn` 计数（修前 6/3/4/9，修后全 0）；
  - code 基线 = 逐点实测（40001 ×18 / 40000 ×4），**不得**统一；
  - 文案模板与称呼 = `backend/bindmsg/bindmsg.go`（唯一 owner；#2603 88 条 + #2604 55 条 + 本单 27 条，作用域 #2604 2 个 + 本单 2 个）；
  - 前端层序 = `resolveErrorCore`（L2 含 CJK 透传，先于 L3 code 家族）。
- **实施前差异核对（先跑，结果入 Work Summary）**：
  - `grep -rn '"invalid request body: " *+ *err.Error()' backend/handlers/*.go | wc -l` → 修前 **6**；修后 **0**；
  - `grep -rn '"Invalid request data: " *+ *err.Error()' backend/handlers/*.go | wc -l` → 修前 **3**；修后 **0**；
  - `grep -rn '"Invalid request: " *+ *err.Error()' backend/handlers/*.go | wc -l` → 修前 **4**；修后 **0**；
  - `grep -rn '"Invalid input: " *+ *err.Error()' backend/handlers/*.go | wc -l` → 修前 **9**；修后 **0**；
  - `grep -rn '"error": *err.Error()' backend/handlers/api.go | wc -l` → 修前 **1**；修后 **0**；
  - `grep -rn '"invalid \\(images\\|specifications\\|pricing\\) format: "' backend/handlers/instrument.go | wc -l` → **6**（#2606 范围，本单不动）；
  - `grep -rho 'bindmsg.Message' backend/handlers/*.go | wc -l` → 修前 **53**（#2603 28 + #2604 25）；修后 **75**（增量 22）。
- **损坏输入拒绝规则**（测试必须覆盖，且都必须是中文）：
  - 控制字符 / 截断 / 多余逗号 → `请求体格式不正确，请检查后重试`；
  - 字段类型不符 → `参数不合法：<称呼>类型不正确`；
  - 未登记称呼 → 回显 JSON snake 名，**绝不**回显 Go 字段名；
  - 22 端点响应**不得**出现 ASCII 字母，且 `api.go:866` 响应体不得出现 `error` 键。
- **工具盲区**：
  - handler 契约测试只覆盖 handler 层（不经 `main.go` 路由、不经 nginx、不经 IAM）；本单不改路由；
  - `merchant_member` / `site_member` / `transit_site_admin` 的归属守卫用 seed 行命中，**不**验证 IAM 侧绑定行为（非本单范围）；
  - 前端展示链路由 #2603 / #2480 层序单测覆盖，本单不重复；真机复验归部署侧（D9）；
  - `TestBindErrorMessages2605` 依赖一次性 PG；不可达必须 `exit 2`/FAIL，禁止静默跳过；
  - 全量 `go test ./...` 不在合同行（P5 baseline 差分承担）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 唯一出口 = `backend/bindmsg.Message(err, scopes ...Scope)`；不新建 handler 层文件 | 本计划锁定 |
| D2 | 文案模板沿用 #2603 的 9 条逐字模板；任何改动需重开分析 | 本计划锁定 |
| D3 | 新增 27 条称呼 + 2 条作用域逐字锁定；称呼不得含 ASCII 字母 | 本计划锁定 |
| D4 | 同名跨域冲突用 `Scope`（`ScopeWx` / `ScopeCategory`），仅 2 个端点显式传作用域；禁止 handler 内联 label pin | 本计划锁定 |
| D5 | 只替换 `message` 表达式，**不改** HTTP / code / 分支顺序；`lease.go` 保持 40000、其余保持 40001 | 本计划锁定 |
| D6 | `api.go:866` 删除 `"error": err.Error()` 键（未登记的调试字段，前端/文档零依赖，见背景第 4 条）；其余字段不变 | 本计划锁定 |
| D7 | 前端（PC / H5 / weapp）**零改动**：L2 含 CJK 透传；`ERROR_MESSAGE_MAP` 英文键全部保留 | 本计划锁定 |
| D8 | 不新增 `docs/spec/api/api.md` 条款：#2603 §1.3 的绑定失败口径已覆盖本类（P7 为 N/A） | 本计划锁定 |
| D9 | 真机复验（用户是否真看到点名字段）归部署侧；closing 时建 `[Ops-Followup of #2605]`（`waitForConfirm`，无 `status:*`） | 本计划锁定 |
| D10 | `instrument.go` 6 处不可达 Marshal 分支 → **#2606**；本单只断言其计数不增加（`OUT_OF_SCOPE_NOT_INCREASED=1`） | #2606 |
| D11 | Lease / Deposit 表在测试内 `AutoMigrate` + 自清，**不修改**共享 `testfixtures.allTables`（最小爆炸半径） | 本计划锁定 |
| D12 | 重命令经 `heavy-run.sh`；handler 测试用一次性 PG（`127.0.0.1:15433`，库 `tuneloop_test`），不碰 dev 库 | 本计划锁定 |
| D13 | 依赖未满足即 `HOLD`：#2604 必须 `status:done`/`ready`（`bindmsg` 的作用域与称呼先落地）；禁止在本单自建同名出口 | 本计划锁定 |

### Out of Scope

- `instrument.go` 的 6 处不可达 Marshal 分支（`invalid images/specifications/pricing format: `）→ **#2606**（已建，依赖 #2604）。
- 裸 `"message": err.Error()`（30 处，多为 DB/内部错误）与非绑定类英文 message（`instrument id is required` / `Merchant not found` 等）→ 后续收敛批次。
- `docs/spec/api/api.md` 条款变更（#2603 已确立绑定失败口径）→ 本单不改（D8）。
- 任何前端改动（PC / H5 / weapp），含 `ERROR_MESSAGE_MAP` / `ERROR_CODE_MAP` 键调整。
- 400 族其它语义分支、状态码归一（40000/40001 混杂现状保留）。
- IAM 绑定/归属行为的验证（本单只用 seed 行满足本地守卫）。
- 全量 `go test ./...` 作为合同行（由 pre-ready P5 baseline 差分承担）。

### Dependencies

- #2604

### Workload

- Size: L
- Basis: 1 个 leaf 包扩面 + 9 个 handler 文件（22 点）+ 2 个测试文件（44 例 + 单元）+ 1 个证据 harness + 5 条证据行（R0–R4）；跨 Issue 依赖 1 个（#2604）
- Confidence: medium

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状（symptom） | 请求体损坏/缺字段时响应 `40000`/`40001` + `Invalid input: …` / `invalid request body: …`；PC/H5 用户只看到通用「请求参数有误」 | #2605 正文、#2603 预生产实测 |
| 2 | 触发条件（trigger） | 任一 22 处 `c.ShouldBindJSON` 失败：缺必填 / 类型不符 / JSON 语法损坏 | 22 处逐点核对 |
| 3 | 根因（root cause） | handler 把 `err.Error()` 直接拼进 `message`（`api.go:866` 还额外拼进 `error` 键）；前端 L2 只透传 CJK → 降级 generic 文案 | 22 处字面量、`errorMessageResolution.mjs` |
| 4 | 测试盲点（test blind spot） | 全仓无测试断言这 22 处 message；「构建通过 + 状态码」被当成验证充分 | `grep --include=*_test.go` → 0 |
| 5 | 同类扫描（same-class paths） | #2603（28）、#2604（25）、本单（22）、#2606（6，不可达）分单收口；`instrument.go` 不可达不写运行时断言 | 背景第 8 条、D10 |
| 6 | 修复层（fix layer） | 提示层单点收口：`bindmsg` 翻译 + 称呼/作用域 + 22 处替换 + 删除 `error` 调试键；不碰业务逻辑/状态码 | 变更点 1–2 |
| 7 | 红绿实证（red-green） | 红：仅落 bindmsg 扩面 + 测试（无替换）→ 44 例全 FAIL；R0 持久化 base 计数 22/9；绿：替换后 R2 全绿 | R0、变更点 6 |
| 8 | 防复发（prevention） | ① 四条字面量收口扫描全 0；② 每例「message 无 ASCII」硬断言；③ 称呼表 ASCII 泄漏断言；④ `error` 键泄漏断言；⑤ 其它族计数不增加 | R1/R2/R3 |
| 9 | 影响面（blast radius） | 仅失败分支的 `message` 文本 + 一个未登记 `error` 键；成功路径、状态码、鉴权、DB 写入不变 | R2 的 code 断言、R3 |
| 10 | 未修边界（deferred） | #2606（6 处不可达）、裸 `err.Error()` 30 处、真机复验 → `[Ops-Followup of #2605]` | Out of Scope、D9、D10 |

*Model: deepseek/deepseek-flash*

