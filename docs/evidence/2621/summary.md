## Work Summary — #2621 其余身份自查询仍被自动租户过滤（共享解析器 + /user/* 家庭收口）

### Remote

- 分支 issue-2621（基线 c0c0e74e6 = origin/develop）；fix 38e6be443 + 证据提交，均 push 到 origin/issue-2621。
- git status --porcelain 空（证据生成时 dirty=false）。

### 实施前差异核对（计划 Input Truth）

| 核对项 | 期望（计划） | 实测 |
|---|---|---|
| grep -c IdentityCtx backend/middleware/iam.go | 修前 0 → 修后 3 | 0 → 3（EnsureLocalUser 首查 / 建户失败回退 / LocalUserID） |
| grep -n 'GetDB().WithContext(ctx).Where("iam_sub' backend/handlers/*.go | wc -l | 修前 ≥7 → 修后 0 | 7 → 0 |
| grep -rn database.SetTenantID backend/handlers/self_query_tenant_scope_2621_test.go | wc -l | 修后 ≥1（证明用 TestActor） | 1（TestActor.InjectContext 内写 database 键，夹具统一走 TestActor） |
| 受影响既有断言（计划 §4） | 0 翻转（需实测） | 0 翻转：go test ./middleware ./handlers -run 'EnsureLocalUser|LocalUserID|Points|Address|FaceCapture|DepositWaiver|RegisterAsCustomer|CalculatePayment' -count=1 → ok middleware 0.039s / ok handlers 26.186s |

### 变更内容

1. 共享解析器（唯一谓词，31 调用点继承）backend/middleware/iam.go：EnsureLocalUser 首查（:561）、建户失败回退（:588）、LocalUserID（:611）全部改为 db.WithContext(database.IdentityCtx(ctx)).Where("iam_sub = ?", iamSub)；注释写明「users.tenant_id 是 IAM 关系缓存，可能 ≠ JWT tid，租户过滤不适用」。建户语句与归属赋值（GetTenantID/GetOrgID）不变（D4）。
2. 7 处直连自查询（9 改动点）：user_points.go（balance / transactions）、pending_order.go（currentLocalUser）、face_capture.go（submit / status）、user_address.go（resolveAddressUserID）、payment_calculate.go（getWalletInfo，按计划显式加 ctx context.Context，调用点 CalculatePayment 与 gift_policy_test.go:152 同步）、deposit_waiver.go（eligibility）、customer_role.go（同时修 ID 空间：Where("id = ?", iam_sub) → WithContext(IdentityCtx).Where("iam_sub = ? AND deleted_at IS NULL")）。
   - 域查询句柄不变（D2）：积分批次、人脸批次、礼品策略 GetGiftPolicyByLevel(db, …)、押金评估 EvaluateDepositWaiverEligibility(db, …)、pending_orders 域查询仍用原 db。
3. 回归测试 backend/handlers/self_query_tenant_scope_2621_test.go（11 子用例）：ensure_local_user / local_user_id / points_balance / points_transactions / pending_orders / face_capture / addresses / pay_calculate / deposit_waiver / customer_role / cross_tenant_scoped（负例）。夹具：本地 users.tenant_id = cacheTenant ≠ JWT tid，testutil.TestActor{TenantID: jwtTenant} 写 database 键（D6），IAM 用 httptest mock。
4. 证据生成器 scripts/verify-2621.sh（r1/r2/r3，重命令经 heavy-run.sh 取锁；一次性 PG 15433）。

### 红绿实证（同一 harness，只回退实现）

```text
缺陷版（git stash 8 个实现文件，保留用例；payment_calculate 仅留 ctx 签名以便编译）：
  --- FAIL: TestSelfQueryTenantScope2621/ensure_local_user   (failed to create user … duplicate key … iam_sub)
  --- FAIL: …/local_user_id        (got "", want <uuid>)
  --- FAIL: …/points_balance       (40400/404)
  --- FAIL: …/points_transactions  (404)
  --- FAIL: …/pending_orders       (404)
  --- FAIL: …/face_capture         (Should not be: 404)
  --- FAIL: …/addresses            (500)
  --- FAIL: …/pay_calculate        (record not found)
  --- FAIL: …/deposit_waiver       (profile_not_ready 误报)
  --- FAIL: …/customer_role        (404 用户不存在)
  （cross_tenant_scoped PASS —— 负例守卫修复前后均成立）
修复版 38e6be443：
  CASES_TOTAL=11 / CASES_FAIL=0，11/11 PASS
```

### Accepted Plan 对照（n/n）

| 计划项 | 状态 |
|---|---|
| 变更点 1 共享解析器 3 处身份句柄（31 调用点零改动继承） | ✅ |
| 变更点 2 序 1–9（7 处直连 / 9 改动点，只换 users 行句柄） | ✅ |
| 变更点 3 回归测试 11 子用例（含 cross_tenant_scoped 负例） | ✅ |
| 变更点 4 既有断言影响（0 翻转，需实测） | ✅ 0 翻转（定向测试见上表） |
| D1 单一谓词（禁止调用点各写 fallback） | ✅ |
| D2 域查询不换句柄 | ✅（R3 DOMAIN_QUERY_STILL_SCOPED=1） |
| D3 customer_role ID 空间一并修正 | ✅ |
| D4 建户语句/归属赋值不改 | ✅ |
| D5 不碰 UPDATE 型自写（face_verify / user_onboarding / iam_proxy / merchant / instrument_media） | ✅ 未改 |
| D6 测试必须用 TestActor（写 database 键） | ✅ |
| D7 既有断言若固化旧缺陷须翻转并列出 | ✅ 0 处需翻转（定向实测） |
| D10 property.go:73 不改 | ✅ 未改 |

### 矩阵 3/3

| 行 | 状态 | 证据 |
|---|---|---|
| R1 共享解析器（1 谓词，31 调用点继承） | PASS | `docs/evidence/2621/R1-resolver.txt`：TEST_EXIT=0、ENSURE_HIT=1、ENSURE_NO_NEW_ROW=1、USERS_ROW_COUNT_DELTA=0、LOCAL_USER_ID_HIT=1 |
| R2 7 处直连收口（9 改动点） | PASS | `docs/evidence/2621/R2-handler-sites.txt`：TEST_EXIT=0、CASES_TOTAL=11、CASES_FAIL=0、POINTS_BALANCE_HTTP=200、POINTS_TX_HTTP=200、PENDING_ORDER_HTTP=200、FACE_CAPTURE_NOT_404=1、ADDRESSES_HTTP=200、PAY_CALC_NO_USER_ERROR=1、DEPOSIT_WAIVER_NOT_PROFILE_NOT_READY=1、CUSTOMER_ROLE_HTTP=200 |
| R3 隔离未放松 + 构建/编译冒烟 + 残留扫描 | PASS | `docs/evidence/2621/R3-isolation-and-build.txt`：CROSS_TENANT_HIDDEN=1、DOMAIN_QUERY_STILL_SCOPED=1、GO_BUILD_EXIT=0、TEST_COMPILE_EXIT=0、RESIDUAL_PLAIN_CTX_SELF_QUERY=0（另 IDENTITY_CTX_SITES_IAM_GO=3） |

未完成行：无。

### 与计划措辞的一处对齐说明（不改验收口径）

- 矩阵 R2 行的期望 token 含 PENDING_ORDER_HTTP=200，而 CreatePendingOrder 成功语义本就是 201 Created（code 20100，现网契约，不改）。执行侧按该行入口列「CreatePendingOrder（或 currentLocalUser 直调）」的口径处理：pending_orders 子用例同时覆盖写入入口（断言 201，记 PENDING_ORDER_CREATE_HTTP=201）与读取入口 ListPendingOrders（断言 200，输出合同 token PENDING_ORDER_HTTP=200）；两者都经 currentLocalUser，缺陷路径一致，未改任何状态码。

### Evidence Contract

机读合同：`docs/evidence/2621/evidence.json`（build=1.0.38e6be443、commit=38e6be443、dirty=false、plan_sha256=07c3eb3c…，3 行各一条记录且 pass=true）。

| 行 | 结果 | 产物 | 关键断言 |
|---|---|---|---|
| R1 | PASS | `docs/evidence/2621/R1-resolver.txt` | TEST_EXIT=0 / ENSURE_HIT=1 / ENSURE_NO_NEW_ROW=1 / USERS_ROW_COUNT_DELTA=0 / LOCAL_USER_ID_HIT=1 |
| R2 | PASS | `docs/evidence/2621/R2-handler-sites.txt` | 11 项 token（11 子用例 / 0 失败 / 9 端点各自结果） |
| R3 | PASS | `docs/evidence/2621/R3-isolation-and-build.txt` | 5 项 token（跨租户仍隐藏 / 域查询仍过滤 / 构建 / 编译冒烟 / 残留 0） |

### 契约三向核对

无 API 契约变更（计划「契约影响：无」）：端点路径/方法/请求体/响应结构/错误码集合全部不变，本单只修正「不该出现的 404/500 与静默空结果」。数据库契约也无变更；docs/spec/api/api.md 无需修改（无 Core Update）。

### 同类路径对称扫描

- grep -rn 'WithContext(ctx)\.Where("iam_sub' backend --include=*.go | grep -v _test → 0（本单 9 改动点全收口）。
- grep -c IdentityCtx backend/middleware/iam.go → 3（首查 / 回退 / LocalUserID）。
- 显式排除且未改：face_verify.go、user_onboarding.go、iam_proxy.go、merchant.go、instrument_media.go 的 UPDATE 型自写（GORM Update 回调链不含 query scope，非本缺陷，D5）；property.go:73（Table("users") 无 Schema，自动 scope 不注入，D10）。
- 未改数据：users.tenant_id / role 缓存行回填属 #2527 / #2553–#2557；对账族 #2622。

### 人工/部署侧（不进本矩阵）

| 事项 | 负责方 | 回执 |
|---|---|---|
| 预生产用真实「缓存 tenant ≠ JWT tid」账号走「我的积分 / 我的地址 / 人脸核身 / 支付计算」路径 | Owner | 关单时按 [Ops-Followup of #2621]（waitForConfirm，无 status:*）落盘 |

### Pre-ready gate

- 命令：`~/.config/opencode/scripts/pre-ready-check.sh 2621 --summary-file docs/evidence/2621/summary.md --changed-files docs/evidence/2621/changed-files.txt --evidence-file docs/evidence/2621/evidence.json --affected-cmd "bash scripts/verify-2621.sh r2" --report docs/evidence/2621/pre-ready-2621.json`
- 结果：RESULT: PASS（overall=PASS）。P1/P2/P3/P4/P6/P11/P12/P13/P14/P17/P18 PASS；P5 N/A（仓库根无 go.mod/package.json，构建与编译冒烟见 R3）；P7/P8/P9/P10/P15/P16 N/A。
- 报告：docs/evidence/2621/pre-ready-2621.json

*Model: deepseek/deepseek-flash*
