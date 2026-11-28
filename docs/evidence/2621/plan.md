## Accepted Plan — #2621 [Bug] 其余身份自查询仍被自动租户过滤：共享解析器 + `/user/*` 家庭 7 处收口｜Rev.3

> **Rev.3（修第二处 Files 缺口）**：变更点序 7 给 `getWalletInfo` 加 `ctx` 参数，其**另一个调用点** `backend/handlers/gift_policy_test.go:152` 必须同改（否则 `handlers` 包编译失败），本版把该测试文件登记进 `### Files`。

> **Rev.2（修 /work 计划缺口）**：`### Files` 补登证据生成器 `scripts/verify-2621.sh` —— `## Evidence Contract` 的 R1/R2/R3 三条命令全部依赖它，缺登记会导致 P6/P12 校验失败（本次由执行侧按 §计划偏差升级报出）。

### 背景与现状核对（代码为据）

| # | 事实 | 位置 / 证据 |
|---|---|---|
| 1 | 缺陷机制与 #2620 同源：`addTenantScope` 对带 `TenantID` 的模型在 **Query/Delete** 上自动注入 `WHERE tenant_id = <JWT tid>`；而 `users.tenant_id` 只是 IAM 关系的**本地缓存**，可能与 JWT `tid` 不一致 | `backend/database/db.go:96-124`；#2620 计划（同族） |
| 2 | #2620 只修了 `/users/me/*`（`wechat-bind` / `wechat-unbind` / `promo-qrcode` / `referrals`）；本单收其余同族点 | #2620；本单正文清单 |
| 3 | **共享解析器是最高杠杆点**：`EnsureLocalUser`（`backend/middleware/iam.go:561` 查询 + `:588` 建户失败后的回退查询）与 `LocalUserID`（`:611`）都用调用方传入的句柄 → 被租户过滤时：`LocalUserID` 静默返回 `("", nil)`；`EnsureLocalUser` 走「建 shadow user」→ `iam_sub` 唯一索引冲突 → 返回 `failed to create user` → 500 | `backend/middleware/iam.go:556-624`；唯一索引：`backend/database/migrations/038_add_users_iam_sub.up.sql` |
| 4 | 共享解析器有 **31 个调用点**（`grep -rn 'middleware.LocalUserID\|middleware.EnsureLocalUser' backend --include=*.go \| grep -v _test \| wc -l`）→ 修 1 处即覆盖 `user_onboarding`/`guarantor`/`invoice`/`notification`/`staff_invite`/`appeal`/`payscore`/`user_rental`/`user_settlement`/`user_address`/`member_invite`/`merchant_member` | 同命令输出 |
| 5 | 另有 **7 处直连自查询**（不走共享解析器），命中即 404/500 | `user_points.go:24,27` 与 `:63,66`；`pending_order.go:19,26`；`face_capture.go:80,84` 与 `:204,207`；`user_address.go:33`；`payment_calculate.go:113`（`getWalletInfo`，句柄来自 `:49`）；`deposit_waiver.go:123,126`；`customer_role.go:29` |
| 6 | `customer_role.go:29` 还有 **ID 空间错误**：用 `GetUserID(ctx)`（= `iam_sub`）去查 `users.id`；正确应是按 `iam_sub` 查（`users.id ≠ iam_sub` 的账号必然 404） | `backend/handlers/customer_role.go:22,29`；同族 #2546 |
| 7 | 这些 handler 里 `db` 同时承载**域数据**查询（积分批次/人脸批次/押金减免评估/礼品策略/订单），域查询**必须继续带租户过滤** → 本单只能给「身份查询」换句柄，**不得**整函数放宽 | `backend/services/*points*`、`EvaluateDepositWaiverEligibility(db, …)`、`GetGiftPolicyByLevel(db, …)`、`face_capture_batches` |
| 8 | 不受影响（**不改**）：`face_verify.go:72,123` / `user_onboarding.go:82,221,243,383,532,611` / `iam_proxy.go:621` / `merchant.go:463` / `instrument_media.go:822` 均为 `UPDATE`，GORM v1.31.1 的 Update 回调链不含 query 回调 | `~/go/pkg/mod/gorm.io/gorm@v1.31.1/callbacks/callbacks.go:65-74` |
| 9 | `property.go:73` 用 `db.Table("users")`（无 Schema）→ 自动 scope 不注入；改成 `Model(&models.User{})` 才会命中，本单仅在测试里做**回归守卫** | `backend/handlers/property.go:73` |
| 10 | 测试盲点与 #2620 相同：多数 handler 测试只注入 `middleware.ContextKeyTenantID`，未写 `database.SetTenantID`，自动 scope 不执行 → 缺陷不可见 | `backend/testutil/role_factory.go:57-73`（`TestActor` 才写 `database` 键） |

**结论**：本单 = 「1 个共享谓词 + 7 个直连点」的身份查询收口。共享解析器内部固定用 `database.IdentityCtx`，其余点**只把 `users` 行的查询**换成身份句柄，域查询保持原句柄（隔离不受影响）。

### 变更点

#### 1. 共享解析器（`backend/middleware/iam.go`，唯一谓词）

| 行 | 函数 | 改动 |
|---|---|---|
| `:561` | `EnsureLocalUser` 首次查询 | `db.Where(...)` → `db.WithContext(database.IdentityCtx(ctx)).Where("iam_sub = ?", iamSub)` |
| `:588` | `EnsureLocalUser` 建户失败后的回退查询 | 同上（否则建户冲突后回退仍失败） |
| `:611` | `LocalUserID` | 同上 |

- 注释逐点写明：「#2621 身份查询：`users.tenant_id` 是 IAM 关系缓存，可能 ≠ JWT tid，租户过滤不适用」。
- **建户语句不动**（`db.Create(&user)` 继续用原句柄，`TenantID/OrgID` 由 `GetTenantID/GetOrgID(ctx)` 显式赋值，见 `:583-592`）→ 不改变 shadow user 的归属语义。
- 31 个调用点**零改动**继承修复（禁止在调用点各写一份 fallback）。

#### 2. 7 处直连自查询（只换「查自己那行」的句柄）

统一写法（逐点机械）：

```go
// 身份查询用身份句柄；db（带租户过滤）继续用于域数据查询
var localUser models.User
if err := database.GetDB().WithContext(database.IdentityCtx(ctx)).
    Where("iam_sub = ?", userID).First(&localUser).Error; err != nil { … }
```

| 序 | 位置 | 端点 | 备注 |
|---|---|---|---|
| 1 | `backend/handlers/user_points.go:24-30` | `GET /api/user/points/balance` | 仅 users 查询换句柄；`GetUserPointsBalance` 等域查询保持 `db` |
| 2 | `backend/handlers/user_points.go:63-68` | `GET /api/user/points/transactions` | 同上 |
| 3 | `backend/handlers/pending_order.go:19-31`（`currentLocalUser`） | 待处理订单 3 个入口 | 该 helper 只查 users，可整体用身份句柄 |
| 4 | `backend/handlers/face_capture.go:80-88` | `POST /user/face-capture` | 仅 users 查询换句柄 |
| 5 | `backend/handlers/face_capture.go:204-211` | `GET /user/face-capture/status` | 同上 |
| 6 | `backend/handlers/user_address.go:33-35`（`resolveAddressUserID`） | `GET/POST/PUT/DELETE /user/addresses*` | 预检换句柄；`EnsureLocalUser` 由变更点 1 继承 |
| 7 | `backend/handlers/payment_calculate.go:113`（`getWalletInfo`） | `POST /pay/calculate` | **签名加 `ctx context.Context`**，只在 users 查询上用 `db.WithContext(database.IdentityCtx(ctx))`；`GetGiftPolicyByLevel(db, …)` 保持原句柄 |
| 8 | `backend/handlers/deposit_waiver.go:123-126` | `GET /user/deposit-waiver/eligibility` | 仅 users 查询换句柄；`EvaluateDepositWaiverEligibility(db, …)` 保持原句柄 |
| 9 | `backend/handlers/customer_role.go:29` | `POST /user/register-as-customer` | **同时修 ID 空间**：`Where("id = ?", userID)` → 身份句柄 + `Where("iam_sub = ?", userID AND deleted_at IS NULL)` |

> 序 8/9 与上表 7 处合计 9 个改动点（`user_points`/`face_capture` 各 2）。

#### 3. 回归测试（新增 1 文件 `backend/handlers/self_query_tenant_scope_2621_test.go`）

统一夹具：`testutil.TestActor{TenantID: jwtTenant, OrgID: jwtOrg, UserID: iamSub, Role: "USER"}`（**必须**走 `TestActor`，它写 `database.SetTenantID` 使自动 scope 真实生效）+ 本地 `users.tenant_id = cacheTenant ≠ jwtTenant`。

| 子用例（逐字锁定） | 入口 | 期望（修后） | 修前（红样） |
|---|---|---|---|
| `ensure_local_user` | `middleware.EnsureLocalUser(ctx, scopedDB)` | 返回本地 `users.id`，**不**新建行（`users` 行数不变） | 建 shadow user → `iam_sub` 唯一冲突 → 返回 error |
| `local_user_id` | `middleware.LocalUserID(ctx, scopedDB)` | 返回本地 id | 返回 `("", nil)`（静默） |
| `points_balance` | `UserPointsHandler.GetBalance` | 200 | 404 `40400` |
| `points_transactions` | `UserPointsHandler.ListTransactions` | 200 | 404 |
| `pending_orders` | `CreatePendingOrder`（或 `currentLocalUser` 直调） | 200 | 404 |
| `face_capture` | `SubmitFaceCapture` / `GetFaceCaptureStatus` | 非 404（业务校验另计） | 404 `40400 user not found` |
| `addresses` | `GET /user/addresses`（`ListAddresses`） | 200 | 500（建 shadow 冲突） |
| `pay_calculate` | `CalculatePayment`（钱包解析路径） | 不含「user not found」错误 | 500 |
| `deposit_waiver` | `GetDepositWaiverEligibility` | 200 且 `Eligible/Reasons` 非 `profile_not_ready` | 200 但误报 `profile_not_ready` |
| `customer_role` | `RegisterAsCustomer` | 命中用户（IAM mock 返回成功）→ 200 | 404 `用户不存在` |
| `cross_tenant_scoped`（**负例**） | 直连 GORM（带租户 ctx） | 按他人 `iam_sub` 查 `users` → `ErrRecordNotFound`；`Find` 只返回本 tenant 行 | 修前后均 PASS |

- 外部依赖离线化：IAM 用 `httptest` mock（照 `backend/handlers/update_current_user_test.go:33-58`）；微信/媒体等不属于本单路径。
- **红绿实证**：先落测试（不含实现）→ 除 `cross_tenant_scoped` 外全 FAIL；再落实现 → 11/11 PASS。红样贴 Work Summary 并写 `evidence.json.negative_result`。

#### 4. 既有断言影响（预期 0 翻转，需实测确认）

- 受影响包：`backend/middleware`、`backend/handlers`。Dev 必须跑 `go test ./middleware ./handlers -run 'EnsureLocalUser|LocalUserID|Points|Address|FaceCapture|DepositWaiver|RegisterAsCustomer|CalculatePayment' -count=1` 并把结果写进 Work Summary。
- 若有既有用例断言「tid 不匹配时自查询 404 / 静默空」→ 属**旧缺陷固化**，必须翻转并在 Work Summary 列出（`文件:行 / 旧期望 → 新期望`）。

### Files（变更文件清单）

- `backend/middleware/iam.go`
- `backend/handlers/user_points.go`
- `backend/handlers/pending_order.go`
- `backend/handlers/face_capture.go`
- `backend/handlers/user_address.go`
- `backend/handlers/payment_calculate.go`
- `backend/handlers/deposit_waiver.go`
- `backend/handlers/customer_role.go`
- `backend/handlers/self_query_tenant_scope_2621_test.go`
- `backend/handlers/gift_policy_test.go`（`getWalletInfo` 加 `ctx` 后的调用点同步）
- `docs/evidence/2621/plan.md`
- `docs/evidence/2621/R1-resolver.txt`
- `docs/evidence/2621/R2-handler-sites.txt`
- `docs/evidence/2621/R3-isolation-and-build.txt`
- `docs/evidence/2621/evidence.json`
- `docs/evidence/2621/pre-ready-2621.json`
- `scripts/verify-2621.sh`（R1/R2/R3 证据生成器）

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 共享解析器（1 谓词，31 调用点继承） | `backend/middleware/iam.go`、`backend/handlers/self_query_tenant_scope_2621_test.go` | `go test ./handlers -run 'TestSelfQueryTenantScope2621/ensure_local_user|local_user_id'` | `users.tenant_id=cacheTenant` ≠ ctx `tid=jwtTenant` | backend（一次性 PG 15433） | `TEST_EXIT=0`、`ENSURE_HIT=1`、`ENSURE_NO_NEW_ROW=1`、`LOCAL_USER_ID_HIT=1` | R1 |
| R2 7 处直连收口（9 改动点） | `backend/handlers/user_points.go`、`backend/handlers/pending_order.go`、`backend/handlers/face_capture.go`、`backend/handlers/user_address.go`、`backend/handlers/payment_calculate.go`、`backend/handlers/deposit_waiver.go`、`backend/handlers/customer_role.go` | `go test ./handlers -run 'TestSelfQueryTenantScope2621'` | 同上（缓存 tenant ≠ JWT tid） | backend（一次性 PG 15433） | `CASES_TOTAL=11`、`CASES_FAIL=0`、`POINTS_BALANCE_HTTP=200`、`POINTS_TX_HTTP=200`、`PENDING_ORDER_HTTP=200`、`FACE_CAPTURE_NOT_404=1`、`ADDRESSES_HTTP=200`、`PAY_CALC_NO_USER_ERROR=1`、`DEPOSIT_WAIVER_NOT_PROFILE_NOT_READY=1`、`CUSTOMER_ROLE_HTTP=200` | R2 |
| R3 隔离未放松 + 构建/编译冒烟 + 残留扫描 | `backend/handlers/self_query_tenant_scope_2621_test.go` | `bash scripts/verify-2621.sh r3` | 跨租户数据查询；修后工作树 | backend | `CROSS_TENANT_HIDDEN=1`、`DOMAIN_QUERY_STILL_SCOPED=1`、`GO_BUILD_EXIT=0`、`TEST_COMPILE_EXIT=0`、`RESIDUAL_PLAIN_CTX_SELF_QUERY=0` | R3 |

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 一次性 PG：`users` 行（`iam_sub=iam-sub-2621`、`tenant_id=cacheTenant`）+ `TestActor{TenantID: jwtTenant≠cacheTenant}` | `TestSelfQueryTenantScope2621/{ensure_local_user,local_user_id}` | `docs/evidence/2621/R1-resolver.txt` | `TEST_EXIT=0`、`ENSURE_HIT=1`、`ENSURE_NO_NEW_ROW=1`、`USERS_ROW_COUNT_DELTA=0`、`LOCAL_USER_ID_HIT=1` | `EnsureLocalUser` 返回 error、或新建了第二行 `users`（唯一冲突/重复建户）、`LocalUserID` 返回空 → 任一即 FAIL |
| R2 | 同上 + IAM mock + 9 个端点各自的最小请求体（积分/地址/人脸/支付计算/押金减免/待处理订单/顾客身份） | `TestSelfQueryTenantScope2621`（11 子用例） | `docs/evidence/2621/R2-handler-sites.txt` | `TEST_EXIT=0`、`CASES_TOTAL=11`、`CASES_FAIL=0`、`POINTS_BALANCE_HTTP=200`、`POINTS_TX_HTTP=200`、`PENDING_ORDER_HTTP=200`、`FACE_CAPTURE_NOT_404=1`、`ADDRESSES_HTTP=200`、`PAY_CALC_NO_USER_ERROR=1`、`DEPOSIT_WAIVER_NOT_PROFILE_NOT_READY=1`、`CUSTOMER_ROLE_HTTP=200` | 任一端点仍 404 `user not found`/`40400`、地址落 `500 user sync failed`、押金减免误报 `profile_not_ready`、`customer_role` 仍按 `id` 查 → 任一即 FAIL |
| R3 | 跨租户数据行（tenant A/B 各 1 行 `users`）+ 修后工作树 | `bash scripts/verify-2621.sh r3` | `docs/evidence/2621/R3-isolation-and-build.txt` | `CROSS_TENANT_HIDDEN=1`、`DOMAIN_QUERY_STILL_SCOPED=1`、`GO_BUILD_EXIT=0`、`TEST_COMPILE_EXIT=0`、`RESIDUAL_PLAIN_CTX_SELF_QUERY=0` | 跨租户可见、域查询（积分批次/礼品策略/押金评估）失去租户过滤、构建或测试包编译失败、残留 `WithContext(ctx)` 自查询 → 任一即 FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2621.sh r1 > docs/evidence/2621/R1-resolver.txt` | `docs/evidence/2621/R1-resolver.txt` | exit 0 且含 `TEST_EXIT=0` `ENSURE_HIT=1` `ENSURE_NO_NEW_ROW=1` `USERS_ROW_COUNT_DELTA=0` `LOCAL_USER_ID_HIT=1` | PASS（5 项 token 全为期望值） | 返回 error、重复建户、`LocalUserID` 空 → FAIL | dev |
| R2 | `bash scripts/verify-2621.sh r2 > docs/evidence/2621/R2-handler-sites.txt` | `docs/evidence/2621/R2-handler-sites.txt` | exit 0 且含 `TEST_EXIT=0` `CASES_TOTAL=11` `CASES_FAIL=0` `POINTS_BALANCE_HTTP=200` `POINTS_TX_HTTP=200` `PENDING_ORDER_HTTP=200` `FACE_CAPTURE_NOT_404=1` `ADDRESSES_HTTP=200` `PAY_CALC_NO_USER_ERROR=1` `DEPOSIT_WAIVER_NOT_PROFILE_NOT_READY=1` `CUSTOMER_ROLE_HTTP=200` | PASS（11 项 token 全为期望值） | 任一端点仍 404/500/误报、用例数 <11 → FAIL | dev |
| R3 | `bash scripts/verify-2621.sh r3 > docs/evidence/2621/R3-isolation-and-build.txt` | `docs/evidence/2621/R3-isolation-and-build.txt` | exit 0 且含 `CROSS_TENANT_HIDDEN=1` `DOMAIN_QUERY_STILL_SCOPED=1` `GO_BUILD_EXIT=0` `TEST_COMPILE_EXIT=0` `RESIDUAL_PLAIN_CTX_SELF_QUERY=0` | PASS（5 项 token 全为期望值） | 跨租户可见、域查询失去过滤、构建/编译失败、残留自查询 → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2621.sh`（`r1`/`r2`/`r3`）；每个文本 artifact 首行 `# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，随后 `GENERATED_AT_HEAD=<40 位 sha>`；证据必须在 fix commit **之后**生成（`FIX_REF` 可覆盖锚点，默认 `git log --grep="^fix(issue-2621)"`）。
- 重命令（`go test` / `go build`）一律经 `~/.config/opencode/scripts/heavy-run.sh` 取锁；测试库用一次性 PostgreSQL（`127.0.0.1:15433`），**禁止**使用 `jobmaster-postgres` 的 dev 库。
- 逐条清单（全部位于 `docs/evidence/2621/`）：
  - `docs/evidence/2621/R1-resolver.txt`
  - `docs/evidence/2621/R2-handler-sites.txt`
  - `docs/evidence/2621/R3-isolation-and-build.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2621 --summary-file tmp/pre-ready-2621-summary.md --changed-files tmp/pre-ready-2621-changed.txt --evidence-file docs/evidence/2621/evidence.json --report docs/evidence/2621/pre-ready-2621.json`
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Input Truth & Tool Preflight

- **Source of truth**：
  - 收口点清单 = `grep -rn 'iam_sub = ?' backend --include=*.go | grep -v _test`（修前 9 个用户自查询点在本单范围；修后对应行必须走 `IdentityCtx`）；
  - 身份谓词 = `backend/database/db.go:222-224`（`IdentityCtx` 只清 TenantID 键）+ `#2620` 已建立的同款口径（`backend/handlers/user_staff.go:1425,1696`）；
  - ID 空间依据 = `backend/database/migrations/038_add_users_iam_sub.up.sql`（`iam_sub` 唯一索引）。
- **实施前差异核对（先跑，结果入 Work Summary）**：
  - `grep -c 'IdentityCtx' backend/middleware/iam.go` → 修前 **0**，修后 **3**（2 个 `EnsureLocalUser` + 1 个 `LocalUserID`）；
  - `grep -n 'GetDB().WithContext(ctx).Where("iam_sub' backend/handlers/*.go | wc -l` → 修前 ≥7，修后 **0**；
  - `grep -rn 'database.SetTenantID' backend/handlers/self_query_tenant_scope_2621_test.go | wc -l` → 修后 ≥1（证明用 `TestActor`）。
- **损坏输入 / 边界拒绝规则**：
  - 匿名（`GetUserID` 空）分支保持 401/404 原样，**不得**因本修复变成 200；
  - 本地确实无行（`iam_sub` 不存在）→ 保持原错误/降级语义（`LocalUserID` 返回空、`EnsureLocalUser` 建 shadow user）；
  - 域数据查询（积分批次/礼品策略/押金评估/人脸批次）必须继续被自动租户过滤（R3 `DOMAIN_QUERY_STILL_SCOPED=1`）。
- **工具盲区**：
  - handler 直调测试不经 nginx/IAM 真实实例；预生产真机复验归部署侧 `[Ops-Followup]`；
  - `customer_role.go` 的 `id → iam_sub` 改法对「本地 id == iam_sub」的历史账号行为不变（两值相同），需在 Work Summary 记录该等价性；
  - 全量 `go test ./...` 不作合同行（P5 baseline 差分承担），R3 只跑 `-run '^$'` 编译冒烟 + `go build ./...`。
- **契约影响**：无 API 契约变更（仅修正不该出现的 404/500 与静默空结果）；`docs/spec/api/api.md` 无需修改 → 无 `## Core Update`。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 单一谓词：身份查询（按 JWT `sub` 查「我自己那行」）固定用 `database.IdentityCtx`；共享解析器内部实现，31 个调用点不各写 fallback | 本计划锁定 |
| D2 | **域查询不换句柄**：只有 `users` 行查询换成身份句柄；积分/批次/人脸/押金/礼品/订单等域查询继续用原 `db`（隔离不放宽） | 本计划锁定（R3 守卫） |
| D3 | `client_role.go` 的 ID 空间一并修正：按 `iam_sub` 查（不再用 `id`），并断言「本地 id ≠ iam_sub」的账号也能命中 | 本计划锁定 |
| D4 | `EnsureLocalUser` 的建户语句与归属赋值（`GetTenantID/GetOrgID(ctx)`）**不改**；只改查询 | 本计划锁定 |
| D5 | 不改 `face_verify.go` / `user_onboarding.go` / `iam_proxy.go` / `merchant.go` / `instrument_media.go` 的 `UPDATE` 型自写（GORM Update 无自动 scope，非本缺陷） | 背景第 8 条 |
| D6 | 测试必须用 `testutil.TestActor`（写 `database` 键）；只注入 `middleware` 键的用例对自动 scope 无效，不得作为本单证据 | 背景第 10 条 |
| D7 | 若扁平化改动导致既有用例失败且该用例固化旧缺陷 → 必须翻转并列出（文件:行 / 旧→新），不得静默跳过 | 语义变更纪律 |
| D8 | 角色/缓存数据纠正（`users.role/tenant_id` 回填）不在本单 → #2527 / #2553–#2557 / #2622 族 | 本计划锁定 |
| D9 | 预生产真机复验（积分/地址/人脸/支付计算路径）归部署侧 → closing 时建 `[Ops-Followup of #2621]`（`waitForConfirm`） | 本计划锁定 |
| D10 | `property.go:73`（`Table("users")` 无 Schema，自动 scope 不注入）不改，只在 R3 加回归守卫断言其行为未变 | 背景第 9 条 |

### Out of Scope

- `/users/me/*`（#2620 已修）。
- `users.tenant_id/role` 缓存行纠正与回填（#2527 / #2553–#2557）；对账工具（#2622 族：#2623/#2624/#2625）。
- 角色权威来源与 `functional_roles` 优先级（#2626）。
- `UPDATE` 型自写（`face_verify.go` / `user_onboarding.go` / `iam_proxy.go` / `merchant.go` / `instrument_media.go`）——非本缺陷路径。
- `addTenantScope` 的全局语义调整（按表/按列隐式豁免）与 `tenant_id` 体系废弃（#2255）。
- 前端（PC / H5 / weapp）零改动。
- 预生产/生产环境的真机复验（`[Ops-Followup of #2621]`）。
- 全量 `go test ./...` 作为合同行（由 pre-ready P5 baseline 差分承担）。

### Dependencies

- #2620

### Workload

- Size: L
- Basis: 8 个 Go 实现文件（1 middleware + 7 handler，9 个改动点）+ 1 测试文件 + 1 证据脚本；3 条证据行（R1/R2/R3）；无 migration、无 API/前端变更
- Confidence: medium

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状（symptom） | 商户/员工上下文账号（缓存 tenant ≠ JWT tid）打开积分、地址、人脸核身、支付计算、押金减免等页面 → 404 `user not found` / 500 `user sync failed` / 静默空列表 | 本单正文；#2620 已复现实例 |
| 2 | 触发条件（trigger） | 身份自查询走带 `tid` 的句柄，且 `users.tenant_id` ≠ JWT `tid` | 背景第 1、5 条 |
| 3 | 根因（root cause） | 「查我自己那行」被当成租户数据查询；共享解析器也用调用方句柄 → 一个错误被 31 个调用点放大 | 背景第 3、4 条 |
| 4 | 测试盲点（test blind spot） | 既有 handler 测试只注入 `middleware` 键，自动 scope 不执行；共享解析器无「缓存 tenant ≠ tid」用例 | 背景第 10 条 |
| 5 | 同类扫描（same-class paths） | `grep` 枚举全部 `iam_sub` 自查询；本单覆盖 9 点，`UPDATE` 型 5 处显式排除（有 GORM 回调证据） | 背景第 5、8、9 条 |
| 6 | 修复层（fix layer） | 共享谓词（middleware）+ 直连点收口；域查询不动 → 隔离面不变 | D1/D2 |
| 7 | 红绿实证（red-green） | 红：只落测试时 10 子用例 FAIL（404/静默空/误报 `profile_not_ready`）；绿：收口后 11/11 PASS | 变更点 3 |
| 8 | 防复发（prevention） | ① 共享解析器单点实现（31 点继承）；② 负例 `CROSS_TENANT_HIDDEN` + `DOMAIN_QUERY_STILL_SCOPED`（隔离不放宽）；③ 残留静态扫描 `RESIDUAL_PLAIN_CTX_SELF_QUERY=0`；④ ID 空间用例（本地 id ≠ iam_sub） | R1/R2/R3、D3 |
| 9 | 影响面（blast radius） | 9 个身份查询点 + 31 个共享调用点的「命中率」；域查询、隔离、成功路径字段不变 | R2/R3 |
| 10 | 未修边界（deferred） | 缓存行纠正 #2527、对账族 #2622、角色权威 #2626、`UPDATE` 型自写、真机复验 `[Ops-Followup]` | Out of Scope、D5/D8/D9 |

*Model: deepseek/deepseek-flash*


