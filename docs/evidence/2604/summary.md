## Work Summary — #2604 绑定失败文案收口第二批（25 处 / 17 文件 → `bindmsg`）

### Remote

- 分支 `issue-2604`（基线 `083a32c9c` = `origin/develop`）；fix `d9a6f2156` + 证据提交，均 push 到 `origin/issue-2604`。
- `git status --porcelain` 空（证据生成时 `dirty=false`）。

### 实施前差异核对（计划 Input Truth）

| 核对项 | 期望（计划） | 实测 |
|---|---|---|
| `grep -rn '"invalid request: " + err.Error()' backend/handlers/*.go \| wc -l` | 修前 25 → 修后 0 | 25 → **0** |
| 同 grep 的文件数 | 17 | **17** |
| `grep -rho 'bindmsg.Message' backend/handlers/*.go \| wc -l` | 修前 28 → 修后 53 | 28 → **53**（增量 25） |
| `grep -rn '"invalid parameters: " + err.Error()'` | 0（#2603 继承） | **0** |
| grep -n 'dive' backend/handlers/instrument.go | 1（`IDs`） | **1** |
| #2605 范围 28 处（5 类字面量） | base = head | **28 = 28**（`OUT_OF_SCOPE_UNCHANGED=1`） |

### 变更内容

1. **`backend/bindmsg/bindmsg.go`（唯一出口扩面）**
   - 新增 **56 条**字段称呼（表内合计 141 条，全部不含 ASCII 字母）；
   - 新增 `Scope`（`ScopePerson`: name→姓名 / company→工作单位 / title→职务；`ScopeRole`: name→角色名称 / code→角色编码），`Message(err, scopes ...Scope)` 变参**向后兼容**，默认作用域取值不变（`code`→优惠码、`name`→名称）；
   - `camelToSnake` 修正**复数缩写**：`IDs`→`ids`、`UserIDs`→`user_ids`（此前 `IDs` 会输出 `i_ds`，把 ASCII 漏给用户），`TransitSiteID`/`ManagerPhone` 保持 #2603 期望；
   - `FieldLabel` 先剥离 `dive` 下标（`IDs[0]`→`IDs`→`乐器列表`）——#2603 D4 的受控例外（本单 instrument.go 确实可达）。
2. **25 处 `message` 表达式替换**（17 文件，按域 5 批）：只换文案，HTTP 状态与 code 保持现状混杂（40000 ×4 / 40001 ×5 / 40002 ×16），分支顺序不动。作用域仅在 8 个端点显式传入（person 6 / role 2）。
3. **测试**：`backend/bindmsg/bindmsg_2604_test.go`（56 称呼 / 71 字段覆盖 / 2 作用域 / 复数缩写 / dive 下标）、`backend/handlers/bind_error_2604_test.go`（25 端点 × 2 用例 = 50 例）。
4. **证据生成器** `scripts/verify-2604.sh`（r0–r4，重命令经 heavy-run.sh 取锁；一次性 PG 15433）。

### 红绿实证（同一 harness，只回退 17 个 handler 文件）

```text
缺陷版（git stash push -- <17 handler 文件>，保留 bindmsg 与测试）：
  auth.PostLogin/syntax: message = "invalid request: invalid character '\x17' in string", want "请求体格式不正确，请检查后重试"
  auth.PostLogin/field:  message = "invalid request: Key: 'Password' Error:Field validation for 'Password' failed on the 'required' tag", want "参数不合法：密码不能为空"
  registration.Create/field: message = "invalid request: json: cannot unmarshal string into Go struct field .id_photos of type map[string]string", want "参数不合法：证件照类型不正确"
  --- FAIL: TestBindErrorMessages2604
修复版 d9a6f2156：
  BIND2604_CASES_TOTAL=50 / BIND2604_CASES_FAIL=0 / BIND2604_ASCII_LEAK=0 / BIND2604_RAW_PHRASE_LEAK=0
  --- PASS: TestBindErrorMessages2604
```

持久化红样：`docs/evidence/2604/R0-red-baseline.txt`（base 提交静态计数 `RED_BASE_SITES=25`、`RED_BASE_FILES=17`、`RED_BASE_ASCII_MESSAGES=25`、修后 `RED_HEAD_SITES=0`）。

### Accepted Plan 对照（n/n）

| 计划项 | 状态 |
|---|---|
| 变更点 1 基础层：56 条称呼 + Scope + 复数缩写 + dive 下标 | ✅ |
| 变更点 2 25 处替换（B1–B5 五批，grep 收口） | ✅ |
| 变更点 3 25 处落点断言（code/Scope/文案逐点） | ✅（R2 50 例） |
| 变更点 4 字段称呼新增表 56 条逐字锁定 | ✅（R1） |
| 变更点 5 作用域称呼表 5 条 | ✅（R1 `SCOPE_*`） |
| 变更点 6 测试（bindmsg 单元 + 50 例契约） | ✅ |
| 变更点 7 证据生成器 r0–r4 | ✅ |
| 既有断言影响（0 翻转） | ✅ 未翻转任何既有断言（#2603 单测与 52 例契约测试未改） |

### 矩阵 5/5

| 行 | 状态 | 证据 |
|---|---|---|
| R0 修前红样基线（25 处 / 17 文件外泄原文） | PASS | `docs/evidence/2604/R0-red-baseline.txt`：RED_BASE_SITES=25、RED_BASE_FILES=17、RED_BASE_ASCII_MESSAGES=25、RED_HEAD_SITES=0 |
| R1 bindmsg 扩面（56 条称呼 + 2 作用域 + 复数缩写/下标归一） | PASS | `docs/evidence/2604/R1-bindmsg-unit.txt`：BINDMSG_UNIT_EXIT=0、LABEL_NEW_2604=56、LABEL_ASCII_LEAK=0、LABEL_COVERAGE_MISSING=0、SCOPE_PERSON_NAME=姓名不能为空、SCOPE_ROLE_CODE=角色编码不能为空、SCOPE_DEFAULT_CODE=优惠码不能为空、CAMEL_IDS=ids、CAMEL_USER_IDS=user_ids、DIVE_INDEX_LABEL=乐器列表 |
| R2 25 端点绑定失败文案（50 例契约测试） | PASS | `docs/evidence/2604/R2-handler-messages.txt`：GO_TEST_EXIT=0、BIND2604_CASES_TOTAL=50、BIND2604_CASES_FAIL=0、BIND2604_ASCII_LEAK=0、BIND2604_RAW_PHRASE_LEAK=0、BIND2604_ROLE_CODE_MSG=参数不合法：角色编码不能为空、BIND2604_PERSON_NAME_MSG=参数不合法：姓名不能为空、BIND2604_IDS_MSG=参数不合法：乐器列表不能为空、BIND2604_SYNTAX_MSG=请求体格式不正确，请检查后重试 |
| R3 25 处收口 / 同类不外溢（静态扫描） | PASS | `docs/evidence/2604/R3-residual-scan.txt`：RESIDUAL_INVALID_REQUEST_RAW=0、RESIDUAL_INVALID_PARAMETERS_RAW=0、SITES_REPLACED_2604=25、HANDLER_FILES_2604=17、BINDMSG_DELTA_2604=25、OUT_OF_SCOPE_UNCHANGED=1 |
| R4 构建 / gofmt / 测试包编译 | PASS | `docs/evidence/2604/R4-build-compile.txt`：GO_BUILD_EXIT=0、GOFMT_DIFF=0、TEST_COMPILE_EXIT=0 |

未完成行：无。

### Evidence Contract

机读合同：`docs/evidence/2604/evidence.json`（build=1.0.d9a6f2156、commit=d9a6f2156、dirty=false，5 行各一条记录且 pass=true）。

| 行 | 结果 | 产物 | 关键断言 |
|---|---|---|---|
| R0 | PASS | `docs/evidence/2604/R0-red-baseline.txt` | RED_BASE_SITES=25 / RED_BASE_FILES=17 / RED_BASE_ASCII_MESSAGES=25 / RED_HEAD_SITES=0 |
| R1 | PASS | `docs/evidence/2604/R1-bindmsg-unit.txt` | 10 项 token（56 称呼 / 无 ASCII / 覆盖 71 字段 / 2 作用域 / 复数缩写 / dive 下标） |
| R2 | PASS | `docs/evidence/2604/R2-handler-messages.txt` | 9 项 token（50 例 / 0 失败 / 无 ASCII / 无原文 / 4 条抽检文案） |
| R3 | PASS | `docs/evidence/2604/R3-residual-scan.txt` | 6 项 token（残留 0 / 25 处 / 17 文件 / delta 25 / 越界未变） |
| R4 | PASS | `docs/evidence/2604/R4-build-compile.txt` | 3 项 token（build / gofmt / 编译冒烟） |

### 契约三向核对

本单**不改 API 契约**（计划 D7）：请求体失败的错误结构与 code 集合逐点不变，只把 `message` 从 Go/JSON 原文换为中文点名文案；`docs/spec/api/api.md` §1.3（#2603 确立的绑定失败口径）已覆盖本类，无文档变更。

| 维度 | 现状（保持） | 实现 | 前端 |
|---|---|---|---|
| 路径/方法 | 25 个既有端点 | 未改路由 | 未改调用 |
| code | 40000 ×4 / 40001 ×5 / 40002 ×16 | 逐点实测保持（R2 每例断言 code） | `ERROR_CODE_MAP` 键不变 |
| message | 中文点名 / `请求体格式不正确，请检查后重试` | `bindmsg.Message(err[, scope])` | L2 含 CJK 透传，`ERROR_MESSAGE_MAP['invalid request']` 键保留 |

### 同类路径对称扫描

- `grep -rn '"invalid request: " + err.Error()' backend/handlers/*.go` → **0**（本单范围全收口）。
- `grep -rn '"invalid parameters: " + err.Error()' backend/handlers/*.go` → **0**（#2603 保持）。
- 同族不同字面量 **28 处 / 10 文件**（`invalid request body: ` 6 / `Invalid request data: ` 3 / `Invalid request: ` 4 / `Invalid input: ` 9 / `invalid * format: ` 6）→ **#2605**，本单只断言计数不变（`OUT_OF_SCOPE_UNCHANGED=1`），未顺手改。
- 未登记字段仍回落 JSON snake 名（绝不回显 Go 字段名）；`dive` 下标归一后 `IDs[0]` 与 `IDs` 同称呼。

### 人工/部署侧（不进本矩阵）

| 事项 | 负责方 | 回执 |
|---|---|---|
| PC/H5/weapp 真机复验（用户是否真看到点名字段） | Owner | 关单时按 `[Ops-Followup of #2604]`（`waitForConfirm`，无 `status:*`）落盘 |

### Pre-ready gate

- 命令：`~/.config/opencode/scripts/pre-ready-check.sh 2604 --summary-file docs/evidence/2604/summary.md --changed-files docs/evidence/2604/changed-files.txt --evidence-file docs/evidence/2604/evidence.json --affected-cmd "bash scripts/verify-2604.sh r2" --report docs/evidence/2604/pre-ready-2604.json`
- 结果：见下方 gate 报告行（`RESULT: PASS` 才置 ready）。
- 报告：docs/evidence/2604/pre-ready-2604.json

*Model: deepseek/deepseek-flash*
