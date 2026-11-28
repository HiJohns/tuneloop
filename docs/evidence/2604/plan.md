## Accepted Plan — #2604 [Bug] 请求体解析失败仍回传 Go 英文原文（25 处 / 17 文件收口 `bindmsg`，含作用域称呼与 `dive` 下标归一）｜Rev.2（裁定 O1+O4）
> **Rev.2 说明（采纳 Investigation Packet 的 O1 + O4）**：
> ① **O1**：`phone` 沿用 #2603 的全局称呼「手机号」，**不新增 `ScopeAddress`** —— 端点表 #15（`user_address.go:186` `UpdateAddress`）的期望文案改为 `参数不合法：手机号类型不正确`，R2/G0 R2 的 `wantMsg` 同步；
> ② **O4**：称呼新增实际为 **56 条**（`user_id` 并不在 #2603 的既有表里），故 `LABEL_NEW_2604` 由 55 回填为 **56**，并新增 `user_id`→用户编号；
> ③ 不采纳 O2/O3（O2 会把作用域机制从 3 值扩到 4 值、D3/D4 口径连带改；O3 会翻转 #2603 既有「手机号」断言，跨单回归）。

### 背景与现状核对（代码为据）

| # | 事实 | 位置 / 命令 |
|---|---|---|
| 1 | 现状共 **25 处 / 17 文件** 直出解析层原文：`"invalid request: " + err.Error()` | `grep -rn '"invalid request: " + err.Error()' backend/handlers/*.go \| wc -l` → 25；`grep -rln … \| wc -l` → 17 |
| 2 | 全部 25 处紧跟 `if err := c.ShouldBindJSON(&req); err != nil {`，**绑定失败即 return**；前置只有取参 / id 判空 / 上下文读取，无鉴权分支 | 逐点核对（见「25 处落点」表「绑定点前置」列） |
| 3 | **Issue 正文的 code 列有 3 处笔误**：`confirmation_session.go:44` 实为 **40001**（非 40002）、`pricing.go:119`/`pricing.go:209` 实为 **40001**（非 40002） | `sed -n '44p' backend/handlers/confirmation_session.go`、`sed -n '119p;209p' backend/handlers/pricing.go`（多行 `gin.H`，code 在下一行） |
| 4 | 真实 code 分布：**40002 ×16**、**40001 ×5**（confirmation_session 1 + face_verify 2 + pricing 2）、**40000 ×4**（permission_manage 2 + role_manage 2） | 同上逐点实测；本单**不得**统一 code（D5） |
| 5 | 唯一出口 `bindmsg.Message(err)` 由 **#2603** 落地（leaf 包，当前主工作树未提交；本单依赖其合并） | `backend/bindmsg/bindmsg.go`（#2603 计划「变更点 1」） |
| 6 | #2603 的称呼表只登记其 26 端点字段（27 + 61 = 88 条）且**明确不做 `dive` 嵌套特化**（其 26 端点无 `dive` 标签，分支不可达） | #2603 Decision Lock D4 |
| 7 | 本单 25 端点共 **71 个 JSON 字段名**，其中 **15 条已在 #2603 登记**（address/code/company/email/id/items/merchant_id/name/password/phone/postal_code/sort_order/status/title/username），**56 条需新增**（含 `user_id`→用户编号） | 逐端点请求体解析（见「字段称呼新增表」） |
| 8 | 本单**确有 `dive`**：`instrument.go:1103` 的 `IDs []string \`binding:"required,dive,required"\``，validator 会给出 `IDs[0]` 形态字段名 → #2603 的 `camelToSnake("IDs")` 输出 `i_ds`（ASCII 泄漏），必须修「复数缩写」+「下标归一」 | `grep -rn 'dive' backend/handlers/instrument.go` → 1 |
| 9 | **同名跨域冲突**（#2603 全局表会给出误导文案）：`code` 在优惠券域=优惠码、在角色域=角色编码；`name` 在网点域=名称、在人员域=姓名；`company` 在仓储域=物流公司、在担保人域=工作单位；`title` 在内容域=标题、在担保人域=职务 | `coupon_pricing.go:64`（`code`→优惠码，受 #2603 断言锁定）vs `role_manage.go:125`（角色 code） |
| 10 | 既有测试**无**断言这 25 处文案 | `grep -rn 'invalid request' --include=*_test.go backend` → 仅 1 条注释性命中（`update_current_user_2446_test.go:150`） |
| 11 | 前端零改动：PC / mobile `resolveErrorCore` 层序 L2 = 含 CJK 透传（先于 L3 code 家族），中文文案直达用户；`ERROR_MESSAGE_MAP['invalid request']` 键**保留**（仍被只写英文短串的其它 handler 使用） | `frontend-pc/src/services/errorMessages.js:118-140`、`frontend-mobile/src/services/errorMessageResolution.mjs` |
| 12 | 同族残差（不同字面量，28 处 / 10 文件：`invalid request body: `6 / `Invalid request data: `3 / `Invalid request: `4 / `Invalid input: `9 / `invalid * format: `6）**不在本单**，已另立 **#2605** | `grep -rn` 见 #2605 正文 |

**结论**：缺陷在提示层（用户看不到「哪个字段错了」）且与 #2603 同根因。修复 = 25 处 `message` 表达式收敛到 `bindmsg` 唯一出口 + 补齐本单 56 条字段称呼 + 修正 `camelToSnake` 复数缩写与 `dive` 下标归一 + 引入**作用域称呼**（仅解决 4 个同名跨域冲突）+ 用「25 端点 × 2 用例」的 handler 契约测试把「不得出现 Go/JSON 原文 / 不得出现 ASCII 字段名 / code 不变」钉成机器可判定负例。

### 变更点

#### 1. 基础层：`backend/bindmsg/bindmsg.go`（唯一 owner，本单在其上扩面）

1. **称呼表新增 56 条**（见「字段称呼新增表」），称呼**不含 ASCII 字母**；已有 15 条（#2603）直接复用，**禁止重复登记**（Go map 字面量重复键即编译失败，作为兜底）。
2. **`camelToSnake` 复数缩写修正**：`IDs`→`ids`、`UserIDs`→`user_ids`；`TransitSiteID`→`transit_site_id`、`Name`→`name` 保持不变（#2603 既有断言不得回归）。规则：先把尾部 `[A-Z]{2,}s` 整体小写化（`UserIDs`→`UserIds`），再走既有逐字符算法；整串全大写（`ID`/`URL`）直接小写。
3. **`dive` 下标归一**：`bindFieldLabel` 查表前先剥离尾部 `[<digits>]`（`IDs[0]`→`IDs`→`ids`→`乐器列表`）。这是 #2603 D4「不做嵌套特化」的**受控例外**——其不可达，本单 `instrument.go:1103` 可达（背景第 8 条）。
4. **作用域称呼**（向后兼容，`Message(err)` 调用点与 #2603 测试零改动）：

```go
// Scope 标识同一 JSON 名在不同域中含义不同的场景（#2604）。
type Scope int

const (
	ScopeDefault Scope = iota
	ScopePerson // name→姓名, company→工作单位, title→职务
	ScopeRole   // name→角色名称, code→角色编码
)

// Message 签名保持 bindmsg.Message(err) 可用（变参默认 ScopeDefault）。
func Message(err error, scopes ...Scope) string
```

**查表优先级**：`scopeLabels[scope][jsonName]` → `bindFieldLabels[jsonName]` → 回显 JSON snake 名（绝不回显 Go 字段名）。作用域表**仅 5 条**，见「作用域称呼表」。

#### 2. 25 处替换（按域 5 批，每批以 grep 收口）

| 批 | 域 | 文件（处数） | 收口断言 |
|---|---|---|---|
| B1 | 认证 / 注册 / 核身 | `auth.go`(2)、`confirmation_session.go`(1)、`registration_session.go`(2)、`face_verify.go`(2) = **7** | `grep -c '"invalid request: " + err.Error()' <file>` = 0 |
| B2 | 权限 / 角色 / 平台员工 / 邮件 | `permission_manage.go`(2)、`role_manage.go`(2)、`platform_staff.go`(1)、`smtp_config.go`(1) = **6** | 同上 |
| B3 | 用户 / 地址 / 证件 / 担保人 | `user_address.go`(2)、`user_management.go`(1)、`user_management_idcard.go`(1)、`user_onboarding.go`(1)、`guarantor.go`(2) = **7** | 同上 |
| B4 | 乐器 / 媒体 / 价格 | `instrument.go`(1)、`instrument_media.go`(1)、`pricing.go`(2) = **4** | 同上 |
| B5 | 结算 | `billing.go`(1) = **1** | 同上 |

替换规则（逐点机械，**只换 message 表达式**）：

```go
// before
c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "invalid request: " + err.Error()})
// after（作用域默认；人员/角色域见「作用域称呼表」）
c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": bindmsg.Message(err)})
```

- 新增 `import "tuneloop-backend/bindmsg"`。
- **禁止**改动任何 400 族判定分支、HTTP 状态、code（40000/40001/40002 混杂现状保留）、分支顺序。
- 非本单字面量（`invalid request body: ` / `Invalid input: ` 等 28 处）**原样保留**，并由 R3 断言「base 与 head 计数相等」（防顺手扩面）。

#### 3. 25 处落点（file:line / handler / code / 作用域 / 用例 2 夹具 / 期望 message）

用例 1 统一为**损坏请求体**（`{"a":"<0x17>"}` 控制字符）→ 期望 `请求体格式不正确，请检查后重试`；用例 2 为**单一点名**用例（下表），保证只有 1 个字段出错：

| # | 位置 | handler | code | Scope | 用例 2 请求体 | 期望 message |
|---|---|---|---|---|---|---|
| 1 | `backend/handlers/auth.go:346` | `PostLogin` | 40002 | default | `{"identifier":"13000000000"}` | `参数不合法：密码不能为空` |
| 2 | `backend/handlers/auth.go:405` | `PostRegister` | 40002 | person | `{"nickname":"复验","phone":"13000000000"}` | `参数不合法：姓名不能为空` |
| 3 | `backend/handlers/confirmation_session.go:44` | `Create` | 40001 | default | `{"user_id":"u","confirm_type":"email","action_type":"site_staff"}` | `参数不合法：确认目标不能为空` |
| 4 | `backend/handlers/registration_session.go:133` | `CreateRegistrationSession` | 40002 | person | `{"nickname":"n","name":"张","phone":"13000000000","id_photos":"x"}` | `参数不合法：证件照类型不正确` |
| 5 | `backend/handlers/registration_session.go:391` | `RebindRegistrationSession` | 40002 | default | `{"wx_code":123}` | `参数不合法：微信授权码类型不正确` |
| 6 | `backend/handlers/face_verify.go:46` | `Token` | 40001 | person | `{"name":"张三"}` | `参数不合法：身份证号不能为空` |
| 7 | `backend/handlers/face_verify.go:105` | `Result` | 40001 | default | `{}` | `参数不合法：核身凭证不能为空` |
| 8 | `backend/handlers/permission_manage.go:119` | `SetUserPermissions` | 40000 | default | `{}` | `参数不合法：权限项列表不能为空` |
| 9 | `backend/handlers/permission_manage.go:168` | `SetUserRole` | 40000 | default | `{}` | `参数不合法：角色编码不能为空` |
| 10 | `backend/handlers/role_manage.go:125` | `CreateRole` | 40000 | role | `{"name":"r","cus_perm_codes":["x"]}` | `参数不合法：角色编码不能为空` |
| 11 | `backend/handlers/role_manage.go:176` | `UpdateRole` | 40000 | role | `{"name":123,"cus_perm_codes":["x"]}` | `参数不合法：角色名称类型不正确` |
| 12 | `backend/handlers/platform_staff.go:145` | `Create` | 40002 | person | `{"username":"staff"}` | `参数不合法：姓名不能为空` |
| 13 | `backend/handlers/smtp_config.go:71` | `UpdateSMTPConfig` | 40002 | default | `{"port":"x"}` | `参数不合法：端口类型不正确` |
| 14 | `backend/handlers/user_address.go:108` | `CreateAddress` | 40002 | default | `{"province":123}` | `参数不合法：省份类型不正确` |
| 15 | `backend/handlers/user_address.go:186` | `UpdateAddress` | 40002 | default | `{"phone":123}` | `参数不合法：手机号类型不正确`（O1：沿用全局称呼，不新增地址作用域） |
| 16 | `backend/handlers/user_management.go:142` | `Update` | 40002 | default | `{"status":123}` | `参数不合法：状态类型不正确` |
| 17 | `backend/handlers/user_management_idcard.go:36` | `UpdateIdCard` | 40002 | default | `{"real_name":123}` | `参数不合法：真实姓名类型不正确` |
| 18 | `backend/handlers/user_onboarding.go:70` | `CompleteOnboarding` | 40002 | person | `{"name":123}` | `参数不合法：姓名类型不正确` |
| 19 | `backend/handlers/guarantor.go:77` | `CreateGuarantor` | 40002 | person | `{"name":"张三","phone":"13000000000","id_photo_front":"a","id_photo_back":"b","other_cert_photo":"c"}` | `参数不合法：身份证号不能为空` |
| 20 | `backend/handlers/guarantor.go:144` | `OCRIDCard` | 40002 | default | `{"storage_key":"k"}` | `参数不合法：证件面不能为空` |
| 21 | `backend/handlers/instrument.go:1103` | `BatchDeleteInstruments` | 40002 | default | `{"ids":[""]}` | `参数不合法：乐器列表不能为空` |
| 22 | `backend/handlers/instrument_media.go:76` | `CreateInstrumentMedia` | 40002 | default | `{"files":"x"}` | `参数不合法：文件列表类型不正确` |
| 23 | `backend/handlers/pricing.go:119` | `UpdateMerchantPricingConfig` | 40001 | default | `{"template_id":"t"}` | `参数不合法：配置项不能为空` |
| 24 | `backend/handlers/pricing.go:209` | `BatchSetInstrumentPricing` | 40001 | default | `{}` | `参数不合法：乐器列表不能为空` |
| 25 | `backend/handlers/billing.go:303` | `UpsertSettlementConfig` | 40002 | default | `{"profit_share_ratio":"x"}` | `参数不合法：分成比例类型不正确` |

**绑定点前置**（测试须满足，否则走不到 bind）：`face_verify` 需注入 userID；`pricing.go:119` 需非空 tenantID；`smtp_config` / `platform_staff` 需 system-admin 上下文（`TestActor{TenantID:"",Role:"ADMIN"}`），`platform_staff` 另需非空 orgID；`user_address` / `user_onboarding` 走 `EnsureLocalUser`（注入 userID + 测试库）；`billing` 路由需 `:id`；`instrument_media` 路由需 `:id`；其余仅需 `httptest` + 注入上下文。

#### 4. 字段称呼新增表（56 条，唯一 owner：`backend/bindmsg/bindmsg.go`；**不得含 ASCII 字母**）

| 域 | 新增条目 |
|---|---|
| 账户/注册 | `identifier`→登录账号、`nickname`→昵称、`wx_code`→微信授权码、`exchange_token`→微信授权凭证、`ref`→推荐码、`agreed_terms`→协议同意 |
| 权限/角色 | `cus_perm_codes`→权限项列表、`role_code`→角色编码、`auto_generate`→自动生成密码、`force_password_change`→强制修改密码 |
| 身份/证件 | `biz_token`→核身凭证、`id_card_no`→身份证号、`id_card_expire`→身份证有效期、`id_card_authority`→身份证签发机关、`id_card_address`→身份证地址、`real_name`→真实姓名、`id_photo_front`→证件正面照、`id_photo_back`→证件反面照、`other_cert_photo`→其它证件照、`id_photos`→证件照、`id_photo_other_type`→其它证件类型、`storage_key`→文件标识、`side`→证件面 |
| 确认/结算 | `confirm_type`→确认方式、`confirm_target`→确认目标、`action_type`→操作类型、`action_target_id`→操作对象、`receiver_type`→收款方类型、`receiver_account`→收款方账号、`profit_share_ratio`→分成比例、`is_enabled`→启用开关 |
| 乐器/媒体/价格 | `ids`→乐器列表、`batch_type`→批次类型、`is_display`→展示开关、`object_type`→对象类型、`object_id`→对象标识、`files`→文件列表、`file_key`→文件标识、`file_type`→文件类型、`template_id`→价格模板、`config`→配置项、`base_daily_rate`→基础日租金、`overrides`→覆盖项 |
| 用户/地址 | `membership_level_id`→会员等级、`recipient_name`→收件人、`province`→省份、`city`→城市、`district`→区县、`detail`→详细地址、`is_default`→默认地址 |
| 邮件 | `host`→邮件服务器、`port`→端口、`from`→发件邮箱、`use_tls`→加密传输、`enabled`→启用状态 |
| 通用 | `user_id`→用户编号 |

（上表 56 条逐字锁定；称呼文本改动需重开分析。）

#### 5. 作用域称呼表（5 条；仅解决同名跨域冲突，全局表保持单一 owner）

| Scope | 覆盖条目 | 使用端点 |
|---|---|---|
| `ScopePerson` | `name`→姓名、`company`→工作单位、`title`→职务 | `auth.go:405`、`face_verify.go:46`、`platform_staff.go:145`、`registration_session.go:133`、`user_onboarding.go:70`、`guarantor.go:77` |
| `ScopeRole` | `name`→角色名称、`code`→角色编码 | `role_manage.go:125`、`role_manage.go:176` |

#### 6. 测试

- `backend/bindmsg/bindmsg_2604_test.go`（新增）：
  - `TestBindMsgLabels2604`：56 条新增称呼逐条等值 + `LABEL_ASCII_LEAK=0`（`[A-Za-z]` 计数为 0）；
  - `TestBindMsgScope2604`：`Message(err, ScopePerson)` 的 `name`=`姓名`、`company`=`工作单位`、`title`=`职务`；`Message(err, ScopeRole)` 的 `code`=`角色编码`、`name`=`角色名称`；**默认作用域不漂移**：`Message(err)` 的 `code`=`优惠码`、`name`=`名称`；
  - `TestBindMsgCamelToSnake2604`：`IDs`→`ids`、`UserIDs`→`user_ids`、`ID`→`id`，且 `TransitSiteID`/`ManagerPhone` 保持 #2603 期望；
  - `TestBindMsgDiveIndex2604`：`IDs[0]`/`IDs[12]` 与 `IDs` 得同一称呼；
  - `TestBindMsgCoverage2604`：25 端点 71 个 JSON 名字全覆盖（`LABEL_COVERAGE_MISSING=0`，未登记回落 JSON snake 名）。
- `backend/handlers/bind_error_2604_test.go`（新增）：`TestBindErrorMessages2604` = **25 端点 × 2 用例 = 50 例**，表驱动 `{name, method, path, actor, seed, body, wantCode, wantMsg}`；每例断言：
  1. HTTP 400 且 `code` = 表中基线（40000/40001/40002 不漂移）；
  2. `message` 无 ASCII 字母（`[A-Za-z]` 计数 0）；
  3. `message` 不含 `invalid character` / `invalid request` / `invalid parameters` / `cannot unmarshal` / `Field validation` / `json:`；
  4. 与 `wantMsg` **逐字相等**；
  5. 聚合 token：`BIND2604_CASES_TOTAL=50`、`BIND2604_CASES_FAIL=0`、`BIND2604_ASCII_LEAK=0`、`BIND2604_RAW_PHRASE_LEAK=0`，以及抽检 `BIND2604_ROLE_CODE_MSG`、`BIND2604_PERSON_NAME_MSG`、`BIND2604_IDS_MSG`、`BIND2604_SYNTAX_MSG`。
- **红绿实证**：先只落 bindmsg 扩面 + 测试、**不做 25 处替换** → R2 全 50 例 FAIL（message 为 `invalid request: …`）；再落替换 → PASS。红样同时由 R0（base 提交静态证据）持久化，Work Summary 贴红样片段。
- 测试库：一次性 PostgreSQL（`127.0.0.1:15433`，库 `tuneloop_test`，`#2341` 口径），**不使用** `jobmaster-postgres`；不可达即 `exit 2`/FAIL，禁止 `TUNELOOP_TEST_ALLOW_SKIP=1` 静默跳过。

#### 7. 证据生成器 `scripts/verify-2604.sh`（`r0`–`r4`，沿用 `scripts/verify-2581.sh` 结构）

- 每个文本 artifact 首行 provenance：`# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，随后 `GENERATED_AT_HEAD=<sha>`；
- 重命令（`go test` / `go build` / `gofmt`）一律经 `~/.config/opencode/scripts/heavy-run.sh` 取锁；
- `r0` 用 `git merge-base origin/develop HEAD` 取 base 提交做「修前」静态计数（确定性，post-fix 可重放）。

### 既有断言影响（0 翻转）

| 检查 | 结果 | 依据 |
|---|---|---|
| Go 测试断言这 25 处文案 | **0 处** | `grep -rn 'invalid request' --include=*_test.go backend` → 1 条注释性命中（`update_current_user_2446_test.go:150`） |
| #2603 的 `bindmsg` 断言 | **0 翻转** | `Message(err)` 变参向后兼容；默认作用域取值不变（R1 断言 `code`=`优惠码`、`name`=`名称`）；`TransitSiteID` 等既有 camel 期望保持不变 |
| 前端测试断言英文原文 | **0 处** | `ERROR_MESSAGE_MAP['invalid request']` / `['invalid request body']` 键保留（仍被只写英文短串的其它 handler 使用） |
| 需要翻转的语义断言 | **无** | 本单只改 `message` 文本，不改 code / HTTP / 分支判定 |

### Files（变更文件清单）

- `backend/bindmsg/bindmsg.go`
- `backend/bindmsg/bindmsg_2604_test.go`
- `backend/handlers/auth.go`
- `backend/handlers/billing.go`
- `backend/handlers/confirmation_session.go`
- `backend/handlers/face_verify.go`
- `backend/handlers/guarantor.go`
- `backend/handlers/instrument.go`
- `backend/handlers/instrument_media.go`
- `backend/handlers/permission_manage.go`
- `backend/handlers/platform_staff.go`
- `backend/handlers/pricing.go`
- `backend/handlers/registration_session.go`
- `backend/handlers/role_manage.go`
- `backend/handlers/smtp_config.go`
- `backend/handlers/user_address.go`
- `backend/handlers/user_management.go`
- `backend/handlers/user_management_idcard.go`
- `backend/handlers/user_onboarding.go`
- `backend/handlers/bind_error_2604_test.go`
- `scripts/verify-2604.sh`
- `docs/evidence/2604/plan.md`
- `docs/evidence/2604/R0-red-baseline.txt`
- `docs/evidence/2604/R1-bindmsg-unit.txt`
- `docs/evidence/2604/R2-handler-messages.txt`
- `docs/evidence/2604/R3-residual-scan.txt`
- `docs/evidence/2604/R4-build-compile.txt`
- `docs/evidence/2604/evidence.json`
- `docs/evidence/2604/pre-ready-2604.json`

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R0 修前红样基线（25 处 / 17 文件外泄原文） | `scripts/verify-2604.sh` | base 提交静态计数 | merge-base 提交 vs 修后 HEAD | repo（静态） | `RED_BASE_SITES=25`、`RED_BASE_FILES=17`、`RED_BASE_ASCII_MESSAGES=25`、`RED_HEAD_SITES=0` | R0 |
| R1 bindmsg 扩面（56 条称呼 + 2 作用域 + 复数缩写/下标归一） | `backend/bindmsg/bindmsg.go`、`backend/bindmsg/bindmsg_2604_test.go` | `go test ./bindmsg` | 56 条称呼 + 5 条作用域 + `IDs`/`IDs[0]`/`user_ids` + 默认作用域不漂移 | backend（纯函数） | `BINDMSG_UNIT_EXIT=0`、`LABEL_NEW_2604=56`、`LABEL_ASCII_LEAK=0`、`LABEL_COVERAGE_MISSING=0`、`SCOPE_PERSON_NAME=姓名`、`SCOPE_ROLE_CODE=角色编码`、`SCOPE_DEFAULT_CODE=优惠码`、`CAMEL_IDS=ids`、`CAMEL_USER_IDS=user_ids`、`DIVE_INDEX_LABEL=乐器列表` | R1 |
| R2 25 端点绑定失败文案（50 例契约测试） | `backend/handlers/bind_error_2604_test.go` 及 17 个 handler 文件 | `POST/PUT` 25 端点（handler 直调） | 损坏请求体（控制字符）/ 单一点名（缺必填或类型不匹配），共 50 例 | backend（一次性 PG 15433） | `BIND2604_CASES_TOTAL=50`、`BIND2604_CASES_FAIL=0`、`BIND2604_ASCII_LEAK=0`、`BIND2604_RAW_PHRASE_LEAK=0`，且 HTTP 400 + code 与修前一致 | R2 |
| R3 25 处收口 / 同类不外溢（静态扫描） | `scripts/verify-2604.sh` | 静态扫描 `backend/handlers`（base vs head） | 修后工作树 | repo（静态） | `RESIDUAL_INVALID_REQUEST_RAW=0`、`RESIDUAL_INVALID_PARAMETERS_RAW=0`、`SITES_REPLACED_2604=25`、`HANDLER_FILES_2604=17`、`BINDMSG_DELTA_2604=25`、`OUT_OF_SCOPE_UNCHANGED=1` | R3 |
| R4 构建 / gofmt / 测试包编译 | `scripts/verify-2604.sh` | `cd backend && go build ./...` 等（经 heavy-run） | 修后工作树 | backend | `GO_BUILD_EXIT=0`、`GOFMT_DIFF=0`、`TEST_COMPILE_EXIT=0` | R4 |

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R0 | `git merge-base origin/develop HEAD` 指向的 base 提交 + 修后 HEAD | `bash scripts/verify-2604.sh r0` | `docs/evidence/2604/R0-red-baseline.txt` | `RED_BASE_SITES=25`、`RED_BASE_FILES=17`、`RED_BASE_ASCII_MESSAGES=25`、`RED_HEAD_SITES=0` | base 计数 ≠25/17、或修后仍有 `invalid request: ` 拼接 → FAIL |
| R1 | 56 条新增称呼字面量 + 5 条作用域条目 + `IDs`/`IDs[0]`/`UserIDs`/`TransitSiteID` 输入 | `go test ./bindmsg -count=1 -v` | `docs/evidence/2604/R1-bindmsg-unit.txt` | `BINDMSG_UNIT_EXIT=0`、`LABEL_NEW_2604=56`、`LABEL_ASCII_LEAK=0`、`LABEL_COVERAGE_MISSING=0`、`SCOPE_PERSON_NAME=姓名`、`SCOPE_ROLE_CODE=角色编码`、`SCOPE_DEFAULT_CODE=优惠码`、`SCOPE_DEFAULT_NAME=名称`、`CAMEL_IDS=ids`、`CAMEL_USER_IDS=user_ids`、`DIVE_INDEX_LABEL=乐器列表` | 任一条称呼含 ASCII、默认作用域被改成角色/人员语义、`IDs` 仍输出 `i_ds`、`dive` 下标未归一 → 任一即 FAIL |
| R2 | `testfixtures.SetupTestDB` + 表驱动 25 端点 × 2 用例（夹具见「25 处落点」表与用例 1 约定） | `go test ./handlers -run '^TestBindErrorMessages2604$' -count=1 -v` | `docs/evidence/2604/R2-handler-messages.txt` | `BIND2604_CASES_TOTAL=50`、`BIND2604_CASES_FAIL=0`、`BIND2604_ASCII_LEAK=0`、`BIND2604_RAW_PHRASE_LEAK=0`、`BIND2604_ROLE_CODE_MSG=参数不合法：角色编码不能为空`、`BIND2604_PERSON_NAME_MSG=参数不合法：姓名不能为空`、`BIND2604_IDS_MSG=参数不合法：乐器列表不能为空`、`BIND2604_SYNTAX_MSG=请求体格式不正确，请检查后重试` | 任一端点返回 `invalid request`/`invalid character`/`cannot unmarshal`/EOF、返回非 400、code 漂移（40000/40001/40002 互换）、消息含 ASCII 字母、用例数 <50 → 任一即 FAIL |
| R3 | 修后 `backend/handlers` 全目录 + base 提交对照 | `bash scripts/verify-2604.sh r3` | `docs/evidence/2604/R3-residual-scan.txt` | `RESIDUAL_INVALID_REQUEST_RAW=0`、`RESIDUAL_INVALID_PARAMETERS_RAW=0`、`SITES_REPLACED_2604=25`、`HANDLER_FILES_2604=17`、`BINDMSG_DELTA_2604=25`、`OUT_OF_SCOPE_UNCHANGED=1` | 残留 `invalid request: " + err.Error()`、替换点数 ≠25、文件数 ≠17、bindmsg 调用点增量 ≠25、越界改动 #2605 的 28 处 → 任一即 FAIL |
| R4 | 修后工作树 | `bash scripts/verify-2604.sh r4` | `docs/evidence/2604/R4-build-compile.txt` | `GO_BUILD_EXIT=0`、`GOFMT_DIFF=0`、`TEST_COMPILE_EXIT=0` | 构建失败 / gofmt 有差异 / 测试包编译失败 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R0 | `bash scripts/verify-2604.sh r0 > docs/evidence/2604/R0-red-baseline.txt` | `docs/evidence/2604/R0-red-baseline.txt` | exit 0 且含 `RED_BASE_SITES=25` `RED_BASE_FILES=17` `RED_BASE_ASCII_MESSAGES=25` `RED_HEAD_SITES=0` | PASS（4 项 token 全为期望值） | base 计数 ≠25/17、修后仍残留原文拼接 → FAIL | dev |
| R1 | `bash scripts/verify-2604.sh r1 > docs/evidence/2604/R1-bindmsg-unit.txt` | `docs/evidence/2604/R1-bindmsg-unit.txt` | exit 0 且含 `BINDMSG_UNIT_EXIT=0` `LABEL_NEW_2604=56` `LABEL_ASCII_LEAK=0` `LABEL_COVERAGE_MISSING=0` `SCOPE_PERSON_NAME=姓名` `SCOPE_ROLE_CODE=角色编码` `SCOPE_DEFAULT_CODE=优惠码` `CAMEL_IDS=ids` `CAMEL_USER_IDS=user_ids` `DIVE_INDEX_LABEL=乐器列表` | PASS（10 项 token 全为期望值） | 称呼含 ASCII、漏登记字段、默认作用域漂移、`IDs` 输出 `i_ds` → FAIL | dev |
| R2 | `bash scripts/verify-2604.sh r2 > docs/evidence/2604/R2-handler-messages.txt` | `docs/evidence/2604/R2-handler-messages.txt` | exit 0 且含 `GO_TEST_EXIT=0` `BIND2604_CASES_TOTAL=50` `BIND2604_CASES_FAIL=0` `BIND2604_ASCII_LEAK=0` `BIND2604_RAW_PHRASE_LEAK=0` `BIND2604_ROLE_CODE_MSG=参数不合法：角色编码不能为空` `BIND2604_PERSON_NAME_MSG=参数不合法：姓名不能为空` `BIND2604_IDS_MSG=参数不合法：乐器列表不能为空` `BIND2604_SYNTAX_MSG=请求体格式不正确，请检查后重试` | PASS（9 项 token 全为期望值） | 任一端点含 Go/JSON 原文或 ASCII 字母、HTTP/code 漂移、用例数 <50 → FAIL | dev |
| R3 | `bash scripts/verify-2604.sh r3 > docs/evidence/2604/R3-residual-scan.txt` | `docs/evidence/2604/R3-residual-scan.txt` | exit 0 且含 `RESIDUAL_INVALID_REQUEST_RAW=0` `RESIDUAL_INVALID_PARAMETERS_RAW=0` `SITES_REPLACED_2604=25` `HANDLER_FILES_2604=17` `BINDMSG_DELTA_2604=25` `OUT_OF_SCOPE_UNCHANGED=1` | PASS（6 项 token 全为期望值） | 残留原文拼接、替换点/文件数不符、越界改动 #2605 范围 → FAIL | dev |
| R4 | `bash scripts/verify-2604.sh r4 > docs/evidence/2604/R4-build-compile.txt` | `docs/evidence/2604/R4-build-compile.txt` | exit 0 且含 `GO_BUILD_EXIT=0` `GOFMT_DIFF=0` `TEST_COMPILE_EXIT=0` | PASS（3 项 token 全为期望值） | 构建失败 / gofmt 差异 / 测试包编译失败 → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2604.sh`（`r0`–`r4`）；每个文本 artifact 首行：`# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，随后 `GENERATED_AT_HEAD=<40 位 sha>`；证据必须在 fix commit **之后**生成（`FIX_REF` 可覆盖锚点，默认 `git log --grep="^fix(issue-2604)"`）。
- 重命令（`go test` / `go build` / `gofmt`）一律经 `~/.config/opencode/scripts/heavy-run.sh` 取锁；handler 测试用一次性 PostgreSQL（`127.0.0.1:15433`，库 `tuneloop_test`，`#2341` 口径），**不**使用 `jobmaster-postgres`。
- 逐条清单（全部位于 `docs/evidence/2604/`）：
  - `docs/evidence/2604/R0-red-baseline.txt`
  - `docs/evidence/2604/R1-bindmsg-unit.txt`
  - `docs/evidence/2604/R2-handler-messages.txt`
  - `docs/evidence/2604/R3-residual-scan.txt`
  - `docs/evidence/2604/R4-build-compile.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2604 --summary-file tmp/pre-ready-2604-summary.md --changed-files tmp/pre-ready-2604-changed.txt --evidence-file docs/evidence/2604/evidence.json --report docs/evidence/2604/pre-ready-2604.json`
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Input Truth & Tool Preflight

- **Source of truth**：
  - 25 处清单 = `grep -rn '"invalid request: " + err.Error()' backend/handlers/*.go`（修前 25，修后 0）；
  - code 基线 = 逐点实测（40000 ×4 / 40001 ×5 / 40002 ×16，见背景第 4 条），**不得**统一；
  - 文案模板与称呼 = `backend/bindmsg/bindmsg.go`（唯一 owner；#2603 落 88 条，本单 +56 条与 5 条作用域）；
  - 前端层序 = `frontend-pc/src/services/errorMessages.js` / `frontend-mobile/src/services/errorMessageResolution.mjs` 的 `resolveErrorCore`（L2 含 CJK 透传，先于 L3 code 家族）。
- **实施前差异核对（先跑，结果入 Work Summary）**：
  - `grep -rn '"invalid request: " + err.Error()' backend/handlers/*.go | wc -l` → 修前 **25**；修后 **0**；
  - `grep -rln '"invalid request: " + err.Error()' backend/handlers/*.go | wc -l` → **17**；
  - `grep -rho 'bindmsg.Message' backend/handlers/*.go | wc -l` → 修前 **28**（#2603 落地后）；修后 **53**（增量 25）；
  - `grep -rn '"invalid parameters: " + err.Error()' backend/handlers/*.go | wc -l` → **0**（继承 #2603，本单必须保持 0）；
  - `grep -n 'dive' backend/handlers/instrument.go | wc -l` → **1**（`IDs`，故本单做下标归一）；
  - #2605 范围 28 处（`invalid request body: `6 / `Invalid request data: `3 / `Invalid request: `4 / `Invalid input: `9 / `invalid * format: `6）→ base 与 head **计数相等**。
- **损坏输入拒绝规则**（测试必须覆盖，且都必须是中文）：
  - 控制字符 / 截断 / 多余逗号 → `请求体格式不正确，请检查后重试`；
  - 字段类型不符（数字填字符串、对象填字符串）→ `参数不合法：<称呼>类型不正确`；
  - 空 body（`EOF`）→ `参数不合法，请检查后重试`（本单不单独设例，行为由 #2603 模板保证）；
  - 未知字段（未登记称呼）→ 回显 JSON snake 名，**绝不**回显 Go 字段名；
  - 25 端点响应**不得**出现 ASCII 字母（R2 硬断言）。
- **工具盲区**：
  - handler 契约测试只覆盖 handler 层（不经 `main.go` 路由、不经 nginx、不经 IAM）；本单**不改路由**，路由挂载由既有 `backend/main.go` 保持不变；
  - 前端展示链路由 #2603 / #2480 的层序单测覆盖，本单不重复；真实浏览器/真机表现归部署侧复验（D10）；
  - `TestBindErrorMessages2604` 依赖一次性 PG（`tuneloop_test`）；不可达必须 `exit 2`/FAIL，禁止静默跳过；
  - 全量 `go test ./...` 不在合同行（P5 baseline 差分承担）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 唯一出口 = `backend/bindmsg.Message(err, scopes ...Scope)`；本单**不**新建 handler 层文件 | 本计划锁定 |
| D2 | 文案模板沿用 #2603 的 9 条逐字模板（`参数不合法：<称呼>…` / `请求体格式不正确，请检查后重试`），任何改动需重开分析 | 本计划锁定 |
| D3 | 新增 56 条称呼 + 5 条作用域条目逐字锁定；称呼不得含 ASCII 字母 | 本计划锁定 |
| D4 | **同名跨域冲突**用 `Scope`（person/role）解决，仅在 8 个端点显式传作用域；**禁止**在各 handler 内联 label pin（单一 owner 原则） | 本计划锁定 |
| D5 | 只替换 `message` 表达式，**不改** HTTP 状态 / code / 分支顺序；code 保留现状混杂（40000 ×4 / 40001 ×5 / 40002 ×16） | 本计划锁定 |
| D6 | 前端（PC / H5 / weapp）**零改动**：L2 含 CJK 透传；`ERROR_MESSAGE_MAP['invalid request']` 键保留（仍被英文短串 handler 使用） | 本计划锁定 |
| D7 | 不新增 `docs/spec/api/api.md` 条款：#2603 的 §1.3 绑定失败口径已覆盖本类（其计划「变更点 7」+ Core Update），本单同口径扩面，无契约变更（P7 为 N/A） | 本计划锁定 |
| D8 | #2605（其它字面量 28 处）**不在本单**；本单只断言其计数不变（`OUT_OF_SCOPE_UNCHANGED=1`） | #2605 |
| D9 | `dive` 下标归一为 #2603 D4 的受控例外：仅因 `instrument.go:1103` 有 `binding:"required,dive,required"` 而可达 | 本计划锁定 |
| D10 | PC/H5/weapp 真机复验（用户是否真看到点名字段）归部署侧，closing 时建 `[Ops-Followup of #2604]`（`waitForConfirm`，无 `status:*`） | 本计划锁定 |
| D11 | 重命令经 `heavy-run.sh`；handler 测试用一次性 PG（`127.0.0.1:15433`，库 `tuneloop_test`），不碰 dev 库 | 本计划锁定 |
| D12 | 依赖未满足即 `HOLD`：`bindmsg` 包必须来自 #2603（`status:done`/`ready`）；`/work` 以 `work_deps.sh` 判定，禁止在本单自建同名包 | 本计划锁定 |

### Out of Scope

- 其它绑定/解析失败字面量 **28 处 / 10 文件**（`invalid request body: `6、`Invalid request data: `3、`Invalid request: `4、`Invalid input: `9、`invalid images/specifications/pricing format: `6）→ **#2605**（已建，依赖 #2604）。
- 裸 `"message": err.Error()`（30 处，多为 DB/内部错误）与非绑定类英文 message（`instrument id is required` 等）→ 由 #2605 之后的收敛批次承担，本单不动。
- `docs/spec/api/api.md` 条款变更（#2603 已确立绑定失败口径）→ 本单不改（D7）。
- 任何前端改动（PC / H5 / weapp），含 `ERROR_MESSAGE_MAP` / `ERROR_CODE_MAP` 键调整。
- 400 族其它语义分支、状态码归一（40000/40001/40002 混杂现状保留）。
- 为 `dive` 引入新的校验行为（如 `items[].id` 的实际校验）——属行为变更，另立 Issue。
- 全量 `go test ./...` 作为合同行（由 pre-ready P5 baseline 差分承担）。

### Dependencies

- #2603

### Workload

- Size: L
- Basis: 1 个 leaf 包扩面 + 17 个 handler 文件（25 点）+ 2 个测试文件（50 例 + 单元）+ 1 个证据 harness + 5 条证据行（R0–R4），无 migration、无新端点、无前端改动；跨 Issue 依赖 1 个（#2603）
- Confidence: high

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状（symptom） | 请求体损坏/缺字段时响应 `400xx` + `invalid request: invalid character '\x17' in string`；PC/H5 用户只看到通用「请求参数有误」/「信息不完整」 | #2604 正文、#2603 预生产实测 |
| 2 | 触发条件（trigger） | 任一 25 处 `c.ShouldBindJSON` 失败：缺必填 / 类型不符 / JSON 语法损坏（控制字符、截断） | 25 处逐点核对 |
| 3 | 根因（root cause） | handler 把 `err.Error()`（`encoding/json` / validator / 绑定层措辞）直接拼进 `message`；前端 L2 只透传 CJK，英文技术串被判「无具体原因」→ 降级 generic 文案 | 25 处字面量、`errorMessageResolution.mjs` |
| 4 | 测试盲点（test blind spot） | 全仓无任何测试断言这 25 处的 message；「构建通过 + 状态码 400」被当成验证充分 | `grep -rn 'invalid request' --include=*_test.go backend` |
| 5 | 同类扫描（same-class paths） | 字面量不同的同族缺陷 28 处 / 10 文件 → #2605；#2603 的 `invalid parameters:` 28 处已独立收口 | R3 的 `OUT_OF_SCOPE_UNCHANGED=1`、`RESIDUAL_INVALID_PARAMETERS_RAW=0` |
| 6 | 修复层（fix layer） | 提示层单点收口：`bindmsg` 翻译 + 称呼表（含作用域）+ 25 处调用点替换；不碰业务逻辑/状态码 | 变更点 1–2 |
| 7 | 红绿实证（red-green） | 红：仅落 bindmsg 扩面 + 测试（无替换）→ 50 例全 FAIL（message 含 `invalid request`），并由 R0 持久化 base 计数 25/17；绿：替换后 R2 全绿 | R0、变更点 6 |
| 8 | 防复发（prevention） | ① 收口扫描 `RESIDUAL_INVALID_REQUEST_RAW=0`；② 每例「message 无 ASCII 字母」硬断言；③ 称呼表 ASCII 泄漏断言；④ 越界防护 `OUT_OF_SCOPE_UNCHANGED=1`；⑤ 依赖 #2603 唯一出口，禁止各自拼字符串 | R1/R2/R3 |
| 9 | 影响面（blast radius） | 仅失败分支的 `message` 文本；成功路径、状态码、鉴权、DB 写入不变；#2605 范围计数不变 | R2 的 code 断言、R3 |
| 10 | 未修边界（deferred） | #2605 的 28 处、裸 `err.Error()` 30 处、真机复验 → `[Ops-Followup of #2604]` | Out of Scope、D8、D10 |

*Model: deepseek/deepseek-flash*


