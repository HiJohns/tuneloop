# 测试夹具审计（#2264 批次 3）

## 已替换（partial → 全表重建）

- `handlers/user_rental_test.go` / `handlers/user_rental_1762_test.go`：删除 `setupUserRentalTables`（仅 Drop/Create 3 表），统一改用 `testfixtures.SetupTestDB`（7 处）。
- 新增 `testutil.SeedVerifiedCustomer` / `testutil.SeedSiteMember`，供下单/维修列表用例自包含（批次 2 已实际调用）。

## 仍存在的 partial Drop/Create（登记子清单，本批不替换）

| 文件 | 类型 | 结论 |
|---|---|---|
| `appeal_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `banner_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `coupon_permille_1751_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `dashboard_stats_2005_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `deposit_waiver_1867_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `iam_integration_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `iam_proxy_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `integration_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `mall_image_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `membership_flow_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `merchant_order_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `migrate_banner_webp_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `notification_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `order_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `prepay_coupon_snapshot_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `prepay_shortfall_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `refund_appeal_notify_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `renewal_1781_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `renewal_1802_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `renewal_prepay_days_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `renewal_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `repair_request_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `state_machine_e2e_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `user_id_photos_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `user_management_idcard_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `user_me_tenant_name_2117_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `warehouse_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |
| `weapp_staff_flow_test.go` | 真实 DB partial | 留待子清单：替换为 SetupTestDB 会显著增加耗时/改动既有期望，另行排期 |

## 结论

- 本 Issue 已消除「前序用例残留数据」这一**主要**顺序耦合来源（下单/维修列表用例自包含）。
- 剩余 28 个文件为「按需建表」的局部夹具：虽仍是 partial，但均在同一用例内先 Drop 再 Create（不依赖前序数据），风险低于 `setupUserRentalTables` 的“只建 3 表、其余表残留”。统一迁移另立子 Issue，避免本 Issue 无限扩大。

## 批次 4 收口（sqlmock 期望泄漏 / 响应解码）

- `handlers/instrument_update_test.go` `TestUpdateInstrument`：改用 `testfixtures.SetupTestDB` 真实库（seed `categories` / `instrument_levels` / `instruments`），删除对 SQL 文本的 sqlmock 期望，避免处理器新增查询导致「expectations 已耗尽」。
  - 子用例按 `docs/spec/api.md` §5.5.2 partial update 重写：404 not found；仅 `description` / 仅 `level_id` / 仅 `category_id` 更新且其他字段不变；非法 `category_id` → 400。
  - 原「缺 name/level/category_id 必须 400」断言作废：`UpdateInstrumentRequest` 无 `name` 字段，partial update 允许省略 `level_id` / `category_id`。
  - `TestUpdateInstrumentPricingField` 保持 sqlmock（回归通过）。
- `handlers/instrument_import_test.go` `TestAPIErrorResponses`：删除无用的 sqlmock/DB 初始化；响应改为 `json.Unmarshal(w.Body...)`（原 `ctx.ShouldBindJSON` 读请求体，恒为 nil）；「missing file」子用例补 tenant ctx 以命中 40002。
- shuffle 残余（三 seed 基线）已由 **#2271** 承接并关闭；本 Issue 不再重复。

## 独立测试库与 shuffle 回归（#2271）

- 测试库按节点隔离：`TUNELOOP_TEST_DB` 优先；否则 `tuneloop_test_<OPENCODE_NODE_ID>`（见 #2288）。
- 并行节点示例：
  ```bash
  TUNELOOP_TEST_DB=tuneloop_test_node2 go test ./handlers -count=1
  TUNELOOP_TEST_DB=tuneloop_test_3134 go test ./handlers -count=1
  ```
- shuffle 回归（#2271）：
  ```bash
  # 必须显式节点/库（脚本拒绝默认落到他节点库；#2288）
  OPENCODE_NODE_ID=node-2 scripts/test-shuffle.sh --update-baseline 20261023 42 7777
  # 或指定 TUNELOOP_TEST_DB=tuneloop_test_xxx
  # 基线缺失时自动跑非 shuffle 全量生成 tmp/test-baseline.txt；也可单独 --update-baseline
  ```
  脚本对每个 seed 输出 NEW/MISSING；存在 NEW 即退出 1。日志在 `tmp/shuffle-2271/`。
- 夹具守卫：`grep -rn "Migrator().DropTable\|Migrator().CreateTable" backend/handlers/*_test.go` = **0**（testfixtures 除外）；残留两处已迁移（`iam_proxy` → SetupTestDB；`dashboard_stats` → 可逆表重命名故障注入）。
