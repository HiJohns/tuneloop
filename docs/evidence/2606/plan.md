## Accepted Plan — #2606 [Bug] instrument.go 400 族收口（2 处裸 `err.Error()` 绑定文案 + 6 处不可达 Marshal 分支改中文常量）

### 背景与现状核对（代码为据）

| # | 事实 | 位置 / 命令 |
|---|---|---|
| 1 | A 类 **2 处**裸 `err.Error()` 绑定文案（可复现）：`CreateInstrument` / `UpdateInstrument` | `grep -c '"message": err.Error()' backend/handlers/instrument.go` → **2**（`:290`、`:552`） |
| 2 | B 类 **6 处** `json.Marshal` 失败分支拼接原文 | `grep -c '"invalid \(images\|specifications\|pricing\) format: "' backend/handlers/instrument.go` → **6**（`:404/:420/:444/:654/:666/:678`） |
| 3 | **B 类不可达**：三字段类型均为 JSON 解码产物（`Images []string`、`Specifications []map[string]interface{}`、`Pricing map[string]interface{}`），`encoding/json` 仅对 channel/func/complex/Inf·NaN/循环引用/非法 map key 失败，`ShouldBindJSON` 不可能产生这些值 | `backend/handlers/instrument.go:60,67,70,85,86,92` |
| 4 | code 现状（必须保持）：40001 ×2（A 类）、40003 ×3（含 `:893` 一处非本单分支）、40004 ×2、40005 ×2 | `grep -n '"code": *4000[345]' backend/handlers/instrument.go` |
| 5 | 唯一出口 `bindmsg` 由 #2603 落地、#2604/#2605 扩面；本单追加 10 条乐器字段称呼 | `backend/bindmsg/bindmsg.go`（依赖 #2604） |
| 6 | A 类 2 端点请求体共 18 个 JSON 字段名，**10 条需新增**（余 8 条已登记：site_id/status/base_daily_rate/deposit/deposit_reduced_enabled/deposit_reduced/description/images） | 见「字段称呼新增表」 |
| 7 | 既有测试无断言这两处 message | `grep -rn 'invalid images format\|Field validation' --include=*_test.go backend` → 0 |
| 8 | 前端零改动：PC / mobile `resolveErrorCore` L2 = 含 CJK 透传（先于 L3 code 家族） | `frontend-pc/src/services/errorMessages.js:118-140`、`frontend-mobile/src/services/errorMessageResolution.mjs` |

**结论**：乐器域是 #2603–#2605 之后最后一处 400 族原文外泄文件。A 类走唯一出口（4 例运行时断言），B 类改成固定中文常量（静态断言 + 不可达见证测试），code / HTTP 全不变。

### 变更点

#### 1. 基础层：`backend/bindmsg/bindmsg.go`（唯一 owner，追加 10 条称呼）

新增：`category_id`→分类、`level_id`→等级、`sn`→序列号、`pricing`→价格配置、`total_price`→总价、`video`→视频、`poster`→海报、`specifications`→规格清单、`properties`→属性值、`level`→级别。

称呼**不含 ASCII 字母**；与既有条目**不得重复**（Go map 重复键即编译失败，作为兜底）。本单不新增 `Scope`（无同名跨域冲突：`pricing`/`sn`/`level` 均为首次登记）。

#### 2. A 类替换（2 处，只换 message 表达式）

```go
// instrument.go:290 / :552
c.JSON(http.StatusBadRequest, gin.H{
	"code":    40001,
	"message": bindmsg.Message(err),
})
```

- 保留 `CreateInstrument` 原有的 `log.Printf("[DEBUG] CreateInstrument - BindJSON error: %v", err)`（日志可含原文，响应体不得含）；
- **禁止**改动 400 族判定分支、HTTP 状态、code。

#### 3. B 类替换（6 处，固定中文常量）

| 位置 | code | 新 message（逐字） |
|---|---|---|
| `instrument.go:404`、`instrument.go:654` | 40003 | `参数不合法：图片格式不正确` |
| `instrument.go:420`、`instrument.go:666` | 40004 | `参数不合法：规格格式不正确` |
| `instrument.go:444`、`instrument.go:678` | 40005 | `参数不合法：价格格式不正确` |

- 保留各自 `log.Printf("[ERROR] Failed to marshal …")`；
- 不新增 helper（6 处常量直写；不引入 `Scope` 或新函数，避免为不可达分支扩面）；
- **禁止**删除分支本身（保留防御性 `if err != nil`，仅替换文案）。

#### 4. 测试

- `backend/bindmsg/bindmsg_2606_test.go`（新增）：`TestBindMsgLabels2606` —— 10 条新增称呼逐条等值、`LABEL_ASCII_LEAK=0`、`LABEL_COVERAGE_MISSING=0`（18 字段全覆盖）。
- `backend/handlers/instrument_400_messages_2606_test.go`（新增）：
  - `TestBindErrorMessages2606`（A 类 4 例，表驱动）：
    1. `CreateInstrument` + 损坏请求体（含 `0x17`）→ 400 + 40001 + `请求体格式不正确，请检查后重试`；
    2. `CreateInstrument` + `{}` → 400 + 40001 + `参数不合法：分类不能为空`；
    3. `UpdateInstrument` + 损坏请求体 → 400 + 40001 + `请求体格式不正确，请检查后重试`（seed `Instrument{ID, TenantID:tid}`，路由 `PUT /api/instruments/:id`）；
    4. `UpdateInstrument` + `{"status":123}` → 400 + 40001 + `参数不合法：状态类型不正确`；
    每例断言：无 ASCII 字母、不含 `Key:` / `Field validation` / `invalid character` / `cannot unmarshal` / `json:`、与期望**逐字相等**；
  - `TestInstrumentMarshalUnreachable2606`（B 类**不可达见证**，不触发 handler）：
    - 语料：嵌套对象/数组/unicode/控制字符/大整数/`null`/深层嵌套/数字字符串（≥10 例）先 `json.Unmarshal` 再 `json.Marshal`，断言 `MARSHAL_ROUNDTRIP_FAIL=0`；
    - 负例：`json.Marshal(func(){})` 与 `json.Marshal(math.Inf(1))` **必须**失败 → `MARSHAL_NONJSON_FAILS=2`（证明该分支只可能被非 JSON 值触发，而 `ShouldBindJSON` 不产生此类值）。
- **红绿实证**：先只落 bindmsg 扩面 + 测试、**不做 8 处替换** → A 类 4 例 FAIL（message 为裸 `Key: …` 或含 `invalid images format`）；R1 静态计数为 2/6。再落替换 → 全绿。

#### 5. 证据生成器 `scripts/verify-2606.sh`（`r1`–`r4`，沿用 `scripts/verify-2581.sh` 结构）

- 每个文本 artifact 首行 provenance：`# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，随后 `GENERATED_AT_HEAD=<sha>`；
- 重命令经 `~/.config/opencode/scripts/heavy-run.sh` 取锁；`r1` 的 base 计数用 `git merge-base origin/develop HEAD`（确定性）。

### 既有断言影响（0 翻转）

| 检查 | 结果 | 依据 |
|---|---|---|
| Go 测试断言这两处绑定文案 | **0 处** | `grep -rn 'invalid images format\|Field validation' --include=*_test.go backend` → 0 |
| 既有 `TestCreateInstrument*` / `TestInstrumentCRUD` | **不改动** | 仅失败分支文案变化，成功路径不变（R4 定向复跑） |
| #2603–#2605 的 `bindmsg` 断言 | **0 翻转** | 仅追加 10 条表项，不改模板/作用域 |
| 需要翻转的语义断言 | **无** | 只改 `message` 文本，不改 code / HTTP / 分支 |

### Files（变更文件清单）

- `backend/bindmsg/bindmsg.go`
- `backend/bindmsg/bindmsg_2606_test.go`
- `backend/handlers/instrument.go`
- `backend/handlers/instrument_400_messages_2606_test.go`
- `scripts/verify-2606.sh`
- `docs/evidence/2606/plan.md`
- `docs/evidence/2606/R1-static-scan.txt`
- `docs/evidence/2606/R2-handler-messages.txt`
- `docs/evidence/2606/R3-marshal-unreachable.txt`
- `docs/evidence/2606/R4-build-compile.txt`
- `docs/evidence/2606/evidence.json`
- `docs/evidence/2606/pre-ready-2606.json`

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 8 处收口静态扫描（A 类裸文案、B 类 Marshal 分支） | `backend/handlers/instrument.go`、`scripts/verify-2606.sh` | 静态扫描 + base 提交对照 | 修后工作树 vs merge-base | repo（静态） | `BARE_BIND_LEAK_BASE=2`、`BARE_BIND_LEAK_SITES=0`、`MARSHAL_LEAK_BASE=6`、`MARSHAL_LEAK_SITES=0`、`IMAGES_INVALID_MSG_COUNT=2`、`SPECS_INVALID_MSG_COUNT=2`、`PRICING_INVALID_MSG_COUNT=2`、`CODE_40003_COUNT=3`、`CODE_40004_COUNT=2`、`CODE_40005_COUNT=2` | R1 |
| R2 A 类 2 端点绑定失败文案（4 例契约测试） | `backend/handlers/instrument_400_messages_2606_test.go` | `POST /api/instruments`、`PUT /api/instruments/:id`（handler 直调） | 损坏请求体 / 缺必填 / 类型不匹配，共 4 例 | backend（一次性 PG 15433） | `BIND2606_CASES_TOTAL=4`、`BIND2606_CASES_FAIL=0`、`BIND2606_ASCII_LEAK=0`、`BIND2606_RAW_PHRASE_LEAK=0`、`BIND2606_CATEGORY_MSG=参数不合法：分类不能为空`、`BIND2606_STATUS_TYPE_MSG=参数不合法：状态类型不正确` | R2 |
| R3 B 类不可达见证（JSON 解码产物 Marshal 不失败） | `backend/handlers/instrument_400_messages_2606_test.go` | `go test ./handlers -run '^TestInstrumentMarshalUnreachable2606$'` | ≥10 例 JSON 语料往返 + 2 例非 JSON 负例 | backend（纯函数，无 DB） | `MARSHAL_ROUNDTRIP_FAIL=0`、`MARSHAL_CORPUS_CASES=10`、`MARSHAL_NONJSON_FAILS=2` | R3 |
| R4 构建 / gofmt / 测试包编译 / 乐器定向测试 | `scripts/verify-2606.sh` | `cd backend && go build ./...`、`go test ./bindmsg`、`go test ./handlers -run 'TestCreateInstrument\|TestInstrumentCRUD'`（经 heavy-run） | 修后工作树 | backend | `GO_BUILD_EXIT=0`、`GOFMT_DIFF=0`、`TEST_COMPILE_EXIT=0`、`BINDMSG_UNIT_EXIT=0`、`INSTRUMENT_TESTS_EXIT=0` | R4 |

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `git merge-base origin/develop HEAD` 指向的 base 提交 + 修后 HEAD | `bash scripts/verify-2606.sh r1` | `docs/evidence/2606/R1-static-scan.txt` | `BARE_BIND_LEAK_BASE=2`、`BARE_BIND_LEAK_SITES=0`、`MARSHAL_LEAK_BASE=6`、`MARSHAL_LEAK_SITES=0`、`IMAGES_INVALID_MSG_COUNT=2`、`SPECS_INVALID_MSG_COUNT=2`、`PRICING_INVALID_MSG_COUNT=2`、`CODE_40003_COUNT=3`、`CODE_40004_COUNT=2`、`CODE_40005_COUNT=2`、`MSG_ASCII_LEAK=0` | base 计数 ≠2/6、任一目标字面量残留、code 计数漂移、中文常量含 ASCII → 任一即 FAIL |
| R2 | `testfixtures.SetupTestDB` + seed `models.Instrument{ID, TenantID}`（Update 用例）+ 4 请求体 | `go test ./handlers -run '^TestBindErrorMessages2606$' -count=1 -v` | `docs/evidence/2606/R2-handler-messages.txt` | `BIND2606_CASES_TOTAL=4`、`BIND2606_CASES_FAIL=0`、`BIND2606_ASCII_LEAK=0`、`BIND2606_RAW_PHRASE_LEAK=0`、`BIND2606_CATEGORY_MSG=参数不合法：分类不能为空`、`BIND2606_STATUS_TYPE_MSG=参数不合法：状态类型不正确`、`BIND2606_SYNTAX_MSG=请求体格式不正确，请检查后重试` | 返回非 400 / code 漂移 / message 含 `Key:`·`Field validation`·`invalid character`·ASCII 字母 / 用例数 <4 → 任一即 FAIL |
| R3 | ≥10 例 JSON 解码语料（嵌套对象、数组、unicode、控制字符、大整数、null、深层嵌套、数字字符串）+ 2 例非 JSON 值（func、Inf） | `go test ./handlers -run '^TestInstrumentMarshalUnreachable2606$' -count=1 -v` | `docs/evidence/2606/R3-marshal-unreachable.txt` | `MARSHAL_ROUNDTRIP_FAIL=0`、`MARSHAL_CORPUS_CASES=10`、`MARSHAL_NONJSON_FAILS=2` | 任一 JSON 语料 Marshal 失败（说明 B 类可达 → 计划假设失效，回 Architect）或非 JSON 负例未失败（见证无效）→ FAIL |
| R4 | 修后工作树 | `bash scripts/verify-2606.sh r4` | `docs/evidence/2606/R4-build-compile.txt` | `GO_BUILD_EXIT=0`、`GOFMT_DIFF=0`、`TEST_COMPILE_EXIT=0`、`BINDMSG_UNIT_EXIT=0`、`INSTRUMENT_TESTS_EXIT=0` | 构建失败 / gofmt 差异 / 编译失败 / 既有乐器测试回归 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2606.sh r1 > docs/evidence/2606/R1-static-scan.txt` | `docs/evidence/2606/R1-static-scan.txt` | exit 0 且含 `BARE_BIND_LEAK_BASE=2` `BARE_BIND_LEAK_SITES=0` `MARSHAL_LEAK_BASE=6` `MARSHAL_LEAK_SITES=0` `IMAGES_INVALID_MSG_COUNT=2` `SPECS_INVALID_MSG_COUNT=2` `PRICING_INVALID_MSG_COUNT=2` `CODE_40003_COUNT=3` `CODE_40004_COUNT=2` `CODE_40005_COUNT=2` `MSG_ASCII_LEAK=0` | PASS（11 项 token 全为期望值） | 目标字面量残留、code 漂移、常量含 ASCII → FAIL | dev |
| R2 | `bash scripts/verify-2606.sh r2 > docs/evidence/2606/R2-handler-messages.txt` | `docs/evidence/2606/R2-handler-messages.txt` | exit 0 且含 `GO_TEST_EXIT=0` `BIND2606_CASES_TOTAL=4` `BIND2606_CASES_FAIL=0` `BIND2606_ASCII_LEAK=0` `BIND2606_RAW_PHRASE_LEAK=0` `BIND2606_CATEGORY_MSG=参数不合法：分类不能为空` `BIND2606_STATUS_TYPE_MSG=参数不合法：状态类型不正确` `BIND2606_SYNTAX_MSG=请求体格式不正确，请检查后重试` | PASS（8 项 token 全为期望值） | 响应含 Go/JSON 原文或 ASCII、HTTP/code 漂移、用例数 <4 → FAIL | dev |
| R3 | `bash scripts/verify-2606.sh r3 > docs/evidence/2606/R3-marshal-unreachable.txt` | `docs/evidence/2606/R3-marshal-unreachable.txt` | exit 0 且含 `GO_TEST_EXIT=0` `MARSHAL_ROUNDTRIP_FAIL=0` `MARSHAL_CORPUS_CASES=10` `MARSHAL_NONJSON_FAILS=2` | PASS（4 项 token 全为期望值） | JSON 语料往返失败（B 类可达）或负例未失败 → FAIL | dev |
| R4 | `bash scripts/verify-2606.sh r4 > docs/evidence/2606/R4-build-compile.txt` | `docs/evidence/2606/R4-build-compile.txt` | exit 0 且含 `GO_BUILD_EXIT=0` `GOFMT_DIFF=0` `TEST_COMPILE_EXIT=0` `BINDMSG_UNIT_EXIT=0` `INSTRUMENT_TESTS_EXIT=0` | PASS（5 项 token 全为期望值） | 构建失败 / gofmt 差异 / 编译失败 / 乐器测试回归 → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2606.sh`（`r1`–`r4`）；每个文本 artifact 首行：`# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，随后 `GENERATED_AT_HEAD=<40 位 sha>`；证据必须在 fix commit **之后**生成（`FIX_REF` 可覆盖锚点，默认 `git log --grep="^fix(issue-2606)"`）。
- 重命令（`go test` / `go build` / `gofmt`）一律经 `~/.config/opencode/scripts/heavy-run.sh` 取锁；handler 测试用一次性 PostgreSQL（`127.0.0.1:15433`，库 `tuneloop_test`，`#2341` 口径），**不**使用 `jobmaster-postgres`。
- 逐条清单（全部位于 `docs/evidence/2606/`）：
  - `docs/evidence/2606/R1-static-scan.txt`
  - `docs/evidence/2606/R2-handler-messages.txt`
  - `docs/evidence/2606/R3-marshal-unreachable.txt`
  - `docs/evidence/2606/R4-build-compile.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2606 --summary-file tmp/pre-ready-2606-summary.md --changed-files tmp/pre-ready-2606-changed.txt --evidence-file docs/evidence/2606/evidence.json --report docs/evidence/2606/pre-ready-2606.json`
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Input Truth & Tool Preflight

- **Source of truth**：
  - A 类清单 = `grep -c '"message": err.Error()' backend/handlers/instrument.go`（修前 2，修后 0）；
  - B 类清单 = `grep -c '"invalid \(images\|specifications\|pricing\) format: "'`（修前 6，修后 0）；
  - code 基线 = `grep -n '"code": *4000[345]'`（40003 ×3 / 40004 ×2 / 40005 ×2，**不得**改动）；
  - 称呼模板 = `backend/bindmsg/bindmsg.go`（唯一 owner）。
- **实施前差异核对（结果入 Work Summary）**：
  - `grep -c '"message": err.Error()' backend/handlers/instrument.go` → 修前 **2**；修后 **0**；
  - `grep -c '"invalid \(images\|specifications\|pricing\) format: "' backend/handlers/instrument.go` → 修前 **6**；修后 **0**；
  - `grep -c '参数不合法：图片格式不正确' backend/handlers/instrument.go` → **2**；`规格格式不正确` → **2**；`价格格式不正确` → **2**；
  - `grep -rho 'bindmsg.Message' backend/handlers/*.go | wc -l` → 修前 **75**（#2603 28 + #2604 25 + #2605 22）；修后 **77**（增量 2）。
- **损坏输入拒绝规则**（测试覆盖，全部中文）：控制字符/截断 → `请求体格式不正确，请检查后重试`；缺 `category_id` → `参数不合法：分类不能为空`；`status` 类型不符 → `参数不合法：状态类型不正确`。
- **工具盲区**：
  - B 类为不可达分支 → 只用静态断言 + 不可达见证测试（有界语料，非形式化证明），**不写**伪造的运行时触发；
  - handler 契约测试只覆盖 handler 层（不经 `main.go` 路由、不经 nginx、不经 IAM）；本单不改路由；
  - 前端展示链路由 #2480 层序单测覆盖，本单不重复；真机复验归部署侧（D6）；
  - 全量 `go test ./...` 不在合同行（P5 baseline 差分承担）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | A 类走 `backend/bindmsg.Message(err)`；B 类用固定中文常量，**不**新增 helper / 不引入 `Scope` | 本计划锁定 |
| D2 | 文案逐字锁定：`参数不合法：图片格式不正确` / `参数不合法：规格格式不正确` / `参数不合法：价格格式不正确`；`bindmsg` 模板沿用 #2603 | 本计划锁定 |
| D3 | 新增 10 条称呼逐字锁定；称呼不得含 ASCII 字母 | 本计划锁定 |
| D4 | 只替换 `message` 表达式，**不改** HTTP / code / 分支；40003 ×3、40004 ×2、40005 ×2 计数不变 | 本计划锁定 |
| D5 | B 类保留 `if err != nil` 防御分支与 `log.Printf`，仅替换文案（不删分支） | 本计划锁定 |
| D6 | 真机复验归部署侧；closing 时建 `[Ops-Followup of #2606]`（`waitForConfirm`，无 `status:*`） | 本计划锁定 |
| D7 | 前端零改动；`ERROR_MESSAGE_MAP` 英文键保留 | 本计划锁定 |
| D8 | 不新增 `docs/spec/api/api.md` 条款（#2603 §1.3 已覆盖绑定失败口径；常量文案属同口径）（P7 为 N/A） | 本计划锁定 |
| D9 | 依赖未满足即 `HOLD`：#2604 必须 `status:done`/`ready` | 本计划锁定 |
| D10 | 重命令经 `heavy-run.sh`；handler 测试用一次性 PG（`127.0.0.1:15433`，库 `tuneloop_test`） | 本计划锁定 |

### Out of Scope

- 其它文件的裸 `"message": err.Error()`（本单后剩余 28 处，多为 DB/内部错误）与非绑定类英文 message → 后续收敛批次。
- `instrument.go:893` 的 `40003` 分支（不含 `err.Error()`，无泄漏）→ 保持原样。
- 非 400 族响应（5xx 内部诊断由 #2497 口径覆盖）。
- `docs/spec/api/api.md` 条款变更。
- 任何前端改动（PC / H5 / weapp）。
- 删除/重构 B 类防御分支（行为变更，且无可观测收益）。
- 全量 `go test ./...` 作为合同行（由 pre-ready P5 baseline 差分承担）。

### Dependencies

- #2604

### Workload

- Size: M
- Basis: 2 个实现文件（`instrument.go` 8 处、`bindmsg.go` 10 条称呼）+ 2 个测试文件（4 例契约 + 不可达见证）+ 1 个证据 harness + 4 条证据行（R1–R4）；无 migration、无新端点、无前端改动
- Confidence: high

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状（symptom） | 乐器新建/编辑缺字段 → `{"code":40001,"message":"Key: 'CreateInstrumentRequest.CategoryID' Error:Field validation for 'CategoryID' failed on the 'required' tag"}`；用户只看到通用「请求参数有误」 | #2606 正文、instrument.go:290/552 |
| 2 | 触发条件（trigger） | A 类：任一绑定失败（缺必填 / 类型不符 / 语法损坏）；B 类：`json.Marshal` 失败（生产不可达） | 8 处逐点核对 |
| 3 | 根因（root cause） | handler 直接把 `err.Error()` 当 `message`（A 类甚至无前缀）；前端 L2 只透传 CJK → 降级 generic 文案 | 8 处字面量、`errorMessageResolution.mjs` |
| 4 | 测试盲点（test blind spot） | 全仓无测试断言这 8 处；「构建通过 + 状态码」被当成验证充分 | `grep --include=*_test.go` → 0 |
| 5 | 同类扫描（same-class paths） | #2603（28）、#2604（25）、#2605（22）已收口；本单为乐器域最后 8 处；其余裸 `err.Error()` 28 处另批 | 背景第 1–4 条、R1 计数 |
| 6 | 修复层（fix layer） | 提示层：A 类走唯一出口、B 类固定常量；不碰业务逻辑/状态码 | 变更点 2–3 |
| 7 | 红绿实证（red-green） | 红：仅落 bindmsg + 测试（无替换）→ A 类 4 例 FAIL，R1 计数 2/6；绿：替换后全绿 | 变更点 4 |
| 8 | 防复发（prevention） | ① 静态收口计数全 0；② code 计数断言防漂移；③ 称为表 ASCII 断言；④ 不可达见证测试记录「为何不写运行时断言」 | R1/R2/R3 |
| 9 | 影响面（blast radius） | 仅失败分支文案；成功路径、状态码、鉴权、DB 写入不变（R4 定向复跑） | R4 |
| 10 | 未修边界（deferred） | 其余 28 处裸 `err.Error()`、`instrument.go:893`（无泄漏）、真机复验 → `[Ops-Followup of #2606]` | Out of Scope、D6 |

*Model: deepseek/deepseek-flash*

