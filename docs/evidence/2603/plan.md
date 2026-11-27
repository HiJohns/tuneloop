## Accepted Plan — #2603 [Bug] 绑定失败文案中文化（26 处收口 `bindmsg`，点名字段；禁止 Go/JSON 原文进响应体）

### 背景与现状核对（代码为据）

| # | 事实 | 位置 |
|---|---|---|
| 1 | 现状共 **28 处 / 13 文件** 直出绑定层原文：`"invalid parameters: " + err.Error()` | `grep -rn 'invalid parameters: " + err.Error()' backend/handlers/*.go \| wc -l` → 28（site.go 2 + 其余 12 文件 26） |
| 2 | 唯一出口 `bindmsg.Message(err)` 已落地（本单种子，**尚未提交**，当前在主工作树 `develop`）：JSON 语法错误 / 类型不匹配 / validator 标签 四类映射 + 未登记字段回显 JSON 名，绝不外泄原文 | `backend/bindmsg/bindmsg.go`（`Message` / `bindFieldLabel` / `camelToSnake`）、`backend/bindmsg/bindmsg_test.go` |
| 3 | `site.go` 创建/更新两处已改（种子） | `backend/handlers/site.go:232`、`:569` |
| 4 | 剩余 26 处分布：property 5、warehouse 4、wechatpay_prepay 2、renewal 2、mall_image 2、inventory 2、iam_proxy 2、banner 2、appeal 2、payment_calculate 1、instrument 1、coupon_pricing 1 | 本计划「26 处落点」表 |
| 5 | 26 处**全部**是一行 `c.JSON(... "message": "invalid parameters: " + err.Error())`，紧跟 `if err := c.ShouldBindJSON(&req); err != nil {`，**绑定失败即 return**；前置只有取参/id 判空/取 DB，无鉴权分支 | 逐点核对结果（见落点表「前置」列） |
| 6 | 前端不需要改：`resolveErrorCore` 层序 L2 = 含 CJK 透传（先于 L1/L3），后端中文文案直达用户；40002 属业务可纠正冲突白名单 | `frontend-pc/src/services/errorMessages.js:97-134`、`docs/spec/api/api.md:42` |
| 7 | 26 处请求体**均不含 `dive`**（validator 不会产出 `items[0].id` 这类嵌套路径），故**不做**嵌套路径特化（否则是不可达代码） | `grep -n 'dive' backend/handlers/{property,warehouse,wechatpay_prepay,renewal,mall_image,inventory,iam_proxy,banner,appeal,payment_calculate,instrument,coupon_pricing}.go` → 0 |
| 8 | 既有测试**无**断言这些 40002 文案：`grep -rn 'invalid parameters' --include=*_test.go backend` → 1 命中（`bindmsg_test.go:6` 注释） | 见「既有断言影响」 |
| 9 | 同类但不同字面量的缺陷仍有 25 处 / 17 文件（`"invalid request: " + err.Error()`），本单**不动** | 已另立 **#2604**（`status:todo`，依赖本单） |

**结论**：缺陷在提示层（用户看不到「哪个字段错了」），修复 = 把 26 处 message 表达式收敛到唯一出口 `bindmsg.Message(err)` + 把 26 处请求体字段全部登记中文称呼，并用「26 端点 × 2 用例」的 handler 契约测试把「不得出现 Go/JSON 原文 / 不得出现 ASCII 字段名」钉成机器可判定的负例。

### 变更点

#### 1. 基础层：`backend/bindmsg`（唯一出口，纯函数）

- `Message(err)` 分支顺序保持不变（validator → `json.UnmarshalTypeError` → `json.SyntaxError` → 兜底）。
- `bindFieldLabels` 补全 **26 处请求体的全部字段**（现有 27 条 → 本单新增 61 条，共 88 条），称呼**不含 ASCII 字母**（`instrument_sn` → `乐器序列号`，不得写成 `乐器 SN`）。
- 未登记字段的回退保持「回显 JSON snake 名」（绝不回显 Go 字段名 `ManagerPhone`）；本单要求 26 端点字段全部登记，故真实响应不会走到该回退。
- **不做**嵌套 `items[0].id` 特化（见背景第 7 条，当前不可达）。

#### 2. 26 处替换（按域 3 批，每批以 grep 收口）

| 批 | 文件 | 处数 | 收口断言 |
|---|---|---|---|
| B1 组织/账户 | `backend/handlers/iam_proxy.go`(2)、`backend/handlers/instrument.go`(1) | 3（site.go 2 处已在种子改动内） | `grep -c 'invalid parameters: " + err.Error()' <file>` = 0 |
| B2 仓储/库存/申诉 | `backend/handlers/warehouse.go`(4)、`backend/handlers/inventory.go`(2)、`backend/handlers/appeal.go`(2) | 8 | 同上 |
| B3 商品/内容/支付/营销 | `backend/handlers/property.go`(5)、`backend/handlers/banner.go`(2)、`backend/handlers/mall_image.go`(2)、`backend/handlers/coupon_pricing.go`(1)、`backend/handlers/wechatpay_prepay.go`(2)、`backend/handlers/renewal.go`(2)、`backend/handlers/payment_calculate.go`(1) | 15 | 同上 |

替换规则（逐点机械）：

```go
// before
c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "invalid parameters: " + err.Error()})
// after（保留原 code / HTTP 状态 / 分支；只换 message 表达式）
c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": bindmsg.Message(err)})
```

- 新增 `import "tuneloop-backend/bindmsg"`（site.go 已示范）。
- **禁止**改动任何 400 族判定分支、状态码、顺序（`instrument.go:870` 保持 `40001`）。
- 非本单的 400 族语义混杂点（`invalid request: ` 25 处）必须原样保留，并由 R3 断言其计数不变（防顺手扩面）。

#### 3. 26 处落点（file:line / 函数 / 前置）

| # | 位置 | 函数 | 绑定点前置 | 缺必填时的点名字段（示例用例） |
|---|---|---|---|---|
| 1 | `backend/handlers/property.go:152` | `CreateProperty` | 取 DB | `property_type`（提交 `{"name":"x"}`） |
| 2 | `backend/handlers/property.go:239` | `UpdateProperty` | 取 DB + `:id` | `name` |
| 3 | `backend/handlers/property.go:323` | `CreatePropertyOption` | 取 DB | `property_id` |
| 4 | `backend/handlers/property.go:373` | `ConfirmPropertyValue` | 取 DB | `value` |
| 5 | `backend/handlers/property.go:497` | `MergePropertyValues` | 取 DB | `source_value` |
| 6 | `backend/handlers/warehouse.go:176` | `UpdateShipping` | 无 | `tracking_number` |
| 7 | `backend/handlers/warehouse.go:326` | `ConfirmDelivery` | 无 | `delivered_at` |
| 8 | `backend/handlers/warehouse.go:526` | `InspectReturn` | 无 | `instrument_sn` |
| 9 | `backend/handlers/warehouse.go:876` | `AssessDamage` | 无 | `damage_description` |
| 10 | `backend/handlers/wechatpay_prepay.go:57` | `PrepayOrder` | 无 | `order_type` |
| 11 | `backend/handlers/wechatpay_prepay.go:678` | `QueryPayment` | 无 | `out_trade_no` |
| 12 | `backend/handlers/renewal.go:132` | `CalculateRenewal` | `:id` | `additional_days` |
| 13 | `backend/handlers/renewal.go:225` | `ConfirmRenewal` | `:id` | `additional_days` |
| 14 | `backend/handlers/mall_image.go:56` | `CreateMallImage` | 取 DB | `image_url` |
| 15 | `backend/handlers/mall_image.go:111` | `UpdateMallImage` | 取 DB + `:id` + 查行 | 无必填 → 用类型用例 `sort_order:"abc"` |
| 16 | `backend/handlers/inventory.go:134` | `TransferInventory` | 无 | `asset_id` |
| 17 | `backend/handlers/inventory.go:413` | `BatchUpdateRent` | 无 | `items`（提交 `{}`） |
| 18 | `backend/handlers/iam_proxy.go:282` | `CreateUser` | 无 | `name` |
| 19 | `backend/handlers/iam_proxy.go:578` | `UpdateIAMUser` | `:id` | 无必填 → 用类型用例 `name:123` |
| 20 | `backend/handlers/banner.go:66` | `CreateBanner` | 取 DB | `image_url` |
| 21 | `backend/handlers/banner.go:135` | `UpdateBanner` | 取 DB + `:id` + 查行 | 无必填 → 用类型用例 `sort_order:"abc"` |
| 22 | `backend/handlers/appeal.go:212` | `ResolveAppeal` | 无 | `decision` |
| 23 | `backend/handlers/appeal.go:460` | `SubmitAppeal` | 无 | `damage_report_id` |
| 24 | `backend/handlers/payment_calculate.go:41` | `CalculatePayment` | 无 | `type`（提交 `{}`） |
| 25 | `backend/handlers/instrument.go:870` | `UpdateInstrumentStatus` | `:id` | `stock_status` |
| 26 | `backend/handlers/coupon_pricing.go:64` | `ValidateCoupon` | 无 | `code` |

#### 4. 字段称呼表（唯一 owner：`backend/bindmsg/bindmsg.go`，本单新增 61 条）

| 域 | 新增条目（字面量逐字锁定，**不得含 ASCII 字母**） |
|---|---|
| 账户/组织 | `email`→邮箱、`username`→账号、`password`→密码、`skip_activation`→跳过激活、`id`→编号 |
| 乐器 | `stock_status`→库存状态 |
| 仓储 | `tracking_number`→物流单号、`company`→物流公司、`shipped_at`→发货时间、`shipping_fee`→物流费、`delivered_at`→收货时间、`instrument_sn`→乐器序列号、`scan_time`→验收时间、`condition`→乐器状况、`notes`→备注、`damage_amount`→定损金额、`damage_description`→损坏说明、`overdue_fee`→逾期费、`additional_shipping_fee`→追加物流费 |
| 库存 | `asset_id`→资产、`from_site_id`→调出网点、`to_site_id`→调入网点、`items`→乐器列表、`daily_rent`→日租金、`deposit`→押金、`deposit_reduced_enabled`→减免押金开关、`deposit_reduced`→减免后押金、`overdue_daily_fee`→逾期日租金 |
| 申诉 | `decision`→处理决定、`adjust_amount`→调整金额、`damage_report_id`→定损单、`appeal_reason`→申诉原因 |
| 属性 | `property_type`→属性类型、`unit`→单位、`options`→可选值、`scope_type`→取值范围、`related_category_id`→关联分类、`related_property_id`→关联属性、`property_id`→属性、`property_name`→属性名称、`value`→属性值、`new_value`→新值、`source_value`→原值、`target_value`→目标值 |
| 内容 | `image_url`→图片地址、`link_url`→链接地址、`title`→标题、`sort_order`→排序、`bg_color`→背景色 |
| 支付/营销 | `order_id`→订单、`order_type`→订单类型、`open_id`→微信身份标识、`gift_used`→乐币抵扣、`session_id`→支付会话、`coupon_code`→优惠码、`code`→优惠码、`deposit_tier`→押金档位、`guarantor_ids`→担保人列表、`out_trade_no`→商户订单号、`additional_days`→续租天数、`amount_cents`→金额 |

#### 5. 文案模板（逐字锁定，前端 L2 透传）

| 输入 | 输出 |
|---|---|
| validator `required` | `参数不合法：<称呼>不能为空` |
| validator `email` | `参数不合法：<称呼>格式不正确` |
| validator `uuid`/`uuid4` | `参数不合法：<称呼>不是合法的唯一标识` |
| validator 数值 (`numeric`/`gte`/`lte`/`gt`/`lt`) | `参数不合法：<称呼>取值不合法` |
| validator `oneof` | `参数不合法：<称呼>取值不在允许范围内` |
| validator 其它标签 | `参数不合法：<称呼>不合法` |
| `json.UnmarshalTypeError` | `参数不合法：<称呼>类型不正确` |
| `json.SyntaxError`（控制字符/截断） | `请求体格式不正确，请检查后重试` |
| 其它（含 EOF）/ nil | `参数不合法，请检查后重试` |

#### 6. 测试

- `backend/bindmsg/bindmsg_test.go`（扩充种子）：模板 9 条 + 称呼表覆盖（26 端点字段全集 88 条全部无 ASCII）+ 未登记字段回退（`unknown_field_xyz`，不得回显 Go 名）+ `Message(nil)`。
- `backend/handlers/bind_error_2603_test.go`（新增）：`TestBindErrorMessages2603` = **26 端点 × 2 用例**（`{控制字符损坏体}` → `请求体格式不正确，请检查后重试`；`{缺必填 / 类型不匹配}` → 含中文称呼的 `参数不合法：…`），每例断言：
  1. HTTP 400 且 `code` 与改动前一致（`instrument` = 40001，其余 40002）；
  2. `message` **不含任何 ASCII 字母**（`[A-Za-z]` 计数 0）；
  3. `message` **不含** `invalid character` / `invalid parameters` / `cannot unmarshal` / `Field validation` / `json:`；
  4. 缺必填例 `message == "参数不合法：<称呼>不能为空"`（逐字相等）。
  - `banner`/`mall_image` 更新分支需先 seed 行（`testfixtures.SetupTestDB` + `models.Banner`/`models.MallImage`）。
- **红绿实证**：先只提交测试 + 种子 helper（不含 26 处替换），跑 `TestBindErrorMessages2603` → FAIL（`message` 为 `invalid parameters: …`）；再提交替换 → PASS。Work Summary 必须贴红样片段。

#### 7. 证据生成器 + 文档

- 新增 `scripts/verify-2603.sh`（`r1`–`r5`，沿用 `scripts/verify-2581.sh` 结构：provenance 首行、`heavy-run.sh` 取锁、一次性 PG `#2341` 口径）。
- `docs/spec/api/api.md` §1.3 追加一条口径（见 Core Update）。

### 既有断言影响（0 翻转）

| 检查 | 结果 | 依据 |
|---|---|---|
| Go 测试断言这 28 处文案 | **0 处** | `grep -rn 'invalid parameters' --include=*_test.go backend` → 仅 `bindmsg_test.go:6` 注释 |
| 前端测试断言英文原文 | **0 处** | `frontend-pc/tests/errorMessages.test.mjs` 只断言层序与 `code_map`；`ERROR_MESSAGE_MAP['invalid parameters']` 键**保留**（仍被 `backend/handlers/user.go:37` 的字面量使用） |
| 需要翻转的语义断言 | **无** | 本单只改 message 文本，不改 code / HTTP / 分支判定 |

### Files（变更文件清单）

- `backend/bindmsg/bindmsg.go`
- `backend/bindmsg/bindmsg_test.go`
- `backend/handlers/site.go`
- `backend/handlers/property.go`
- `backend/handlers/warehouse.go`
- `backend/handlers/wechatpay_prepay.go`
- `backend/handlers/renewal.go`
- `backend/handlers/mall_image.go`
- `backend/handlers/inventory.go`
- `backend/handlers/iam_proxy.go`
- `backend/handlers/banner.go`
- `backend/handlers/appeal.go`
- `backend/handlers/payment_calculate.go`
- `backend/handlers/instrument.go`
- `backend/handlers/coupon_pricing.go`
- `backend/handlers/bind_error_2603_test.go`
- `scripts/verify-2603.sh`
- `docs/spec/api/api.md`
- `docs/evidence/2603/plan.md`
- `docs/evidence/2603/R1-bindmsg-unit.txt`
- `docs/evidence/2603/R2-handler-messages.txt`
- `docs/evidence/2603/R3-residual-scan.txt`
- `docs/evidence/2603/R4-build-compile.txt`
- `docs/evidence/2603/R5-api-doc.txt`
- `docs/evidence/2603/evidence.json`
- `docs/evidence/2603/pre-ready-2603.json`

### Core Update

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | §1.3 只写「`message` 为机器可读标识（英文为主）」+ 五层回退，未写**绑定失败**这一子类的口径 | §1.3 追加一条：**请求体绑定失败（#2603）**——`c.ShouldBindJSON` 失败时 `message` 必须由 `backend/bindmsg.Message(err)` 生成（中文点名「哪一项不能为空 / 格式不对」或「请求体格式不正确，请检查后重试」），**禁止**拼接 `err.Error()` 的 Go/JSON 原文（`invalid character` / `invalid parameters: …`）；该中文经 L2 透传直达用户，40002 属业务可纠正冲突白名单（#2569 口径可落表单内常驻提示） | 这是 §1.3「英文为主的机器码」的一处**已存在例外**的显式化（同 `§4.5.1` 网点端点已写「40002 具体原因（中文）」）；不写清会导致后续 handler 继续直出原文 |

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 翻译模板 + 字段称呼表（88 条，无 ASCII） | `backend/bindmsg/bindmsg.go`、`backend/bindmsg/bindmsg_test.go` | `go test ./bindmsg` | 9 类错误输入 + 26 端点字段全集 + 未登记字段 | backend（纯函数） | 9 条模板逐字相等；`LABEL_ASCII_LEAK=0`、`LABEL_COVERAGE_MISSING=0`、未登记回退为 JSON snake 名 | R1 |
| R2 26 端点绑定失败文案（契约测试） | `backend/handlers/bind_error_2603_test.go`、`backend/handlers/site.go`、`backend/handlers/property.go`、`backend/handlers/warehouse.go` | `POST/PUT` 26 个端点（handler 直调） | 损坏请求体（控制字符）/ 缺必填 / 类型不匹配，共 52 例 | backend（一次性 PG 15433） | `BIND2603_CASES_TOTAL=52`、`CASES_FAIL=0`、`ASCII_LEAK=0`、`RAW_PHRASE_LEAK=0`，且 HTTP 400 + code 与修前一致 | R2 |
| R3 收口扫描（26 处全替换、同类不外溢） | `scripts/verify-2603.sh` | 静态扫描 `backend/handlers` | 修后工作树 | repo（静态） | `RESIDUAL_BIND_RAW=0`、`RESIDUAL_INVALID_PREFIX=0`、`BINDMSG_CALL_SITES=28`、`OUT_OF_SCOPE_INVALID_REQUEST=25` | R3 |
| R4 构建 / gofmt / 测试编译 | `scripts/verify-2603.sh` | `cd backend && go build ./...` 等 | 修后工作树 | backend | `GO_BUILD_EXIT=0`、`GOFMT_DIFF=0`、`TEST_COMPILE_EXIT=0` | R4 |
| R5 API 口径文档同步 | `docs/spec/api/api.md` | 文档结构断言 | 修后文档 | 文档 | `API_BIND_RULE=1`、`API_FORBID_RAW=1`、`API_BINDMSG_REF=1` | R5 |

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 9 类错误输入（validator required/email/uuid/数值/oneof/其它、`UnmarshalTypeError`、含 `\x17` 的 `SyntaxError`、`EOF`/nil）+ 26 端点字段全集表 | `TestBindErrorMessage2603` / `TestBindFieldLabelsCoverage2603` | `docs/evidence/2603/R1-bindmsg-unit.txt` | `BINDMSG_UNIT_EXIT=0`、`BMSG_REQUIRED=参数不合法：名称不能为空`、`BMSG_EMAIL=参数不合法：邮箱格式不正确`、`BMSG_UUID=参数不合法：编号不是合法的唯一标识`、`BMSG_RANGE=参数不合法：数量取值不合法`、`BMSG_ONEOF=参数不合法：状态取值不在允许范围内`、`BMSG_TYPE=参数不合法：纬度类型不正确`、`BMSG_SYNTAX=请求体格式不正确，请检查后重试`、`BMSG_FALLBACK=参数不合法，请检查后重试`、`LABEL_ASCII_LEAK=0`、`LABEL_COVERAGE_MISSING=0` | 任一模板含 ASCII 字母、含 `invalid`/`cannot unmarshal`/`Field validation`、`LABEL_COVERAGE_MISSING>0`、回退返回 Go 字段名（如 `ManagerPhone`）→ 任一即 FAIL |
| R2 | `testfixtures.SetupTestDB` + seed `Banner`/`MallImage` 各 1 行；26 端点各 2 请求体 | `TestBindErrorMessages2603`（`httptest` + `gin` 直调 26 handler） | `docs/evidence/2603/R2-handler-messages.txt` | `GO_TEST_EXIT=0`、`BIND2603_CASES_TOTAL=52`、`BIND2603_CASES_FAIL=0`、`BIND2603_ASCII_LEAK=0`、`BIND2603_RAW_PHRASE_LEAK=0`、`BIND2603_SITE_MSG=参数不合法：名称不能为空`、`BIND2603_PROPERTY_MSG=参数不合法：属性类型不能为空`、`BIND2603_WAREHOUSE_MSG=参数不合法：物流单号不能为空`、`BIND2603_COUPON_MSG=参数不合法：优惠码不能为空`、`BIND2603_SYNTAX_MSG=请求体格式不正确，请检查后重试` | 任一端点返回 `invalid character`/`invalid parameters`/`cannot unmarshal`/EOF、返回非 400、code 变化（40002→其它 或 instrument 40001→40002）、消息含 ASCII 字母 → 任一即 FAIL |
| R3 | 修后 `backend/handlers` 全目录 | `bash scripts/verify-2603.sh r3` | `docs/evidence/2603/R3-residual-scan.txt` | `RESIDUAL_BIND_RAW=0`、`RESIDUAL_INVALID_PREFIX=0`、`BINDMSG_CALL_SITES=28`、`BINDMSG_HANDLER_FILES=13`、`OUT_OF_SCOPE_INVALID_REQUEST=25` | 残留任一 `invalid parameters: " + err.Error()`、调用点数 ≠28、`invalid request:` 计数被改动（≠25）→ 任一即 FAIL |
| R4 | 修后工作树 | `bash scripts/verify-2603.sh r4` | `docs/evidence/2603/R4-build-compile.txt` | `GO_BUILD_EXIT=0`、`GOFMT_DIFF=0`、`TEST_COMPILE_EXIT=0` | 构建失败 / gofmt 有差异 / 测试包编译失败 → FAIL |
| R5 | 修后 `docs/spec/api/api.md` §1.3 | `bash scripts/verify-2603.sh r5` | `docs/evidence/2603/R5-api-doc.txt` | `API_BIND_RULE=1`、`API_FORBID_RAW=1`、`API_BINDMSG_REF=1` | §1.3 未写绑定失败口径 / 未禁 `err.Error()` 原文 / 未引用 `backend/bindmsg` → 任一即 FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2603.sh r1 > docs/evidence/2603/R1-bindmsg-unit.txt` | `docs/evidence/2603/R1-bindmsg-unit.txt` | exit 0 且含 `BINDMSG_UNIT_EXIT=0` `BMSG_REQUIRED=参数不合法：名称不能为空` `BMSG_EMAIL=参数不合法：邮箱格式不正确` `BMSG_UUID=参数不合法：编号不是合法的唯一标识` `BMSG_RANGE=参数不合法：数量取值不合法` `BMSG_ONEOF=参数不合法：状态取值不在允许范围内` `BMSG_TYPE=参数不合法：纬度类型不正确` `BMSG_SYNTAX=请求体格式不正确，请检查后重试` `BMSG_FALLBACK=参数不合法，请检查后重试` `LABEL_ASCII_LEAK=0` `LABEL_COVERAGE_MISSING=0` | PASS（11 项 token 全为期望值） | 模板含 ASCII/`invalid`/`Field validation`、称呼表漏字段、回退返回 Go 字段名 → FAIL | dev |
| R2 | `bash scripts/verify-2603.sh r2 > docs/evidence/2603/R2-handler-messages.txt` | `docs/evidence/2603/R2-handler-messages.txt` | exit 0 且含 `GO_TEST_EXIT=0` `BIND2603_CASES_TOTAL=52` `BIND2603_CASES_FAIL=0` `BIND2603_ASCII_LEAK=0` `BIND2603_RAW_PHRASE_LEAK=0` `BIND2603_SITE_MSG=参数不合法：名称不能为空` `BIND2603_PROPERTY_MSG=参数不合法：属性类型不能为空` `BIND2603_WAREHOUSE_MSG=参数不合法：物流单号不能为空` `BIND2603_COUPON_MSG=参数不合法：优惠码不能为空` `BIND2603_SYNTAX_MSG=请求体格式不正确，请检查后重试` | PASS（10 项 token 全为期望值） | 任一响应含 Go/JSON 原文或 ASCII 字母、HTTP/code 变化、用例数<52 → FAIL | dev |
| R3 | `bash scripts/verify-2603.sh r3 > docs/evidence/2603/R3-residual-scan.txt` | `docs/evidence/2603/R3-residual-scan.txt` | exit 0 且含 `RESIDUAL_BIND_RAW=0` `RESIDUAL_INVALID_PREFIX=0` `BINDMSG_CALL_SITES=28` `BINDMSG_HANDLER_FILES=13` `OUT_OF_SCOPE_INVALID_REQUEST=25` | PASS（5 项 token 全为期望值） | 残留原文拼接、调用点数≠28、越界改动同类 25 处 → FAIL | dev |
| R4 | `bash scripts/verify-2603.sh r4 > docs/evidence/2603/R4-build-compile.txt` | `docs/evidence/2603/R4-build-compile.txt` | exit 0 且含 `GO_BUILD_EXIT=0` `GOFMT_DIFF=0` `TEST_COMPILE_EXIT=0` | PASS（3 项 token 全为期望值） | 构建失败 / gofmt 差异 / 测试包编译失败 → FAIL | dev |
| R5 | `bash scripts/verify-2603.sh r5 > docs/evidence/2603/R5-api-doc.txt` | `docs/evidence/2603/R5-api-doc.txt` | exit 0 且含 `API_BIND_RULE=1` `API_FORBID_RAW=1` `API_BINDMSG_REF=1` | PASS（3 项 token 全为期望值） | 文档缺口径 / 未禁原文 / 未引用唯一出口 → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2603.sh`（`r1`–`r5`）；每个文本 artifact 首行：`# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，随后 `GENERATED_AT_HEAD=<40 位 sha>`；证据必须在 fix commit **之后**生成（`FIX_REF` 可覆盖锚点，默认 `git log --grep="^fix(issue-2603)"`）。
- 重命令（`go test` / `go build` / `gofmt`）一律经 `~/.config/opencode/scripts/heavy-run.sh` 取锁；handler 测试用一次性 PostgreSQL（`127.0.0.1:15433`，`#2341` 口径，库 `tuneloop_test`），**不**使用 `jobmaster-postgres`。
- 逐条清单（全部位于 `docs/evidence/2603/`）：
  - `docs/evidence/2603/R1-bindmsg-unit.txt`
  - `docs/evidence/2603/R2-handler-messages.txt`
  - `docs/evidence/2603/R3-residual-scan.txt`
  - `docs/evidence/2603/R4-build-compile.txt`
  - `docs/evidence/2603/R5-api-doc.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2603 --summary-file tmp/pre-ready-2603-summary.md --changed-files tmp/pre-ready-2603-changed.txt --evidence-file docs/evidence/2603/evidence.json --report docs/evidence/2603/pre-ready-2603.json`
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Input Truth & Tool Preflight

- **Source of truth**：
  - 28 处清单 = `grep -rn 'invalid parameters: " + err.Error()' backend/handlers/*.go`（修前 28，修后 0）；
  - 文案模板与字段称呼 = `backend/bindmsg/bindmsg.go`（唯一 owner，本单补齐）；
  - 前端层序 = `frontend-pc/src/services/errorMessages.js` 的 `resolveErrorCore`（L2 含 CJK 透传，先于 L1/L3）；
  - 端点 `code` 基线 = 逐点实测（`instrument.go:870` 为 40001，其余 40002），**不得**顺手统一。
- **实施前差异核对（先跑，结果入 Work Summary）**：
  - `grep -rn 'invalid parameters: " + err.Error()' backend/handlers/*.go | wc -l` → 修前 **28**；修后 **0**；
  - `grep -rho 'bindmsg.Message(err)' backend/handlers/*.go | wc -l` → 修前 **2**（site.go 种子）；修后 **28**；
  - `grep -rn '"invalid request: " + err.Error()' backend/handlers/*.go | wc -l` → 修前/修后均为 **25**（越界即 FAIL）；
  - `grep -n 'dive' backend/handlers/{property,warehouse,wechatpay_prepay,renewal,mall_image,inventory,iam_proxy,banner,appeal,payment_calculate,instrument,coupon_pricing}.go | wc -l` → **0**（故不做嵌套特化）。
- **损坏输入拒绝规则**（测试必须覆盖，且都必须是中文）：
  - 控制字符 / 截断 / 多余逗号 → `请求体格式不正确，请检查后重试`；
  - 字段类型不符（数字填字符串、数组填字符串）→ `参数不合法：<称呼>类型不正确`；
  - 空 body（`EOF`）→ `参数不合法，请检查后重试`；
  - 未知字段（未登记称呼）→ 回显 JSON snake 名，**绝不**回显 Go 字段名；
  - 未登记称呼**不得**出现在 26 端点响应（R2 以「无 ASCII 字母」硬断言）。
- **工具盲区**：
  - handler 契约测试只覆盖 handler 层（不经 nginx、不经 IAM）；真实浏览器/真机表现归部署侧复验；
  - `TestBindErrorMessages2603` 依赖一次性 PG（`tuneloop_test`）；不可达时必须 `exit 2`/FAIL，**禁止** `TUNELOOP_TEST_ALLOW_SKIP=1` 静默跳过；
  - 全量 `go test ./...` 不在合同行（P5 baseline 差分承担）。
- **契约影响**：`docs/spec/api/api.md` §1.3 新增绑定失败口径 → Work Summary 必须含 `### 契约三向核对`（pre-ready P7）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 唯一出口 = `backend/bindmsg.Message(err)`（leaf 包，可脱离 gin 单测）。Issue 正文写的 `backend/handlers/bind_error.go` 是实现建议，**以本表为准**（handlers 包已 200+ 文件，且 leaf 包避免 import 环） | 本计划锁定 |
| D2 | 9 条文案模板逐字锁定（见「文案模板」表），任何一条改动需重开分析 | 本计划锁定 |
| D3 | 字段称呼表覆盖 26 端点全部字段（现有 27 + 新增 61 = 88 条），称呼**不得含 ASCII 字母**（`instrument_sn` → `乐器序列号`） | 本计划锁定 |
| D4 | **不做**嵌套 `items[0].id` 特化：26 端点无 `dive` 标签，该分支不可达（避免不可达代码） | 本计划锁定 |
| D5 | 只替换 `message` 表达式，**不改** HTTP 状态 / code / 分支顺序；`instrument.go:870` 保持 `40001` | 本计划锁定 |
| D6 | 前端（PC/H5/weapp）**零改动**：L2「含 CJK 透传」已实现；`ERROR_MESSAGE_MAP['invalid parameters']` 键保留（`user.go:37` 仍用字面量） | 本计划锁定 |
| D7 | `"invalid request: " + err.Error()` 25 处 / 17 文件 → **#2604**（已建，依赖本单）；本单只断言其计数不变 | #2604 |
| D8 | 种子改动（`backend/bindmsg/*` + `site.go` 2 处）当前在主工作树未提交；`/work` 必须在 `issue-2603` worktree 分支上重新落盘并提交，**禁止**在 `develop` 提交 | 本计划锁定 |
| D9 | 契约测试 26 端点 × 2 用例 = 52 例；缺失用例端点按「类型不匹配」补齐（无必填字段的 3 个更新端点） | 本计划锁定 |
| D10 | 重命令经 `heavy-run.sh`；handler 测试用一次性 PG（`127.0.0.1:15433`，库 `tuneloop_test`），不碰 `jobmaster-postgres` 的 dev 库 | 本计划锁定 |
| D11 | PC 渲染/真机复验（表单内常驻提示是否真出现）归部署侧，closing 时建 `[Ops-Followup of #2603]`（`waitForConfirm`，无 `status:*`） | 本计划锁定 |
| D12 | 全量 `go test ./...` 不作合同行（P5 baseline 承担）；本单合同只跑定向包 | 本计划锁定 |

### Out of Scope

- `"invalid request: " + err.Error()` 25 处 / 17 文件（`auth`/`billing`/`guarantor`/`permission_manage`/`role_manage`/`user_address` 等）→ **#2604**（已在本次 analyze 创建）。
- `backend/handlers/user.go:37` 的字面量 `"invalid parameters"`（无 `err.Error()` 拼接，走前端 message_map）→ 保持。
- 非绑定类的英文 message（如 `instrument id is required` / `mall image id is required` / `already first`）→ #2604 及后续收敛，本单不动。
- 5xx 中文内部诊断透传（#2497 已收口）与 `ERROR_CODE_MAP` 增补 → 不在本单。
- 任何前端改动（PC / H5 / weapp），含 mobile 的 `ERROR_MESSAGE_MAP` 键调整。
- 400 族其它语义分支（256 调用点）——本单只换 message 表达式。
- 嵌套 `dive` 校验引入（会让 `items[].id` 真正被校验）——属行为变更，另立 Issue。
- 全量 `go test ./...` 作为合同行（由 pre-ready P5 baseline 差分承担）。

### Dependencies

- 无

### Workload

- Size: L
- Basis: 16 个实现/测试文件（1 leaf 包 + 14 handler + 1 新测试）+ 1 个证据 harness + 1 份核心文档 + 5 条证据行（R1–R5）；无 migration、无新端点、无跨仓依赖
- Confidence: high

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状（symptom） | 网点新建请求体损坏 → `40002` + `invalid parameters: invalid character '\x17' in string`；用户只看到通用「信息不完整，请补充必填项」 | #2603 正文、预生产实测 |
| 2 | 触发条件（trigger） | 任一请求体绑定失败：缺必填 / 类型不符 / JSON 语法损坏（控制字符、截断） | 26 处 `ShouldBindJSON` 分支 |
| 3 | 根因（root cause） | handler 直接把 `err.Error()`（Go `encoding/json` / validator 措辞）拼进 `message`；前端 L2 只透传 CJK，英文技术串被判「无具体原因」→ 降级通用文案 | 28 处字面量、`errorMessages.js:97-134` |
| 4 | 测试盲点（test blind spot） | 全仓无任何测试断言这 28 处的 message；「构建通过 + 状态码 400」被当作验证充分 | `grep -rn 'invalid parameters' --include=*_test.go backend` → 仅注释 1 处 |
| 5 | 同类扫描（same-class paths） | 字面量不同的同族缺陷 25 处 / 17 文件（`invalid request: `）→ #2604；本单断言其计数不变 | `grep -c '"invalid request: " + err.Error()'` |
| 6 | 修复层（fix layer） | 提示层单点收口：`bindmsg.Message`（翻译）+ 字段称呼表（点名）+ 26 处调用点替换；不碰业务逻辑/状态码 | 变更点 1–2 |
| 7 | 红绿实证（red-green） | 红：仅提交测试（无替换）时 52 例全 FAIL（message 含 `invalid parameters`）；绿：替换后 52 例 PASS，R2 token 全绿 | 变更点 6、R2 |
| 8 | 防复发（prevention） | ① 收口扫描 `RESIDUAL_BIND_RAW=0`；② 每例「message 无 ASCII 字母」硬断言；③ 称呼表 ASCII 泄漏断言；④ 文档写明唯一出口 | R1/R2/R3/R5 |
| 9 | 影响面（blast radius） | 仅失败分支的 `message` 文本；成功路径、状态码、鉴权、DB 写入不变；同类 25 处计数不变 | R2 的 code 断言、R3 |
| 10 | 未修边界（deferred） | `invalid request:` 25 处 → #2604；非绑定类英文 message → #2604 及后续；PC 表单内提示真机复验 → `[Ops-Followup of #2603]` | Out of Scope、D7、D11 |

*Model: deepseek/deepseek-flash*

