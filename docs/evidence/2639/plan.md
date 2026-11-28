## Accepted Plan — #2639 [Bug] [Core Update Required] 400 族硬编码英文 message 残差收口（33 处 / 8 文件，纯中文业务文案，零 bindmsg 改动）

### 背景与现状核对（代码为据，基线 `origin/develop` @ `dd7cc16c4`）

| # | 事实 | 位置 / 命令 |
|---|---|---|
| 1 | **Issue 的「33 处」是单行 grep 口径，漏 1 处多行 `gin.H`**：`gift_policies.go:155-158` 的 `pay_ratio` 报错把 `"code"` 与 `"message"` 拆成两行，单行正则 `"code": *4[0-9]{4}.*"message": *"[A-Za-z]` **匹配不到** | 基线用多行感知扫描（见 §Input Truth `count_ascii_400xx`）→ `400xx + ASCII 起始 message` 实数 **34 处**；单行 grep → **33 处** |
| 2 | 34 处的 code 分布：`40001` × 1（`warning.go:69`）、`40002` × 32、`40003` × 1（`warning.go:160`） | `bash scripts/verify-2639.sh r1` 的 `BASELINE_400XX_HITS=34` |
| 3 | Issue 表第 26 行 `user_management.go:142`（`"invalid request: " + err.Error()`）**不是**「硬编码英文常量」，而是 `invalid request: ` 前缀族，归 **#2604**（已完成）；Issue 正文亦标注「#2604 已收口（本单只需断言计数不变）」→ 本单**不改这一行**，并以逐字断言钉住它 | `grep -n '"invalid request: " + err.Error()' backend/handlers/user_management.go` → 1 处分行 142 |
| 4 | 因此本单实现集 = **33 处**（34 − `user_management.go:142`），与 Issue 的 33 **数量相同、集合不同**（本单含 `gift_policies.go:158`，不含 `user_management.go:142`）；须以 §「落点表」为准逐点核对 | 落点表 33 行 |
| 5 | 10 文件里 `400xx` 之外另有 **16 处** ASCII 常量（`403xx` × 5、`404xx` × 11）；前端 `ERROR_CODE_MAP` 对 `40300`/`40400` 的兜底（「暂无权限执行此操作」/「未找到相关数据」）语义正确，而 `40002` 的兜底「信息不完整，请补充必填项」会**吞掉可纠正的业务原因** → 本单只收 `400xx`，16 处显式排除 | `grep -cE '"code": *40[34][0-9]{2}.*"message": *"[A-Za-z]'` → **16**；`frontend-pc/src/services/errorMessages.js` L14–L29 |
| 6 | 前端（PC / H5 / weapp）**零改动**：`resolveErrorCore` L2 对含 CJK 的 `message` 原样透传，中文业务文案直接到达用户 | `frontend-pc/src/services/errorMessages.js:111-135`、`frontend-mobile/src/services/errorMessageResolution.mjs` |
| 7 | **不得改 `backend/bindmsg`**：兄弟单 #2604 / #2605 / #2608（均 `status:done`、尚未合入 `origin/develop`）各自扩展过该包的称呼表/作用域，本单再加第四套会出现**同名 map key 重复（Go 编译错误）与作用域类型冲突**；`origin/develop` 现有称呼表已覆盖本单唯二需要点名的字段：`code`→优惠码、`status`→状态 | `git diff develop origin/issue-2604 -- backend/bindmsg/bindmsg.go`、`…issue-2605…`、`…issue-2608…`；`backend/bindmsg/bindmsg.go:115-218` |
| 8 | `merchant_rebate.go` / `webhook_config.go` 属 Issue 的 10 文件清单，但其 `400xx + ASCII` 计数为 **0**（只有 403/404）→ 本单不改这两文件 | 落点表为空；`grep -cE '"code": *400[0-9]{2}.*"message": *"[A-Za-z]'` → 0 / 0 |

**结论**：缺陷在提示层——`400xx` 分支把英文常量（或含 JSON 字段名的文案）放进 `message`，前端判「无具体原因」后落到 `ERROR_CODE_MAP[40002]` 的「信息不完整，请补充必填项」，用户看不到「优惠码已过期 / 推荐奖励比例超出范围 / 推广类型不正确」这类**可纠正**的原因。修复 = 33 处 `message` 换成纯中文业务文案（不点名 JSON 字段、不含 ASCII 字母），绑定失败分支（2 处）复用 `bindmsg.Message(err)`；HTTP/code/分支顺序一律不动。

### 落点表（33 处 · 基线行号 · 目标文案逐字）

| # | 位置（行） | 函数 | 触发 | HTTP/code | 现状 message | 目标 message |
|---|---|---|---|---|---|---|
| 1 | `backend/handlers/discount.go:142` | `ApplyDiscountCode` | 绑定失败（`binding:"required"`） | 400 / 40002 | `code is required` | `bindmsg.Message(err)` → 「参数不合法：优惠码不能为空」 |
| 2 | `discount.go:154` | `ApplyDiscountCode` | 码已停用 | 400 / 40002 | `discount code is inactive` | 「优惠码已停用」 |
| 3 | `discount.go:158` | `ApplyDiscountCode` | 使用次数用尽 | 400 / 40002 | `discount code usage limit reached` | 「优惠码使用次数已达上限」 |
| 4 | `discount.go:162` | `ApplyDiscountCode` | 码已过期 | 400 / 40002 | `discount code has expired` | 「优惠码已过期」 |
| 5 | `discount.go:168` | `ApplyDiscountCode` | 策略停用或不存在 | 400 / 40002 | `discount policy is inactive or not found` | 「折扣策略已停用或不存在」 |
| 6 | `discount.go:172` | `ApplyDiscountCode` | 策略尚未生效 | 400 / 40002 | `discount policy not yet valid` | 「折扣策略尚未生效」 |
| 7 | `discount.go:176` | `ApplyDiscountCode` | 策略已过期 | 400 / 40002 | `discount policy has expired` | 「折扣策略已过期」 |
| 8 | `gift_policies.go:139` | `UpdateGiftPolicy` | `level_id < 0` | 400 / 40002 | `level_id 不能为负数` | 「会员等级不能为负数」 |
| 9 | `gift_policies.go:155-158` | `UpdateGiftPolicy` | `pay_ratio` 越界（**多行 gin.H**） | 400 / 40002 | `pay_ratio 必须在 0~<max> 之间（…）` | 「乐币抵扣比例必须在 0~<max> 之间（赠点最多抵扣应付金额的 <max*100>%）」（保留原动态数值表达式） |
| 10 | `gift_policies.go:163` | `UpdateGiftPolicy` | `referral_ratio` 越界 | 400 / 40002 | `referral_ratio 必须在 0~1.0 之间` | 「推荐奖励比例必须在 0~1 之间」 |
| 11 | `gift_policies.go:167` | `UpdateGiftPolicy` | `referral_reg_points` 越界 | 400 / 40002 | `referral_reg_points 必须在 0~100000 之间` | 「推荐注册赠点必须在 0~100000 之间」 |
| 12 | `gift_policies.go:227` | `UpdatePointSettings` | `pay_ratio_max` 越界 | 400 / 40002 | `pay_ratio_max 必须在 0~1 之间` | 「最高抵扣比例必须在 0~1 之间」 |
| 13 | `gift_policies.go:231` | `UpdatePointSettings` | 有效期月数越界 | 400 / 40002 | `point_batch_validity_months 必须在 1~120 之间` | 「乐币有效期月数必须在 1~120 之间」 |
| 14 | `gift_policies.go:235` | `UpdatePointSettings` | 提醒天数越界 | 400 / 40002 | `point_expiry_reminder_days 必须在 0~365 之间` | 「到期提醒提前天数必须在 0~365 之间」 |
| 15 | `instrument_promo_override.go:34` | `GetInstrumentPromoOverrides` | 路径 id 非 UUID | 400 / 40002 | `invalid instrument id` | 「乐器编号不正确」 |
| 16 | `instrument_promo_override.go:54` | `UpdateInstrumentPromoOverride` | 路径 id 非 UUID | 400 / 40002 | `invalid instrument id` | 「乐器编号不正确」 |
| 17 | `instrument_promo_override.go:67` | `UpdateInstrumentPromoOverride` | `override_type` 枚举外 | 400 / 40002 | `override_type must be discount, rebate, or rent_to_own` | 「推广类型必须是折扣、返利或租转售」 |
| 18 | `membership_benefits.go:23` | `ListLevelBenefits` | 路径 id 非数字 | 400 / 40002 | `invalid level id` | 「会员等级编号不正确」 |
| 19 | `membership_benefits.go:40` | `UpdateLevelBenefits` | 路径 id 非数字 | 400 / 40002 | `invalid level id` | 「会员等级编号不正确」 |
| 20 | `membership_benefits.go:88` | `GetMembershipBenefits` | 缺 `level_id` 查询参数 | 400 / 40002 | `level_id is required` | 「请选择会员等级」 |
| 21 | `membership_benefits.go:93` | `GetMembershipBenefits` | `level_id` 非数字 | 400 / 40002 | `invalid level_id` | 「会员等级编号不正确」 |
| 22 | `membership_level.go:47` | `UpdateMembershipLevel` | 路径 id 非数字 | 400 / 40002 | `invalid id` | 「编号不正确」 |
| 23 | `membership_level.go:79` | `DeleteMembershipLevel` | 路径 id 非数字 | 400 / 40002 | `invalid id` | 「编号不正确」 |
| 24 | `points_policy.go:30` | `ListPointsPolicies` | 路由前缀无法判定作用域 | 400 / 40002 | `could not determine scope from path` | 「请求地址不正确」 |
| 25 | `points_policy.go:60` | `CreatePointsPolicy` | 同上 | 400 / 40002 | `could not determine scope from path` | 「请求地址不正确」 |
| 26 | `points_policy.go:105` | `UpdatePointsPolicy` | 路径 id 非 UUID | 400 / 40002 | `invalid id` | 「编号不正确」 |
| 27 | `user_management.go:161` | `Update` | `status` 取值非法 | 400 / 40002 | `status must be active or disabled` | 「用户状态必须是启用或停用」 |
| 28 | `user_management.go:173` | `Update` | 无可改字段 | 400 / 40002 | `no editable fields provided` | 「未提供可修改的内容」 |
| 29 | `user_management.go:393` | `GrantPoints` | `reason` 为空 | 400 / 40002 | `reason is required` | 「请填写赠送原因」 |
| 30 | `user_management.go:398` | `GrantPoints` | `amount <= 0` | 400 / 40002 | `amount must be positive` | 「赠送金额必须大于 0」 |
| 31 | `warning.go:69` | `CreateWarning` | 绑定失败 **或** `reason` 为空（同一分支） | 400 / 40001 | `reason is required` | 「请求参数有误，请填写预警原因」（两种触发共用一句，不拆分支） |
| 32 | `warning.go:154` | `UpdateWarningStatus` | 绑定失败（结构体无 required 标签，仅语法/类型错） | 400 / 40002 | `status required` | `bindmsg.Message(err)` → 「参数不合法：状态类型不正确」/「请求体格式不正确，请检查后重试」 |
| 33 | `warning.go:160` | `UpdateWarningStatus` | `status` 枚举外 | 400 / 40003 | `invalid status` | 「预警状态取值不正确」 |

**显式排除（逐字不动）**：`backend/handlers/user_management.go:142` 的 `"invalid request: " + err.Error()`（40002，归 #2604）。R1 以 `OOS_2604_LINE_UNCHANGED=1` 断言它与基线逐字一致。

### 变更点

1. **handler 文案替换（33 处 / 8 文件）**：按落点表逐点替换 `message` 字面量；`gift_policies.go:158` 保留 `strconv.FormatFloat` 动态数值（数字/小数点非 ASCII 字母）；`warning.go:154`、`discount.go:142` 改为 `bindmsg.Message(err)`（`import "tuneloop-backend/bindmsg"`）。**禁止**改 HTTP 状态、`code`、分支顺序、判定条件、日志。
2. **新增契约测试** `backend/handlers/error_message_2639_test.go`：33 个站点各 1 例 + 2 个绑定形态变体（`discount` 语法损坏、`warning` 类型不符）= **35 例**；逐例断言 HTTP、`code` 与**逐字中文 message**，并统一断言 `message` 不含 `[A-Za-z]`、不含 `_`（JSON 字段名）、不含 Go/JSON 原文短语（`invalid character`/`cannot unmarshal`/`Field validation`/`json:`/`unexpected end of JSON`/`EOF`）。fixture：折扣 6 例用一次性 PG 里插入 `discount_policies`/`discount_codes` 行（迁移已建表，缺表时 `testfixtures.AutoMigrateMissingTables`）；`user_management.Update` 2 例插 1 行 `users`；其余 25 例在 DB 访问前返回（`gift_policies` 的 3 例需 `system_settings` 可读，迁移 055 已建表）。
3. **新增证据生成器** `scripts/verify-2639.sh`（`r1`–`r4`）：`r1` 多行感知 ASCII 扫描（基线 34 / 修后 1 且命中 `user_management.go:40002:142`）+ `_` 字段名扫描 + 403/404 计数 + 前端 0 diff；`r2` 契约测试（经 `heavy-run.sh` 取锁，一次性 PG `127.0.0.1:15433`）；`r3` `go build ./...` + `gofmt -l` + `go test ./... -run '^$'` 全包编译冒烟；`r4` `api.md` §1.3 结构断言。首行 provenance `# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`。
4. **核心文档同步** `docs/spec/api/api.md` §1.3：见 `## Core Update`。

### 既有断言影响（0 翻转）

| 检查 | 结果 | 依据 |
|---|---|---|
| 既有 Go 测试断言这 33 处 message | **0 处** | 对 7 个相关既有测试（`discount_code_flow_test.go` / `gift_policy_test.go` / `membership_benefits_test.go` / `membership_flow_test.go` / `promo_override_1863_test.go` / `points_grant_test.go` / `membership_write_scope_2276_test.go`）逐一 grep `message`：仅 `membership_flow_test.go` 命中 2 处，断言的是 `微信支付身份缺失`（prepay 端点，非本单 33 处），其余 6 文件 0 命中 |
| `TestBindFieldLabelsCoverage*` / `bindmsg` 单测 | **0 翻转** | 本单不改 `backend/bindmsg`（D2），只用已登记的 `code`/`status` 称呼 |
| 前端错误解析层序 | **0 翻转** | 本单前端零改动（D6） |
| 需要翻转的语义断言 | **无** | 只改 message 文本，不改 code / HTTP / 分支顺序 / 判定条件 |

### Core Update

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | §1.3 已有「用户可见文案（#1675/#2480/#2504）」（五层回退）与「请求体绑定失败（#2603）」两条口径；未写非绑定失败的 `400xx` 服务校验文案规则，示例仍散见 `level_id is required` 一类英文常量 | 在 §1.3 第 32 行「请求体绑定失败（#2603）」段后**新增**一段 `> **400 族服务校验文案（#2639）**`：非绑定失败的可纠正业务原因（`40001`/`40002`/`40003`）`message` 必须是中文业务原因（例：优惠码已过期 / 推荐奖励比例必须在 0~1 之间 / 推广类型必须是折扣、返利或租转售），禁止英文常量（`invalid id`、`status required`、`could not determine scope from path`）与 JSON 字段名（`level_id`、`referral_ratio`）；`40002` 兜底「信息不完整，请补充必填项」会吞掉可纠正原因；需点名字段时唯一出口仍是 `backend/bindmsg`（`FieldLabel`/`Message`），不点名业务文案不得复制 bindmsg 模板；HTTP/code 维持修前基线 | 本单把 33 处英文常量改为中文业务文案，若不写清则下一位实现者会在 `bindmsg` 之外再造称呼表，或把「不点名文案」误写回字段名 |

> 与核心文档**无冲突**：§1.3 现有一条中文透传口径（L2）与 #2603 绑定失败口径原样保留，本单只是补齐「非绑定失败的 400 族」这一未覆盖分支；本单 Issue 正文未带 `## Core Update` 章节，属「扩展未覆盖分支」而非推翻既有规格，按 #2608 先例在计划中标 `[Core Update Required]` 并以本表声明改动内容。

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 33 处 ASCII 常量归零 + 多行 `gin.H` 不漏检 + 越界集合不动 | `scripts/verify-2639.sh` | `bash scripts/verify-2639.sh r1`（静态扫描 `backend/handlers`） | 修后工作树 vs `git merge-base HEAD origin/develop` 基线 | repo（静态） | `BASELINE_400XX_HITS=34`、`BASELINE_MULTILINE_HIT=1`、`POSTFIX_400XX_HITS=1`、`POSTFIX_HIT_IS_OOS_2604=1`、`OOS_2604_LINE_UNCHANGED=1`、`UNDERSCORE_400XX_HITS=0`、`OOS_403_404_COUNT=16`、`FRONTEND_DIFF=0` | R1 |
| R2 33 站点 + 2 绑定变体的中文文案（契约测试） | `backend/handlers/error_message_2639_test.go`、`backend/handlers/discount.go`、`backend/handlers/gift_policies.go`、`backend/handlers/instrument_promo_override.go`、`backend/handlers/membership_benefits.go`、`backend/handlers/membership_level.go`、`backend/handlers/points_policy.go`、`backend/handlers/user_management.go`、`backend/handlers/warning.go` | handler 直调（`httptest` + `gin`） | 33 个站点各自的拒绝输入 + 2 个绑定形态变体 | backend（一次性 PG 15433） | `GO_TEST_EXIT=0`、`MSG2639_CASES_TOTAL=35`、`MSG2639_CASES_FAIL=0`、`MSG2639_ASCII_LEAK=0`、`MSG2639_FIELDNAME_LEAK=0`、`MSG2639_HTTP_CODE_DRIFT=0`、`MSG2639_DISCOUNT_EXPIRED=优惠码已过期`、`MSG2639_REFERRAL_RANGE=推荐奖励比例必须在 0~1 之间`、`MSG2639_OVERRIDE_TYPE=推广类型必须是折扣、返利或租转售`、`MSG2639_BIND_CODE=参数不合法：优惠码不能为空`、`MSG2639_BIND_STATUS=参数不合法：状态类型不正确`、`MSG2639_WARNING_REASON=请求参数有误，请填写预警原因` | R2 |
| R3 构建 / 格式 / 全包编译冒烟 | `scripts/verify-2639.sh` | `bash scripts/verify-2639.sh r3` | 修后工作树 | backend | `GO_BUILD_EXIT=0`、`GOFMT_DIFF=0`、`TEST_COMPILE_EXIT=0` | R3 |
| R4 API 口径文档同步 | `docs/spec/api/api.md` | `bash scripts/verify-2639.sh r4`（§1.3 结构断言） | 修后文档 | 文档 | `API_400_RULE=1`、`API_400_NO_ASCII=1`、`API_400_BINDMSG_REF=1`、`API_2639_REF=1` | R4 |

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 基线 `git merge-base HEAD origin/develop` 的 10 文件 + 修后工作树 10 文件（含 `gift_policies.go` 多行 `gin.H` 样本） | `bash scripts/verify-2639.sh r1` | `docs/evidence/2639/R1-residual-scan.txt` | `BASELINE_400XX_HITS=34`、`BASELINE_MULTILINE_HIT=1`、`POSTFIX_400XX_HITS=1`、`POSTFIX_HIT_IS_OOS_2604=1`、`OOS_2604_LINE_UNCHANGED=1`、`UNDERSCORE_400XX_HITS=0`、`OOS_403_404_COUNT=16`、`FRONTEND_DIFF=0`、`SCAN_EXIT=0` | 修后残留任一 in-scope 英文常量；基线计数 ≠34（说明扫描漏检多行 `gin.H`）；`user_management.go:142` 被改动；403/404 计数漂移；`frontend-*` 出现 diff → 任一即 FAIL |
| R2 | 一次性 PG `127.0.0.1:15433` 克隆库：`discount_policies` 3 行 + `discount_codes` 6 行（停用 / 用尽 / 过期 / 策略缺失 / 未生效 / 已过期）、`users` 1 行；其余 25 例在 DB 访问前返回 | `TestErrorMessages2639`（`httptest` + `gin` 直调 33 个拒绝分支） | `docs/evidence/2639/R2-handler-messages.txt` | `GO_TEST_EXIT=0`、`MSG2639_CASES_TOTAL=35`、`MSG2639_CASES_FAIL=0`、`MSG2639_ASCII_LEAK=0`、`MSG2639_FIELDNAME_LEAK=0`、`MSG2639_HTTP_CODE_DRIFT=0`、`MSG2639_DISCOUNT_EXPIRED=优惠码已过期`、`MSG2639_REFERRAL_RANGE=推荐奖励比例必须在 0~1 之间`、`MSG2639_OVERRIDE_TYPE=推广类型必须是折扣、返利或租转售`、`MSG2639_BIND_CODE=参数不合法：优惠码不能为空`、`MSG2639_BIND_STATUS=参数不合法：状态类型不正确`、`MSG2639_WARNING_REASON=请求参数有误，请填写预警原因` | 任一例 message 含 ASCII 字母 / 含 `_` 字段名 / 含 Go-JSON 原文；HTTP 或 code 相对基线漂移；用例数 <35；`warning.go:69` 被拆成两个分支 → 任一即 FAIL |
| R3 | 修后工作树 | `bash scripts/verify-2639.sh r3` | `docs/evidence/2639/R3-build-compile.txt` | `GO_BUILD_EXIT=0`、`GOFMT_DIFF=0`、`TEST_COMPILE_EXIT=0` | 构建失败 / gofmt 有差异 / 测试包编译失败 → FAIL |
| R4 | 修后 `docs/spec/api/api.md` §1.3 | `bash scripts/verify-2639.sh r4` | `docs/evidence/2639/R4-api-doc.txt` | `API_400_RULE=1`、`API_400_NO_ASCII=1`、`API_400_BINDMSG_REF=1`、`API_2639_REF=1` | §1.3 未写 400 族中文业务文案规则 / 未禁英文常量与 JSON 字段名 / 未指向 `backend/bindmsg` 唯一出口 → 任一即 FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2639.sh r1 > docs/evidence/2639/R1-residual-scan.txt` | `docs/evidence/2639/R1-residual-scan.txt` | exit 0 且含 `BASELINE_400XX_HITS=34` `BASELINE_MULTILINE_HIT=1` `POSTFIX_400XX_HITS=1` `POSTFIX_HIT_IS_OOS_2604=1` `OOS_2604_LINE_UNCHANGED=1` `UNDERSCORE_400XX_HITS=0` `OOS_403_404_COUNT=16` `FRONTEND_DIFF=0` `SCAN_EXIT=0` | PASS（9 项 token 全为期望值） | 修后残留 in-scope 英文常量；基线计数≠34（多行 gin.H 漏检）；`user_management.go:142` 被改动；403/404 计数漂移；`frontend-*` 有 diff → FAIL | dev |
| R2 | `bash scripts/verify-2639.sh r2 > docs/evidence/2639/R2-handler-messages.txt` | `docs/evidence/2639/R2-handler-messages.txt` | exit 0 且含 `GO_TEST_EXIT=0` `MSG2639_CASES_TOTAL=35` `MSG2639_CASES_FAIL=0` `MSG2639_ASCII_LEAK=0` `MSG2639_FIELDNAME_LEAK=0` `MSG2639_HTTP_CODE_DRIFT=0` `MSG2639_DISCOUNT_EXPIRED=优惠码已过期` `MSG2639_REFERRAL_RANGE=推荐奖励比例必须在 0~1 之间` `MSG2639_OVERRIDE_TYPE=推广类型必须是折扣、返利或租转售` `MSG2639_BIND_CODE=参数不合法：优惠码不能为空` `MSG2639_BIND_STATUS=参数不合法：状态类型不正确` `MSG2639_WARNING_REASON=请求参数有误，请填写预警原因` | PASS（12 项 token 全为期望值） | 任一响应含 ASCII 字母或 `_` 字段名；含 `invalid character`/`cannot unmarshal`/`Field validation`/`json:`；HTTP 或 code 漂移；用例数<35 → FAIL | dev |
| R3 | `bash scripts/verify-2639.sh r3 > docs/evidence/2639/R3-build-compile.txt` | `docs/evidence/2639/R3-build-compile.txt` | exit 0 且含 `GO_BUILD_EXIT=0` `GOFMT_DIFF=0` `TEST_COMPILE_EXIT=0` | PASS（3 项 token 全为期望值） | 构建失败 / gofmt 差异 / 全包编译失败 → FAIL | dev |
| R4 | `bash scripts/verify-2639.sh r4 > docs/evidence/2639/R4-api-doc.txt` | `docs/evidence/2639/R4-api-doc.txt` | exit 0 且含 `API_400_RULE=1` `API_400_NO_ASCII=1` `API_400_BINDMSG_REF=1` `API_2639_REF=1` | PASS（4 项 token 全为期望值） | §1.3 缺 400 族中文业务文案规则 / 未禁英文常量与 JSON 字段名 / 未指向 `backend/bindmsg` → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2639.sh`（`r1`–`r4`）；每个文本 artifact 首行 `# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，随后 `GENERATED_AT_HEAD=<40 位 sha>`；证据必须在 fix commit **之后**生成（`FIX_REF` 可覆盖锚点，默认 `git log --grep="^fix(issue-2639)"`）。
- 重命令（`go build` / `go test` / `gofmt`）一律经 `~/.config/opencode/scripts/heavy-run.sh` 取锁；契约测试用一次性 PostgreSQL（`127.0.0.1:15433`，`#2341` 口径，模板库 `tuneloop_test_template` 克隆为节点库），**不使用** `jobmaster-postgres`。
- 逐条清单（全部位于 `docs/evidence/2639/`）：
  - `docs/evidence/2639/R1-residual-scan.txt`
  - `docs/evidence/2639/R2-handler-messages.txt`
  - `docs/evidence/2639/R3-build-compile.txt`
  - `docs/evidence/2639/R4-api-doc.txt`
- 其它持久证据：`docs/evidence/2639/plan.md`（与本 Accepted Plan 评论逐字一致，`scripts/plan-source.sh --issue 2639 --fresh` 校验 sha256）、`docs/evidence/2639/evidence.json`、`docs/evidence/2639/pre-ready-2639.json`。
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Input Truth & Tool Preflight

- **Source of truth**：
  - 落点集合 = 多行感知扫描 `count_ascii_400xx()`（`scripts/verify-2639.sh`）：逐行维护「最近一个 `"code": NNNNN`」状态，`400xx` 之后的 `"message": "[A-Za-z]` 在 **≤3 行**内配对；单行 grep 只能看到 33 处，**必须**用该扫描，否则 `gift_policies.go:155-158` 会被漏掉（Issue 表即漏了它）。
  - code / HTTP 基线 = 逐点核对落点表（40001 × 1、40002 × 31、40003 × 1，全部 HTTP 400）。
  - 前端解析层序 = `frontend-pc/src/services/errorMessages.js` 的 `resolveErrorCore`（L2 含 CJK 直接透传）。
  - 文档口径 = `docs/spec/api/api.md` §1.3（本轮新增 400 族段落）。
- **实施前差异核对（先跑，结果入 Work Summary）**：`BASELINE_400XX_HITS=34`（`POSTFIX_400XX_HITS` 目标 1）；`OOS_403_404_COUNT=16` 与修后一致；`grep -c '"invalid request: " + err.Error()' backend/handlers/user_management.go` = 1（修后仍为 1 且逐字一致）。
- **损坏输入拒绝规则**（契约测试必须覆盖，且都为中文）：控制字符/截断 → `bindmsg` 语法文案「请求体格式不正确，请检查后重试」；字段类型不符（`{"status":123}`）→ 「参数不合法：状态类型不正确」；缺必填（`{}` → `code`）→ 「参数不合法：优惠码不能为空」；空 body（`EOF`）→ 「参数不合法，请检查后重试」。
- **工具盲区（显式登记）**：
  - 扫描器只覆盖 `backend/handlers/*.go` 的 `gin.H` 字面量；`c.JSON(..., gin.H{...})` 之外的自定义响应封装不在扫描内（本单 33 处全在 `gin.H`）→ 以「落点表逐点人工核对」+ 扫描计数双向兜底。
  - `warning.go:69` 是 `if err := ShouldBindJSON(...); err != nil || req.Reason == ""` 的**合流分支**：无法区分语法错与空原因，故用一句覆盖两种触发（D4），不拆分分支（拆分即改变分支结构）。
  - 契约测试只覆盖 handler 层（不经 nginx/IAM 中间件）；PC/H5/weapp 真实交互复验不在本单（前端零改动，D6）。
  - `go test ./handlers` 由 `TestMain` 克隆模板库 → PG `127.0.0.1:15433` 不可达必须显式失败，**禁止**静默跳过。
  - 全量 `go test ./...` 不作合同行（P5 baseline 差分承担；R3 只做 `-run '^$'` 全包编译冒烟）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 口径：本单 = **`400xx`（40001/40002/40003）**，403xx/404xx 16 处与 50000 内部错误显式排除（`40002` 兜底文案会吞掉可纠正原因，403/404 兜底语义正确） | 本计划锁定 |
| D2 | **不改 `backend/bindmsg`**：不新增称呼、不加作用域、不复制模板；兄弟单 #2604/#2605/#2608 已各自扩展该包，第四次扩展会造成同名 map key 重复（编译错误）与作用域类型冲突 | 本计划锁定 |
| D3 | 需点名 JSON 字段的分支只允许两种结局：① `bindmsg.Message(err)`（既有称呼 `code`→优惠码、`status`→状态）；② 不点名字段的中文业务文案（本单 31 处）。**禁止**在 handler 内拼 `level_id`/`referral_ratio` 等 ASCII 字段名 | 本计划锁定 |
| D4 | `warning.go:69` 合流分支保持单分支结构，用「请求参数有误，请填写预警原因」覆盖语法错与空原因两种触发 | 本计划锁定 |
| D5 | 只替换 `message` 表达式；HTTP 状态、`code`、分支顺序、判定条件、日志一律不动（逐点见落点表基线列） | 本计划锁定 |
| D6 | 前端（PC / H5 / weapp）零改动；R1 以 `FRONTEND_DIFF=0` 断言 | 本计划锁定 |
| D7 | `user_management.go:142`（`"invalid request: "` 前缀族）归 #2604，本单不改，R1 以 `OOS_2604_LINE_UNCHANGED=1` 钉住逐字内容 | 本计划锁定 |
| D8 | 文案措辞与 `bindmsg` 已登记称呼（#2608）保持一致（会员等级 / 推荐奖励比例 / 推荐注册赠点 / 最高抵扣比例 / 乐币有效期月数 / 到期提醒提前天数 / 推广类型），避免同一概念两种称呼；本单只是**字面量**，不引用其代码 | 本计划锁定 |
| D9 | 重命令经 `heavy-run.sh`；契约测试用一次性 PG（`127.0.0.1:15433`），不碰 dev 库 | 本计划锁定 |
| D10 | 证据必须在 fix commit **之后**生成；`docs/evidence/2639/plan.md` 与 Accepted Plan 评论逐字一致 | 本计划锁定 |
| D11 | 全量 `go test ./...` 不作合同行（P5 baseline 承担）；R2 只跑 `go test ./handlers -run TestErrorMessages2639`，R3 跑 `-run '^$'` 编译冒烟 | 本计划锁定 |

### Out of Scope

- `user_management.go:142` 的 `"invalid request: " + err.Error()`（40002）→ **#2604**（已完成，等待合入 `origin/develop`）；本单只断言其逐字不变，最终全仓 `400xx` 归零由 #2604 合并承担。
- 10 文件内 16 处 `403xx`/`404xx` 英文常量（`merchant_rebate.go` 的 `tenant required`/`merchant not found`、`points_policy.go` 的 `merchant scope not found`/`policy not found`、`points_policy.go:144`、`webhook_config.go` 等）→ 前端 `ERROR_CODE_MAP` 对 40300/40400 的兜底语义正确，另立后续批次，不阻塞本单。
- 全仓 `backend/handlers/*.go` 其它 `400xx` 英文常量（不在这 10 文件内，如同一基线的其它域）与约 1550 处 `50000`/`20000` 文案 → 全站文案收敛另立批次。
- `instrument.go` 的 400 族（绑定裸 `err.Error()` 与 6 处不可达 `Marshal` 分支）→ **#2606**。
- 前端任何改动（`resolveErrorCore`、`ERROR_MESSAGE_MAP`、`ERROR_CODE_MAP`）与 PC 表单内常驻提示复验。
- 引入 `dive` 嵌套校验、拆分 `warning.go:69` 合流分支、统一 `40002` 语义到新 code（均属行为变更，另立 Issue）。
- 全量 `go test ./...` 作为合同行（由 pre-ready P5 baseline 差分承担）。

### Dependencies

- #2608

### Files（变更文件清单）

- `backend/handlers/discount.go`
- `backend/handlers/gift_policies.go`
- `backend/handlers/instrument_promo_override.go`
- `backend/handlers/membership_benefits.go`
- `backend/handlers/membership_level.go`
- `backend/handlers/points_policy.go`
- `backend/handlers/user_management.go`
- `backend/handlers/warning.go`
- `backend/handlers/error_message_2639_test.go`
- `scripts/verify-2639.sh`
- `docs/spec/api/api.md`
- `docs/evidence/2639/plan.md`
- `docs/evidence/2639/evidence.json`
- `docs/evidence/2639/pre-ready-2639.json`
- `docs/evidence/2639/R1-residual-scan.txt`
- `docs/evidence/2639/R2-handler-messages.txt`
- `docs/evidence/2639/R3-build-compile.txt`
- `docs/evidence/2639/R4-api-doc.txt`

### Workload

- Size: M
- Basis: 8 个 handler 文件 + 1 新契约测试（35 例）+ 1 证据脚本（4 个子命令）+ 1 核心文档段落 + 5 个持久证据文件；33 处文案替换，无 migration、无新端点、无前端改动、无跨仓依赖；测试需一次性 PG 与 7 行 fixture
- Confidence: high

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状（symptom） | 折扣 / 礼品积分 / 会员等级权益 / 用户与预警等后台操作被拒时，只看到「信息不完整，请补充必填项」，看不到真实原因（如优惠码已过期） | `frontend-pc/src/services/errorMessages.js:15` |
| 2 | 触发条件（trigger） | `40001`/`40002`/`40003` 分支 `message` 为英文常量或含 JSON 字段名（33 处） | 落点表 |
| 3 | 根因（root cause） | handler 内直接写死英文常量/字段名，未经 `bindmsg` 或中文业务文案；前端 L2 只透传含 CJK 的 `message`，ASCII 被判「无具体原因」→ L3 `ERROR_CODE_MAP[40002]` | `resolveErrorCore` L2/L3 |
| 4 | 测试盲点（test blind spot） | 既有测试只断言 HTTP/code/成功路径，无一条断言这 33 处 message 文案；「构建通过 + 400」被当作验证充分 | `grep -n 'message' backend/handlers/*_test.go`（33 处 0 断言） |
| 5 | 同类扫描（same-class paths） | 同族仍有多批：`invalid request:` 前缀 25 处（#2604）、其它前缀（#2605）、`instrument.go`（#2606）、403/404 16 处（本单排除）、全仓 ~1550 处 | Input Truth 计数 |
| 6 | 修复层（fix layer） | 仅提示层：33 处 `message` 文案；不改业务判定/状态码/鉴权/DB 写入 | 落点表 + D5 |
| 7 | 共享契约边界（shared contract） | 字段称呼与绑定模板唯一 owner 仍是 `backend/bindmsg`；本单不扩展该包，需点名的 2 处复用既有出口，其余 31 处不点名字段 | D2/D3 |
| 8 | 红绿实证（red-green） | 红：仅加扫描与测试、不改文案 → R1 报告修后计数仍为 34（`POSTFIX_400XX_HITS` 不符），且 R2 35 例中 33 例 FAIL；绿：替换文案后 R1 命中 1 处且为 #2604 行，R2 35/35 PASS | R1/R2 |
| 9 | 防复发（prevention） | ① 多行感知静态收口（含 `gin.H` 换行写法）；② 每例「message 无 ASCII / 无 `_`」硬断言；③ 403/404 与 #2604 行计数/内容钉死；④ §1.3 写明 400 族口径 | R1/R2/R4 |
| 10 | 影响面（blast radius） | 仅失败分支文案；成功路径、状态码、鉴权、DB 写入不变；前端零改动 | R2 的 code 断言、R1 `FRONTEND_DIFF=0` |
| 11 | 未修边界（deferred） | 403/404 16 处、`user_management.go:142`（#2604）、`instrument.go`（#2606）、全仓文案收敛 | Out of Scope |

*Model: deepseek/deepseek-flash*

