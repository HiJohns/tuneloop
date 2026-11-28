## Accepted Plan — #2608 [Bug] [Core Update Required] 绑定失败裸回传 `err.Error()` 的 14 处收口（复用 `bindmsg`，点名字段 + 同名异义字段作用域）

### 背景与现状核对（代码为据）

| # | 事实 | 位置 / 命令 |
|---|---|---|
| 1 | 本单目标 14 处「绑定失败即裸拼原文」与 Issue 表逐点一致：分布 10 文件、code 全 `40002` | `grep -A1 'if err := c.ShouldBindJSON' backend/handlers/*.go \| grep -c '"message": err.Error()'` → **14**（discount 2、gift_policies 2、instrument_promo_override 1、membership_benefits 1、membership_level 2、merchant_rebate 1、points_policy 2、user_management 1、warning 1、webhook_config 1） |
| 2 | 全仓 `"message": err.Error()` 共 **63** 处；其中 49 处为**非绑定分支**（29 处 `50000` DB 错误 + 2 处 `40002` 服务校验 + 18 处其它文件），本单不动 | `grep -rc '"message": err.Error()' backend/handlers/*.go` → 63；本单 10 文件内 = 45（`40002` 16 + `50000` 29） |
| 3 | 唯一出口 `backend/bindmsg` 已随 #2603 **合入 `origin/develop`**（不是「仅在分支上」），称呼表 85 条、9 条文案模板 | `git cat-file -e origin/develop:backend/bindmsg/bindmsg.go` → 存在；`sed -n '/^var bindFieldLabels/,/^}/p' … \| grep -cP '^\t"[a-z_0-9]+"'` → 85 |
| 4 | 14 处形态完全一致，绑定失败即 `return`；前置只有取参/`uuid.Parse`/`Atoi`/取 scope，无鉴权或 DB 写入 | 落点表「前置」列 |
| 5 | 14 处请求体**均无 `dive` 标签**，validator 不会产出 `items[0].id` 这类嵌套路径 → **不做**嵌套路径特化（否则是不可达代码） | `grep -n 'dive' backend/handlers/{discount,gift_policies,instrument_promo_override,membership_benefits,membership_level,merchant_rebate,points_policy,user_management,warning,webhook_config}.go` → 0 |
| 6 | 前端（PC/H5/weapp）**零改动**：`resolveErrorCore` L2「含 CJK 透传」先于 L1/L3，中文文案直达用户 | `frontend-pc/src/services/errorMessages.js`、`frontend-mobile/src/services/errorMessages.js` |
| 7 | 既有测试对这 14 处 message **0 断言**（8 个相关测试文件均无 `message` 断言） | `grep -n 'message\|Message' backend/handlers/{gift_policy_test,membership_benefits_test,points_grant_test,promo_override_1863_test,merchant_config_scope_2251_test,write_attribution_2260_test,amount_conversion_1987_test,webhook_legacy_fallback_2262_test}.go` → 0 |
| 8 | 称呼表按 **JSON 字段名全局唯一**：`items` 已被 #2603 注册为「乐器列表」（`inventory.BatchUpdateRent`），而 `membership_benefits.UpdateLevelBenefits` 的 `items` 语义是「权益项」→ **同名异义**，必须作用域化，否则用户会看到错误的实体名（违反「用户名词 == 输出实体」） | `backend/bindmsg/bindmsg.go` 的 `bindFieldLabels`；`backend/handlers/membership_benefits.go:44-47` |
| 9 | `camelToSnake` 对「缩写 + 复数」拆错：`IDs` → `i_ds`、`GuarantorIDs` → `guarantor_i_ds`。后者是 #2603 已收口端点 `wechatpay_prepay.PrepayOrder` 的字段（已登记 `guarantor_ids` → 担保人列表，但 Go 字段名还原不回来）→ 现在仍是**活的 ASCII 外溢点**；Issue §3 要求本族修正 | `backend/bindmsg/bindmsg.go` 的 `camelToSnake`；Issue 正文 §3 |
| 10 | 同类但不同字面量的 25 处 `"invalid request: " + err.Error()` 属 **#2604**，本单只断言其计数不变 | `grep -rn '"invalid request: " + err.Error()' backend/handlers/*.go \| wc -l` → 25 |

**结论**：缺陷在提示层——绑定失败时用户只能看到前端 `code_map` 通用兜底（「信息不完整，请补充必填项」），看不到**哪个字段**错了。修复 = 14 处 message 表达式收敛到 `bindmsg` + 补齐称呼（含作用域 + 缩写复数修正），并用「14 端点 × 2 用例」的 handler 契约测试把「不得出现 Go/JSON 原文、不得出现 ASCII 字段名、不得点错实体名」钉成机器可判定的负例。

### 变更点

#### 1. 基础层：称呼表 +31（`backend/bindmsg/bindmsg.go`，唯一 owner）

- 现有 85 条 → **116 条**；新增条目全部**不含 ASCII 字母**。
- 未登记字段的回退保持「回显 JSON snake 名」，绝不回显 Go 字段名（`ManagerPhone` 类）。
- 与 #2603 的 9 条文案模板**完全复用**，本单不新增模板。

#### 2. 基础层：同名异义字段作用域（`MessageScoped`）

```go
// MessageScoped renders a binding failure with endpoint-scoped field labels.
// Scoped labels only override fields whose JSON name carries a different
// meaning in the given endpoint (e.g. membership_benefits.items = 权益项,
// while inventory.items = 乐器列表). The global table stays the single owner.
func MessageScoped(scope string, err error) string {
    if err == nil {
        return FallbackMessage
    }
    // 与 Message 同一分支顺序，仅 FieldLabel 换成 FieldLabelScoped(scope, field)
}

func FieldLabelScoped(scope, field string) string {
    if label, ok := bindFieldLabelsScoped[scope][field]; ok && label != "" {
        return label
    }
    return FieldLabel(field)
}

// bindFieldLabelsScoped is the single owner of endpoint-scoped overrides.
var bindFieldLabelsScoped = map[string]map[string]string{
    "membership_benefits": {"items": "权益项"},
}
```

- `Message(err)` 语义**不变**（= `MessageScoped("", err)`），#2603 已交付的 28 处调用点与既有断言不受影响。
- 作用域名唯一使用点：`membership_benefits.UpdateLevelBenefits`（`bindmsg.MessageScoped("membership_benefits", err)`）。
- 除该点外**禁止**新增作用域 / 禁止把称呼复制到 handler（单 owner 见 D1/D6）。

#### 3. 基础层：`camelToSnake` 缩写 + 复数修正（Issue §3 强制）

- 规则：当结尾是小写 `s` 且其前一字符为大写时，按「词干 + `s`」还原（`IDs` → `id` + `s` → `ids`；`GuarantorIDs` → `guarantor_id` + `s` → `guarantor_ids`；`URLs` → `urls`）。
- 非再现性回归保护：`Status`/`Address`/`Notes` 等「词尾 s 前为小写」的字段**不走**该分支，行为不变。
- 登记 `ids` → 「编号列表」（供本族 #2606 的 `instrument.go` 批量编号点位复用）。

#### 4. 14 处替换（逐点机械，只换 message 表达式）

```go
// before
c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
// after（保留原 HTTP / code / 分支顺序）
c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": bindmsg.Message(err)})
```

- 新增 `import "tuneloop-backend/bindmsg"`（10 文件均无同名标识符冲突）。
- **禁止**改动任何 400 族判定分支、状态码、顺序；**禁止**顺手替换 49 处非绑定分支。
- 唯一例外：`membership_benefits.go` 用 `bindmsg.MessageScoped("membership_benefits", err)`（见变更点 2）。

#### 5. 14 处落点（file:line / 函数 / 前置 / 字段用例）

| # | 位置（`if` 行 / `c.JSON` 行） | 函数 | 绑定点前置 | 字段用例（请求体） | 期望 message |
|---|---|---|---|---|---|
| 1 | `backend/handlers/discount.go:24` / `:25` | `CreateDiscountPolicy` | 无 | `{}` | `参数不合法：名称不能为空` |
| 2 | `backend/handlers/discount.go:71` / `:72` | `CreateDiscountCode` | 无 | `{}` | `参数不合法：优惠码不能为空` |
| 3 | `backend/handlers/gift_policies.go:133` / `:134` | `UpdateGiftPolicy` | 无 | `{"pay_ratio":"abc"}` | `参数不合法：乐币抵扣比例类型不正确` |
| 4 | `backend/handlers/gift_policies.go:222` / `:223` | `UpdatePointSettings` | 无 | `{"pay_ratio_max":"abc"}` | `参数不合法：最高抵扣比例类型不正确` |
| 5 | `backend/handlers/instrument_promo_override.go:62` / `:63` | `UpdateInstrumentPromoOverride` | `:id` UUID 解析 | `{}` | `参数不合法：推广类型不能为空` |
| 6 | `backend/handlers/membership_benefits.go:46` / `:47` | `UpdateLevelBenefits` | `:id` `Atoi` | `{"items":"abc"}` | `参数不合法：权益项类型不正确`（作用域称呼） |
| 7 | `backend/handlers/membership_level.go:29` / `:30` | `CreateMembershipLevel` | 无 | `{}` | `参数不合法：编号不能为空` |
| 8 | `backend/handlers/membership_level.go:54` / `:55` | `UpdateMembershipLevel` | `:id` `Atoi` | `{"min_amount":"abc"}` | `参数不合法：最低消费金额类型不正确` |
| 9 | `backend/handlers/merchant_rebate.go:31` / `:32` | `UpdateMerchantRebateOptIn` | 无 | `{"opt_in":"abc"}` | `参数不合法：返点开关类型不正确` |
| 10 | `backend/handlers/points_policy.go:69` / `:70` | `CreatePointsPolicy` | `scopeTypeFromPath` 需 `/merchant/` 前缀 | `{"valid_days":"abc"}` | `参数不合法：有效天数类型不正确` |
| 11 | `backend/handlers/points_policy.go:113` / `:114` | `UpdatePointsPolicy` | `:id` UUID 解析 | `{"max_pay_ratio":"abc"}` | `参数不合法：最高抵扣比例类型不正确` |
| 12 | `backend/handlers/user_management.go:387` / `:388` | `(*UserManagementHandler).GrantPoints` | 无 | `{"amount":"abc"}` | `参数不合法：金额类型不正确` |
| 13 | `backend/handlers/warning.go:134` / `:135` | `UpdateWarningSettings` | 无 | `{"cooldown_minutes":"abc"}` | `参数不合法：冷却间隔类型不正确` |
| 14 | `backend/handlers/webhook_config.go:66` / `:67` | `UpdateMerchantWebhook` | `requireMerchantAdmin`（context 商户管理员） | `{"enabled":"abc"}` | `参数不合法：启用状态类型不正确` |

#### 6. 称呼表新增 31 条（逐字锁定，不得含 ASCII 字母）

| 域 | 新增条目 |
|---|---|
| 折扣（discount.go） | `rent_discount`→租金折扣、`deposit_discount`→押金折扣、`shipping_discount`→物流费折扣、`max_amount`→最高优惠金额、`valid_from`→生效时间、`valid_to`→失效时间、`policy_id`→折扣策略、`max_uses`→最大使用次数、`expires_at`→过期时间 |
| 礼品 / 积分（gift_policies.go、points_policy.go） | `level_id`→会员等级、`pay_ratio`→乐币抵扣比例、`referral_ratio`→推荐奖励比例、`referral_reg_points`→推荐注册赠点、`is_active`→启用状态、`pay_ratio_max`→最高抵扣比例、`point_batch_validity_months`→乐币有效期月数、`point_expiry_reminder_days`→到期提醒提前天数、`scope_id`→作用范围、`max_pay_ratio`→最高抵扣比例、`valid_days`→有效天数 |
| 推广 / 会员（instrument_promo_override.go、membership_level.go） | `override_type`→推广类型、`enabled`→启用状态、`content`→推广内容、`min_amount`→最低消费金额 |
| 返点 / 用户（merchant_rebate.go、user_management.go） | `opt_in`→返点开关 |
| 通知（warning.go、webhook_config.go） | `emails`→收件邮箱、`cooldown_minutes`→冷却间隔、`webhook_enabled`→企业微信通知开关、`webhook_url`→企业微信通知地址、`url`→通知地址 |
| 缩写复数（Issue §3） | `ids`→编号列表 |

作用域称呼（不进全局表）：`membership_benefits` + `items` → 权益项。

#### 7. 测试（红绿实证）

- `backend/bindmsg/bindmsg_test.go`（扩充）：
  - `TestBindMessageScoped2608`：`MessageScoped("membership_benefits", typeErr(items))` = `参数不合法：权益项类型不正确`；`Message(typeErr(items))` = `参数不合法：乐器列表类型不正确`（证明 #2603 的 inventory 口径**未被改写**）；`MessageScoped("unknown", …)` 回落全局表。
  - `TestCamelToSnakeAcronymPlural2608`：`FieldLabel("IDs")`=编号列表、`FieldLabel("GuarantorIDs")`=担保人列表、`FieldLabel("URLs")`=`urls`（未登记，回显 JSON 名）、回归 `FieldLabel("InstrumentSN")`=乐器序列号、`FieldLabel("IsActive")`=启用状态。
  - `TestBindFieldLabelsCoverage2608`：14 端点字段全集（含 31 条新增）全部登记 + 全部称呼 `[A-Za-z]` 计数 0。
- `backend/handlers/bind_error_2608_test.go`（新增）：`TestBindErrorMessages2608` = **14 端点 × 2 用例 = 28**（`{"probe":"\x17"}` → `请求体格式不正确，请检查后重试`；落点表「字段用例」→ 逐字期望值）。每例断言：
  1. HTTP 400 且 `code == 40002`（与修前一致）；
  2. `message` 的 `[A-Za-z]` 计数 0；
  3. `message` 不含 `invalid character` / `cannot unmarshal` / `Field validation` / `json:` / `EOF`；
  4. 字段用例 `message` 逐字相等（落点表期望值）。
  - 14 个绑定分支都在 DB 访问之前返回，故**不**依赖 `testfixtures.SetupTestDB`；`UpdateMerchantWebhook` 用例需在 context 注入商户管理员身份（`ContextKeyRole=ADMIN`、`ContextKeyTenantID == ContextKeyOrgID`）。
- **红绿实证**：先只提交测试 + 称呼表（不含 14 处替换）→ `TestBindErrorMessages2608` 28 例全 FAIL（message 含 `invalid character` / `Field validation`），Work Summary 贴红样；替换后全 PASS。

#### 8. 证据生成器

- 新增 `scripts/verify-2608.sh`（`r1`–`r5`，沿用 `scripts/verify-2603.sh` 结构：provenance 首行、`heavy-run.sh` 取锁、一次性 PG `127.0.0.1:15433`）。

#### 9. 文档

- `docs/spec/api/api.md` §1.3「请求体绑定失败（#2603）」段末补一句作用域口径（见 Core Update）。

### 既有断言影响（0 翻转）

| 检查 | 结果 | 依据 |
|---|---|---|
| Go 测试断言这 14 处绑定文案 | **0 处** | 背景第 7 条 |
| #2603 已交付断言（`TestBindErrorMessages2603` / `TestBindFieldLabelsCoverage2603`） | **0 翻转** | `Message(err)` 语义与称呼表既有 85 条不变；`items`→乐器列表 保持，作用域只在 `membership_benefits` 端点生效 |
| `camelToSnake` 既有行为断言 | **0 翻转** | 现有断言只覆盖 `ManagerPhone`/`unknown_field_xyz`/注册表条目；`IDs`/`GuarantorIDs` 此前无断言（缺陷态） |
| 前端断言 | **0 处** | 前端只断言层序与 code_map |
| 需要翻转的语义断言 | **无** | 本单只改 message 文本，不改 code / HTTP / 分支 |

### Files（变更文件清单）

- `backend/bindmsg/bindmsg.go`
- `backend/bindmsg/bindmsg_test.go`
- `backend/handlers/bind_error_2608_test.go`
- `backend/handlers/discount.go`
- `backend/handlers/gift_policies.go`
- `backend/handlers/instrument_promo_override.go`
- `backend/handlers/membership_benefits.go`
- `backend/handlers/membership_level.go`
- `backend/handlers/merchant_rebate.go`
- `backend/handlers/points_policy.go`
- `backend/handlers/user_management.go`
- `backend/handlers/warning.go`
- `backend/handlers/webhook_config.go`
- `scripts/verify-2608.sh`
- `docs/spec/api/api.md`
- `docs/evidence/2608/plan.md`
- `docs/evidence/2608/R1-bindmsg-unit.txt`
- `docs/evidence/2608/R2-handler-messages.txt`
- `docs/evidence/2608/R3-residual-scan.txt`
- `docs/evidence/2608/R4-build-compile.txt`
- `docs/evidence/2608/R5-api-doc.txt`
- `docs/evidence/2608/evidence.json`
- `docs/evidence/2608/pre-ready-2608.json`

### Core Update

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | §1.3「请求体绑定失败（#2603）」已写明唯一出口 `bindmsg.Message(err)`、禁止 `err.Error()` 原文、称呼表唯一 owner；未写「同一 JSON 字段名在不同端点含义不同」的收敛规则 | 在该段末追加：若同一 JSON 字段名在不同端点含义不同（如 `items` 在库存批量改租金为「乐器列表」、在会员权益为「权益项」），端点必须调用 `bindmsg.MessageScoped(<端点作用域>, err)`，作用域称呼表的唯一 owner 仍是 `backend/bindmsg/bindmsg.go`；禁止在 handler 内自定义称呼或复制文案 | 本单新增 `MessageScoped` 出口，若不写清会出现「端点各自拼称呼」的分叉（与 §1.3 单一 owner 口径冲突） |

> 与核心文档**无冲突**：§1.3 现有 #2603 口径原样保留，本单只是把出口从「Message」扩为「Message / MessageScoped」并写明同名异义字段的收敛规则。

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 称呼表 116 条 + 作用域称呼 + 缩写复数还原 + 9 条模板 | `backend/bindmsg/bindmsg.go`、`backend/bindmsg/bindmsg_test.go` | `cd backend && go test ./bindmsg` | 14 端点字段全集 / `IDs`·`GuarantorIDs` / `items` 双语义 / 9 类错误输入 | backend（纯函数） | `LABEL_ASCII_LEAK=0`、`LABEL_COVERAGE_MISSING=0`；作用域 `权益项` 与全局 `乐器列表` 并存；`IDs`→编号列表、`GuarantorIDs`→担保人列表；#2603 模板逐字不变 | R1 |
| R2 14 端点绑定失败文案（契约测试） | `backend/handlers/bind_error_2608_test.go`、`backend/handlers/discount.go`、`backend/handlers/gift_policies.go`、`backend/handlers/instrument_promo_override.go`、`backend/handlers/membership_benefits.go`、`backend/handlers/membership_level.go`、`backend/handlers/merchant_rebate.go`、`backend/handlers/points_policy.go`、`backend/handlers/user_management.go`、`backend/handlers/warning.go`、`backend/handlers/webhook_config.go` | `POST/PUT` 14 端点（handler 直调） | 损坏请求体（控制字符）/ 缺必填 / 类型不符，共 28 例 | backend（一次性 PG 15433） | `BIND2608_CASES_TOTAL=28`、`CASES_FAIL=0`、`ASCII_LEAK=0`、`RAW_PHRASE_LEAK=0`，HTTP 400 + `40002` 不变 | R2 |
| R3 收口扫描（14 处全替换、非绑定分支与同类不外溢） | `scripts/verify-2608.sh` | 静态扫描 `backend/handlers` | 修后工作树 | repo（静态） | `BINDING_BRANCH_RAW=0`、`BARE_ERR_TOTAL=49`（修前 63）、`BINDMSG_CALL_SITES=41`、`SCOPED_CALL_SITES=1`（合计 42 个调用点）、`BINDMSG_HANDLER_FILES=23`、`OUT_OF_SCOPE_INVALID_REQUEST=25` | R3 |
| R4 构建 / gofmt / 测试编译 | `scripts/verify-2608.sh` | `cd backend && go build ./...`、`gofmt -l`、`go test ./... -run '^$'` | 修后工作树 | backend | `GO_BUILD_EXIT=0`、`GOFMT_DIFF=0`、`TEST_COMPILE_EXIT=0` | R4 |
| R5 API 口径文档同步 | `docs/spec/api/api.md` | 文档结构断言（§1.3） | 修后文档 | 文档 | `API_BIND_RULE=1`、`API_SCOPED_NOTE=1`、`API_BINDMSG_REF=1`、`API_FORBID_RAW=1` | R5 |

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 9 类错误输入（validator required/email/uuid/数值/oneof/其它、`UnmarshalTypeError`、含 `\x17` 的 `SyntaxError`、`EOF`/nil）+ 14 端点字段全集（含 31 条新增）+ `IDs`/`GuarantorIDs`/`URLs` 缩写复数样本 | `TestBindErrorMessage2603` / `TestBindFieldLabelsCoverage2603` / `TestBindMessageScoped2608` / `TestCamelToSnakeAcronymPlural2608` / `TestBindFieldLabelsCoverage2608` | `docs/evidence/2608/R1-bindmsg-unit.txt` | `BINDMSG_UNIT_EXIT=0`、`BMSG_REQUIRED=参数不合法：名称不能为空`、`BIND2608_SCOPED_MSG=参数不合法：权益项类型不正确`、`BIND2608_UNSCOPED_MSG=参数不合法：乐器列表类型不正确`、`BIND2608_SCOPED_UNKNOWN=参数不合法：乐器列表类型不正确`、`BIND2608_IDS_LABEL=编号列表`、`BIND2608_GUARANTOR_IDS_LABEL=担保人列表`、`BIND2608_OPT_IN_LABEL=返点开关`、`BIND2608_LEVEL_ID_LABEL=会员等级`、`LABEL_ASCII_LEAK=0`、`LABEL_COVERAGE_MISSING=0` | 模板含 ASCII/`invalid`/`Field validation`；称呼表漏字段；登记称呼含 ASCII；作用域覆盖污染全局 `items`；`GuarantorIDs` 还原为 `guarantor_i_ds` → 任一即 FAIL |
| R2 | 无 DB 依赖（14 分支均在 DB 访问前返回）；商户管理员 context（`ADMIN` + `tid==oid`）；14 端点 × 2 请求体 | `TestBindErrorMessages2608`（`httptest` + `gin` 直调 14 handler） | `docs/evidence/2608/R2-handler-messages.txt` | `GO_TEST_EXIT=0`、`BIND2608_CASES_TOTAL=28`、`BIND2608_CASES_FAIL=0`、`BIND2608_ASCII_LEAK=0`、`BIND2608_RAW_PHRASE_LEAK=0`、`BIND2608_DISCOUNT_MSG=参数不合法：名称不能为空`、`BIND2608_DISCOUNTCODE_MSG=参数不合法：优惠码不能为空`、`BIND2608_POINTS_MSG=参数不合法：有效天数类型不正确`、`BIND2608_BENEFITS_MSG=参数不合法：权益项类型不正确`、`BIND2608_REBATE_MSG=参数不合法：返点开关类型不正确`、`BIND2608_SYNTAX_MSG=请求体格式不正确，请检查后重试` | 任一端点返回 `invalid character`/`cannot unmarshal`/`Field validation`/EOF；返回非 400；code 变化（40002→其它）；message 含 ASCII 字母；`items` 在权益端点显示为「乐器列表」→ 任一即 FAIL |
| R3 | 修后 `backend/handlers` 全目录（10 目标文件 + 其余） | `bash scripts/verify-2608.sh r3` | `docs/evidence/2608/R3-residual-scan.txt` | `BINDING_BRANCH_RAW=0`、`BARE_ERR_TOTAL=49`、`BARE_ERR_TOUCHED_FILES=31`、`BARE_40002_SERVICE_RAW=2`、`BINDMSG_CALL_SITES=41`、`SCOPED_CALL_SITES=1`（合计 42 个调用点）、`BINDMSG_HANDLER_FILES=23`、`OUT_OF_SCOPE_INVALID_REQUEST=25`、`OUT_OF_SCOPE_INVALID_PARAMETERS=0` | 残留任一绑定分支原文；`BARE_ERR_TOTAL≠49`（顺手动非绑定分支）；调用点数≠41 或作用域调用≠1；`invalid request:` 计数≠25 → 任一即 FAIL |
| R4 | 修后工作树 | `bash scripts/verify-2608.sh r4` | `docs/evidence/2608/R4-build-compile.txt` | `GO_BUILD_EXIT=0`、`GOFMT_DIFF=0`、`TEST_COMPILE_EXIT=0` | 构建失败 / gofmt 有差异 / 测试包编译失败 → FAIL |
| R5 | 修后 `docs/spec/api/api.md` §1.3 | `bash scripts/verify-2608.sh r5` | `docs/evidence/2608/R5-api-doc.txt` | `API_BIND_RULE=1`、`API_SCOPED_NOTE=1`、`API_BINDMSG_REF=1`、`API_FORBID_RAW=1` | §1.3 未写同名异义字段作用域口径 / 未禁 `err.Error()` 原文 / 未引用 `backend/bindmsg` → 任一即 FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2608.sh r1 > docs/evidence/2608/R1-bindmsg-unit.txt` | `docs/evidence/2608/R1-bindmsg-unit.txt` | exit 0 且含 `BINDMSG_UNIT_EXIT=0` `BMSG_REQUIRED=参数不合法：名称不能为空` `BIND2608_SCOPED_MSG=参数不合法：权益项类型不正确` `BIND2608_UNSCOPED_MSG=参数不合法：乐器列表类型不正确` `BIND2608_SCOPED_UNKNOWN=参数不合法：乐器列表类型不正确` `BIND2608_IDS_LABEL=编号列表` `BIND2608_GUARANTOR_IDS_LABEL=担保人列表` `BIND2608_OPT_IN_LABEL=返点开关` `BIND2608_LEVEL_ID_LABEL=会员等级` `LABEL_ASCII_LEAK=0` `LABEL_COVERAGE_MISSING=0` | PASS（11 项 token 全为期望值） | 模板含 ASCII/`invalid`/`Field validation`、称呼表漏字段、登记称呼含 ASCII、作用域污染全局 `items`、`GuarantorIDs` 还原为 `guarantor_i_ds` → FAIL | dev |
| R2 | `bash scripts/verify-2608.sh r2 > docs/evidence/2608/R2-handler-messages.txt` | `docs/evidence/2608/R2-handler-messages.txt` | exit 0 且含 `GO_TEST_EXIT=0` `BIND2608_CASES_TOTAL=28` `BIND2608_CASES_FAIL=0` `BIND2608_ASCII_LEAK=0` `BIND2608_RAW_PHRASE_LEAK=0` `BIND2608_DISCOUNT_MSG=参数不合法：名称不能为空` `BIND2608_DISCOUNTCODE_MSG=参数不合法：优惠码不能为空` `BIND2608_POINTS_MSG=参数不合法：有效天数类型不正确` `BIND2608_BENEFITS_MSG=参数不合法：权益项类型不正确` `BIND2608_REBATE_MSG=参数不合法：返点开关类型不正确` `BIND2608_SYNTAX_MSG=请求体格式不正确，请检查后重试` | PASS（11 项 token 全为期望值） | 任一响应含 Go/JSON 原文或 ASCII 字母、HTTP/code 变化、用例数<28、权益端点显示「乐器列表」→ FAIL | dev |
| R3 | `bash scripts/verify-2608.sh r3 > docs/evidence/2608/R3-residual-scan.txt` | `docs/evidence/2608/R3-residual-scan.txt` | exit 0 且含 `BINDING_BRANCH_RAW=0` `BARE_ERR_TOTAL=49` `BARE_ERR_TOUCHED_FILES=31` `BARE_40002_SERVICE_RAW=2` `BINDMSG_CALL_SITES=41` `SCOPED_CALL_SITES=1` `BINDMSG_HANDLER_FILES=23` `OUT_OF_SCOPE_INVALID_REQUEST=25` `OUT_OF_SCOPE_INVALID_PARAMETERS=0` | PASS（9 项 token 全为期望值） | 残留绑定分支原文、非绑定分支计数漂移、调用点数≠41、越界改 `invalid request:` 25 处 → FAIL | dev |
| R4 | `bash scripts/verify-2608.sh r4 > docs/evidence/2608/R4-build-compile.txt` | `docs/evidence/2608/R4-build-compile.txt` | exit 0 且含 `GO_BUILD_EXIT=0` `GOFMT_DIFF=0` `TEST_COMPILE_EXIT=0` | PASS（3 项 token 全为期望值） | 构建失败 / gofmt 差异 / 测试包编译失败 → FAIL | dev |
| R5 | `bash scripts/verify-2608.sh r5 > docs/evidence/2608/R5-api-doc.txt` | `docs/evidence/2608/R5-api-doc.txt` | exit 0 且含 `API_BIND_RULE=1` `API_SCOPED_NOTE=1` `API_BINDMSG_REF=1` `API_FORBID_RAW=1` | PASS（4 项 token 全为期望值） | 文档未写作用域口径 / 未禁原文 / 未引用唯一出口 → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2608.sh`（`r1`–`r5`）；每个文本 artifact 首行：`# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，随后 `GENERATED_AT_HEAD=<40 位 sha>`；证据必须在 fix commit **之后**生成（`FIX_REF` 可覆盖锚点，默认 `git log --grep="^fix(issue-2608)"`）。
- 重命令（`go test` / `go build` / `gofmt`）一律经 `~/.config/opencode/scripts/heavy-run.sh` 取锁；handler 测试用一次性 PostgreSQL（`127.0.0.1:15433`，`#2341` 口径，模板库 `tuneloop_test_template` 克隆为节点库），**不**使用 `jobmaster-postgres`。
- 逐条清单（全部位于 `docs/evidence/2608/`）：
  - `docs/evidence/2608/R1-bindmsg-unit.txt`
  - `docs/evidence/2608/R2-handler-messages.txt`
  - `docs/evidence/2608/R3-residual-scan.txt`
  - `docs/evidence/2608/R4-build-compile.txt`
  - `docs/evidence/2608/R5-api-doc.txt`
- `docs/evidence/2608/plan.md` 必须与本 Accepted Plan 评论逐字一致（`scripts/plan-source.sh --issue 2608 --fresh` 校验 sha256）。
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2608 --summary-file tmp/pre-ready-2608-summary.md --changed-files tmp/pre-ready-2608-changed.txt --evidence-file docs/evidence/2608/evidence.json --report docs/evidence/2608/pre-ready-2608.json`
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Input Truth & Tool Preflight

- **Source of truth**：
  - 14 处清单 = `grep -A1 'if err := c.ShouldBindJSON' backend/handlers/*.go | grep -c '"message": err.Error()'`（修前 **14**，修后 **0**）；
  - 称呼表 / 文案模板 / 作用域覆盖表 = `backend/bindmsg/bindmsg.go`（唯一 owner）；
  - code 基线 = 14 处**全部** `40002` + HTTP 400（逐点核对，**不得**顺手统一或改成 40001）；
  - 前端层序 = `frontend-pc/src/services/errorMessages.js` 的 `resolveErrorCore`（L2 含 CJK 透传）。
- **实施前差异核对（先跑，结果入 Work Summary）**：
  - `grep -A1 'if err := c.ShouldBindJSON' backend/handlers/*.go | grep -c '"message": err.Error()'` → 修前 **14**；修后 **0**；
  - `grep -rc '"message": err.Error()' backend/handlers/*.go | awk -F: '{s+=$2} END {print s}'` → 修前 **63**；修后 **49**（本单 10 文件 45→31）；
  - `grep -rho 'bindmsg.Message(err)' backend/handlers/*.go | wc -l` → 修前 **28**；修后 **41**（14 处中 13 处用 `Message`）；`grep -rho 'bindmsg.MessageScoped(' … | wc -l` → 修后 **1**（`membership_benefits`），合计 42 个调用点；
  - `grep -rn '"invalid request: " + err.Error()' backend/handlers/*.go | wc -l` → 修前/修后均为 **25**（越界即 FAIL）。
- **损坏输入拒绝规则**（测试必须覆盖，且都必须是中文）：
  - 控制字符 / 截断 / 多余逗号 → `请求体格式不正确，请检查后重试`；
  - 字段类型不符（数字填字符串/布尔填字符串）→ `参数不合法：<称呼>类型不正确`；
  - 缺必填 → `参数不合法：<称呼>不能为空`；
  - 空 body（`EOF`）→ `参数不合法，请检查后重试`；
  - 未登记称呼**不得**出现在 14 端点响应（R2 以「无 ASCII 字母」硬断言）。
- **工具盲区（显式登记）**：
  - R3 的 `-A1` 静态探针假设「绑定分支的 `c.JSON` 紧邻 `if` 行」；多行写法会漏检 → 用 `BARE_ERR_TOTAL` 精确增量断言（63→49）兜底，二者不一致即 FAIL；
  - handler 契约测试只覆盖 handler 层（不经 nginx、不经 IAM 中间件）；真实浏览器/真机表现归部署侧复验（`[Ops-Followup of #2608]`）；
  - R2 不依赖 DB，但 `go test ./handlers` 由 `TestMain` 克隆模板库 → PG `127.0.0.1:15433` 不可达时必须显式失败，**禁止**静默跳过；
  - 全量 `go test ./...` 不在合同行（P5 baseline 差分承担）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 唯一出口仍是 `backend/bindmsg`（leaf 包）；称呼 / 模板 / 作用域覆盖的**单一 owner** 是该文件，handler 内禁止自定义称呼或复制文案 | 本计划锁定 |
| D2 | 只替换 `message` 表达式，**不改** HTTP 状态 / code / 分支顺序；14 处全部保持 `40002` + HTTP 400 | 本计划锁定 |
| D3 | 文案模板沿用 #2603 的 9 条，本单不新增模板；不得为此改模板措辞 | 本计划锁定 |
| D4 | 称呼表 85 → 116（+31，逐字见变更点 6），所有称呼不含 ASCII 字母 | 本计划锁定 |
| D5 | `camelToSnake` 仅对「结尾小写 s + 前一字大写」做词干还原（`IDs`/`GuarantorIDs`/`URLs`），其余行为不变；并登记 `ids` | 本计划锁定 |
| D6 | 同名异义字段用 `MessageScoped(scope, err)` 收敛：`membership_benefits` 的 `items` → 权益项；作用域表唯一 owner = `bindmsg`；本单只开 1 个作用域（Rule of Three：不为单点造框架，只在真实冲突处引入最小覆盖层） | 本计划锁定 |
| D7 | 前端（PC/H5/weapp）**零改动**（L2 CJK 透传已实现） | 本计划锁定 |
| D8 | 49 处非绑定分支（29 处 `50000` DB 错误 + 2 处 `40002` 服务校验 + 18 处其它文件）不在本单：`50000` 属内部错误码面，`40002` 两处文案已是中文；R3 以精确计数断言未被顺手改动 | 本计划锁定 |
| D9 | 批次顺序：#2604 与本单是**兄弟单**（同一 `bindmsg` 称呼表 + `user_management.go` 的合并顺序约束）；`bindmsg` 出口已随 #2603 合入 `origin/develop`，代码前置无阻塞 | 本计划锁定 |
| D10 | 重命令经 `heavy-run.sh`；handler 测试用一次性 PG（`127.0.0.1:15433`），不碰 dev 库 | 本计划锁定 |
| D11 | 证据必须在 fix commit **之后**生成；`plan.md` 与 Accepted Plan 评论逐字一致 | 本计划锁定 |
| D12 | 全量 `go test ./...` 不作合同行（P5 baseline 承担）；本单合同只跑定向包 | 本计划锁定 |
| D13 | PC 表单内常驻提示的真机复验归部署侧，closing 时建 `[Ops-Followup of #2608]`（`waitForConfirm`，无 `status:*`） | 本计划锁定 |

### Out of Scope

- 49 处非绑定分支的 `err.Error()`：`50000` DB 错误（29 处，本单 10 文件内）+ `40002` 服务校验（`warning.go`/`webhook_config.go` 2 处，文案已是中文）+ 18 处其它文件 → 保持原样；如需统一另立 Issue。
- `"invalid request: " + err.Error()` 25 处 / 17 文件 → **#2604**（本单只断言计数不变）。
- 其它前缀族（`invalid request body` / `Invalid request data` / `Invalid request` / `Invalid input`）→ **#2605**。
- `instrument.go` 的裸 `err.Error()` 与 6 处不可达 `json.Marshal` 分支 → **#2606**。
- 400 族其它语义分支（非绑定失败）与任何前端改动（PC / H5 / weapp）。
- 嵌套 `dive` 校验引入（会让 `items[].id` 真正被校验）→ 属行为变更，另立 Issue。
- 全量 `go test ./...` 作为合同行（由 pre-ready P5 baseline 差分承担）。

### Dependencies

- #2604

### Workload

- Size: M
- Basis: 15 个实现/测试/文档文件（1 leaf 包 + 10 handler + 1 新测试 + 1 证据 harness + 1 核心文档）+ 8 条持久证据文件；14 处替换 + 28 条 handler 用例 + 31 条称呼；无 migration、无新端点、无前端改动、无跨仓依赖
- Confidence: high

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状（symptom） | 折扣 / 礼品积分 / 会员等级 / 返点 / 预警 / Webhook 等 PC 页面在请求体异常时只显示通用「信息不完整，请补充必填项」，用户不知道哪个字段错了 | Issue 正文、`ErrorCodeMap[40002]` |
| 2 | 触发条件（trigger） | 任一请求体绑定失败：缺必填 / 类型不符 / JSON 语法损坏（控制字符、截断） | 14 处 `ShouldBindJSON` 分支 |
| 3 | 根因（root cause） | handler 把 `err.Error()`（Go `encoding/json` / validator 措辞）直接放进 `message`；前端 L2 只透传含 CJK 的文案，英文技术串被判「无具体原因」→ 降级通用文案 | 14 处字面量、`errorMessages.js` L2 |
| 4 | 测试盲点（test blind spot） | 全仓无任何测试断言这 14 处 message；「构建通过 + 400」被当作验证充分 | 背景第 7 条（8 个相关测试文件 0 断言） |
| 5 | 同类扫描（same-class paths） | 同族仍有 25 处 `invalid request: `（#2604）、28 处其它前缀（#2605）、`instrument.go` 8 处（#2606）、49 处非绑定分支（本单 Out of Scope） | 4 条 `grep` 计数 |
| 6 | 修复层（fix layer） | 提示层单点收口：`bindmsg.Message`（翻译）+ 称呼表（点名）+ 1 个作用域覆盖（同名异义）+ 14 处调用点替换；不碰业务逻辑 / 状态码 | 变更点 1–4 |
| 7 | 共享契约边界（shared contract） | 称呼 / 模板 / 作用域表唯一 owner = `bindmsg`；`items` 全局语义（乐器列表）不得被单端点覆盖污染，故用 `MessageScoped` 而非改全局表 | 变更点 2、D1/D6 |
| 8 | 红绿实证（red-green） | 红：仅落称呼表 + 测试（无替换）→ 28 例全 FAIL（message 含 `invalid character` / `Field validation`）；绿：替换后 28/28 PASS | 变更点 7、R2 |
| 9 | 防复发（prevention） | ① 绑定分支静态收口 `BINDING_BRANCH_RAW=0`；② 每例「message 无 ASCII」硬断言；③ 称呼表 ASCII 泄漏断言；④ 作用域不污染全局的对照断言；⑤ 文档写明作用域出口 | R1/R2/R3/R5 |
| 10 | 影响面（blast radius） | 仅失败分支的 `message` 文本；成功路径、状态码、鉴权、DB 写入不变；非绑定分支 63→49 精确增量、同类 25 处计数不变 | R2 的 code 断言、R3 |
| 11 | 未修边界（deferred） | #2604/#2605/#2606 同族批次；49 处非绑定分支；PC 表单提示真机复验 → `[Ops-Followup of #2608]` | Out of Scope、D8、D9、D13 |

*Model: deepseek/deepseek-flash*

