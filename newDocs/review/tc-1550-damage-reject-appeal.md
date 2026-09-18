# TC-1550 Review — Damage flow 拒绝+申诉

> TC Issue: #1550 | 生成日期: 2026-08-07 | 类型: 后端测试

## 1. 目标

验证顾客拒绝定损 → 进入申诉流程 → 员工重审定损结果。

## 2. 验证方式

- [x] 后端测试: `go test -run TestScenarioA_RejectDamageVariant ./handlers/ -v`

## 3. 流程步骤与证据

### Step 1: 验收损坏 → 拒绝 → 申诉

**代码证据**:
- `backend/handlers/warehouse.go:420-422` — InspectReturn damaged → pending_damage_response
- `backend/handlers/order.go:1442` — RejectDamage → 创建 Appeal 记录

**测试证据**:
```
--- PASS: TestScenarioA_RejectDamageVariant (1.13s)
  子测试: InspectDamaged, RejectDamage
```

## 4. 结论

| 检查项 | 结果 | 证据 |
|--------|:---:|------|
| 验收 damaged → pending_damage_response | ✅ | InspectDamaged |
| 拒绝定损 → 创建 Appeal | ✅ | RejectDamage |
| 申诉记录创建 | ✅ | Appeal 表 |
| 订单状态保留 | ✅ | pending_damage_response |

**总判定**: ✅ 通过

---

*Model: deepseek/deepseek-v4-flash*
