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
